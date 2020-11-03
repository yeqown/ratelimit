package bbr

import (
	"context"
	"math"
	"sync/atomic"
	"time"

	limit "github.com/yeqown/ratelimit"
	"github.com/yeqown/ratelimit/pkg/metric"
)

var (
	initTime    = time.Now()
	defaultConf = &Config{
		Window:       time.Second * 10,
		WinBucket:    100,
		CPUThreshold: 500, // cores * 2.5
	}
)

// Config contains configs of bbr limiter.
type Config struct {
	// Window .
	Window time.Duration

	// WinBucket .
	WinBucket int

	// CPUThreshold .
	CPUThreshold int64
}

// BBR implements bbr-like limiter.
// It is inspired by sentinel.
// https://github.com/alibaba/Sentinel/wiki/%E7%B3%BB%E7%BB%9F%E8%87%AA%E9%80%82%E5%BA%94%E9%99%90%E6%B5%81
type BBR struct {
	conf            *Config
	cpu             cpuGetter
	passStat        metric.RollingCounter
	rtStat          metric.RollingCounter
	inFlight        int64 // requests in dealing
	winBucketPerSec int64 //

	// prevDrop the previous dropped request's time gap from initTime
	// if this is not 0, means the system need to limit traffic
	prevDrop atomic.Value
	// prevDropHit shows is there a dropped request
	//prevDropHit int32

	// rawMaxPass means BDP (Bandwidth Delayed Product)
	rawMaxPass int64
	// rawMinRT means minRTT
	rawMinRT int64
}

// NewLimiter create a limit.Limiter
func NewLimiter(conf *Config) limit.Limiter {
	if conf == nil {
		conf = defaultConf
	}

	size := conf.WinBucket
	bucketDuration := conf.Window / time.Duration(conf.WinBucket)
	opt := metric.RollingCounterOpts{Size: size, BucketDuration: bucketDuration}
	passStat := metric.NewRollingCounter(opt)
	rtStat := metric.NewRollingCounter(opt)
	var getter cpuGetter = func() int64 {
		return atomic.LoadInt64(&cpu)
	}

	limiter := &BBR{
		cpu:             getter,
		conf:            conf,
		passStat:        passStat,
		rtStat:          rtStat,
		winBucketPerSec: int64(time.Second) / (int64(conf.Window) / int64(conf.WinBucket)), // 1 / 10 * 1000 = 100
	}

	return limiter
}

func (l *BBR) maxPASS() int64 {
	rawMaxPass := atomic.LoadInt64(&l.rawMaxPass)
	if rawMaxPass > 0 && l.passStat.TimeSpan() < 1 {
		return rawMaxPass
	}

	f := l.passStat.Reduce(func(iterator metric.Iterator) (r float64) {
		r = 1.0

		for i := 1; iterator.Next() && i < l.conf.WinBucket; i++ {
			bucket := iterator.Bucket()
			count := 0.0
			for _, p := range bucket.Points {
				count += p
			}

			r = math.Max(r, count)
		}

		return
	})

	rawMaxPass = int64(f)
	if rawMaxPass == 0 {
		rawMaxPass = 1
	}
	atomic.StoreInt64(&l.rawMaxPass, rawMaxPass)

	return rawMaxPass
}

// minRT get minimum RT from rtStat
// FIXME: minRT is not right
func (l *BBR) minRT() int64 {
	rawMinRT := atomic.LoadInt64(&l.rawMinRT)
	if rawMinRT > 0 && l.rtStat.TimeSpan() < 1 {
		return rawMinRT
	}

	f := l.rtStat.Reduce(func(iterator metric.Iterator) (r float64) {
		r = math.MaxFloat64

		for i := 1; iterator.Next() && i < l.conf.WinBucket; i++ {
			bucket := iterator.Bucket()
			if len(bucket.Points) == 0 {
				continue
			}
			total := 0.0
			for _, p := range bucket.Points {
				total += p
			}
			avg := total / float64(bucket.Count)
			r = math.Min(r, avg)
		}

		return r
	})

	rawMinRT = int64(math.Ceil(f))
	if rawMinRT <= 0 {
		rawMinRT = 1
	}

	atomic.StoreInt64(&l.rawMinRT, rawMinRT)

	return rawMinRT
}

// maxFlight = math.Floor((MaxPass * MinRT * WindowSize)/1000 + 0.5)
func (l *BBR) maxFlight() int64 {
	return int64(math.Floor(float64(l.maxPASS()*l.minRT()*l.winBucketPerSec)/1000.0 + 0.5))
}

// shouldDrop means is there need to limit request
// https://github.com/alibaba/sentinel-golang/blob/master/core/system/slot.go
func (l *BBR) shouldDrop() bool {
	if l.cpu() < l.conf.CPUThreshold {
		// cpu is less than conf.CPUThreshold, then get prevDrop
		prevDrop, _ := l.prevDrop.Load().(time.Duration)
		if prevDrop == 0 {
			// cpu is ok and no previous dropped request, just pass
			return false
		}

		if time.Since(initTime)-prevDrop <= time.Second {
			// if previous dropped request happened before and over than 1s.
			// this means to limit request
			//if atomic.LoadInt32(&l.prevDropHit) == 0 {
			//	// mark previousDropHit flag to true
			//	// FIXME: no place to reset and use?
			//	atomic.StoreInt32(&l.prevDropHit, 1)
			//}
			inFlight := atomic.LoadInt64(&l.inFlight)
			return inFlight > 1 && inFlight > l.maxFlight()
		}

		// no need to limit and  drop request.
		// clear previous dropped request gap
		l.prevDrop.Store(time.Duration(0))
		return false
	}

	// cpu is exceed limit
	inFlight := atomic.LoadInt64(&l.inFlight)
	drop := inFlight > 1 && inFlight > l.maxFlight()
	if drop {
		prevDrop, _ := l.prevDrop.Load().(time.Duration)
		if prevDrop == 0 {
			// update the prevDrop at when only the current request should drop and prevDrop is not exists
			l.prevDrop.Store(time.Since(initTime))
		}
	}

	return drop
}

// Stats contains the metrics' snapshot of bbr.
type Stat struct {
	CPU         int64 // CPU usage
	InFlight    int64 // count of requests in flight
	MaxInFlight int64 // the maximum count of requests could be handled by system
	MinRT       int64 // the minimum RT
	MaxPass     int64 // the maximum ?
}

// Stat tasks a snapshot of the bbr limiter.
func (l *BBR) Stat() Stat {
	return Stat{
		CPU:         l.cpu(),
		InFlight:    atomic.LoadInt64(&l.inFlight),
		MinRT:       l.minRT(),
		MaxPass:     l.maxPASS(),
		MaxInFlight: l.maxFlight(),
	}
}

// Allow checks all inbound traffic.
// Once overload is detected, it raises limit.ErrLimitExceed error.
func (l *BBR) Allow(ctx context.Context, opts ...limit.AllowOption) (func(info limit.DoneInfo), error) {
	allowOpts := limit.DefaultAllowOpts()
	for _, opt := range opts {
		opt.Apply(&allowOpts)
	}

	if l.shouldDrop() {
		return nil, limit.ErrLimitExceed
	}

	atomic.AddInt64(&l.inFlight, 1)
	start := time.Since(initTime)

	return func(do limit.DoneInfo) {
		rt := int64((time.Since(initTime) - start) / time.Millisecond)
		l.rtStat.Add(rt)
		atomic.AddInt64(&l.inFlight, -1)

		switch do.Op {
		case limit.Success:
			l.passStat.Add(1)
			return
		default:
			return
		}

	}, nil
}
