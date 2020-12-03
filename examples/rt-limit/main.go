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

		// mock RT increasing cost latency
		// TODO(@yeqiang):

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

		defer done(ratelimit.DoneInfo{Op: ratelimit.Success})

		f(w, req)
	}
}
