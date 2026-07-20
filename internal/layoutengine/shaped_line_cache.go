// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
)

var ErrShapedLineCacheLimit = errors.New("layoutengine: shaped line cache limits are invalid")

type ShapedLineCacheLimits struct {
	MaxEntries uint64
	MaxBytes   uint64
}

type ShapedLineCacheStats struct {
	Entries uint64
	Bytes   uint64
	Hits    uint64
	Misses  uint64
}

type shapedLineCacheEntry struct {
	value ShapedLineLayout
	bytes uint64
}

// ShapedLineCache is a concurrency-safe byte-accounted FIFO cache. It is
// deliberately separate from ShapeCache because width changes line layout but
// not glyph shaping.
type ShapedLineCache struct {
	mu     sync.Mutex
	limits ShapedLineCacheLimits
	items  map[string]shapedLineCacheEntry
	order  []string
	bytes  uint64
	hits   uint64
	misses uint64
}

func NewShapedLineCache(limits ShapedLineCacheLimits) (*ShapedLineCache, error) {
	if limits.MaxEntries == 0 || limits.MaxBytes == 0 || limits.MaxEntries > 1<<20 || limits.MaxBytes > 1<<30 {
		return nil, ErrShapedLineCacheLimit
	}
	return &ShapedLineCache{limits: limits, items: make(map[string]shapedLineCacheEntry)}, nil
}

func (cache *ShapedLineCache) Get(key string) (ShapedLineLayout, bool) {
	if cache == nil {
		return ShapedLineLayout{}, false
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	entry, ok := cache.items[key]
	if !ok {
		cache.misses++
		return ShapedLineLayout{}, false
	}
	cache.hits++
	return cloneShapedLineLayout(entry.value), true
}

func (cache *ShapedLineCache) Put(key string, value ShapedLineLayout) {
	if cache == nil || key == "" || value.Validate() != nil {
		return
	}
	encoded, err := value.CanonicalJSON()
	if err != nil {
		return
	}
	size := uint64(len(key) + len(encoded))
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if _, exists := cache.items[key]; exists || size > cache.limits.MaxBytes {
		return
	}
	for uint64(len(cache.items)) >= cache.limits.MaxEntries || size > cache.limits.MaxBytes-cache.bytes {
		oldest := cache.order[0]
		cache.order = cache.order[1:]
		cache.bytes -= cache.items[oldest].bytes
		delete(cache.items, oldest)
	}
	cache.items[key] = shapedLineCacheEntry{value: cloneShapedLineLayout(value), bytes: size}
	cache.order = append(cache.order, key)
	cache.bytes += size
}

func (cache *ShapedLineCache) Stats() ShapedLineCacheStats {
	if cache == nil {
		return ShapedLineCacheStats{}
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	return ShapedLineCacheStats{Entries: uint64(len(cache.items)), Bytes: cache.bytes, Hits: cache.hits, Misses: cache.misses}
}

func shapedLineCacheKey(shaped ShapedText, maxWidth Fixed) (string, error) {
	encoded, err := json.Marshal(struct {
		Shaped   ShapedText `json:"shaped"`
		MaxWidth Fixed      `json:"max_width"`
	}{shaped, maxWidth})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

// BreakShapedTextCached uses the exact immutable shaping result as input. Cache
// hits and misses return detached layouts and never invoke a shaper.
func BreakShapedTextCached(ctx context.Context, shaped ShapedText, maxWidth Fixed, limits ShapedLineLimits, cache *ShapedLineCache) (ShapedLineLayout, error) {
	if cache == nil {
		return BreakShapedText(ctx, shaped, maxWidth, limits)
	}
	key, err := shapedLineCacheKey(shaped, maxWidth)
	if err != nil {
		return ShapedLineLayout{}, err
	}
	if value, ok := cache.Get(key); ok {
		return value, nil
	}
	value, err := BreakShapedText(ctx, shaped, maxWidth, limits)
	if err != nil {
		return ShapedLineLayout{}, err
	}
	cache.Put(key, value)
	return cloneShapedLineLayout(value), nil
}
