// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import "compress/zlib"

// Option configures document construction. Options are created by the With*
// functions in this package.
type Option func(*normalizedOptions)

// WithOrientation sets the default page orientation.
func WithOrientation(orientation Orientation) Option {
	return func(options *normalizedOptions) { options.orientationStr = orientation.String() }
}

// WithUnit sets the unit of measure used for document geometry.
func WithUnit(unit Unit) Option {
	return func(options *normalizedOptions) { options.unitStr = unit.String() }
}

// WithPageSize sets the named default page size.
func WithPageSize(pageSize PageSizeName) Option {
	return func(options *normalizedOptions) {
		options.sizeStr = pageSize.String()
		options.size = Size{}
	}
}

// WithCustomPageSize sets an explicit default page size in the configured unit.
func WithCustomPageSize(size Size) Option {
	return func(options *normalizedOptions) { options.size = size }
}

// WithFontDir sets the directory used for font resources.
func WithFontDir(fontDir string) Option {
	return func(options *normalizedOptions) { options.fontDirStr = fontDir }
}

// WithOptimize switches generated page and template streams to best zlib
// compression. It is not a whole-PDF optimizer.
func WithOptimize(optimize bool) Option {
	return func(options *normalizedOptions) {
		policy := defaultCompressionPolicy()
		if optimize {
			policy.Level = zlib.BestCompression
		}
		options.compressionPolicy = policy
		options.compressionPolicySet = true
	}
}

// WithBestCompression switches generated page and template streams to best zlib
// compression. It is equivalent to WithOptimize(true).
func WithBestCompression() Option { return WithOptimize(true) }

// WithCompressionPolicy sets explicit stream-compression behavior.
func WithCompressionPolicy(policy CompressionPolicy) Option {
	return func(options *normalizedOptions) {
		options.compressionPolicy = policy
		options.compressionPolicySet = true
	}
}

// WithLimits sets resource and document limits. Zero fields leave the package
// default or existing document setting unchanged.
func WithLimits(limits Limits) Option {
	return func(options *normalizedOptions) {
		options.limits = limits
		options.limitsSet = true
	}
}

// WithSecurityPolicy sets explicit feature gates. A zero SecurityPolicy denies
// all gated features because the policy was explicitly installed.
func WithSecurityPolicy(policy SecurityPolicy) Option {
	return func(options *normalizedOptions) {
		options.securityPolicy = policy
		options.securityPolicySet = true
	}
}

// WithOutputPolicy sets output-time defaults such as deterministic metadata,
// file sync behavior, and one-shot final streaming.
func WithOutputPolicy(policy OutputPolicy) Option {
	return func(options *normalizedOptions) {
		options.outputPolicy = policy
		options.outputPolicySet = true
		options.deterministicOutput = policy.Deterministic
	}
}

// WithHooks installs optional production diagnostics callbacks.
func WithHooks(hooks Hooks) Option {
	return func(options *normalizedOptions) {
		options.hooks = hooks
		options.hooksSet = true
	}
}

// WithDeterministicOutput enables deterministic output defaults for this
// document.
func WithDeterministicOutput() Option {
	return func(options *normalizedOptions) { options.deterministicOutput = true }
}

// WithProductionPolicy applies an operational policy. Later options override
// fields set by this policy.
func WithProductionPolicy(policy ProductionPolicy) Option {
	return func(options *normalizedOptions) {
		options.runtimePolicy = runtimePolicyFromProductionPolicy(policy)
	}
}

// WithServerSafeDefaults applies the built-in server-safe production policy.
// Later options override fields set by this option.
func WithServerSafeDefaults() Option { return WithProductionPolicy(ServerSafePolicy()) }

// WithNoCompression disables Flate compression for generated streams.
func WithNoCompression() Option {
	return func(options *normalizedOptions) {
		policy := defaultCompressionPolicy()
		policy.Mode = CompressionDisabled
		policy.Level = zlib.NoCompression
		options.compressionPolicy = policy
		options.compressionPolicySet = true
	}
}

// WithPageCompressionWorkers sets how many goroutines may compress page
// streams during output. Passing 0 disables background page compression.
func WithPageCompressionWorkers(workers int) Option {
	return func(options *normalizedOptions) {
		options.pageCompressionWorkers = workers
		options.pageCompressionWorkersSet = true
	}
}

// WithAttachmentCompressionWorkers sets how many goroutines may compress
// embedded attachments during output. Passing 0 disables background attachment
// compression.
func WithAttachmentCompressionWorkers(workers int) Option {
	return func(options *normalizedOptions) {
		options.attachmentCompressionWorkers = workers
		options.attachmentCompressionWorkersSet = true
	}
}

// WithResourceCachePolicy sets the cache policy for file-backed images and
// UTF-8 fonts loaded by path.
func WithResourceCachePolicy(policy ResourceCachePolicy) Option {
	return func(options *normalizedOptions) { options.cachePolicy = policy }
}

// WithImageCache uses cache for file-backed image registration. A nil cache
// leaves the cache policy unchanged.
func WithImageCache(cache *ImageCache) Option {
	return func(options *normalizedOptions) {
		if cache != nil {
			options.imageCache = cache
			options.imageCacheSet = true
		}
	}
}

// WithFontCache uses cache for UTF-8 font registration. A nil cache leaves the
// cache policy unchanged.
func WithFontCache(cache *FontCache) Option {
	return func(options *normalizedOptions) {
		if cache != nil {
			options.fontCache = cache
			options.fontCacheSet = true
		}
	}
}

// WithResourceLoader installs a generalized loader for supported resource
// kinds. Specialized loaders and explicit content still take precedence where
// their APIs define that behavior.
func WithResourceLoader(loader ResourceLoader) Option {
	return func(options *normalizedOptions) {
		if loader != nil {
			options.resourceLoader = loader
			options.resourceLoaderSet = true
		}
	}
}

func buildOptions(options ...Option) normalizedOptions {
	var cfg normalizedOptions
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
	runtimePolicy
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
