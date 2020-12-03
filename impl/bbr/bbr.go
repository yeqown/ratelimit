package bbr

import (
	"context"
	"math"
	"sync/atomic"
	"time"

	limit "github.com/yeqown/ratelimit"
	rw "github.com/yeqown/ratelimit/internal/rolling-window"
)

var (
	_InitTime = time.Now()
)

// BBR implements bbr-like limiter, it is inspired by sentinel.
// https://github.com/alibaba/Sentinel/wiki/%E7%B3%BB%E7%BB%9F%E8%87%AA%E9%80%82%E5%BA%94%E9%99%90%E6%B5%81
type BBR struct {
	conf *Config
	cpu  func() int64
	pass *rw.RollingWindow
	rtt  *rw.RollingWindow

	// inflight requests in dealing
	inflight int64

	// bps bucket count in rw.RollingWindow per second
	bps int64

	// prevDrop the previous dropped request's time gap from _InitTime
	// if this is not 0, means the system need to limit traffic
	prevDrop atomic.Value

	// rawMaxPass means BDP (Bandwidth Delayed Product)
	rawMaxPass int64

	// rawMinRT means minRTT
	rawMinRT int64
}

// NewLimiter create a limit.Limiter
func NewLimiter(conf *Config) limit.Limiter {
	comptaibleConfig(conf)

	bucketDuration := conf.Window / time.Duration(conf.WinBucket)

	limiter := &BBR{
		conf:     conf,
		cpu:      cpugetter,
		pass:     rw.NewRollingWindow(conf.WinBucket, bucketDuration),
		rtt:      rw.NewRollingWindow(conf.WinBucket, bucketDuration),
		inflight: 0,
		bps:      int64(time.Second) / (int64(conf.Window) / int64(conf.WinBucket)), // 1 / 10 * 1000 = 100
	}

	// continuously get cpu load
	go cpuproc()

	return limiter
}

func (l *BBR) maxPASS() int64 {
	rawMaxPass := atomic.LoadInt64(&l.rawMaxPass)
	if rawMaxPass > 0 && l.pass.TimeSpan() < 1 {
		// just in one bucket duration, no need to do Stat again
		return rawMaxPass
	}

	// stat from window.buckets
	r := 1.0
	l.pass.Iterate(func(b *rw.Bucket) {
		math.Max(r, float64(b.Count()))
	})

	rawMaxPass = int64(r)
	if rawMaxPass == 0 {
		rawMaxPass = 1
	}
	atomic.StoreInt64(&l.rawMaxPass, rawMaxPass)

	return rawMaxPass
}

// minRTT get minimum RT from rtt
func (l *BBR) minRTT() int64 {
	rawMinRT := atomic.LoadInt64(&l.rawMinRT)
	if rawMinRT > 0 && l.rtt.TimeSpan() < 1 {
		return rawMinRT
	}

	r := math.MaxFloat64
	l.rtt.Iterate(func(b *rw.Bucket) {
		// FIXME: avg is not so good. maybe p99 or p90
		r = math.Min(r, b.Avg())
	})

	rawMinRT = int64(math.Ceil(r))
	if rawMinRT <= 0 {
		rawMinRT = 1
	}
	atomic.StoreInt64(&l.rawMinRT, rawMinRT)

	return rawMinRT
}

// maxFlight = math.Floor((MaxPass * MinRT * WindowSize)/1000 + 0.5)
func (l *BBR) maxFlight() int64 {
	return int64(math.Floor(float64(l.maxPASS()*l.minRTT()*l.bps)/1000.0 + 0.5))
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

		if time.Since(_InitTime)-prevDrop <= time.Second {
			// if previous dropped request happened before and over than 1s.
			// this means to limit request

			//if atomic.LoadInt32(&l.prevDropHit) == 0 {
			//	// mark previousDropHit flag to true
			//	// FIXME: no place to reset and use?
			//	atomic.StoreInt32(&l.prevDropHit, 1)
			//}
			inFlight := atomic.LoadInt64(&l.inflight)
			return inFlight > 1 && inFlight > l.maxFlight()
		}

		// no need to limit and  drop request.
		// clear previous dropped request gap
		l.prevDrop.Store(time.Duration(0))
		return false
	}

	// cpu is exceed limit
	inFlight := atomic.LoadInt64(&l.inflight)
	drop := inFlight > 1 && inFlight > l.maxFlight()
	if drop {
		prevDrop, _ := l.prevDrop.Load().(time.Duration)
		if prevDrop == 0 {
			// update the prevDrop at when only the current request should drop and prevDrop is not exists
			l.prevDrop.Store(time.Since(_InitTime))
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
		InFlight:    atomic.LoadInt64(&l.inflight),
		MinRT:       l.minRTT(),
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

	atomic.AddInt64(&l.inflight, 1)
	start := time.Since(_InitTime)

	return func(do limit.DoneInfo) {
		rt := int64((time.Since(_InitTime) - start) / time.Millisecond)
		l.rtt.Add(rt)
		atomic.AddInt64(&l.inflight, -1)

		switch do.Op {
		case limit.Success:
			l.pass.Add(1)
			return
		default:
			return
		}
	}, nil
}
