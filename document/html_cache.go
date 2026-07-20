// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"context"
	"crypto/sha256"
	"sync"
)

const (
	htmlCompiledCacheLimit    = 64
	htmlCompiledCacheMaxBytes = 64 * 1024 * 1024
)

type htmlCompiledCacheKey struct {
	size int
	sum  [32]byte
}

var sharedCompiledHTMLCache = struct {
	sync.Mutex
	entries map[htmlCompiledCacheKey]*CompiledHTML
	order   []htmlCompiledCacheKey
	bytes   int64
	sizes   map[htmlCompiledCacheKey]int64
}{
	entries: make(map[htmlCompiledCacheKey]*CompiledHTML),
	sizes:   make(map[htmlCompiledCacheKey]int64),
}

func compileHTMLForWriteContext(ctx context.Context, htmlStr string, maxDataImageBytes int, useSharedCache bool) (*CompiledHTML, error) {
	if !useSharedCache {
		return compileHTMLWithDataImageLimitContext(ctx, htmlStr, true, maxDataImageBytes)
	}
	key := htmlCompiledCacheKey{size: len(htmlStr), sum: sha256.Sum256([]byte(htmlStr))}
	if compiled, ok := lookupSharedCompiledHTML(key); ok {
		return compiled, nil
	}
	compiled, err := compileHTMLWithDataImageLimitContext(ctx, htmlStr, true, maxDataImageBytes)
	if err != nil {
		return nil, err
	}
	storeSharedCompiledHTML(key, htmlStr, compiled)
	return compiled, nil
}

func lookupSharedCompiledHTML(key htmlCompiledCacheKey) (*CompiledHTML, bool) {
	sharedCompiledHTMLCache.Lock()
	compiled, ok := sharedCompiledHTMLCache.entries[key]
	sharedCompiledHTMLCache.Unlock()
	return compiled, ok
}

func storeSharedCompiledHTML(key htmlCompiledCacheKey, htmlStr string, compiled *CompiledHTML) {
	if compiled == nil {
		return
	}
	entryBytes := compiledHTMLCacheBytes(htmlStr, compiled)
	if entryBytes <= 0 || entryBytes > htmlCompiledCacheMaxBytes {
		return
	}
	sharedCompiledHTMLCache.Lock()
	defer sharedCompiledHTMLCache.Unlock()
	if _, exists := sharedCompiledHTMLCache.entries[key]; exists {
		return
	}
	sharedCompiledHTMLCache.entries[key] = compiled
	sharedCompiledHTMLCache.order = append(sharedCompiledHTMLCache.order, key)
	sharedCompiledHTMLCache.sizes[key] = entryBytes
	sharedCompiledHTMLCache.bytes += entryBytes
	for (len(sharedCompiledHTMLCache.order) > htmlCompiledCacheLimit || sharedCompiledHTMLCache.bytes > htmlCompiledCacheMaxBytes) && len(sharedCompiledHTMLCache.order) > 0 {
		evict := sharedCompiledHTMLCache.order[0]
		sharedCompiledHTMLCache.order = sharedCompiledHTMLCache.order[1:]
		if oldSize, ok := sharedCompiledHTMLCache.sizes[evict]; ok {
			sharedCompiledHTMLCache.bytes -= oldSize
			delete(sharedCompiledHTMLCache.sizes, evict)
		}
		delete(sharedCompiledHTMLCache.entries, evict)
	}
}

func clearSharedCompiledHTMLCache() {
	sharedCompiledHTMLCache.Lock()
	sharedCompiledHTMLCache.entries = make(map[htmlCompiledCacheKey]*CompiledHTML)
	sharedCompiledHTMLCache.order = nil
	sharedCompiledHTMLCache.sizes = make(map[htmlCompiledCacheKey]int64)
	sharedCompiledHTMLCache.bytes = 0
	sharedCompiledHTMLCache.Unlock()
}

func sharedCompiledHTMLCacheStats() CacheStats {
	sharedCompiledHTMLCache.Lock()
	defer sharedCompiledHTMLCache.Unlock()
	return CacheStats{
		Entries: len(sharedCompiledHTMLCache.entries),
		Bytes:   sharedCompiledHTMLCache.bytes,
	}
}

func compiledHTMLCacheBytes(htmlStr string, compiled *CompiledHTML) int64 {
	if compiled == nil {
		return 0
	}
	size := int64(len(htmlStr))
	size += int64(len(compiled.tokens)+len(compiled.tokenNode)+len(compiled.elementEnd)) * 32
	size += int64(len(compiled.nodeIndexes)) * 40
	size += int64(len(compiled.elementText)) * 48
	size += int64(len(compiled.elementMeta)) * 48
	size += int64(len(compiled.tables)) * 256
	size += int64(len(compiled.inlineSVGs)) * 4096
	size += int64(len(compiled.cssRules)) * 256
	for _, token := range compiled.tokens {
		size += int64(len(token.Str))
		size += htmlAttrMapCacheBytes(token.Attr)
	}
	for style, decl := range compiled.styleDeclarations {
		size += int64(len(style)) + htmlAttrMapCacheBytes(decl)
	}
	for _, img := range compiled.dataImages {
		size += int64(len(img.name) + len(img.data))
	}
	return size
}

func htmlAttrMapCacheBytes(values map[string]string) int64 {
	var size int64
	for key, value := range values {
		size += int64(len(key) + len(value) + 32)
	}
	return size
}
