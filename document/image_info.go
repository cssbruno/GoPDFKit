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

type imageParser struct {
	k             float64
	compressLevel int
	pdfVersion    string
	sourceLimit   int
	decodedLimit  int
	err           error
}

func newImageParser(scale float64, compressLevel int, pdfVersion string) *imageParser {
	return newImageParserWithLimits(scale, compressLevel, pdfVersion, maxImageSourceBytes, maxImageDecodedBytes)
}

func newImageParserWithLimits(scale float64, compressLevel int, pdfVersion string, sourceLimit, decodedLimit int) *imageParser {
	if sourceLimit == 0 {
		sourceLimit = maxImageSourceBytes
	}
	if decodedLimit == 0 {
		decodedLimit = maxImageDecodedBytes
	}
	return &imageParser{k: scale, compressLevel: compressLevel, pdfVersion: pdfVersion, sourceLimit: sourceLimit, decodedLimit: decodedLimit}
}

func (p *imageParser) newImageInfo() *ImageInfo {
	return &ImageInfo{scale: p.k, dpi: 72}
}

func (p *imageParser) compressBytes(data []byte) []byte {
	level := p.compressLevel
	if !validCompressionLevel(level) {
		level = defaultCompressionLevel()
	}
	out, err := sliceCompressLevel(data, level)
	if err != nil {
		p.err = err
		return nil
	}
	return out
}
