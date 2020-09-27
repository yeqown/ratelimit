package metric

import (
	"fmt"
	"time"
)

var _ Metric = &rollingCounter{}

// RollingCounter represents a ring window based on time duration.
// e.g. [[1], [3], [5]]
type RollingCounter interface {
	Metric

	// Sum get sum of
	Sum() float64

	// TimeSpan .
	TimeSpan() int

	// Reduce applies the reduction function to all buckets within the window.
	Reduce(func(Iterator) float64) float64
}

// RollingCounterOpts contains the arguments for creating RollingCounter.
type RollingCounterOpts struct {
	Size           int
	BucketDuration time.Duration
}

type rollingCounter struct {
	policy *RollingPolicy
}

// NewRollingCounter creates a new RollingCounter bases on RollingCounterOpts.
func NewRollingCounter(opts RollingCounterOpts) RollingCounter {
	window := NewWindow(WindowOpts{Size: opts.Size})
	policy := NewRollingPolicy(window, RollingPolicyOpts{BucketDuration: opts.BucketDuration})
	return &rollingCounter{
		policy: policy,
	}
}

func (r *rollingCounter) Add(val int64) {
	if val < 0 {
		panic(fmt.Errorf("stat/metric: cannot decrease in value. val: %d", val))
	}

	r.policy.Add(float64(val))
}

// Reduce .
func (r *rollingCounter) Reduce(f func(Iterator) float64) float64 {
	return r.policy.Reduce(f)
}

// Sum .
func (r *rollingCounter) Sum() float64 {
	return r.policy.Reduce(Sum)
}

// Value .
func (r *rollingCounter) Value() int64 {
	return int64(r.Sum())
}

// TimeSpan .
func (r *rollingCounter) TimeSpan() int {
	return r.policy.timespan()
}
