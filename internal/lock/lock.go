package lock

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
)

// Locker manages exclusive locks for a workspace.
type Locker struct {
	lockPath string
	flock    *flock.Flock
}

// New creates a new Locker for the specified workspace root directory.
func New(workspaceRoot string) *Locker {
	lockFile := filepath.Join(workspaceRoot, ".asoul.lock")
	return &Locker{
		lockPath: lockFile,
		flock:    flock.New(lockFile),
	}
}

// Lock acquires an exclusive lock on the workspace with a timeout.
func (l *Locker) Lock(ctx context.Context, timeout time.Duration) (func() error, error) {
	lockCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ok, err := l.flock.TryLockContext(lockCtx, 100*time.Millisecond)
	if err != nil {
		return nil, fmt.Errorf("failed attempting to acquire workspace lock (%s): %w", l.lockPath, err)
	}
	if !ok {
		return nil, fmt.Errorf("workspace is locked by another asoul process (%s)", l.lockPath)
	}

	unlock := func() error {
		return l.flock.Unlock()
	}
	return unlock, nil
}
