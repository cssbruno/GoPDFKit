// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"errors"
	"fmt"
	"io"
	"maps"
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
	if f.err != nil {
		return
	}
	if strings.TrimSpace(imgName) == "" {
		f.err = errors.New("image name should not be blank")
		return
	}
	info, ok := f.images[imgName]
	if ok {
		if info == nil {
			f.err = fmt.Errorf("registered image is invalid: %s", imgName)
		}
		return
	}
	if options.ImageType == "" {
		f.err = errors.New("image type should be specified if reading from custom reader")
		return
	}
	imageType := normalizeImageType(options.ImageType)
	switch imageType {
	case "jpg":
		info = f.parsejpg(r)
	case "png":
		info = f.parsepng(r, options.ReadDpi)
	case "gif":
		info = f.parsegif(r)
	case "webp":
		info = f.parsewebp(r)
	default:
		f.err = fmt.Errorf("unsupported image type: %s", imageType)
	}
	if f.err != nil {
		return
	}
	if info == nil {
		f.err = errors.New("image parser returned no image info")
		return
	}
	if info.i, f.err = generateImageID(info); f.err != nil {
		return
	}
	f.images[imgName] = info
	return
}

// RegisterImageOptions registers an image, adding it to the PDF file but not
// adding it to the page. File-backed images are cached across documents by
// path, stat metadata, image type, and DPI options. Use ImageOptions() with the
// same filename to add the image to the page. ImageOptions() calls this
// function, so this function is only necessary if you need information about
// the image before placing it. See ImageOptions() for restrictions on the image
// and options parameters.
func (f *Document) RegisterImageOptions(fileStr string, options ImageOptions) (info *ImageInfo) {
	info, ok := f.images[fileStr]
	if ok {
		return
	}
	info, err := sharedImageFileCache.RegisterImageOptions(fileStr, fileStr, options)
	if err != nil {
		f.err = err
		return nil
	}
	return f.registerCachedImageInfo(fileStr, info)
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
	for name, data := range objs {
		f.importedObjs[name] = append([]byte(nil), data...)
	}
}

// ImportObjPos imports external template object hash positions.
func (f *Document) ImportObjPos(objPos map[string]map[int]string) {
	for name, positions := range objPos {
		copied := make(map[int]string, len(positions))
		maps.Copy(copied, positions)
		f.importedObjPos[name] = copied
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
	maps.Copy(f.importedTplObjs, tpls)
}
