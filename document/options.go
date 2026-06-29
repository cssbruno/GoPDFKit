// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import "compress/zlib"

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

// WithCompressionPolicy sets explicit stream-compression behavior.
func WithCompressionPolicy(policy CompressionPolicy) Option {
	return func(options *Options) {
		policy := policy
		options.CompressionPolicy = &policy
	}
}

// WithLimits sets resource and document limits. Zero fields leave the package
// default or existing document setting unchanged.
func WithLimits(limits Limits) Option {
	return func(options *Options) {
		limits := limits
		options.Limits = &limits
	}
}

// WithSecurityPolicy sets explicit feature gates. A zero SecurityPolicy denies
// all gated features because the policy was explicitly installed.
func WithSecurityPolicy(policy SecurityPolicy) Option {
	return func(options *Options) {
		policy := policy
		options.SecurityPolicy = &policy
	}
}

// WithOutputPolicy sets output-time defaults such as deterministic metadata,
// file sync behavior, and one-shot final streaming.
func WithOutputPolicy(policy OutputPolicy) Option {
	return func(options *Options) {
		policy := policy
		options.OutputPolicy = &policy
	}
}

// WithHooks installs optional production diagnostics callbacks.
func WithHooks(hooks Hooks) Option {
	return func(options *Options) {
		hooks := hooks
		options.Hooks = &hooks
	}
}

// WithDeterministicOutput enables deterministic output defaults for this
// document.
func WithDeterministicOutput() Option {
	return func(options *Options) {
		options.DeterministicOutput = true
	}
}

// WithProductionPolicy applies an operational policy. Later options override
// fields set by this policy.
func WithProductionPolicy(policy ProductionPolicy) Option {
	return func(options *Options) {
		limits := policy.Limits
		security := policy.Security
		output := policy.Output
		hooks := policy.Hooks
		compression := policy.Compression
		options.Limits = &limits
		options.SecurityPolicy = &security
		options.OutputPolicy = &output
		options.Hooks = &hooks
		options.CompressionPolicy = &compression
		if policy.CacheSet || policy.Cache != ResourceCacheShared {
			options.CachePolicy = policy.Cache
		} else {
			options.CachePolicy = ResourceCacheDocument
		}
		options.DeterministicOutput = policy.Deterministic || policy.Output.Deterministic
	}
}

// WithServerSafeDefaults applies the built-in server-safe production policy.
// Later options override fields set by this option.
func WithServerSafeDefaults() Option {
	return WithProductionPolicy(ServerSafePolicy())
}

// WithNoCompression disables Flate compression for generated streams.
func WithNoCompression() Option {
	return func(options *Options) {
		policy := defaultCompressionPolicy()
		policy.Mode = CompressionDisabled
		policy.Enabled = false
		policy.Level = zlib.NoCompression
		options.CompressionPolicy = &policy
		options.Optimize = false
	}
}

// WithPageCompressionWorkers sets how many goroutines may compress page
// streams during output. Passing 0 disables background page compression.
func WithPageCompressionWorkers(workers int) Option {
	return func(options *Options) {
		workerCount := workers
		options.PageCompressionWorkers = &workerCount
	}
}

// WithAttachmentCompressionWorkers sets how many goroutines may compress
// embedded attachments during output. Passing 0 disables background attachment
// compression.
func WithAttachmentCompressionWorkers(workers int) Option {
	return func(options *Options) {
		workerCount := workers
		options.AttachmentCompressionWorkers = &workerCount
	}
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

// WithFontCache uses cache for UTF-8 font registration. A nil cache leaves the
// cache policy unchanged.
func WithFontCache(cache *FontCache) Option {
	return func(options *Options) {
		options.FontCache = cache
	}
}

// WithUTF8FontCache is an alias for WithFontCache.
func WithUTF8FontCache(cache *FontCache) Option {
	return WithFontCache(cache)
}

// WithResourceLoader installs a generalized loader for supported resource
// kinds. Specialized loaders and explicit content still take precedence where
// their APIs define that behavior.
func WithResourceLoader(loader ResourceLoader) Option {
	return func(options *Options) {
		options.ResourceLoader = loader
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
	orientationStr                  string
	unitStr                         string
	sizeStr                         string
	fontDirStr                      string
	size                            Size
	optimize                        bool
	compressionPolicy               CompressionPolicy
	compressionPolicySet            bool
	pageCompressionWorkers          int
	pageCompressionWorkersSet       bool
	attachmentCompressionWorkers    int
	attachmentCompressionWorkersSet bool
	cachePolicy                     ResourceCachePolicy
	imageCache                      *ImageCache
	imageCacheSet                   bool
	fontCache                       *FontCache
	fontCacheSet                    bool
	resourceLoader                  ResourceLoader
	resourceLoaderSet               bool
	limits                          Limits
	limitsSet                       bool
	securityPolicy                  SecurityPolicy
	securityPolicySet               bool
	outputPolicy                    OutputPolicy
	outputPolicySet                 bool
	hooks                           Hooks
	hooksSet                        bool
	deterministicOutput             bool
}

type runtimePolicy struct {
	compressionPolicy               CompressionPolicy
	compressionPolicySet            bool
	pageCompressionWorkers          int
	pageCompressionWorkersSet       bool
	attachmentCompressionWorkers    int
	attachmentCompressionWorkersSet bool
	cachePolicy                     ResourceCachePolicy
	imageCache                      *ImageCache
	imageCacheSet                   bool
	fontCache                       *FontCache
	fontCacheSet                    bool
	resourceLoader                  ResourceLoader
	resourceLoaderSet               bool
	limits                          Limits
	limitsSet                       bool
	securityPolicy                  SecurityPolicy
	securityPolicySet               bool
	outputPolicy                    OutputPolicy
	outputPolicySet                 bool
	hooks                           Hooks
	hooksSet                        bool
	deterministicOutput             bool
}

func (options Options) normalized() normalizedOptions {
	cfg := normalizedOptions{
		orientationStr:      options.Orientation.String(),
		unitStr:             options.Unit.String(),
		sizeStr:             options.PageSize.String(),
		fontDirStr:          options.FontDir,
		size:                options.Size,
		optimize:            options.Optimize,
		cachePolicy:         options.CachePolicy,
		imageCache:          options.ImageCache,
		imageCacheSet:       options.ImageCache != nil,
		fontCache:           options.FontCache,
		fontCacheSet:        options.FontCache != nil,
		resourceLoader:      options.ResourceLoader,
		resourceLoaderSet:   options.ResourceLoader != nil,
		deterministicOutput: options.DeterministicOutput,
	}
	if options.CompressionPolicy != nil {
		cfg.compressionPolicy = *options.CompressionPolicy
		cfg.compressionPolicySet = true
	}
	if options.PageCompressionWorkers != nil {
		cfg.pageCompressionWorkers = *options.PageCompressionWorkers
		cfg.pageCompressionWorkersSet = true
	}
	if options.AttachmentCompressionWorkers != nil {
		cfg.attachmentCompressionWorkers = *options.AttachmentCompressionWorkers
		cfg.attachmentCompressionWorkersSet = true
	}
	if options.Limits != nil {
		cfg.limits = *options.Limits
		cfg.limitsSet = true
	}
	if options.SecurityPolicy != nil {
		cfg.securityPolicy = *options.SecurityPolicy
		cfg.securityPolicySet = true
	}
	if options.OutputPolicy != nil {
		cfg.outputPolicy = *options.OutputPolicy
		cfg.outputPolicySet = true
		if options.OutputPolicy.Deterministic {
			cfg.deterministicOutput = true
		}
	}
	if options.Hooks != nil {
		cfg.hooks = *options.Hooks
		cfg.hooksSet = true
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

func (cfg normalizedOptions) runtimePolicy() runtimePolicy {
	return runtimePolicy{
		compressionPolicy:               cfg.compressionPolicy,
		compressionPolicySet:            cfg.compressionPolicySet,
		pageCompressionWorkers:          cfg.pageCompressionWorkers,
		pageCompressionWorkersSet:       cfg.pageCompressionWorkersSet,
		attachmentCompressionWorkers:    cfg.attachmentCompressionWorkers,
		attachmentCompressionWorkersSet: cfg.attachmentCompressionWorkersSet,
		cachePolicy:                     cfg.cachePolicy,
		imageCache:                      cfg.imageCache,
		imageCacheSet:                   cfg.imageCacheSet,
		fontCache:                       cfg.fontCache,
		fontCacheSet:                    cfg.fontCacheSet,
		resourceLoader:                  cfg.resourceLoader,
		resourceLoaderSet:               cfg.resourceLoaderSet,
		limits:                          cfg.limits,
		limitsSet:                       cfg.limitsSet,
		securityPolicy:                  cfg.securityPolicy,
		securityPolicySet:               cfg.securityPolicySet,
		outputPolicy:                    cfg.outputPolicy,
		outputPolicySet:                 cfg.outputPolicySet,
		hooks:                           cfg.hooks,
		hooksSet:                        cfg.hooksSet,
		deterministicOutput:             cfg.deterministicOutput,
	}
}
