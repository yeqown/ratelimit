## Algorithm Introduction

```go
func (l bbrLimiter) shouldDrop() bool {
	return cpu > 800 && inFlight > (maxPassPer5s * minRttPer5s * windowsPer1s / 1000)
}
```