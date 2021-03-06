package main

import (
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"github.com/yeqown/ratelimit"
	"github.com/yeqown/ratelimit/impl/bbr"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

func main() {
	f := func(w http.ResponseWriter, req *http.Request) {
		var val float64

		// mock CPU cost operation
		for i := 0; i < 100000; i++ {
			val = 999.9999999 * 88888.88888 * 77777.77777 * rand.Float64()
		}

		_, _ = fmt.Fprintf(w, "val=%f", val)
	}
	// use limiter
	http.HandleFunc("/benchmark", withBBR(f))

	// start server
	fmt.Println("running on: http://127.0.0.1:8080")
	panic(http.ListenAndServe(":8080", nil))
}

// withBBR rate-limit middleware
func withBBR(f http.HandlerFunc) http.HandlerFunc {
	l := bbr.New(nil)

	return func(w http.ResponseWriter, req *http.Request) {
		defer func() {
			fmt.Printf("%+v\n", l.(*bbr.BBR).Stat())
		}()

		done, err := l.Allow(req.Context())
		if err != nil {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = fmt.Fprintf(w, "Error: %v", err)
			return
		}

		defer done(ratelimit.DoneInfo{Op: ratelimit.Success})

		f(w, req)
	}
}
