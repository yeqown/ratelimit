package bbr

import (
	"fmt"
	"os"
	"sync/atomic"
	"time"

	cpustat "github.com/yeqown/ratelimit/internal/cpu"
)

const (
	_QueryCPUdelay = 100 * time.Millisecond
)

var (
	// cpu is the load value of present CPU
	cpu int64

	// decay is a parameter to calculate cpu load, it's value in [0.0, 1.0].
	decay = 0.95
)

func cpugetter() int64 {
	return atomic.LoadInt64(&cpu)
}

// cpuproc always get "Moving Average" of current cpu usage.
// cpu = cpuᵗ⁻¹ * decay + cpuᵗ * (1 - decay)
func cpuproc(_init int64) {
	if _init > 0 {
		atomic.StoreInt64(&cpu, _init)
	}

	ticker := time.NewTicker(_QueryCPUdelay)
	defer func() {
		ticker.Stop()
		if err := recover(); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "rate.limit.cpuproc() err(%+v)", err)
			go cpuproc(_init)
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
