// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

// DocumentStats summarizes the current in-memory shape of a Document.
//
// Counts reflect registered resources before final PDF serialization. The
// memory estimate is intentionally approximate and is meant for operational
// diagnostics, not heap accounting.
type DocumentStats struct {
	Pages                int
	Images               int
	Fonts                int
	Templates            int
	Attachments          int
	ImportedPages        int
	EstimatedMemoryBytes int64
}

// CacheStats summarizes a reusable resource cache.
//
// Entries counts cached resource records. Bytes is the approximate amount of
// payload data retained by the cache. Image cache aliases that share one parsed
// image payload are counted once.
type CacheStats struct {
	Entries int
	Bytes   int64
}

// SharedCachesStats summarizes package-level resource caches.
type SharedCachesStats struct {
	Images CacheStats
	Fonts  CacheStats
	HTML   CacheStats
}

// Stats returns a best-effort snapshot of the document's current resource
// counts and buffered data size.
func (f *Document) Stats() DocumentStats {
	if f == nil {
		return DocumentStats{}
	}
	resources := f.ensureResourceStore()
	pageCount := f.PageCount()
	if pageCount < 0 {
		pageCount = 0
	}
	stats := DocumentStats{
		Pages:                pageCount,
		Images:               len(resources.images),
		Fonts:                len(resources.fonts),
		Templates:            len(resources.templates),
		Attachments:          len(f.attachments) + countAnnotationAttachments(f.pageAttachments),
		ImportedPages:        len(resources.importedPages),
		EstimatedMemoryBytes: f.estimatedMemoryBytes(),
	}
	return stats
}

func countAnnotationAttachments(pageAttachments [][]annotationAttach) int {
	count := 0
	for _, attachments := range pageAttachments {
		count += len(attachments)
	}
	return count
}

func (f *Document) estimatedMemoryBytes() int64 {
	if f == nil {
		return 0
	}
	resources := f.ensureResourceStore()
	var size int64
	size += int64(f.buffer.Len())
	for _, page := range f.pages {
		if page != nil {
			size += int64(page.Len())
		}
	}
	for _, info := range resources.images {
		size += imageInfoCacheBytes(info)
	}
	for _, font := range resources.fonts {
		size += int64(len(font.Cw) * 4)
		if font.utf8File != nil && font.utf8File.fileReader != nil {
			size += int64(len(font.utf8File.fileReader.array))
		}
	}
	for _, template := range resources.templates {
		if concrete, ok := template.(*DocumentTpl); ok {
			for _, page := range concrete.bytes {
				size += int64(len(page))
			}
		}
	}
	for _, attachment := range f.attachments {
		size += attachmentEstimatedBytes(attachment)
	}
	for _, attachments := range f.pageAttachments {
		for _, attachment := range attachments {
			if attachment.Attachment != nil {
				size += attachmentEstimatedBytes(*attachment.Attachment)
			}
		}
	}
	for _, page := range resources.importedPages {
		if page == nil {
			continue
		}
		for _, object := range page.rewrittenObjects {
			size += int64(len(object))
		}
		size += int64(len(page.rewrittenResources))
	}
	for _, object := range resources.importedObjs {
		size += int64(len(object))
	}
	size += int64(cap(f.contentScratch))
	size += int64(len(f.xmp))
	return size
}

func attachmentEstimatedBytes(attachment Attachment) int64 {
	size := int64(len(attachment.Content))
	size += int64(len(attachment.FilePath) + len(attachment.Filename) + len(attachment.Description) + len(attachment.MIMEType))
	return size
}

// Stats returns a snapshot of this cache's retained image data.
func (c *ImageCache) Stats() CacheStats {
	if c == nil {
		return CacheStats{}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	stats := CacheStats{}
	seen := make(map[string]bool, len(c.images)+len(c.fileImages))
	for _, info := range c.fileImages {
		addImageCacheStats(&stats, seen, info)
	}
	for _, info := range c.images {
		addImageCacheStats(&stats, seen, info)
	}
	return stats
}

func addImageCacheStats(stats *CacheStats, seen map[string]bool, info *ImageInfo) {
	if info == nil {
		return
	}
	key := info.i
	if key != "" {
		if seen[key] {
			return
		}
		seen[key] = true
	}
	stats.Entries++
	stats.Bytes += imageInfoCacheBytes(info)
}

// Clear removes all entries from this image cache.
func (c *ImageCache) Clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.images = make(map[string]*ImageInfo)
	c.fileImages = make(map[imageFileCacheKey]*ImageInfo)
	c.fileImageOrder = nil
	c.fileImageBytes = 0
	c.fileTypes = make(map[string]string)
	c.mu.Unlock()
}

// Stats returns a snapshot of this cache's retained UTF-8 font data.
func (c *FontCache) Stats() CacheStats {
	if c == nil {
		return CacheStats{}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	stats := CacheStats{Entries: len(c.fonts)}
	for _, font := range c.fonts {
		stats.Bytes += int64(len(font.data))
	}
	return stats
}

// Clear removes all entries from this UTF-8 font cache.
func (c *FontCache) Clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.fonts = make(map[string]cachedUTF8Font)
	c.mu.Unlock()
}

// SharedCacheStats returns a snapshot of package-level image, UTF-8 font, and
// compiled HTML caches.
func SharedCacheStats() SharedCachesStats {
	return SharedCachesStats{
		Images: sharedImageCacheStats(),
		Fonts:  sharedFontCacheStats(),
		HTML:   sharedCompiledHTMLCacheStats(),
	}
}

// ClearSharedCaches removes all package-level resource cache entries.
func ClearSharedCaches() {
	if sharedImageFileCache != nil {
		sharedImageFileCache.Clear()
	}
	clearSharedCompiledHTMLCache()
	sharedUTF8FontFileCache.Lock()
	sharedUTF8FontFileCache.fonts = make(map[sharedUTF8FontFileCacheKey]cachedUTF8Font)
	sharedUTF8FontFileCache.order = nil
	sharedUTF8FontFileCache.bytes = 0
	sharedUTF8FontFileCache.Unlock()
}

func sharedImageCacheStats() CacheStats {
	if sharedImageFileCache == nil {
		return CacheStats{}
	}
	return sharedImageFileCache.Stats()
}

func sharedFontCacheStats() CacheStats {
	sharedUTF8FontFileCache.Lock()
	defer sharedUTF8FontFileCache.Unlock()
	return CacheStats{
		Entries: len(sharedUTF8FontFileCache.fonts),
		Bytes:   sharedUTF8FontFileCache.bytes,
	}
}
