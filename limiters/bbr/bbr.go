package bbr

import (
	"context"
	"fmt"
	"math"
	"os"
	"sync/atomic"
	"time"

	limit "github.com/yeqown/ratelimit"
	cpustat "github.com/yeqown/ratelimit/pkg/cpu"
	"github.com/yeqown/ratelimit/pkg/metric"
)

var (
	cpu         int64
	decay       = 0.95
	initTime    = time.Now()
	defaultConf = &Config{
		Window:       time.Second * 10,
		WinBucket:    100,
		CPUThreshold: 800,
	}
)

type cpuGetter func() int64

func init() {
	go cpuproc()
}

// cpu = cpuᵗ⁻¹ * decay + cpuᵗ * (1 - decay)
func cpuproc() {
	ticker := time.NewTicker(time.Millisecond * 250)
	defer func() {
		ticker.Stop()
		if err := recover(); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "rate.limit.cpuproc() err(%+v)", err)
			go cpuproc()
		}
	}()

	// EMA algorithm: https://blog.csdn.net/m0_38106113/article/details/81542863
	for range ticker.C {
		stat := &cpustat.Stat{}
		cpustat.ReadStat(stat)
		pre := atomic.LoadInt64(&cpu)
		cur := int64(float64(pre)*decay + float64(stat.Usage)*(1.0-decay))
		atomic.StoreInt64(&cpu, cur)
	}
}

// Stats contains the metrics' snapshot of bbr.
type Stat struct {
	CPU         int64
	InFlight    int64
	MaxInFlight int64
	MinRT       int64 // the minimum RT
	MaxPass     int64 // the maximum ?
}

// BBR implements bbr-like limiter.
// It is inspired by sentinel.
// https://github.com/alibaba/Sentinel/wiki/%E7%B3%BB%E7%BB%9F%E8%87%AA%E9%80%82%E5%BA%94%E9%99%90%E6%B5%81
type BBR struct {
	cpu             cpuGetter
	passStat        metric.RollingCounter
	rtStat          metric.RollingCounter
	inFlight        int64
	winBucketPerSec int64
	conf            *Config
	prevDrop        atomic.Value
	prevDropHit     int32
	rawMaxPASS      int64
	rawMinRt        int64
}

// Config contains configs of bbr limiter.
type Config struct {
	Enabled      bool
	Window       time.Duration
	WinBucket    int
	Rule         string
	Debug        bool
	CPUThreshold int64
}

func (l *BBR) maxPASS() int64 {
	rawMaxPass := atomic.LoadInt64(&l.rawMaxPASS)
	if rawMaxPass > 0 && l.passStat.Timespan() < 1 {
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
	atomic.StoreInt64(&l.rawMaxPASS, rawMaxPass)

	return rawMaxPass
}

// minRT get minimum RT from rtStat
func (l *BBR) minRT() int64 {
	rawMinRT := atomic.LoadInt64(&l.rawMinRt)
	if rawMinRT > 0 && l.rtStat.Timespan() < 1 {
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
	atomic.StoreInt64(&l.rawMinRt, rawMinRT)

	return rawMinRT
}

func (l *BBR) maxFlight() int64 {
	return int64(math.Floor(float64(l.maxPASS()*l.minRT()*l.winBucketPerSec)/1000.0 + 0.5))
}

func (l *BBR) shouldDrop() bool {
	if l.cpu() < l.conf.CPUThreshold {
		prevDrop, _ := l.prevDrop.Load().(time.Duration)
		if prevDrop == 0 {
			return false
		}

		if time.Since(initTime)-prevDrop <= time.Second {
			if atomic.LoadInt32(&l.prevDropHit) == 0 {
				atomic.StoreInt32(&l.prevDropHit, 1)
			}
			inFlight := atomic.LoadInt64(&l.inFlight)
			return inFlight > 1 && inFlight > l.maxFlight()
		}
		l.prevDrop.Store(time.Duration(0))

		return false
	}

	inFlight := atomic.LoadInt64(&l.inFlight)
	drop := inFlight > 1 && inFlight > l.maxFlight()
	if drop {
		prevDrop, _ := l.prevDrop.Load().(time.Duration)
		if prevDrop != 0 {
			return drop
		}
		l.prevDrop.Store(time.Since(initTime))
	}

	return drop
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
// Once overload is detected, it raises ecode.LimitExceed error.
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

// NewLimiter 创建一个limiter
func NewLimiter(conf *Config) limit.Limiter {
	if conf == nil {
		conf = defaultConf
	}
	size := conf.WinBucket
	bucketDuration := conf.Window / time.Duration(conf.WinBucket)
	passStat := metric.NewRollingCounter(metric.RollingCounterOpts{Size: size, BucketDuration: bucketDuration})
	rtStat := metric.NewRollingCounter(metric.RollingCounterOpts{Size: size, BucketDuration: bucketDuration})
	cpu := func() int64 {
		return atomic.LoadInt64(&cpu)
	}
	limiter := &BBR{
		cpu:             cpu,
		conf:            conf,
		passStat:        passStat,
		rtStat:          rtStat,
		winBucketPerSec: int64(time.Second) / (int64(conf.Window) / int64(conf.WinBucket)),
	}

	return limiter
}
