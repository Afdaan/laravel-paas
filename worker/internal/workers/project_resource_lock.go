package workers

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

func acquireProjectResourceLock(ctx context.Context, projectsPath string, projectID uint) (*os.File, error) {
	lockDir := filepath.Join(projectsPath, ".deployment-locks")
	if err := os.MkdirAll(lockDir, 0700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(lockDir, fmt.Sprintf("%d.lock", projectID)), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			_ = file.Close()
			return nil, err
		}
		err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			if err := ctx.Err(); err != nil {
				_ = file.Close()
				return nil, err
			}
			return file, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EINTR) {
			_ = file.Close()
			return nil, err
		}
		select {
		case <-ctx.Done():
			_ = file.Close()
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}
