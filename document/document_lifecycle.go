// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"compress/zlib"
	"context"
	"errors"
	"fmt"
	"runtime"
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

func defaultCompressionPolicy() CompressionPolicy {
	pageWorkers := runtime.GOMAXPROCS(0)
	if pageWorkers > 4 {
		pageWorkers = 4
	}
	if pageWorkers < 1 {
		pageWorkers = 1
	}
	return CompressionPolicy{
		Mode:                     CompressionEnabled,
		Level:                    zlib.BestSpeed,
		PageWorkers:              pageWorkers,
		AttachmentWorkers:        defaultAttachmentCompressionWorkers,
		TinyStreamThresholdBytes: defaultTinyStreamCompressionThreshold,
	}
}

func compressionPolicyHasFields(policy CompressionPolicy) bool {
	return policy.Level != 0 ||
		policy.PageWorkers != CompressionWorkersDefault ||
		policy.AttachmentWorkers != CompressionWorkersDefault ||
		policy.TinyStreamThresholdBytes != 0
}

func normalizeCompressionPolicy(policy CompressionPolicy) (CompressionPolicy, error) {
	defaults := defaultCompressionPolicy()
	if policy == (CompressionPolicy{}) {
		return defaults, nil
	}
	var enabled bool
	switch policy.Mode {
	case CompressionDefault:
		enabled = compressionPolicyHasFields(policy)
	case CompressionEnabled:
		enabled = true
	case CompressionDisabled:
		enabled = false
	default:
		return CompressionPolicy{}, fmt.Errorf("invalid compression mode: %d", policy.Mode)
	}
	if policy.Level == 0 && enabled {
		policy.Level = defaults.Level
	}
	if enabled && !validCompressionLevel(policy.Level) {
		return CompressionPolicy{}, fmt.Errorf("invalid compression level: %d", policy.Level)
	}
	if !enabled {
		policy.Level = zlib.NoCompression
	} else if policy.Level == zlib.NoCompression {
		enabled = false
	}
	if policy.PageWorkers < 0 {
		if policy.PageWorkers != CompressionWorkersDisabled {
			return CompressionPolicy{}, fmt.Errorf("invalid page compression workers: %d", policy.PageWorkers)
		}
		policy.PageWorkers = 0
	} else if policy.PageWorkers == CompressionWorkersDefault {
		policy.PageWorkers = defaults.PageWorkers
	}
	if policy.AttachmentWorkers < 0 {
		if policy.AttachmentWorkers != CompressionWorkersDisabled {
			return CompressionPolicy{}, fmt.Errorf("invalid attachment compression workers: %d", policy.AttachmentWorkers)
		}
		policy.AttachmentWorkers = 0
	} else if policy.AttachmentWorkers == CompressionWorkersDefault {
		policy.AttachmentWorkers = defaults.AttachmentWorkers
	}
	if policy.TinyStreamThresholdBytes < 0 {
		return CompressionPolicy{}, fmt.Errorf("invalid tiny stream threshold: %d", policy.TinyStreamThresholdBytes)
	}
	if policy.TinyStreamThresholdBytes == 0 {
		policy.TinyStreamThresholdBytes = defaults.TinyStreamThresholdBytes
	}
	if enabled {
		policy.Mode = CompressionEnabled
	} else {
		policy.Mode = CompressionDisabled
	}
	return policy, nil
}

// SetCompressionPolicy sets generated stream compression and background
// compression worker limits.
func (f *Document) SetCompressionPolicy(policy CompressionPolicy) error {
	policy, err := normalizeCompressionPolicy(policy)
	if err != nil {
		f.SetError(err)
		return err
	}
	f.compress = policy.Mode == CompressionEnabled
	f.compressLevel = policy.Level
	f.pageCompressionWorkers = policy.PageWorkers
	f.attachmentCompressionWorkers = policy.AttachmentWorkers
	f.compressionTinyStreamThreshold = policy.TinyStreamThresholdBytes
	return nil
}

// CompressionPolicy returns the document's current compression settings.
func (f *Document) CompressionPolicy() CompressionPolicy {
	return CompressionPolicy{
		Mode:                     compressionModeForEnabled(f.compress),
		Level:                    f.compressLevel,
		PageWorkers:              f.pageCompressionWorkers,
		AttachmentWorkers:        f.attachmentCompressionWorkers,
		TinyStreamThresholdBytes: f.compressionTinyStreamThreshold,
	}
}

func compressionModeForEnabled(enabled bool) CompressionMode {
	if enabled {
		return CompressionEnabled
	}
	return CompressionDisabled
}

// SetPageCompressionWorkers sets how many goroutines may compress page streams
// during output. Passing 0 disables background page compression; pages are then
// compressed synchronously as they are written.
func (f *Document) SetPageCompressionWorkers(workers int) {
	if workers < 0 {
		f.SetErrorf("invalid page compression workers: %d", workers)
		return
	}
	f.pageCompressionWorkers = workers
}

// SetAttachmentCompressionWorkers sets how many goroutines may compress
// embedded attachments during output. Passing 0 disables background attachment
// compression; attachments are then compressed synchronously as they are
// embedded.
func (f *Document) SetAttachmentCompressionWorkers(workers int) {
	if workers < 0 {
		f.SetErrorf("invalid attachment compression workers: %d", workers)
		return
	}
	f.attachmentCompressionWorkers = workers
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

const defaultTinyStreamCompressionThreshold = 32

func (f *Document) compressStreamBytes(data []byte) ([]byte, bool) {
	threshold := f.compressionTinyStreamThreshold
	if threshold <= 0 {
		threshold = defaultTinyStreamCompressionThreshold
	}
	if !f.compress || len(data) < threshold {
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
	f.state = documentStateOpen
}

// Close terminates the PDF document. It is not necessary to call this method
// explicitly because Output, OutputAndClose, and OutputFileAndClose do it
// automatically. If the document contains no page, AddPage is called to
// prevent the generation of an invalid document.
func (f *Document) Close() {
	f.closeContext(context.Background())
}

func (f *Document) closeContext(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := outputCanceledError(ctx); err != nil {
		f.SetError(err)
		return
	}
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
	if f.state == documentStateClosed {
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
	f.enddocContext(ctx)
}
