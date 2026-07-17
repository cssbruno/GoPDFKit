// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
)

type coreFontMetricsDigestInput struct {
	SchemaVersion      uint8                     `json:"schema_version"`
	Face               layoutengine.CoreFontFace `json:"face"`
	BaseFont           string                    `json:"base_font"`
	Encoding           string                    `json:"encoding"`
	UnderlinePosition  int                       `json:"underline_position"`
	UnderlineThickness int                       `json:"underline_thickness"`
	Widths             []int                     `json:"widths"`
}

type embeddedFontMetricsDigestInput struct {
	SchemaVersion      uint8       `json:"schema_version"`
	Name               string      `json:"name"`
	Ascent             int         `json:"ascent"`
	Descent            int         `json:"descent"`
	CapHeight          int         `json:"cap_height"`
	Flags              int         `json:"flags"`
	FontBBox           fontBoxType `json:"font_bbox"`
	ItalicAngle        int         `json:"italic_angle"`
	StemV              int         `json:"stem_v"`
	MissingWidth       int         `json:"missing_width"`
	UnderlinePosition  int         `json:"underline_position"`
	UnderlineThickness int         `json:"underline_thickness"`
	Widths             []int       `json:"widths"`
}

func typedEmbeddedUTF8FontResource(font fontDefinition) (layoutengine.CoreFontResource, []byte, error) {
	if font.Tp != "UTF8" || font.utf8File == nil || font.utf8File.fileReader == nil || len(font.utf8File.fileReader.array) == 0 {
		return layoutengine.CoreFontResource{}, nil, errors.New("resolved font is not an embedded UTF-8 TrueType font")
	}
	data := append([]byte(nil), font.utf8File.fileReader.array...)
	if uint64(len(data)) > uint64(^uint32(0)) {
		return layoutengine.CoreFontResource{}, nil, errors.New("embedded UTF-8 font exceeds plan length capacity")
	}
	encoded, err := json.Marshal(embeddedFontMetricsDigestInput{
		SchemaVersion: 1, Name: font.Name, Ascent: font.Desc.Ascent, Descent: font.Desc.Descent,
		CapHeight: font.Desc.CapHeight, Flags: font.Desc.Flags, FontBBox: font.Desc.FontBBox,
		ItalicAngle: font.Desc.ItalicAngle, StemV: font.Desc.StemV, MissingWidth: font.Desc.MissingWidth,
		UnderlinePosition: font.Up, UnderlineThickness: font.Ut, Widths: append([]int(nil), font.Cw...),
	})
	if err != nil {
		return layoutengine.CoreFontResource{}, nil, fmt.Errorf("encode embedded UTF-8 font metrics: %w", err)
	}
	metricsDigest := sha256.Sum256(encoded)
	contentDigest := sha256.Sum256(data)
	return layoutengine.CoreFontResource{
		ID: 1, MetricsDigest: layoutengine.CoreFontMetricsDigest(hex.EncodeToString(metricsDigest[:])),
		EmbeddedUTF8: &layoutengine.EmbeddedUTF8Font{Name: font.Name,
			Digest: layoutengine.CoreFontMetricsDigest(hex.EncodeToString(contentDigest[:])), ByteLength: uint32(len(data))},
	}, data, nil
}

func (f *Document) typedLayoutFontSourcesContext(ctx context.Context, plan layoutengine.LayoutPlan) (plannedFontSources, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	wanted := make(map[layoutengine.CoreFontMetricsDigest]layoutengine.EmbeddedUTF8Font)
	for _, resource := range plan.Projection().Fonts {
		if resource.EmbeddedUTF8 != nil {
			wanted[resource.EmbeddedUTF8.Digest] = *resource.EmbeddedUTF8
		}
	}
	if len(wanted) == 0 {
		return nil, nil
	}
	if f == nil || f.resources == nil {
		return nil, errors.New("embedded UTF-8 plan has no source document font catalog")
	}
	sources := make(plannedFontSources, len(wanted))
	var total uint64
	for index, font := range f.resources.fontsByKey(true) {
		if index&7 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		resource, data, err := typedEmbeddedUTF8FontResource(font)
		if err != nil {
			continue
		}
		descriptor, needed := wanted[resource.EmbeddedUTF8.Digest]
		if !needed || resource.MetricsDigest == "" {
			continue
		}
		if _, exists := sources[descriptor.Digest]; exists {
			continue
		}
		if uint32(len(data)) != descriptor.ByteLength || total+uint64(len(data)) > uint64(maxFontSourceBytes) {
			return nil, errors.New("embedded UTF-8 plan font source limit exceeded")
		}
		total += uint64(len(data))
		sources[descriptor.Digest] = append([]byte(nil), data...)
	}
	if len(sources) != len(wanted) {
		return nil, errors.New("embedded UTF-8 plan font bytes are unavailable")
	}
	return sources, nil
}

func typedCoreFontResource(font fontDefinition) (layoutengine.CoreFontResource, error) {
	face, encoding, ok := typedCoreFontFace(font)
	if !ok || font.Tp != "Core" || font.utf8File != nil {
		return layoutengine.CoreFontResource{}, errors.New("resolved font is not a canonical PDF core font")
	}
	if len(font.Cw) != 256 {
		return layoutengine.CoreFontResource{}, fmt.Errorf("core font width table has %d entries, want 256", len(font.Cw))
	}
	widths := append([]int(nil), font.Cw...)
	for index, width := range widths {
		if width < 0 {
			return layoutengine.CoreFontResource{}, fmt.Errorf("core font width %d is negative", index)
		}
	}
	encoded, err := json.Marshal(coreFontMetricsDigestInput{
		SchemaVersion: 1, Face: face, BaseFont: font.Name, Encoding: encoding,
		UnderlinePosition: font.Up, UnderlineThickness: font.Ut, Widths: widths,
	})
	if err != nil {
		return layoutengine.CoreFontResource{}, fmt.Errorf("encode core font metrics: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return layoutengine.CoreFontResource{
		ID: 1, Face: face, MetricsDigest: layoutengine.CoreFontMetricsDigest(hex.EncodeToString(digest[:])),
	}, nil
}

func typedCoreFontFace(font fontDefinition) (layoutengine.CoreFontFace, string, bool) {
	const winANSI = "win_ansi"
	switch font.Name {
	case "Courier":
		return layoutengine.CoreFontCourier, winANSI, true
	case "Courier-Bold":
		return layoutengine.CoreFontCourierBold, winANSI, true
	case "Courier-Oblique":
		return layoutengine.CoreFontCourierOblique, winANSI, true
	case "Courier-BoldOblique":
		return layoutengine.CoreFontCourierBoldOblique, winANSI, true
	case "Helvetica":
		return layoutengine.CoreFontHelvetica, winANSI, true
	case "Helvetica-Bold":
		return layoutengine.CoreFontHelveticaBold, winANSI, true
	case "Helvetica-Oblique":
		return layoutengine.CoreFontHelveticaOblique, winANSI, true
	case "Helvetica-BoldOblique":
		return layoutengine.CoreFontHelveticaBoldOblique, winANSI, true
	case "Times-Roman":
		return layoutengine.CoreFontTimesRoman, winANSI, true
	case "Times-Bold":
		return layoutengine.CoreFontTimesBold, winANSI, true
	case "Times-Italic":
		return layoutengine.CoreFontTimesItalic, winANSI, true
	case "Times-BoldItalic":
		return layoutengine.CoreFontTimesBoldItalic, winANSI, true
	case "Symbol":
		return layoutengine.CoreFontSymbol, "builtin", true
	case "ZapfDingbats":
		return layoutengine.CoreFontZapfDingbats, "builtin", true
	default:
		return "", "", false
	}
}

// typedCoreGlyphAdvances converts cumulative legacy layout positions to Fixed
// before differencing them. This preserves every new-plan cursor and an exact
// final sum. It deliberately does not inherit the legacy PDF serializer's
// two-decimal Tf/Td quantization; fixed command geometry is the new painter's
// normative input.
func typedCoreGlyphAdvances(pdf *Document, codes string, width layoutengine.Fixed) ([]layoutengine.Fixed, error) {
	if pdf == nil || len(codes) == 0 {
		return nil, errors.New("core glyph run is empty")
	}
	advances := make([]layoutengine.Fixed, len(codes))
	var cumulativeUser float64
	var previous layoutengine.Fixed
	for index := range []byte(codes) {
		code := codes[index]
		cumulativeUser += float64(pdf.currentFontRuneWidth(rune(code))) * pdf.fontSize / 1000
		if code == ' ' {
			cumulativeUser += pdf.ws
		}
		current, err := layoutengine.FixedFromPoints(pdf.UnitToPointConvert(cumulativeUser))
		if err != nil {
			return nil, fmt.Errorf("glyph %d cumulative advance: %w", index, err)
		}
		advance, err := current.Sub(previous)
		if err != nil || advance < 0 {
			return nil, fmt.Errorf("glyph %d has an invalid fixed advance", index)
		}
		advances[index] = advance
		previous = current
	}
	if previous != width {
		return nil, fmt.Errorf("glyph advances total %d, want planned line width %d", previous, width)
	}
	return advances, nil
}

func typedUTF8GlyphAdvances(pdf *Document, text string, width layoutengine.Fixed) ([]layoutengine.Fixed, error) {
	if pdf == nil || text == "" || !pdf.isCurrentUTF8 {
		return nil, errors.New("embedded UTF-8 glyph run is empty or has no active UTF-8 font")
	}
	advances := make([]layoutengine.Fixed, 0, utf8.RuneCountInString(text))
	var cumulativeUser float64
	var previous layoutengine.Fixed
	for index, character := range []rune(text) {
		if character < 0x20 || character > 0xffff {
			return nil, fmt.Errorf("glyph %d is outside the supported BMP text contract", index)
		}
		cumulativeUser += float64(pdf.currentFontRuneWidth(character)) * pdf.fontSize / 1000
		if character == ' ' {
			cumulativeUser += pdf.ws
		}
		current, err := layoutengine.FixedFromPoints(pdf.UnitToPointConvert(cumulativeUser))
		if err != nil {
			return nil, fmt.Errorf("glyph %d cumulative advance: %w", index, err)
		}
		advance, err := current.Sub(previous)
		if err != nil || advance < 0 {
			return nil, fmt.Errorf("glyph %d has an invalid fixed advance", index)
		}
		advances = append(advances, advance)
		previous = current
	}
	if previous != width {
		return nil, fmt.Errorf("glyph advances total %d, want planned line width %d", previous, width)
	}
	return advances, nil
}
