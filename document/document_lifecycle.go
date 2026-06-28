// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"compress/zlib"
	"errors"
	"fmt"
)

// SetDisplayMode sets advisory display directives for the document viewer.
// Pages can be displayed entirely on screen, occupy the full width of the
// window, use real size, be scaled by a specific zoom factor, or use the
// viewer default (configured in the Preferences menu of Adobe Reader). The page
// layout can be specified so that pages are displayed individually or in
// pairs.
//
// zoomStr can be "fullpage" to display the entire page on screen, "fullwidth"
// to use the maximum window width, "real" to use real size (equivalent to 100%
// zoom), or "default" to use the viewer default mode.
//
// layoutStr can be "single" (or "SinglePage") to display one page at once,
// "continuous" (or "OneColumn") to display pages continuously, "two" (or
// "TwoColumnLeft") to display two pages on two columns with odd-numbered pages
// on the left, or "TwoColumnRight" to display two pages on two columns with
// odd-numbered pages on the right, or "TwoPageLeft" to display pages two at a
// time with odd-numbered pages on the left, or "TwoPageRight" to display pages
// two at a time with odd-numbered pages on the right, or "default" to use the
// viewer default mode.
func (f *Document) SetDisplayMode(zoomStr, layoutStr string) {
	if f.err != nil {
		return
	}
	if layoutStr == "" {
		layoutStr = "default"
	}
	switch zoomStr {
	case "fullpage", "fullwidth", "real", "default":
		f.zoomMode = zoomStr
	default:
		f.err = fmt.Errorf("incorrect zoom display mode: %s", zoomStr)
		return
	}
	switch layoutStr {
	case "single", "continuous", "two", "default", "SinglePage", "OneColumn", "TwoColumnLeft", "TwoColumnRight", "TwoPageLeft", "TwoPageRight":
		f.layoutMode = layoutStr
	default:
		f.err = fmt.Errorf("incorrect layout display mode: %s", layoutStr)
		return
	}
}

// SetDefaultCompression controls the default setting of the internal
// compression flag. See SetCompression for more details. Compression is on by
// default. Prefer NewWithDefaults for per-document configuration.
func SetDefaultCompression(compress bool) {
	_gl.Lock()
	defer _gl.Unlock()
	_gl.noCompress = !compress
}

// SetCompression activates or deactivates page compression with zlib. When
// activated, the internal representation of each page is compressed, which
// leads to a compression ratio of about 2 for the resulting document.
// Compression is on by default.
func (f *Document) SetCompression(compress bool) {
	if compress && f.compressLevel == zlib.NoCompression {
		f.compressLevel = zlib.BestSpeed
	}
	f.compress = compress
}

// SetCompressionLevel sets the zlib level used when PDF streams are compressed.
// Valid values are the constants accepted by compress/zlib.NewWriterLevel,
// including zlib.HuffmanOnly, zlib.DefaultCompression, zlib.NoCompression and
// levels 1 through 9. Passing zlib.NoCompression disables Flate compression for
// page and template streams, matching SetCompression(false).
func (f *Document) SetCompressionLevel(level int) {
	if !validCompressionLevel(level) {
		f.SetErrorf("invalid compression level: %d", level)
		return
	}
	f.compressLevel = level
	f.compress = level != zlib.NoCompression
}

// SetNoCompression disables Flate compression for page and template streams.
func (f *Document) SetNoCompression() {
	f.SetCompressionLevel(zlib.NoCompression)
}

func (f *Document) compressBytes(data []byte) []byte {
	level := f.compressLevel
	if !validCompressionLevel(level) {
		level = defaultCompressionLevel()
	}
	out, err := sliceCompressLevel(data, level)
	if err != nil {
		f.SetError(err)
		return nil
	}
	return out
}

func defaultCompressionLevel() int {
	return zlib.BestSpeed
}

const tinyStreamCompressionThreshold = 32

func (f *Document) compressStreamBytes(data []byte) ([]byte, bool) {
	if !f.compress || len(data) < tinyStreamCompressionThreshold {
		return data, false
	}
	return f.compressBytes(data), f.err == nil
}

// AliasNbPages defines an alias for the total number of pages. It will be
// substituted as the document is closed. An empty string is replaced with the
// string "{nb}".
//
// See the example for AddPage for a demonstration of this method.
func (f *Document) AliasNbPages(aliasStr string) {
	if aliasStr == "" {
		aliasStr = "{nb}"
	}
	f.aliasNbPagesStr = aliasStr
	f.aliasNeedlesDirty = true
	f.markPagesContainingAlias(aliasStr)
}

// RTL enables right-to-left text layout mode.
func (f *Document) RTL() {
	f.isRTL = true
}

// LTR disables right-to-left text layout mode.
func (f *Document) LTR() {
	f.isRTL = false
}

// open starts a PDF document.
func (f *Document) open() {
	f.state = 1
}

// Close terminates the PDF document. It is not necessary to call this method
// explicitly because Output, OutputAndClose, and OutputFileAndClose do it
// automatically. If the document contains no page, AddPage is called to
// prevent the generation of an invalid document.
func (f *Document) Close() {
	if f.err == nil {
		if f.clipNest > 0 {
			f.err = errors.New("clip procedure must be explicitly ended")
		} else if f.transformNest > 0 {
			f.err = errors.New("transformation procedure must be explicitly ended")
		}
	}
	if f.err != nil {
		return
	}
	if f.state == 3 {
		return
	}
	if f.page == 0 {
		f.AddPage()
		if f.err != nil {
			return
		}
	}
	f.inFooter = true
	if f.footerFnc != nil {
		f.footerFnc()
	} else if f.footerFncLpi != nil {
		f.footerFncLpi(true)
	}
	f.inFooter = false
	f.endpage()
	f.enddoc()
}
