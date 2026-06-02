/****************************************************************************
 * Software: GoPDFKit                                                         *
 * License:  MIT License                                                    *
 *                                                                          *
 * Copyright (c) 2026 cssBruno                                              *
 ****************************************************************************/

package gopdfkit

// GetImageInfo returns information about the registered image specified by
// imageStr. If the image has not been registered, nil is returned. The
// internal error is not modified by this method.

func (f *Fpdf) GetImageInfo(imageStr string) (info *ImageInfo) {
	return f.images[imageStr]
}

func bufEqual(buf []byte, str string) bool {
	return string(buf[0:len(str)]) == str
}

func be16(buf []byte) int {
	return 256*int(buf[0]) + int(buf[1])
}

func (f *Fpdf) newImageInfo() *ImageInfo {
	return &ImageInfo{scale: f.k, dpi: 72}
}
