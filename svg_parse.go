/****************************************************************************
 * Software: GoPDFKit                                                         *
 * License:  MIT License                                                    *
 *                                                                          *
 * Copyright (c) 2026 cssBruno                                              *
 ****************************************************************************/

package gopdfkit

import (
	"encoding/xml"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
)

var pathCmdSub *strings.Replacer

func init() {
	pathCmdSub = strings.NewReplacer(",", " ", "A", " A ", "a", " a ", "L", " L ", "l", " l ", "C", " C ", "c", " c ", "M", " M ", "m", " m ", "H", " H ", "h", " h ", "S", " S ", "s", " s ", "T", " T ", "t", " t ", "V", " V ", "v", " v ", "Q", " Q ", "q", " q ", "Z", " Z ", "z", " z ")
}

func pathFields(pathStr string) []string {
	return svgNumberFields(pathCmdSub.Replace(pathStr))
}

func svgNumberFields(numStr string) []string {
	strList := strings.Fields(strings.Replace(numStr, ",", " ", -1))
	fields := make([]string, 0, len(strList))
	for _, str := range strList {
		start := 0
		for j := 1; j < len(str); j++ {
			if (str[j] == '-' || str[j] == '+') && str[j-1] != 'e' && str[j-1] != 'E' {
				if start < j {
					fields = append(fields, str[start:j])
				}
				start = j
			}
		}
		if start < len(str) {
			fields = append(fields, str[start:])
		}
	}
	return fields
}

func pathArgStart(c byte) bool {
	return c == '-' || c == '+' || c == '.' || (c >= '0' && c <= '9')
}

func svgParseNumber(value, context string) (float64, error) {
	n, err := strconv.ParseFloat(value, 64)
	if err != nil || !svgFinite(n) {
		return 0, fmt.Errorf("invalid SVG %s value: %s", context, value)
	}
	return n, nil
}

type svgPathRawSegment struct {
	cmd  byte
	args []float64
}

func svgPathCommandArgCount(cmd byte) (int, bool) {
	switch cmd {
	case 'M', 'm', 'L', 'l', 'T', 't':
		return 2, true
	case 'H', 'h', 'V', 'v':
		return 1, true
	case 'S', 's', 'Q', 'q':
		return 4, true
	case 'C', 'c':
		return 6, true
	case 'A', 'a':
		return 7, true
	case 'Z', 'z':
		return 0, true
	default:
		return 0, false
	}
}

func svgPathCommandRelative(cmd byte) bool {
	return cmd >= 'a' && cmd <= 'z'
}

func svgPathCommandUpper(cmd byte) byte {
	if svgPathCommandRelative(cmd) {
		return cmd - ('a' - 'A')
	}
	return cmd
}

func svgArcFlag(value float64, name string) (bool, error) {
	switch value {
	case 0:
		return false, nil
	case 1:
		return true, nil
	default:
		return false, fmt.Errorf("invalid SVG arc %s flag: %.12g", name, value)
	}
}

func normalizePathSegments(raw []svgPathRawSegment) ([]SVGSegment, error) {
	segs := make([]SVGSegment, 0, len(raw))
	var x, y, startX, startY float64
	var cubicCtrlX, cubicCtrlY, quadCtrlX, quadCtrlY float64
	var prevCmd byte
	point := func(args []float64, pos int, relative bool) (float64, float64) {
		px, py := args[pos], args[pos+1]
		if relative {
			px += x
			py += y
		}
		return px, py
	}
	for _, rawSeg := range raw {
		cmd := svgPathCommandUpper(rawSeg.cmd)
		relative := svgPathCommandRelative(rawSeg.cmd)
		args := rawSeg.args
		switch cmd {
		case 'M':
			x, y = point(args, 0, relative)
			startX, startY = x, y
			segs = append(segs, svgSegment('M', x, y))
		case 'L':
			x, y = point(args, 0, relative)
			segs = append(segs, svgSegment('L', x, y))
		case 'H':
			if relative {
				x += args[0]
			} else {
				x = args[0]
			}
			segs = append(segs, svgSegment('H', x))
		case 'V':
			if relative {
				y += args[0]
			} else {
				y = args[0]
			}
			segs = append(segs, svgSegment('V', y))
		case 'C':
			x0, y0 := point(args, 0, relative)
			x1, y1 := point(args, 2, relative)
			x, y = point(args, 4, relative)
			cubicCtrlX, cubicCtrlY = x1, y1
			segs = append(segs, svgSegment('C', x0, y0, x1, y1, x, y))
		case 'S':
			x0, y0 := x, y
			if prevCmd == 'C' || prevCmd == 'S' {
				x0 = 2*x - cubicCtrlX
				y0 = 2*y - cubicCtrlY
			}
			x1, y1 := point(args, 0, relative)
			x, y = point(args, 2, relative)
			cubicCtrlX, cubicCtrlY = x1, y1
			segs = append(segs, svgSegment('C', x0, y0, x1, y1, x, y))
		case 'Q':
			x0, y0 := point(args, 0, relative)
			x, y = point(args, 2, relative)
			quadCtrlX, quadCtrlY = x0, y0
			segs = append(segs, svgSegment('Q', x0, y0, x, y))
		case 'T':
			x0, y0 := x, y
			if prevCmd == 'Q' || prevCmd == 'T' {
				x0 = 2*x - quadCtrlX
				y0 = 2*y - quadCtrlY
			}
			x, y = point(args, 0, relative)
			quadCtrlX, quadCtrlY = x0, y0
			segs = append(segs, svgSegment('Q', x0, y0, x, y))
		case 'A':
			largeArc, err := svgArcFlag(args[3], "large-arc")
			if err != nil {
				return nil, err
			}
			sweep, err := svgArcFlag(args[4], "sweep")
			if err != nil {
				return nil, err
			}
			endX, endY := point(args, 5, relative)
			arcSegs, err := svgArcSegments(x, y, args[0], args[1], args[2], largeArc, sweep, endX, endY)
			if err != nil {
				return nil, err
			}
			segs = append(segs, arcSegs...)
			x, y = endX, endY
		case 'Z':
			segs = append(segs, svgSegment('Z'))
			x, y = startX, startY
		}
		prevCmd = cmd
	}
	return segs, nil
}

func svgVectorAngle(ux, uy, vx, vy float64) float64 {
	denominator := math.Hypot(ux, uy) * math.Hypot(vx, vy)
	if denominator == 0 {
		return 0
	}
	ratio := (ux*vx + uy*vy) / denominator
	if ratio < -1 {
		ratio = -1
	} else if ratio > 1 {
		ratio = 1
	}
	angle := math.Acos(ratio)
	if ux*vy-uy*vx < 0 {
		angle = -angle
	}
	return angle
}

func svgArcPoint(cx, cy, rx, ry, cosPhi, sinPhi, theta float64) (float64, float64) {
	cosTheta, sinTheta := math.Cos(theta), math.Sin(theta)
	return cx + rx*cosPhi*cosTheta - ry*sinPhi*sinTheta, cy + rx*sinPhi*cosTheta + ry*cosPhi*sinTheta
}

func svgArcDerivative(rx, ry, cosPhi, sinPhi, theta float64) (float64, float64) {
	cosTheta, sinTheta := math.Cos(theta), math.Sin(theta)
	return -rx*cosPhi*sinTheta - ry*sinPhi*cosTheta, -rx*sinPhi*sinTheta + ry*cosPhi*cosTheta
}

func svgArcSegments(x1, y1, rx, ry, xAxisRotation float64, largeArc, sweep bool, x2, y2 float64) ([]SVGSegment, error) {
	if !svgFinite(rx) || !svgFinite(ry) || !svgFinite(xAxisRotation) || !svgFinite(x2) || !svgFinite(y2) {
		return nil, fmt.Errorf("invalid SVG arc: non-finite value")
	}
	rx, ry = math.Abs(rx), math.Abs(ry)
	if x1 == x2 && y1 == y2 {
		return nil, nil
	}
	if rx == 0 || ry == 0 {
		return []SVGSegment{svgSegment('L', x2, y2)}, nil
	}
	phi := xAxisRotation * math.Pi / 180
	cosPhi, sinPhi := math.Cos(phi), math.Sin(phi)
	dx, dy := (x1-x2)/2, (y1-y2)/2
	x1p := cosPhi*dx + sinPhi*dy
	y1p := -sinPhi*dx + cosPhi*dy
	rx2, ry2 := rx*rx, ry*ry
	x1p2, y1p2 := x1p*x1p, y1p*y1p
	lambda := x1p2/rx2 + y1p2/ry2
	if lambda > 1 {
		scale := math.Sqrt(lambda)
		rx *= scale
		ry *= scale
		rx2, ry2 = rx*rx, ry*ry
	}
	denominator := rx2*y1p2 + ry2*x1p2
	if denominator == 0 {
		return []SVGSegment{svgSegment('L', x2, y2)}, nil
	}
	sign := 1.0
	if largeArc == sweep {
		sign = -1
	}
	centerScaleSq := (rx2*ry2 - rx2*y1p2 - ry2*x1p2) / denominator
	if centerScaleSq < 0 {
		centerScaleSq = 0
	}
	centerScale := sign * math.Sqrt(centerScaleSq)
	cxp := centerScale * rx * y1p / ry
	cyp := -centerScale * ry * x1p / rx
	cx := cosPhi*cxp - sinPhi*cyp + (x1+x2)/2
	cy := sinPhi*cxp + cosPhi*cyp + (y1+y2)/2
	v1x, v1y := (x1p-cxp)/rx, (y1p-cyp)/ry
	v2x, v2y := (-x1p-cxp)/rx, (-y1p-cyp)/ry
	theta1 := svgVectorAngle(1, 0, v1x, v1y)
	delta := svgVectorAngle(v1x, v1y, v2x, v2y)
	if !sweep && delta > 0 {
		delta -= 2 * math.Pi
	} else if sweep && delta < 0 {
		delta += 2 * math.Pi
	}
	pieces := int(math.Ceil(math.Abs(delta) / (math.Pi / 2)))
	if pieces < 1 {
		pieces = 1
	}
	step := delta / float64(pieces)
	segs := make([]SVGSegment, 0, pieces)
	for j := 0; j < pieces; j++ {
		start := theta1 + float64(j)*step
		end := start + step
		p1x, p1y := svgArcPoint(cx, cy, rx, ry, cosPhi, sinPhi, start)
		p2x, p2y := svgArcPoint(cx, cy, rx, ry, cosPhi, sinPhi, end)
		d1x, d1y := svgArcDerivative(rx, ry, cosPhi, sinPhi, start)
		d2x, d2y := svgArcDerivative(rx, ry, cosPhi, sinPhi, end)
		alpha := 4.0 / 3.0 * math.Tan(step/4.0)
		segs = append(segs, svgSegment('C', p1x+alpha*d1x, p1y+alpha*d1y, p2x-alpha*d2x, p2y-alpha*d2y, p2x, p2y))
	}
	last := &segs[len(segs)-1]
	last.Arg[4], last.Arg[5] = x2, y2
	return segs, nil
}

func pathParse(pathStr string) (segs []SVGSegment, err error) {
	raw := []svgPathRawSegment{}
	var args []float64
	var cmd byte
	var argCount int
	strList := pathFields(pathStr)
	for j, str := range strList {
		if str == "" {
			continue
		}
		if !pathArgStart(str[0]) {
			if len(args) > 0 {
				return nil, fmt.Errorf("expecting additional (%d) numeric arguments", argCount-len(args))
			}
			cmd = str[0]
			var ok bool
			argCount, ok = svgPathCommandArgCount(cmd)
			if !ok {
				return nil, fmt.Errorf("expecting SVG path command at position %d, got %s", j, str)
			}
			if argCount == 0 {
				raw = append(raw, svgPathRawSegment{cmd: cmd})
				cmd = 0
			}
			continue
		}
		if cmd == 0 || argCount == 0 {
			return nil, fmt.Errorf("expecting SVG path command at position %d, got %s", j, str)
		}
		n, err := svgParseNumber(str, "path")
		if err != nil {
			return nil, err
		}
		args = append(args, n)
		if len(args) == argCount {
			rawArgs := make([]float64, len(args))
			copy(rawArgs, args)
			raw = append(raw, svgPathRawSegment{cmd: cmd, args: rawArgs})
			args = args[:0]
			if cmd == 'M' {
				cmd = 'L'
			} else if cmd == 'm' {
				cmd = 'l'
			} else {
				argCount, _ = svgPathCommandArgCount(cmd)
			}
		}
	}
	if len(args) > 0 {
		return nil, fmt.Errorf("expecting additional (%d) numeric arguments", argCount-len(args))
	}
	return normalizePathSegments(raw)
}

type svgNode struct {
	XMLName  xml.Name
	Attrs    []xml.Attr `xml:",any,attr"`
	Text     string     `xml:",chardata"`
	Children []svgNode  `xml:",any"`
}

func (node svgNode) attr(name string) string {
	for _, attr := range node.Attrs {
		if attr.Name.Local == name {
			return attr.Value
		}
	}
	return ""
}

func svgGradientStop(node svgNode) (SVGGradientStop, bool) {
	if node.XMLName.Local != "stop" {
		return SVGGradientStop{}, false
	}
	declarations := parseStyleDeclarations(node.attr("style"))
	for _, attr := range node.Attrs {
		if attr.Name.Local != "style" {
			if declarations == nil {
				declarations = map[string]string{}
			}
			declarations[strings.ToLower(attr.Name.Local)] = attr.Value
		}
	}
	offset, ok := svgGradientOffset(declarations["offset"])
	if !ok {
		return SVGGradientStop{}, false
	}
	color, ok := parseCSSPaint(declarations["stop-color"])
	if !ok || color.None {
		color = CSSColorType{Set: true}
	}
	opacity := 1.0
	if n, ok := parseSVGOpacity(declarations["stop-opacity"]); ok {
		opacity = n
	}
	return SVGGradientStop{Offset: offset, Color: color, Opacity: opacity}, true
}

func svgResolveGradient(id string, refs map[string]svgNode, cache map[string]SVGGradient, seen map[string]bool) (SVGGradient, bool, error) {
	if gradient, ok := cache[id]; ok {
		return gradient, gradient.Set, nil
	}
	if seen[id] {
		return SVGGradient{}, false, fmt.Errorf("recursive SVG gradient reference: %s", id)
	}
	node, ok := refs[id]
	if !ok {
		return SVGGradient{}, false, nil
	}
	if node.XMLName.Local != "linearGradient" && node.XMLName.Local != "radialGradient" {
		return SVGGradient{}, false, nil
	}
	seen[id] = true
	gradient := svgDefaultGradient(node.XMLName.Local)
	if refID := svgUseRef(node); refID != "" {
		if base, ok, err := svgResolveGradient(refID, refs, cache, seen); err != nil {
			return SVGGradient{}, false, err
		} else if ok {
			gradient = base
			gradient.Kind = svgGradientKind(node.XMLName.Local)
			gradient.Set = true
		}
	}
	gradient = svgApplyGradientNode(gradient, node)
	delete(seen, id)
	cache[id] = gradient
	return gradient, gradient.Set, nil
}

func svgDefaultGradient(name string) SVGGradient {
	if name == "radialGradient" {
		return SVGGradient{Set: true, Kind: "radial", Units: "objectBoundingBox", CX: 0.5, CY: 0.5, FX: 0.5, FY: 0.5, R: 0.5}
	}
	return SVGGradient{Set: true, Kind: "linear", Units: "objectBoundingBox", X1: 0, Y1: 0, X2: 1, Y2: 0}
}

func svgApplyGradientNode(gradient SVGGradient, node svgNode) SVGGradient {
	gradient.Set = true
	gradient.Kind = svgGradientKind(node.XMLName.Local)
	gradient.Units = svgGradientUnits(node.attr("gradientUnits"), gradient.Units)
	if gradient.Kind == "linear" {
		gradient.X1 = svgGradientCoordinate(node.attr("x1"), gradient.X1, gradient.Units)
		gradient.Y1 = svgGradientCoordinate(node.attr("y1"), gradient.Y1, gradient.Units)
		gradient.X2 = svgGradientCoordinate(node.attr("x2"), gradient.X2, gradient.Units)
		gradient.Y2 = svgGradientCoordinate(node.attr("y2"), gradient.Y2, gradient.Units)
	} else {
		gradient.CX = svgGradientCoordinate(node.attr("cx"), gradient.CX, gradient.Units)
		gradient.CY = svgGradientCoordinate(node.attr("cy"), gradient.CY, gradient.Units)
		gradient.R = svgGradientCoordinate(node.attr("r"), gradient.R, gradient.Units)
		gradient.FX = svgGradientCoordinate(node.attr("fx"), gradient.FX, gradient.Units)
		gradient.FY = svgGradientCoordinate(node.attr("fy"), gradient.FY, gradient.Units)
	}
	stops := make([]SVGGradientStop, 0, len(node.Children))
	for _, child := range node.Children {
		if stop, ok := svgGradientStop(child); ok {
			stops = append(stops, stop)
		}
	}
	if len(stops) > 0 {
		sort.SliceStable(stops, func(i, j int) bool {
			return stops[i].Offset < stops[j].Offset
		})
		gradient.Stops = stops
	}
	return gradient
}

func svgDefaultPattern() SVGPattern {
	return SVGPattern{Set: true, Units: "objectBoundingBox"}
}

func svgApplyPatternNode(pattern SVGPattern, node svgNode) SVGPattern {
	pattern.Set = true
	pattern.Units = svgPatternUnits(node.attr("patternUnits"), pattern.Units)
	pattern.X = svgPatternCoordinate(node.attr("x"), pattern.X, pattern.Units)
	pattern.Y = svgPatternCoordinate(node.attr("y"), pattern.Y, pattern.Units)
	pattern.Wd = svgPatternCoordinate(node.attr("width"), pattern.Wd, pattern.Units)
	pattern.Ht = svgPatternCoordinate(node.attr("height"), pattern.Ht, pattern.Units)
	return pattern
}

func svgResolvePattern(id string, refs map[string]svgNode, gradients map[string]SVGGradient, cache map[string]SVGPattern, rules []htmlCSSRule, ancestors []HTMLSegmentType, depth int, seen map[string]bool) (SVGPattern, bool, error) {
	if depth > svgMaxNestingDepth {
		return SVGPattern{}, false, fmt.Errorf("SVG nesting depth exceeds %d", svgMaxNestingDepth)
	}
	if pattern, ok := cache[id]; ok {
		return pattern, pattern.Set, nil
	}
	if seen[id] {
		return SVGPattern{}, false, fmt.Errorf("recursive SVG pattern reference: %s", id)
	}
	node, ok := refs[id]
	if !ok || node.XMLName.Local != "pattern" {
		return SVGPattern{}, false, nil
	}
	seen[id] = true
	pattern := svgDefaultPattern()
	if refID := svgUseRef(node); refID != "" {
		if base, ok, err := svgResolvePattern(refID, refs, gradients, cache, rules, ancestors, depth+1, seen); err != nil {
			return SVGPattern{}, false, err
		} else if ok {
			pattern = base
			pattern.Set = true
		}
	}
	pattern = svgApplyPatternNode(pattern, node)
	if pattern.Wd <= 0 || pattern.Ht <= 0 {
		delete(seen, id)
		cache[id] = SVGPattern{}
		return SVGPattern{}, false, nil
	}
	elements, err := svgPatternElements(node, pattern, refs, gradients, rules, ancestors, depth+1)
	if err != nil {
		return SVGPattern{}, false, err
	}
	if len(elements) > 0 {
		pattern.Elements = elements
	}
	delete(seen, id)
	cache[id] = pattern
	return pattern, pattern.Set && len(pattern.Elements) > 0, nil
}

func svgPatternElements(node svgNode, pattern SVGPattern, refs map[string]svgNode, gradients map[string]SVGGradient, rules []htmlCSSRule, ancestors []HTMLSegmentType, depth int) ([]SVGElement, error) {
	transform := svgIdentityMatrix()
	if viewTransform, err := svgViewBoxTransform(node.attr("viewBox"), node.attr("preserveAspectRatio"), pattern.Wd, pattern.Ht); err != nil {
		return nil, err
	} else {
		transform = viewTransform
	}
	style := svgNodeStyle(node, SVGStyle{}, rules, ancestors)
	sig := SVG{}
	for _, child := range node.Children {
		if err := svgCollectDepth(child, style, transform, &sig, refs, gradients, nil, rules, append(ancestors, svgHTMLSegment(node)), depth+1, false); err != nil {
			return nil, err
		}
	}
	return sig.Elements, nil
}

func svgCollect(node svgNode, style SVGStyle, transform svgMatrix, sig *SVG) error {
	refs := map[string]svgNode{}
	svgIndexRefs(node, refs)
	gradients := map[string]SVGGradient{}
	patterns := map[string]SVGPattern{}
	rules := svgCollectStyleRules(node)
	return svgCollectDepth(node, style, transform, sig, refs, gradients, patterns, rules, nil, 0, false)
}

func svgCollectStyleRules(node svgNode) []htmlCSSRule {
	rules := []htmlCSSRule{}
	if node.XMLName.Local == "style" {
		rules = append(rules, parseHTMLCSSRules(node.Text)...)
	}
	for _, child := range node.Children {
		rules = append(rules, svgCollectStyleRules(child)...)
		if len(rules) >= htmlMaxCSSRules {
			return rules[:htmlMaxCSSRules]
		}
	}
	return rules
}

func svgHTMLSegment(node svgNode) HTMLSegmentType {
	attrs := map[string]string{}
	for _, attr := range node.Attrs {
		attrs[strings.ToLower(attr.Name.Local)] = attr.Value
	}
	return HTMLSegmentType{Cat: 'O', Str: strings.ToLower(node.XMLName.Local), Attr: attrs}
}

func svgIndexRefs(node svgNode, refs map[string]svgNode) {
	if id := strings.TrimSpace(node.attr("id")); id != "" {
		refs[id] = node
	}
	for _, child := range node.Children {
		svgIndexRefs(child, refs)
	}
}

func svgUseRef(node svgNode) string {
	ref := strings.TrimSpace(node.attr("href"))
	if ref == "" {
		return ""
	}
	if strings.HasPrefix(ref, "#") {
		return strings.TrimPrefix(ref, "#")
	}
	return ""
}

func svgUseTransform(node, ref svgNode, transform svgMatrix) (svgMatrix, error) {
	x, err := svgOptionalLength(node, "x", 0)
	if err != nil {
		return svgIdentityMatrix(), err
	}
	y, err := svgOptionalLength(node, "y", 0)
	if err != nil {
		return svgIdentityMatrix(), err
	}
	transform = transform.multiply(svgMatrix{A: 1, D: 1, E: x, F: y})
	minX, minY, viewWd, viewHt, ok, err := svgViewBox(ref.attr("viewBox"))
	if err != nil {
		return svgIdentityMatrix(), err
	}
	if !ok || viewWd <= 0 || viewHt <= 0 {
		return transform, nil
	}
	wd, wdOK := svgLength(node.attr("width"))
	ht, htOK := svgLength(node.attr("height"))
	if !wdOK || !htOK || wd <= 0 || ht <= 0 {
		return transform, nil
	}
	scaleX := wd / viewWd
	scaleY := ht / viewHt
	return transform.multiply(svgMatrix{A: scaleX, D: scaleY}).multiply(svgMatrix{A: 1, D: 1, E: -minX, F: -minY}), nil
}

func svgResolveStyleRefs(style SVGStyle, transform svgMatrix, refs map[string]svgNode, gradients map[string]SVGGradient, patterns map[string]SVGPattern, rules []htmlCSSRule, ancestors []HTMLSegmentType, depth int) (SVGStyle, error) {
	if style.FillRef != "" {
		gradient, ok, err := svgResolveGradient(style.FillRef, refs, gradients, map[string]bool{})
		if err != nil {
			return style, err
		}
		if ok {
			style.FillGradient = gradient
		} else if patterns != nil {
			pattern, ok, err := svgResolvePattern(style.FillRef, refs, gradients, patterns, rules, ancestors, depth+1, map[string]bool{})
			if err != nil {
				return style, err
			}
			if ok {
				style.FillPattern = pattern
			}
		}
	}
	if style.ClipRef != "" {
		clip, rule, err := svgResolveClipPath(style.ClipRef, refs, transform, rules, ancestors, depth+1)
		if err != nil {
			return style, err
		}
		style.ClipPath = clip
		style.ClipRule = rule
	}
	return style, nil
}

func svgResolveClipPath(id string, refs map[string]svgNode, transform svgMatrix, rules []htmlCSSRule, ancestors []HTMLSegmentType, depth int) ([]SVGSegment, string, error) {
	if depth > svgMaxNestingDepth {
		return nil, "", fmt.Errorf("SVG nesting depth exceeds %d", svgMaxNestingDepth)
	}
	node, ok := refs[id]
	if !ok || node.XMLName.Local != "clipPath" {
		return nil, "", nil
	}
	nodeTransform, err := svgTransform(node.attr("transform"))
	if err != nil {
		return nil, "", err
	}
	transform = transform.multiply(nodeTransform)
	rule := parseSVGFillRule(firstNonEmpty(node.attr("clip-rule"), node.attr("fill-rule")))
	segs, err := svgCollectClipSegments(node, SVGStyle{}, transform, refs, rules, ancestors, depth+1)
	return segs, rule, err
}

func svgCollectClipSegments(node svgNode, style SVGStyle, transform svgMatrix, refs map[string]svgNode, rules []htmlCSSRule, ancestors []HTMLSegmentType, depth int) ([]SVGSegment, error) {
	if depth > svgMaxNestingDepth {
		return nil, fmt.Errorf("SVG nesting depth exceeds %d", svgMaxNestingDepth)
	}
	el := svgHTMLSegment(node)
	style = svgNodeStyle(node, style, rules, ancestors)
	nodeTransform, err := svgTransform(node.attr("transform"))
	if err != nil {
		return nil, err
	}
	transform = transform.multiply(nodeTransform)
	if style.Hidden {
		return nil, nil
	}
	if node.XMLName.Local == "use" {
		refID := svgUseRef(node)
		if refID == "" {
			return nil, nil
		}
		ref, ok := refs[refID]
		if !ok {
			return nil, nil
		}
		useTransform, err := svgUseTransform(node, ref, transform)
		if err != nil {
			return nil, err
		}
		return svgCollectClipSegments(ref, style, useTransform, refs, rules, append(ancestors, el), depth+1)
	}
	out := []SVGSegment{}
	if path, ok, err := svgElementPath(node, style, transform); err != nil {
		return nil, err
	} else if ok {
		out = append(out, path.Segments...)
	}
	for _, child := range node.Children {
		childSegs, err := svgCollectClipSegments(child, style, transform, refs, rules, append(ancestors, el), depth+1)
		if err != nil {
			return nil, err
		}
		out = append(out, childSegs...)
	}
	return out, nil
}

func svgCollectDepth(node svgNode, style SVGStyle, transform svgMatrix, sig *SVG, refs map[string]svgNode, gradients map[string]SVGGradient, patterns map[string]SVGPattern, rules []htmlCSSRule, ancestors []HTMLSegmentType, depth int, renderingRef bool) error {
	if depth > svgMaxNestingDepth {
		return fmt.Errorf("SVG nesting depth exceeds %d", svgMaxNestingDepth)
	}
	el := svgHTMLSegment(node)
	style = svgNodeStyle(node, style, rules, ancestors)
	nodeTransform, err := svgTransform(node.attr("transform"))
	if err != nil {
		return err
	}
	transform = transform.multiply(nodeTransform)
	switch node.XMLName.Local {
	case "clipPath", "defs", "linearGradient", "radialGradient", "pattern":
		return nil
	case "symbol":
		if !renderingRef {
			return nil
		}
	case "use":
		refID := svgUseRef(node)
		if refID == "" {
			return nil
		}
		ref, ok := refs[refID]
		if !ok {
			return nil
		}
		useTransform, err := svgUseTransform(node, ref, transform)
		if err != nil {
			return err
		}
		return svgCollectDepth(ref, style, useTransform, sig, refs, gradients, patterns, rules, append(ancestors, el), depth+1, true)
	}
	style, err = svgResolveStyleRefs(style, transform, refs, gradients, patterns, rules, ancestors, depth)
	if err != nil {
		return err
	}
	path, ok, err := svgElementPath(node, style, transform)
	if err != nil {
		return err
	}
	if ok {
		if !svgPathFinite(path) {
			return fmt.Errorf("invalid SVG path: non-finite value")
		}
		sig.Paths = append(sig.Paths, path)
		sig.Segments = append(sig.Segments, path.Segments)
		sig.Elements = append(sig.Elements, SVGElement{Kind: "path", Path: path})
	}
	text, ok, err := svgText(node, style, transform)
	if err != nil {
		return err
	}
	if ok {
		if !svgTextFinite(text) {
			return fmt.Errorf("invalid SVG text: non-finite value")
		}
		sig.Texts = append(sig.Texts, text)
		sig.Elements = append(sig.Elements, SVGElement{Kind: "text", Text: text})
	}
	image, ok, err := svgImage(node, style, transform)
	if err != nil {
		return err
	}
	if ok {
		if !svgImageFinite(image) {
			return fmt.Errorf("invalid SVG image: non-finite value")
		}
		sig.Images = append(sig.Images, image)
		sig.Elements = append(sig.Elements, SVGElement{Kind: "image", Image: image})
	}
	for _, child := range node.Children {
		if err := svgCollectDepth(child, style, transform, sig, refs, gradients, patterns, rules, append(ancestors, el), depth+1, renderingRef); err != nil {
			return err
		}
	}
	return nil
}

// SVGParse parses a scalable vector graphics (SVG) buffer into a descriptor.
// Paths, lines, rectangles, circles, ellipses, polylines, polygons, text, and
// inherited presentation attributes are converted to data that SVGWrite can
// render.
func SVGParse(buf []byte) (sig SVG, err error) {
	var src svgNode
	err = xml.Unmarshal(buf, &src)
	if err == nil {
		sig.Wd, sig.Ht, err = svgExtent(src)
	}
	if err == nil {
		var transform svgMatrix
		transform, err = svgRootTransform(src, sig.Wd, sig.Ht)
		if err == nil {
			err = svgCollect(src, SVGStyle{}, transform, &sig)
		}
	}
	if err != nil {
		sig = SVG{}
	}
	return
}

func SVGFileParse(svgFileStr string) (sig SVG, err error) {
	var buf []byte
	buf, err = os.ReadFile(svgFileStr)
	if err == nil {
		sig, err = SVGParse(buf)
	}
	return
}
