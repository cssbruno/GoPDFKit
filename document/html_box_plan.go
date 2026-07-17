// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/cssbruno/gopdfkit/layout"
)

var htmlUnifiedBlockBoxProperties = func() map[string]bool {
	result := map[string]bool{
		"background-color": true,
		"margin":           true, "margin-top": true, "margin-right": true, "margin-bottom": true, "margin-left": true,
		"padding": true, "padding-top": true, "padding-right": true, "padding-bottom": true, "padding-left": true,
		"border": true,
		"width":  true, "height": true, "min-width": true, "min-height": true, "max-width": true, "max-height": true,
		"overflow": true, "overflow-x": true, "overflow-y": true,
		"border-radius": true, "box-shadow": true, "box-sizing": true,
	}
	for _, side := range []string{"top", "right", "bottom", "left"} {
		result["border-"+side] = true
		result["border-"+side+"-width"] = true
		result["border-"+side+"-style"] = true
		result["border-"+side+"-color"] = true
	}
	return result
}()

func htmlUnifiedBlockBoxTag(tag string) bool {
	if tag == "p" || tag == "pre" || tag == "blockquote" || tag == "address" || tag == "div" || tag == "section" || tag == "article" || tag == "main" || tag == "html" || tag == "body" || tag == "header" || tag == "footer" {
		return true
	}
	_, heading := htmlPlanHeadingLevel(tag)
	return heading
}

func htmlUnifiedParseBlockBox(tag string, token int, decl map[string]string, fontSize float64, headingMargins bool, pdf *Document) (layout.BoxStyle, error) {
	box := layout.BoxStyle{}
	box.KeepTogether = strings.EqualFold(strings.TrimSpace(firstNonEmpty(decl["break-inside"], decl["page-break-inside"])), "avoid")
	if headingMargins {
		decl = htmlUnifiedHeadingDefaultDeclarations(tag, decl, fontSize)
	}

	if err := htmlUnifiedValidateBoxConstraints(tag, token, decl, false); err != nil {
		return layout.BoxStyle{}, err
	}

	if value := strings.TrimSpace(decl["background-color"]); value != "" {
		color, ok := parseCSSColor(value)
		if !ok {
			return layout.BoxStyle{}, htmlPlanUnsupported(tag, token, "resolved background-color is invalid")
		}
		box.BackgroundColor = layout.DocumentColor{R: color.R, G: color.G, B: color.B, Set: true}
	}
	if err := htmlUnifiedApplyBoxSpacing(&box.Margin, "margin", decl, pdf.PointConvert); err != nil {
		return layout.BoxStyle{}, htmlPlanUnsupported(tag, token, err.Error())
	}
	if err := htmlUnifiedApplyBoxSpacing(&box.Padding, "padding", decl, pdf.PointConvert); err != nil {
		return layout.BoxStyle{}, htmlPlanUnsupported(tag, token, err.Error())
	}

	borderDecl := make(map[string]string)
	for name, value := range decl {
		if name == "border" || strings.HasPrefix(name, "border-top") || strings.HasPrefix(name, "border-right") || strings.HasPrefix(name, "border-bottom") || strings.HasPrefix(name, "border-left") {
			borderDecl[name] = value
		}
	}
	if len(borderDecl) != 0 {
		cell := layout.TableCell{}
		if err := htmlPlanApplyStrictCellStyle(&cell, htmlUnifiedDeclarationString(borderDecl), pdf.PointConvert); err != nil {
			return layout.BoxStyle{}, htmlPlanUnsupported(tag, token, err.Error())
		}
		box.Border = cell.Box.Border
	}
	radius, err := htmlUnifiedBorderRadius(decl["border-radius"], pdf.PointConvert)
	if err != nil {
		return layout.BoxStyle{}, htmlPlanUnsupported(tag, token, err.Error())
	}
	box.BorderRadius = radius
	if radius > 0 {
		sides := []layout.BorderSide{box.Border.Top, box.Border.Right, box.Border.Bottom, box.Border.Left}
		first := sides[0]
		for _, side := range sides[1:] {
			if side != first {
				return layout.BoxStyle{}, htmlPlanUnsupported(tag, token, "resolved rounded borders require equal solid width and color on all four sides")
			}
		}
		if first != (layout.BorderSide{}) && !box.BackgroundColor.Set {
			return layout.BoxStyle{}, htmlPlanUnsupported(tag, token, "resolved rounded borders require an opaque background-color")
		}
	}
	shadow, err := htmlUnifiedBoxShadow(decl["box-shadow"], pdf.PointConvert)
	if err != nil {
		return layout.BoxStyle{}, htmlPlanUnsupported(tag, token, err.Error())
	}
	box.Shadow = shadow
	return box, nil
}

// htmlUnifiedHeadingDefaultDeclarations adds the bounded browser UA heading
// stylesheet defaults that affect flow geometry. It is side-aware so an
// authored margin-top or margin-bottom overrides only that side while the
// other sides retain their heading defaults.
func htmlUnifiedHeadingDefaultDeclarations(tag string, decl map[string]string, fontSize float64) map[string]string {
	level, ok := htmlPlanHeadingLevel(tag)
	if !ok || fontSize <= 0 || !finiteNumbers(fontSize) {
		return decl
	}
	result := cloneStringMap(decl)
	if _, shorthand := result["margin"]; shorthand {
		return result
	}
	if result == nil {
		result = make(map[string]string, 4)
	}
	marginEm := [...]float64{0, 0.67, 0.83, 1, 1.33, 1.67, 2.33}
	margin := marginEm[level] * fontSize
	for _, side := range []string{"top", "right", "bottom", "left"} {
		name := "margin-" + side
		if _, explicit := result[name]; !explicit {
			result[name] = strconv.FormatFloat(margin, 'f', 6, 64) + "pt"
		}
	}
	return result
}

// The unified box cohort intentionally accepts only geometry that can be
// replayed identically by PDF, SVG, and the direct raster painter. Radius is
// one fixed circular value. Shadow is one opaque, outer, zero-blur shadow.
func htmlUnifiedBorderRadius(raw string, pointsToUnits func(float64) float64) (float64, error) {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" || raw == "0" || raw == "0pt" || raw == "0px" {
		return 0, nil
	}
	if strings.Contains(raw, "/") || len(strings.Fields(raw)) != 1 || strings.HasSuffix(raw, "%") {
		return 0, fmt.Errorf("resolved border-radius %q requires one fixed non-negative circular radius", raw)
	}
	radius, err := htmlPlanStrictTableLength(raw, pointsToUnits)
	if err != nil || radius <= 0 {
		return 0, fmt.Errorf("resolved border-radius %q requires one fixed positive circular radius", raw)
	}
	return radius, nil
}

func htmlUnifiedBoxShadow(raw string, pointsToUnits func(float64) float64) (layout.BoxShadowStyle, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.EqualFold(raw, "none") {
		return layout.BoxShadowStyle{}, nil
	}
	if htmlCSSTopLevelComma(raw) >= 0 {
		return layout.BoxShadowStyle{}, fmt.Errorf("resolved box-shadow supports exactly one shadow")
	}
	tokens := htmlCSSValueFields(raw)
	lengths := make([]float64, 0, 4)
	var color layout.DocumentColor
	for _, token := range tokens {
		if strings.EqualFold(token, "inset") {
			return layout.BoxShadowStyle{}, fmt.Errorf("resolved box-shadow does not support inset shadows")
		}
		if parsed, alpha, ok := parseCSSColorWithAlpha(token); ok {
			if color.Set || parsed.None || !parsed.Set || alpha != 1 {
				return layout.BoxShadowStyle{}, fmt.Errorf("resolved box-shadow requires one opaque RGB color")
			}
			color = layout.DocumentColor{R: parsed.R, G: parsed.G, B: parsed.B, Set: true}
			continue
		}
		if len(lengths) == 4 {
			return layout.BoxShadowStyle{}, fmt.Errorf("resolved box-shadow has too many lengths")
		}
		sign := float64(1)
		value := strings.TrimSpace(token)
		if strings.HasPrefix(value, "-") {
			sign, value = -1, strings.TrimSpace(value[1:])
		} else if strings.HasPrefix(value, "+") {
			value = strings.TrimSpace(value[1:])
		}
		length, err := htmlPlanStrictTableLength(value, pointsToUnits)
		if err != nil {
			return layout.BoxShadowStyle{}, fmt.Errorf("resolved box-shadow length %q is invalid", token)
		}
		lengths = append(lengths, sign*length)
	}
	if len(lengths) < 2 || !color.Set {
		return layout.BoxShadowStyle{}, fmt.Errorf("resolved box-shadow requires two offsets and one opaque RGB color")
	}
	if len(lengths) >= 3 && lengths[2] != 0 {
		return layout.BoxShadowStyle{}, fmt.Errorf("resolved box-shadow blur must be zero")
	}
	shadow := layout.BoxShadowStyle{OffsetX: lengths[0], OffsetY: lengths[1], Color: color}
	if len(lengths) == 4 {
		shadow.Spread = lengths[3]
	}
	return shadow, nil
}

// Flex sizing is relationship-aware and validated by the flex cohort after
// parent/child resolution. The early element scan therefore defers dimension
// checks; non-flex elements are checked in a second atomic pass.
func htmlUnifiedValidateBoxConstraints(tag string, token int, decl map[string]string, enforceDimensions bool) error {
	for _, property := range []struct {
		name     string
		defaults []string
	}{
		{"width", nil}, {"height", nil},
		{"min-width", nil}, {"min-height", nil},
		{"max-width", nil}, {"max-height", nil},
		{"overflow", []string{"", "visible", "hidden", "clip"}}, {"overflow-x", []string{"", "visible", "hidden", "clip"}}, {"overflow-y", []string{"", "visible", "hidden", "clip"}},
		{"border-radius", nil}, {"box-shadow", nil},
		{"box-sizing", []string{"", "content-box", "border-box"}},
	} {
		if !enforceDimensions && (property.name == "width" || property.name == "height" || strings.HasPrefix(property.name, "min-") || strings.HasPrefix(property.name, "max-")) {
			continue
		}
		value := strings.ToLower(strings.TrimSpace(decl[property.name]))
		if property.defaults == nil {
			if property.name == "border-radius" || property.name == "box-shadow" {
				continue // exact grammar is validated while producing BoxStyle.
			}
			if value == "" || value == "auto" || strings.HasPrefix(property.name, "max-") && value == "none" {
				continue
			}
			if !htmlUnifiedBoxLengthSyntax(value) {
				return htmlPlanUnsupported(tag, token, fmt.Sprintf("resolved %s %q is outside the exact box-model cohort", property.name, decl[property.name]))
			}
			continue
		}
		if !htmlUnifiedContainsString(property.defaults, value) {
			return htmlPlanUnsupported(tag, token, fmt.Sprintf("resolved %s %q is outside the exact box-model cohort", property.name, decl[property.name]))
		}
	}
	return nil
}

func htmlUnifiedBoxLengthSyntax(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if strings.HasSuffix(value, "%") {
		number := strings.TrimSpace(strings.TrimSuffix(value, "%"))
		var parsed float64
		if _, err := fmt.Sscan(number, &parsed); err != nil || !finiteNumbers(parsed) || parsed < 0 || parsed > 1000 {
			return false
		}
		return true
	}
	_, err := htmlPlanStrictTableLength(value, func(points float64) float64 { return points })
	return err == nil
}

func htmlUnifiedValidateNonFlexBoxConstraints(compiled *CompiledHTML) error {
	for _, node := range compiled.nodeIndexes {
		token := node.Token
		resolved := compiled.unifiedResolved[token]
		if resolved.flexItem != nil || resolved.flexContainer != nil {
			continue
		}
		tag := compiled.tokens[token].Str
		decl := resolved.decl
		if tag == "img" || tag == "table" || tag == "td" || tag == "th" {
			decl = cloneStringMap(decl)
			delete(decl, "width")
			delete(decl, "min-width")
			delete(decl, "max-width")
			if tag == "img" {
				delete(decl, "height")
				delete(decl, "min-height")
				delete(decl, "max-height")
			}
		}
		if err := htmlUnifiedValidateBoxConstraints(tag, token, decl, true); err != nil {
			return err
		}
	}
	return nil
}

func htmlUnifiedApplyBoxSpacing(target *layout.Spacing, prefix string, decl map[string]string, pointsToUnits func(float64) float64) error {
	if value := strings.TrimSpace(decl[prefix]); value != "" {
		if strings.Contains(value, "%") {
			return nil // resolved against the exact containing block during lowering
		}
		parts := strings.Fields(value)
		if len(parts) < 1 || len(parts) > 4 {
			return fmt.Errorf("resolved %s %q must contain one through four fixed non-negative lengths", prefix, value)
		}
		values := make([]float64, len(parts))
		for index, part := range parts {
			length, err := htmlPlanStrictTableLength(part, pointsToUnits)
			if err != nil {
				return fmt.Errorf("resolved %s %q %v", prefix, value, err)
			}
			values[index] = length
		}
		switch len(values) {
		case 1:
			*target = layout.Spacing{Top: values[0], Right: values[0], Bottom: values[0], Left: values[0]}
		case 2:
			*target = layout.Spacing{Top: values[0], Right: values[1], Bottom: values[0], Left: values[1]}
		case 3:
			*target = layout.Spacing{Top: values[0], Right: values[1], Bottom: values[2], Left: values[1]}
		case 4:
			*target = layout.Spacing{Top: values[0], Right: values[1], Bottom: values[2], Left: values[3]}
		}
	}
	sides := []struct {
		name   string
		target *float64
	}{
		{prefix + "-top", &target.Top}, {prefix + "-right", &target.Right},
		{prefix + "-bottom", &target.Bottom}, {prefix + "-left", &target.Left},
	}
	for _, side := range sides {
		if value := strings.TrimSpace(decl[side.name]); value != "" {
			if strings.HasSuffix(value, "%") {
				continue // resolved against the exact containing block during lowering
			}
			length, err := htmlPlanStrictTableLength(value, pointsToUnits)
			if err != nil {
				return fmt.Errorf("resolved %s %q %v", side.name, value, err)
			}
			*side.target = length
		}
	}
	return nil
}

func htmlUnifiedResolveBlockBox(compiled *CompiledHTML, token int, availableWidth, availableHeight float64, pointsToUnits func(float64) float64) (layout.BoxStyle, error) {
	resolved := compiled.unifiedResolved[token]
	box := resolved.box
	decl := resolved.decl
	parseLength := func(property, raw string, relative float64) (float64, error) {
		raw = strings.ToLower(strings.TrimSpace(raw))
		if raw == "" || raw == "auto" || strings.HasPrefix(property, "max-") && raw == "none" {
			return 0, nil
		}
		if strings.HasSuffix(raw, "%") {
			if relative <= 0 {
				return 0, fmt.Errorf("resolved %s percentage requires a definite containing block", property)
			}
			var percent float64
			if _, err := fmt.Sscan(strings.TrimSpace(strings.TrimSuffix(raw, "%")), &percent); err != nil || !finiteNumbers(percent) || percent < 0 || percent > 1000 {
				return 0, fmt.Errorf("resolved %s %q is invalid", property, raw)
			}
			value := relative * percent / 100
			if !finiteNumbers(value) {
				return 0, fmt.Errorf("resolved %s percentage overflows", property)
			}
			return value, nil
		}
		value, err := htmlPlanStrictTableLength(raw, pointsToUnits)
		if err != nil {
			return 0, fmt.Errorf("resolved %s %q %v", property, raw, err)
		}
		return value, nil
	}
	applySpacing := func(target *layout.Spacing, prefix string) error {
		if raw := strings.TrimSpace(decl[prefix]); raw != "" {
			parts := strings.Fields(raw)
			if len(parts) < 1 || len(parts) > 4 {
				return fmt.Errorf("resolved %s must contain one through four lengths", prefix)
			}
			values := make([]float64, len(parts))
			for index, part := range parts {
				value, err := parseLength(prefix, part, availableWidth)
				if err != nil {
					return err
				}
				values[index] = value
			}
			switch len(values) {
			case 1:
				*target = layout.Spacing{Top: values[0], Right: values[0], Bottom: values[0], Left: values[0]}
			case 2:
				*target = layout.Spacing{Top: values[0], Right: values[1], Bottom: values[0], Left: values[1]}
			case 3:
				*target = layout.Spacing{Top: values[0], Right: values[1], Bottom: values[2], Left: values[1]}
			case 4:
				*target = layout.Spacing{Top: values[0], Right: values[1], Bottom: values[2], Left: values[3]}
			}
		}
		for _, side := range []struct {
			name string
			out  *float64
		}{{"top", &target.Top}, {"right", &target.Right}, {"bottom", &target.Bottom}, {"left", &target.Left}} {
			if raw := strings.TrimSpace(decl[prefix+"-"+side.name]); raw != "" {
				value, err := parseLength(prefix+"-"+side.name, raw, availableWidth)
				if err != nil {
					return err
				}
				*side.out = value
			}
		}
		return nil
	}
	if err := applySpacing(&box.Margin, "margin"); err != nil {
		return layout.BoxStyle{}, htmlPlanUnsupported(compiled.tokens[token].Str, token, err.Error())
	}
	if err := applySpacing(&box.Padding, "padding"); err != nil {
		return layout.BoxStyle{}, htmlPlanUnsupported(compiled.tokens[token].Str, token, err.Error())
	}
	boxSizing := strings.ToLower(strings.TrimSpace(decl["box-sizing"]))
	if boxSizing == "" {
		boxSizing = "content-box"
	}
	horizontalInsets := box.Padding.Left + box.Padding.Right + box.Border.Left.Width + box.Border.Right.Width
	verticalInsets := box.Padding.Top + box.Padding.Bottom + box.Border.Top.Width + box.Border.Bottom.Width
	for _, dimension := range []struct {
		name     string
		relative float64
		insets   float64
		target   *float64
	}{
		{"width", availableWidth, horizontalInsets, &box.Width}, {"height", availableHeight, verticalInsets, &box.Height},
		{"min-width", availableWidth, horizontalInsets, &box.MinWidth}, {"min-height", availableHeight, verticalInsets, &box.MinHeight},
		{"max-width", availableWidth, horizontalInsets, &box.MaxWidth}, {"max-height", availableHeight, verticalInsets, &box.MaxHeight},
	} {
		value, err := parseLength(dimension.name, decl[dimension.name], dimension.relative)
		if err != nil {
			return layout.BoxStyle{}, htmlPlanUnsupported(compiled.tokens[token].Str, token, err.Error())
		}
		if value > 0 && boxSizing == "content-box" {
			value += dimension.insets
		}
		*dimension.target = value
	}
	if box.Width > 0 {
		if box.Width < box.MinWidth {
			box.Width = box.MinWidth
		}
		if box.MaxWidth > 0 && box.Width > box.MaxWidth {
			box.Width = box.MaxWidth
		}
	}
	if box.Height > 0 {
		if box.Height < box.MinHeight {
			box.Height = box.MinHeight
		}
		if box.MaxHeight > 0 && box.Height > box.MaxHeight {
			box.Height = box.MaxHeight
		}
	}
	overflowX := strings.ToLower(strings.TrimSpace(firstNonEmpty(decl["overflow-x"], decl["overflow"], "visible")))
	overflowY := strings.ToLower(strings.TrimSpace(firstNonEmpty(decl["overflow-y"], decl["overflow"], "visible")))
	if overflowX == "clip" {
		overflowX = "hidden"
	}
	if overflowY == "clip" {
		overflowY = "hidden"
	}
	if overflowX != overflowY {
		return layout.BoxStyle{}, htmlPlanUnsupported(compiled.tokens[token].Str, token, "resolved overflow axes must use the same visible or hidden policy")
	}
	box.Overflow = ""
	if overflowX == "hidden" {
		box.Overflow = "hidden"
	}
	if box.Width > 0 && box.Width+box.Margin.Left+box.Margin.Right > availableWidth || box.MinWidth > 0 && box.MinWidth+box.Margin.Left+box.Margin.Right > availableWidth {
		return layout.BoxStyle{}, htmlPlanUnsupported(compiled.tokens[token].Str, token, "resolved box width exceeds its containing block")
	}
	return box, nil
}

func htmlUnifiedBoxContentBounds(box layout.BoxStyle, availableWidth, availableHeight float64) (float64, float64, error) {
	borderWidth := availableWidth - box.Margin.Left - box.Margin.Right
	if box.Width > 0 {
		borderWidth = box.Width
	}
	if borderWidth < box.MinWidth {
		borderWidth = box.MinWidth
	}
	if box.MaxWidth > 0 && borderWidth > box.MaxWidth {
		borderWidth = box.MaxWidth
	}
	contentWidth := borderWidth - box.Border.Left.Width - box.Padding.Left - box.Padding.Right - box.Border.Right.Width
	contentHeight := float64(0)
	if box.Height > 0 {
		contentHeight = box.Height - box.Border.Top.Width - box.Padding.Top - box.Padding.Bottom - box.Border.Bottom.Width
	}
	if contentWidth <= 0 || contentHeight < 0 {
		return 0, 0, fmt.Errorf("resolved box edges leave an invalid content area")
	}
	return contentWidth, contentHeight, nil
}

func htmlUnifiedContainsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func htmlUnifiedVisualBox(box layout.BoxStyle) bool {
	box.KeepTogether, box.KeepWithNext, box.Orphans, box.Widows = false, false, 0, 0
	return box != (layout.BoxStyle{})
}
