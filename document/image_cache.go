// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// ImageCache stores parsed image data for reuse across documents.
//
// A Document already deduplicates images registered more than once inside that
// same document. ImageCache is for server-style workloads that create many
// documents with the same logos or other repeated assets. Its methods are safe
// for concurrent use.
type ImageCache struct {
	mu                sync.RWMutex
	images            map[string]*ImageInfo
	fileImages        map[imageFileCacheKey]*ImageInfo
	fileImageOrder    []imageFileCacheKey
	fileImageBytes    int64
	maxFileImageBytes int64
	fileTypes         map[string]string
}

type imageFileCacheKey struct {
	path      string
	size      int64
	modTime   int64
	imageType string
	readDpi   bool
}

const maxSharedImageFileCacheBytes = 128 * 1024 * 1024

var sharedImageFileCache = newImageCache(maxSharedImageFileCacheBytes)

// NewImageCache creates an empty reusable image cache.
func NewImageCache() *ImageCache {
	return newImageCache(0)
}

func newImageCache(maxFileImageBytes int64) *ImageCache {
	return &ImageCache{
		images:            make(map[string]*ImageInfo),
		fileImages:        make(map[imageFileCacheKey]*ImageInfo),
		maxFileImageBytes: maxFileImageBytes,
		fileTypes:         make(map[string]string),
	}
}

// RegisterImageOptionsReader parses and stores an image read from r.
// ImageType must be set in options when registering from a reader.
func (c *ImageCache) RegisterImageOptionsReader(name string, options ImageOptions, r io.Reader) (*ImageInfo, error) {
	if c == nil {
		return nil, errors.New("image cache is nil")
	}
	if name == "" {
		return nil, errors.New("image cache name is empty")
	}
	if r == nil {
		return nil, errors.New("image reader is nil")
	}
	if options.ImageType == "" {
		return nil, errors.New("image type should be specified if reading from custom reader")
	}
	options.ImageType = normalizeImageType(options.ImageType)

	info, _, err := parseImageOptionsReader(options, r, 1, defaultCompressionLevel(), "")
	if err != nil {
		return nil, err
	}
	if info.i, err = generateImageID(info); err != nil {
		return nil, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.images == nil {
		c.images = make(map[string]*ImageInfo)
	}
	c.images[name] = info.cloneMetadata()
	return c.images[name].cloneMetadata(), nil
}

// RegisterImageOptions parses and stores an image from a file.
func (c *ImageCache) RegisterImageOptions(name, fileStr string, options ImageOptions) (*ImageInfo, error) {
	info, _, err := c.registerImageOptions(name, fileStr, options)
	return info, err
}

func (c *ImageCache) registerImageOptions(name, fileStr string, options ImageOptions) (*ImageInfo, bool, error) {
	if c == nil {
		return nil, false, errors.New("image cache is nil")
	}
	if name == "" {
		name = fileStr
	}
	cachePath := imageCachePath(fileStr)
	if c.maxFileImageBytes > 0 && name == fileStr {
		name = cachePath
	}
	imageType, err := c.imageTypeForFile(cachePath, fileStr, options.ImageType)
	if err != nil {
		return nil, false, err
	}
	options.ImageType = imageType
	stat, err := os.Stat(fileStr)
	if err != nil {
		return nil, false, err
	}
	key := imageFileCacheKey{
		path:      cachePath,
		size:      stat.Size(),
		modTime:   stat.ModTime().UnixNano(),
		imageType: options.ImageType,
		readDpi:   options.ReadDpi,
	}
	c.mu.RLock()
	cached := c.fileImages[key]
	c.mu.RUnlock()
	if cached != nil {
		c.mu.Lock()
		if c.images == nil {
			c.images = make(map[string]*ImageInfo)
		}
		c.images[name] = cached.cloneMetadata()
		info := c.images[name].cloneMetadata()
		c.mu.Unlock()
		return info, true, nil
	}
	file, err := os.Open(fileStr) // #nosec G304 -- Cache API accepts explicit caller-selected image paths.
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = file.Close() }()
	info, err := c.RegisterImageOptionsReader(name, options, file)
	if err != nil {
		return nil, false, err
	}
	c.mu.Lock()
	if c.fileImages == nil {
		c.fileImages = make(map[imageFileCacheKey]*ImageInfo)
	}
	if cached := c.images[name]; cached != nil {
		c.storeFileImageLocked(key, name, cached)
	}
	c.mu.Unlock()
	return info, false, nil
}

func (c *ImageCache) resourceImage(name string, key imageFileCacheKey) (*ImageInfo, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.RLock()
	cached := c.fileImages[key]
	c.mu.RUnlock()
	if cached == nil {
		return nil, false
	}
	c.mu.Lock()
	if c.images == nil {
		c.images = make(map[string]*ImageInfo)
	}
	c.images[name] = cached.cloneMetadata()
	info := c.images[name].cloneMetadata()
	c.mu.Unlock()
	return info, true
}

func (c *ImageCache) storeResourceImage(name string, key imageFileCacheKey, info *ImageInfo) {
	if c == nil || info == nil {
		return
	}
	c.mu.Lock()
	if c.images == nil {
		c.images = make(map[string]*ImageInfo)
	}
	c.images[name] = info.cloneMetadata()
	c.storeFileImageLocked(key, name, c.images[name])
	c.mu.Unlock()
}

func (c *ImageCache) storeFileImageLocked(key imageFileCacheKey, name string, info *ImageInfo) {
	if c.fileImages == nil {
		c.fileImages = make(map[imageFileCacheKey]*ImageInfo)
	}
	if c.maxFileImageBytes <= 0 {
		c.fileImages[key] = info.cloneMetadata()
		return
	}
	entryBytes := imageInfoCacheBytes(info)
	if entryBytes > c.maxFileImageBytes {
		delete(c.images, name)
		return
	}
	if old, ok := c.fileImages[key]; ok {
		c.fileImageBytes -= imageInfoCacheBytes(old)
	} else {
		c.fileImageOrder = append(c.fileImageOrder, key)
	}
	c.fileImages[key] = info.cloneMetadata()
	c.fileImageBytes += entryBytes
	for c.fileImageBytes > c.maxFileImageBytes && len(c.fileImageOrder) > 0 {
		evict := c.fileImageOrder[0]
		c.fileImageOrder = c.fileImageOrder[1:]
		if old, ok := c.fileImages[evict]; ok {
			c.fileImageBytes -= imageInfoCacheBytes(old)
			delete(c.fileImages, evict)
			delete(c.images, evict.path)
		}
	}
}

func imageInfoCacheBytes(info *ImageInfo) int64 {
	if info == nil {
		return 0
	}
	return int64(len(info.data)+len(info.smask)+len(info.pal)+len(info.trns)*8) + 256
}

func imageCachePath(fileStr string) string {
	if abs, err := filepath.Abs(fileStr); err == nil {
		return abs
	}
	return fileStr
}

func imageResourceCacheKey(info ResourceInfo, options ImageOptions) imageFileCacheKey {
	return imageFileCacheKey{
		path:      "resource:" + info.StableID,
		size:      info.Size,
		modTime:   info.ModTime.UnixNano(),
		imageType: normalizeImageType(options.ImageType),
		readDpi:   options.ReadDpi,
	}
}

func (c *ImageCache) imageTypeForFile(cachePath, fileStr, imageType string) (string, error) {
	if imageType != "" {
		return normalizeImageType(imageType), nil
	}
	c.mu.RLock()
	if c.fileTypes != nil {
		if cached, ok := c.fileTypes[cachePath]; ok {
			c.mu.RUnlock()
			return cached, nil
		}
	}
	c.mu.RUnlock()
	inferred, ok := inferImageTypeFromPath(fileStr)
	if !ok {
		return "", fmt.Errorf("image file has no extension and no type was specified: %s", fileStr)
	}
	c.mu.Lock()
	if c.fileTypes == nil {
		c.fileTypes = make(map[string]string)
	}
	c.fileTypes[cachePath] = inferred
	c.mu.Unlock()
	return inferred, nil
}

// Get returns a copy of the parsed image stored under name.
func (c *ImageCache) Get(name string) (*ImageInfo, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	info, ok := c.images[name]
	if !ok || info == nil {
		return nil, false
	}
	return info.cloneMetadata(), true
}

// RegisterImageFromCache registers a cached image with this document.
// The returned ImageInfo can be used with ImageOptions using the same name.
func (f *Document) RegisterImageFromCache(name string, cache *ImageCache) *ImageInfo {
	if f.err != nil {
		return nil
	}
	if name == "" {
		f.err = errors.New("image cache name is empty")
		return nil
	}
	resources := f.ensureResourceStore()
	if info, ok := resources.image(name); ok {
		return info
	}
	info, ok := cache.Get(name)
	if !ok {
		if f.hooks.OnResourceCacheMiss != nil {
			f.hooks.OnResourceCacheMiss("image", name)
		}
		f.err = fmt.Errorf("image cache entry not found: %s", name)
		return nil
	}
	if f.hooks.OnResourceCacheHit != nil {
		f.hooks.OnResourceCacheHit("image", name)
	}
	return f.registerCachedImageInfo(name, info)
}

func (f *Document) registerCachedImageInfo(name string, info *ImageInfo) *ImageInfo {
	if info == nil {
		f.err = errors.New("image cache entry is invalid")
		return nil
	}
	info.scale = f.k
	if info.dpi == 0 {
		info.dpi = 72
	}
	if len(info.smask) > 0 {
		f.requirePDFVersion("1.4")
	}
	if info.i == "" {
		if info.i, f.err = generateImageID(info); f.err != nil {
			return nil
		}
	}
	if err := f.validateImageInfoLimits(info); err != nil {
		f.err = err
		return nil
	}
	f.ensureResourceStore().setImage(name, info)
	return info
}

// ImageFromCache places a cached image on the current page.
func (f *Document) ImageFromCache(name string, cache *ImageCache, x, y, w, h float64, flow bool, options ImageOptions, link int, linkStr string) {
	if f.err != nil {
		return
	}
	info := f.RegisterImageFromCache(name, cache)
	if f.err != nil {
		return
	}
	f.imageOut(info, x, y, w, h, options.AllowNegativePosition, flow, link, linkStr, taggedContentOptions{
		Role:     taggedRoleFigure,
		AltText:  options.AltText,
		Artifact: options.Artifact,
	})
}
