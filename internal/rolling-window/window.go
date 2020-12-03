package rollingwin

import (
	"sync"
	"time"
)

const _BufSize = 0

type RollingWindow struct {
	// mu for ringBuckets safety while concurrent visiting.
	mu sync.Mutex
	// ringBuckets a ring container to storage all.
	ringBuckets []Bucket
	// size of buckets.
	size uint32
	// bucketDuration to indicate how long time-span of each Bucket.
	bucketDuration time.Duration

	// lastSp to record last offset Bucket index.
	lastSp uint32
	// lastAppend the time of last append operation happened.
	lastAppend time.Time
}

// NewRollingWindow .
func NewRollingWindow(size uint32, duration time.Duration) *RollingWindow {
	rw := &RollingWindow{
		ringBuckets:    make([]Bucket, size+_BufSize),
		size:           size,
		bucketDuration: duration,
		lastSp:         0,
		lastAppend:     time.Now(),
	}
	rw.init()

	return rw
}

func (w *RollingWindow) init() {
	for i := uint32(0); i < w.size; i++ {
		w.ringBuckets[i].id = i
	}
}

// TODO: concurrent visit
func (w *RollingWindow) Add(val int64) {
	// sp which Bucket to insert the val.
	sp := w.TimeSpan()
	// TODO: reset expired buckets
	if sp > 0 {
		w.lastAppend = w.lastAppend.Add(w.bucketDuration * time.Duration(sp))
		offset := w.lastSp
		// get start position
		s := w.lastSp + 1
		if sp > w.size {
			// if IDLE is more than windows, then clear operation is same to clear all.
			sp = w.size
		}
		// e2 only when IDLE or START is over than size (for ring handling)
		e1, e2 := s+sp, uint32(0)
		if e1 > w.size {
			e2 = e1 - w.size
			e1 = w.size
		}
		// reset buckets
		for i := s; i < e1; i++ {
			w.ringBuckets[i].reset()
			offset = i
		}
		for i := uint32(0); i < e2; i++ {
			w.ringBuckets[i].reset()
			offset = i
		}
		w.lastSp = offset
	}

	// calculate which position to append the data.
	w.lastSp = sp
	w.ringBuckets[sp].append(val)
}

func (w *RollingWindow) Iterate(iterator func(b *Bucket)) {
	w.mu.Lock()
	sp := w.TimeSpan()
	if count := w.size; count > 0 {
		offset := w.lastSp + sp + 1
		if offset >= w.size {
			offset = offset - w.size
		}

		for i := uint32(0); i < w.size; i++ {
			pos := (i + offset) % w.size
			iterator(&(w.ringBuckets[pos]))
		}
	}
	w.mu.Unlock()
}

// TimeSpan how many span the is idle since last operation happened.
func (w *RollingWindow) TimeSpan() uint32 {
	return uint32(time.Since(w.lastAppend) / w.bucketDuration)
}
