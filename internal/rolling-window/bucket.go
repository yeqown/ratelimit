package rollingwin

import (
	"sync"
	"sync/atomic"
	"time"
)

const _InitCap = 128

// Bucket to store all points of
type Bucket struct {
	// id means the order of the Bucket in RollingWindow.ringBuckets
	id uint32
	// mu if lock fo points.
	mu sync.Mutex

	// duration means how long of time-span would be save into
	// the same one Bucket.
	duration time.Duration
	// points recording all point of data.
	points []int64
	// count variable for Bucket be called, it's equal to points' length.
	count uint32
}

func newBucket(dur time.Duration) *Bucket {
	return &Bucket{
		duration: dur,
		points:   make([]int64, 0, _InitCap),
		count:    0,
	}
}

func (b *Bucket) reset() {
	atomic.StoreUint32(&b.count, 0)

	b.mu.Lock()
	b.points = make([]int64, 0, _InitCap)
	b.mu.Unlock()
}

func (b *Bucket) append(val int64) {
	atomic.AddUint32(&b.count, 1)

	b.mu.Lock()
	b.points = append(b.points, val)
	b.mu.Unlock()
}
func (b *Bucket) Count() uint32 {
	return atomic.LoadUint32(&b.count)
}

// DONE: THINK ABOUT OVERFLOW IF AVG = SUM / COUNT
func (b *Bucket) Avg() (avg float64) {
	avg = float64(0)
	count := 0

	b.Iterate(func(v int64) {
		if count == 0 {
			avg = float64(v)
		}

		avg = (avg*float64(count))/float64(count+1) + (float64(v) / float64(count+1))
		count++
	})

	return avg
}

func (b *Bucket) Iterate(f func(v int64)) {
	b.mu.Lock()
	dst := make([]int64, b.count)
	copy(dst, b.points)
	b.mu.Unlock()

	for _, v := range dst {
		f(v)
	}
}
