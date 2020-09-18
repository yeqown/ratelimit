package main

import (
	"fmt"
	"math/rand"
	"net/http"
	"time"

	limit "github.com/yeqown/ratelimit"

	"github.com/yeqown/ratelimit/limiters/bbr"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

func main() {
	f := func(w http.ResponseWriter, req *http.Request) {
		var val float64

		// mock CPU cost operation
		for i := 0; i < 1000000; i++ {
			val = 999.9999999 * 88888.88888 * 77777.77777 * rand.Float64()
		}

		_, _ = fmt.Fprintf(w, "val=%f", val)
	}

	// empty
	//http.HandleFunc("/benchmark", f)

	// use limiter
	http.HandleFunc("/benchmark", withRatelimiter(f))

	// start server
	fmt.Println("running on: http://127.0.0.1:8080")
	panic(http.ListenAndServe(":8080", nil))
}

// ratelimit middleware
func withRatelimiter(f http.HandlerFunc) http.HandlerFunc {
	l := bbr.NewLimiter(nil)

	return func(w http.ResponseWriter, req *http.Request) {
		done, err := l.Allow(req.Context())
		if err != nil {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = fmt.Fprintf(w, "Error: %v", err)
			return
		}

		defer done(limit.DoneInfo{Op: limit.Success})

		f(w, req)
	}
}
