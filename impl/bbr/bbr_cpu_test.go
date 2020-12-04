package bbr

import (
	"testing"
	"time"
)

func Test_cpugetter(t *testing.T) {
	count := 10
	ticker := time.NewTicker(1 * time.Second)
	go cpuproc(500)

	for range ticker.C {
		t.Log(cpugetter())
		count--

		if count < 0 {
			break
		}
	}
}
