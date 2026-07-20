// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"crypto/sha256"
	"strings"
	"unicode/utf8"
)

// cachePartition is deliberately private. It binds every retained compiled
// projection, plan, context source, and idempotency table to the complete
// authority tuple without exposing that tuple through capability handles.
type cachePartition struct{ digest [sha256.Size]byte }

func normalizeCachePartition(project, policy string, domain DisclosureDomain) (string, string, cachePartition, error) {
	if project == "" {
		project = "default"
	}
	if policy == "" {
		policy = "unversioned"
	}
	valid := func(value string) bool {
		return utf8.ValidString(value) && len(value) <= MaxQueryBytesHard && strings.TrimSpace(value) == value
	}
	if !valid(project) || !valid(policy) {
		return "", "", cachePartition{}, workspaceError("INVALID_CACHE_PARTITION", "project and policy revision must be bounded valid UTF-8 without surrounding whitespace", ErrInvalidLimits)
	}
	sum := sha256.Sum256([]byte("paperd/cache-partition/v1\x00" + project + "\x00" + policy + "\x00" + string(domain)))
	return project, policy, cachePartition{digest: sum}, nil
}

func (w *Workspace) ownsPartition(partition cachePartition) bool {
	return partition != (cachePartition{}) && partition == w.partition
}
