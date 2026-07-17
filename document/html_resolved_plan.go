// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/cssbruno/gopdfkit/layout"
)

// htmlUnifiedResolvedElement is the selector-free HTML frontend output used
// by the unified adapter. It deliberately contains document-model values, not
// selectors, specificity, or cascade state.
type htmlUnifiedResolvedElement struct {
	text          layout.TextStyle
	box           layout.BoxStyle
	decl          map[string]string
	destination   string
	displayNone   bool
	textTransform string
	preserveWS    bool
	preserveLines bool
	whiteSpace    string
	flexContainer *htmlUnifiedFlexContainer
	flexItem      *htmlUnifiedFlexItem
}

var htmlUnifiedTextProperties = map[string]bool{
	"color": true, "font-family": true, "font-size": true,
	"font":       true,
	"font-style": true, "font-weight": true, "line-height": true,
	"text-align": true, "text-decoration": true, "white-space": true,
	"tab-size": true, "text-transform": true,
}

var htmlUnifiedBreakProperties = map[string]bool{
	"break-before": true, "page-break-before": true,
	"break-after": true, "page-break-after": true,
	"break-inside": true, "page-break-inside": true,
}

// htmlUnifiedFlowProperties contains the normal-flow compatibility subset.
// Static positioning and a no-op float are exact defaults in the canonical
// vertical planner; display:none is lowered by pruning the complete element
// subtree before block or inline lowering.
var htmlUnifiedFlowProperties = map[string]bool{
	"display": true, "position": true, "float": true, "clear": true,
}

var htmlUnifiedImageProperties = map[string]bool{
	"width": true, "height": true, "max-width": true, "max-height": true,
	"object-fit": true, "text-align": true,
}

var htmlUnifiedTableBoxProperties = func() map[string]bool {
	result := map[string]bool{
		"background-color": true, "border-collapse": true,
		"border": true,
		"width":  true, "min-width": true, "max-width": true,
		"padding": true, "padding-top": true, "padding-right": true,
		"padding-bottom": true, "padding-left": true,
		"text-align": true, "vertical-align": true,
	}
	for _, side := range []string{"top", "right", "bottom", "left"} {
		result["border-"+side] = true
		result["border-"+side+"-width"] = true
		result["border-"+side+"-style"] = true
		result["border-"+side+"-color"] = true
	}
	return result
}()

// resolveCompiledHTMLUnifiedSnapshot performs the whole-fragment capability
// scan and creates a detached, selector-free snapshot. No layout is attempted
// until every opening element has passed this scan.
func (f *Document) resolveCompiledHTMLUnifiedSnapshot(ctx context.Context, compiled *CompiledHTML, lineHeight float64) (*CompiledHTML, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	clone := *compiled
	clone.tokens = append([]HTMLSegmentType(nil), compiled.tokens...)
	clone.cssRules = nil
	clone.unifiedResolved = make([]htmlUnifiedResolvedElement, len(compiled.tokens))

	base := htmlTextStyle{fontFamily: firstNonEmpty(f.fontFamily, "Helvetica"), fontSize: f.fontSizePt, lineHeight: lineHeight, align: "L"}
	r, g, b := f.GetTextColor()
	base.color = CSSColorType{R: r, G: g, B: b, Set: r != 0 || g != 0 || b != 0}
	resolvedLegacy := make([]htmlTextStyle, len(compiled.tokens))

	for index, token := range compiled.tokens {
		if index&255 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		if token.Cat != 'O' {
			continue
		}
		if htmlUnifiedTokenInsideSVG(compiled, index) {
			// Inline SVG has already passed the bounded XML/SVG compiler. Its
			// presentation attributes and internal stylesheet vocabulary are
			// validated by the SVG-to-display-plan cohort, not HTML CSS lowering.
			continue
		}
		inherited := base
		headingDefaults := true
		if nodeIndex := compiled.tokenNode[index]; nodeIndex >= 0 {
			if parent := compiled.nodeIndexes[nodeIndex].Parent; parent >= 0 {
				parentToken := compiled.nodeIndexes[parent].Token
				inherited = resolvedLegacy[parentToken]
				headingDefaults = strings.ToLower(strings.TrimSpace(clone.unifiedResolved[parentToken].decl["display"])) != "flex"
			}
		}
		style := inherited
		textTransform := "none"
		if nodeIndex := compiled.tokenNode[index]; nodeIndex >= 0 {
			if parent := compiled.nodeIndexes[nodeIndex].Parent; parent >= 0 {
				textTransform = clone.unifiedResolved[compiled.nodeIndexes[parent].Token].textTransform
			}
		}
		whiteSpace := "normal"
		if nodeIndex := compiled.tokenNode[index]; nodeIndex >= 0 {
			if parent := compiled.nodeIndexes[nodeIndex].Parent; parent >= 0 {
				parentToken := compiled.nodeIndexes[parent].Token
				if inheritedMode := clone.unifiedResolved[parentToken].whiteSpace; inheritedMode != "" {
					whiteSpace = inheritedMode
				}
			}
		}
		htmlUnifiedApplyElementTextDefaults(token.Str, &style, &whiteSpace)
		decl, _ := compiled.declarations(index)
		if err := htmlUnifiedValidateResolvedDeclarations(token.Str, index, decl); err != nil {
			return nil, err
		}
		// Apply bounded browser user-agent heading defaults before authored CSS.
		// The cascade snapshot contains authored declarations only, so these
		// defaults belong in the selector-free resolution phase.
		if headingDefaults {
			if _, heading := htmlPlanHeadingLevel(token.Str); heading {
				if _, explicit := decl["font-size"]; !explicit {
					style.fontSize = htmlHeadingFontSize(inherited.fontSize, token.Str)
				}
				if _, explicit := decl["font-weight"]; !explicit {
					style.bold = true
				}
				if _, explicit := decl["line-height"]; !explicit && style.lineHeight < style.fontSize*1.2 {
					style.lineHeight = style.fontSize * 1.2
				}
			}
		}
		if value := strings.TrimSpace(decl["line-height"]); value != "" && !strings.EqualFold(value, "normal") {
			if _, ok := parseHTMLLineHeight(value, inherited.lineHeight, f); !ok {
				return nil, htmlPlanUnsupported(token.Str, index, fmt.Sprintf("resolved line-height %q is invalid", value))
			}
		}
		applyHTMLStyleDeclarations(&style, decl, inherited.fontSize, inherited.lineHeight, f)
		if value, ok := decl["text-align"]; ok {
			switch strings.ToLower(strings.TrimSpace(value)) {
			case "left":
				style.align = "L"
			case "center":
				style.align = "C"
			case "right":
				style.align = "R"
			case "justify":
				style.align = "J"
			default:
				return nil, htmlPlanUnsupported(token.Str, index, fmt.Sprintf("resolved text-align %q is unsupported", value))
			}
		}
		if value, ok := decl["text-transform"]; ok {
			textTransform = strings.ToLower(strings.TrimSpace(value))
		}
		if value, ok := decl["font-weight"]; ok {
			switch strings.ToLower(strings.TrimSpace(value)) {
			case "normal", "400", "500", "lighter":
				style.bold = false
			case "bold", "bolder", "600", "700", "800", "900":
				style.bold = true
			default:
				return nil, htmlPlanUnsupported(token.Str, index, fmt.Sprintf("resolved font-weight %q is unsupported", value))
			}
		}
		if value, ok := decl["font-style"]; ok {
			switch strings.ToLower(strings.TrimSpace(value)) {
			case "normal":
				style.italic = false
			case "italic", "oblique":
				style.italic = true
			default:
				return nil, htmlPlanUnsupported(token.Str, index, fmt.Sprintf("resolved font-style %q is unsupported", value))
			}
		}
		if value, ok := decl["text-decoration"]; ok {
			lower := strings.ToLower(strings.TrimSpace(value))
			style.underline = strings.Contains(lower, "underline")
			style.strike = strings.Contains(lower, "line-through")
			if lower != "none" && !style.underline && !style.strike {
				return nil, htmlPlanUnsupported(token.Str, index, fmt.Sprintf("resolved text-decoration %q is unsupported", value))
			}
		}
		if value, ok := decl["white-space"]; ok {
			whiteSpace = strings.ToLower(strings.TrimSpace(value))
			style.preserveWhitespace = whiteSpace == "pre" || whiteSpace == "pre-wrap" || whiteSpace == "break-spaces"
		}
		resolvedLegacy[index] = style
		box, err := htmlUnifiedResolvedBox(token.Str, index, decl, style.fontSize, headingDefaults, f)
		if err != nil {
			return nil, err
		}
		textStyle := htmlUnifiedLayoutTextStyle(style, lineHeight)
		textStyle.WhiteSpace = whiteSpace
		if raw, ok := decl["tab-size"]; ok {
			value, parseErr := strconv.Atoi(strings.TrimSpace(raw))
			if parseErr != nil || value < 1 || value > 16 {
				return nil, htmlPlanUnsupported(token.Str, index, fmt.Sprintf("resolved tab-size %q must be an integer from 1 through 16", raw))
			}
			textStyle.TabSize = uint8(value)
		}
		clone.unifiedResolved[index] = htmlUnifiedResolvedElement{
			text: textStyle, box: box, destination: htmlUnifiedDestination(token),
			displayNone:   strings.EqualFold(strings.TrimSpace(decl["display"]), "none"),
			textTransform: textTransform,
			decl:          cloneStringMap(decl), preserveWS: style.preserveWhitespace,
			preserveLines: whiteSpace == "pre" || whiteSpace == "pre-wrap" || whiteSpace == "pre-line" || whiteSpace == "break-spaces",
			whiteSpace:    whiteSpace,
		}
		attrs := cloneStringMap(token.Attr)
		delete(attrs, "class")
		delete(attrs, "id")
		delete(attrs, "style")
		clone.tokens[index].Attr = attrs
	}
	if err := htmlUnifiedResolveFlexCohort(ctx, &clone); err != nil {
		return nil, err
	}
	if err := htmlUnifiedValidateNonFlexBoxConstraints(&clone); err != nil {
		return nil, err
	}
	return &clone, nil
}

func htmlUnifiedDestination(token HTMLSegmentType) string {
	name := strings.TrimSpace(token.Attr["id"])
	if name == "" && token.Str == "a" {
		name = strings.TrimSpace(token.Attr["name"])
	}
	if name == "" || validateTypedDestinationName(name) != nil {
		return ""
	}
	return name
}

func htmlUnifiedValidateResolvedDeclarations(tag string, token int, decl map[string]string) error {
	allowed := make(map[string]bool, len(htmlUnifiedTextProperties)+len(htmlUnifiedBreakProperties)+len(htmlUnifiedFlowProperties)+len(htmlUnifiedTableBoxProperties))
	for name := range htmlUnifiedTextProperties {
		allowed[name] = true
	}
	for name := range htmlUnifiedBreakProperties {
		allowed[name] = true
	}
	for name := range htmlUnifiedFlowProperties {
		allowed[name] = true
	}
	for name := range htmlUnifiedFlexProperties {
		allowed[name] = true
	}
	if htmlUnifiedBlockLikeTag(tag, decl) {
		for name := range htmlUnifiedBlockBoxProperties {
			allowed[name] = true
		}
	}
	if tag == "img" {
		for name := range htmlUnifiedImageProperties {
			allowed[name] = true
		}
	}
	if tag == "table" {
		allowed["background-color"] = true
		allowed["border-collapse"] = true
		allowed["width"] = true
	}
	if tag == "th" || tag == "td" {
		for name := range htmlUnifiedTableBoxProperties {
			allowed[name] = true
		}
		delete(allowed, "border-collapse")
	}
	if tag == "ol" || tag == "ul" {
		allowed["list-style"] = true
		allowed["list-style-type"] = true
	}
	for name, value := range decl {
		if !allowed[name] {
			return htmlPlanUnsupported(tag, token, fmt.Sprintf("resolved CSS property %q is outside the unified cohort", name))
		}
		if name == "white-space" {
			switch strings.ToLower(strings.TrimSpace(value)) {
			case "normal", "nowrap", "pre", "pre-wrap", "pre-line", "break-spaces":
			default:
				return htmlPlanUnsupported(tag, token, fmt.Sprintf("resolved white-space value %q is unsupported", value))
			}
		}
		if name == "text-transform" {
			switch strings.ToLower(strings.TrimSpace(value)) {
			case "none", "capitalize", "uppercase", "lowercase":
			default:
				return htmlPlanUnsupported(tag, token, fmt.Sprintf("resolved text-transform value %q is unsupported", value))
			}
		}
		if name == "display" {
			switch strings.ToLower(strings.TrimSpace(value)) {
			case "none", "contents", "block", "inline", "inline-block", "flex":
			default:
				return htmlPlanUnsupported(tag, token, fmt.Sprintf("resolved display value %q is unsupported", value))
			}
		}
		if name == "position" {
			if !strings.EqualFold(strings.TrimSpace(value), "static") {
				return htmlPlanUnsupported(tag, token, fmt.Sprintf("resolved CSS property \"position\" value %q is unsupported; only static is representable", value))
			}
		}
		if name == "float" {
			if !strings.EqualFold(strings.TrimSpace(value), "none") {
				return htmlPlanUnsupported(tag, token, fmt.Sprintf("resolved CSS property \"float\" value %q is unsupported; only none is representable", value))
			}
		}
		if name == "clear" {
			switch strings.ToLower(strings.TrimSpace(value)) {
			case "none", "left", "right", "both":
			default:
				return htmlPlanUnsupported(tag, token, fmt.Sprintf("resolved clear value %q is unsupported", value))
			}
		}
		if name == "object-fit" {
			switch strings.ToLower(strings.TrimSpace(value)) {
			case "fill", "contain", "cover":
			default:
				return htmlPlanUnsupported(tag, token, fmt.Sprintf("resolved object-fit value %q is unsupported", value))
			}
		}
		if name == "color" || name == "background-color" || strings.HasSuffix(name, "-color") {
			if _, ok := parseCSSColor(value); !ok {
				return htmlPlanUnsupported(tag, token, fmt.Sprintf("resolved CSS color %q is invalid", value))
			}
		}
		if name == "font-family" && htmlFontFamily(value) == "" {
			return htmlPlanUnsupported(tag, token, fmt.Sprintf("resolved font-family %q is unsupported", value))
		}
		if name == "font" {
			if _, ok := htmlExpandFontShorthand(value); !ok {
				return htmlPlanUnsupported(tag, token, fmt.Sprintf("resolved font shorthand %q is unsupported", value))
			}
		}
		if name == "font-size" {
			if _, ok := parseHTMLFontSize(value, 12); !ok {
				return htmlPlanUnsupported(tag, token, fmt.Sprintf("resolved font-size %q is invalid", value))
			}
		}
		if name == "break-inside" || name == "page-break-inside" {
			switch strings.ToLower(strings.TrimSpace(value)) {
			case "auto", "avoid":
			default:
				return htmlPlanUnsupported(tag, token, fmt.Sprintf("resolved %s value %q is unsupported", name, value))
			}
		}
	}
	return nil
}

func htmlUnifiedApplyElementTextDefaults(tag string, style *htmlTextStyle, whiteSpace *string) {
	switch tag {
	case "strong", "b":
		style.bold = true
	case "em", "i", "cite", "var":
		style.italic = true
	case "code", "kbd", "samp", "tt":
		style.fontFamily = "Courier"
	case "pre":
		style.fontFamily = "Courier"
		style.preserveWhitespace = true
		*whiteSpace = "pre"
	}
}

func htmlUnifiedResolvedBox(tag string, token int, decl map[string]string, fontSize float64, headingMargins bool, pdf *Document) (layout.BoxStyle, error) {
	if htmlUnifiedBlockLikeTag(tag, decl) {
		return htmlUnifiedParseBlockBox(tag, token, decl, fontSize, headingMargins, pdf)
	}
	box := layout.BoxStyle{KeepTogether: strings.EqualFold(strings.TrimSpace(firstNonEmpty(decl["break-inside"], decl["page-break-inside"])), "avoid")}
	if tag != "table" && tag != "th" && tag != "td" {
		return box, nil
	}
	if color, ok := parseCSSColor(decl["background-color"]); ok {
		box.BackgroundColor = layout.DocumentColor{R: color.R, G: color.G, B: color.B, Set: true}
	} else if strings.TrimSpace(decl["background-color"]) != "" {
		return layout.BoxStyle{}, htmlPlanUnsupported(tag, token, "resolved background-color is invalid")
	}
	if tag == "th" || tag == "td" {
		cell := layout.TableCell{}
		boxDecl := htmlUnifiedFilteredDeclarations(decl, htmlUnifiedTableBoxProperties)
		if len(boxDecl) != 0 {
			if err := htmlPlanApplyStrictCellStyle(&cell, htmlUnifiedDeclarationString(boxDecl), pdf.PointConvert); err != nil {
				return layout.BoxStyle{}, htmlPlanUnsupported(tag, token, err.Error())
			}
		}
		box = cell.Box
	}
	return box, nil
}

func htmlUnifiedBlockLikeTag(tag string, decl map[string]string) bool {
	if htmlUnifiedBlockBoxTag(tag) {
		return true
	}
	display := strings.ToLower(strings.TrimSpace(decl["display"]))
	return display == "block" || display == "inline-block"
}

func htmlUnifiedFilteredDeclarations(decl map[string]string, allowed map[string]bool) map[string]string {
	result := make(map[string]string)
	for name, value := range decl {
		if allowed[name] {
			result[name] = value
		}
	}
	return result
}

func htmlUnifiedLayoutTextStyle(style htmlTextStyle, fallbackLineHeight float64) layout.TextStyle {
	lineHeight := style.lineHeight
	if lineHeight <= 0 {
		lineHeight = fallbackLineHeight
	}
	return layout.TextStyle{
		FontFamily: style.fontFamily, FontSize: style.fontSize, Bold: style.bold,
		Italic: style.italic, Underline: style.underline, StrikeThrough: style.strike,
		Color: layout.DocumentColor{R: style.color.R, G: style.color.G, B: style.color.B, Set: style.color.Set},
		Align: style.align, LineHeight: lineHeight,
	}
}

func cloneStringMap(source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func htmlUnifiedDeclarationString(decl map[string]string) string {
	keys := make([]string, 0, len(decl))
	for key := range decl {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var result strings.Builder
	for _, key := range keys {
		if result.Len() != 0 {
			result.WriteByte(';')
		}
		result.WriteString(key)
		result.WriteByte(':')
		result.WriteString(decl[key])
	}
	return result.String()
}
