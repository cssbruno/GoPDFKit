// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

// GetImageInfo returns information about the registered image specified by
// imageStr. If the image has not been registered, nil is returned. The
// internal error is not modified by this method.
func (f *Document) GetImageInfo(imageStr string) (info *ImageInfo) {
	return f.images[imageStr]
}

func (f *Document) newImageInfo() *ImageInfo {
	return &ImageInfo{scale: f.k, dpi: 72}
}
