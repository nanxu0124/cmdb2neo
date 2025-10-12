package util

import (
	"context"
	"time"
)

// Retry 尝试执行 fn，失败则按退避重试。
func Retry(ctx context.Context, attempts int, backoff time.Duration, fn func() error) error {
	if attempts <= 0 {
		attempts = 1
	}
	var err error
	for i := 0; i < attempts; i++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		err = fn()
		if err == nil {
			return nil
		}
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		backoff *= 2
	}
	return err
}
