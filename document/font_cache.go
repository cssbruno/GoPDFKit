// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"errors"
	"fmt"
	"io"
	"sync"
)

// FontCache stores parsed UTF-8 TrueType font metrics for reuse across
// documents. Its methods are safe for concurrent use; each Document still
// receives its own mutable subset state.
type FontCache struct {
	mu    sync.RWMutex
	fonts map[string]cachedUTF8Font
}

type cachedUTF8Font struct {
	def      fontDefinition
	data     []byte
	static   *utf8StaticTables
	sourceID [32]byte
}

type sharedUTF8FontFileCacheKey struct {
	path    string
	size    int64
	modTime int64
	fontKey string
}

var sharedUTF8FontFileCache = struct {
	sync.Mutex
	fonts map[sharedUTF8FontFileCacheKey]cachedUTF8Font
	order []sharedUTF8FontFileCacheKey
	bytes int64
}{
	fonts: make(map[sharedUTF8FontFileCacheKey]cachedUTF8Font),
}

const maxSharedUTF8FontFileCacheBytes = 128 * 1024 * 1024

// NewFontCache returns an empty reusable UTF-8 font cache.
func NewFontCache() *FontCache {
	return &FontCache{fonts: make(map[string]cachedUTF8Font)}
}

// AddUTF8Font loads and parses a TrueType font file into the cache.
func (c *FontCache) AddUTF8Font(family, style, path string) error {
	data, err := readFontResourceFile(path, maxFontSourceBytes)
	if err != nil {
		return err
	}
	return c.AddUTF8FontFromBytes(family, style, data)
}

// AddUTF8FontFromReader loads and parses a TrueType font reader into the cache.
func (c *FontCache) AddUTF8FontFromReader(family, style string, r io.Reader) error {
	data, err := readFontResourceReader(r, maxFontSourceBytes)
	if err != nil {
		return err
	}
	return c.AddUTF8FontFromBytes(family, style, data)
}

// AddUTF8FontFromBytes loads and parses TrueType font bytes into the cache.
func (c *FontCache) AddUTF8FontFromBytes(family, style string, data []byte) error {
	if c == nil {
		return errors.New("font cache is nil")
	}
	if err := validateFontDataSize(data, maxFontSourceBytes, "font data"); err != nil {
		return err
	}
	key := getFontKey(fontFamilyEscape(family), style)
	if !validPDFNameFragment(key) {
		return fmt.Errorf("invalid UTF-8 font name: %s", key)
	}
	// Parse the font-only tables once here so every document that embeds this
	// cached font reuses them read-only instead of re-parsing per document.
	cached, err := newCachedUTF8Font(key, "", data)
	if err != nil {
		return err
	}
	c.mu.Lock()
	if c.fonts == nil {
		c.fonts = make(map[string]cachedUTF8Font)
	}
	c.fonts[key] = cached
	c.mu.Unlock()
	return nil
}

func (c *FontCache) font(family, style string) (cachedUTF8Font, bool) {
	if c == nil {
		return cachedUTF8Font{}, false
	}
	key := getFontKey(fontFamilyEscape(family), style)
	c.mu.RLock()
	font, ok := c.fonts[key]
	c.mu.RUnlock()
	return font, ok
}

func (c *FontCache) put(fontKey string, cached cachedUTF8Font) {
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.fonts == nil {
		c.fonts = make(map[string]cachedUTF8Font)
	}
	c.fonts[fontKey] = cached
	c.mu.Unlock()
}

func cachedUTF8FontFromFile(fontKey, fontPath string, size, modTime int64) (cachedUTF8Font, error) {
	key := sharedUTF8FontFileCacheKey{path: fontPath, size: size, modTime: modTime, fontKey: fontKey}
	if cached, ok := lookupSharedUTF8FontFile(key); ok {
		return cached, nil
	}
	data, err := readFontResourceFile(fontPath, maxFontSourceBytes)
	if err != nil {
		return cachedUTF8Font{}, err
	}
	cached, err := newCachedUTF8Font(fontKey, fontPath, data)
	if err != nil {
		return cachedUTF8Font{}, err
	}
	storeSharedUTF8FontFile(key, cached)
	return cached, nil
}

func lookupSharedUTF8FontFile(key sharedUTF8FontFileCacheKey) (cachedUTF8Font, bool) {
	sharedUTF8FontFileCache.Lock()
	cached, ok := sharedUTF8FontFileCache.fonts[key]
	sharedUTF8FontFileCache.Unlock()
	return cached, ok
}

func storeSharedUTF8FontFile(key sharedUTF8FontFileCacheKey, cached cachedUTF8Font) {
	entryBytes := int64(len(cached.data))
	if entryBytes > maxSharedUTF8FontFileCacheBytes {
		return
	}
	sharedUTF8FontFileCache.Lock()
	defer sharedUTF8FontFileCache.Unlock()
	if old, ok := sharedUTF8FontFileCache.fonts[key]; ok {
		sharedUTF8FontFileCache.bytes -= int64(len(old.data))
	} else {
		sharedUTF8FontFileCache.order = append(sharedUTF8FontFileCache.order, key)
	}
	sharedUTF8FontFileCache.fonts[key] = cached
	sharedUTF8FontFileCache.bytes += entryBytes
	for sharedUTF8FontFileCache.bytes > maxSharedUTF8FontFileCacheBytes && len(sharedUTF8FontFileCache.order) > 0 {
		evict := sharedUTF8FontFileCache.order[0]
		sharedUTF8FontFileCache.order = sharedUTF8FontFileCache.order[1:]
		if old, ok := sharedUTF8FontFileCache.fonts[evict]; ok {
			sharedUTF8FontFileCache.bytes -= int64(len(old.data))
			delete(sharedUTF8FontFileCache.fonts, evict)
		}
	}
}

func newCachedUTF8Font(fontKey, fontPath string, data []byte) (cachedUTF8Font, error) {
	stored := append([]byte(nil), data...)
	parsed, err := parseUTF8Font(stored)
	if err != nil {
		return cachedUTF8Font{}, err
	}
	def := utf8FontDefinitionFromParsed(fontKey, fontPath, parsed)
	def.utf8File = nil
	static, err := parsed.staticTablesFromParsedFont()
	if err != nil {
		return cachedUTF8Font{}, err
	}
	return cachedUTF8Font{def: def, data: stored, static: static, sourceID: static.sourceID}, nil
}

// AddUTF8FontFromCache imports a cached UTF-8 TrueType font into this document.
func (f *Document) AddUTF8FontFromCache(family, style string, cache *FontCache) {
	_ = f.AddUTF8FontFromCacheError(family, style, cache)
}

// AddUTF8FontFromCacheError imports a cached UTF-8 TrueType font into this
// document and returns failures directly.
func (f *Document) AddUTF8FontFromCacheError(family, style string, cache *FontCache) error {
	if f.err != nil {
		return f.err
	}
	family = fontFamilyEscape(family)
	fontKey := getFontKey(family, style)
	if _, ok := f.ensureResourceStore().font(fontKey); ok {
		return nil
	}
	cached, ok := cache.font(family, style)
	if !ok {
		if f.hooks.OnResourceCacheMiss != nil {
			f.hooks.OnResourceCacheMiss("font", fontKey)
		}
		f.SetErrorf("cached UTF-8 font not found: %s %s", family, style)
		return f.err
	}
	if f.hooks.OnResourceCacheHit != nil {
		f.hooks.OnResourceCacheHit("font", fontKey)
	}
	f.addCachedUTF8Font(fontKey, family, style, cached)
	return f.err
}

func (f *Document) addCachedUTF8Font(fontKey, family, style string, cached cachedUTF8Font) {
	def := cached.def
	def.File = ""
	def.Name = fontKey
	def.Cw = cached.def.Cw
	def.usedRunes = defaultUTF8UsedRunes(f.aliasNbPagesStr)
	def.utf8File = cached.newUTF8Font()
	if def.utf8File == nil {
		f.SetErrorf("cached UTF-8 font data is empty: %s %s", family, style)
		return
	}
	if def.i == "" {
		var err error
		def.i, err = generateFontID(def)
		if err != nil {
			f.SetError(err)
			return
		}
	}
	f.ensureResourceStore().setFont(fontKey, def)
}

func (c cachedUTF8Font) newUTF8Font() *utf8FontFile {
	if len(c.data) == 0 {
		return nil
	}
	reader := fileReader{array: c.data}
	utf := newUTF8Font(&reader)
	utf.static = c.static
	utf.sourceID = c.sourceID
	utf.Ascent = c.def.Desc.Ascent
	utf.Descent = c.def.Desc.Descent
	utf.CapHeight = c.def.Desc.CapHeight
	utf.Flags = c.def.Desc.Flags
	utf.Bbox = c.def.Desc.FontBBox
	utf.ItalicAngle = c.def.Desc.ItalicAngle
	utf.StemV = c.def.Desc.StemV
	utf.DefaultWidth = float64(c.def.Desc.MissingWidth)
	utf.UnderlinePosition = float64(c.def.Up)
	utf.UnderlineThickness = float64(c.def.Ut)
	utf.CharWidths = c.def.Cw
	return utf
}
