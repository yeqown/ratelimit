package bbr

import (
	"context"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/yeqown/ratelimit"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

func TestBBR_Allow(t *testing.T) {
	l := New(nil)
	wg := sync.WaitGroup{}
	wg.Add(100)
	for i := 0; i < 100; i++ {
		time.Sleep(time.Duration(i%50) * time.Millisecond)

		go func() {
			defer wg.Done()

			doneNotify, err := l.Allow(context.Background())
			if err == ratelimit.ErrLimitExceed {
				t.Log("LimitErrLimitExceed triggered")
				return
			}

			if doneNotify == nil {
				panic("should not be nil")
			}

			t.Logf("one finished: %+v", l.(*BBR).Stat())
			// do worker
			worker()
			doneNotify(ratelimit.DoneInfo{
				Err: nil,
				Op:  ratelimit.Success,
			})
		}()
	}
	wg.Wait()
}

func worker() {
	// mock CPU cost operation
	var val float64
	for i := 0; i < 1000000+rand.Intn(1000); i++ {
		val = 999.9999999 * 88888.88888 * 77777.77777 * rand.Float64()
		//println(val)
	}
	println(val)
}

func TestNew(t *testing.T) {
	l := New(nil)

	bbr := l.(*BBR)
	assert.Equal(t, int64(500), bbr.conf.CPUThreshold)
	assert.Equal(t, uint32(100), bbr.conf.WinBucket)
	assert.Equal(t, 10*time.Second, bbr.conf.Window)

}
