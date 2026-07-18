// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

//go:build darwin || linux

package paperd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

func lockPersistenceRoot(ctx context.Context, root string) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, workspaceError("PERSISTENCE_CANCELLED", "persistence lock was cancelled", err)
	}
	path := filepath.Join(root, ".paperd.lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600) // #nosec G304 -- root is the validated workspace persistence directory.
	if err != nil {
		return nil, workspaceError("PERSISTENCE_LOCK", "persistence lock cannot be opened", ErrPersistence)
	}
	info, statErr := file.Stat()
	if statErr != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		_ = file.Close()
		return nil, workspaceError("PERSISTENCE_LOCK", "persistence lock path is unsafe", ErrPersistenceCorrupt)
	}
	for {
		err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			_ = file.Close()
			return nil, workspaceError("PERSISTENCE_LOCK", "persistence lock cannot be acquired", ErrPersistence)
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			_ = file.Close()
			return nil, workspaceError("PERSISTENCE_CANCELLED", "persistence lock was cancelled", ctx.Err())
		case <-timer.C:
		}
	}
	if err := ctx.Err(); err != nil {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
		return nil, workspaceError("PERSISTENCE_CANCELLED", "persistence lock was cancelled", err)
	}
	return func() {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
	}, nil
}
