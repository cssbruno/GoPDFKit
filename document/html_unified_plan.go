// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
	"github.com/cssbruno/gopdfkit/layout"
)

// ErrHTMLPlanUnsupported reports that a compiled HTML fragment cannot be
// lowered as one unit to the unified planner. Public HTML entry points fail
// atomically for this error; there is no automatic legacy renderer route.
var ErrHTMLPlanUnsupported = errors.New("document: HTML unified plan unsupported")

const htmlUnifiedMaxTextBytes = 4 << 20

type htmlAuthoredWhitespaceContextKey struct{}

func withHTMLAuthoredWhitespace(ctx context.Context) context.Context {
	return context.WithValue(ctx, htmlAuthoredWhitespaceContextKey{}, true)
}

func preservesHTMLAuthoredWhitespace(ctx context.Context) bool {
	preserve, _ := ctx.Value(htmlAuthoredWhitespaceContextKey{}).(bool)
	return preserve
}

// PlanCompiledHTML lowers the initial strict HTML text/list cohort through the
// same immutable LayoutDocumentPlan used by typed and .paper inputs. Planning
// is atomic and does not mutate the receiving Document.
func (f *Document) PlanCompiledHTML(lineHeight float64, compiled *CompiledHTML) (LayoutDocumentPlan, error) {
	return f.PlanCompiledHTMLContext(context.Background(), lineHeight, compiled)
}

// PlanCompiledHTMLContext is the cancellation-aware form of PlanCompiledHTML.
// The entire fragment is capability-scanned and lowered before layout begins.
func (f *Document) PlanCompiledHTMLContext(ctx context.Context, lineHeight float64, compiled *CompiledHTML) (LayoutDocumentPlan, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return LayoutDocumentPlan{}, err
	}
	if compiled == nil {
		return LayoutDocumentPlan{}, errors.New("document: compiled HTML is nil")
	}
	if lineHeight <= 0 {
		return LayoutDocumentPlan{}, errors.New("document: HTML unified plan line height must be positive")
	}
	if err := compiled.validate(); err != nil {
		return LayoutDocumentPlan{}, err
	}
	if len(compiled.recovery) != 0 {
		return LayoutDocumentPlan{}, htmlPlanUnsupported("malformed-recovery", 0, "recovered HTML is not accepted by the strict unified cohort")
	}
	resolved, err := f.resolveCompiledHTMLUnifiedSnapshot(ctx, compiled, lineHeight)
	if err != nil {
		return LayoutDocumentPlan{}, err
	}
	resolved, err = f.resolveCompiledHTMLImageSources(ctx, resolved)
	if err != nil {
		return LayoutDocumentPlan{}, err
	}
	availableWidth := f.w - f.lMargin - f.rMargin
	availableHeight := f.h - f.tMargin - f.bMargin
	var model *layout.LayoutDocument
	var svgMetas []htmlUnifiedSVGMeta
	var svgPlaceholder layoutengine.ImageContentDigest
	if len(resolved.inlineSVGs) != 0 {
		if _, soleErr := htmlUnifiedInlineSVGMeta(resolved); soleErr == nil {
			return f.planCompiledHTMLInlineSVGContext(ctx, resolved)
		}
		svgMetas, err = htmlUnifiedMixedSVGMetas(resolved)
		if err != nil {
			return LayoutDocumentPlan{}, err
		}
		model, svgPlaceholder, err = lowerCompiledHTMLMixedSVGUnitsBounds(ctx, resolved, lineHeight, f.PointConvert, availableWidth, availableHeight, svgMetas)
	} else {
		model, err = lowerCompiledHTMLTextCohortUnitsBounds(ctx, resolved, lineHeight, f.PointConvert, availableWidth, availableHeight)
	}
	if err != nil {
		return LayoutDocumentPlan{}, err
	}
	if err := ctx.Err(); err != nil {
		return LayoutDocumentPlan{}, err
	}
	plan, err := f.PlanLayoutDocumentContext(withHTMLAuthoredWhitespace(ctx), model)
	if err != nil {
		if errors.Is(err, ErrLayoutDocumentPlanUnsupported) {
			return LayoutDocumentPlan{}, fmt.Errorf("%w: typed lowering: %w", ErrHTMLPlanUnsupported, err)
		}
		return LayoutDocumentPlan{}, err
	}
	if len(svgMetas) != 0 {
		composed, composeErr := composeHTMLMixedSVGPlan(ctx, plan.plan, svgMetas, svgPlaceholder)
		if composeErr != nil {
			return LayoutDocumentPlan{}, composeErr
		}
		composed, bindErr := bindTypedDeterministicInputs(composed, plan.tree, model)
		if bindErr != nil {
			return LayoutDocumentPlan{}, fmt.Errorf("document: bind mixed HTML/SVG deterministic inputs: %w", bindErr)
		}
		plan.plan = composed
		plan.imageSources = withoutHTMLSVGPlaceholder(plan.imageSources, svgPlaceholder)
		layoutHash, hashErr := composed.Hash()
		if hashErr != nil {
			return LayoutDocumentPlan{}, hashErr
		}
		plan.hash, hashErr = hashTypedLayoutDocumentEnvelope(layoutHash.String(), plan.envelope)
		if hashErr != nil {
			return LayoutDocumentPlan{}, hashErr
		}
	}
	return plan, nil
}

func lowerCompiledHTMLTextCohort(ctx context.Context, compiled *CompiledHTML, lineHeight float64) (*layout.LayoutDocument, error) {
	return lowerCompiledHTMLTextCohortUnits(ctx, compiled, lineHeight, func(value float64) float64 { return value })
}

func lowerCompiledHTMLTextCohortUnits(ctx context.Context, compiled *CompiledHTML, lineHeight float64, pointsToUnits func(float64) float64) (*layout.LayoutDocument, error) {
	return lowerCompiledHTMLTextCohortUnitsWidth(ctx, compiled, lineHeight, pointsToUnits, 0)
}

func lowerCompiledHTMLTextCohortUnitsWidth(ctx context.Context, compiled *CompiledHTML, lineHeight float64, pointsToUnits func(float64) float64, availableWidth float64) (*layout.LayoutDocument, error) {
	return lowerCompiledHTMLTextCohortUnitsBounds(ctx, compiled, lineHeight, pointsToUnits, availableWidth, 0)
}

func lowerCompiledHTMLTextCohortUnitsBounds(ctx context.Context, compiled *CompiledHTML, lineHeight float64, pointsToUnits func(float64) float64, availableWidth, availableHeight float64) (*layout.LayoutDocument, error) {
	if len(compiled.unifiedResolved) != len(compiled.tokens) {
		clone := *compiled
		clone.unifiedResolved = make([]htmlUnifiedResolvedElement, len(compiled.tokens))
		for index, token := range compiled.tokens {
			if token.Cat != 'O' {
				continue
			}
			style := layout.TextStyle{LineHeight: lineHeight}
			if _, heading := htmlPlanHeadingLevel(token.Str); heading {
				style.Bold = true
			}
			clone.unifiedResolved[index] = htmlUnifiedResolvedElement{text: style, textTransform: "none", decl: parseStyleDeclarations(token.Attr["style"])}
		}
		compiled = &clone
	}
	model := &layout.LayoutDocument{}
	textBytes := 0
	state := &htmlPlanLoweringState{boxContainingHeights: map[int]float64{0: availableHeight}}
	body, err := lowerHTMLPlanBlockRangeWidthState(ctx, compiled, 0, len(compiled.tokens), lineHeight, &textBytes, 0, pointsToUnits, availableWidth, state)
	if err != nil {
		return nil, err
	}
	model.Body = body
	if len(model.Body) == 0 {
		return nil, htmlPlanUnsupported("fragment", 0, "fragment has no plannable content")
	}
	return model, nil
}

func lowerHTMLPlanBlockRange(ctx context.Context, compiled *CompiledHTML, start, limit int, lineHeight float64, textBytes *int, depth int, pointsToUnits func(float64) float64) ([]layout.Block, error) {
	return lowerHTMLPlanBlockRangeWidthState(ctx, compiled, start, limit, lineHeight, textBytes, depth, pointsToUnits, 0, &htmlPlanLoweringState{})
}

func lowerHTMLPlanBlockRangeWidth(ctx context.Context, compiled *CompiledHTML, start, limit int, lineHeight float64, textBytes *int, depth int, pointsToUnits func(float64) float64, availableWidth float64) ([]layout.Block, error) {
	return lowerHTMLPlanBlockRangeWidthState(ctx, compiled, start, limit, lineHeight, textBytes, depth, pointsToUnits, availableWidth, &htmlPlanLoweringState{})
}

func lowerHTMLPlanBlockRangeWidthState(ctx context.Context, compiled *CompiledHTML, start, limit int, lineHeight float64, textBytes *int, depth int, pointsToUnits func(float64) float64, availableWidth float64, state *htmlPlanLoweringState) ([]layout.Block, error) {
	if depth > 512 {
		return nil, htmlPlanUnsupported("fragment", start, "block nesting exceeds the unified adapter limit")
	}
	if state.boxContainingHeights == nil {
		state.boxContainingHeights = make(map[int]float64)
	}
	tokens := compiled.tokens
	body := make([]layout.Block, 0)
	for index := start; index < limit; {
		if index&255 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		token := tokens[index]
		switch token.Cat {
		case 'T':
			text := collapseHTMLPlanText(token.Str)
			if text != "" {
				*textBytes += len(text)
				if *textBytes > htmlUnifiedMaxTextBytes {
					return nil, ErrHTMLLimitExceeded
				}
				body = append(body, htmlPlanParagraph(text, lineHeight))
			}
			index++
		case 'O':
			if compiled.unifiedResolved[index].displayNone {
				// A suppressed element contributes no box, text, image, table,
				// or descendant flow. Void images have no closing token; all
				// other elements can be skipped using their exact boundary.
				if token.Str == "img" {
					index++
					continue
				}
				end, boundaryErr := htmlPlanElementEnd(compiled, index)
				if boundaryErr != nil {
					return nil, boundaryErr
				}
				index = end + 1
				continue
			}
			if token.Str == "svg" && state.inlineSVGBlocks != nil {
				block, ok := state.inlineSVGBlocks[index]
				if !ok {
					return nil, htmlPlanUnsupported("svg", index, "inline SVG has no unified-flow placeholder")
				}
				body = append(body, block)
				end, err := htmlPlanElementEnd(compiled, index)
				if err != nil {
					return nil, err
				}
				index = end + 1
				continue
			}
			if token.Str == "img" {
				image, err := htmlPlanImage(compiled, index, pointsToUnits, availableWidth)
				if err != nil {
					return nil, err
				}
				body = append(body, image)
				index++
				continue
			}
			end, err := htmlPlanElementEnd(compiled, index)
			if err != nil {
				return nil, err
			}
			resolvedBox := compiled.unifiedResolved[index].box
			if htmlUnifiedBlockLikeTag(token.Str, compiled.unifiedResolved[index].decl) {
				resolvedBox, err = htmlUnifiedResolveBlockBox(compiled, index, availableWidth, state.boxContainingHeights[depth], pointsToUnits)
				if err != nil {
					return nil, err
				}
			}
			before, after, err := false, false, error(nil)
			if token.Str != "table" {
				before, after, err = htmlPlanBlockBreakPolicy(compiled, index)
			}
			if err != nil {
				return nil, err
			}
			if before {
				body = append(body, layout.PageBreakBlock{After: true})
			}
			switch {
			case token.Str == "p":
				segments, err := htmlPlanInlineSegments(ctx, compiled, index+1, end, textBytes)
				if err != nil {
					return nil, err
				}
				if len(segments) != 0 {
					resolved := compiled.unifiedResolved[index]
					body = append(body, layout.ParagraphBlock{Segments: segments, Style: resolved.text, Box: resolvedBox})
				}
			case token.Str == "pre" || token.Str == "blockquote" || token.Str == "address":
				segments, err := htmlPlanInlineSegments(ctx, compiled, index+1, end, textBytes)
				if err != nil {
					return nil, err
				}
				if len(segments) != 0 {
					resolved := compiled.unifiedResolved[index]
					body = append(body, layout.ParagraphBlock{Segments: segments, Style: resolved.text, Box: resolvedBox})
				}
			case func() bool { _, ok := htmlPlanHeadingLevel(token.Str); return ok }():
				segments, err := htmlPlanInlineSegments(ctx, compiled, index+1, end, textBytes)
				if err != nil {
					return nil, err
				}
				if len(segments) != 0 {
					level, _ := htmlPlanHeadingLevel(token.Str)
					resolved := compiled.unifiedResolved[index]
					body = append(body, layout.HeadingBlock{Level: level, Segments: segments, Style: resolved.text, Box: resolvedBox})
				}
			case token.Str == "span" && htmlUnifiedBlockLikeTag(token.Str, compiled.unifiedResolved[index].decl):
				segments, err := htmlPlanInlineSegments(ctx, compiled, index+1, end, textBytes)
				if err != nil {
					return nil, err
				}
				if len(segments) != 0 {
					resolved := compiled.unifiedResolved[index]
					body = append(body, layout.ParagraphBlock{Segments: segments, Style: resolved.text, Box: resolvedBox})
				}
			case token.Str == "ul" || token.Str == "ol":
				list, err := htmlPlanList(ctx, compiled, index, end, lineHeight, textBytes)
				if err != nil {
					return nil, err
				}
				if len(list.Items) != 0 {
					body = append(body, list)
				}
			case token.Str == "html" || token.Str == "body" || token.Str == "div" || token.Str == "section" || token.Str == "article" || token.Str == "main" || token.Str == "header" || token.Str == "footer":
				if compiled.unifiedResolved[index].flexContainer != nil {
					container, err := htmlPlanFlexBlock(ctx, compiled, index, lineHeight, textBytes, pointsToUnits, state)
					if err != nil {
						return nil, err
					}
					body = append(body, container)
				} else {
					childWidth, childHeight := availableWidth, state.boxContainingHeights[depth]
					if availableWidth > 0 {
						var boundsErr error
						childWidth, childHeight, boundsErr = htmlUnifiedBoxContentBounds(resolvedBox, availableWidth, state.boxContainingHeights[depth])
						if boundsErr != nil {
							return nil, htmlPlanUnsupported(token.Str, index, boundsErr.Error())
						}
					}
					state.boxContainingHeights[depth+1] = childHeight
					children, err := lowerHTMLPlanBlockRangeWidthState(ctx, compiled, index+1, end, lineHeight, textBytes, depth+1, pointsToUnits, childWidth, state)
					if err != nil {
						return nil, err
					}
					if strings.EqualFold(strings.TrimSpace(compiled.unifiedResolved[index].decl["display"]), "contents") && htmlUnifiedVisualBox(resolvedBox) {
						return nil, htmlPlanUnsupported(token.Str, index, "display:contents requires an undecorated wrapper in the unified flow cohort")
					}
					if htmlUnifiedVisualBox(resolvedBox) {
						if len(children) == 0 {
							return nil, htmlPlanUnsupported(token.Str, index, "a decorated structural wrapper requires plannable content")
						}
						body = append(body, layout.SectionBlock{Blocks: children, Box: resolvedBox})
					} else {
						body = append(body, children...)
					}
				}
			case token.Str == "head" || token.Str == "style" || token.Str == "title" || token.Str == "script":
				// Non-rendered frontend structure is consumed during compilation.
			case token.Str == "dl":
				definitions, err := htmlPlanDefinitionList(ctx, compiled, index, end, lineHeight, textBytes)
				if err != nil {
					return nil, err
				}
				body = append(body, definitions...)
			case token.Str == "figure":
				figure, err := htmlPlanFigure(ctx, compiled, index, end, lineHeight, textBytes, pointsToUnits, availableWidth)
				if err != nil {
					return nil, err
				}
				body = append(body, figure)
			case token.Str == "table":
				table, err := htmlPlanTable(ctx, compiled, index, end, lineHeight, textBytes, pointsToUnits, availableWidth, state)
				if err != nil {
					return nil, err
				}
				body = append(body, table)
			default:
				return nil, htmlPlanUnsupported(token.Str, index, "element is not in the initial unified text/list cohort")
			}
			if after {
				body = append(body, layout.PageBreakBlock{After: true})
			}
			index = end + 1
		case 'C':
			return nil, htmlPlanUnsupported(token.Str, index, "unexpected block-level closing element")
		default:
			return nil, htmlPlanUnsupported("token", index, "unknown token category")
		}
	}
	return body, nil
}

const (
	htmlUnifiedMaxTableColumns     = 1024
	htmlUnifiedMaxNestedTableDepth = 8
)

type htmlPlanLoweringState struct {
	tableDepth           int
	tableRows            int
	boxContainingHeights map[int]float64
	inlineSVGBlocks      map[int]layout.ImageBlock
}

type htmlPlanTableColumnHint struct {
	column                   int
	width, minimum, maximum  float64
	hasWidth, hasMin, hasMax bool
	token                    int
}

type htmlPlanTableCellDefaults struct {
	padding float64
	border  layout.BorderSide
}

func htmlPlanTable(ctx context.Context, compiled *CompiledHTML, start, end int, lineHeight float64, textBytes *int, pointsToUnits func(float64) float64, availableWidth float64, state *htmlPlanLoweringState) (layout.TableBlock, error) {
	if state == nil {
		state = &htmlPlanLoweringState{}
	}
	state.tableDepth++
	defer func() { state.tableDepth-- }()
	if state.tableDepth > htmlUnifiedMaxNestedTableDepth {
		return layout.TableBlock{}, ErrHTMLLimitExceeded
	}
	tableAttrs := compiled.tokens[start].Attr
	for name := range tableAttrs {
		if name != "style" && name != "width" && name != "border" && name != "cellpadding" && name != "bordercolor" {
			return layout.TableBlock{}, htmlPlanUnsupported("table", start, "table attribute is outside the unified cohort")
		}
	}
	cellDefaults := htmlPlanTableCellDefaults{padding: 1.5}
	if raw, exists := tableAttrs["cellpadding"]; exists {
		points, ok := parseHTMLBoxLength(raw, nil, 0)
		if !ok || points < 0 || !finiteNumbers(points) {
			return layout.TableBlock{}, htmlPlanUnsupported("table", start, "cellpadding must be a bounded non-negative CSS length")
		}
		cellDefaults.padding = pointsToUnits(points)
	}
	if raw, exists := tableAttrs["border"]; exists {
		if strings.TrimSpace(raw) == "" {
			raw = "1"
		}
		points, ok := parseHTMLBoxLength(raw, nil, 0)
		if !ok || points < 0 || !finiteNumbers(points) {
			return layout.TableBlock{}, htmlPlanUnsupported("table", start, "border must be a bounded non-negative CSS length")
		}
		width := pointsToUnits(points)
		if width > 0 {
			cellDefaults.border = layout.BorderSide{Width: width, Style: "solid", Color: layout.DocumentColor{Set: true}}
		}
	}
	if raw := strings.TrimSpace(tableAttrs["bordercolor"]); raw != "" {
		color, ok := parseCSSColor(raw)
		if !ok {
			return layout.TableBlock{}, htmlPlanUnsupported("table", start, "bordercolor is invalid")
		}
		cellDefaults.border.Color = layout.DocumentColor{R: color.R, G: color.G, B: color.B, Set: true}
	}
	var table layout.TableBlock
	decl := compiled.unifiedResolved[start].decl
	table.Box.KeepTogether = compiled.unifiedResolved[start].box.KeepTogether
	tableWidth := strings.ToLower(strings.TrimSpace(firstNonEmpty(decl["width"], tableAttrs["width"])))
	if tableWidth != "" && tableWidth != "auto" && tableWidth != "100%" {
		return layout.TableBlock{}, htmlPlanUnsupported("table", start, "the exact flow table width is auto or 100%")
	}
	if len(decl) != 0 {
		if value := decl["background-color"]; value != "" {
			color, ok := parseCSSColor(value)
			if !ok {
				return layout.TableBlock{}, htmlPlanUnsupported("table", start, "background-color is invalid")
			}
			table.Box.BackgroundColor = layout.DocumentColor{R: color.R, G: color.G, B: color.B, Set: true}
		}
		if value := strings.ToLower(decl["border-collapse"]); value != "" {
			switch value {
			case "collapse":
				table.Style.BorderCollapse = true
			case "separate":
			default:
				return layout.TableBlock{}, htmlPlanUnsupported("table", start, "border-collapse supports collapse or separate")
			}
		}
	}
	seen := map[string]bool{}
	phase := 0
	var columnHints []htmlPlanTableColumnHint
	for index := start + 1; index < end; {
		if err := ctx.Err(); err != nil {
			return layout.TableBlock{}, err
		}
		token := compiled.tokens[index]
		if token.Cat == 'T' && collapseHTMLPlanText(token.Str) == "" {
			index++
			continue
		}
		if token.Cat != 'O' || len(token.Attr) != 0 {
			return layout.TableBlock{}, htmlPlanUnsupported(token.Str, index, "table requires direct attribute-free caption/thead/tbody/tfoot children")
		}
		childEnd, err := htmlPlanElementEnd(compiled, index)
		if err != nil || childEnd > end {
			if err != nil {
				return layout.TableBlock{}, err
			}
			return layout.TableBlock{}, htmlPlanUnsupported(token.Str, index, "table child exceeds table")
		}
		if token.Str != "tr" && seen[token.Str] {
			return layout.TableBlock{}, htmlPlanUnsupported(token.Str, index, "duplicate table section")
		}
		switch token.Str {
		case "caption":
			if phase != 0 {
				return layout.TableBlock{}, htmlPlanUnsupported("caption", index, "caption must be the first table child")
			}
			segments, err := htmlPlanInlineSegments(ctx, compiled, index+1, childEnd, textBytes)
			if err != nil {
				return layout.TableBlock{}, err
			}
			if strings.TrimSpace(layout.TextSegmentsPlainText(segments)) == "" {
				return layout.TableBlock{}, htmlPlanUnsupported("caption", index, "caption is empty")
			}
			table.CaptionSegments = append([]layout.TextSegment(nil), segments...)
		case "thead", "tbody", "tfoot":
			want := map[string]int{"thead": 1, "tbody": 2, "tfoot": 3}[token.Str]
			if want < phase || token.Str == "thead" && phase > 0 {
				return layout.TableBlock{}, htmlPlanUnsupported(token.Str, index, "table sections are out of order")
			}
			phase = want
			section, hints, err := htmlPlanTableRows(ctx, compiled, index, childEnd, lineHeight, textBytes, state, pointsToUnits, availableWidth, cellDefaults)
			if err != nil {
				return layout.TableBlock{}, err
			}
			columnHints = append(columnHints, hints...)
			switch token.Str {
			case "thead":
				table.Header = section
			case "tbody":
				table.Body = section
			case "tfoot":
				table.Footer = section
			}
		case "tr":
			if seen["tbody"] || phase > 2 {
				return layout.TableBlock{}, htmlPlanUnsupported("tr", index, "direct rows cannot mix with tbody or follow tfoot")
			}
			phase = 2
			cursor := index
			for cursor < end {
				candidate := compiled.tokens[cursor]
				if candidate.Cat == 'T' && collapseHTMLPlanText(candidate.Str) == "" {
					cursor++
					continue
				}
				if candidate.Cat != 'O' || candidate.Str != "tr" || len(candidate.Attr) != 0 {
					break
				}
				candidateEnd, candidateErr := htmlPlanElementEnd(compiled, cursor)
				if candidateErr != nil {
					return layout.TableBlock{}, candidateErr
				}
				cursor = candidateEnd + 1
			}
			section, hints, rowErr := htmlPlanTableRows(ctx, compiled, index, cursor, lineHeight, textBytes, state, pointsToUnits, availableWidth, cellDefaults)
			if rowErr != nil {
				return layout.TableBlock{}, rowErr
			}
			table.Body = section
			columnHints = append(columnHints, hints...)
			seen["tbody"] = true
			index = cursor
			continue
		default:
			return layout.TableBlock{}, htmlPlanUnsupported(token.Str, index, "direct tr and non-table children are unsupported")
		}
		seen[token.Str] = true
		index = childEnd + 1
	}
	if !seen["tbody"] || len(table.Body) == 0 {
		return layout.TableBlock{}, htmlPlanUnsupported("table", start, "one non-empty tbody is required")
	}
	columns := 0
	for _, section := range [][]layout.TableRow{table.Header, table.Body, table.Footer} {
		if len(section) == 0 {
			continue
		}
		sectionColumns, err := htmlPlanTableColumnCount(section)
		if err != nil {
			return layout.TableBlock{}, htmlPlanUnsupported("table", start, err.Error())
		}
		if columns != 0 && sectionColumns != columns {
			return layout.TableBlock{}, htmlPlanUnsupported("table", start, "table sections do not share one rectangular column grid")
		}
		columns = sectionColumns
	}
	if columns == 0 || columns > htmlUnifiedMaxTableColumns {
		return layout.TableBlock{}, ErrHTMLLimitExceeded
	}
	table.Columns = make([]layout.TableColumn, columns)
	for _, hint := range columnHints {
		if hint.column < 0 || hint.column >= len(table.Columns) {
			return layout.TableBlock{}, htmlPlanUnsupported("table", hint.token, "cell width hint resolves outside the table grid")
		}
		column := &table.Columns[hint.column]
		if hint.hasWidth {
			if column.Width != 0 && column.Width != hint.width {
				return layout.TableBlock{}, htmlPlanUnsupported("table", hint.token, "conflicting fixed width hints target one column")
			}
			column.Width = hint.width
		}
		if hint.hasMin && hint.minimum > column.MinWidth {
			column.MinWidth = hint.minimum
		}
		if hint.hasMax && (column.MaxWidth == 0 || hint.maximum < column.MaxWidth) {
			column.MaxWidth = hint.maximum
		}
		if column.MaxWidth > 0 && column.MaxWidth < column.MinWidth || column.Width > 0 && (column.Width < column.MinWidth || column.MaxWidth > 0 && column.Width > column.MaxWidth) {
			return layout.TableBlock{}, htmlPlanUnsupported("table", hint.token, "column width hint conflicts with its minimum or maximum")
		}
	}
	table.Style.RepeatHeader = len(table.Header) != 0
	table.Style.KeepRows = table.Box.KeepTogether
	if len(table.Body) >= 2 {
		table.Box.Orphans = 2
	}
	return table, nil
}

func htmlPlanTableRows(ctx context.Context, compiled *CompiledHTML, start, end int, lineHeight float64, textBytes *int, state *htmlPlanLoweringState, pointsToUnits func(float64) float64, availableWidth float64, defaults htmlPlanTableCellDefaults) ([]layout.TableRow, []htmlPlanTableColumnHint, error) {
	rows := make([]layout.TableRow, 0)
	var hints []htmlPlanTableColumnHint
	var active []int
	rangeStart := start + 1
	if compiled.tokens[start].Str == "tr" {
		rangeStart = start
	}
	for index := rangeStart; index < end; {
		token := compiled.tokens[index]
		if token.Cat == 'T' && collapseHTMLPlanText(token.Str) == "" {
			index++
			continue
		}
		if token.Cat != 'O' || token.Str != "tr" || len(token.Attr) != 0 {
			return nil, nil, htmlPlanUnsupported(token.Str, index, "table sections require direct attribute-free tr children")
		}
		rowEnd, err := htmlPlanElementEnd(compiled, index)
		if err != nil {
			return nil, nil, err
		}
		if compiled.unifiedResolved[index].displayNone {
			// A hidden row consumes no grid slot and cannot affect rowspan
			// occupancy in the retained table model.
			index = rowEnd + 1
			continue
		}
		row := layout.TableRow{KeepTogether: true}
		columnIndex := 0
		for cellIndex := index + 1; cellIndex < rowEnd; {
			cellToken := compiled.tokens[cellIndex]
			if cellToken.Cat == 'T' && collapseHTMLPlanText(cellToken.Str) == "" {
				cellIndex++
				continue
			}
			if cellToken.Cat != 'O' || (cellToken.Str != "td" && cellToken.Str != "th") {
				return nil, nil, htmlPlanUnsupported(cellToken.Str, cellIndex, "rows require direct th/td children")
			}
			cellEnd, err := htmlPlanElementEnd(compiled, cellIndex)
			if err != nil {
				return nil, nil, err
			}
			if compiled.unifiedResolved[cellIndex].displayNone {
				cellIndex = cellEnd + 1
				continue
			}
			for name := range cellToken.Attr {
				if name != "colspan" && name != "rowspan" && name != "style" && name != "width" && name != "align" && name != "valign" {
					return nil, nil, htmlPlanUnsupported(cellToken.Str, cellIndex, "cells allow only colspan, rowspan, width, and resolved style")
				}
			}
			colspanRaw, colspanSet := cellToken.Attr["colspan"]
			colspan, err := htmlPlanTableSpan(colspanRaw, colspanSet)
			if err != nil {
				return nil, nil, htmlPlanUnsupported(cellToken.Str, cellIndex, "invalid colspan")
			}
			rowspanRaw, rowspanSet := cellToken.Attr["rowspan"]
			rowspan, err := htmlPlanTableSpan(rowspanRaw, rowspanSet)
			if err != nil {
				return nil, nil, htmlPlanUnsupported(cellToken.Str, cellIndex, "invalid rowspan")
			}
			style := compiled.unifiedResolved[cellIndex].text
			if cellToken.Str == "th" {
				style.Bold = true
			}
			decl := compiled.unifiedResolved[cellIndex].decl
			cellContentWidth := availableWidth
			if colspan == 1 {
				if width, set, widthErr := htmlPlanImageDimension(firstNonEmpty(decl["width"], cellToken.Attr["width"]), pointsToUnits, availableWidth); widthErr == nil && set {
					cellContentWidth = width
				}
			}
			blocks, err := htmlPlanTableCellBlocks(ctx, compiled, cellIndex, cellEnd, lineHeight, textBytes, pointsToUnits, style, state, cellContentWidth)
			if err != nil {
				return nil, nil, err
			}
			if len(blocks) == 0 {
				return nil, nil, htmlPlanUnsupported(cellToken.Str, cellIndex, "empty cells are unsupported")
			}
			cell := layout.TableCell{Header: cellToken.Str == "th", ColSpan: colspan, RowSpan: rowspan, Blocks: blocks, Style: style}
			cell.Box.KeepTogether = compiled.unifiedResolved[cellIndex].box.KeepTogether
			switch strings.ToLower(strings.TrimSpace(cellToken.Attr["align"])) {
			case "", "left":
			case "center", "middle":
				cell.Align = "center"
			case "right":
				cell.Align = "right"
			default:
				return nil, nil, htmlPlanUnsupported(cellToken.Str, cellIndex, "align supports left, center, middle, or right")
			}
			switch strings.ToLower(strings.TrimSpace(cellToken.Attr["valign"])) {
			case "", "top":
			case "middle":
				cell.VerticalAlign = "middle"
			case "bottom":
				cell.VerticalAlign = "bottom"
			default:
				return nil, nil, htmlPlanUnsupported(cellToken.Str, cellIndex, "valign supports top, middle, or bottom")
			}
			if defaults.padding > 0 {
				cell.Box.Padding = layout.Spacing{Top: defaults.padding, Right: defaults.padding, Bottom: defaults.padding, Left: defaults.padding}
			}
			if defaults.border.Width > 0 {
				cell.Box.Border = layout.BorderStyle{Top: defaults.border, Right: defaults.border, Bottom: defaults.border, Left: defaults.border}
			}
			for columnIndex < len(active) && active[columnIndex] > 0 {
				columnIndex++
			}
			hint := htmlPlanTableColumnHint{column: columnIndex, token: cellIndex}
			for _, dimension := range []struct {
				value  string
				target *float64
				set    *bool
			}{
				{firstNonEmpty(decl["width"], cellToken.Attr["width"]), &hint.width, &hint.hasWidth},
				{decl["min-width"], &hint.minimum, &hint.hasMin},
				{decl["max-width"], &hint.maximum, &hint.hasMax},
			} {
				value, set, dimensionErr := htmlPlanImageDimension(dimension.value, pointsToUnits, availableWidth)
				if dimensionErr != nil {
					return nil, nil, htmlPlanUnsupported(cellToken.Str, cellIndex, "column width "+dimensionErr.Error())
				}
				*dimension.target, *dimension.set = value, set
			}
			if (hint.hasWidth || hint.hasMin || hint.hasMax) && colspan != 1 {
				return nil, nil, htmlPlanUnsupported(cellToken.Str, cellIndex, "width constraints on spanning cells are ambiguous")
			}
			if hint.hasWidth || hint.hasMin || hint.hasMax {
				hints = append(hints, hint)
			}
			if decl := htmlUnifiedFilteredDeclarations(compiled.unifiedResolved[cellIndex].decl, htmlUnifiedTableBoxProperties); len(decl) != 0 {
				if err := htmlPlanApplyStrictCellStyle(&cell, htmlUnifiedDeclarationString(decl), pointsToUnits); err != nil {
					return nil, nil, htmlPlanUnsupported(cellToken.Str, cellIndex, err.Error())
				}
			}
			row.Cells = append(row.Cells, cell)
			for len(active) < columnIndex+colspan {
				active = append(active, 0)
			}
			for slot := columnIndex; slot < columnIndex+colspan; slot++ {
				active[slot] = rowspan
			}
			columnIndex += colspan
			cellIndex = cellEnd + 1
		}
		if len(row.Cells) == 0 {
			return nil, nil, htmlPlanUnsupported("tr", index, "empty rows are unsupported")
		}
		rows = append(rows, row)
		state.tableRows++
		if state.tableRows > htmlDefaultMaxTableRows {
			return nil, nil, ErrHTMLLimitExceeded
		}
		for slot := range active {
			if active[slot] > 0 {
				active[slot]--
			}
		}
		index = rowEnd + 1
	}
	if len(rows) == 0 {
		return nil, nil, htmlPlanUnsupported(compiled.tokens[start].Str, start, "empty table sections are unsupported")
	}
	return rows, hints, nil
}

func htmlPlanTableCellBlocks(ctx context.Context, compiled *CompiledHTML, start, end int, lineHeight float64, textBytes *int, pointsToUnits func(float64) float64, inherited layout.TextStyle, state *htmlPlanLoweringState, availableWidth float64) ([]layout.Block, error) {
	cellNode := compiled.tokenNode[start]
	hasBlock := false
	for index := start + 1; index < end; index++ {
		token := compiled.tokens[index]
		if token.Cat != 'O' {
			continue
		}
		node := compiled.tokenNode[index]
		if node < 0 || compiled.nodeIndexes[node].Parent != cellNode {
			continue
		}
		switch token.Str {
		case "p", "h1", "h2", "h3", "h4", "h5", "h6", "ul", "ol", "dl", "div", "section", "article", "main", "img", "figure", "table":
			hasBlock = true
		}
	}
	if !hasBlock {
		segments, err := htmlPlanInlineSegments(ctx, compiled, start+1, end, textBytes)
		if err != nil {
			return nil, err
		}
		if len(segments) == 0 {
			return nil, nil
		}
		return []layout.Block{layout.ParagraphBlock{Segments: segments, Style: inherited}}, nil
	}
	blocks, err := lowerHTMLPlanBlockRangeWidthState(ctx, compiled, start+1, end, lineHeight, textBytes, 1, pointsToUnits, availableWidth, state)
	if err != nil {
		return nil, err
	}
	for index, block := range blocks {
		switch value := block.(type) {
		case layout.ParagraphBlock:
			value.Style = layout.MergedTextStyle(inherited, value.Style)
			blocks[index] = value
		case layout.HeadingBlock:
			value.Style = layout.MergedTextStyle(inherited, value.Style)
			blocks[index] = value
		}
	}
	return blocks, nil
}

func htmlPlanTableSpan(raw string, set bool) (int, error) {
	if !set {
		return 1, nil
	}
	if raw == "" || strings.TrimSpace(raw) != raw || raw[0] == '0' {
		return 0, errors.New("non-canonical span")
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > htmlUnifiedMaxTableColumns {
		return 0, errors.New("span out of range")
	}
	return n, nil
}

func htmlPlanStrictTableDeclarations(raw string, allowed map[string]bool) (map[string]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, errors.New("style is empty")
	}
	result := make(map[string]string)
	for _, part := range strings.Split(raw, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, value, ok := strings.Cut(part, ":")
		name = strings.ToLower(strings.TrimSpace(name))
		value = strings.TrimSpace(value)
		if !ok || name == "" || value == "" || !allowed[name] {
			return nil, fmt.Errorf("style property %q is outside the strict table decoration cohort", name)
		}
		if _, exists := result[name]; exists {
			return nil, fmt.Errorf("style property %q is duplicated", name)
		}
		result[name] = value
	}
	if len(result) == 0 {
		return nil, errors.New("style has no declarations")
	}
	return result, nil
}

func htmlPlanApplyStrictCellStyle(cell *layout.TableCell, raw string, pointsToUnits func(float64) float64) error {
	allowed := map[string]bool{"background-color": true, "padding": true, "padding-top": true, "padding-right": true, "padding-bottom": true, "padding-left": true, "border": true, "text-align": true, "vertical-align": true,
		"width": true, "min-width": true, "max-width": true}
	for _, side := range []string{"top", "right", "bottom", "left"} {
		allowed["border-"+side] = true
		allowed["border-"+side+"-width"] = true
		allowed["border-"+side+"-style"] = true
		allowed["border-"+side+"-color"] = true
	}
	decl, err := htmlPlanStrictTableDeclarations(raw, allowed)
	if err != nil {
		return err
	}
	if value := decl["background-color"]; value != "" {
		color, ok := parseCSSColor(value)
		if !ok {
			return errors.New("background-color is invalid")
		}
		cell.Box.BackgroundColor = layout.DocumentColor{R: color.R, G: color.G, B: color.B, Set: true}
	}
	if value := decl["padding"]; value != "" {
		length, err := htmlPlanStrictTableLength(value, pointsToUnits)
		if err != nil {
			return fmt.Errorf("padding %w", err)
		}
		cell.Box.Padding = layout.Spacing{Top: length, Right: length, Bottom: length, Left: length}
	}
	for _, side := range []struct {
		name   string
		target *float64
	}{{"padding-top", &cell.Box.Padding.Top}, {"padding-right", &cell.Box.Padding.Right}, {"padding-bottom", &cell.Box.Padding.Bottom}, {"padding-left", &cell.Box.Padding.Left}} {
		if value := decl[side.name]; value != "" {
			length, err := htmlPlanStrictTableLength(value, pointsToUnits)
			if err != nil {
				return fmt.Errorf("%s %w", side.name, err)
			}
			*side.target = length
		}
	}
	if value := decl["border"]; value != "" {
		side, err := htmlPlanStrictBorderSide(value, pointsToUnits)
		if err != nil {
			return fmt.Errorf("border %w", err)
		}
		cell.Box.Border = layout.BorderStyle{Top: side, Right: side, Bottom: side, Left: side}
	}
	sides := []struct {
		name string
		side *layout.BorderSide
	}{{"top", &cell.Box.Border.Top}, {"right", &cell.Box.Border.Right}, {"bottom", &cell.Box.Border.Bottom}, {"left", &cell.Box.Border.Left}}
	for _, entry := range sides {
		if value := decl["border-"+entry.name]; value != "" {
			side, err := htmlPlanStrictBorderSide(value, pointsToUnits)
			if err != nil {
				return fmt.Errorf("border-%s %w", entry.name, err)
			}
			*entry.side = side
		}
		if value := decl["border-"+entry.name+"-width"]; value != "" {
			width, err := htmlPlanStrictTableLength(value, pointsToUnits)
			if err != nil {
				return fmt.Errorf("border-%s-width %w", entry.name, err)
			}
			entry.side.Width = width
		}
		if value := strings.ToLower(decl["border-"+entry.name+"-style"]); value != "" {
			if value != "solid" && value != "none" {
				return fmt.Errorf("border-%s-style supports solid or none", entry.name)
			}
			entry.side.Style = value
		}
		if value := decl["border-"+entry.name+"-color"]; value != "" {
			color, ok := parseCSSColor(value)
			if !ok {
				return fmt.Errorf("border-%s-color is invalid", entry.name)
			}
			entry.side.Color = layout.DocumentColor{R: color.R, G: color.G, B: color.B, Set: true}
		}
		if entry.side.Style == "none" {
			*entry.side = layout.BorderSide{}
			continue
		}
		if entry.side.Width > 0 {
			if entry.side.Style == "" {
				entry.side.Style = "solid"
			}
			if !entry.side.Color.Set {
				entry.side.Color = layout.DocumentColor{Set: true}
			}
		} else if entry.side.Style != "" || entry.side.Color.Set {
			return fmt.Errorf("border-%s needs a positive width", entry.name)
		}
	}
	if value := strings.ToLower(decl["text-align"]); value != "" {
		switch value {
		case "left":
			cell.Align = "left"
		case "center":
			cell.Align = "center"
		case "right":
			cell.Align = "right"
		default:
			return errors.New("text-align supports left, center, or right")
		}
	}
	if value := strings.ToLower(decl["vertical-align"]); value != "" {
		switch value {
		case "top", "middle", "bottom":
			cell.VerticalAlign = value
		default:
			return errors.New("vertical-align supports top, middle, or bottom")
		}
	}
	return nil
}

func htmlPlanStrictBorderSide(raw string, pointsToUnits func(float64) float64) (layout.BorderSide, error) {
	if strings.ToLower(strings.TrimSpace(raw)) == "none" {
		return layout.BorderSide{}, nil
	}
	parts := strings.Fields(raw)
	if len(parts) != 3 || strings.ToLower(parts[1]) != "solid" {
		return layout.BorderSide{}, errors.New("requires WIDTH solid COLOR or none")
	}
	width, err := htmlPlanStrictTableLength(parts[0], pointsToUnits)
	if err != nil || width <= 0 {
		return layout.BorderSide{}, errors.New("width must be positive pt/px")
	}
	color, ok := parseCSSColor(parts[2])
	if !ok {
		return layout.BorderSide{}, errors.New("color is invalid")
	}
	return layout.BorderSide{Width: width, Style: "solid", Color: layout.DocumentColor{R: color.R, G: color.G, B: color.B, Set: true}}, nil
}

func htmlPlanStrictTableLength(raw string, pointsToUnits func(float64) float64) (float64, error) {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "0" {
		return 0, nil
	}
	scale := 1.0
	switch {
	case strings.HasSuffix(raw, "pt"):
		raw = strings.TrimSuffix(raw, "pt")
	case strings.HasSuffix(raw, "px"):
		raw = strings.TrimSuffix(raw, "px")
		scale = .75
	default:
		return 0, errors.New("must use pt or px")
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1000 {
		return 0, errors.New("is outside the bounded non-negative range")
	}
	return pointsToUnits(value * scale), nil
}

func htmlPlanTableColumnCount(rows []layout.TableRow) (int, error) {
	max := 0
	active := make([]int, 0)
	for _, row := range rows {
		column := 0
		for _, cell := range row.Cells {
			for column < len(active) && active[column] > 0 {
				column++
			}
			span := cell.ColSpan
			if span < 1 {
				span = 1
			}
			rowSpan := cell.RowSpan
			if rowSpan < 1 {
				rowSpan = 1
			}
			if column+span > htmlUnifiedMaxTableColumns {
				return 0, errors.New("table column count is outside limits")
			}
			for len(active) < column+span {
				active = append(active, 0)
			}
			for slot := column; slot < column+span; slot++ {
				if active[slot] > 0 {
					return 0, errors.New("table spans overlap")
				}
				active[slot] = rowSpan
			}
			column += span
		}
		if len(active) > max {
			max = len(active)
		}
		for slot := range active {
			if active[slot] > 0 {
				active[slot]--
			}
		}
	}
	for _, remaining := range active {
		if remaining > 0 {
			return 0, errors.New("rowspan extends outside its table section")
		}
	}
	if max == 0 || max > htmlUnifiedMaxTableColumns {
		return 0, errors.New("table column count is outside limits")
	}
	if _, err := typedTablePlacements(rows, max, 0, "html.table"); err != nil {
		return 0, fmt.Errorf("table grid is not rectangular: %w", err)
	}
	return max, nil
}

func htmlPlanFigure(ctx context.Context, compiled *CompiledHTML, start, end int, lineHeight float64, textBytes *int, pointsToUnits func(float64) float64, availableWidth float64) (layout.ImageBlock, error) {
	var image layout.ImageBlock
	haveImage := false
	for index := start + 1; index < end; {
		if err := ctx.Err(); err != nil {
			return layout.ImageBlock{}, err
		}
		token := compiled.tokens[index]
		if token.Cat == 'T' && collapseHTMLPlanText(token.Str) == "" {
			index++
			continue
		}
		if token.Cat != 'O' {
			return layout.ImageBlock{}, htmlPlanUnsupported(token.Str, index, "figure requires one direct image and an optional figcaption")
		}
		switch token.Str {
		case "img":
			if haveImage {
				return layout.ImageBlock{}, htmlPlanUnsupported("img", index, "figure contains more than one image")
			}
			candidate, err := htmlPlanImage(compiled, index, pointsToUnits, availableWidth)
			if err != nil {
				return layout.ImageBlock{}, err
			}
			plannedImage, ok := candidate.(layout.ImageBlock)
			if !ok {
				return layout.ImageBlock{}, htmlPlanUnsupported("img", index, "figure image source cannot use text fallback")
			}
			image = plannedImage
			image.Box.KeepTogether = true
			haveImage = true
			index++
		case "figcaption":
			if !haveImage || len(image.Caption) != 0 || len(token.Attr) != 0 {
				return layout.ImageBlock{}, htmlPlanUnsupported("figcaption", index, "caption must be one attribute-free direct child after the figure image")
			}
			captionEnd, err := htmlPlanElementEnd(compiled, index)
			if err != nil || captionEnd > end {
				if err != nil {
					return layout.ImageBlock{}, err
				}
				return layout.ImageBlock{}, htmlPlanUnsupported("figcaption", index, "caption exceeds its figure")
			}
			segments, err := htmlPlanInlineSegments(ctx, compiled, index+1, captionEnd, textBytes)
			if err != nil {
				return layout.ImageBlock{}, err
			}
			if len(segments) == 0 {
				return layout.ImageBlock{}, htmlPlanUnsupported("figcaption", index, "caption is empty")
			}
			style := compiled.unifiedResolved[index].text
			style.Italic = true
			if style.Align == "" || style.Align == "L" {
				style.Align = "C"
			}
			if style.LineHeight <= 0 {
				style.LineHeight = lineHeight
			}
			if htmlUnifiedVisualBox(compiled.unifiedResolved[index].box) {
				return layout.ImageBlock{}, htmlPlanUnsupported("figcaption", index, "caption box decoration is outside the unified figure cohort")
			}
			image.Caption = segments
			image.CaptionStyle = style
			index = captionEnd + 1
		default:
			return layout.ImageBlock{}, htmlPlanUnsupported(token.Str, index, "element is not a direct figure image or caption")
		}
	}
	if !haveImage {
		return layout.ImageBlock{}, htmlPlanUnsupported("figure", start, "figure has no image")
	}
	return image, nil
}

func htmlPlanImage(compiled *CompiledHTML, index int, pointsToUnits func(float64) float64, availableWidth float64) (layout.Block, error) {
	token := compiled.tokens[index]
	for name := range token.Attr {
		switch name {
		case "src", "alt", "width", "height", "align":
		default:
			return nil, htmlPlanUnsupported("img", index, fmt.Sprintf("attribute %q is outside the unified image cohort", name))
		}
	}
	if strings.TrimSpace(token.Attr["src"]) == "" {
		alt := strings.TrimSpace(token.Attr["alt"])
		if alt == "" {
			return nil, htmlPlanUnsupported("img", index, "a source or non-empty alt fallback is required")
		}
		style := compiled.unifiedResolved[index].text
		return layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: alt}}, Style: style}, nil
	}
	source, ok := compiled.dataImage(index)
	if !ok {
		return nil, htmlPlanUnsupported("img", index, "bounded PNG/JPEG source snapshot is unavailable")
	}
	if pointsToUnits == nil {
		pointsToUnits = func(value float64) float64 { return value }
	}
	intrinsicWidth, intrinsicHeight, err := htmlPlanImageIntrinsicSize(source.data, pointsToUnits)
	if err != nil {
		return nil, htmlPlanUnsupported("img", index, err.Error())
	}
	format := strings.ToLower(strings.TrimSpace(source.options.ImageType))
	if format != "png" && format != "jpg" && format != "jpeg" {
		return nil, htmlPlanUnsupported("img", index, "compiled image format is unsupported")
	}
	decl := compiled.unifiedResolved[index].decl
	widthValue := firstNonEmpty(strings.TrimSpace(decl["width"]), token.Attr["width"])
	heightValue := firstNonEmpty(strings.TrimSpace(decl["height"]), token.Attr["height"])
	width, hasWidth, err := htmlPlanImageDimension(widthValue, pointsToUnits, availableWidth)
	if err != nil {
		return nil, htmlPlanUnsupported("img", index, "width "+err.Error())
	}
	height, hasHeight, err := htmlPlanImageDimension(heightValue, pointsToUnits, 0)
	if err != nil {
		return nil, htmlPlanUnsupported("img", index, "height "+err.Error())
	}
	switch {
	case !hasWidth && !hasHeight:
		width, height = intrinsicWidth, intrinsicHeight
	case hasWidth && !hasHeight:
		height = width * intrinsicHeight / intrinsicWidth
	case !hasWidth && hasHeight:
		width = height * intrinsicWidth / intrinsicHeight
	}
	maxWidth, hasMaxWidth, err := htmlPlanImageDimension(decl["max-width"], pointsToUnits, availableWidth)
	if err != nil {
		return nil, htmlPlanUnsupported("img", index, "max-width "+err.Error())
	}
	maxHeight, hasMaxHeight, err := htmlPlanImageDimension(decl["max-height"], pointsToUnits, 0)
	if err != nil {
		return nil, htmlPlanUnsupported("img", index, "max-height "+err.Error())
	}
	scale := 1.0
	if hasMaxWidth && width > maxWidth {
		scale = maxWidth / width
	}
	if hasMaxHeight && height*scale > maxHeight {
		scale = maxHeight / height
	}
	width, height = width*scale, height*scale
	fit := layout.ImageFitAuto
	switch strings.ToLower(strings.TrimSpace(decl["object-fit"])) {
	case "", "fill":
	case "contain":
		fit = layout.ImageFitContain
	case "cover":
		fit = layout.ImageFitCover
	default:
		return nil, htmlPlanUnsupported("img", index, fmt.Sprintf("object-fit %q is unsupported", decl["object-fit"]))
	}
	align := strings.ToLower(strings.TrimSpace(firstNonEmpty(decl["text-align"], token.Attr["align"])))
	switch align {
	case "", "left":
		align = "left"
	case "center", "middle":
		align = "center"
	case "right":
	default:
		return nil, htmlPlanUnsupported("img", index, fmt.Sprintf("alignment %q is unsupported", align))
	}
	return layout.ImageBlock{
		Data: append([]byte(nil), source.data...), Format: format,
		Alt: token.Attr["alt"], Width: width, Height: height,
		Fit: fit, Align: align,
	}, nil
}

func htmlPlanBlockBreakPolicy(compiled *CompiledHTML, index int) (bool, bool, error) {
	token := compiled.tokens[index]
	declarations := compiled.unifiedResolved[index].decl
	if len(declarations) == 0 {
		if token.Str == "ol" || token.Str == "ul" {
			return false, false, nil
		}
		if len(token.Attr) != 0 {
			return false, false, htmlPlanUnsupported(token.Str, index, "block attributes are outside the unified cohort")
		}
		return false, false, nil
	}
	allowed := map[string]bool{"break-before": true, "page-break-before": true, "break-after": true, "page-break-after": true, "break-inside": true, "page-break-inside": true}
	for name := range htmlUnifiedTextProperties {
		allowed[name] = true
	}
	allowed["display"] = true
	for name := range htmlUnifiedFlowProperties {
		allowed[name] = true
	}
	if htmlUnifiedBlockLikeTag(token.Str, declarations) {
		for name := range htmlUnifiedBlockBoxProperties {
			allowed[name] = true
		}
	}
	if declarations := compiled.unifiedResolved[index].flexContainer; declarations != nil {
		for name := range htmlUnifiedFlexContainerProperties {
			allowed[name] = true
		}
	}
	if token.Str == "ol" || token.Str == "ul" {
		allowed["list-style"] = true
		allowed["list-style-type"] = true
	}
	for name, value := range declarations {
		if !allowed[name] {
			return false, false, htmlPlanUnsupported(token.Str, index, fmt.Sprintf("style property %q is outside the initial page-break cohort", name))
		}
		if name == "break-before" || name == "page-break-before" || name == "break-after" || name == "page-break-after" {
			switch strings.ToLower(strings.TrimSpace(value)) {
			case "auto", "avoid", "always", "page", "left", "right":
			default:
				return false, false, htmlPlanUnsupported(token.Str, index, fmt.Sprintf("style property %q has unsupported value %q", name, value))
			}
		}
	}
	before := htmlBreakForcesPage(declarations["break-before"]) || htmlBreakForcesPage(declarations["page-break-before"])
	after := htmlBreakForcesPage(declarations["break-after"]) || htmlBreakForcesPage(declarations["page-break-after"])
	return before, after, nil
}

func htmlPlanParagraph(text string, lineHeight float64) layout.ParagraphBlock {
	return layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: text}}, Style: layout.TextStyle{LineHeight: lineHeight}}
}

func htmlPlanParagraphSegments(segments []layout.TextSegment, lineHeight float64) layout.ParagraphBlock {
	return layout.ParagraphBlock{Segments: append([]layout.TextSegment(nil), segments...), Style: layout.TextStyle{LineHeight: lineHeight}}
}

func htmlPlanElementEnd(compiled *CompiledHTML, start int) (int, error) {
	if start < 0 || start >= len(compiled.elementEnd) {
		return 0, htmlPlanUnsupported("element", start, "element boundary is unavailable")
	}
	end := compiled.elementEnd[start]
	if end <= start || end >= len(compiled.tokens) || compiled.tokens[end].Cat != 'C' || compiled.tokens[end].Str != compiled.tokens[start].Str {
		return 0, htmlPlanUnsupported(compiled.tokens[start].Str, start, "element boundary is not exact")
	}
	return end, nil
}

func htmlPlanInlineSegments(ctx context.Context, compiled *CompiledHTML, start, end int, textBytes *int) ([]layout.TextSegment, error) {
	tokens := compiled.tokens
	baseStyle := layout.TextStyle{}
	if start > 0 && start-1 < len(compiled.unifiedResolved) {
		baseStyle = compiled.unifiedResolved[start-1].text
	}
	basePreserve, basePreserveLines := false, false
	if start > 0 && start-1 < len(compiled.unifiedResolved) {
		basePreserve = compiled.unifiedResolved[start-1].preserveWS
		basePreserveLines = compiled.unifiedResolved[start-1].preserveLines
	}
	destination := ""
	baseTransform := "none"
	if start > 0 && start-1 < len(compiled.unifiedResolved) {
		destination = compiled.unifiedResolved[start-1].destination
		baseTransform = compiled.unifiedResolved[start-1].textTransform
	}
	if strings.TrimSpace(baseTransform) == "" {
		baseTransform = "none"
	}
	out := htmlPlanSegmentBuilder{style: baseStyle, baseStyle: baseStyle, transform: baseTransform, baseTransform: baseTransform, destination: destination, preserve: basePreserve, preserveLines: basePreserveLines, atLineStart: true}
	type inlineFrame struct {
		tag                   string
		previousLink          string
		previousStyle         layout.TextStyle
		previousTransform     string
		previousPreserve      bool
		previousPreserveLines bool
	}
	stack := make([]inlineFrame, 0, 8)
	for index := start; index < end; index++ {
		if index&255 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		token := tokens[index]
		switch token.Cat {
		case 'T':
			out.append(token.Str)
			*textBytes += len(token.Str)
			if *textBytes > htmlUnifiedMaxTextBytes {
				return nil, ErrHTMLLimitExceeded
			}
		case 'O':
			switch token.Str {
			case "span", "strong", "b", "em", "i", "cite", "var", "code", "kbd", "samp", "tt":
				if compiled.unifiedResolved[index].displayNone {
					end, boundaryErr := htmlPlanElementEnd(compiled, index)
					if boundaryErr != nil {
						return nil, boundaryErr
					}
					index = end
					continue
				}
				if len(token.Attr) != 0 {
					return nil, htmlPlanUnsupported(token.Str, index, "inline attributes are outside the unified cohort after resolved CSS lowering")
				}
				stack = append(stack, inlineFrame{tag: token.Str, previousLink: out.link, previousStyle: out.style, previousTransform: out.transform, previousPreserve: out.preserve, previousPreserveLines: out.preserveLines})
				if name := compiled.unifiedResolved[index].destination; name != "" {
					out.setDestination(name)
				}
				out.setStyle(compiled.unifiedResolved[index].text)
				out.setTransform(compiled.unifiedResolved[index].textTransform)
				out.setWhitespace(compiled.unifiedResolved[index].preserveWS, compiled.unifiedResolved[index].preserveLines)
			case "a":
				if compiled.unifiedResolved[index].displayNone {
					end, boundaryErr := htmlPlanElementEnd(compiled, index)
					if boundaryErr != nil {
						return nil, boundaryErr
					}
					index = end
					continue
				}
				if out.link != "" {
					return nil, htmlPlanUnsupported(token.Str, index, "nested links are not allowed")
				}
				href, exists := token.Attr["href"]
				if !exists || len(token.Attr) != 1 || strings.TrimSpace(href) != href || href == "" {
					return nil, htmlPlanUnsupported(token.Str, index, "links require exactly one canonical href attribute")
				}
				stack = append(stack, inlineFrame{tag: token.Str, previousLink: out.link, previousStyle: out.style, previousTransform: out.transform, previousPreserve: out.preserve, previousPreserveLines: out.preserveLines})
				out.setLink(href)
				if name := compiled.unifiedResolved[index].destination; name != "" {
					out.setDestination(name)
				}
				out.setStyle(compiled.unifiedResolved[index].text)
				out.setTransform(compiled.unifiedResolved[index].textTransform)
				out.setWhitespace(compiled.unifiedResolved[index].preserveWS, compiled.unifiedResolved[index].preserveLines)
			case "br":
				if len(token.Attr) != 0 {
					return nil, htmlPlanUnsupported(token.Str, index, "br attributes are not in the initial unified cohort")
				}
				out.lineBreak()
			default:
				return nil, htmlPlanUnsupported(token.Str, index, "inline element is not in the initial unified cohort")
			}
		case 'C':
			if token.Str != "span" && token.Str != "a" && token.Str != "strong" && token.Str != "b" && token.Str != "em" && token.Str != "i" && token.Str != "cite" && token.Str != "var" && token.Str != "code" && token.Str != "kbd" && token.Str != "samp" && token.Str != "tt" {
				return nil, htmlPlanUnsupported(token.Str, index, "inline closing element is not in the initial unified cohort")
			}
			if len(stack) == 0 || stack[len(stack)-1].tag != token.Str {
				return nil, htmlPlanUnsupported(token.Str, index, "inline nesting is unbalanced")
			}
			frame := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			out.setLink(frame.previousLink)
			out.setStyle(frame.previousStyle)
			out.setTransform(frame.previousTransform)
			out.setWhitespace(frame.previousPreserve, frame.previousPreserveLines)
		}
	}
	if len(stack) != 0 {
		return nil, htmlPlanUnsupported("inline", start, "inline nesting is unbalanced")
	}
	return htmlPlanExpandSegmentTabs(out.finish(), baseStyle), nil
}

func htmlPlanExpandSegmentTabs(segments []layout.TextSegment, base layout.TextStyle) []layout.TextSegment {
	column := 0
	for index := range segments {
		style := layout.MergedTextStyle(base, segments[index].EffectiveStyle())
		tab := int(style.TabSize)
		if tab == 0 {
			tab = 8
		}
		var expanded strings.Builder
		for _, r := range segments[index].Text {
			switch r {
			case '\n':
				expanded.WriteRune(r)
				column = 0
			case '\t':
				spaces := tab - column%tab
				expanded.WriteString(strings.Repeat(" ", spaces))
				column += spaces
			default:
				expanded.WriteRune(r)
				column++
			}
		}
		segments[index].Text = expanded.String()
	}
	return segments
}

func htmlPlanList(ctx context.Context, compiled *CompiledHTML, start, end int, lineHeight float64, textBytes *int) (layout.ListBlock, error) {
	ordered := compiled.tokens[start].Str == "ol"
	resolvedList := compiled.unifiedResolved[start]
	list := layout.ListBlock{Ordered: ordered, Style: resolvedList.text, Box: resolvedList.box}
	for name, value := range compiled.tokens[start].Attr {
		switch name {
		case "start":
			if !ordered {
				return layout.ListBlock{}, htmlPlanUnsupported("ul", start, "start is valid only on ordered lists")
			}
			parsed, err := strconv.Atoi(value)
			if err != nil || strconv.Itoa(parsed) != value || parsed < -9999 || parsed > 9999 {
				return layout.ListBlock{}, htmlPlanUnsupported("ol", start, "start must be a canonical bounded integer")
			}
			list.Start = parsed
		case "type":
			if !ordered {
				return layout.ListBlock{}, htmlPlanUnsupported("ul", start, "unordered marker type is outside the exact ASCII cohort")
			}
			styles := map[string]string{"1": "decimal", "a": "lower-alpha", "A": "upper-alpha", "i": "lower-roman", "I": "upper-roman"}
			marker, ok := styles[value]
			if !ok {
				return layout.ListBlock{}, htmlPlanUnsupported("ol", start, "type must be 1, a, A, i, or I")
			}
			list.MarkerStyle = marker
		default:
			return layout.ListBlock{}, htmlPlanUnsupported(compiled.tokens[start].Str, start, "list attributes are outside the bounded counter cohort")
		}
	}
	if list.Style.LineHeight <= 0 {
		list.Style.LineHeight = lineHeight
	}
	if cssMarker := strings.ToLower(strings.TrimSpace(firstNonEmpty(resolvedList.decl["list-style-type"], resolvedList.decl["list-style"]))); cssMarker != "" {
		allowed := map[string]bool{"decimal": true, "lower-alpha": true, "upper-alpha": true, "lower-roman": true, "upper-roman": true, "none": true, "disc": !ordered}
		if !allowed[cssMarker] || cssMarker == "disc" {
			return layout.ListBlock{}, htmlPlanUnsupported(compiled.tokens[start].Str, start, fmt.Sprintf("resolved list marker %q is outside the exact ASCII cohort", cssMarker))
		}
		list.MarkerStyle = cssMarker
	} else if list.MarkerStyle != "" {
		// The canonical HTML type attribute already selected the marker.
	} else if ordered {
		list.MarkerStyle = "decimal"
	} else {
		// The exact typed planner deliberately supports an ASCII marker cohort.
		list.MarkerStyle = "dash"
	}
	for index := start + 1; index < end; {
		token := compiled.tokens[index]
		if token.Cat == 'T' && collapseHTMLPlanText(token.Str) == "" {
			index++
			continue
		}
		if token.Cat != 'O' || token.Str != "li" {
			return layout.ListBlock{}, htmlPlanUnsupported(token.Str, index, "lists require direct li children")
		}
		itemEnd, err := htmlPlanElementEnd(compiled, index)
		if err != nil || itemEnd > end {
			if err != nil {
				return layout.ListBlock{}, err
			}
			return layout.ListBlock{}, htmlPlanUnsupported("li", index, "list item exceeds its list")
		}
		item := layout.ListItem{}
		for name, raw := range token.Attr {
			if name != "value" || !ordered {
				return layout.ListBlock{}, htmlPlanUnsupported("li", index, "only ordered-list items accept value")
			}
			value, valueErr := strconv.Atoi(raw)
			if valueErr != nil || strconv.Itoa(value) != raw || value < -9999 || value > 9999 {
				return layout.ListBlock{}, htmlPlanUnsupported("li", index, "value must be a canonical bounded integer")
			}
			item.Value, item.ValueSet = value, true
		}
		blocks, err := htmlPlanStructuredListEntry(ctx, compiled, index, itemEnd, lineHeight, textBytes)
		if err != nil {
			return layout.ListBlock{}, err
		}
		if len(blocks) == 0 {
			return layout.ListBlock{}, htmlPlanUnsupported("li", index, "empty list items are not plannable")
		}
		if _, nestedFirst := blocks[0].(layout.ListBlock); nestedFirst {
			return layout.ListBlock{}, htmlPlanUnsupported("li", index, "a nested list requires leading item text")
		}
		item.Blocks = blocks
		list.Items = append(list.Items, item)
		index = itemEnd + 1
	}
	return list, nil
}

func htmlPlanStructuredListEntry(ctx context.Context, compiled *CompiledHTML, start, end int, lineHeight float64, textBytes *int) ([]layout.Block, error) {
	parent := compiled.tokenNode[start]
	blocks := make([]layout.Block, 0, 2)
	decl := compiled.unifiedResolved[start].decl
	if htmlBreakForcesPage(decl["break-before"]) || htmlBreakForcesPage(decl["page-break-before"]) || htmlBreakForcesPage(decl["break-after"]) || htmlBreakForcesPage(decl["page-break-after"]) {
		return nil, htmlPlanUnsupported(compiled.tokens[start].Str, start, "forced breaks inside list entries are outside the atomic nested-list cohort")
	}
	for cursor := start + 1; cursor < end; {
		next := end
		for index := cursor; index < end; index++ {
			if compiled.tokens[index].Cat != 'O' {
				continue
			}
			node := compiled.tokenNode[index]
			if node >= 0 && compiled.nodeIndexes[node].Parent == parent {
				switch compiled.tokens[index].Str {
				case "ul", "ol", "dl", "p":
					next = index
				}
				if next != end {
					break
				}
			}
		}
		if next > cursor {
			segments, err := htmlPlanInlineSegments(ctx, compiled, cursor, next, textBytes)
			if err != nil {
				return nil, err
			}
			if len(segments) != 0 {
				style := compiled.unifiedResolved[start].text
				if style.LineHeight <= 0 {
					style.LineHeight = lineHeight
				}
				box := layout.BoxStyle{}
				if len(blocks) == 0 {
					box = compiled.unifiedResolved[start].box
				}
				blocks = append(blocks, layout.ParagraphBlock{Segments: segments, Style: style, Box: box})
			}
			cursor = next
			continue
		}
		childEnd, err := htmlPlanElementEnd(compiled, next)
		if err != nil || childEnd > end {
			return nil, htmlPlanUnsupported(compiled.tokens[next].Str, next, "nested list entry exceeds its parent")
		}
		switch compiled.tokens[next].Str {
		case "ul", "ol":
			list, listErr := htmlPlanList(ctx, compiled, next, childEnd, lineHeight, textBytes)
			if listErr != nil {
				return nil, listErr
			}
			blocks = append(blocks, list)
		case "dl":
			definitions, definitionErr := htmlPlanDefinitionList(ctx, compiled, next, childEnd, lineHeight, textBytes)
			if definitionErr != nil {
				return nil, definitionErr
			}
			blocks = append(blocks, definitions...)
		case "p":
			segments, segmentErr := htmlPlanInlineSegments(ctx, compiled, next+1, childEnd, textBytes)
			if segmentErr != nil {
				return nil, segmentErr
			}
			if len(segments) != 0 {
				resolved := compiled.unifiedResolved[next]
				blocks = append(blocks, layout.ParagraphBlock{Segments: segments, Style: resolved.text, Box: resolved.box})
			}
		}
		cursor = childEnd + 1
	}
	return blocks, nil
}

func htmlPlanDefinitionList(ctx context.Context, compiled *CompiledHTML, start, end int, lineHeight float64, textBytes *int) ([]layout.Block, error) {
	blocks := make([]layout.Block, 0)
	haveTerm := false
	for index := start + 1; index < end; {
		token := compiled.tokens[index]
		if token.Cat == 'T' && collapseHTMLPlanText(token.Str) == "" {
			index++
			continue
		}
		if token.Cat != 'O' || (token.Str != "dt" && token.Str != "dd") {
			return nil, htmlPlanUnsupported(token.Str, index, "definition lists require direct dt/dd children")
		}
		childEnd, err := htmlPlanElementEnd(compiled, index)
		if err != nil || childEnd > end {
			if err != nil {
				return nil, err
			}
			return nil, htmlPlanUnsupported(token.Str, index, "definition item exceeds its list")
		}
		before, after, err := htmlPlanBlockBreakPolicy(compiled, index)
		if err != nil {
			return nil, err
		}
		if before {
			blocks = append(blocks, layout.PageBreakBlock{After: true})
		}
		entryBlocks, err := htmlPlanStructuredListEntry(ctx, compiled, index, childEnd, lineHeight, textBytes)
		if err != nil {
			return nil, err
		}
		if len(entryBlocks) == 0 {
			return nil, htmlPlanUnsupported(token.Str, index, "empty definition entries are not plannable")
		}
		if token.Str == "dt" {
			haveTerm = true
			resolved := compiled.unifiedResolved[index]
			box := resolved.box
			box.KeepWithNext = true
			paragraph, ok := entryBlocks[0].(layout.ParagraphBlock)
			if !ok {
				return nil, htmlPlanUnsupported(token.Str, index, "a definition term requires leading inline text")
			}
			blocks = append(blocks, layout.HeadingBlock{Level: 6, Segments: paragraph.Segments, Style: resolved.text, Box: box})
			blocks = append(blocks, entryBlocks[1:]...)
		} else {
			if !haveTerm {
				return nil, htmlPlanUnsupported(token.Str, index, "dd must follow a dt")
			}
			blocks = append(blocks, entryBlocks...)
		}
		if after {
			blocks = append(blocks, layout.PageBreakBlock{After: true})
		}
		index = childEnd + 1
	}
	if !haveTerm {
		return nil, htmlPlanUnsupported("dl", start, "definition list has no terms")
	}
	return blocks, nil
}

func collapseHTMLPlanText(value string) string {
	var out htmlPlanTextBuilder
	out.append(value)
	return strings.TrimSpace(out.String())
}

type htmlPlanTextBuilder struct {
	text         strings.Builder
	pendingSpace bool
}

func (out *htmlPlanTextBuilder) append(value string) {
	for _, r := range value {
		if unicode.IsSpace(r) {
			out.pendingSpace = out.text.Len() != 0
			continue
		}
		if out.pendingSpace {
			out.text.WriteByte(' ')
		}
		out.text.WriteRune(r)
		out.pendingSpace = false
	}
}

func (out *htmlPlanTextBuilder) lineBreak() {
	value := out.text.String()
	if len(value) != 0 && value[len(value)-1] != '\n' {
		out.text.WriteByte('\n')
	}
	out.pendingSpace = false
}

func (out *htmlPlanTextBuilder) String() string { return out.text.String() }

type htmlPlanSegmentBuilder struct {
	segments      []layout.TextSegment
	text          strings.Builder
	link          string
	destination   string
	style         layout.TextStyle
	baseStyle     layout.TextStyle
	transform     string
	baseTransform string
	preserve      bool
	preserveLines bool
	pendingSpace  bool
	hasOutput     bool
	atLineStart   bool
}

func (out *htmlPlanSegmentBuilder) append(value string) {
	value = htmlApplyTextTransform(value, out.transform)
	if out.preserve {
		out.text.WriteString(strings.ReplaceAll(value, "\r\n", "\n"))
		out.hasOutput = out.hasOutput || value != ""
		out.pendingSpace = false
		return
	}
	value = strings.ReplaceAll(strings.ReplaceAll(value, "\r\n", "\n"), "\r", "\n")
	for _, r := range value {
		if out.preserveLines && r == '\n' {
			out.lineBreak()
			continue
		}
		if unicode.IsSpace(r) {
			out.pendingSpace = out.hasOutput && !out.atLineStart
			continue
		}
		if out.pendingSpace {
			out.text.WriteByte(' ')
		}
		out.text.WriteRune(r)
		out.pendingSpace = false
		out.hasOutput = true
		out.atLineStart = false
	}
}

func (out *htmlPlanSegmentBuilder) setWhitespace(preserve, preserveLines bool) {
	if preserve == out.preserve && preserveLines == out.preserveLines {
		return
	}
	out.flush()
	out.preserve = preserve
	out.preserveLines = preserveLines
}

func (out *htmlPlanSegmentBuilder) lineBreak() {
	if !out.hasOutput {
		return
	}
	value := out.text.String()
	if len(value) == 0 || value[len(value)-1] != '\n' {
		out.text.WriteByte('\n')
	}
	out.pendingSpace = false
	out.atLineStart = true
}

func (out *htmlPlanSegmentBuilder) setLink(link string) {
	if link == out.link {
		return
	}
	// Collapsed whitespace belongs to the lexical segment in which it was
	// authored. Materialize it before changing link ownership so an adjacent
	// anchor does not accidentally expand its clickable range.
	if out.pendingSpace && out.hasOutput {
		out.text.WriteByte(' ')
		out.pendingSpace = false
	}
	out.flush()
	out.link = link
}

func (out *htmlPlanSegmentBuilder) setDestination(destination string) {
	if destination == out.destination {
		return
	}
	if out.pendingSpace && out.hasOutput {
		out.text.WriteByte(' ')
		out.pendingSpace = false
	}
	out.flush()
	out.destination = destination
}

func (out *htmlPlanSegmentBuilder) setStyle(style layout.TextStyle) {
	if style == out.style {
		return
	}
	if out.pendingSpace && out.hasOutput {
		out.text.WriteByte(' ')
		out.pendingSpace = false
	}
	out.flush()
	out.style = style
}

func (out *htmlPlanSegmentBuilder) setTransform(transform string) {
	transform = strings.ToLower(strings.TrimSpace(transform))
	if transform == "" {
		transform = "none"
	}
	if transform == out.transform {
		return
	}
	if out.pendingSpace && out.hasOutput {
		out.text.WriteByte(' ')
		out.pendingSpace = false
	}
	out.flush()
	out.transform = transform
}

func (out *htmlPlanSegmentBuilder) flush() {
	if out.text.Len() == 0 {
		return
	}
	style := out.style
	if style == out.baseStyle {
		style = layout.TextStyle{}
	}
	out.segments = append(out.segments, layout.TextSegment{Text: out.text.String(), Style: style, Link: out.link, Destination: out.destination})
	// A named HTML anchor applies to the first finalized glyph range only. Do
	// not repeat the same destination when a later style/link transition
	// flushes the remainder of the element.
	out.destination = ""
	out.text.Reset()
}

func (out *htmlPlanSegmentBuilder) finish() []layout.TextSegment {
	out.flush()
	return append([]layout.TextSegment(nil), out.segments...)
}

// htmlApplyTextTransform implements the small inherited CSS text-transform
// cohort that maps directly to finalized text segments. Transforming before
// whitespace collapsing preserves layout while keeping the authored text
// human-readable in the retained plan.
func htmlApplyTextTransform(value, transform string) string {
	switch strings.ToLower(strings.TrimSpace(transform)) {
	case "uppercase":
		return strings.ToUpper(value)
	case "lowercase":
		return strings.ToLower(value)
	case "capitalize":
		var out strings.Builder
		out.Grow(len(value))
		wordStart := true
		for _, r := range value {
			if unicode.IsLetter(r) {
				if wordStart {
					out.WriteString(strings.ToUpper(string(r)))
				} else {
					out.WriteRune(r)
				}
				wordStart = false
				continue
			}
			out.WriteRune(r)
			wordStart = !unicode.IsDigit(r)
		}
		return out.String()
	default:
		return value
	}
}

func htmlPlanUnsupported(tag string, token int, reason string) error {
	if tag == "" {
		tag = "unknown"
	}
	return fmt.Errorf("%w: token %d <%s>: %s", ErrHTMLPlanUnsupported, token, tag, reason)
}

func htmlPlanHeadingLevel(tag string) (int, bool) {
	if len(tag) != 2 || tag[0] != 'h' || tag[1] < '1' || tag[1] > '6' {
		return 0, false
	}
	return int(tag[1] - '0'), true
}
