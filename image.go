/****************************************************************************
 * Software: GoPDFKit                                                         *
 * License:  MIT License                                                    *
 *                                                                          *
 * Copyright (c) 2026 cssBruno                                              *
 ****************************************************************************/

package gopdfkit

import (
	"fmt"
	"io"
	"maps"
	"os"
	"strings"
)

// ImageTypeFromMime returns the image type used in various image-related
// functions (for example, ImageOptions()) that is associated with the specified MIME
// type. For example, "jpg" is returned if mimeStr is "image/jpeg". An error is
// set if the specified MIME type is not supported.

func (f *Fpdf) ImageTypeFromMime(mimeStr string) (tp string) {
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

// ImageOptions puts a JPEG, PNG, GIF or WebP image in the current page. The
// size it will take on the page can be specified in different ways. If both w
// and h are 0, the image is rendered at 96 dpi. If either w or h is zero, it will be
// calculated from the other dimension so that the aspect ratio is maintained.
// If w and/or h are -1, the dpi for that dimension will be read from the
// ImageInfo object. PNG files can contain dpi information, and if present,
// this information will be populated in the ImageInfo object and used in
// Width, Height, and Extent calculations. Otherwise, the SetDpi function can
// be used to change the dpi from the default of 72.
//
// If w and h are any other negative value, their absolute values
// indicate their dpi extents.
//
// Supported JPEG formats are 24 bit, 32 bit and gray scale. Supported PNG
// formats are 24 bit, indexed color, and 8 bit indexed gray scale. GIF and
// WebP images are converted to PNG before embedding. If a GIF image is
// animated, only the first frame is rendered. Transparency is supported. It is
// possible to put a link on the image.
//
// imageNameStr may be the name of an image as registered with a call to either
// RegisterImageOptionsReader() or RegisterImageOptions(). In the first case,
// the image is loaded using an io.Reader. This is generally useful when the
// image is obtained from some other means than as a disk-based file. In the
// second case, the image is loaded as a file. Alternatively, imageNameStr may
// directly specify a sufficiently qualified filename.
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

func (f *Fpdf) ImageOptions(imageNameStr string, x, y, w, h float64, flow bool, options ImageOptions, link int, linkStr string) {
	if f.err != nil {
		return
	}
	info := f.RegisterImageOptions(imageNameStr, options)
	if f.err != nil {
		return
	}
	f.imageOut(info, x, y, w, h, options.AllowNegativePosition, flow, link, linkStr)
	return
}

// ImageOptions provides a place to hang any options we want to use while
// parsing an image.
//
// ImageType's possible values are (case insensitive):
// "JPG", "JPEG", "PNG", "GIF" and "WEBP". If empty, the type is inferred from
// the file extension.
//
// ReadDpi defines whether to attempt to automatically read the image
// dpi information from the image file. Normally, this should be set
// to true (understanding that not all images will have this info
// available). However, for backwards compatibility with previous
// versions of the API, it defaults to false.
//
// AllowNegativePosition can be set to true in order to prevent the default
// coercion of negative x values to the current x position.

type ImageOptions struct {
	ImageType             string
	ReadDpi               bool
	AllowNegativePosition bool
}

// RegisterImageOptionsReader registers an image, reading it from Reader r,
// adding it to the PDF file but not adding it to the page. Use ImageOptions()
// with the same name to add the image to the page. Note that ImageType should
// be specified in this case.
//
// See ImageOptions() for restrictions on the image and the options parameters.

func (f *Fpdf) RegisterImageOptionsReader(imgName string, options ImageOptions, r io.Reader) (info *ImageInfo) {
	if f.err != nil {
		return
	}
	info, ok := f.images[imgName]
	if ok {
		return
	}
	if options.ImageType == "" {
		f.err = fmt.Errorf("image type should be specified if reading from custom reader")
		return
	}
	options.ImageType = strings.ToLower(options.ImageType)
	if options.ImageType == "jpeg" {
		options.ImageType = "jpg"
	}
	switch options.ImageType {
	case "jpg":
		info = f.parsejpg(r)
	case "png":
		info = f.parsepng(r, options.ReadDpi)
	case "gif":
		info = f.parsegif(r)
	case "webp":
		info = f.parsewebp(r)
	default:
		f.err = fmt.Errorf("unsupported image type: %s", options.ImageType)
	}
	if f.err != nil {
		return
	}
	if info.i, f.err = generateImageID(info); f.err != nil {
		return
	}
	f.images[imgName] = info
	return
}

// RegisterImageOptions registers an image, adding it to the PDF file but not
// adding it to the page. Use ImageOptions() with the same filename to add the
// image to the page. Note that ImageOptions() calls this function, so this
// function is only necessary if you need information about the image before
// placing it. See ImageOptions() for restrictions on the image and options
// parameters.

func (f *Fpdf) RegisterImageOptions(fileStr string, options ImageOptions) (info *ImageInfo) {
	info, ok := f.images[fileStr]
	if ok {
		return
	}
	file, err := os.Open(fileStr)
	if err != nil {
		f.err = err
		return
	}
	defer file.Close()
	if options.ImageType == "" {
		pos := strings.LastIndex(fileStr, ".")
		if pos < 0 {
			f.err = fmt.Errorf("image file has no extension and no type was specified: %s", fileStr)
			return
		}
		options.ImageType = fileStr[pos+1:]
	}
	return f.RegisterImageOptionsReader(fileStr, options, file)
}

// ImportObjects imports external template objects into the current document.
func (f *Fpdf) ImportObjects(objs map[string][]byte) {
	maps.Copy(f.importedObjs, objs)
}

// ImportObjPos imports external template object hash positions.
func (f *Fpdf) ImportObjPos(objPos map[string]map[int]string) {
	maps.Copy(f.importedObjPos, objPos)
}

// UseImportedTemplate draws an imported PDF template onto the current page.

func (f *Fpdf) UseImportedTemplate(tplName string, scaleX float64, scaleY float64, tX float64, tY float64) {
	if !validPDFResourceName(tplName) {
		f.SetErrorf("invalid imported template name: %s", tplName)
		return
	}
	f.outf("q 0 J 1 w 0 j 0 G 0 g q %.4F 0 0 %.4F %.4F %.4F cm %s Do Q Q\n", scaleX*f.k, scaleY*f.k, tX*f.k, (tY+f.h)*f.k, tplName)
}

// ImportTemplates imports external template names for inclusion in the procset
// dictionary.
func (f *Fpdf) ImportTemplates(tpls map[string]string) {
	for tplName := range tpls {
		if !validPDFResourceName(tplName) {
			f.SetErrorf("invalid imported template name: %s", tplName)
			return
		}
	}
	maps.Copy(f.importedTplObjs, tpls)
}
