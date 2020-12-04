package ratelimit

import (
	"context"

	"github.com/pkg/errors"
)

var (
	// ErrLimitExceed 调用超出限制
	ErrLimitExceed = errors.New("request is limited")
)

// Op operations type.
type Op int

const (
	// Success operation type: success
	Success Op = iota
	// Ignore operation type: ignore
	Ignore
	// Drop operation type: drop
	Drop
)

type allowOptions struct{}

// AllowOptions allow options.
type AllowOption interface {
	Apply(*allowOptions)
}

// DoneInfo done info.
type DoneInfo struct {
	Err error
	Op  Op
}

// DefaultAllowOpts returns the default allow options.
func DefaultAllowOpts() allowOptions {
	return allowOptions{}
}

// Limiter limit interface.
type Limiter interface {
	Allow(ctx context.Context, opts ...AllowOption) (func(info DoneInfo), error)
}
