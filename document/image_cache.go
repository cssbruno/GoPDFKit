// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

// ImageCache stores parsed image data for reuse across documents.
//
// A Document already deduplicates images registered more than once inside that
// same document. ImageCache is for server-style workloads that create many
// documents with the same logos or other repeated assets.
type ImageCache struct {
	mu     sync.RWMutex
	images map[string]*ImageInfo
}

// NewImageCache creates an empty reusable image cache.
func NewImageCache() *ImageCache {
	return &ImageCache{images: make(map[string]*ImageInfo)}
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

	pdf := New("P", "mm", "A4", "")
	info := pdf.RegisterImageOptionsReader(name, options, r)
	if pdf.Err() {
		return nil, pdf.Error()
	}
	if info == nil {
		return nil, errors.New("image parser returned no image info")
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.images == nil {
		c.images = make(map[string]*ImageInfo)
	}
	c.images[name] = info.clone()
	return c.images[name].cloneMetadata(), nil
}

// RegisterImageOptions parses and stores an image from a file.
func (c *ImageCache) RegisterImageOptions(name, fileStr string, options ImageOptions) (*ImageInfo, error) {
	if name == "" {
		name = fileStr
	}
	file, err := os.Open(fileStr)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	if options.ImageType == "" {
		pos := strings.LastIndex(fileStr, ".")
		if pos < 0 {
			return nil, fmt.Errorf("image file has no extension and no type was specified: %s", fileStr)
		}
		options.ImageType = fileStr[pos+1:]
	}
	return c.RegisterImageOptionsReader(name, options, file)
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
	if info, ok := f.images[name]; ok {
		return info
	}
	info, ok := cache.Get(name)
	if !ok {
		f.err = fmt.Errorf("image cache entry not found: %s", name)
		return nil
	}
	recomputeID := info.i == "" || info.scale != f.k
	info.scale = f.k
	if info.dpi == 0 {
		info.dpi = 72
	}
	if recomputeID {
		if info.i, f.err = generateImageID(info); f.err != nil {
			return nil
		}
	}
	f.images[name] = info
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
