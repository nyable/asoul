package lock_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"asoul/internal/lock"
)

func TestLockHonorsContextCancellation(t *testing.T) {
	dir := t.TempDir()
	first := lock.New(dir)
	unlock, err := first.Lock(context.Background(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	_, err = lock.New(dir).Lock(ctx, time.Second)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 250*time.Millisecond {
		t.Fatalf("canceled lock took %v", elapsed)
	}
}
