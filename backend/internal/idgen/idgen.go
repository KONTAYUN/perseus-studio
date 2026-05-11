package idgen

import (
	"fmt"
	"sync/atomic"
	"time"
)

var seq atomic.Uint64

func New(prefix string) string {
	return NewAt(prefix, time.Now().UTC())
}

func NewAt(prefix string, now time.Time) string {
	return fmt.Sprintf("%s_%d_%d", prefix, now.UTC().UnixNano(), seq.Add(1))
}
