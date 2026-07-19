// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
	"github.com/cssbruno/gopdfkit/internal/papercompile"
	"github.com/cssbruno/gopdfkit/internal/paperlang"
	"github.com/cssbruno/gopdfkit/layout"
)

type typedTableCellMeasurement struct {
	node       layoutengine.NodeID
	key        layoutengine.NodeKey
	instance   layoutengine.InstanceID
	row        uint32
	column     uint32
	rowSpan    uint32
	columnSpan uint32
	header     bool
	scope      string
	caption    bool
	vertical   string
	actualText string
	height     layoutengine.Fixed
	blocks     []paperRowColumnMeasurement
	content    []typedTableCellContent
	segments   []layout.TextSegment
	margins    [4]layoutengine.Fixed // top, right, bottom, left
	padding    [4]layoutengine.Fixed // top, right, bottom, left
	background layoutengine.CoreRGBColor
	borders    [4]typedTableBorder
}

type typedTableCellContent struct {
	text       *paperRowColumnMeasurement
	image      *paperMeasuredImage
	nested     *paperRowColumnMeasurement
	role       layoutengine.SemanticRole
	heading    uint8
	alt        string
	actualText string
	segments   []layout.TextSegment
	ancestors  []typedTableContentAncestor
}

// typedTableContentAncestor retains semantic containers that do not paint
// independently. In particular, list and list-item ownership must survive the
// canonical lowering to paragraph/image paint primitives.
type typedTableContentAncestor struct {
	role       layoutengine.SemanticRole
	identity   string
	actualText string
}

type typedTableExpandedBlock struct {
	block     layout.Block
	ancestors []typedTableContentAncestor
}

type typedTableNestedSemantics struct {
	projection layoutengine.LayoutPlanProjection
	fragments  map[layoutengine.FragmentID]layoutengine.FragmentID
	prefix     string
}

type typedTableBorder struct {
	width layoutengine.Fixed
	color layoutengine.CoreRGBColor
}

func (f *Document) planTypedTable(ctx context.Context, doc *layout.LayoutDocument, table layout.TableBlock, path string) (layoutengine.LayoutPlan, error) {
	return f.planTypedTableBodies(ctx, doc, table, path, nil)
}

func (f *Document) planTypedTableBodiesMapped(ctx context.Context, doc *layout.LayoutDocument, table layout.TableBlock, path string, mapping papercompile.CompileMapping, bodyIndex int, selectBody paperBodySelector) (layoutengine.LayoutPlan, error) {
	planned, err := f.planTypedTableBodies(ctx, doc, table, path, selectBody)
	if err != nil {
		return layoutengine.LayoutPlan{}, err
	}
	projection := planned.Projection()
	nodes := projection.SemanticNodes
	for _, mapped := range mapping.Nodes {
		if mapped.ID == "" {
			continue
		}
		var target layoutengine.SemanticNodeID
		switch {
		case mapped.Kind == paperlang.NodeDocument:
			target = 1
		case mapped.Kind == paperlang.NodeTable && mapped.BodyIndex == bodyIndex:
			target = 2
		default:
			continue
		}
		if int(target) > len(nodes) {
			return layoutengine.LayoutPlan{}, fmt.Errorf("document: table source mapping target %d is unavailable", target)
		}
		identity := paperBlockIdentity(mapping, mapped.BodyIndex, mapped.SegmentIndex, mapped.NestedBlockIndex, bodyIndex)
		if mapped.Kind == paperlang.NodeDocument {
			identity.key = layoutengine.NodeKey(mapped.ID)
			identity.instance = layoutengine.InstanceID(mapped.ID)
			if mapped.InstancePath != "" {
				identity.instance = layoutengine.InstanceID(mapped.InstancePath + "/" + mapped.ID)
			}
			identity.source = paperLayoutSourceSpan(mapped.Span)
		}
		nodes[target-1].Key = identity.key
		nodes[target-1].Instance = identity.instance
		nodes[target-1].Source = identity.source
	}
	return layoutengine.ReplaceSemantics(planned, nodes, projection.SemanticFragments, projection.ReadingOrder)
}

func (f *Document) planTypedTableBodies(ctx context.Context, doc *layout.LayoutDocument, table layout.TableBlock, path string, selectBody paperBodySelector) (layoutengine.LayoutPlan, error) {
	table = typedTableWithInferredColumns(table)
	if err := validateTypedTableSurface(table, path); err != nil {
		return layoutengine.LayoutPlan{}, err
	}
	left, top, right, bottom := typedShadowMargins(f, doc.PageTemplate.Margins)
	contentWidth, bodyHeight := f.w-left-right, f.h-top-bottom
	if contentWidth <= 0 || bodyHeight <= 0 {
		return layoutengine.LayoutPlan{}, newTypedShadowUnsupported(typedShadowGeometry, "page margins leave no body area")
	}
	pageSize, body, err := typedShadowFixedGeometry(f, left, top, contentWidth, bodyHeight)
	if err != nil {
		return layoutengine.LayoutPlan{}, newTypedShadowUnsupported(typedShadowGeometry, err.Error())
	}
	columns := make([]layoutengine.TableColumn, len(table.Columns))
	for index, column := range table.Columns {
		minimum, minimumErr := layoutengine.FixedFromPoints(f.UnitToPointConvert(column.MinWidth))
		maximum, maximumErr := layoutengine.FixedFromPoints(f.UnitToPointConvert(column.MaxWidth))
		if column.MinWidthPercent != 0 {
			minimum, minimumErr = typedContainerPercent(body.Width, column.MinWidthPercent)
		}
		if column.MaxWidthPercent != 0 {
			maximum, maximumErr = typedContainerPercent(body.Width, column.MaxWidthPercent)
		}
		if minimumErr != nil || maximumErr != nil {
			return layoutengine.LayoutPlan{}, typedTableUnsupported(fmt.Sprintf("%s.columns[%d]", path, index), "minimum or maximum width is not representable")
		}
		columns[index] = layoutengine.TableColumn{Kind: layoutengine.TableTrackIntrinsic, MinWidth: minimum, MaxWidth: maximum}
		if column.Width > 0 || column.WidthPercent != 0 {
			width, widthErr := layoutengine.FixedFromPoints(f.UnitToPointConvert(column.Width))
			if column.WidthPercent != 0 {
				width, widthErr = typedContainerPercent(body.Width, column.WidthPercent)
			}
			if widthErr != nil || width <= 0 {
				return layoutengine.LayoutPlan{}, typedTableUnsupported(fmt.Sprintf("%s.columns[%d]", path, index), "fixed width must be positive and representable")
			}
			columns[index].Kind = layoutengine.TableTrackFixed
			columns[index].Width = width
		}
	}
	tableWidth := typedTablePlanWidth(body.Width, columns)

	rows := make([]layout.TableRow, 0, len(table.Header)+len(table.Body)+len(table.Footer))
	rows = append(rows, table.Header...)
	rows = append(rows, table.Body...)
	rows = append(rows, table.Footer...)
	placements, err := typedTablePlacements(rows, len(columns), uint32(len(table.Header)), path) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
	if err != nil {
		return layoutengine.LayoutPlan{}, err
	}
	measurements := make([]typedTableCellMeasurement, 0, len(placements)+1)
	var caption *layoutengine.TableCaption
	captionSegments := table.CaptionSegments
	if len(captionSegments) == 0 && strings.TrimSpace(table.Caption) != "" {
		captionSegments = []layout.TextSegment{{Text: strings.TrimSpace(table.Caption)}}
	}
	if len(captionSegments) != 0 {
		placement := typedTablePlacement{
			cell: layout.TableCell{Blocks: []layout.Block{layout.ParagraphBlock{
				Segments: append([]layout.TextSegment(nil), captionSegments...),
				Style:    layout.TextStyle{LineHeight: 12, Bold: true},
			}}},
			path: path + ".caption", row: 0, column: 0, rowSpan: 1,
			columnSpan: uint32(len(columns)), node: 1, // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
		}
		measurement, measureErr := f.measureTypedTableCell(ctx, doc, placement, tableWidth, layout.DocumentColor{})
		if measureErr != nil {
			return layoutengine.LayoutPlan{}, measureErr
		}
		measurement.caption = true
		measurement.key = "@typed-table-caption"
		measurement.instance = "@typed-table-caption"
		measurements = append(measurements, measurement)
		caption = &layoutengine.TableCaption{Node: measurement.node, Key: measurement.key, Instance: measurement.instance, MinHeight: measurement.height}
		for index := range placements {
			placements[index].node++
		}
	}
	engineCells := make([]layoutengine.TableCell, len(placements))
	// Intrinsic sizing needs only core-font advances. Constructing a complete
	// scratch PDF document for every cell dominated large-table allocation
	// volume, so identical resolved styles share one plan-local read-only metric
	// context. The cache is deliberately local: callers may plan concurrently
	// with different font configuration without synchronization or aliasing.
	intrinsicScratch := make(map[layout.TextStyle]*Document)
	for index, placement := range placements {
		minimum, preferred, intrinsicErr := f.measureTypedTableCellIntrinsic(ctx, doc, placement, intrinsicScratch)
		if intrinsicErr != nil {
			return layoutengine.LayoutPlan{}, intrinsicErr
		}
		engineCells[index] = layoutengine.TableCell{
			Node: placement.node, Key: layoutengine.NodeKey(fmt.Sprintf("@typed-table-r%d-c%d", placement.row+1, placement.column+1)),
			Instance: layoutengine.InstanceID(fmt.Sprintf("@typed-table-r%d-c%d", placement.row+1, placement.column+1)),
			Row:      placement.row, Column: placement.column, RowSpan: placement.rowSpan, ColumnSpan: placement.columnSpan,
			MinWidth: minimum, PreferredWidth: preferred,
		}
	}
	limits := layoutengine.DefaultTablePlanLimits()
	if f.limits.MaxPages > 0 && uint32(f.limits.MaxPages) < limits.MaxPages { // #nosec G115 -- fixed-width conversion is bounded by the surrounding parser, planner, or resource invariant
		limits.MaxPages = uint32(f.limits.MaxPages) // #nosec G115 -- fixed-width conversion is bounded by the surrounding parser, planner, or resource invariant
	}
	resolvedColumns, err := layoutengine.ResolveTableColumnWidths(ctx, tableWidth, columns, engineCells, limits)
	if err != nil {
		return layoutengine.LayoutPlan{}, fmt.Errorf("document: resolve typed table columns: %w", err)
	}
	for index, placement := range placements {
		if err := ctx.Err(); err != nil {
			return layoutengine.LayoutPlan{}, err
		}
		spanWidth, spanErr := typedTableSpanWidth(resolvedColumns, placement.column, placement.columnSpan)
		if spanErr != nil {
			return layoutengine.LayoutPlan{}, spanErr
		}
		measurement, measureErr := f.measureTypedTableCell(ctx, doc, placement, spanWidth, table.Box.BackgroundColor)
		if measureErr != nil {
			return layoutengine.LayoutPlan{}, measureErr
		}
		measurements = append(measurements, measurement)
		engineCells[index].Node, engineCells[index].Key, engineCells[index].Instance = measurement.node, measurement.key, measurement.instance
		engineCells[index].MinHeight = measurement.height
	}
	headerRows := uint32(0)
	if table.Style.RepeatHeader {
		headerRows = uint32(len(table.Header)) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
	}
	var tableBodyForPage func(uint32) (layoutengine.Rect, error)
	if selectBody != nil {
		tableBodyForPage = func(page uint32) (layoutengine.Rect, error) { return selectBody(page, body) }
	}
	rowKeepWithNext := make([]bool, len(rows))
	tableOrphans, tableWidows := table.Box.Orphans, table.Box.Widows
	for rowIndex, row := range rows {
		rowKeepWithNext[rowIndex] = row.KeepWithNext
		if row.Orphans > tableOrphans {
			tableOrphans = row.Orphans
		}
		if row.Widows > tableWidows {
			tableWidows = row.Widows
		}
		for _, cell := range row.Cells {
			rowKeepWithNext[rowIndex] = rowKeepWithNext[rowIndex] || cell.Box.KeepWithNext
		}
	}
	geometry, err := layoutengine.PlanTable(ctx, layoutengine.TablePlanInput{
		PageSize: pageSize, Body: body, Width: tableWidth, Rows: uint32(len(rows)), // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
		Columns: columns, HeaderRows: headerRows, Cells: engineCells,
		Caption: caption, BodyForPage: tableBodyForPage,
		KeepTogether: table.Box.KeepTogether, KeepWithNext: table.Box.KeepWithNext,
		Orphans: tableOrphans, Widows: tableWidows, RowKeepWithNext: rowKeepWithNext,
	}, limits)
	if err != nil {
		return layoutengine.LayoutPlan{}, fmt.Errorf("document: plan typed table: %w", err)
	}
	return composeTypedTablePlan(ctx, geometry, measurements, strings.TrimSpace(doc.Language), table.Style.BorderCollapse)
}

// typedTablePlanWidth lets an entirely fixed grid retain its authored width
// when the page body is wider. The layout engine still receives the body width
// for intrinsic or mixed grids, and for fixed grids that overflow the body, so
// its explicit-width validation remains authoritative.
func typedTablePlanWidth(bodyWidth layoutengine.Fixed, columns []layoutengine.TableColumn) layoutengine.Fixed {
	total := layoutengine.Fixed(0)
	for _, column := range columns {
		if column.Kind != layoutengine.TableTrackFixed {
			return bodyWidth
		}
		var err error
		total, err = total.Add(column.Width)
		if err != nil {
			return bodyWidth
		}
	}
	if total > 0 && total <= bodyWidth {
		return total
	}
	return bodyWidth
}

type typedTablePlacement struct {
	cell       layout.TableCell
	path       string
	row        uint32
	column     uint32
	rowSpan    uint32
	columnSpan uint32
	header     bool
	node       layoutengine.NodeID
}

func typedTablePlacements(rows []layout.TableRow, columnCount int, headerRows uint32, path string) ([]typedTablePlacement, error) {
	if len(rows) == 0 || columnCount == 0 {
		return nil, typedTableUnsupported(path, "table requires at least one row and column")
	}
	occupied := make([][]bool, len(rows))
	for row := range occupied {
		occupied[row] = make([]bool, columnCount)
	}
	placements := make([]typedTablePlacement, 0)
	for rowIndex, row := range rows {
		column := 0
		for cellIndex, cell := range row.Cells {
			for column < columnCount && occupied[rowIndex][column] {
				column++
			}
			cellPath := fmt.Sprintf("%s.rows[%d].cells[%d]", path, rowIndex, cellIndex)
			if column >= columnCount {
				return nil, typedTableUnsupported(cellPath, "cell starts outside the declared columns")
			}
			columnSpan, rowSpan := cell.ColSpan, cell.RowSpan
			if columnSpan <= 0 {
				columnSpan = 1
			}
			if rowSpan <= 0 {
				rowSpan = 1
			}
			if column+columnSpan > columnCount || rowIndex+rowSpan > len(rows) {
				return nil, typedTableUnsupported(cellPath, "rowspan or colspan extends outside the table")
			}
			for r := rowIndex; r < rowIndex+rowSpan; r++ {
				for c := column; c < column+columnSpan; c++ {
					if occupied[r][c] {
						return nil, typedTableUnsupported(cellPath, "rowspan or colspan overlaps another cell")
					}
					occupied[r][c] = true
				}
			}
			placements = append(placements, typedTablePlacement{
				cell: cell, path: cellPath, row: uint32(rowIndex), column: uint32(column),
				rowSpan: uint32(rowSpan), columnSpan: uint32(columnSpan), header: uint32(rowIndex) < headerRows || cell.Header, // #nosec G115 -- fixed-width conversion is bounded by the surrounding parser, planner, or resource invariant
				node: layoutengine.NodeID(len(placements) + 1), // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
			})
			column += columnSpan
		}
	}
	// HTML and authored document tables commonly omit trailing cells. Preserve
	// a rectangular geometry by materializing those slots as empty cells rather
	// than rejecting an otherwise unambiguous grid. Rowspan-covered slots are
	// already marked occupied and are never synthesized.
	for row := range occupied {
		for column, used := range occupied[row] {
			if used {
				continue
			}
			placements = append(placements, typedTablePlacement{
				cell: layout.TableCell{}, path: fmt.Sprintf("%s.rows[%d].implicit[%d]", path, row, column),
				row: uint32(row), column: uint32(column), rowSpan: 1, columnSpan: 1, header: uint32(row) < headerRows,
			})
		}
	}
	sort.SliceStable(placements, func(i, j int) bool {
		if placements[i].row != placements[j].row {
			return placements[i].row < placements[j].row
		}
		return placements[i].column < placements[j].column
	})
	for index := range placements {
		placements[index].node = layoutengine.NodeID(index + 1) // #nosec G115 -- table cells are bounded by planner limits.
	}
	return placements, nil
}

func validateTypedTableSurface(table layout.TableBlock, path string) error {
	if strings.TrimSpace(table.Caption) != "" && len(table.CaptionSegments) != 0 {
		return typedTableUnsupported(path+".caption", "Caption and CaptionSegments are mutually exclusive")
	}
	if table.BoxRef != nil || table.Box.Margin != (layout.Spacing{}) || table.Box.Padding != (layout.Spacing{}) || table.Box.Border != (layout.BorderStyle{}) {
		return typedTableUnsupported(path, "table box supports only a solid background in the strict decoration cohort")
	}
	if len(table.Columns) == 0 {
		return typedTableUnsupported(path+".columns", "at least one column is required")
	}
	for index, column := range table.Columns {
		if !finiteNumbers(column.Width, column.MinWidth, column.MaxWidth) || column.Width < 0 || column.MinWidth < 0 || column.MaxWidth < 0 ||
			column.WidthPercent > 100_000_000 || column.MinWidthPercent > 100_000_000 || column.MaxWidthPercent > 100_000_000 ||
			column.Width > 0 && column.WidthPercent != 0 || column.MinWidth > 0 && column.MinWidthPercent != 0 || column.MaxWidth > 0 && column.MaxWidthPercent != 0 ||
			(column.MaxWidth > 0 && column.MaxWidth < column.MinWidth) ||
			(column.Width > 0 && (column.Width < column.MinWidth || (column.MaxWidth > 0 && column.Width > column.MaxWidth))) {
			return typedTableUnsupported(fmt.Sprintf("%s.columns[%d]", path, index), "width bounds must be finite, non-negative, and choose one fixed or container-relative value")
		}
	}
	if table.Box.Orphans > 1<<20 || table.Box.Widows > 1<<20 {
		return typedTableUnsupported(path+".box", "widow and orphan counts exceed the bounded table policy limit")
	}
	for index, row := range append(append(append([]layout.TableRow(nil), table.Header...), table.Body...), table.Footer...) {
		if row.Orphans > 1<<20 || row.Widows > 1<<20 {
			return typedTableUnsupported(fmt.Sprintf("%s.rows[%d]", path, index), "widow and orphan counts exceed the bounded row policy limit")
		}
		if table.Style.BorderCollapse {
			for cellIndex, cell := range row.Cells {
				if cell.EffectiveBox().Margin != (layout.Spacing{}) {
					return typedTableUnsupported(fmt.Sprintf("%s.rows[%d].cells[%d].box.margin", path, index, cellIndex), "cell margins require separated table borders")
				}
			}
		}
	}
	return nil
}

// typedTableWithInferredColumns keeps the public table model convenient for
// callers that provide rows without an explicit track declaration. The
// planner still uses a fixed, detached column slice so pagination and replay
// remain deterministic.
func typedTableWithInferredColumns(table layout.TableBlock) layout.TableBlock {
	if len(table.Columns) != 0 {
		return table
	}
	count := 0
	rows := append(append(append([]layout.TableRow(nil), table.Header...), table.Body...), table.Footer...)
	for _, row := range rows {
		width := 0
		for _, cell := range row.Cells {
			span := cell.ColSpan
			if span < 1 {
				span = 1
			}
			width += span
		}
		if width > count {
			count = width
		}
	}
	if count == 0 {
		return table
	}
	table.Columns = make([]layout.TableColumn, count)
	return table
}

func (f *Document) measureTypedTableCellIntrinsic(ctx context.Context, doc *layout.LayoutDocument, placement typedTablePlacement, scratchByStyle map[layout.TextStyle]*Document) (layoutengine.Fixed, layoutengine.Fixed, error) {
	if err := layoutengine.ChargePlanningWork(ctx, "typed table intrinsic measurement", 1); err != nil {
		return 0, 0, err
	}
	cell := placement.cell
	cell.Box = cell.EffectiveBox()
	cell.BoxRef = nil
	if cell.Box.Orphans > 1<<20 || cell.Box.Widows > 1<<20 {
		return 0, 0, typedTableUnsupported(placement.path, "widow and orphan counts exceed the bounded cell policy limit")
	}
	cell.Style, cell.StyleRef = cell.EffectiveStyle(), nil
	for _, value := range []float64{cell.Box.Padding.Top, cell.Box.Padding.Right, cell.Box.Padding.Bottom, cell.Box.Padding.Left} {
		if !finiteNumbers(value) || value < 0 {
			return 0, 0, typedTableUnsupported(placement.path+".box.padding", "padding must be finite and non-negative")
		}
	}
	for _, value := range []float64{cell.Box.Margin.Top, cell.Box.Margin.Right, cell.Box.Margin.Bottom, cell.Box.Margin.Left} {
		if !finiteNumbers(value) || value < 0 {
			return 0, 0, typedTableUnsupported(placement.path+".box.margin", "margins must be finite and non-negative")
		}
	}
	padding := cell.Box.Padding.Left + cell.Box.Padding.Right
	margin := cell.Box.Margin.Left + cell.Box.Margin.Right
	base := 2*f.cMargin + padding + margin
	minimum, preferred := base, base
	blocks, err := typedTableExpandedCellBlocks(cell.Blocks, cell.EffectiveStyle(), placement.path)
	if err != nil {
		return 0, 0, err
	}
	for blockIndex, expanded := range blocks {
		block := expanded.block
		if err := ctx.Err(); err != nil {
			return 0, 0, err
		}
		if image, ok := block.(layout.ImageBlock); ok {
			if err := validateTypedPlanningImage(image, fmt.Sprintf("%s.blocks[%d]", placement.path, blockIndex)); err != nil {
				return 0, 0, err
			}
			measured, measureErr := f.measureTypedPlanningImageContext(ctx, image, f.w)
			if measureErr != nil {
				return 0, 0, measureErr
			}
			width := f.PointConvert(measured.flowWidth().Points()) + base
			if width > minimum {
				minimum = width
			}
			if width > preferred {
				preferred = width
			}
			continue
		}
		if nested, ok := block.(layout.TableBlock); ok {
			// Nested table tracks already describe their complete table width. The
			// owning cell's padding is accounted by its own intrinsic base above;
			// adding it again makes an exact-width nested table spuriously exceed
			// an exact-width outer cell.
			minimumWidth, preferredWidth, widthErr := typedNestedTableIntrinsicWidth(f, nested, 0)
			if widthErr != nil {
				return 0, 0, typedTableUnsupported(fmt.Sprintf("%s.blocks[%d]", placement.path, blockIndex), widthErr.Error())
			}
			if minimumWidth > minimum {
				minimum = minimumWidth
			}
			if preferredWidth > preferred {
				preferred = preferredWidth
			}
			continue
		}
		if nested, ok := block.(layout.RowColumnBlock); ok {
			minimumWidth, preferredWidth, widthErr := typedNestedRowColumnIntrinsicWidth(nested, base)
			if widthErr != nil {
				return 0, 0, typedTableUnsupported(fmt.Sprintf("%s.blocks[%d]", placement.path, blockIndex), widthErr.Error())
			}
			minimum, preferred = max(minimum, minimumWidth), max(preferred, preferredWidth)
			continue
		}
		if nested, ok := block.(layout.SectionBlock); ok && htmlUnifiedVisualBox(nested.EffectiveBox()) {
			minimumWidth, preferredWidth, widthErr := typedNestedDecoratedBlockIntrinsicWidth(nested.EffectiveBox(), base)
			if widthErr != nil {
				return 0, 0, typedTableUnsupported(fmt.Sprintf("%s.blocks[%d]", placement.path, blockIndex), widthErr.Error())
			}
			minimum, preferred = max(minimum, minimumWidth), max(preferred, preferredWidth)
			continue
		}
		paragraph, ok := paperParagraphBlock(block)
		if !ok {
			return 0, 0, typedTableUnsupported(fmt.Sprintf("%s.blocks[%d]", placement.path, blockIndex), fmt.Sprintf("%s is unsupported after structured expansion", block.DocumentBlockKind()))
		}
		paragraph.Style = layout.MergedTextStyle(cell.EffectiveStyle(), paragraph.EffectiveStyle())
		paragraph.StyleRef = nil
		style := layout.MergedTextStyle(plannerDefaultTextStyle(f), paragraph.EffectiveStyle())
		scratch := scratchByStyle[style]
		if scratch == nil {
			scratch = documentNew("P", f.unitStr, "", f.fontDirStr, Size{Wd: f.w, Ht: f.h})
			scratch.cMargin, scratch.ws = f.cMargin, f.ws
			scratch.fontFamily, scratch.fontStyle = f.fontFamily, f.fontStyle
			scratch.fontSizePt, scratch.fontSize = f.fontSizePt, f.fontSizePt/scratch.k
			applyPlannerTextStyle(scratch, style)
			if scratch.err == nil && !scratch.isCurrentUTF8 {
				scratchByStyle[style] = scratch
			}
		}
		if scratch.err != nil || scratch.isCurrentUTF8 {
			return 0, 0, typedTableUnsupported(fmt.Sprintf("%s.blocks[%d]", placement.path, blockIndex), "core font metrics could not be resolved for intrinsic sizing")
		}
		text := normalizeCoreMultiCellText(layout.TextSegmentsPlainText(paragraph.Segments))
		for _, line := range strings.Split(text, "\n") {
			if width := scratch.GetStringWidth(line) + base; width > preferred {
				preferred = width
			}
			for _, word := range strings.Fields(line) {
				if width := scratch.GetStringWidth(word) + base; width > minimum {
					minimum = width
				}
			}
		}
	}
	minimumFixed, err := layoutengine.FixedFromPoints(f.UnitToPointConvert(minimum))
	if err != nil || minimumFixed <= 0 {
		return 0, 0, typedTableUnsupported(placement.path, "minimum intrinsic width is not representable")
	}
	preferredFixed, err := layoutengine.FixedFromPoints(f.UnitToPointConvert(preferred))
	if err != nil || preferredFixed < minimumFixed {
		return 0, 0, typedTableUnsupported(placement.path, "preferred intrinsic width is not representable")
	}
	return minimumFixed, preferredFixed, nil
}

func (f *Document) measureTypedTableCell(ctx context.Context, doc *layout.LayoutDocument, placement typedTablePlacement, width layoutengine.Fixed, tableBackground layout.DocumentColor) (typedTableCellMeasurement, error) {
	cell := placement.cell
	cell.Box = cell.EffectiveBox()
	cell.BoxRef = nil
	if cell.Box.Orphans > 1<<20 || cell.Box.Widows > 1<<20 {
		return typedTableCellMeasurement{}, typedTableUnsupported(placement.path, "widow and orphan counts exceed the bounded cell policy limit")
	}
	cell.Style, cell.StyleRef = cell.EffectiveStyle(), nil
	align := strings.ToUpper(strings.TrimSpace(cell.Align))
	switch align {
	case "", "L", "LEFT", "C", "CENTER", "R", "RIGHT":
	default:
		return typedTableCellMeasurement{}, typedTableUnsupported(placement.path+".align", fmt.Sprintf("%q is unsupported", cell.Align))
	}
	vertical := strings.ToUpper(strings.TrimSpace(cell.VerticalAlign))
	switch vertical {
	case "", "T", "TOP", "M", "MIDDLE", "C", "CENTER", "B", "BOTTOM":
	default:
		return typedTableCellMeasurement{}, typedTableUnsupported(placement.path+".vertical_align", fmt.Sprintf("%q is unsupported", cell.VerticalAlign))
	}
	identity := fmt.Sprintf("@typed-table-r%d-c%d", placement.row+1, placement.column+1)
	result := typedTableCellMeasurement{
		node: placement.node, key: layoutengine.NodeKey(identity), instance: layoutengine.InstanceID(identity),
		row: placement.row, column: placement.column, rowSpan: placement.rowSpan, columnSpan: placement.columnSpan,
		header: placement.header, vertical: vertical,
	}
	scope := strings.TrimSpace(cell.Scope)
	if scope != "" {
		if !placement.header {
			return typedTableCellMeasurement{}, typedTableUnsupported(placement.path+".scope", "scope is only valid on header cells")
		}
		switch strings.ToLower(scope) {
		case "row":
			scope = "Row"
		case "column":
			scope = "Column"
		case "both":
			scope = "Both"
		default:
			return typedTableCellMeasurement{}, typedTableUnsupported(placement.path+".scope", "scope must be row, column, or both")
		}
	}
	result.scope = scope
	margins := []float64{cell.Box.Margin.Top, cell.Box.Margin.Right, cell.Box.Margin.Bottom, cell.Box.Margin.Left}
	for index, value := range margins {
		if !finiteNumbers(value) || value < 0 {
			return typedTableCellMeasurement{}, typedTableUnsupported(placement.path+".box.margin", "margins must be finite and non-negative")
		}
		fixed, err := layoutengine.FixedFromPoints(f.UnitToPointConvert(value))
		if err != nil {
			return typedTableCellMeasurement{}, err
		}
		result.margins[index] = fixed
	}
	padding := []float64{cell.Box.Padding.Top, cell.Box.Padding.Right, cell.Box.Padding.Bottom, cell.Box.Padding.Left}
	for index, value := range padding {
		if !finiteNumbers(value) || value < 0 {
			return typedTableCellMeasurement{}, typedTableUnsupported(placement.path+".box.padding", "padding must be finite and non-negative")
		}
		fixed, err := layoutengine.FixedFromPoints(f.UnitToPointConvert(value))
		if err != nil {
			return typedTableCellMeasurement{}, err
		}
		result.padding[index] = fixed
	}
	background := cell.Box.BackgroundColor
	if !background.Set {
		background = tableBackground
	}
	if background.Set {
		if !validDocumentColor(background) {
			return typedTableCellMeasurement{}, typedTableUnsupported(placement.path+".box.background", "color channels must be 0..255")
		}
		result.background = layoutengine.CoreRGBColor{R: uint8(background.R), G: uint8(background.G), B: uint8(background.B), Set: true} // #nosec G115 -- low-width representation is explicitly normalized before packing
	}
	sides := []layout.BorderSide{cell.Box.Border.Top, cell.Box.Border.Right, cell.Box.Border.Bottom, cell.Box.Border.Left}
	for index, side := range sides {
		if !finiteNumbers(side.Width) || side.Width < 0 {
			return typedTableCellMeasurement{}, typedTableUnsupported(placement.path+".box.border", "border widths must be finite and non-negative")
		}
		if side.Width == 0 {
			if side.Style != "" || side.Color.Set {
				return typedTableCellMeasurement{}, typedTableUnsupported(placement.path+".box.border", "zero-width borders cannot carry style or color")
			}
			continue
		}
		if style := strings.ToLower(strings.TrimSpace(side.Style)); style != "" && style != "solid" {
			return typedTableCellMeasurement{}, typedTableUnsupported(placement.path+".box.border", "only solid borders are supported")
		}
		color := side.Color
		if !color.Set {
			color = layout.DocumentColor{Set: true}
		}
		if !validDocumentColor(color) {
			return typedTableCellMeasurement{}, typedTableUnsupported(placement.path+".box.border", "color channels must be 0..255")
		}
		fixed, err := layoutengine.FixedFromPoints(f.UnitToPointConvert(side.Width))
		if err != nil || fixed <= 0 {
			return typedTableCellMeasurement{}, typedTableUnsupported(placement.path+".box.border", "border width is not representable")
		}
		result.borders[index] = typedTableBorder{width: fixed, color: layoutengine.CoreRGBColor{R: uint8(color.R), G: uint8(color.G), B: uint8(color.B), Set: true}} // #nosec G115 -- low-width representation is explicitly normalized before packing
	}
	innerWidth, err := width.Sub(result.margins[1])
	if err != nil {
		return typedTableCellMeasurement{}, err
	}
	innerWidth, err = innerWidth.Sub(result.margins[3])
	if err != nil {
		return typedTableCellMeasurement{}, err
	}
	innerWidth, err = innerWidth.Sub(result.padding[1])
	if err != nil {
		return typedTableCellMeasurement{}, err
	}
	innerWidth, err = innerWidth.Sub(result.padding[3])
	if err != nil || innerWidth <= 0 {
		return typedTableCellMeasurement{}, typedTableUnsupported(placement.path+".box", "horizontal margins and padding consume the cell width")
	}
	blocks, expandErr := typedTableExpandedCellBlocks(cell.Blocks, cell.EffectiveStyle(), placement.path)
	if expandErr != nil {
		return typedTableCellMeasurement{}, expandErr
	}
	if len(blocks) == 0 {
		result.height, _ = layoutengine.FixedFromPoints(f.UnitToPointConvert(layout.ResolvedLineHeight(cell.EffectiveStyle())))
		if result.height <= 0 {
			result.height, _ = layoutengine.FixedFromPoints(12)
		}
		result.height, _ = result.height.Add(result.padding[0])
		result.height, _ = result.height.Add(result.padding[2])
		result.height, _ = result.height.Add(result.margins[0])
		result.height, _ = result.height.Add(result.margins[2])
		return result, nil
	}
	texts := make([]string, 0, len(blocks))
	for blockIndex, expanded := range blocks {
		block := expanded.block
		if image, ok := block.(layout.ImageBlock); ok {
			path := fmt.Sprintf("%s.blocks[%d]", placement.path, blockIndex)
			if err := validateTypedPlanningImage(image, path); err != nil {
				return typedTableCellMeasurement{}, err
			}
			measured, err := f.measureTypedPlanningImageContext(ctx, image, f.PointConvert(innerWidth.Points()))
			if err != nil {
				return typedTableCellMeasurement{}, fmt.Errorf("%s: %w", path, err)
			}
			alt := strings.TrimSpace(image.Alt)
			result.content = append(result.content, typedTableCellContent{image: &measured, role: layoutengine.SemanticRoleArtifact, alt: alt, ancestors: append([]typedTableContentAncestor(nil), expanded.ancestors...)})
			if alt != "" {
				result.content[len(result.content)-1].role = layoutengine.SemanticRoleFigure
				result.content[len(result.content)-1].actualText = alt
				texts = append(texts, alt)
			}
			result.height, err = result.height.Add(measured.flowHeight())
			if err != nil {
				return typedTableCellMeasurement{}, err
			}
			continue
		}
		if nested, ok := block.(layout.TableBlock); ok {
			path := fmt.Sprintf("%s.blocks[%d]", placement.path, blockIndex)
			measurement, measureErr := f.measureTypedNestedTable(ctx, doc, nested, path, innerWidth)
			if measureErr != nil {
				return typedTableCellMeasurement{}, measureErr
			}
			result.content = append(result.content, typedTableCellContent{nested: &measurement, role: layoutengine.SemanticRoleTable, ancestors: append([]typedTableContentAncestor(nil), expanded.ancestors...)})
			result.height, err = result.height.Add(measurement.height)
			if err != nil {
				return typedTableCellMeasurement{}, err
			}
			continue
		}
		if nested, ok := block.(layout.RowColumnBlock); ok {
			path := fmt.Sprintf("%s.blocks[%d]", placement.path, blockIndex)
			measurement, measureErr := f.measureTypedNestedRowColumn(ctx, doc, nested, path, innerWidth)
			if measureErr != nil {
				return typedTableCellMeasurement{}, measureErr
			}
			result.content = append(result.content, typedTableCellContent{nested: &measurement, role: layoutengine.SemanticRoleSection, ancestors: append([]typedTableContentAncestor(nil), expanded.ancestors...)})
			result.height, err = result.height.Add(measurement.height)
			if err != nil {
				return typedTableCellMeasurement{}, err
			}
			continue
		}
		if nested, ok := block.(layout.SectionBlock); ok && htmlUnifiedVisualBox(nested.EffectiveBox()) {
			path := fmt.Sprintf("%s.blocks[%d]", placement.path, blockIndex)
			measurement, measureErr := f.measureTypedNestedDecoratedBlock(ctx, doc, nested, path, innerWidth)
			if measureErr != nil {
				return typedTableCellMeasurement{}, measureErr
			}
			result.content = append(result.content, typedTableCellContent{nested: &measurement, role: layoutengine.SemanticRoleSection, ancestors: append([]typedTableContentAncestor(nil), expanded.ancestors...)})
			result.height, err = result.height.Add(measurement.height)
			if err != nil {
				return typedTableCellMeasurement{}, err
			}
			continue
		}
		paragraph, ok := paperParagraphBlock(block)
		if !ok {
			return typedTableCellMeasurement{}, typedTableUnsupported(fmt.Sprintf("%s.blocks[%d]", placement.path, blockIndex), fmt.Sprintf("%s is unsupported after structured expansion", block.DocumentBlockKind()))
		}
		paragraph.Style = layout.MergedTextStyle(cell.EffectiveStyle(), paragraph.EffectiveStyle())
		paragraph.StyleRef = nil
		if align != "" {
			paragraph.Style.Align = textAlign(align)
		}
		authoredSegments := append([]layout.TextSegment(nil), paragraph.Segments...)
		mixedCoreShadow := typedParagraphNeedsMixedCoreShadow(paragraph, f)
		if !mixedCoreShadow {
			paragraph.Segments = make([]layout.TextSegment, len(authoredSegments))
			for index, segment := range authoredSegments {
				paragraph.Segments[index] = layout.TextSegment{Text: segment.Text, Link: segment.Link, Destination: segment.Destination}
			}
		}
		margins := typedShadowMarginsForCell(f, doc)
		measurement, err := f.measurePaperRowColumnChild(ctx, doc, paragraph, margins.left, margins.top, margins.right, margins.bottom, innerWidth)
		if err != nil {
			return typedTableCellMeasurement{}, fmt.Errorf("%s.blocks[%d]: %w", placement.path, blockIndex, err)
		}
		if !mixedCoreShadow {
			measurement, err = f.restylePaperMeasurement(measurement, paragraph.Style, authoredSegments)
			if err != nil {
				return typedTableCellMeasurement{}, fmt.Errorf("%s.blocks[%d]: %w", placement.path, blockIndex, err)
			}
		}
		result.blocks = append(result.blocks, measurement)
		role, heading := layoutengine.SemanticRoleParagraph, uint8(0)
		if value, ok := block.(layout.HeadingBlock); ok {
			role = layoutengine.SemanticRoleHeading
			if value.Level > 0 && value.Level <= 6 {
				heading = uint8(value.Level)
			}
		}
		result.content = append(result.content, typedTableCellContent{text: &result.blocks[len(result.blocks)-1], role: role, heading: heading, actualText: typedTableCanonicalActualText(layout.TextSegmentsPlainText(authoredSegments)), segments: authoredSegments, ancestors: append([]typedTableContentAncestor(nil), expanded.ancestors...)})
		result.segments = append(result.segments, authoredSegments...)
		result.height, err = result.height.Add(measurement.height)
		if err != nil {
			return typedTableCellMeasurement{}, err
		}
		texts = append(texts, typedTableCanonicalActualText(layout.TextSegmentsPlainText(authoredSegments)))
	}
	result.height, err = result.height.Add(result.padding[0])
	if err != nil {
		return typedTableCellMeasurement{}, err
	}
	result.height, err = result.height.Add(result.padding[2])
	if err != nil {
		return typedTableCellMeasurement{}, err
	}
	result.height, err = result.height.Add(result.margins[0])
	if err != nil {
		return typedTableCellMeasurement{}, err
	}
	result.height, err = result.height.Add(result.margins[2])
	if err != nil {
		return typedTableCellMeasurement{}, err
	}
	result.actualText = strings.Join(texts, "; ")
	return result, nil
}

func validDocumentColor(color layout.DocumentColor) bool {
	return color.R >= 0 && color.R <= 255 && color.G >= 0 && color.G <= 255 && color.B >= 0 && color.B <= 255
}

// restylePaperMeasurement splits already-wrapped core-font runs at exact
// authored segment boundaries. Only geometry-compatible inline overrides are
// accepted: color and core face variants whose natural advance equals the
// measured slice. This keeps line breaking and annotation geometry causal.
func (f *Document) restylePaperMeasurement(measurement paperRowColumnMeasurement, base layout.TextStyle, segments []layout.TextSegment) (paperRowColumnMeasurement, error) {
	styled := false
	for _, segment := range segments {
		styled = styled || segment.StyleRef != nil || segment.Style != (layout.TextStyle{})
	}
	if !styled {
		return measurement, nil
	}
	projection := measurement.plan
	for _, font := range projection.Fonts {
		if font.EmbeddedUTF8 != nil {
			return paperRowColumnMeasurement{}, newTypedShadowUnsupported(typedShadowParagraphContract, "inline style overrides on embedded UTF-8 text are not yet represented")
		}
	}
	text := layout.TextSegmentsPlainText(segments)
	type segmentRange struct {
		start, end int
		style      layout.TextStyle
	}
	ranges := make([]segmentRange, 0, len(segments))
	position := 0
	for _, segment := range segments {
		style := layout.MergedTextStyle(base, segment.EffectiveStyle())
		if style.FontFamily != "" && base.FontFamily != "" && !strings.EqualFold(style.FontFamily, base.FontFamily) ||
			(style.FontSize != 0 && base.FontSize != 0 && style.FontSize != base.FontSize) ||
			(style.LineHeight != 0 && base.LineHeight != 0 && style.LineHeight != base.LineHeight) ||
			style.Align != base.Align || style.Underline || style.StrikeThrough || !validCoreGlyphColor(style.Color) {
			return paperRowColumnMeasurement{}, newTypedShadowUnsupported(typedShadowParagraphContract, "inline style changes layout geometry or uses an unsupported decoration")
		}
		ranges = append(ranges, segmentRange{start: position, end: position + len(segment.Text), style: style})
		position += len(segment.Text)
	}
	fonts := make([]layoutengine.CoreFontResource, 0, 4)
	fontIndex := make(map[paperCoreFontIdentity]layoutengine.FontResourceID)
	runs := make([]layoutengine.CoreGlyphRun, 0, len(projection.GlyphRuns)+len(segments))
	cursor := 0
	for runIndex, run := range projection.GlyphRuns {
		relative := strings.Index(text[cursor:], run.Codes)
		if relative < 0 {
			return paperRowColumnMeasurement{}, newTypedShadowUnsupported(typedShadowParagraphContract, fmt.Sprintf("authored text does not match glyph run %d", runIndex))
		}
		runStart, runEnd := cursor+relative, cursor+relative+len(run.Codes)
		for _, authored := range ranges {
			start, end := max(runStart, authored.start), min(runEnd, authored.end)
			if start >= end {
				continue
			}
			pieceStart, pieceEnd := start-runStart, end-runStart
			piece := run
			piece.Codes = run.Codes[pieceStart:pieceEnd]
			piece.Advances = append([]layoutengine.Fixed(nil), run.Advances[pieceStart:pieceEnd]...)
			piece.Origin.X = run.Origin.X
			for _, advance := range run.Advances[:pieceStart] {
				piece.Origin.X, _ = piece.Origin.X.Add(advance)
			}
			scratch := documentNew("P", f.unitStr, "", f.fontDirStr, Size{Wd: f.w, Ht: f.h})
			scratch.cMargin, scratch.ws = f.cMargin, f.ws
			scratch.fontFamily, scratch.fontStyle = f.fontFamily, f.fontStyle
			scratch.fontSizePt, scratch.fontSize = f.fontSizePt, f.fontSizePt/scratch.k
			applyPlannerTextStyle(scratch, authored.style)
			if scratch.err != nil || scratch.isCurrentUTF8 {
				return paperRowColumnMeasurement{}, newTypedShadowUnsupported(typedShadowParagraphContract, "inline core font could not be resolved")
			}
			resource, err := typedCoreFontResource(scratch.currentFont)
			if err != nil {
				return paperRowColumnMeasurement{}, newTypedShadowUnsupported(typedShadowParagraphContract, err.Error())
			}
			var measured layoutengine.Fixed
			for _, advance := range piece.Advances {
				measured, err = measured.Add(advance)
				if err != nil {
					return paperRowColumnMeasurement{}, err
				}
			}
			natural, err := layoutengine.FixedFromPoints(f.UnitToPointConvert(scratch.GetStringWidth(piece.Codes)))
			delta := int64(natural) - int64(measured)
			if delta < 0 {
				delta = -delta
			}
			if err != nil || delta > 1 {
				return paperRowColumnMeasurement{}, newTypedShadowUnsupported(typedShadowParagraphContract,
					fmt.Sprintf("inline core font metrics change finalized line geometry: measured=%d natural=%d face=%s text=%q", measured, natural, resource.Face, piece.Codes))
			}
			identity := paperFontIdentity(resource)
			fontID := fontIndex[identity]
			if !fontID.Valid() {
				fontID = layoutengine.FontResourceID(len(fonts) + 1) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
				resource.ID = fontID
				fonts = append(fonts, resource)
				fontIndex[identity] = fontID
			}
			piece.Font = fontID
			piece.FontSize, err = layoutengine.FixedFromPoints(scratch.fontSizePt)
			if err != nil || piece.FontSize <= 0 {
				return paperRowColumnMeasurement{}, newTypedShadowUnsupported(typedShadowParagraphContract, "inline font size is not representable")
			}
			piece.Color = coreGlyphColor(authored.style.Color)
			runs = append(runs, piece)
		}
		cursor = runEnd
	}
	var previousLine uint32
	var lineAdvance layoutengine.Fixed
	lineStarted := false
	for index := range runs {
		line := projection.Lines[runs[index].Line]
		if !lineStarted || runs[index].Line != previousLine {
			previousLine, lineAdvance, lineStarted = runs[index].Line, 0, true
		}
		runs[index].Origin = layoutengine.Point{X: line.Bounds.X, Y: line.Baseline}
		runs[index].Origin.X, _ = runs[index].Origin.X.Add(lineAdvance)
		for _, advance := range runs[index].Advances {
			lineAdvance, _ = lineAdvance.Add(advance)
		}
	}
	geometryPages := append([]layoutengine.PlannedPage(nil), projection.Pages...)
	for index := range geometryPages {
		geometryPages[index].Commands = layoutengine.IndexRange{}
	}
	geometry, err := layoutengine.NewLayoutPlan(layoutengine.LayoutPlanInput{Pages: geometryPages, Fragments: projection.Fragments, Lines: projection.Lines, PageRegions: projection.PageRegions, GridTracks: projection.GridTracks, Breaks: projection.Breaks, Diagnostics: projection.Diagnostics})
	if err != nil {
		return paperRowColumnMeasurement{}, err
	}
	restyled, err := layoutengine.AttachCoreGlyphRuns(geometry, fonts, runs)
	if err != nil {
		return paperRowColumnMeasurement{}, err
	}
	measurement.plan = restyled.Projection()
	return measurement, nil
}

// typedTableExpandedCellBlocks lowers the structured block vocabulary to the
// same exact paragraph/image primitives used by measurement and final paint.
// It deliberately performs no geometry work: both intrinsic measurement and
// width-constrained measurement consume this identical canonical sequence.
func typedTableExpandedCellBlocks(blocks []layout.Block, inherited layout.TextStyle, path string) ([]typedTableExpandedBlock, error) {
	const maxDepth = 64
	var expand func([]layout.Block, layout.TextStyle, string, int, []typedTableContentAncestor) ([]typedTableExpandedBlock, error)
	expand = func(input []layout.Block, style layout.TextStyle, current string, depth int, ancestors []typedTableContentAncestor) ([]typedTableExpandedBlock, error) {
		if depth > maxDepth {
			return nil, typedTableUnsupported(current, "structured cell nesting exceeds 64 levels")
		}
		normalized := layout.NormalizeBlocks(input)
		result := make([]typedTableExpandedBlock, 0, len(normalized))
		for index, block := range normalized {
			blockPath := fmt.Sprintf("%s.blocks[%d]", current, index)
			switch value := block.(type) {
			case layout.ParagraphBlock:
				value.Style = layout.MergedTextStyle(style, value.EffectiveStyle())
				value.StyleRef = nil
				result = append(result, typedTableExpandedBlock{block: value, ancestors: append([]typedTableContentAncestor(nil), ancestors...)})
			case layout.HeadingBlock:
				value.Style = layout.MergedTextStyle(style, value.EffectiveStyle())
				value.StyleRef = nil
				result = append(result, typedTableExpandedBlock{block: value, ancestors: append([]typedTableContentAncestor(nil), ancestors...)})
			case layout.ImageBlock:
				result = append(result, typedTableExpandedBlock{block: value, ancestors: append([]typedTableContentAncestor(nil), ancestors...)})
				if len(value.Caption) != 0 {
					result = append(result, typedTableExpandedBlock{block: layout.ParagraphBlock{Segments: append([]layout.TextSegment(nil), value.Caption...), Style: layout.MergedTextStyle(style, layout.TextStyle{Italic: true})}, ancestors: append([]typedTableContentAncestor(nil), ancestors...)})
				}
			case layout.TableBlock:
				if depth >= maxDepth {
					return nil, typedTableUnsupported(blockPath, "nested table depth exceeds 64 levels")
				}
				result = append(result, typedTableExpandedBlock{block: value, ancestors: append([]typedTableContentAncestor(nil), ancestors...)})
			case layout.RowColumnBlock:
				if depth >= maxDepth {
					return nil, typedTableUnsupported(blockPath, "nested row-column depth exceeds 64 levels")
				}
				result = append(result, typedTableExpandedBlock{block: value, ancestors: append([]typedTableContentAncestor(nil), ancestors...)})
			case layout.ListBlock:
				if err := typedTableValidateStructuredBox(value.EffectiveBox(), blockPath); err != nil {
					return nil, err
				}
				switch strings.ToLower(strings.TrimSpace(value.MarkerStyle)) {
				case "", "decimal", "bullet", "disc", "dash":
				default:
					return nil, typedTableUnsupported(blockPath+".marker_style", fmt.Sprintf("%q is unsupported", value.MarkerStyle))
				}
				listStyle := layout.MergedTextStyle(style, value.EffectiveStyle())
				listIdentity := fmt.Sprintf("%s/list", blockPath)
				listAncestors := append(append([]typedTableContentAncestor(nil), ancestors...), typedTableContentAncestor{role: layoutengine.SemanticRoleList, identity: listIdentity})
				for itemIndex, item := range value.Items {
					itemPath := fmt.Sprintf("%s.items[%d]", blockPath, itemIndex)
					itemAncestors := append(append([]typedTableContentAncestor(nil), listAncestors...), typedTableContentAncestor{role: layoutengine.SemanticRoleListItem, identity: itemPath, actualText: typedTableListItemText(item)})
					children, err := expand(item.Blocks, listStyle, itemPath, depth+1, itemAncestors)
					if err != nil {
						return nil, err
					}
					marker, markerErr := paperListMarker(value, itemIndex)
					if markerErr != nil {
						return nil, typedTableUnsupported(blockPath+".marker_style", markerErr.Error())
					}
					marker += " "
					if len(children) == 0 {
						children = []typedTableExpandedBlock{{block: layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: marker}}, Style: listStyle}, ancestors: itemAncestors}}
					} else {
						switch first := children[0].block.(type) {
						case layout.ParagraphBlock:
							first.Segments = append([]layout.TextSegment{{Text: marker}}, first.Segments...)
							children[0].block = first
						case layout.HeadingBlock:
							first.Segments = append([]layout.TextSegment{{Text: marker}}, first.Segments...)
							children[0].block = first
						default:
							children = append([]typedTableExpandedBlock{{block: layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: marker}}, Style: listStyle}, ancestors: itemAncestors}}, children...)
						}
					}
					result = append(result, children...)
				}
			case layout.NoteBoxBlock:
				if err := typedTableValidateStructuredBox(value.EffectiveBox(), blockPath); err != nil {
					return nil, err
				}
				noteStyle := layout.MergedTextStyle(style, value.EffectiveStyle())
				if title := strings.TrimSpace(value.Title); title != "" {
					result = append(result, typedTableExpandedBlock{block: layout.HeadingBlock{Level: 4, Segments: []layout.TextSegment{{Text: title}}, Style: layout.MergedTextStyle(noteStyle, layout.TextStyle{Bold: true})}, ancestors: append([]typedTableContentAncestor(nil), ancestors...)})
				}
				children, err := expand(value.Body, noteStyle, blockPath+".body", depth+1, ancestors)
				if err != nil {
					return nil, err
				}
				result = append(result, children...)
			case layout.SectionBlock:
				if htmlUnifiedVisualBox(value.EffectiveBox()) {
					if depth >= maxDepth {
						return nil, typedTableUnsupported(blockPath, "nested decorated block depth exceeds 64 levels")
					}
					if strings.TrimSpace(value.Title) != "" || len(value.Blocks) != 1 {
						return nil, typedTableUnsupported(blockPath, "decorated table-cell sections require exactly one child and no synthetic title")
					}
					result = append(result, typedTableExpandedBlock{block: value, ancestors: append([]typedTableContentAncestor(nil), ancestors...)})
					continue
				}
				if err := typedTableValidateStructuredBox(value.EffectiveBox(), blockPath); err != nil {
					return nil, err
				}
				if title := strings.TrimSpace(value.Title); title != "" {
					result = append(result, typedTableExpandedBlock{block: layout.HeadingBlock{Level: 3, Segments: []layout.TextSegment{{Text: title}}, Style: layout.MergedTextStyle(style, layout.TextStyle{Bold: true})}, ancestors: append([]typedTableContentAncestor(nil), ancestors...)})
				}
				children, err := expand(value.Blocks, style, blockPath+".body", depth+1, ancestors)
				if err != nil {
					return nil, err
				}
				result = append(result, children...)
			case layout.ClauseBlock:
				if value.BreakBefore || value.BreakAfter {
					return nil, typedTableUnsupported(blockPath, "explicit clause breaks are invalid inside an atomic table cell")
				}
				if err := typedTableValidateStructuredBox(value.EffectiveBox(), blockPath); err != nil {
					return nil, err
				}
				title := strings.TrimSpace(strings.TrimSpace(value.Number) + " " + strings.TrimSpace(value.Title))
				if title != "" {
					result = append(result, typedTableExpandedBlock{block: layout.HeadingBlock{Level: 4, Segments: []layout.TextSegment{{Text: title}}, Style: layout.MergedTextStyle(style, layout.TextStyle{Bold: true})}, ancestors: append([]typedTableContentAncestor(nil), ancestors...)})
				}
				children, err := expand(value.Blocks, style, blockPath+".body", depth+1, ancestors)
				if err != nil {
					return nil, err
				}
				result = append(result, children...)
			default:
				return nil, typedTableUnsupported(blockPath, fmt.Sprintf("%s is not valid structured table-cell content", block.DocumentBlockKind()))
			}
		}
		return result, nil
	}
	return expand(blocks, inherited, path, 0, nil)
}

func typedTableListItemText(item layout.ListItem) string {
	var parts []string
	for _, block := range layout.NormalizeBlocks(item.Blocks) {
		if paragraph, ok := paperParagraphBlock(block); ok {
			if text := typedTableCanonicalActualText(layout.TextSegmentsPlainText(paragraph.Segments)); text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "; ")
}

func typedTableCanonicalActualText(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func typedTableValidateStructuredBox(box layout.BoxStyle, path string) error {
	if box.Orphans > 1<<20 || box.Widows > 1<<20 {
		return typedTableUnsupported(path, "widow and orphan counts exceed the bounded structured policy limit")
	}
	box.KeepTogether, box.KeepWithNext, box.Orphans, box.Widows = false, false, 0, 0
	if box != (layout.BoxStyle{}) {
		return typedTableUnsupported(path, "nested structured visual box decoration is not represented inside table cells")
	}
	return nil
}

func typedNestedTableIntrinsicWidth(f *Document, table layout.TableBlock, base float64) (float64, float64, error) {
	table = typedTableWithInferredColumns(table)
	if err := validateTypedTableSurface(table, "nested_table"); err != nil {
		return 0, 0, err
	}
	minimum, preferred := base, base
	for _, column := range table.Columns {
		columnMinimum := column.MinWidth
		if column.Width > 0 {
			columnMinimum = column.Width
		}
		columnPreferred := column.Width
		if columnPreferred <= 0 {
			columnPreferred = column.MaxWidth
		}
		if columnPreferred <= 0 {
			columnPreferred = max(columnMinimum, 12)
		}
		minimum += columnMinimum
		preferred += max(columnMinimum, columnPreferred)
	}
	if minimum <= 0 {
		minimum = 2 * f.cMargin
	}
	preferred = max(preferred, minimum)
	if !finiteNumbers(minimum, preferred) || minimum <= 0 || preferred < minimum {
		return 0, 0, errors.New("intrinsic width is outside the bounded representable range")
	}
	return minimum, preferred, nil
}

func typedNestedRowColumnIntrinsicWidth(container layout.RowColumnBlock, base float64) (float64, float64, error) {
	if len(container.Items) == 0 {
		return 0, 0, errors.New("row-column container has no items")
	}
	minimum, preferred := base, base
	for _, item := range container.Items {
		width := item.Track.Size
		if width <= 0 {
			width = item.Track.Min
		}
		if width <= 0 {
			width = 12
		}
		if container.Direction == layout.RowDirection {
			minimum += width
			preferred += width
		} else {
			minimum = max(minimum, base+width)
			preferred = max(preferred, base+width)
		}
	}
	if container.Direction == layout.RowDirection && len(container.Items) > 1 {
		minimum += float64(len(container.Items)-1) * container.Gap
		preferred += float64(len(container.Items)-1) * container.Gap
	}
	if !finiteNumbers(minimum, preferred) || minimum <= 0 || preferred < minimum {
		return 0, 0, errors.New("intrinsic row-column width is outside the bounded representable range")
	}
	return minimum, preferred, nil
}

func typedNestedDecoratedBlockIntrinsicWidth(box layout.BoxStyle, base float64) (float64, float64, error) {
	insets := box.Margin.Left + box.Margin.Right + box.Border.Left.Width + box.Border.Right.Width + box.Padding.Left + box.Padding.Right
	minimum := base + insets + 12
	var preferred float64
	if box.Width > 0 {
		minimum, preferred = base+box.Width, base+box.Width
	} else {
		minimum = max(minimum, base+box.MinWidth)
		preferred = minimum
		if box.MaxWidth > 0 {
			preferred = max(minimum, base+box.MaxWidth)
		}
	}
	if !finiteNumbers(minimum, preferred) || minimum <= 0 || preferred < minimum {
		return 0, 0, errors.New("intrinsic decorated block width is outside the bounded representable range")
	}
	return minimum, preferred, nil
}

func (f *Document) measureTypedNestedTable(ctx context.Context, doc *layout.LayoutDocument, table layout.TableBlock, path string, width layoutengine.Fixed) (paperRowColumnMeasurement, error) {
	if width <= 0 {
		return paperRowColumnMeasurement{}, typedTableUnsupported(path, "nested table width must be positive")
	}
	scratch := *f
	scratch.w = f.PointConvert(width.Points())
	// A cell is an atomic table row payload. Give the nested planner a large,
	// bounded measurement page, then reject any nested pagination instead of
	// silently splitting one cell across outer pages.
	scratch.h = max(f.h, f.PointConvert(100000))
	scratch.lMargin, scratch.tMargin, scratch.rMargin, scratch.bMargin = 0, 0, 0, 0
	scratch.limits.MaxPages = 1
	// HTML fixed column hints describe the authored table width, but a nested
	// table is laid out inside the outer cell's content box. Scale only the
	// nested copy when padding makes that content box fractionally narrower;
	// the public model and authored proportions remain unchanged.
	available := f.PointConvert(width.Points())
	fixedTotal := 0.0
	for _, column := range table.Columns {
		if column.Width > 0 {
			fixedTotal += column.Width
		}
	}
	if fixedTotal > available && available > 0 {
		table.Columns = append([]layout.TableColumn(nil), table.Columns...)
		scale := available / fixedTotal
		for index := range table.Columns {
			if table.Columns[index].Width > 0 {
				table.Columns[index].Width *= scale
			}
		}
	}
	nestedDoc := &layout.LayoutDocument{Language: doc.Language, Body: []layout.Block{table}}
	plan, err := scratch.planTypedTable(ctx, nestedDoc, table, path)
	if err != nil {
		return paperRowColumnMeasurement{}, err
	}
	projection := plan.Projection()
	if len(projection.Pages) != 1 {
		return paperRowColumnMeasurement{}, typedTableUnsupported(path, "nested table must fit one atomic cell page")
	}
	var top, bottom layoutengine.Fixed
	for index, fragment := range projection.Fragments {
		if index == 0 || fragment.BorderBox.Y < top {
			top = fragment.BorderBox.Y
		}
		fragmentBottom, bottomErr := fragment.BorderBox.Bottom()
		if bottomErr != nil {
			return paperRowColumnMeasurement{}, bottomErr
		}
		if index == 0 || fragmentBottom > bottom {
			bottom = fragmentBottom
		}
	}
	if len(projection.Fragments) == 0 {
		return paperRowColumnMeasurement{}, typedTableUnsupported(path, "nested table produced no fragments")
	}
	height, err := bottom.Sub(top)
	if err != nil || height <= 0 {
		return paperRowColumnMeasurement{}, typedTableUnsupported(path, "nested table height is not representable")
	}
	body, err := layoutengine.NewRect(0, top, width, height)
	if err != nil {
		return paperRowColumnMeasurement{}, err
	}
	return paperRowColumnMeasurement{plan: projection, body: body, height: height}, nil
}

func (f *Document) measureTypedNestedRowColumn(ctx context.Context, doc *layout.LayoutDocument, container layout.RowColumnBlock, path string, width layoutengine.Fixed) (paperRowColumnMeasurement, error) {
	if width <= 0 {
		return paperRowColumnMeasurement{}, typedTableUnsupported(path, "nested row-column width must be positive")
	}
	height, err := layoutengine.FixedFromPoints(100000)
	if err != nil {
		return paperRowColumnMeasurement{}, err
	}
	body, err := layoutengine.NewRect(0, 0, width, height)
	if err != nil {
		return paperRowColumnMeasurement{}, err
	}
	scratch := *f
	scratch.w = f.PointConvert(width.Points())
	scratch.h = f.PointConvert(height.Points())
	scratch.lMargin, scratch.tMargin, scratch.rMargin, scratch.bMargin = 0, 0, 0, 0
	measurement, err := scratch.measurePaperNestedRowColumn(ctx, doc, papercompile.CompileMapping{}, 0, container, body, width, 1)
	if err != nil {
		return paperRowColumnMeasurement{}, typedTableUnsupported(path, err.Error())
	}
	return measurement, nil
}

func (f *Document) measureTypedNestedDecoratedBlock(ctx context.Context, doc *layout.LayoutDocument, block layout.SectionBlock, path string, width layoutengine.Fixed) (paperRowColumnMeasurement, error) {
	if width <= 0 {
		return paperRowColumnMeasurement{}, typedTableUnsupported(path, "nested decorated block width must be positive")
	}
	scratch := *f
	scratch.w = f.PointConvert(width.Points())
	scratch.h = max(f.h, f.PointConvert(100000))
	scratch.limits.MaxPages = 1
	nestedDoc := &layout.LayoutDocument{Language: doc.Language, Body: []layout.Block{block}}
	plan, err := scratch.planTypedMixedBodies(ctx, nestedDoc, nil)
	if err != nil {
		return paperRowColumnMeasurement{}, typedTableUnsupported(path, err.Error())
	}
	projection := plan.Projection()
	if len(projection.Pages) != 1 || len(projection.Fragments) == 0 {
		return paperRowColumnMeasurement{}, typedTableUnsupported(path, "nested decorated block must produce one non-empty atomic page")
	}
	var left, top, right, bottom layoutengine.Fixed
	for index, fragment := range projection.Fragments {
		fragmentRight, rightErr := fragment.BorderBox.Right()
		fragmentBottom, bottomErr := fragment.BorderBox.Bottom()
		if rightErr != nil || bottomErr != nil {
			return paperRowColumnMeasurement{}, errors.Join(rightErr, bottomErr)
		}
		if index == 0 || fragment.BorderBox.X < left {
			left = fragment.BorderBox.X
		}
		if index == 0 || fragment.BorderBox.Y < top {
			top = fragment.BorderBox.Y
		}
		if index == 0 || fragmentRight > right {
			right = fragmentRight
		}
		if index == 0 || fragmentBottom > bottom {
			bottom = fragmentBottom
		}
	}
	height, err := bottom.Sub(top)
	if err != nil || height <= 0 {
		return paperRowColumnMeasurement{}, typedTableUnsupported(path, "nested decorated block height is not representable")
	}
	body, err := layoutengine.NewRect(left, top, right-left, height)
	if err != nil {
		return paperRowColumnMeasurement{}, err
	}
	return paperRowColumnMeasurement{plan: projection, body: body, height: height}, nil
}

// typedShadowMarginsForCell is expanded at the call site by the small wrapper
// below; keeping it as one value avoids duplicating margin resolution logic.
type typedCellMargins struct{ left, top, right, bottom float64 }

func typedShadowMarginsForCell(f *Document, doc *layout.LayoutDocument) typedCellMargins {
	l, t, r, b := typedShadowMargins(f, doc.PageTemplate.Margins)
	return typedCellMargins{l, t, r, b}
}

func typedTableSpanWidth(columns []layoutengine.Fixed, start, count uint32) (layoutengine.Fixed, error) {
	if count == 0 || uint64(start)+uint64(count) > uint64(len(columns)) {
		return 0, errors.New("document: typed table span is outside its columns")
	}
	var width layoutengine.Fixed
	for _, column := range columns[start : start+count] {
		var err error
		width, err = width.Add(column)
		if err != nil {
			return 0, err
		}
	}
	return width, nil
}

func typedTableUnsupported(path, detail string) error {
	return newTypedShadowUnsupported(typedShadowBlockKind, path+": "+detail)
}

func composeTypedTablePlan(ctx context.Context, base layoutengine.LayoutPlan, measurements []typedTableCellMeasurement, language string, collapse bool) (layoutengine.LayoutPlan, error) {
	projection := base.Projection()
	originalFragments := projection.Fragments
	fragments := make([]layoutengine.Fragment, 0, len(originalFragments)*2)
	fragmentRemap := make(map[layoutengine.FragmentID]layoutengine.FragmentID, len(originalFragments))
	var nextContentNode layoutengine.NodeID
	for _, fragment := range originalFragments {
		if fragment.Node > nextContentNode {
			nextContentNode = fragment.Node
		}
	}
	contentNodes := make(map[layoutengine.NodeKey]layoutengine.NodeID)
	contentNode := func(parent layoutengine.Fragment, index int) (layoutengine.NodeKey, layoutengine.NodeID) {
		key := layoutengine.NodeKey(fmt.Sprintf("%s/content-%d", parent.Key, index+1))
		if node := contentNodes[key]; node.Valid() {
			return key, node
		}
		nextContentNode++
		contentNodes[key] = nextContentNode
		return key, nextContentNode
	}
	byNode := make(map[layoutengine.NodeID]typedTableCellMeasurement, len(measurements))
	for _, measurement := range measurements {
		byNode[measurement.node] = measurement
	}
	collapsed := map[layoutengine.FragmentID][]typedCollapsedBorder{}
	if collapse {
		var err error
		collapsed, err = typedTableCollapsedBorders(ctx, projection, byNode)
		if err != nil {
			return layoutengine.LayoutPlan{}, err
		}
	}
	fonts := make([]layoutengine.CoreFontResource, 0)
	fontIndex := make(map[paperCoreFontIdentity]layoutengine.FontResourceID)
	runs := make([]layoutengine.CoreGlyphRun, 0)
	imageResources := make([]layoutengine.ImageResource, 0)
	imageIndex := make(map[layoutengine.ImageContentDigest]layoutengine.ImageResourceID)
	images := make([]layoutengine.PlannedImage, 0)
	items := make([]layoutengine.DisplayItem, 0)
	destinations := make([]layoutengine.PlannedDestination, 0)
	links := make([]layoutengine.PlannedLink, 0)
	paths := make([]layoutengine.PlannedPath, 0)
	fills := make([]layoutengine.PlannedFill, 0)
	strokes := make([]layoutengine.PlannedStroke, 0)
	lines := make([]layoutengine.PlannedLine, 0)
	nestedSemantics := make([]typedTableNestedSemantics, 0)
	for pageIndex := range projection.Pages {
		page := &projection.Pages[pageIndex]
		originalRange := page.Fragments
		page.Fragments = layoutengine.IndexRange{Start: uint32(len(fragments))} // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
		page.Lines = layoutengine.IndexRange{Start: uint32(len(lines))}         // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
		fragmentEnd := int(originalRange.Start + originalRange.Count)
		for fragmentIndex := int(originalRange.Start); fragmentIndex < fragmentEnd; fragmentIndex++ {
			fragment := originalFragments[fragmentIndex]
			originalFragmentID := fragment.ID
			fragment.ID = layoutengine.FragmentID(len(fragments) + 1) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
			fragmentRemap[originalFragmentID] = fragment.ID
			fragments = append(fragments, fragment)
		}
		for fragmentIndex := int(originalRange.Start); fragmentIndex < fragmentEnd; fragmentIndex++ {
			if fragmentIndex&255 == 0 {
				if err := ctx.Err(); err != nil {
					return layoutengine.LayoutPlan{}, err
				}
			}
			originalFragmentID := originalFragments[fragmentIndex].ID
			plannedFragmentIndex := int(page.Fragments.Start) + fragmentIndex - int(originalRange.Start)
			fragment := fragments[plannedFragmentIndex]
			measurement, ok := byNode[fragment.Node]
			if !ok {
				return layoutengine.LayoutPlan{}, fmt.Errorf("document: typed table fragment %d has no cell measurement", fragment.ID)
			}
			remaining, err := fragment.ContentBox.Height.Sub(measurement.height)
			if err != nil {
				return layoutengine.LayoutPlan{}, err
			}
			if remaining < 0 {
				remaining = 0
			}
			marginInsets := layoutengine.Insets{
				Top: measurement.margins[0], Right: measurement.margins[1],
				Bottom: measurement.margins[2], Left: measurement.margins[3],
			}
			marginBox := fragment.ContentBox
			borderBox, err := marginBox.Inset(marginInsets)
			if err != nil {
				return layoutengine.LayoutPlan{}, typedTableUnsupported("table.cell.box.margin", "cell margins consume the allocated cell box")
			}
			fragment.MarginBox = marginBox
			fragment.BorderBox = borderBox
			fragment.PaddingBox = borderBox
			fragment.ContentBox = borderBox
			fragments[plannedFragmentIndex] = fragment
			yOffset := layoutengine.Fixed(0)
			switch measurement.vertical {
			case "M", "MIDDLE", "C", "CENTER":
				yOffset = remaining / 2
			case "B", "BOTTOM":
				yOffset = remaining
			}
			cellY, err := fragment.ContentBox.Y.Add(yOffset)
			if err != nil {
				return layoutengine.LayoutPlan{}, err
			}
			if measurement.background.Set {
				if len(paths) >= 1<<20 {
					return layoutengine.LayoutPlan{}, typedTableUnsupported("table.decorations", "graphic resource limit exceeded")
				}
				path := typedTableRectPath(fragment.BorderBox)
				paths = append(paths, path)
				fills = append(fills, layoutengine.PlannedFill{Path: uint32(len(paths) - 1), Rule: layoutengine.FillNonZero, Color: measurement.background, Fragment: fragment.ID}) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
				items = append(items, layoutengine.DisplayItem{Kind: layoutengine.CommandFillPath, Payload: uint32(len(fills) - 1)})                                                // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
			}
			borders := make([]typedCollapsedBorder, 0, 4)
			if collapse {
				borders = collapsed[originalFragmentID]
			} else {
				for side, border := range measurement.borders {
					if border.width > 0 {
						path, pathErr := typedTableBorderPath(fragment.BorderBox, side)
						if pathErr != nil {
							return layoutengine.LayoutPlan{}, pathErr
						}
						borders = append(borders, typedCollapsedBorder{path: path, border: border})
					}
				}
			}
			for _, draw := range borders {
				border := draw.border
				if border.width <= 0 {
					continue
				}
				if len(paths) >= 1<<20 {
					return layoutengine.LayoutPlan{}, typedTableUnsupported("table.decorations", "graphic resource limit exceeded")
				}
				paths = append(paths, draw.path)
				strokes = append(strokes, layoutengine.PlannedStroke{Path: uint32(len(paths) - 1), Color: border.color, Width: border.width, Fragment: fragment.ID}) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
				items = append(items, layoutengine.DisplayItem{Kind: layoutengine.CommandStrokePath, Payload: uint32(len(strokes) - 1)})                             // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
			}
			cellY, err = cellY.Add(measurement.padding[0])
			if err != nil {
				return layoutengine.LayoutPlan{}, err
			}
			var blockOffset layoutengine.Fixed
			for contentIndex, content := range measurement.content {
				if content.nested != nil {
					block := *content.nested
					nested := block.plan
					if len(nested.Pages) != 1 || len(nested.Transforms) != 0 || len(nested.Clips) != 0 {
						return layoutengine.LayoutPlan{}, typedTableUnsupported("table.nested", "nested cell projection must be one page without transforms or clips")
					}
					targetX, nestedErr := fragment.ContentBox.X.Add(measurement.padding[3])
					if nestedErr != nil {
						return layoutengine.LayoutPlan{}, nestedErr
					}
					targetY, nestedErr := cellY.Add(blockOffset)
					if nestedErr != nil {
						return layoutengine.LayoutPlan{}, nestedErr
					}
					dx, nestedErr := targetX.Sub(block.body.X)
					if nestedErr != nil {
						return layoutengine.LayoutPlan{}, nestedErr
					}
					dy, nestedErr := targetY.Sub(block.body.Y)
					if nestedErr != nil {
						return layoutengine.LayoutPlan{}, nestedErr
					}
					prefix := fmt.Sprintf("%s/content-%d/", fragment.Key, contentIndex+1)
					fragmentMap := make(map[layoutengine.FragmentID]layoutengine.FragmentID, len(nested.Fragments))
					nodeMap := make(map[layoutengine.NodeID]layoutengine.NodeID)
					for _, childFragment := range nested.Fragments {
						oldID, oldNode := childFragment.ID, childFragment.Node
						if !nodeMap[oldNode].Valid() {
							nextContentNode++
							nodeMap[oldNode] = nextContentNode
						}
						childFragment.ID = layoutengine.FragmentID(len(fragments) + 1) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
						childFragment.Node = nodeMap[oldNode]
						childFragment.Key = layoutengine.NodeKey(prefix + string(childFragment.Key))
						childFragment.Instance = layoutengine.InstanceID(prefix + string(childFragment.Instance))
						childFragment.Page, childFragment.Region = fragment.Page, fragment.Region
						childFragment.MarginBox, nestedErr = translateTypedRect(childFragment.MarginBox, dx, dy)
						if nestedErr == nil {
							childFragment.BorderBox, nestedErr = translateTypedRect(childFragment.BorderBox, dx, dy)
						}
						if nestedErr == nil {
							childFragment.PaddingBox, nestedErr = translateTypedRect(childFragment.PaddingBox, dx, dy)
						}
						if nestedErr == nil {
							childFragment.ContentBox, nestedErr = translateTypedRect(childFragment.ContentBox, dx, dy)
						}
						if nestedErr != nil {
							return layoutengine.LayoutPlan{}, nestedErr
						}
						fragmentMap[oldID] = childFragment.ID
						fragments = append(fragments, childFragment)
					}
					localFonts := make(map[layoutengine.FontResourceID]layoutengine.FontResourceID, len(nested.Fonts))
					for _, font := range nested.Fonts {
						localID := font.ID
						identity := paperFontIdentity(font)
						globalID, exists := fontIndex[identity]
						if !exists {
							globalID = layoutengine.FontResourceID(len(fonts) + 1) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
							font.ID = globalID
							fonts = append(fonts, font)
							fontIndex[identity] = globalID
						}
						localFonts[localID] = globalID
					}
					localImages := make(map[layoutengine.ImageResourceID]layoutengine.ImageResourceID, len(nested.ImageResources))
					for _, resource := range nested.ImageResources {
						localID := resource.ID
						globalID, exists := imageIndex[resource.Digest]
						if !exists {
							globalID = layoutengine.ImageResourceID(len(imageResources) + 1) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
							resource.ID = globalID
							imageResources = append(imageResources, resource)
							imageIndex[resource.Digest] = globalID
						}
						localImages[localID] = globalID
					}
					lineMap := make(map[uint32]uint32, len(nested.Lines))
					for oldIndex, line := range nested.Lines {
						line.Fragment = fragmentMap[line.Fragment]
						line.Bounds, nestedErr = translateTypedRect(line.Bounds, dx, dy)
						if nestedErr == nil {
							line.Baseline, nestedErr = line.Baseline.Add(dy)
						}
						if nestedErr != nil {
							return layoutengine.LayoutPlan{}, nestedErr
						}
						lineMap[uint32(oldIndex)] = uint32(len(lines)) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
						lines = append(lines, line)
					}
					pathBase := uint32(len(paths)) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
					for _, path := range nested.Paths {
						path, nestedErr = translatePaperNestedPath(path, dx, dy)
						if nestedErr != nil {
							return layoutengine.LayoutPlan{}, nestedErr
						}
						paths = append(paths, path)
					}
					destinationMap := make(map[layoutengine.DestinationID]layoutengine.DestinationID, len(nested.Destinations))
					for _, destination := range nested.Destinations {
						oldID := destination.ID
						destination.ID = layoutengine.DestinationID(len(destinations) + 1) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
						destination.Page = fragment.Page
						destination.Fragment = fragmentMap[destination.Fragment]
						destination.Point, nestedErr = translateTypedPoint(destination.Point, dx, dy)
						if nestedErr != nil {
							return layoutengine.LayoutPlan{}, nestedErr
						}
						destinationMap[oldID] = destination.ID
						destinations = append(destinations, destination)
					}
					for _, command := range nested.Commands {
						switch command.Kind {
						case layoutengine.CommandGlyphRun:
							run := nested.GlyphRuns[command.Payload]
							run.Line, run.Font = lineMap[run.Line], localFonts[run.Font]
							run.Origin, nestedErr = translateTypedPoint(run.Origin, dx, dy)
							if nestedErr != nil {
								return layoutengine.LayoutPlan{}, nestedErr
							}
							runs = append(runs, run)
							items = append(items, layoutengine.DisplayItem{Kind: command.Kind, Payload: uint32(len(runs) - 1), Page: fragment.Page}) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
						case layoutengine.CommandFillPath:
							fill := nested.Fills[command.Payload]
							fill.Path += pathBase
							fill.Fragment = fragmentMap[fill.Fragment]
							fills = append(fills, fill)
							items = append(items, layoutengine.DisplayItem{Kind: command.Kind, Payload: uint32(len(fills) - 1), Page: fragment.Page}) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
						case layoutengine.CommandStrokePath:
							stroke := nested.Strokes[command.Payload]
							stroke.Path += pathBase
							stroke.Fragment = fragmentMap[stroke.Fragment]
							strokes = append(strokes, stroke)
							items = append(items, layoutengine.DisplayItem{Kind: command.Kind, Payload: uint32(len(strokes) - 1), Page: fragment.Page}) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
						case layoutengine.CommandImage:
							image := nested.Images[command.Payload]
							image.Resource = localImages[image.Resource]
							image.Fragment = fragmentMap[image.Fragment]
							image.Bounds, nestedErr = translateTypedRect(image.Bounds, dx, dy)
							if nestedErr != nil {
								return layoutengine.LayoutPlan{}, nestedErr
							}
							if image.Crop != nil {
								crop := *image.Crop
								crop.Clip, nestedErr = translateTypedRect(crop.Clip, dx, dy)
								if nestedErr != nil {
									return layoutengine.LayoutPlan{}, nestedErr
								}
								image.Crop = &crop
							}
							images = append(images, image)
							items = append(items, layoutengine.DisplayItem{Kind: command.Kind, Payload: uint32(len(images) - 1), Page: fragment.Page}) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
						case layoutengine.CommandLink:
							link := nested.Links[command.Payload]
							link.Fragment = fragmentMap[link.Fragment]
							link.Bounds, nestedErr = translateTypedRect(link.Bounds, dx, dy)
							if nestedErr != nil {
								return layoutengine.LayoutPlan{}, nestedErr
							}
							if link.Destination.Valid() {
								link.Destination = destinationMap[link.Destination]
							}
							links = append(links, link)
							items = append(items, layoutengine.DisplayItem{Kind: command.Kind, Payload: uint32(len(links) - 1), Page: fragment.Page}) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
						default:
							return layoutengine.LayoutPlan{}, typedTableUnsupported("table.nested", fmt.Sprintf("nested command %q is unsupported", command.Kind))
						}
					}
					nestedSemantics = append(nestedSemantics, typedTableNestedSemantics{projection: nested, fragments: fragmentMap, prefix: prefix})
					blockOffset, nestedErr = blockOffset.Add(block.height)
					if nestedErr != nil {
						return layoutengine.LayoutPlan{}, nestedErr
					}
					continue
				}
				if content.image != nil {
					image := *content.image
					targetX, err := fragment.ContentBox.X.Add(measurement.padding[3])
					if err != nil {
						return layoutengine.LayoutPlan{}, err
					}
					availableWidth, err := fragment.ContentBox.Width.Sub(measurement.padding[1])
					if err == nil {
						availableWidth, err = availableWidth.Sub(measurement.padding[3])
					}
					if err != nil || availableWidth <= 0 {
						return layoutengine.LayoutPlan{}, typedTableUnsupported("table.image", "cell padding leaves no image width")
					}
					targetY, err := cellY.Add(blockOffset)
					if err != nil {
						return layoutengine.LayoutPlan{}, err
					}
					target, err := layoutengine.NewRect(targetX, targetY, availableWidth, image.flowHeight())
					if err != nil {
						return layoutengine.LayoutPlan{}, err
					}
					x, err := image.targetX(target)
					if err != nil {
						return layoutengine.LayoutPlan{}, err
					}
					marginBox, outer, err := image.boxes(x, targetY)
					if err != nil {
						return layoutengine.LayoutPlan{}, err
					}
					contentKey, contentNodeID := contentNode(fragment, contentIndex)
					contentFragment := typedTableContentFragment(fragment, contentKey, contentNodeID, outer)
					contentFragment.MarginBox = marginBox
					contentFragment.ID = layoutengine.FragmentID(len(fragments) + 1) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
					fragments = append(fragments, contentFragment)
					if image.background.Set {
						paths = append(paths, typedTableRectPath(outer))
						fills = append(fills, layoutengine.PlannedFill{Path: uint32(len(paths) - 1), Rule: layoutengine.FillNonZero, Color: image.background, Fragment: contentFragment.ID}) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
						items = append(items, layoutengine.DisplayItem{Kind: layoutengine.CommandFillPath, Payload: uint32(len(fills) - 1)})                                                 // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
					}
					resourceID, exists := imageIndex[image.resource.Digest]
					if !exists {
						resourceID = layoutengine.ImageResourceID(len(imageResources) + 1) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
						resource := image.resource
						resource.ID = resourceID
						imageResources = append(imageResources, resource)
						imageIndex[resource.Digest] = resourceID
					}
					image.resource.ID = resourceID
					placement, err := image.placement(contentFragment.ID, outer)
					if err != nil {
						return layoutengine.LayoutPlan{}, err
					}
					placement.Source = fragment.Source
					images = append(images, placement)
					items = append(items, layoutengine.DisplayItem{Kind: layoutengine.CommandImage, Payload: uint32(len(images) - 1)}) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
					for side, border := range image.borders {
						if border.width <= 0 {
							continue
						}
						path, err := typedTableBorderPath(outer, side)
						if err != nil {
							return layoutengine.LayoutPlan{}, err
						}
						paths = append(paths, path)
						strokes = append(strokes, layoutengine.PlannedStroke{Path: uint32(len(paths) - 1), Color: border.color, Width: border.width, Fragment: contentFragment.ID}) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
						items = append(items, layoutengine.DisplayItem{Kind: layoutengine.CommandStrokePath, Payload: uint32(len(strokes) - 1)})                                    // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
					}
					blockOffset, err = blockOffset.Add(image.flowHeight())
					if err != nil {
						return layoutengine.LayoutPlan{}, err
					}
					continue
				}
				if content.text == nil {
					return layoutengine.LayoutPlan{}, errors.New("document: typed table content has no text or image payload")
				}
				block := *content.text
				contentX, err := fragment.ContentBox.X.Add(measurement.padding[3])
				if err != nil {
					return layoutengine.LayoutPlan{}, err
				}
				contentY, err := cellY.Add(blockOffset)
				if err != nil {
					return layoutengine.LayoutPlan{}, err
				}
				contentWidth, err := fragment.ContentBox.Width.Sub(measurement.padding[1])
				if err == nil {
					contentWidth, err = contentWidth.Sub(measurement.padding[3])
				}
				if err != nil || contentWidth <= 0 {
					return layoutengine.LayoutPlan{}, typedTableUnsupported("table.content", "cell padding leaves no content width")
				}
				contentBounds, err := layoutengine.NewRect(contentX, contentY, contentWidth, block.height)
				if err != nil {
					return layoutengine.LayoutPlan{}, err
				}
				contentKey, contentNodeID := contentNode(fragment, contentIndex)
				contentFragment := typedTableContentFragment(fragment, contentKey, contentNodeID, contentBounds)
				contentFragment.ID = layoutengine.FragmentID(len(fragments) + 1) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
				fragments = append(fragments, contentFragment)
				localFonts := make(map[layoutengine.FontResourceID]layoutengine.FontResourceID, len(block.plan.Fonts))
				for _, font := range block.plan.Fonts {
					localID := font.ID
					identity := paperFontIdentity(font)
					globalID, exists := fontIndex[identity]
					if !exists {
						globalID = layoutengine.FontResourceID(len(fonts) + 1) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
						font.ID = globalID
						fonts = append(fonts, font)
						fontIndex[identity] = globalID
					}
					localFonts[localID] = globalID
				}
				lineMap := make(map[uint32]uint32, len(block.plan.Lines))
				for localIndex, line := range block.plan.Lines {
					xOffset, err := line.Bounds.X.Sub(block.body.X)
					if err != nil {
						return layoutengine.LayoutPlan{}, err
					}
					yLocal, err := line.Bounds.Y.Sub(block.body.Y)
					if err != nil {
						return layoutengine.LayoutPlan{}, err
					}
					x, err := fragment.ContentBox.X.Add(measurement.padding[3])
					if err != nil {
						return layoutengine.LayoutPlan{}, err
					}
					x, err = x.Add(xOffset)
					if err != nil {
						return layoutengine.LayoutPlan{}, err
					}
					y, err := cellY.Add(blockOffset)
					if err != nil {
						return layoutengine.LayoutPlan{}, err
					}
					y, err = y.Add(yLocal)
					if err != nil {
						return layoutengine.LayoutPlan{}, err
					}
					bounds, err := layoutengine.NewRect(x, y, line.Bounds.Width, line.Bounds.Height)
					if err != nil {
						return layoutengine.LayoutPlan{}, err
					}
					baselineOffset, err := line.Baseline.Sub(line.Bounds.Y)
					if err != nil {
						return layoutengine.LayoutPlan{}, err
					}
					baseline, err := y.Add(baselineOffset)
					if err != nil {
						return layoutengine.LayoutPlan{}, err
					}
					globalLine := uint32(len(lines)) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
					lines = append(lines, layoutengine.PlannedLine{Fragment: contentFragment.ID, Index: uint32(localIndex), Bounds: bounds, Baseline: baseline})
					lineMap[uint32(localIndex)] = globalLine
				}
				for _, run := range block.plan.GlyphRuns {
					localLine := run.Line
					globalLine := lineMap[localLine]
					line := lines[globalLine]
					runOffset, err := run.Origin.X.Sub(block.plan.Lines[localLine].Bounds.X)
					if err != nil {
						return layoutengine.LayoutPlan{}, err
					}
					run.Line, run.Font = globalLine, localFonts[run.Font]
					run.Origin = layoutengine.Point{X: line.Bounds.X, Y: line.Baseline}
					run.Origin.X, err = run.Origin.X.Add(runOffset)
					if err != nil {
						return layoutengine.LayoutPlan{}, err
					}
					runs = append(runs, run)
					items = append(items, layoutengine.DisplayItem{Kind: layoutengine.CommandGlyphRun, Payload: uint32(len(runs) - 1)}) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
				}
				if len(block.plan.Strokes) != 0 {
					dx, dxErr := contentX.Sub(block.body.X)
					dy, dyErr := contentY.Sub(block.body.Y)
					if dxErr != nil || dyErr != nil {
						return layoutengine.LayoutPlan{}, typedTableUnsupported("table.decoration", fmt.Sprintf("text decoration translation offset: %v %v", dxErr, dyErr))
					}
					for _, stroke := range block.plan.Strokes {
						if uint64(stroke.Path) >= uint64(len(block.plan.Paths)) {
							return layoutengine.LayoutPlan{}, typedTableUnsupported("table.decoration", "text decoration references a missing path")
						}
						path, pathErr := translatePaperNestedPath(block.plan.Paths[stroke.Path], dx, dy)
						if pathErr != nil {
							return layoutengine.LayoutPlan{}, pathErr
						}
						paths = append(paths, path)
						stroke.Path = uint32(len(paths) - 1) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
						stroke.Fragment = contentFragment.ID
						strokes = append(strokes, stroke)
						items = append(items, layoutengine.DisplayItem{Kind: layoutengine.CommandStrokePath, Payload: uint32(len(strokes) - 1)}) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
					}
				}
				for _, link := range block.plan.Links {
					if link.URI == "" || link.Destination.Valid() {
						return layoutengine.LayoutPlan{}, typedTableUnsupported("table.link", "only external cell links are supported")
					}
					xOffset, err := link.Bounds.X.Sub(block.body.X)
					if err != nil {
						return layoutengine.LayoutPlan{}, err
					}
					yOffset, err := link.Bounds.Y.Sub(block.body.Y)
					if err != nil {
						return layoutengine.LayoutPlan{}, err
					}
					x, err := fragment.ContentBox.X.Add(measurement.padding[3])
					if err != nil {
						return layoutengine.LayoutPlan{}, err
					}
					x, err = x.Add(xOffset)
					if err != nil {
						return layoutengine.LayoutPlan{}, err
					}
					y, err := cellY.Add(blockOffset)
					if err != nil {
						return layoutengine.LayoutPlan{}, err
					}
					y, err = y.Add(yOffset)
					if err != nil {
						return layoutengine.LayoutPlan{}, err
					}
					bounds, err := layoutengine.NewRect(x, y, link.Bounds.Width, link.Bounds.Height)
					if err != nil {
						return layoutengine.LayoutPlan{}, err
					}
					links = append(links, layoutengine.PlannedLink{Fragment: contentFragment.ID, Bounds: bounds, URI: link.URI, Source: fragment.Source})
				}
				blockOffset, err = blockOffset.Add(block.height)
				if err != nil {
					return layoutengine.LayoutPlan{}, err
				}
			}
		}
		page.Fragments.Count = uint32(len(fragments)) - page.Fragments.Start // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
		page.Lines.Count = uint32(len(lines)) - page.Lines.Start             // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
	}
	for index := range projection.Breaks {
		projection.Breaks[index].Preceding = fragmentRemap[projection.Breaks[index].Preceding]
		projection.Breaks[index].Triggering = fragmentRemap[projection.Breaks[index].Triggering]
	}
	for index := range projection.Diagnostics {
		remapTypedTableDiagnosticLocation(&projection.Diagnostics[index].Location, fragmentRemap)
		for related := range projection.Diagnostics[index].Related {
			remapTypedTableDiagnosticLocation(&projection.Diagnostics[index].Related[related].Location, fragmentRemap)
		}
	}
	geometry, err := layoutengine.NewLayoutPlan(layoutengine.LayoutPlanInput{
		Pages: projection.Pages, Fragments: fragments, Lines: lines,
		PageRegions: projection.PageRegions, GridTracks: projection.GridTracks,
		Breaks: projection.Breaks, Diagnostics: projection.Diagnostics,
	})
	if err != nil {
		return layoutengine.LayoutPlan{}, err
	}
	plan, err := layoutengine.AttachDisplayList(geometry, layoutengine.DisplayListInput{Fonts: fonts, GlyphRuns: runs, ImageResources: imageResources, Images: images, Destinations: destinations, Links: links, Paths: paths, Fills: fills, Strokes: strokes, Items: items})
	if err != nil {
		return layoutengine.LayoutPlan{}, err
	}
	linkedSegments := make(map[layoutengine.NodeID][]layout.TextSegment)
	for _, measurement := range measurements {
		for contentIndex, content := range measurement.content {
			if len(content.segments) == 0 {
				continue
			}
			hasLinkMetadata := false
			for _, segment := range content.segments {
				hasLinkMetadata = hasLinkMetadata || strings.TrimSpace(segment.Link) != "" || strings.TrimSpace(segment.Destination) != ""
			}
			if !hasLinkMetadata {
				continue
			}
			key := layoutengine.NodeKey(fmt.Sprintf("%s/content-%d", measurement.key, contentIndex+1))
			if node := contentNodes[key]; node.Valid() {
				linkedSegments[node] = append([]layout.TextSegment(nil), content.segments...)
			}
		}
	}
	if len(linkedSegments) != 0 {
		plan, err = attachTypedSegmentLinks(plan, linkedSegments)
		if err != nil {
			return layoutengine.LayoutPlan{}, err
		}
	}
	return attachTypedTableSemantics(plan, measurements, language, nestedSemantics)
}

func typedTableContentFragment(parent layoutengine.Fragment, identity layoutengine.NodeKey, node layoutengine.NodeID, bounds layoutengine.Rect) layoutengine.Fragment {
	return layoutengine.Fragment{
		ID: 0, Node: node,
		Key: identity, Instance: layoutengine.InstanceID(identity), Page: parent.Page, Region: parent.Region,
		BorderBox: bounds, ContentBox: bounds, Source: parent.Source,
		Continuation: parent.Continuation, Repeated: parent.Repeated,
	}
}

func remapTypedTableDiagnosticLocation(location *layoutengine.DiagnosticLocation, remap map[layoutengine.FragmentID]layoutengine.FragmentID) {
	if location != nil && location.Fragment.Valid() {
		location.Fragment = remap[location.Fragment]
	}
}

type typedCollapsedBorder struct {
	path   layoutengine.PlannedPath
	border typedTableBorder
}
type typedTableEdgeLine struct {
	page       uint32
	vertical   bool
	coordinate layoutengine.Fixed
}
type typedTableEdgeSegment struct {
	line       typedTableEdgeLine
	start, end layoutengine.Fixed
}
type typedTableEdgeCandidate struct {
	segment  typedTableEdgeSegment
	fragment layoutengine.FragmentID
	border   typedTableBorder
}

func typedTableCollapsedBorders(ctx context.Context, projection layoutengine.LayoutPlanProjection, measurements map[layoutengine.NodeID]typedTableCellMeasurement) (map[layoutengine.FragmentID][]typedCollapsedBorder, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	points := make(map[typedTableEdgeLine][]layoutengine.Fixed)
	candidates := make([]typedTableEdgeCandidate, 0)
	for index, fragment := range projection.Fragments {
		if index&255 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		measurement, ok := measurements[fragment.Node]
		if !ok || measurement.caption {
			continue
		}
		right, err := fragment.BorderBox.Right()
		if err != nil {
			return nil, err
		}
		bottom, err := fragment.BorderBox.Bottom()
		if err != nil {
			return nil, err
		}
		edges := []typedTableEdgeSegment{{line: typedTableEdgeLine{page: fragment.Page, coordinate: fragment.BorderBox.Y}, start: fragment.BorderBox.X, end: right}, {line: typedTableEdgeLine{page: fragment.Page, vertical: true, coordinate: right}, start: fragment.BorderBox.Y, end: bottom}, {line: typedTableEdgeLine{page: fragment.Page, coordinate: bottom}, start: fragment.BorderBox.X, end: right}, {line: typedTableEdgeLine{page: fragment.Page, vertical: true, coordinate: fragment.BorderBox.X}, start: fragment.BorderBox.Y, end: bottom}}
		for side, edge := range edges {
			border := measurement.borders[side]
			if border.width <= 0 {
				continue
			}
			points[edge.line] = append(points[edge.line], edge.start, edge.end)
			candidates = append(candidates, typedTableEdgeCandidate{segment: edge, fragment: fragment.ID, border: border})
		}
	}
	for line, values := range points {
		sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
		unique := values[:0]
		for _, value := range values {
			if len(unique) == 0 || unique[len(unique)-1] != value {
				unique = append(unique, value)
			}
		}
		points[line] = unique
	}
	winners := make(map[typedTableEdgeSegment]typedTableEdgeCandidate)
	work := 0
	for index, candidate := range candidates {
		if index&255 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		values := points[candidate.segment.line]
		start := sort.Search(len(values), func(i int) bool { return values[i] >= candidate.segment.start })
		for i := start; i+1 < len(values) && values[i] < candidate.segment.end; i++ {
			if values[i+1] > candidate.segment.end {
				break
			}
			work++
			if work > 1<<20 {
				return nil, typedTableUnsupported("table.style.border_collapse", "collapsed edge work limit exceeded")
			}
			segment := typedTableEdgeSegment{line: candidate.segment.line, start: values[i], end: values[i+1]}
			current, exists := winners[segment]
			if !exists || typedTableBorderWins(candidate, current) {
				candidate.segment = segment
				winners[segment] = candidate
			}
		}
	}
	ordered := make([]typedTableEdgeCandidate, 0, len(winners))
	for _, winner := range winners {
		ordered = append(ordered, winner)
	}
	sort.Slice(ordered, func(i, j int) bool {
		a, b := ordered[i].segment, ordered[j].segment
		if a.line.page != b.line.page {
			return a.line.page < b.line.page
		}
		if a.line.vertical != b.line.vertical {
			return !a.line.vertical
		}
		if a.line.coordinate != b.line.coordinate {
			return a.line.coordinate < b.line.coordinate
		}
		if a.start != b.start {
			return a.start < b.start
		}
		return a.end < b.end
	})
	result := make(map[layoutengine.FragmentID][]typedCollapsedBorder)
	for _, winner := range ordered {
		var start, end layoutengine.Point
		line := winner.segment.line
		if line.vertical {
			start = layoutengine.Point{X: line.coordinate, Y: winner.segment.start}
			end = layoutengine.Point{X: line.coordinate, Y: winner.segment.end}
		} else {
			start = layoutengine.Point{X: winner.segment.start, Y: line.coordinate}
			end = layoutengine.Point{X: winner.segment.end, Y: line.coordinate}
		}
		bounds, err := layoutengine.RectFromPoints(start, end)
		if err != nil {
			return nil, err
		}
		path := layoutengine.PlannedPath{Bounds: bounds, Segments: []layoutengine.PathSegment{{Kind: layoutengine.PathMoveTo, Point: start}, {Kind: layoutengine.PathLineTo, Point: end}}}
		result[winner.fragment] = append(result[winner.fragment], typedCollapsedBorder{path: path, border: winner.border})
	}
	return result, nil
}

func typedTableBorderWins(candidate, current typedTableEdgeCandidate) bool {
	if candidate.border.width != current.border.width {
		return candidate.border.width > current.border.width
	}
	return candidate.fragment < current.fragment
}

func typedTableRectPath(box layoutengine.Rect) layoutengine.PlannedPath {
	right, _ := box.Right()
	bottom, _ := box.Bottom()
	return layoutengine.PlannedPath{Bounds: box, Segments: []layoutengine.PathSegment{{Kind: layoutengine.PathMoveTo, Point: layoutengine.Point{X: box.X, Y: box.Y}}, {Kind: layoutengine.PathLineTo, Point: layoutengine.Point{X: right, Y: box.Y}}, {Kind: layoutengine.PathLineTo, Point: layoutengine.Point{X: right, Y: bottom}}, {Kind: layoutengine.PathLineTo, Point: layoutengine.Point{X: box.X, Y: bottom}}, {Kind: layoutengine.PathClose}}}
}

func typedTableBorderPath(box layoutengine.Rect, side int) (layoutengine.PlannedPath, error) {
	right, err := box.Right()
	if err != nil {
		return layoutengine.PlannedPath{}, err
	}
	bottom, err := box.Bottom()
	if err != nil {
		return layoutengine.PlannedPath{}, err
	}
	var start, end layoutengine.Point
	switch side {
	case 0:
		start = layoutengine.Point{X: box.X, Y: box.Y}
		end = layoutengine.Point{X: right, Y: box.Y}
	case 1:
		start = layoutengine.Point{X: right, Y: box.Y}
		end = layoutengine.Point{X: right, Y: bottom}
	case 2:
		start = layoutengine.Point{X: box.X, Y: bottom}
		end = layoutengine.Point{X: right, Y: bottom}
	case 3:
		start = layoutengine.Point{X: box.X, Y: box.Y}
		end = layoutengine.Point{X: box.X, Y: bottom}
	default:
		return layoutengine.PlannedPath{}, errors.New("document: invalid table border side")
	}
	bounds, err := layoutengine.RectFromPoints(start, end)
	if err != nil {
		return layoutengine.PlannedPath{}, err
	}
	return layoutengine.PlannedPath{Bounds: bounds, Segments: []layoutengine.PathSegment{{Kind: layoutengine.PathMoveTo, Point: start}, {Kind: layoutengine.PathLineTo, Point: end}}}, nil
}

func attachTypedTableSemantics(plan layoutengine.LayoutPlan, measurements []typedTableCellMeasurement, language string, nestedTables []typedTableNestedSemantics) (layoutengine.LayoutPlan, error) {
	projection := plan.Projection()
	nodes := []layoutengine.SemanticNode{
		{ID: 1, Role: layoutengine.SemanticRoleDocument, Key: "@typed-document", Instance: "@typed-document", Attributes: layoutengine.SemanticAttributes{Language: language}},
		{ID: 2, Parent: 1, Role: layoutengine.SemanticRoleTable, Key: "@typed-table", Instance: "@typed-table"},
	}
	maxRow := uint32(0)
	for _, cell := range measurements {
		if cell.caption {
			continue
		}
		if cell.row > maxRow {
			maxRow = cell.row
		}
	}
	rowSemantics := make([]layoutengine.SemanticNodeID, maxRow+1)
	for row := uint32(0); row <= maxRow; row++ {
		id := layoutengine.SemanticNodeID(len(nodes) + 1) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
		identity := layoutengine.NodeKey(fmt.Sprintf("@typed-table-row-%d", row+1))
		nodes = append(nodes, layoutengine.SemanticNode{ID: id, Parent: 2, Role: layoutengine.SemanticRoleRow, Key: identity, Instance: layoutengine.InstanceID(identity)})
		rowSemantics[row] = id
	}
	semanticByKey := make(map[layoutengine.NodeKey]layoutengine.SemanticNodeID, len(measurements)*2)
	nestedParents := make(map[string]layoutengine.SemanticNodeID, len(nestedTables))
	appendContentSemantics := func(parent layoutengine.SemanticNodeID, cell typedTableCellMeasurement) {
		ancestorNodes := make(map[string]layoutengine.SemanticNodeID)
		for contentIndex, content := range cell.content {
			contentParent := parent
			chain := ""
			for _, ancestor := range content.ancestors {
				chain += "/" + ancestor.identity
				if existing := ancestorNodes[chain]; existing.Valid() {
					contentParent = existing
					continue
				}
				key := layoutengine.NodeKey(fmt.Sprintf("%s/container-%d", cell.key, len(ancestorNodes)+1))
				attributes := layoutengine.SemanticAttributes{}
				if ancestor.role == layoutengine.SemanticRoleListItem && ancestor.actualText != "" {
					attributes.ActualText = ancestor.actualText
				}
				id := layoutengine.SemanticNodeID(len(nodes) + 1) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
				nodes = append(nodes, layoutengine.SemanticNode{ID: id, Parent: contentParent, Role: ancestor.role, Key: key, Instance: layoutengine.InstanceID(key), Attributes: attributes})
				ancestorNodes[chain] = id
				contentParent = id
			}
			role := content.role
			if content.nested != nil {
				nestedParents[fmt.Sprintf("%s/content-%d/", cell.key, contentIndex+1)] = contentParent
				continue
			}
			if role == "" {
				role = layoutengine.SemanticRoleParagraph
			}
			key := layoutengine.NodeKey(fmt.Sprintf("%s/content-%d", cell.key, contentIndex+1))
			attributes := layoutengine.SemanticAttributes{HeadingLevel: content.heading}
			if role == layoutengine.SemanticRoleFigure {
				attributes.AlternateText = content.alt
			}
			if content.actualText != "" && role != layoutengine.SemanticRoleArtifact && role != layoutengine.SemanticRoleFigure {
				attributes.ActualText = content.actualText
			}
			nodes = append(nodes, layoutengine.SemanticNode{
				ID: layoutengine.SemanticNodeID(len(nodes) + 1), Parent: contentParent, Role: role, // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
				Key: key, Instance: layoutengine.InstanceID(key), Attributes: attributes,
			})
			semanticByKey[key] = nodes[len(nodes)-1].ID
		}
	}
	for _, cell := range measurements {
		id := layoutengine.SemanticNodeID(len(nodes) + 1) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
		semanticKey := layoutengine.NodeKey(string(cell.key) + "/semantic")
		if cell.caption {
			nodes = append(nodes, layoutengine.SemanticNode{ID: id, Parent: 2, Role: layoutengine.SemanticRoleParagraph, Key: semanticKey, Instance: layoutengine.InstanceID(semanticKey), Attributes: layoutengine.SemanticAttributes{ActualText: cell.actualText}})
			artifactID := layoutengine.SemanticNodeID(len(nodes) + 1) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
			nodes = append(nodes, layoutengine.SemanticNode{ID: artifactID, Parent: id, Role: layoutengine.SemanticRoleArtifact, Key: cell.key, Instance: cell.instance})
			semanticByKey[cell.key] = artifactID
			appendContentSemantics(id, cell)
			continue
		}
		attributes := layoutengine.SemanticAttributes{TableHeader: cell.header, TableScope: cell.scope}
		if cell.rowSpan > 1 {
			attributes.TableRowSpan = cell.rowSpan
		}
		if cell.columnSpan > 1 {
			attributes.TableColumnSpan = cell.columnSpan
		}
		nodes = append(nodes, layoutengine.SemanticNode{
			ID: id, Parent: rowSemantics[cell.row], Role: layoutengine.SemanticRoleCell,
			Key: semanticKey, Instance: layoutengine.InstanceID(semanticKey),
			Attributes: attributes,
		})
		artifactID := layoutengine.SemanticNodeID(len(nodes) + 1) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
		nodes = append(nodes, layoutengine.SemanticNode{ID: artifactID, Parent: id, Role: layoutengine.SemanticRoleArtifact, Key: cell.key, Instance: cell.instance})
		semanticByKey[cell.key] = artifactID
		appendContentSemantics(id, cell)
	}
	for _, nested := range nestedTables {
		parent := nestedParents[nested.prefix]
		if !parent.Valid() {
			return layoutengine.LayoutPlan{}, fmt.Errorf("document: nested table %q has no outer semantic parent", nested.prefix)
		}
		semanticMap := map[layoutengine.SemanticNodeID]layoutengine.SemanticNodeID{1: parent}
		for _, childNode := range nested.projection.SemanticNodes {
			if childNode.ID == 1 && childNode.Role == layoutengine.SemanticRoleDocument {
				continue
			}
			oldID, oldParent := childNode.ID, childNode.Parent
			childNode.ID = layoutengine.SemanticNodeID(len(nodes) + 1) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
			childNode.Parent = semanticMap[oldParent]
			if !childNode.Parent.Valid() {
				return layoutengine.LayoutPlan{}, fmt.Errorf("document: nested table semantic parent %d is unavailable", oldParent)
			}
			childNode.Key = layoutengine.NodeKey(nested.prefix + string(childNode.Key))
			childNode.Instance = layoutengine.InstanceID(nested.prefix + string(childNode.Instance))
			semanticMap[oldID] = childNode.ID
			nodes = append(nodes, childNode)
		}
		for _, association := range nested.projection.SemanticFragments {
			fragment := nested.fragments[association.Fragment]
			semantic := semanticMap[association.Semantic]
			if !fragment.Valid() || !semantic.Valid() {
				return layoutengine.LayoutPlan{}, errors.New("document: nested table semantic association is incomplete")
			}
			semanticByKey[projection.Fragments[fragment-1].Key] = semantic
		}
	}
	associations := make([]layoutengine.SemanticFragmentAssociation, 0, len(projection.Fragments))
	reading := make([]layoutengine.ReadingOccurrence, 0, len(projection.Fragments))
	pageIndex := make(map[uint32]uint32)
	for _, fragment := range projection.Fragments {
		semantic := semanticByKey[fragment.Key]
		if !semantic.Valid() {
			return layoutengine.LayoutPlan{}, fmt.Errorf("document: table fragment %d has no semantic owner", fragment.ID)
		}
		associations = append(associations, layoutengine.SemanticFragmentAssociation{Semantic: semantic, Page: fragment.Page, Fragment: fragment.ID})
		if nodes[semantic-1].Role != layoutengine.SemanticRoleArtifact {
			reading = append(reading, layoutengine.ReadingOccurrence{Semantic: semantic, Page: fragment.Page, Fragment: fragment.ID, ReadingIndex: pageIndex[fragment.Page]})
			pageIndex[fragment.Page]++
		}
	}
	return layoutengine.AttachSemantics(plan, nodes, associations, reading)
}
