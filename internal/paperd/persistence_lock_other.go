// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

//go:build !darwin && !linux

package paperd

import "context"

func lockPersistenceRoot(context.Context, string) (func(), error) {
	return nil, workspaceError("PERSISTENCE_LOCK_UNSUPPORTED", "multi-process persistence locking requires Linux or macOS", ErrPersistence)
}
