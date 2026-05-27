package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDo_SucceedsFirstTry(t *testing.T) {
	calls := 0
	err := Do(context.Background(), Config{MaxAttempts: 3, BaseDelay: time.Millisecond}, func() error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Errorf("want 1 call, got %d", calls)
	}
}

func TestDo_RetriesAndEventuallySucceeds(t *testing.T) {
	calls := 0
	err := Do(context.Background(), Config{MaxAttempts: 4, BaseDelay: time.Millisecond, MaxDelay: 10 * time.Millisecond}, func() error {
		calls++
		if calls < 3 {
			return errors.New("transient")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 3 {
		t.Errorf("want 3 calls, got %d", calls)
	}
}

func TestDo_GivesUpAfterMaxAttempts(t *testing.T) {
	calls := 0
	target := errors.New("persistent")
	err := Do(context.Background(), Config{MaxAttempts: 3, BaseDelay: time.Millisecond}, func() error {
		calls++
		return target
	})
	if !errors.Is(err, target) {
		t.Errorf("want target error returned, got %v", err)
	}
	if calls != 3 {
		t.Errorf("want 3 attempts, got %d", calls)
	}
}

func TestDo_StopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()
	err := Do(ctx, Config{MaxAttempts: 100, BaseDelay: 20 * time.Millisecond}, func() error {
		calls++
		return errors.New("keep failing")
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("want context.Canceled, got %v", err)
	}
	if calls > 10 {
		t.Errorf("retry should have stopped early; got %d calls", calls)
	}
}

func TestDo_RejectsInvalidConfig(t *testing.T) {
	err := Do(context.Background(), Config{MaxAttempts: 0}, func() error { return nil })
	if err == nil {
		t.Fatal("expected error for MaxAttempts=0")
	}
}
