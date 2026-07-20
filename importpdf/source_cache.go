// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package importpdf

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
)

const defaultSourceCacheMaxBytes int64 = 128 * 1024 * 1024

// SourceCache stores parsed PDF sources for reuse across documents.
type SourceCache struct {
	mu       sync.RWMutex
	files    map[sourceFileCacheKey]*sourceFileCacheEntry
	maxBytes int64
	bytes    int64
	clock    uint64
}

type sourceFileCacheKey struct {
	path    string
	size    int64
	modTime int64
}

type sourceFileCacheEntry struct {
	source *Source
	size   int64
	used   uint64
}

// NewSourceCache creates an empty reusable PDF import source cache.
func NewSourceCache() *SourceCache {
	return NewSourceCacheWithMaxBytes(defaultSourceCacheMaxBytes)
}

// NewSourceCacheWithMaxBytes creates an empty reusable PDF import source cache
// with a byte budget based on source file sizes. Files larger than maxBytes are
// parsed successfully but not cached.
func NewSourceCacheWithMaxBytes(maxBytes int64) *SourceCache {
	if maxBytes < 0 {
		maxBytes = 0
	}
	return &SourceCache{files: make(map[sourceFileCacheKey]*sourceFileCacheEntry), maxBytes: maxBytes}
}

// OpenFile returns a parsed source for path, reusing a cached parse when the
// path, size, and modification time match.
func (c *SourceCache) OpenFile(path string) (*Source, error) {
	if c == nil {
		return nil, errors.New("PDF source cache is nil")
	}
	canonicalPath, info, err := canonicalSourceCachePath(path)
	if err != nil {
		return nil, err
	}
	key := sourceFileCacheKey{
		path:    canonicalPath,
		size:    info.Size(),
		modTime: info.ModTime().UnixNano(),
	}
	if source := c.cachedSource(key); source != nil {
		return source, nil
	}
	source, err := OpenFile(path)
	if err != nil {
		return nil, err
	}
	c.storeSource(key, source)
	return source, nil
}

func (c *SourceCache) cachedSource(key sourceFileCacheKey) *Source {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry := c.files[key]
	if entry == nil {
		return nil
	}
	c.clock++
	entry.used = c.clock
	return entry.source
}

func (c *SourceCache) storeSource(key sourceFileCacheKey, source *Source) {
	if source == nil || c.maxBytes <= 0 || key.size > c.maxBytes {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.files == nil {
		c.files = make(map[sourceFileCacheKey]*sourceFileCacheEntry)
	}
	if entry := c.files[key]; entry != nil {
		c.clock++
		entry.used = c.clock
		return
	}
	c.clock++
	c.files[key] = &sourceFileCacheEntry{source: source, size: key.size, used: c.clock}
	c.bytes += key.size
	c.evictLocked()
}

func (c *SourceCache) evictLocked() {
	for c.bytes > c.maxBytes && len(c.files) > 0 {
		var oldestKey sourceFileCacheKey
		var oldestEntry *sourceFileCacheEntry
		for key, entry := range c.files {
			if entry == nil {
				delete(c.files, key)
				continue
			}
			if oldestEntry == nil || entry.used < oldestEntry.used {
				oldestKey = key
				oldestEntry = entry
			}
		}
		if oldestEntry == nil {
			// The cache only stores non-nil entries, but recover a consistent
			// empty state if an invariant violation is observed.
			c.bytes = 0
			return
		}
		delete(c.files, oldestKey)
		c.bytes -= oldestEntry.size
	}
}

func canonicalSourceCachePath(path string) (string, os.FileInfo, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", nil, err
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return "", nil, err
	}
	canonicalPath := absPath
	if resolved, err := filepath.EvalSymlinks(absPath); err == nil {
		canonicalPath = resolved
	}
	return canonicalPath, info, nil
}
