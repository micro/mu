package codex

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// Serialize refresh-token rotation across the server and setup CLI processes.
func lock(ctx context.Context) (func(), error) {
	if err := os.MkdirAll(authDir(), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(authDir(), "lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() { f.Close() }, nil
		}
		if err != syscall.EWOULDBLOCK && err != syscall.EAGAIN {
			f.Close()
			return nil, err
		}
		select {
		case <-ctx.Done():
			f.Close()
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}
