// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestShapedLineCacheHitWidthKeyAndDetachment(t *testing.T) {
	cache, err := NewShapedLineCache(ShapedLineCacheLimits{MaxEntries: 4, MaxBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	shaped := shapedLineFixture("a b", DirectionLTR, []ShapedGlyph{shapedLineGlyph('a', 5, 0, 1), shapedLineGlyph(' ', 5, 1, 2), shapedLineGlyph('b', 5, 2, 3)})
	first, err := BreakShapedTextCached(context.Background(), shaped, 10, ShapedLineLimits{}, cache)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BreakShapedTextCached(context.Background(), shaped, 10, ShapedLineLimits{}, cache)
	if err != nil {
		t.Fatal(err)
	}
	first.Lines[0].Glyphs[0].ID = 999
	if second.Lines[0].Glyphs[0].ID == 999 {
		t.Fatal("cache values alias callers")
	}
	if _, err := BreakShapedTextCached(context.Background(), shaped, 15, ShapedLineLimits{}, cache); err != nil {
		t.Fatal(err)
	}
	stats := cache.Stats()
	if stats.Hits != 1 || stats.Misses != 2 || stats.Entries != 2 || stats.Bytes == 0 {
		t.Fatalf("stats = %+v", stats)
	}
}

func TestShapedLineCacheEvictionOversizeAndLimits(t *testing.T) {
	if _, err := NewShapedLineCache(ShapedLineCacheLimits{}); !errors.Is(err, ErrShapedLineCacheLimit) {
		t.Fatalf("limits error = %v", err)
	}
	cache, err := NewShapedLineCache(ShapedLineCacheLimits{MaxEntries: 1, MaxBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	shaped := shapedLineFixture("ab", DirectionLTR, []ShapedGlyph{shapedLineGlyph('a', 5, 0, 1), shapedLineGlyph('b', 5, 1, 2)})
	if _, err := BreakShapedTextCached(context.Background(), shaped, 5, ShapedLineLimits{}, cache); err != nil {
		t.Fatal(err)
	}
	if _, err := BreakShapedTextCached(context.Background(), shaped, 10, ShapedLineLimits{}, cache); err != nil {
		t.Fatal(err)
	}
	if cache.Stats().Entries != 1 {
		t.Fatalf("eviction stats = %+v", cache.Stats())
	}
	tiny, err := NewShapedLineCache(ShapedLineCacheLimits{MaxEntries: 1, MaxBytes: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BreakShapedTextCached(context.Background(), shaped, 10, ShapedLineLimits{}, tiny); err != nil {
		t.Fatal(err)
	}
	if tiny.Stats().Entries != 0 {
		t.Fatal("oversized entry was retained")
	}
}

func TestShapedLineCacheConcurrentAccess(t *testing.T) {
	cache, err := NewShapedLineCache(ShapedLineCacheLimits{MaxEntries: 8, MaxBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	shaped := shapedLineFixture("a b", DirectionLTR, []ShapedGlyph{shapedLineGlyph('a', 5, 0, 1), shapedLineGlyph(' ', 5, 1, 2), shapedLineGlyph('b', 5, 2, 3)})
	var wait sync.WaitGroup
	errorsFound := make(chan error, 32)
	for i := 0; i < 32; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			layout, err := BreakShapedTextCached(context.Background(), shaped, 10, ShapedLineLimits{}, cache)
			if err != nil {
				errorsFound <- err
				return
			}
			layout.Lines[0].Glyphs[0].ID++
		}()
	}
	wait.Wait()
	close(errorsFound)
	for err := range errorsFound {
		t.Fatal(err)
	}
	if cache.Stats().Entries != 1 {
		t.Fatalf("stats = %+v", cache.Stats())
	}
}
