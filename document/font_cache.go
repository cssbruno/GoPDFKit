// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"errors"
	"fmt"
	"io"
	"sync"
)

// FontCache stores parsed UTF-8 TrueType font metrics for reuse across
// documents. Each Document still receives its own mutable subset state.
type FontCache struct {
	mu    sync.RWMutex
	fonts map[string]cachedUTF8Font
}

type cachedUTF8Font struct {
	def  fontDefinition
	data []byte
}

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
	def, err := utf8FontDefinition(key, "", data)
	if err != nil {
		return err
	}
	def.utf8File = nil
	c.mu.Lock()
	if c.fonts == nil {
		c.fonts = make(map[string]cachedUTF8Font)
	}
	c.fonts[key] = cachedUTF8Font{def: def, data: append([]byte(nil), data...)}
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

// AddUTF8FontFromCache imports a cached UTF-8 TrueType font into this document.
func (f *Document) AddUTF8FontFromCache(family, style string, cache *FontCache) {
	if f.err != nil {
		return
	}
	family = fontFamilyEscape(family)
	fontKey := getFontKey(family, style)
	if _, ok := f.fonts[fontKey]; ok {
		return
	}
	cached, ok := cache.font(family, style)
	if !ok {
		f.SetErrorf("cached UTF-8 font not found: %s %s", family, style)
		return
	}
	def := cached.def
	def.File = ""
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
	f.fonts[fontKey] = def
}

func (c cachedUTF8Font) newUTF8Font() *utf8FontFile {
	if len(c.data) == 0 {
		return nil
	}
	reader := fileReader{array: c.data}
	utf := newUTF8Font(&reader)
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
