package metric

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCount(t *testing.T) {
	opts := RollingCounterOpts{Size: 10}
	pointGauge := NewRollingCounter(opts)
	for i := 0; i < opts.Size; i++ {
		pointGauge.Add(int64(i))
	}
	result := pointGauge.Reduce(Count)
	assert.Equal(t, float64(10), result, "validate count of pointGauge")
}
