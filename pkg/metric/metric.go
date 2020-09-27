package metric

// Metric is a sample interface.
// Implementations of Metrics in metric package are Counter, Gauge,
// PointGauge, RollingCounter and RollingGauge.
type Metric interface {
	// Add adds the given value to the counter.
	Add(int64)
	// Value gets the current value.
	// If the metrics' type is PointGauge, RollingCounter, RollingGauge,
	// it returns the sum value within the window.
	Value() int64
}
