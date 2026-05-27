// Package retry implements a small exponential-backoff helper used by the
// DynamoDB adapters. Persistence calls retry transient errors here, on top
// of SQS's native redrive, so throttling or brief LocalStack hiccups don't
// burn maxReceiveCount attempts and push the message to the DLQ.
package retry

import (
	"context"
	"errors"
	"time"
)

// Config controls how Do behaves.
type Config struct {
	// MaxAttempts caps total attempts (including the first). Must be >= 1.
	MaxAttempts int
	// BaseDelay is the delay before the second attempt. Subsequent delays
	// double: BaseDelay, 2*BaseDelay, 4*BaseDelay, ...
	BaseDelay time.Duration
	// MaxDelay caps an individual sleep. Zero means uncapped.
	MaxDelay time.Duration
}

// Default returns sensible defaults: 4 attempts with 100ms, 200ms, 400ms
// waits between them and a 2s ceiling.
func Default() Config {
	return Config{MaxAttempts: 4, BaseDelay: 100 * time.Millisecond, MaxDelay: 2 * time.Second}
}

// Do executes fn until it returns nil or attempts run out. The delay
// between attempts doubles each iteration. Context cancellation interrupts
// the wait and returns ctx.Err().
func Do(ctx context.Context, cfg Config, fn func() error) error {
	if cfg.MaxAttempts < 1 {
		return errors.New("retry: MaxAttempts must be >= 1")
	}
	delay := cfg.BaseDelay
	var lastErr error
	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		if err := fn(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if attempt == cfg.MaxAttempts {
			break
		}
		wait := delay
		if cfg.MaxDelay > 0 && wait > cfg.MaxDelay {
			wait = cfg.MaxDelay
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
		delay *= 2
	}
	return lastErr
}
