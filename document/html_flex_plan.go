// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/cssbruno/gopdfkit/layout"
)

// These properties are accepted by the generic resolved-style scan only so
// the flex cohort can validate them with parent/child context. The lowering
// path receives the typed values below and never selectors or cascade state.
var htmlUnifiedFlexProperties = map[string]bool{
	"display": true, "flex-direction": true, "flex-wrap": true,
	"justify-content": true, "align-items": true, "align-content": true,
	"gap": true, "row-gap": true, "column-gap": true,
	"flex": true, "flex-grow": true, "flex-shrink": true,
	"flex-basis": true, "align-self": true, "order": true,
	"width": true, "height": true, "min-width": true, "min-height": true,
	"max-width": true, "max-height": true,
}

var htmlUnifiedFlexContainerProperties = map[string]bool{
	"display": true, "flex-direction": true, "flex-wrap": true,
	"justify-content": true, "align-items": true, "align-content": true,
	"gap": true, "row-gap": true, "column-gap": true,
	"width": true, "height": true, "min-width": true, "min-height": true,
	"max-width": true, "max-height": true,
}

var htmlUnifiedFlexItemProperties = map[string]bool{
	"flex": true, "flex-grow": true, "flex-shrink": true,
	"flex-basis": true, "align-self": true,
	"width": true, "height": true, "min-width": true, "min-height": true,
	"max-width": true, "max-height": true,
}

type htmlUnifiedFlexContainer struct {
	direction    layout.RowColumnDirection
	gap          float64
	crossGap     float64
	crossSize    float64
	wrap         string
	mainAlign    string
	align        string
	alignContent string
	reverseMain  bool
}

type htmlUnifiedFlexItem struct {
	track           layout.RowColumnTrack
	crossSize       float64
	crossMin        float64
	crossMax        float64
	crossMinPercent uint32
	crossMaxPercent uint32
	align           string
}

// htmlPlanFlexBlock consumes only the selector-free metadata produced by the
// capability scan. Direct children were already proven to be exact element
// boundaries, so lowering cannot accidentally create legacy layout islands.
func htmlPlanFlexBlock(ctx context.Context, compiled *CompiledHTML, token int, lineHeight float64, textBytes *int, pointsToUnits func(float64) float64, state *htmlPlanLoweringState) (layout.RowColumnBlock, error) {
	resolved := compiled.unifiedResolved[token]
	if resolved.flexContainer == nil {
		return layout.RowColumnBlock{}, htmlPlanUnsupported(compiled.tokens[token].Str, token, "resolved flex metadata is unavailable")
	}
	nodeIndex := compiled.tokenNode[token]
	if nodeIndex < 0 || nodeIndex >= len(compiled.nodeIndexes) {
		return layout.RowColumnBlock{}, htmlPlanUnsupported(compiled.tokens[token].Str, token, "flex node boundary is unavailable")
	}
	container := layout.RowColumnBlock{
		Direction: resolved.flexContainer.direction, Gap: resolved.flexContainer.gap,
		CrossGap: resolved.flexContainer.crossGap, CrossSize: resolved.flexContainer.crossSize,
		Wrap: resolved.flexContainer.wrap, MainAlign: resolved.flexContainer.mainAlign,
		CrossAlign: resolved.flexContainer.align, AlignContent: resolved.flexContainer.alignContent, ReverseMain: resolved.flexContainer.reverseMain,
	}
	for childNode := compiled.nodeIndexes[nodeIndex].FirstChild; childNode >= 0; childNode = compiled.nodeIndexes[childNode].NextSibling {
		child := compiled.nodeIndexes[childNode]
		childResolved := compiled.unifiedResolved[child.Token]
		if childResolved.displayNone {
			continue
		}
		if childResolved.flexItem == nil {
			return layout.RowColumnBlock{}, htmlPlanUnsupported(compiled.tokens[child.Token].Str, child.Token, "resolved flex item metadata is unavailable")
		}
		var block layout.Block
		if childResolved.flexContainer != nil {
			nested, err := htmlPlanFlexBlock(ctx, compiled, child.Token, lineHeight, textBytes, pointsToUnits, state)
			if err != nil {
				return layout.RowColumnBlock{}, err
			}
			block = nested
		} else if level, heading := htmlPlanHeadingLevel(compiled.tokens[child.Token].Str); heading {
			segments, err := htmlPlanInlineSegments(ctx, compiled, child.Token+1, child.EndToken, textBytes)
			if err != nil {
				return layout.RowColumnBlock{}, err
			}
			if len(segments) == 0 {
				return layout.RowColumnBlock{}, htmlPlanUnsupported(compiled.tokens[child.Token].Str, child.Token, "flex item has no plannable text")
			}
			block = layout.HeadingBlock{Level: level, Segments: segments, Style: childResolved.text, Box: childResolved.box}
		} else if compiled.tokens[child.Token].Str == "p" {
			segments, err := htmlPlanInlineSegments(ctx, compiled, child.Token+1, child.EndToken, textBytes)
			if err != nil {
				return layout.RowColumnBlock{}, err
			}
			if len(segments) == 0 {
				return layout.RowColumnBlock{}, htmlPlanUnsupported(compiled.tokens[child.Token].Str, child.Token, "flex item has no plannable text")
			}
			block = layout.ParagraphBlock{Segments: segments, Style: childResolved.text, Box: childResolved.box}
		} else {
			blocks, err := lowerHTMLPlanBlockRangeWidthState(ctx, compiled, child.Token+1, child.EndToken, lineHeight, textBytes, 1, pointsToUnits, 0, state)
			if err != nil {
				return layout.RowColumnBlock{}, err
			}
			if len(blocks) != 1 {
				return layout.RowColumnBlock{}, htmlPlanUnsupported(compiled.tokens[child.Token].Str, child.Token, "structured flex item requires exactly one plannable child")
			}
			block = blocks[0]
		}
		container.Items = append(container.Items, layout.RowColumnItem{
			Block: block, Track: childResolved.flexItem.track, CrossSize: childResolved.flexItem.crossSize, CrossAlign: childResolved.flexItem.align,
			CrossMin: childResolved.flexItem.crossMin, CrossMax: childResolved.flexItem.crossMax,
			CrossMinPercent: childResolved.flexItem.crossMinPercent, CrossMaxPercent: childResolved.flexItem.crossMaxPercent,
		})
	}
	return container, nil
}

// htmlUnifiedResolveFlexCohort is the relationship-aware half of the atomic
// capability scan. It runs only after every selector has been resolved and
// before any layout call. The initial cohort deliberately accepts one line,
// direct paragraph/heading items, exact or weighted basis, main-axis gap, and
// cross-axis alignment. Every other flex contract fails the whole fragment.
func htmlUnifiedResolveFlexCohort(ctx context.Context, compiled *CompiledHTML) error {
	if ctx == nil {
		ctx = context.Background()
	}
	for nodeIndex, node := range compiled.nodeIndexes {
		if nodeIndex&255 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		token := node.Token
		decl := compiled.unifiedResolved[token].decl
		if compiled.unifiedResolved[token].displayNone {
			// display:none removes the element from every formatting context;
			// flex declarations on a suppressed subtree are intentionally not
			// interpreted as orphaned container/item properties.
			continue
		}
		display, hasDisplay := decl["display"]
		if !hasDisplay {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(display)) {
		case "flex":
			// Continue through the relationship-aware flex cohort below.
		case "none", "contents", "block", "inline", "inline-block":
			// Ordinary CSS display modes remain in normal flow. Their block
			// geometry is lowered by the HTML text adapter, not by flex.
			continue
		default:
			return htmlPlanUnsupported(compiled.tokens[token].Str, token, fmt.Sprintf("resolved display %q is outside the unified display cohort", display))
		}
		container, err := htmlUnifiedParseFlexContainer(compiled.tokens[token].Str, token, decl)
		if err != nil {
			return err
		}
		compiled.unifiedResolved[token].flexContainer = &container
		if err := htmlUnifiedResolveFlexChildren(ctx, compiled, nodeIndex); err != nil {
			return err
		}
	}

	// Reject orphaned or misplaced flex declarations after all parent-child
	// relationships have been materialized. This also catches properties on a
	// supported prefix before planning can begin.
	for _, node := range compiled.nodeIndexes {
		token := node.Token
		resolved := compiled.unifiedResolved[token]
		if resolved.displayNone {
			continue
		}
		for name := range resolved.decl {
			tag := compiled.tokens[token].Str
			if name == "display" {
				continue
			}
			if name == "width" && tag == "table" || (name == "width" || name == "min-width" || name == "max-width") && (tag == "td" || tag == "th") || htmlUnifiedImageProperties[name] && tag == "img" {
				continue
			}
			if htmlUnifiedBlockBoxTag(tag) && (name == "width" || name == "height" || name == "min-width" || name == "min-height" || name == "max-width" || name == "max-height") && resolved.flexContainer == nil && resolved.flexItem == nil {
				continue
			}
			if htmlUnifiedFlexContainerProperties[name] && resolved.flexContainer == nil && !(htmlUnifiedFlexItemProperties[name] && resolved.flexItem != nil) {
				return htmlPlanUnsupported(compiled.tokens[token].Str, token, fmt.Sprintf("resolved flex container property %q requires display:flex", name))
			}
			if htmlUnifiedFlexItemProperties[name] && resolved.flexItem == nil && !(htmlUnifiedFlexContainerProperties[name] && resolved.flexContainer != nil) {
				return htmlPlanUnsupported(compiled.tokens[token].Str, token, fmt.Sprintf("resolved flex item property %q requires a direct supported flex child", name))
			}
			if htmlUnifiedFlexProperties[name] && !htmlUnifiedFlexContainerProperties[name] && !htmlUnifiedFlexItemProperties[name] {
				return htmlPlanUnsupported(compiled.tokens[token].Str, token, fmt.Sprintf("resolved flex property %q is outside the initial unified flex cohort", name))
			}
		}
	}
	return nil
}

func htmlUnifiedParseFlexContainer(tag string, token int, decl map[string]string) (htmlUnifiedFlexContainer, error) {
	allowed := make(map[string]bool, len(htmlUnifiedTextProperties)+len(htmlUnifiedBreakProperties)+len(htmlUnifiedFlexContainerProperties)+len(htmlUnifiedFlexItemProperties)+len(htmlUnifiedBlockBoxProperties))
	for name := range htmlUnifiedTextProperties {
		allowed[name] = true
	}
	for name := range htmlUnifiedBreakProperties {
		allowed[name] = true
	}
	for name := range htmlUnifiedFlexContainerProperties {
		allowed[name] = true
	}
	// A flex container may itself be a flex item. Parent-context validation
	// below remains responsible for proving that these item declarations are
	// attached to a direct child rather than accepting them as orphans.
	for name := range htmlUnifiedFlexItemProperties {
		allowed[name] = true
	}
	for name := range htmlUnifiedBlockBoxProperties {
		allowed[name] = true
	}
	for name := range decl {
		if !allowed[name] {
			return htmlUnifiedFlexContainer{}, htmlPlanUnsupported(tag, token, fmt.Sprintf("resolved flex container property %q is outside the initial cohort", name))
		}
	}

	container := htmlUnifiedFlexContainer{direction: layout.RowDirection, wrap: "nowrap", mainAlign: "start", align: "stretch", alignContent: "stretch"}
	switch value := strings.ToLower(strings.TrimSpace(decl["flex-direction"])); value {
	case "", "row":
	case "row-reverse":
		container.reverseMain = true
	case "column":
		container.direction = layout.ColumnDirection
	case "column-reverse":
		container.direction = layout.ColumnDirection
		container.reverseMain = true
	default:
		return htmlUnifiedFlexContainer{}, htmlPlanUnsupported(tag, token, fmt.Sprintf("resolved flex-direction %q is unsupported", value))
	}
	switch value := strings.ToLower(strings.TrimSpace(decl["flex-wrap"])); value {
	case "", "nowrap":
	case "wrap", "wrap-reverse":
		container.wrap = value
	default:
		return htmlUnifiedFlexContainer{}, htmlPlanUnsupported(tag, token, fmt.Sprintf("resolved flex-wrap %q is unsupported", value))
	}
	mainAlign, err := htmlUnifiedFlexMainAlignment(decl["justify-content"])
	if err != nil {
		return htmlUnifiedFlexContainer{}, htmlPlanUnsupported(tag, token, "resolved justify-content "+err.Error())
	}
	container.mainAlign = mainAlign
	align, err := htmlUnifiedFlexAlignment(decl["align-items"], "stretch")
	if err != nil {
		return htmlUnifiedFlexContainer{}, htmlPlanUnsupported(tag, token, "resolved align-items "+err.Error())
	}
	container.align = align
	alignContent, err := htmlUnifiedFlexAlignContent(decl["align-content"])
	if err != nil {
		return htmlUnifiedFlexContainer{}, htmlPlanUnsupported(tag, token, "resolved align-content "+err.Error())
	}
	container.alignContent = alignContent
	mainGap, crossGap, err := htmlUnifiedFlexGaps(container.direction, decl)
	if err != nil {
		return htmlUnifiedFlexContainer{}, htmlPlanUnsupported(tag, token, err.Error())
	}
	container.gap, container.crossGap = mainGap, crossGap
	mainName, crossName := "width", "height"
	if container.direction == layout.ColumnDirection {
		mainName, crossName = "height", "width"
	}
	if value := strings.TrimSpace(decl[mainName]); value != "" && !strings.EqualFold(value, "auto") {
		return htmlUnifiedFlexContainer{}, htmlPlanUnsupported(tag, token, fmt.Sprintf("resolved flex container %s %q is outside the full-main-axis cohort", mainName, value))
	}
	if value := strings.TrimSpace(decl[crossName]); value != "" && !strings.EqualFold(value, "auto") {
		size, ok := htmlUnifiedFixedCSSLength(value, false)
		if !ok {
			return htmlUnifiedFlexContainer{}, htmlPlanUnsupported(tag, token, fmt.Sprintf("resolved flex container %s %q must be a positive fixed length", crossName, value))
		}
		container.crossSize = size
	}
	for _, name := range []string{"min-width", "min-height", "max-width", "max-height"} {
		value := strings.ToLower(strings.TrimSpace(decl[name]))
		if value != "" && value != "auto" && value != "none" && value != "0" && value != "0pt" && value != "0px" {
			return htmlUnifiedFlexContainer{}, htmlPlanUnsupported(tag, token, fmt.Sprintf("resolved flex container %s %q is outside the initial cohort", name, decl[name]))
		}
	}
	return container, nil
}

func htmlUnifiedFlexMainAlignment(value string) (string, error) {
	switch normalized := strings.ToLower(strings.TrimSpace(value)); normalized {
	case "", "normal", "start", "flex-start":
		return "start", nil
	case "center":
		return "center", nil
	case "end", "flex-end":
		return "end", nil
	case "space-between", "space-around", "space-evenly":
		return normalized, nil
	default:
		return "", fmt.Errorf("value %q is unsupported", value)
	}
}

// htmlUnifiedFlexMainGap resolves the CSS row/column gap pair before lowering.
// Row flex uses the column gap and column flex uses the row gap. Cross-axis
// gaps are intentionally ignored for nowrap containers because no cross-axis
// tracks exist; once wrapping is introduced they must become planner input.
func htmlUnifiedFlexGaps(direction layout.RowColumnDirection, decl map[string]string) (float64, float64, error) {
	var rowGap, columnGap float64
	if value := strings.TrimSpace(decl["gap"]); value != "" {
		fields := strings.Fields(value)
		if len(fields) < 1 || len(fields) > 2 {
			return 0, 0, fmt.Errorf("resolved gap %q must contain one or two finite non-negative fixed lengths", value)
		}
		parsed, ok := htmlUnifiedFlexGapLength(fields[0])
		if !ok {
			return 0, 0, fmt.Errorf("resolved row gap %q must be a finite non-negative fixed length", fields[0])
		}
		rowGap, columnGap = parsed, parsed
		if len(fields) == 2 {
			parsed, ok = htmlUnifiedFlexGapLength(fields[1])
			if !ok {
				return 0, 0, fmt.Errorf("resolved column gap %q must be a finite non-negative fixed length", fields[1])
			}
			columnGap = parsed
		}
	}
	for _, entry := range []struct {
		name   string
		target *float64
	}{
		{name: "row-gap", target: &rowGap},
		{name: "column-gap", target: &columnGap},
	} {
		if value := strings.TrimSpace(decl[entry.name]); value != "" {
			parsed, ok := htmlUnifiedFlexGapLength(value)
			if !ok {
				return 0, 0, fmt.Errorf("resolved %s %q must be a finite non-negative fixed length", entry.name, value)
			}
			*entry.target = parsed
		}
	}
	if direction == layout.ColumnDirection {
		return rowGap, columnGap, nil
	}
	return columnGap, rowGap, nil
}

func htmlUnifiedFlexAlignContent(value string) (string, error) {
	switch normalized := strings.ToLower(strings.TrimSpace(value)); normalized {
	case "", "normal", "stretch":
		return "stretch", nil
	case "start", "flex-start":
		return "start", nil
	case "center":
		return "center", nil
	case "end", "flex-end":
		return "end", nil
	case "space-between", "space-around", "space-evenly":
		return normalized, nil
	default:
		return "", fmt.Errorf("value %q is unsupported", value)
	}
}

func htmlUnifiedFlexGapLength(value string) (float64, bool) {
	if strings.EqualFold(strings.TrimSpace(value), "normal") {
		return 0, true
	}
	return htmlUnifiedFixedCSSLength(value, true)
}

func htmlUnifiedResolveFlexChildren(ctx context.Context, compiled *CompiledHTML, containerNode int) error {
	container := compiled.nodeIndexes[containerNode]
	tag, token := compiled.tokens[container.Token].Str, container.Token
	if len(compiled.tokens[token].Attr) != 0 {
		return htmlPlanUnsupported(tag, token, "flex container has attributes outside selector/style resolution")
	}
	cursor := token + 1
	children := 0
	for childNode := container.FirstChild; childNode >= 0; childNode = compiled.nodeIndexes[childNode].NextSibling {
		if children&255 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		child := compiled.nodeIndexes[childNode]
		if strings.EqualFold(strings.TrimSpace(compiled.unifiedResolved[child.Token].decl["display"]), "none") {
			cursor = child.EndToken + 1
			continue
		}
		if compiled.unifiedResolved[child.Token].displayNone {
			cursor = child.EndToken + 1
			continue
		}
		if err := htmlUnifiedFlexWhitespace(ctx, compiled, tag, token, cursor, child.Token); err != nil {
			return err
		}
		childToken := compiled.tokens[child.Token]
		if childToken.Str != "p" {
			if _, heading := htmlPlanHeadingLevel(childToken.Str); !heading && childToken.Str != "div" && childToken.Str != "section" && childToken.Str != "article" {
				return htmlPlanUnsupported(childToken.Str, child.Token, "flex items must be direct text blocks or bounded structural wrappers")
			}
		}
		if len(childToken.Attr) != 0 {
			return htmlPlanUnsupported(childToken.Str, child.Token, "flex item has attributes outside selector/style resolution")
		}
		item, err := htmlUnifiedParseFlexItem(childToken.Str, child.Token, compiled.unifiedResolved[child.Token].decl, compiled.unifiedResolved[container.Token].flexContainer.direction)
		if err != nil {
			return err
		}
		if item.crossSize > 0 && item.align == "" && compiled.unifiedResolved[container.Token].flexContainer.align == "stretch" {
			item.align = "start"
		}
		compiled.unifiedResolved[child.Token].flexItem = &item
		children++
		cursor = child.EndToken + 1
	}
	if err := htmlUnifiedFlexWhitespace(ctx, compiled, tag, token, cursor, container.EndToken); err != nil {
		return err
	}
	if children == 0 {
		return htmlPlanUnsupported(tag, token, "flex container requires at least one direct item")
	}
	containerMeta := compiled.unifiedResolved[container.Token].flexContainer
	if containerMeta.wrap != "nowrap" {
		if containerMeta.crossSize <= 0 && containerMeta.direction == layout.RowDirection {
			crossName := "height"
			return htmlPlanUnsupported(tag, token, "wrapped flex container requires a definite positive "+crossName)
		}
		for childNode := container.FirstChild; childNode >= 0; childNode = compiled.nodeIndexes[childNode].NextSibling {
			child := compiled.nodeIndexes[childNode]
			if compiled.unifiedResolved[child.Token].displayNone {
				continue
			}
			item := compiled.unifiedResolved[child.Token].flexItem
			positiveBasis := item.track.Kind == layout.RowColumnTrackFixed && item.track.Size > 0 ||
				item.track.Kind == layout.RowColumnTrackFlex && (item.track.BasisKind == layout.RowColumnFlexBasisFixed && item.track.Basis > 0 ||
					item.track.BasisKind == layout.RowColumnFlexBasisPercent && item.track.BasisPercent > 0 || item.track.BasisKind == layout.RowColumnFlexBasisContent)
			if !positiveBasis {
				return htmlPlanUnsupported(compiled.tokens[child.Token].Str, child.Token, "wrapped flex items require a positive fixed or percentage main-axis basis, or a measurable content basis")
			}
		}
	}
	return nil
}

func htmlUnifiedFlexWhitespace(ctx context.Context, compiled *CompiledHTML, tag string, token, start, end int) error {
	for index := start; index < end; index++ {
		if index&255 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		part := compiled.tokens[index]
		if part.Cat == 'T' && strings.TrimSpace(part.Str) == "" {
			continue
		}
		return htmlPlanUnsupported(tag, token, "flex container has direct content outside supported item elements")
	}
	return nil
}

func htmlUnifiedParseFlexItem(tag string, token int, decl map[string]string, direction layout.RowColumnDirection) (htmlUnifiedFlexItem, error) {
	allowed := make(map[string]bool, len(htmlUnifiedTextProperties)+len(htmlUnifiedFlexItemProperties)+len(htmlUnifiedFlexContainerProperties)+len(htmlUnifiedBlockBoxProperties))
	for name := range htmlUnifiedTextProperties {
		allowed[name] = true
	}
	for name := range htmlUnifiedFlexItemProperties {
		allowed[name] = true
	}
	// A direct item may also establish a nested flex formatting context. The
	// final relationship scan still rejects these properties on ordinary items
	// that do not resolve display:flex.
	for name := range htmlUnifiedFlexContainerProperties {
		allowed[name] = true
	}
	for name := range htmlUnifiedBlockBoxProperties {
		allowed[name] = true
	}
	for name := range decl {
		if !allowed[name] {
			return htmlUnifiedFlexItem{}, htmlPlanUnsupported(tag, token, fmt.Sprintf("resolved flex item property %q is outside the initial cohort", name))
		}
	}
	track, err := htmlUnifiedFlexTrack(decl)
	if err != nil {
		return htmlUnifiedFlexItem{}, htmlPlanUnsupported(tag, token, err.Error())
	}
	align, alignErr := htmlUnifiedFlexAlignment(decl["align-self"], "")
	if alignErr != nil {
		return htmlUnifiedFlexItem{}, htmlPlanUnsupported(tag, token, "resolved align-self "+alignErr.Error())
	}
	track, crossSize, crossMin, crossMax, crossMinPercent, crossMaxPercent, dimensionErr := htmlUnifiedFlexDimensions(direction, decl, track)
	if dimensionErr != nil {
		return htmlUnifiedFlexItem{}, htmlPlanUnsupported(tag, token, dimensionErr.Error())
	}
	return htmlUnifiedFlexItem{track: track, crossSize: crossSize, crossMin: crossMin, crossMax: crossMax, crossMinPercent: crossMinPercent, crossMaxPercent: crossMaxPercent, align: align}, nil
}

func htmlUnifiedFlexDimensions(direction layout.RowColumnDirection, decl map[string]string, track layout.RowColumnTrack) (layout.RowColumnTrack, float64, float64, float64, uint32, uint32, error) {
	mainName, minMainName := "width", "min-width"
	maxMainName := "max-width"
	crossName, minCrossName, maxCrossName := "height", "min-height", "max-height"
	if direction == layout.ColumnDirection {
		mainName, minMainName, maxMainName, crossName, minCrossName, maxCrossName = "height", "min-height", "max-height", "width", "min-width", "max-width"
	}
	if value := strings.TrimSpace(decl[mainName]); value != "" && !strings.EqualFold(value, "auto") && (track.Kind == layout.RowColumnTrackAuto || track.Kind == layout.RowColumnTrackFlex && track.BasisKind == layout.RowColumnFlexBasisContent) {
		if track.Kind == layout.RowColumnTrackFlex {
			if percent, ok := htmlUnifiedFlexPercent(value); ok {
				track.BasisKind, track.BasisPercent = layout.RowColumnFlexBasisPercent, percent
			} else if size, ok := htmlUnifiedFixedCSSLength(value, false); ok {
				track.BasisKind, track.Basis = layout.RowColumnFlexBasisFixed, size
			} else {
				return layout.RowColumnTrack{}, 0, 0, 0, 0, 0, fmt.Errorf("resolved %s %q must be a positive fixed length or percentage for an auto flex basis", mainName, value)
			}
		} else {
			size, ok := htmlUnifiedFixedCSSLength(value, false)
			if !ok {
				return layout.RowColumnTrack{}, 0, 0, 0, 0, 0, fmt.Errorf("resolved %s %q must be a positive fixed length", mainName, value)
			}
			track = layout.RowColumnTrack{Kind: layout.RowColumnTrackFixed, Size: size}
		}
	}
	if value := strings.TrimSpace(decl[minMainName]); value != "" && !strings.EqualFold(value, "auto") {
		if percent, ok := htmlUnifiedFlexPercent(value); ok {
			track.MinPercent = percent
		} else {
			minimum, ok := htmlUnifiedFixedCSSLength(value, true)
			if !ok {
				return layout.RowColumnTrack{}, 0, 0, 0, 0, 0, fmt.Errorf("resolved %s %q must be a non-negative fixed length or percentage", minMainName, value)
			}
			track.Min = minimum
			if track.Kind == layout.RowColumnTrackFixed && track.Size < minimum {
				track.Size = minimum
			}
		}
	}
	if value := strings.TrimSpace(decl[maxMainName]); value != "" && !strings.EqualFold(value, "none") && !strings.EqualFold(value, "auto") {
		maximum, percent, isPercent := 0.0, uint32(0), false
		if percent, isPercent = htmlUnifiedFlexPercent(value); !isPercent {
			var ok bool
			maximum, ok = htmlUnifiedFixedCSSLength(value, false)
			if !ok {
				return layout.RowColumnTrack{}, 0, 0, 0, 0, 0, fmt.Errorf("resolved %s %q must be a positive fixed length or percentage", maxMainName, value)
			}
		}
		if track.Kind == layout.RowColumnTrackFraction {
			track = layout.RowColumnTrack{Kind: layout.RowColumnTrackFlex, BasisKind: layout.RowColumnFlexBasisFixed, Grow: track.Weight, Shrink: 1, Min: track.Min, MinPercent: track.MinPercent}
		}
		if track.Kind == layout.RowColumnTrackFixed {
			if isPercent {
				return layout.RowColumnTrack{}, 0, 0, 0, 0, 0, fmt.Errorf("resolved %s percentage requires a flexible main-axis basis", maxMainName)
			}
			if track.Size > maximum {
				track.Size = maximum
			}
		} else if track.Kind == layout.RowColumnTrackFlex {
			if isPercent {
				track.MaxPercent = percent
			} else {
				track.Max = maximum
			}
		} else {
			return layout.RowColumnTrack{}, 0, 0, 0, 0, 0, fmt.Errorf("resolved %s requires a definite or flexible main-axis basis", maxMainName)
		}
	}
	var crossSize float64
	if value := strings.TrimSpace(decl[crossName]); value != "" && !strings.EqualFold(value, "auto") {
		var ok bool
		crossSize, ok = htmlUnifiedFixedCSSLength(value, false)
		if !ok {
			return layout.RowColumnTrack{}, 0, 0, 0, 0, 0, fmt.Errorf("resolved %s %q must be a positive fixed length", crossName, value)
		}
	}
	var crossMin, crossMax float64
	var crossMinPercent, crossMaxPercent uint32
	if value := strings.TrimSpace(decl[minCrossName]); value != "" && !strings.EqualFold(value, "auto") {
		if percent, ok := htmlUnifiedFlexPercent(value); ok {
			crossMinPercent = percent
		} else if fixed, ok := htmlUnifiedFixedCSSLength(value, true); ok {
			crossMin = fixed
		} else {
			return layout.RowColumnTrack{}, 0, 0, 0, 0, 0, fmt.Errorf("resolved %s %q must be a non-negative fixed length or percentage", minCrossName, value)
		}
	}
	if value := strings.TrimSpace(decl[maxCrossName]); value != "" && !strings.EqualFold(value, "none") && !strings.EqualFold(value, "auto") {
		if percent, ok := htmlUnifiedFlexPercent(value); ok {
			crossMaxPercent = percent
		} else if fixed, ok := htmlUnifiedFixedCSSLength(value, false); ok {
			crossMax = fixed
		} else {
			return layout.RowColumnTrack{}, 0, 0, 0, 0, 0, fmt.Errorf("resolved %s %q must be a positive fixed length or percentage", maxCrossName, value)
		}
	}
	return track, crossSize, crossMin, crossMax, crossMinPercent, crossMaxPercent, nil
}

func htmlUnifiedFlexTrack(decl map[string]string) (layout.RowColumnTrack, error) {
	var track layout.RowColumnTrack
	var err error
	if shorthand := strings.TrimSpace(decl["flex"]); shorthand != "" {
		track, err = htmlUnifiedFlexShorthandTrack(shorthand)
		if err != nil {
			return layout.RowColumnTrack{}, err
		}
	} else {
		track, _ = htmlUnifiedFlexFactorTrack(htmlUnifiedCSSFactor{}, htmlUnifiedCSSFactor{integral: 1, scaled: 1_000_000}, "content")
	}
	if decl["flex-grow"] == "" && decl["flex-shrink"] == "" && decl["flex-basis"] == "" {
		return track, nil
	}
	// Resolved longhands override their shorthand components. This is stable
	// regardless of whether the winning declaration came from a selector or
	// inline style; the frontend never leaks cascade state into the planner.
	grow := htmlUnifiedTrackGrow(track)
	shrink := htmlUnifiedTrackShrink(track)
	basis := htmlUnifiedTrackBasis(track)
	if value := strings.TrimSpace(decl["flex-grow"]); value != "" {
		grow, err = htmlUnifiedFlexFactor(value)
		if err != nil {
			return layout.RowColumnTrack{}, fmt.Errorf("resolved flex-grow %q is unsupported", value)
		}
	}
	if value := strings.TrimSpace(decl["flex-shrink"]); value != "" {
		shrink, err = htmlUnifiedFlexFactor(value)
		if err != nil {
			return layout.RowColumnTrack{}, fmt.Errorf("resolved flex-shrink %q is unsupported", value)
		}
	}
	if value := strings.TrimSpace(decl["flex-basis"]); value != "" {
		basis = value
	}
	if basis == "" {
		basis = "auto"
	}
	return htmlUnifiedFlexFactorTrack(grow, shrink, basis)
}

type htmlUnifiedCSSFactor struct {
	integral uint32
	scaled   uint64
}

func htmlUnifiedFlexFactor(value string) (htmlUnifiedCSSFactor, error) {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || parsed < 0 || parsed > 4294.967295 {
		return htmlUnifiedCSSFactor{}, fmt.Errorf("invalid factor")
	}
	scaledFloat := parsed * 1_000_000
	if scaledFloat != float64(uint64(scaledFloat)) {
		return htmlUnifiedCSSFactor{}, fmt.Errorf("factor has more than six decimal places")
	}
	scaled := uint64(scaledFloat)
	result := htmlUnifiedCSSFactor{scaled: scaled}
	if scaled%1_000_000 == 0 {
		result.integral = uint32(scaled / 1_000_000)
	}
	return result, nil
}

func htmlUnifiedTrackGrow(track layout.RowColumnTrack) htmlUnifiedCSSFactor {
	if track.GrowFactor != 0 {
		return htmlUnifiedCSSFactor{scaled: track.GrowFactor}
	}
	return htmlUnifiedCSSFactor{integral: track.Grow, scaled: uint64(track.Grow) * 1_000_000}
}
func htmlUnifiedTrackShrink(track layout.RowColumnTrack) htmlUnifiedCSSFactor {
	if track.ShrinkFactor != 0 {
		return htmlUnifiedCSSFactor{scaled: track.ShrinkFactor}
	}
	return htmlUnifiedCSSFactor{integral: track.Shrink, scaled: uint64(track.Shrink) * 1_000_000}
}
func htmlUnifiedTrackBasis(track layout.RowColumnTrack) string {
	switch track.BasisKind {
	case layout.RowColumnFlexBasisFixed:
		return strconv.FormatFloat(track.Basis, 'f', -1, 64) + "pt"
	case layout.RowColumnFlexBasisPercent:
		return strconv.FormatFloat(float64(track.BasisPercent)/1_000_000, 'f', -1, 64) + "%"
	case layout.RowColumnFlexBasisContent:
		return "content"
	}
	if track.Kind == layout.RowColumnTrackFraction {
		return "0%"
	}
	return "auto"
}

func htmlUnifiedFlexShorthandTrack(value string) (layout.RowColumnTrack, error) {
	if strings.EqualFold(value, "none") {
		return htmlUnifiedFlexFactorTrack(htmlUnifiedCSSFactor{}, htmlUnifiedCSSFactor{}, "content")
	}
	if strings.EqualFold(value, "initial") {
		return htmlUnifiedFlexFactorTrack(htmlUnifiedCSSFactor{}, htmlUnifiedCSSFactor{integral: 1, scaled: 1_000_000}, "content")
	}
	if strings.EqualFold(value, "auto") {
		return htmlUnifiedFlexFactorTrack(htmlUnifiedCSSFactor{integral: 1, scaled: 1_000_000}, htmlUnifiedCSSFactor{integral: 1, scaled: 1_000_000}, "auto")
	}
	fields := strings.Fields(strings.ToLower(value))
	if len(fields) == 1 {
		factor, factorErr := htmlUnifiedFlexFactor(fields[0])
		if factorErr == nil && factor.scaled > 0 {
			if factor.integral > 0 {
				return layout.RowColumnTrack{Kind: layout.RowColumnTrackFraction, Weight: factor.integral}, nil
			}
			return htmlUnifiedFlexFactorTrack(factor, htmlUnifiedCSSFactor{integral: 1, scaled: 1_000_000}, "0%")
		}
		return htmlUnifiedFlexFactorTrack(htmlUnifiedCSSFactor{integral: 1, scaled: 1_000_000}, htmlUnifiedCSSFactor{integral: 1, scaled: 1_000_000}, fields[0])
	}
	if len(fields) == 2 {
		grow, err := htmlUnifiedFlexFactor(fields[0])
		if err != nil {
			return layout.RowColumnTrack{}, fmt.Errorf("resolved flex shorthand %q is unsupported", value)
		}
		if shrink, shrinkErr := htmlUnifiedFlexFactor(fields[1]); shrinkErr == nil {
			return htmlUnifiedFlexFactorTrack(grow, shrink, "0%")
		}
		return htmlUnifiedFlexFactorTrack(grow, htmlUnifiedCSSFactor{integral: 1, scaled: 1_000_000}, fields[1])
	}
	if len(fields) != 3 {
		return layout.RowColumnTrack{}, fmt.Errorf("resolved flex shorthand %q is unsupported", value)
	}
	grow, growErr := htmlUnifiedFlexFactor(fields[0])
	shrink, shrinkErr := htmlUnifiedFlexFactor(fields[1])
	if growErr != nil || shrinkErr != nil {
		return layout.RowColumnTrack{}, fmt.Errorf("resolved flex shorthand %q is unsupported", value)
	}
	if grow.integral > 0 && shrink.integral == 1 && (fields[2] == "0" || fields[2] == "0%" || fields[2] == "0pt" || fields[2] == "0px") {
		return layout.RowColumnTrack{Kind: layout.RowColumnTrackFraction, Weight: grow.integral}, nil
	}
	return htmlUnifiedFlexFactorTrack(grow, shrink, fields[2])
}

func htmlUnifiedFlexFactorTrack(grow, shrink htmlUnifiedCSSFactor, basis string) (layout.RowColumnTrack, error) {
	track := layout.RowColumnTrack{Kind: layout.RowColumnTrackFlex, Grow: grow.integral, Shrink: shrink.integral}
	if grow.integral == 0 {
		track.GrowFactor = grow.scaled
	}
	if shrink.integral == 0 {
		track.ShrinkFactor = shrink.scaled
	}
	if strings.EqualFold(strings.TrimSpace(basis), "auto") || strings.EqualFold(strings.TrimSpace(basis), "content") {
		track.BasisKind = layout.RowColumnFlexBasisContent
		return track, nil
	}
	if percent, ok := htmlUnifiedFlexPercent(basis); ok {
		if percent == 0 && grow.scaled == 0 {
			return layout.RowColumnTrack{}, fmt.Errorf("resolved zero percentage flex-basis requires a positive integral flex-grow")
		}
		if grow.integral > 0 && shrink.integral == 1 && percent == 0 {
			return layout.RowColumnTrack{Kind: layout.RowColumnTrackFraction, Weight: grow.integral}, nil
		}
		track.BasisKind, track.BasisPercent = layout.RowColumnFlexBasisPercent, percent
		return track, nil
	}
	size, ok := htmlUnifiedFixedCSSLength(basis, true)
	if !ok {
		return layout.RowColumnTrack{}, fmt.Errorf("resolved flex-basis %q must be auto, a fixed length, or a percentage", basis)
	}
	if size == 0 && grow.scaled == 0 {
		return layout.RowColumnTrack{}, fmt.Errorf("resolved zero flex-basis requires a positive integral flex-grow")
	}
	if grow.scaled == 0 && shrink.scaled == 0 && size > 0 {
		return layout.RowColumnTrack{Kind: layout.RowColumnTrackFixed, Size: size}, nil
	}
	track.BasisKind, track.Basis = layout.RowColumnFlexBasisFixed, size
	return track, nil
}

func htmlUnifiedFlexPercent(value string) (uint32, bool) {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if !strings.HasSuffix(trimmed, "%") {
		return 0, false
	}
	number := strings.TrimSpace(strings.TrimSuffix(trimmed, "%"))
	parsed, err := strconv.ParseFloat(number, 64)
	if err != nil || parsed < 0 || parsed > 100 {
		return 0, false
	}
	scaled := parsed * 1_000_000
	if scaled != float64(uint32(scaled)) {
		return 0, false
	}
	return uint32(scaled), true
}

func htmlUnifiedFlexWeightDefault(value string, fallback uint32) (uint32, bool, error) {
	weight, present, err := htmlUnifiedFlexWeight(value)
	if !present && err == nil {
		return fallback, false, nil
	}
	return weight, present, err
}

func htmlUnifiedFlexWeight(value string) (uint32, bool, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false, nil
	}
	parsed, err := strconv.ParseUint(value, 10, 32)
	if err != nil {
		return 0, true, err
	}
	return uint32(parsed), true, nil
}

func htmlUnifiedFixedCSSLength(value string, allowZero bool) (float64, bool) {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	if strings.HasSuffix(trimmed, "%") && trimmed != "0%" && trimmed != "0.0%" {
		return 0, false
	}
	length, ok := parseHTMLBoxLength(trimmed, nil, 0)
	if !ok || length < 0 || (!allowZero && length == 0) {
		return 0, false
	}
	return length, true
}

func htmlUnifiedFlexAlignment(value, fallback string) (string, error) {
	switch normalized := strings.ToLower(strings.TrimSpace(value)); normalized {
	case "":
		return fallback, nil
	case "normal", "stretch":
		return "stretch", nil
	case "start", "flex-start":
		return "start", nil
	case "center":
		return "center", nil
	case "end", "flex-end":
		return "end", nil
	default:
		return "", fmt.Errorf("value %q is unsupported", value)
	}
}
