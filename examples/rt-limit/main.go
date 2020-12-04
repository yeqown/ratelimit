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
		// mock RT increasing cost latency
		r := 100 + (rand.Uint32() % 200)
		time.Sleep(time.Duration(r) * time.Millisecond)
		_, _ = fmt.Fprintf(w, "latency=%v ms", r)
	}

	// empty
	//http.HandleFunc("/benchmark", f)

	// use limiter
	http.HandleFunc("/benchmark", withBBR(f))

	// start server
	fmt.Println("running on: http://127.0.0.1:8080")
	panic(http.ListenAndServe(":8080", nil))
}

// withBBR middleware
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
