package bbr

import (
	"runtime"
	"time"
)

var (
	defaultConf = &Config{
		Window:       time.Second * 10,
		WinBucket:    100,
		CPUThreshold: 0,
	}
)

// Config contains configs of bbr limiter.
type Config struct {
	// Window time.Duration of window contains.
	Window time.Duration
	// WinBucket indicates how many bucket the window holds.
	WinBucket uint32
	// CPUThreshold indicates the threshold of the CPU limit.
	// if it's not set, default is CORE * 2.5
	CPUThreshold int64
}

func comptaibleConfig(conf *Config) {
	if conf == nil {
		conf = defaultConf
	}

	if conf.Window == 0 {
		conf.Window = defaultConf.Window
	}
	if conf.WinBucket == 0 {
		conf.WinBucket = defaultConf.WinBucket
	}
	if conf.CPUThreshold == 0 {
		conf.CPUThreshold = int64(float64(cores()) * 2.5)
	}
}

func cores() int {
	return runtime.NumCPU() / 2
}
