// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"context"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
	"github.com/cssbruno/gopdfkit/layout"
)

const typedQRPayloadByteLimit = 4096

// paperExpandQRVerification lowers a typed QR block into immutable image and
// text children with the same left/center/right geometry as the legacy
// renderer. Left and right are one exact grid row; center stacks the text two
// document units below the image.
func paperExpandQRVerification(
	ctx context.Context,
	expanded *[]paperPlanningBlock,
	block layout.QRVerificationBlock,
	bodyIndex, segmentIndex, nestedIndex int,
	path string,
	nextGroup *uint32,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	policy, visualBox := paperContainerBoxPolicy(block.EffectiveBox())
	if policy.applyOrphans || policy.applyWidows {
		return fmt.Errorf("%s: widow/orphan policy applies only to QR verification text", path)
	}
	policy.keepTogether = policy.keepTogether || block.QR.KeepTogether

	payload := strings.TrimSpace(block.QR.URL)
	if payload == "" {
		payload = strings.TrimSpace(block.QR.Value)
	}
	if payload == "" {
		return fmt.Errorf("%s.qr: value or URL is required", path)
	}
	if !utf8.ValidString(payload) {
		return fmt.Errorf("%s.qr: payload is not valid UTF-8", path)
	}
	if len(payload) > typedQRPayloadByteLimit {
		return fmt.Errorf("%s.qr: payload exceeds %d-byte limit", path, typedQRPayloadByteLimit)
	}
	size := block.QR.Size
	if size == 0 {
		size = 25
	}
	if math.IsNaN(size) || math.IsInf(size, 0) || size <= 0 {
		return fmt.Errorf("%s.qr.size: must be finite and positive", path)
	}
	align, err := typedQRAlign(block.QR.Align)
	if err != nil {
		return fmt.Errorf("%s.qr.align: %w", path, err)
	}
	pngData, err := QRCodePNG(payload, defaultQRCodeSizePx)
	if err != nil {
		return fmt.Errorf("%s.qr: %w", path, err)
	}

	start := len(*expanded)
	image := layout.ImageBlock{
		Data: pngData, Format: "png", Width: size, Height: size,
		Fit: layout.ImageFitContain, Align: align,
		Alt: typedQRAlternateText(block.QR),
	}
	segments := typedQRTextSegments(block)
	style := block.EffectiveStyle()
	if style.Align == "" {
		style.Align = "L"
	}
	paragraph := layout.ParagraphBlock{Segments: segments, Style: style}
	if align == "center" {
		image.Box.Padding.Bottom = 2
		if err := paperExpandPlanningBlock(ctx, expanded, image, bodyIndex, segmentIndex, nestedIndex, path+".qr", nextGroup); err != nil {
			return err
		}
		if len(segments) != 0 {
			if err := paperExpandPlanningBlock(ctx, expanded, paragraph, bodyIndex, segmentIndex, nestedIndex, path+".text", nextGroup); err != nil {
				return err
			}
		}
	} else {
		image.Align = "left"
		imageCell := paperPlanningGridCell{image: &image, path: path + ".qr", semanticText: image.Alt,
			semanticRole: layoutengine.SemanticRoleFigure, requestedWidth: size, segmentIndex: segmentIndex}
		textCell := paperPlanningGridCell{paragraph: paragraph, path: path + ".text",
			semanticText: layout.TextSegmentsPlainText(segments), semanticRole: layoutengine.SemanticRoleParagraph,
			segmentIndex: segmentIndex}
		cells := []paperPlanningGridCell{imageCell, textCell}
		if align == "right" {
			cells[0], cells[1] = cells[1], cells[0]
		}
		row := paperPlanningGridRow{cells: cells, columnCount: 2, gapPoints: 4, gapInDocumentUnits: true}
		*expanded = append(*expanded, paperPlanningBlock{bodyIndex: bodyIndex, segmentIndex: segmentIndex,
			nestedIndex: nestedIndex, path: path, gridRow: &row, keepTogether: true})
	}
	if err := paperApplyContainerBox(expanded, start, visualBox, path); err != nil {
		return err
	}
	return paperApplyPaginationPolicy(expanded, start, policy, nextGroup, path)
}

func typedQRAlign(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "l", "left":
		return "left", nil
	case "c", "center":
		return "center", nil
	case "r", "right":
		return "right", nil
	default:
		return "", fmt.Errorf("%q is unsupported", value)
	}
}

func typedQRAlternateText(qr layout.QRBlock) string {
	if label := strings.TrimSpace(qr.Label); label != "" {
		return label + " QR code"
	}
	return "Verification QR code"
}

func typedQRTextSegments(block layout.QRVerificationBlock) []layout.TextSegment {
	if len(block.Text) != 0 {
		return append([]layout.TextSegment(nil), block.Text...)
	}
	label := strings.TrimSpace(block.QR.Label)
	if label == "" {
		label = "Verification"
	}
	segments := []layout.TextSegment{{Text: label}}
	if url := strings.TrimSpace(block.QR.URL); url != "" {
		segments = append(segments, layout.TextSegment{Text: "\n" + url, Link: url})
	} else if value := strings.TrimSpace(block.QR.Value); value != "" {
		segments = append(segments, layout.TextSegment{Text: "\n" + value})
	}
	return segments
}
