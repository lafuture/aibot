package state

import (
	"context"
	"time"
)

type State struct {
	Step string            `json:"step"`
	Data map[string]string `json:"data,omitempty"`
}

type Store interface {
	Get(ctx context.Context, key string) (State, bool, error)
	Set(ctx context.Context, key string, st State, ttl time.Duration) error
	Clear(ctx context.Context, key string) error
}
