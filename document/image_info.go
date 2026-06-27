// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

// GetImageInfo returns information about the registered image specified by
// imageStr. If the image has not been registered, nil is returned. The
// internal error is not modified by this method.
func (f *Document) GetImageInfo(imageStr string) (info *ImageInfo) {
	info = f.images[imageStr]
	if info == nil {
		return nil
	}
	return info.clone()
}

func (f *Document) newImageInfo() *ImageInfo {
	return &ImageInfo{scale: f.k, dpi: 72}
}
