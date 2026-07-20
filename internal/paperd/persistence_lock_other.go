// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

//go:build !darwin && !linux

package paperd

import "context"

func lockPersistenceRoot(context.Context, string) (func(), error) {
	return nil, workspaceError("PERSISTENCE_LOCK_UNSUPPORTED", "multi-process persistence locking requires Linux or macOS", ErrPersistence)
}
