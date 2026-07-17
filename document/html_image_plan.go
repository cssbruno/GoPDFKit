// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
)

// resolveCompiledHTMLImageSources snapshots every non-data image before
// lowering. The detached CompiledHTML clone owns the bytes, so planning and
// later painting never reopen a path or consult ambient state.
func (f *Document) resolveCompiledHTMLImageSources(ctx context.Context, compiled *CompiledHTML) (*CompiledHTML, error) {
	if compiled == nil {
		return nil, errors.New("document: compiled HTML is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if f == nil {
		return nil, errors.New("document: nil image planning document")
	}
	clone := *compiled
	clone.dataImages = make(map[int]compiledHTMLDataImage, len(compiled.dataImages))
	limit := f.imageSourceLimit()
	if limit <= 0 {
		return nil, errors.New("document: image source limit is invalid")
	}
	unique := make(map[string][]byte)
	loadedByName := make(map[string]compiledHTMLDataImage)
	totalBytes := 0
	retain := func(index int, source compiledHTMLDataImage) error {
		name := compiledHTMLDataImageName(source.data)
		data, exists := unique[name]
		if !exists {
			if len(unique) >= maxPlannedImageResources {
				return fmt.Errorf("document: planned image resource count exceeds limit: %d", maxPlannedImageResources)
			}
			if len(source.data) > limit-totalBytes {
				return ErrImageTooLarge
			}
			data = append([]byte(nil), source.data...)
			unique[name] = data
			totalBytes += len(data)
		}
		clone.dataImages[index] = compiledHTMLDataImage{name: name, options: source.options, data: data}
		return nil
	}
	for index, token := range clone.tokens {
		if index&127 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		if token.Cat != 'O' || token.Str != "img" {
			continue
		}
		if source, exists := compiled.dataImages[index]; exists {
			if err := retain(index, source); err != nil {
				return nil, htmlPlanUnsupported("img", index, "retain bounded image source: "+err.Error())
			}
		}
		if err := htmlPreflightUnifiedImage(token, index, clone.unifiedResolved[index].decl); err != nil {
			return nil, err
		}
		src := strings.TrimSpace(token.Attr["src"])
		if src == "" {
			continue
		}
		if _, exists := clone.dataImages[index]; exists {
			continue
		}
		if err := validateHTMLImageSource(src); err != nil {
			return nil, htmlPlanUnsupported("img", index, err.Error())
		}
		if parsed, err := url.Parse(src); err != nil || parsed.Scheme != "" {
			return nil, htmlPlanUnsupported("img", index, "non-file resource names must not use a URI scheme")
		}
		if f == nil || !f.securityPolicySet || !f.securityPolicy.AllowLocalHTMLImages {
			return nil, fmt.Errorf("%w: local HTML images require an explicit allowing security policy", ErrSecurityPolicyDenied)
		}
		if cached, exists := loadedByName[src]; exists {
			if err := retain(index, cached); err != nil {
				return nil, htmlPlanUnsupported("img", index, "retain bounded image source: "+err.Error())
			}
			continue
		}
		data, format, err := f.readHTMLPlanningImage(ctx, src)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, err
			}
			return nil, htmlPlanUnsupported("img", index, "load bounded local/catalog image: "+err.Error())
		}
		loaded := compiledHTMLDataImage{
			name: compiledHTMLDataImageName(data), options: ImageOptions{ImageType: format, ReadDpi: true}, data: data,
		}
		loadedByName[src] = loaded
		if err := retain(index, loaded); err != nil {
			return nil, htmlPlanUnsupported("img", index, "retain bounded image source: "+err.Error())
		}
	}
	return &clone, nil
}

func htmlPreflightUnifiedImage(token HTMLSegmentType, index int, decl map[string]string) error {
	for name := range token.Attr {
		switch name {
		case "src", "alt", "width", "height", "align":
		default:
			return htmlPlanUnsupported("img", index, fmt.Sprintf("attribute %q is outside the unified image cohort", name))
		}
	}
	for _, entry := range []struct {
		name, value    string
		percentageBase float64
	}{
		{"width", firstNonEmpty(decl["width"], token.Attr["width"]), 1},
		{"height", firstNonEmpty(decl["height"], token.Attr["height"]), 0},
		{"max-width", decl["max-width"], 1}, {"max-height", decl["max-height"], 0},
	} {
		if _, _, err := htmlPlanImageDimension(entry.value, func(value float64) float64 { return value }, entry.percentageBase); err != nil {
			return htmlPlanUnsupported("img", index, entry.name+" "+err.Error())
		}
	}
	return nil
}

func (f *Document) readHTMLPlanningImage(ctx context.Context, name string) ([]byte, string, error) {
	loader := f.resourceLoader
	if loader == nil {
		loader = FileResourceLoader{}
	}
	reader, info, err := loader.OpenResource(ctx, ResourceImage, name)
	if err != nil {
		return nil, "", err
	}
	if reader == nil {
		return nil, "", errors.New("resource loader returned nil reader")
	}
	defer func() { _ = reader.Close() }()
	limit := f.imageSourceLimit()
	if limit <= 0 {
		return nil, "", errors.New("image source limit is invalid")
	}
	if info.Size > int64(limit) {
		return nil, "", ErrImageTooLarge
	}
	data := make([]byte, 0, min(limit, 64<<10))
	buffer := make([]byte, 64<<10)
	for {
		if err := ctx.Err(); err != nil {
			return nil, "", err
		}
		count, readErr := reader.Read(buffer)
		if count > 0 {
			if count > limit-len(data) {
				return nil, "", ErrImageTooLarge
			}
			data = append(data, buffer[:count]...)
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return nil, "", readErr
		}
		if count == 0 {
			return nil, "", io.ErrNoProgress
		}
	}
	format := htmlPlanningImageFormat(name, data)
	if format == "" {
		return nil, "", errors.New("only PNG and JPEG sources are supported")
	}
	return data, format, nil
}

func htmlPlanningImageFormat(name string, data []byte) string {
	extension := strings.ToLower(filepath.Ext(name))
	if extension == ".png" && bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")) {
		return "png"
	}
	if (extension == ".jpg" || extension == ".jpeg") && len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff {
		return "jpg"
	}
	if bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")) {
		return "png"
	}
	if len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff {
		return "jpg"
	}
	return ""
}

func htmlPlanImageIntrinsicSize(data []byte, pointsToUnits func(float64) float64) (float64, float64, error) {
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width <= 0 || config.Height <= 0 {
		return 0, 0, errors.New("decoded image has invalid intrinsic dimensions")
	}
	width := pointsToUnits(float64(config.Width) * 72 / 96)
	height := pointsToUnits(float64(config.Height) * 72 / 96)
	if !finiteNumbers(width, height) || width <= 0 || height <= 0 {
		return 0, 0, errors.New("intrinsic CSS image dimensions are not representable")
	}
	return width, height, nil
}

func htmlPlanImageDimension(value string, pointsToUnits func(float64) float64, percentageBase float64) (float64, bool, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "auto") || strings.EqualFold(value, "none") {
		return 0, false, nil
	}
	if strings.HasSuffix(value, "%") {
		percent, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(value, "%")), 64)
		if err != nil || math.IsNaN(percent) || math.IsInf(percent, 0) || percent <= 0 || percent > 1000 {
			return 0, false, errors.New("must be a finite positive percentage within the bounded range")
		}
		if percentageBase <= 0 || !finiteNumbers(percentageBase) {
			return 0, false, errors.New("percentage requires a definite containing width")
		}
		result := percentageBase * percent / 100
		if result <= 0 || !finiteNumbers(result) {
			return 0, false, errors.New("percentage is not representable in document units")
		}
		return result, true, nil
	}
	if strings.Contains(value, "%") {
		return 0, false, errors.New("contains a malformed percentage")
	}
	points, ok := parseHTMLBoxLength(value, nil, 0)
	if !ok || points <= 0 || math.IsNaN(points) || math.IsInf(points, 0) {
		return 0, false, errors.New("must be a finite positive absolute CSS length")
	}
	result := pointsToUnits(points)
	if math.IsNaN(result) || math.IsInf(result, 0) || result <= 0 {
		return 0, false, errors.New("is not representable in document units")
	}
	return result, true, nil
}
