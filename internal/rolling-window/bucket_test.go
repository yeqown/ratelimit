package rollingwin

import (
	"testing"
	"time"

	"github.com/magiconair/properties/assert"
)

const _defaultDuration = time.Second

func TestBucket_Avg(t *testing.T) {
	b := newBucket(_defaultDuration)
	for i := 1; i <= 100; i++ {
		b.append(int64(i))
	}

	assert.Equal(t, float64(1+100)*0.5, b.Avg())
}
