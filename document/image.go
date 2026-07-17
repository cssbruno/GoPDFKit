// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ImageTypeFromMime returns the image type used in various image-related
// functions, such as ImageOptions(), for the specified MIME type. For example,
// "jpg" is returned if mimeStr is "image/jpeg". An error is set if the
// specified MIME type is not supported.
func (f *Document) ImageTypeFromMime(mimeStr string) (tp string) {
	switch mimeStr {
	case "image/png":
		tp = "png"
	case "image/jpg":
		tp = "jpg"
	case "image/jpeg":
		tp = "jpg"
	case "image/gif":
		tp = "gif"
	case "image/webp":
		tp = "webp"
	default:
		f.SetErrorf("unsupported image type: %s", mimeStr)
	}
	return
}

// ImageOptions puts a JPEG, PNG, GIF, or WebP image on the current page. The
// size it takes on the page can be specified in different ways. If both w and h
// are 0, the image is rendered at 96 DPI. If either w or h is zero, it is
// calculated from the other dimension so the aspect ratio is maintained.
// If w and/or h are -1, the DPI for that dimension will be read from the
// ImageInfo object. PNG files can contain DPI information, and if present,
// this information will be populated in the ImageInfo object and used in
// Width, Height, and Extent calculations. Otherwise, the SetDpi function can
// be used to change the DPI from the default of 72.
//
// If w and h are any other negative value, their absolute values indicate
// their DPI extents.
//
// Supported JPEG formats are 24-bit, 32-bit, and grayscale. Supported PNG
// formats are 24-bit, indexed color, and 8-bit indexed grayscale. GIF and
// WebP images are converted to PNG before embedding. If a GIF image is
// animated, only the first frame is rendered. Transparency is supported. It is
// possible to put a link on the image.
//
// imageNameStr may be the name of an image as registered with a call to either
// RegisterImageOptionsReader() or RegisterImageOptions(). In the first case,
// the image is loaded using an io.Reader. This is generally useful when the
// image is obtained from a source other than a disk-based file. In the second
// case, the image is loaded as a file. Alternatively, imageNameStr may directly
// specify a sufficiently qualified filename.
//
// However the image is loaded, if it is used more than once only one copy is
// embedded in the file.
//
// If x is negative, the current abscissa is used.
//
// If flow is true, the current y value is advanced after placing the image and
// a page break may be made if necessary.
//
// If link refers to an internal page anchor (that is, it is non-zero; see
// AddLink()), the image will be a clickable internal link. Otherwise, if
// linkStr specifies a URL, the image will be a clickable external link.
func (f *Document) ImageOptions(imageNameStr string, x, y, w, h float64, flow bool, options ImageOptions, link int, linkStr string) {
	if f.err != nil {
		return
	}
	info := f.RegisterImageOptions(imageNameStr, options)
	if f.err != nil {
		return
	}
	f.imageOut(info, x, y, w, h, options.AllowNegativePosition, flow, link, linkStr, taggedContentOptions{
		Role:     taggedRoleFigure,
		AltText:  options.AltText,
		Artifact: options.Artifact,
	})
}

// ImageOptions configures image parsing and placement.
//
// ImageType's possible values are case-insensitive:
// "JPG", "JPEG", "PNG", "GIF", and "WEBP". If empty, the type is inferred
// from the file extension.
//
// ReadDpi controls whether image DPI information is read automatically from the
// image file. Normally, this should be set to true, although not all images
// include DPI metadata. For backward compatibility with previous versions of
// the API, it defaults to false.
//
// AllowNegativePosition can be set to true to prevent the default
// coercion of negative x values to the current x position.
type ImageOptions struct {
	ImageType             string // Explicit image type: jpg, png, gif, or webp.
	ReadDpi               bool   // Whether to read DPI metadata from the image.
	AllowNegativePosition bool   // Whether negative coordinates are preserved.
	AltText               string // Alternate text for meaningful PDF/UA image content.
	Artifact              bool   // Whether the image is decorative and should be tagged as an artifact.
}

// RegisterImageOptionsReader registers an image by reading it from r, adding it
// to the PDF file but not adding it to the page. Use ImageOptions() with the
// same name to add the image to the page. Note that ImageType should
// be specified in this case.
//
// See ImageOptions() for restrictions on the image and the options parameters.
func (f *Document) RegisterImageOptionsReader(imgName string, options ImageOptions, r io.Reader) (info *ImageInfo) {
	info, err := f.RegisterImageOptionsReaderError(imgName, options, r)
	if err != nil {
		f.err = err
	}
	return info
}

// RegisterImageOptionsReaderError registers an image from r and returns
// parsing failures directly.
func (f *Document) RegisterImageOptionsReaderError(imgName string, options ImageOptions, r io.Reader) (*ImageInfo, error) {
	return f.RegisterImageOptionsReaderContext(context.Background(), imgName, options, r)
}

// RegisterImageOptionsReaderContext registers an image from r and checks ctx
// while reading image bytes and around format parsing.
func (f *Document) RegisterImageOptionsReaderContext(ctx context.Context, imgName string, options ImageOptions, r io.Reader) (*ImageInfo, error) {
	if f.err != nil {
		return nil, f.err
	}
	if err := outputCanceledError(ctx); err != nil {
		return nil, err
	}
	if strings.TrimSpace(imgName) == "" {
		return nil, errors.New("image name should not be blank")
	}
	resources := f.ensureResourceStore()
	info, ok := resources.image(imgName)
	if ok {
		if info == nil {
			return nil, fmt.Errorf("registered image is invalid: %s", imgName)
		}
		return info, nil
	}
	info, minVersion, err := parseImageOptionsReaderWithLimitsContext(ctx, options, r, f.k, f.compressLevel, f.pdfVersion, f.imageSourceLimit(), f.imageDecodedLimit())
	if err != nil {
		return nil, err
	}
	f.requirePDFVersion(minVersion)
	if info.i, err = generateImageID(info); err != nil {
		return nil, err
	}
	resources.setImage(imgName, info)
	return info, nil
}

// RegisterImageOptions registers an image, adding it to the PDF file but not
// adding it to the page. File-backed images are cached across documents by
// path, stat metadata, image type, and DPI options. Use ImageOptions() with the
// same filename to add the image to the page. ImageOptions() calls this
// function, so this function is only necessary if you need information about
// the image before placing it. See ImageOptions() for restrictions on the image
// and options parameters.
func (f *Document) RegisterImageOptions(fileStr string, options ImageOptions) (info *ImageInfo) {
	info, err := f.RegisterImageOptionsError(fileStr, options)
	if err != nil {
		f.err = err
	}
	return info
}

// RegisterImageOptionsError registers an image from fileStr and returns
// failures directly.
func (f *Document) RegisterImageOptionsError(fileStr string, options ImageOptions) (*ImageInfo, error) {
	if f.err != nil {
		return nil, f.err
	}
	resources := f.ensureResourceStore()
	info, ok := resources.image(fileStr)
	if ok {
		if err := f.validateImageInfoLimits(info); err != nil {
			return nil, err
		}
		return info, nil
	}
	if f.resourceLoader != nil {
		return f.registerImageOptionsResource(context.Background(), fileStr, options)
	}
	if err := f.checkImageFileSourceLimit(fileStr); err != nil {
		return nil, err
	}
	if f.imageCache == nil {
		file, err := openImageFile(fileStr)
		if err != nil {
			return nil, err
		}
		defer func() { _ = file.Close() }()
		if options.ImageType == "" {
			imageType, ok := inferImageTypeFromPath(fileStr)
			if !ok {
				return nil, fmt.Errorf("image file has no extension and no type was specified: %s", fileStr)
			}
			options.ImageType = imageType
		}
		info, minVersion, err := parseImageOptionsReaderWithLimits(options, file, f.k, f.compressLevel, f.pdfVersion, f.imageSourceLimit(), f.imageDecodedLimit())
		if err != nil {
			return nil, err
		}
		f.requirePDFVersion(minVersion)
		if info.i, err = generateImageID(info); err != nil {
			return nil, err
		}
		resources.setImage(fileStr, info)
		return info, nil
	}
	info, hit, err := f.imageCache.registerImageOptions(fileStr, fileStr, options)
	if err != nil {
		return nil, err
	}
	if hit {
		if f.hooks.OnResourceCacheHit != nil {
			f.hooks.OnResourceCacheHit("image", fileStr)
		}
	} else if f.hooks.OnResourceCacheMiss != nil {
		f.hooks.OnResourceCacheMiss("image", fileStr)
	}
	registered := f.registerCachedImageInfo(fileStr, info)
	if f.err != nil {
		return nil, f.err
	}
	if registered == nil {
		return nil, errors.New("image cache registration returned nil image info")
	}
	if len(registered.smask) > 0 {
		f.requirePDFVersion("1.4")
	}
	if err := f.validateImageInfoLimits(registered); err != nil {
		return nil, err
	}
	return registered, nil
}

func (f *Document) registerImageOptionsResource(ctx context.Context, name string, options ImageOptions) (*ImageInfo, error) {
	reader, info, err := f.resourceLoader.OpenResource(ctx, ResourceImage, name)
	if err != nil {
		return nil, err
	}
	if reader == nil {
		return nil, fmt.Errorf("resource loader returned nil reader: %s", name)
	}
	defer func() { _ = reader.Close() }()
	if f.limits.MaxImageSourceBytes > 0 && info.Size > f.limits.MaxImageSourceBytes {
		err := fmt.Errorf("%w: source image exceeds maximum size", ErrImageTooLarge)
		f.SetError(err)
		return nil, err
	}
	if options.ImageType == "" {
		imageType, ok := inferImageTypeFromPath(name)
		if !ok {
			return nil, fmt.Errorf("image resource has no extension and no type was specified: %s", name)
		}
		options.ImageType = imageType
	}
	options.ImageType = normalizeImageType(options.ImageType)
	var cacheKey imageFileCacheKey
	if f.imageCache != nil && info.StableID != "" {
		cacheKey = imageResourceCacheKey(info, options)
		if cached, ok := f.imageCache.resourceImage(name, cacheKey); ok {
			if f.hooks.OnResourceCacheHit != nil {
				f.hooks.OnResourceCacheHit("image", name)
			}
			return f.registerCachedImageInfo(name, cached), f.err
		}
		if f.hooks.OnResourceCacheMiss != nil {
			f.hooks.OnResourceCacheMiss("image", name)
		}
	}
	parsed, minVersion, err := parseImageOptionsReaderWithLimitsContext(ctx, options, reader, f.k, f.compressLevel, f.pdfVersion, f.imageSourceLimit(), f.imageDecodedLimit())
	if err != nil {
		return nil, err
	}
	f.requirePDFVersion(minVersion)
	if parsed.i, err = generateImageID(parsed); err != nil {
		return nil, err
	}
	if cacheKey.path != "" {
		f.imageCache.storeResourceImage(name, cacheKey, parsed)
	}
	resources := f.ensureResourceStore()
	resources.setImage(name, parsed)
	if err := f.validateImageInfoLimits(parsed); err != nil {
		return nil, err
	}
	return parsed, nil
}

func (f *Document) checkImageFileSourceLimit(fileStr string) error {
	if f.limits.MaxImageSourceBytes <= 0 {
		return nil
	}
	if info, err := os.Stat(fileStr); err == nil && info.Mode().IsRegular() && info.Size() > f.limits.MaxImageSourceBytes {
		err := fmt.Errorf("%w: source image exceeds maximum size", ErrImageTooLarge)
		f.SetError(err)
		return err
	}
	return nil
}

func (f *Document) validateImageInfoLimits(info *ImageInfo) error {
	if info == nil || f.limits.MaxImageDecodedBytes <= 0 {
		return nil
	}
	decoded := estimatedImageDecodedBytes(info)
	if decoded > f.limits.MaxImageDecodedBytes {
		err := fmt.Errorf("%w: decoded image exceeds maximum size", ErrImageTooLarge)
		f.SetError(err)
		return err
	}
	return nil
}

func estimatedImageDecodedBytes(info *ImageInfo) int64 {
	if info == nil || info.w <= 0 || info.h <= 0 {
		return 0
	}
	components := int64(3)
	switch info.cs {
	case "DeviceGray", "Indexed":
		components = 1
	case "DeviceCMYK":
		components = 4
	}
	bpc := int64(info.bpc)
	if bpc <= 0 {
		bpc = 8
	}
	pixels := int64(info.w) * int64(info.h)
	if pixels <= 0 {
		return 0
	}
	return (pixels*components*bpc + 7) / 8
}

func parseImageOptionsReader(options ImageOptions, r io.Reader, scale float64, compressLevel int, pdfVersion string) (*ImageInfo, string, error) {
	return parseImageOptionsReaderWithLimitsContext(context.Background(), options, r, scale, compressLevel, pdfVersion, maxImageSourceBytes, maxImageDecodedBytes)
}

func parseImageOptionsReaderWithLimits(options ImageOptions, r io.Reader, scale float64, compressLevel int, pdfVersion string, sourceLimit, decodedLimit int) (*ImageInfo, string, error) {
	return parseImageOptionsReaderWithLimitsContext(context.Background(), options, r, scale, compressLevel, pdfVersion, sourceLimit, decodedLimit)
}

func parseImageOptionsReaderWithLimitsContext(ctx context.Context, options ImageOptions, r io.Reader, scale float64, compressLevel int, pdfVersion string, sourceLimit, decodedLimit int) (*ImageInfo, string, error) {
	if r == nil {
		return nil, "", errors.New("image reader is nil")
	}
	if err := outputCanceledError(ctx); err != nil {
		return nil, "", err
	}
	if options.ImageType == "" {
		return nil, "", errors.New("image type should be specified if reading from custom reader")
	}
	parser := newImageParserWithLimits(scale, compressLevel, pdfVersion, sourceLimit, decodedLimit)
	r = contextReader{ctx: ctx, r: r}
	imageType := normalizeImageType(options.ImageType)
	var info *ImageInfo
	switch imageType {
	case "jpg":
		info = parser.parsejpg(r)
	case "png":
		info = parser.parsepng(r, options.ReadDpi)
	case "gif":
		info = parser.parsegif(r)
	case "webp":
		info = parser.parsewebp(r)
	default:
		return nil, "", fmt.Errorf("unsupported image type: %s", imageType)
	}
	if parser.err != nil {
		return nil, "", parser.err
	}
	if err := outputCanceledError(ctx); err != nil {
		return nil, "", err
	}
	if info == nil {
		return nil, "", errors.New("image parser returned no image info")
	}
	return info, parser.pdfVersion, nil
}

func openImageFile(fileStr string) (*os.File, error) {
	return os.Open(fileStr) // #nosec G304 -- RegisterImageOptions is an explicit caller-path API.
}

func normalizeImageType(imageType string) string {
	switch imageType {
	case "jpg", "png", "gif", "webp":
		return imageType
	case "jpeg":
		return "jpg"
	case "JPG", "PNG", "GIF", "WEBP":
		return strings.ToLower(imageType)
	case "JPEG":
		return "jpg"
	}
	normalized := strings.ToLower(strings.TrimSpace(imageType))
	if normalized == "jpeg" {
		return "jpg"
	}
	return normalized
}

func inferImageTypeFromPath(path string) (string, bool) {
	ext := filepath.Ext(path)
	if len(ext) <= 1 {
		return "", false
	}
	return normalizeImageType(ext[1:]), true
}

// ImportObjects imports external template objects into the current document.
func (f *Document) ImportObjects(objs map[string][]byte) {
	resources := f.ensureResourceStore()
	for name, data := range objs {
		resources.addImportedObject(name, data)
	}
}

// ImportObjPos imports external template object hash positions.
func (f *Document) ImportObjPos(objPos map[string]map[int]string) {
	resources := f.ensureResourceStore()
	for name, positions := range objPos {
		resources.addImportedObjectPositions(name, positions)
	}
}

// UseImportedTemplate draws an imported PDF template onto the current page.
func (f *Document) UseImportedTemplate(tplName string, scaleX float64, scaleY float64, tX float64, tY float64) {
	if f.err != nil {
		return
	}
	if f.page <= 0 {
		f.SetErrorf("cannot use an imported template without first adding a page")
		return
	}
	if !validPDFResourceName(tplName) {
		f.SetErrorf("invalid imported template name: %s", tplName)
		return
	}
	if !finiteNumbers(scaleX, scaleY, tX, tY) || scaleX == 0 || scaleY == 0 {
		f.SetErrorf("invalid imported template placement")
		return
	}
	content := []byte(sprintf("q 0 J 1 w 0 j 0 G 0 g q %.4F 0 0 %.4F %.4F %.4F cm %s Do Q Q", scaleX*f.k, scaleY*f.k, tX*f.k, (tY+f.h)*f.k, tplName))
	f.outTaggedContent(content, taggedContentOptions{Artifact: true})
}

// ImportTemplates imports external template names for inclusion in the ProcSet
// dictionary.
func (f *Document) ImportTemplates(tpls map[string]string) {
	for tplName := range tpls {
		if !validPDFResourceName(tplName) {
			f.SetErrorf("invalid imported template name: %s", tplName)
			return
		}
	}
	f.ensureResourceStore().addImportedTemplates(tpls)
}
