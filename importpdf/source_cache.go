// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package importpdf

import (
	"errors"
	"os"
	"sync"
)

// SourceCache stores parsed PDF sources for reuse across documents.
type SourceCache struct {
	mu    sync.RWMutex
	files map[sourceFileCacheKey]*Source
}

type sourceFileCacheKey struct {
	path    string
	size    int64
	modTime int64
}

// NewSourceCache creates an empty reusable PDF import source cache.
func NewSourceCache() *SourceCache {
	return &SourceCache{files: make(map[sourceFileCacheKey]*Source)}
}

// OpenFile returns a parsed source for path, reusing a cached parse when the
// path, size, and modification time match.
func (c *SourceCache) OpenFile(path string) (*Source, error) {
	if c == nil {
		return nil, errors.New("PDF source cache is nil")
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	key := sourceFileCacheKey{
		path:    path,
		size:    info.Size(),
		modTime: info.ModTime().UnixNano(),
	}
	c.mu.RLock()
	source := c.files[key]
	c.mu.RUnlock()
	if source != nil {
		return source, nil
	}
	source, err = OpenFile(path)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	if c.files == nil {
		c.files = make(map[sourceFileCacheKey]*Source)
	}
	if cached := c.files[key]; cached != nil {
		source = cached
	} else {
		c.files[key] = source
	}
	c.mu.Unlock()
	return source, nil
}
