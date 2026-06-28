// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

// Option mutates document construction settings.
type Option func(*Options)

// WithOrientation sets the default page orientation.
func WithOrientation(orientation Orientation) Option {
	return func(options *Options) {
		options.Orientation = orientation
	}
}

// WithUnit sets the unit of measure used for document geometry.
func WithUnit(unit Unit) Option {
	return func(options *Options) {
		options.Unit = unit
	}
}

// WithPageSize sets the named default page size.
func WithPageSize(pageSize PageSizeName) Option {
	return func(options *Options) {
		options.PageSize = pageSize
		options.Size = Size{}
	}
}

// WithCustomPageSize sets an explicit default page size in the configured unit.
func WithCustomPageSize(size Size) Option {
	return func(options *Options) {
		options.Size = size
	}
}

// WithFontDir sets the directory used for font resources.
func WithFontDir(fontDir string) Option {
	return func(options *Options) {
		options.FontDir = fontDir
	}
}

// WithOptimize switches generated page and template streams to best zlib
// compression. It is not a whole-PDF optimizer.
func WithOptimize(optimize bool) Option {
	return func(options *Options) {
		options.Optimize = optimize
	}
}

// WithBestCompression switches generated page and template streams to best zlib
// compression. It is equivalent to WithOptimize(true).
func WithBestCompression() Option {
	return WithOptimize(true)
}

// WithResourceCachePolicy sets the cache policy for file-backed images and
// UTF-8 fonts loaded by path.
func WithResourceCachePolicy(policy ResourceCachePolicy) Option {
	return func(options *Options) {
		options.CachePolicy = policy
	}
}

// WithImageCache uses cache for file-backed image registration. A nil cache
// leaves the cache policy unchanged.
func WithImageCache(cache *ImageCache) Option {
	return func(options *Options) {
		options.ImageCache = cache
	}
}

// WithLegacyConstructorArgs applies the string arguments accepted by New. It is
// mainly useful while migrating old code to typed construction options.
func WithLegacyConstructorArgs(orientationStr, unitStr, sizeStr, fontDirStr string) Option {
	return func(options *Options) {
		options.OrientationStr = orientationStr
		options.UnitStr = unitStr
		options.SizeStr = sizeStr
		options.FontDirStr = fontDirStr
	}
}

func buildOptions(options ...Option) Options {
	var cfg Options
	for _, option := range options {
		if option != nil {
			option(&cfg)
		}
	}
	return cfg
}

type normalizedOptions struct {
	orientationStr string
	unitStr        string
	sizeStr        string
	fontDirStr     string
	size           Size
	optimize       bool
	cachePolicy    ResourceCachePolicy
	imageCache     *ImageCache
	imageCacheSet  bool
}

func (options Options) normalized() normalizedOptions {
	cfg := normalizedOptions{
		orientationStr: options.Orientation.String(),
		unitStr:        options.Unit.String(),
		sizeStr:        options.PageSize.String(),
		fontDirStr:     options.FontDir,
		size:           options.Size,
		optimize:       options.Optimize,
		cachePolicy:    options.CachePolicy,
		imageCache:     options.ImageCache,
		imageCacheSet:  options.ImageCache != nil,
	}
	if cfg.orientationStr == "" {
		cfg.orientationStr = options.OrientationStr
	}
	if cfg.unitStr == "" {
		cfg.unitStr = options.UnitStr
	}
	if cfg.sizeStr == "" {
		cfg.sizeStr = options.SizeStr
	}
	if cfg.fontDirStr == "" {
		cfg.fontDirStr = options.FontDirStr
	}
	return cfg
}
