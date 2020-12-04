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
	conf     *Config
	cpu      func() int64
	complete *rw.RollingWindow

	// rt contains all completed requests round-trip time (millisecond).
	rt *rw.RollingWindow

	// inflight requests in dealing
	inflight int64

	// bps bucket count in rw.RollingWindow per second
	bps int64

	// prevDrop the previous dropped request's time gap from _InitTime
	// if this is not 0, means the system need to limit traffic
	prevDrop atomic.Value

	// rawMaxComplete means BDP (Bandwidth Delayed Product)
	rawMaxComplete int64

	// rawMinRT means minRTT of all request in past duration of window.
	rawMinRT int64
}

// New create a BBR limiter
func New(conf *Config) limit.Limiter {
	conf = compatibleConfig(conf)

	bucketDuration := conf.Window / time.Duration(conf.WinBucket)

	l := &BBR{
		conf:     conf,
		cpu:      cpugetter,
		complete: rw.NewRollingWindow(conf.WinBucket, bucketDuration),
		rt:       rw.NewRollingWindow(conf.WinBucket, bucketDuration),
		inflight: 0,
		bps:      int64(conf.WinBucket) / int64(conf.Window/time.Second),
	}

	// continuously get cpu load. init cpu = l.conf.CPUThreshold,
	// to start with low request.
	go cpuproc(l.conf.CPUThreshold)

	return l
}

func (l *BBR) maxComplete() int64 {
	c := atomic.LoadInt64(&l.rawMaxComplete)
	if c > 0 && l.complete.TimeSpan() < 1 {
		// just in one bucket duration, no need to do statForDebug again
		return c
	}

	// Stat from window.buckets
	r := 1.0
	l.complete.Iterate(func(b *rw.Bucket) {
		r = math.Max(r, float64(b.Count()))
	})

	c = int64(r)
	if c == 0 {
		c = 1
	}
	atomic.StoreInt64(&l.rawMaxComplete, c)

	return c
}

// minRTT get minimum RT from rt
func (l *BBR) minRTT() int64 {
	rawMinRTT := atomic.LoadInt64(&l.rawMinRT)
	if rawMinRTT > 0 && l.rt.TimeSpan() < 1 {
		return rawMinRTT
	}

	r := math.MaxFloat64
	l.rt.Iterate(func(b *rw.Bucket) {
		if b.Count() == 0 {
			return
		}
		// FIXME: avg is not so good. maybe p99 or p90
		r = math.Min(r, b.Avg())
	})

	rawMinRTT = int64(math.Ceil(r))
	if rawMinRTT <= 0 {
		rawMinRTT = 1
	}
	atomic.StoreInt64(&l.rawMinRT, rawMinRTT)

	return rawMinRTT
}

// https://github.com/alibaba/sentinel-golang/blob/master/core/system/slot.go
func (l *BBR) shouldDropV2() bool {
	cpu := l.cpu()
	if cpu > l.conf.CPUThreshold {
		if !l.checkSimple() {
			return true
		}
	}

	// if cpu is under of CPUThreshold, no strategy for this case.
	// TODO(@yeqown): do something or no need to do.

	return false
}

// maxFlight = math.Floor((MaxPass * MinRTT * WindowSize)/1000 + 0.5)
// estimate how many request could be accepted by server in 1 second.
func (l *BBR) maxFlight() float64 {
	return float64(l.maxComplete()*l.minRTT()) / 1000.0
}

func (l *BBR) checkSimple() bool {
	concurrency := atomic.LoadInt64(&l.inflight)
	minRt := l.minRTT()
	maxComplete := l.maxComplete()
	if concurrency > 1 && float64(concurrency) > float64(maxComplete*minRt)/1000.0 {
		//if concurrency > 1 && float64(concurrency) > l.maxFlight() {
		return false
	}

	return true
}

//
//// shouldDrop means is there need to limit request.
//func (l *BBR) shouldDrop() bool {
//	if l.cpu() < l.conf.CPUThreshold {
//		// cpu is less than conf.CPUThreshold, then get prevDrop
//		prevDrop, _ := l.prevDrop.Load().(time.Duration)
//		if prevDrop == 0 {
//			// cpu is ok and no previous dropped request, just complete
//			return false
//		}
//
//		if time.Since(_InitTime)-prevDrop <= time.Second {
//			// if previous dropped request happened before and over than 1s.
//			// this means to limit request
//			inFlight := atomic.LoadInt64(&l.inflight)
//			return inFlight > 1 && float64(inFlight) > l.maxFlight()
//		}
//
//		// no need to limit and drop request.
//		// clear previous dropped request gap
//		l.prevDrop.Store(time.Duration(0))
//		return false
//	}
//
//	// cpu is exceed limit
//	inFlight := atomic.LoadInt64(&l.inflight)
//	drop := inFlight > 1 && float64(inFlight) > l.maxFlight()
//	if drop {
//		prevDrop, _ := l.prevDrop.Load().(time.Duration)
//		if prevDrop == 0 {
//			// update the prevDrop at when only the current request should drop and prevDrop is not exists
//			l.prevDrop.Store(time.Since(_InitTime))
//		}
//	}
//
//	return drop
//}

// statForDebug contains the metrics' snapshot of bbr.
type statForDebug struct {
	CPU         int64 // CPU usage
	InFlight    int64 // count of requests in flight
	MaxInFlight int64 // the maximum count of requests could be handled by system
	MinRTT      int64 // the minimum RT
	MaxPass     int64 // the maximum ?
}

// statForDebug tasks a snapshot of the bbr limiter.
func (l *BBR) Stat() statForDebug {
	return statForDebug{
		CPU:         l.cpu(),
		InFlight:    atomic.LoadInt64(&l.inflight),
		MinRTT:      l.minRTT(),
		MaxPass:     l.maxComplete(),
		MaxInFlight: int64(l.maxFlight()),
	}
}

// Allow checks all inbound traffic.
// Once overload is detected, it raises limit.ErrLimitExceed error.
func (l *BBR) Allow(ctx context.Context, opts ...limit.AllowOption) (func(info limit.DoneInfo), error) {
	allowOpts := limit.DefaultAllowOpts()
	for _, opt := range opts {
		opt.Apply(&allowOpts)
	}

	if l.shouldDropV2() {
		return nil, limit.ErrLimitExceed
	}

	atomic.AddInt64(&l.inflight, 1)
	start := time.Now()

	return func(do limit.DoneInfo) {
		cost := time.Since(start) / time.Millisecond
		l.rt.Add(int64(cost))
		atomic.AddInt64(&l.inflight, -1)

		switch do.Op {
		case limit.Success:
			l.complete.Add(1)
			return
		default:
			return
		}
	}, nil
}
