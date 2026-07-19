// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"context"
	"fmt"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
	"github.com/cssbruno/gopdfkit/internal/papercompile"
	"github.com/cssbruno/gopdfkit/layout"
)

const (
	typedMetadataGridGapPoints  = 8.0
	typedSignatureGridGapPoints = 8.0
	typedGridMaxColumns         = 64
)

func (f *Document) measurePaperGridRow(ctx context.Context, doc *layout.LayoutDocument, mapping papercompile.CompileMapping,
	block paperPlanningBlock, contentWidth layoutengine.Fixed, left, top, right, bottom float64, nextNode *layoutengine.NodeID, fallback *int) (paperMeasuredGridRow, error) {
	if block.gridRow == nil || len(block.gridRow.cells) == 0 {
		return paperMeasuredGridRow{}, fmt.Errorf("%s: grid row has no cells", block.path)
	}
	count := len(block.gridRow.cells)
	trackCount := block.gridRow.columnCount
	if trackCount < count {
		trackCount = count
	}
	gap, err := layoutengine.FixedFromPoints(block.gridRow.gapPoints)
	if block.gridRow.gapInDocumentUnits {
		gap, err = fixedFromDocumentUnits(f, block.gridRow.gapPoints)
	}
	if err != nil || gap < 0 {
		return paperMeasuredGridRow{}, fmt.Errorf("%s: grid gap is invalid", block.path)
	}
	gapTotal := layoutengine.Fixed(0)
	for index := 1; index < trackCount; index++ {
		gapTotal, err = gapTotal.Add(gap)
		if err != nil {
			return paperMeasuredGridRow{}, fmt.Errorf("%s: grid gap total overflows", block.path)
		}
	}
	available, err := contentWidth.Sub(gapTotal)
	if err != nil || available <= 0 {
		return paperMeasuredGridRow{}, fmt.Errorf("%s: grid gaps leave no column width", block.path)
	}
	widths := make([]layoutengine.Fixed, trackCount)
	var explicit layoutengine.Fixed
	auto := trackCount - count
	for index, cell := range block.gridRow.cells {
		if cell.requestedWidth == 0 {
			auto++
			continue
		}
		width, widthErr := layoutengine.FixedFromPoints(f.UnitToPointConvert(cell.requestedWidth))
		if widthErr != nil || width <= 0 {
			return paperMeasuredGridRow{}, fmt.Errorf("%s: requested width is invalid", cell.path)
		}
		widths[index] = width
		explicit, err = explicit.Add(width)
		if err != nil {
			return paperMeasuredGridRow{}, fmt.Errorf("%s: requested widths overflow", block.path)
		}
	}
	remaining, err := available.Sub(explicit)
	if err != nil || remaining < 0 {
		return paperMeasuredGridRow{}, fmt.Errorf("%s: requested column widths plus gaps exceed the body width", block.path)
	}
	if auto > 0 {
		share := layoutengine.Fixed(int64(remaining) / int64(auto))
		if share <= 0 {
			return paperMeasuredGridRow{}, fmt.Errorf("%s: resolved automatic column width is empty", block.path)
		}
		leftover := layoutengine.Fixed(int64(remaining) - int64(share)*int64(auto))
		for index := range widths {
			if widths[index] != 0 {
				continue
			}
			widths[index] = share
			if leftover > 0 {
				widths[index]++
				leftover--
			}
		}
	}

	row := paperMeasuredGridRow{cells: make([]paperMeasuredGridCell, count), trackWidths: append([]layoutengine.Fixed(nil), widths...), gap: gap}
	if block.gridRow.minimumHeightPoints > 0 {
		row.height, err = layoutengine.FixedFromPoints(block.gridRow.minimumHeightPoints)
		if block.gridRow.minimumHeightInDocumentUnits {
			row.height, err = fixedFromDocumentUnits(f, block.gridRow.minimumHeightPoints)
		}
		if err != nil || row.height <= 0 {
			return paperMeasuredGridRow{}, fmt.Errorf("%s: minimum row height is invalid", block.path)
		}
	}
	if block.gridRow.lineOffsetPoints > 0 {
		row.lineOffset, err = layoutengine.FixedFromPoints(block.gridRow.lineOffsetPoints)
		if block.gridRow.lineOffsetInDocumentUnits {
			row.lineOffset, err = fixedFromDocumentUnits(f, block.gridRow.lineOffsetPoints)
		}
		if err != nil || row.lineOffset <= 0 {
			return paperMeasuredGridRow{}, fmt.Errorf("%s: signature line offset is invalid", block.path)
		}
	}
	var offset layoutengine.Fixed
	for index, cell := range block.gridRow.cells {
		if err := ctx.Err(); err != nil {
			return paperMeasuredGridRow{}, err
		}
		var measurement paperRowColumnMeasurement
		var image *paperMeasuredImage
		if cell.image != nil {
			measuredImage, measureErr := f.measureTypedPlanningImageContext(ctx, *cell.image, f.PointConvert(widths[index].Points()))
			if measureErr != nil {
				return paperMeasuredGridRow{}, fmt.Errorf("%s: %w", cell.path, measureErr)
			}
			if measuredImage.flowWidth() > widths[index] {
				return paperMeasuredGridRow{}, fmt.Errorf("%s: resolved image exceeds its grid track", cell.path)
			}
			image = &measuredImage
		} else if !cell.artifactOnly {
			var measureErr error
			measurement, measureErr = f.measurePaperRowColumnChild(ctx, doc, cell.paragraph, left, top, right, bottom, widths[index])
			if measureErr != nil {
				return paperMeasuredGridRow{}, fmt.Errorf("%s: %w", cell.path, measureErr)
			}
			if cell.compactLineHeight > 0 {
				lineHeight, lineErr := layoutengine.FixedFromPoints(cell.compactLineHeight)
				if cell.compactLineHeightInDocumentUnits {
					lineHeight, lineErr = fixedFromDocumentUnits(f, cell.compactLineHeight)
				}
				if lineErr != nil || lineHeight <= 0 {
					return paperMeasuredGridRow{}, fmt.Errorf("%s: compact line height is invalid", cell.path)
				}
				for lineIndex := range measurement.plan.Lines {
					y, yErr := measurement.body.Y.Add(layoutengine.Fixed(int64(lineHeight) * int64(lineIndex)))
					if yErr != nil {
						return paperMeasuredGridRow{}, fmt.Errorf("%s: compact line position overflows", cell.path)
					}
					line := &measurement.plan.Lines[lineIndex]
					line.Bounds.Y, line.Bounds.Height = y, lineHeight
					line.Baseline, yErr = y.Add(layoutengine.Fixed(int64(lineHeight) * 4 / 5))
					if yErr != nil {
						return paperMeasuredGridRow{}, fmt.Errorf("%s: compact baseline overflows", cell.path)
					}
				}
				measurement.height = layoutengine.Fixed(int64(lineHeight) * int64(len(measurement.plan.Lines)))
			}
		}
		*fallback = *fallback + 1
		identity := paperBlockIdentity(mapping, block.bodyIndex, cell.segmentIndex, index, *fallback)
		measurement.identity = identity
		*nextNode = *nextNode + 1
		topInset, insetErr := layoutengine.FixedFromPoints(cell.topInsetPoints)
		if cell.topInsetInDocumentUnits {
			topInset, insetErr = fixedFromDocumentUnits(f, cell.topInsetPoints)
		}
		if insetErr != nil || topInset < 0 {
			return paperMeasuredGridRow{}, fmt.Errorf("%s: top inset is invalid", cell.path)
		}
		row.cells[index] = paperMeasuredGridCell{measurement: measurement, image: image, node: *nextNode,
			semanticText: cell.semanticText, semanticRole: cell.semanticRole,
			segments: append([]layout.TextSegment(nil), cell.paragraph.Segments...),
			offsetX:  offset, width: widths[index], topInset: topInset, artifactOnly: cell.artifactOnly}
		cellHeight := measurement.height
		if image != nil {
			cellHeight = image.flowHeight()
		}
		cellHeight, err = cellHeight.Add(topInset)
		if err != nil {
			return paperMeasuredGridRow{}, fmt.Errorf("%s: cell height overflows", cell.path)
		}
		if cellHeight > row.height {
			row.height = cellHeight
		}
		offset, err = offset.Add(widths[index])
		if err == nil && index+1 < count {
			offset, err = offset.Add(gap)
		}
		if err != nil {
			return paperMeasuredGridRow{}, fmt.Errorf("%s: column offset overflows", block.path)
		}
	}
	if row.height <= 0 {
		return paperMeasuredGridRow{}, fmt.Errorf("%s: resolved row height is empty", block.path)
	}
	return row, nil
}
