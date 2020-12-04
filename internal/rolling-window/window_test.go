package rollingwin

import (
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

func Test_RollingWindow(t *testing.T) {
	w := NewRollingWindow(4, 1*time.Second)
	for i := 0; i < 4; i++ {
		w.Add(1)
	}

	assert.Equal(t, 4, int(w.size))
	assert.Equal(t, 4, len(w.ringBuckets))
	assert.Equal(t, 0, int(w.lastSp))     // all ops are finished in one seconds
	assert.Equal(t, 0, int(w.TimeSpan())) // the same reason as above

	total := uint32(0)
	w.Iterate(func(b *Bucket) {
		t.Log(b.count)
		total += b.count
	})
	assert.Equal(t, 4, int(total))
}

// 5s' length record wants to be saved into 4s container.
// only 8 records should be found at last.
func Test_RollingWindow_Add(t *testing.T) {
	duration := 1 * time.Second
	durationHalf := duration / 2
	size := uint32(4)

	w := NewRollingWindow(size, duration)
	for i := 0; i < 10; i++ {
		w.Add(1)
		//sp := w.TimeSpan()
		//t.Log(sp, w.lastAppend.Second())
		//assert.Equal(t, uint32(i%2), sp)
		time.Sleep(durationHalf)
	}

	w.Iterate(func(b *Bucket) {
		t.Logf("%+v", b)
	})
}

func Test_Iterate(t *testing.T) {
	w := NewRollingWindow(100, 100*time.Millisecond)

	var debug = func() {
		minAvg := math.MaxFloat64
		maxCount := uint32(0)
		w.Iterate(func(b *Bucket) {
			if b.Count() > maxCount {
				maxCount = b.Count()
			}

			if b.Count() > 0 && b.Avg() < minAvg {
				minAvg = b.Avg()
			}
		})

		t.Log(minAvg, maxCount)
	}

	for i := 0; i < 700; i++ {
		if i%100 == 0 {
			time.Sleep(1 * time.Second)
			if i > 500 {
				debug()
			}
		}

		if i%500 == 0 {
			debug()
		}
		w.Add(int64(rand.Uint32()))
	}
}

func BenchmarkRollingWindow_Add(b *testing.B) {
	duration := 100 * time.Millisecond
	w := NewRollingWindow(4, duration)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.Add(int64(i))
	}
}
