// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"crypto/sha256"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const maxSVGSourceBytes = 4 * 1024 * 1024

const svgParseCacheLimit = 32

var svgParseCache = struct {
	sync.Mutex
	keys []svgParseCacheKey
	data map[svgParseCacheKey]SVG
}{data: make(map[svgParseCacheKey]SVG)}

type svgParseCacheKey struct {
	size int
	sum  [32]byte
}

func svgNumberFields(numStr string) []string {
	return svgScanFields(numStr, false)
}

func svgScanFields(s string, commands bool) []string {
	fields := make([]string, 0, 8)
	for i := 0; i < len(s); {
		for i < len(s) && (isASCIISpace(s[i]) || s[i] == ',') {
			i++
		}
		if i >= len(s) {
			break
		}
		start := i
		if commands && ((s[i] >= 'A' && s[i] <= 'Z') || (s[i] >= 'a' && s[i] <= 'z')) {
			fields = append(fields, s[i:i+1])
			i++
			continue
		}
		if s[i] == '-' || s[i] == '+' {
			i++
		}
		digits := 0
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
			digits++
		}
		if i < len(s) && s[i] == '.' {
			i++
			for i < len(s) && s[i] >= '0' && s[i] <= '9' {
				i++
				digits++
			}
		}
		if digits > 0 && i < len(s) && (s[i] == 'e' || s[i] == 'E') {
			exp := i + 1
			if exp < len(s) && (s[exp] == '-' || s[exp] == '+') {
				exp++
			}
			expDigits := exp
			for exp < len(s) && s[exp] >= '0' && s[exp] <= '9' {
				exp++
			}
			if exp > expDigits {
				i = exp
			}
		}
		if i == start || (i == start+1 && (s[start] == '-' || s[start] == '+')) {
			for i < len(s) && !isASCIISpace(s[i]) && s[i] != ',' {
				i++
			}
		}
		fields = append(fields, s[start:i])
	}
	return fields
}

func svgParseCacheGet(key svgParseCacheKey) (SVG, bool) {
	svgParseCache.Lock()
	defer svgParseCache.Unlock()
	sig, ok := svgParseCache.data[key]
	if !ok {
		return SVG{}, false
	}
	return cloneSVG(sig), true
}

func svgParseCachePut(key svgParseCacheKey, sig SVG) {
	svgParseCache.Lock()
	defer svgParseCache.Unlock()
	if _, ok := svgParseCache.data[key]; ok {
		svgParseCache.data[key] = cloneSVG(sig)
		return
	}
	if len(svgParseCache.keys) >= svgParseCacheLimit {
		evict := svgParseCache.keys[0]
		copy(svgParseCache.keys, svgParseCache.keys[1:])
		svgParseCache.keys = svgParseCache.keys[:len(svgParseCache.keys)-1]
		delete(svgParseCache.data, evict)
	}
	svgParseCache.keys = append(svgParseCache.keys, key)
	svgParseCache.data[key] = cloneSVG(sig)
}

func cloneSVG(sig SVG) SVG {
	sig.Segments = cloneSVGSegmentGroups(sig.Segments)
	sig.Paths = cloneSVGPaths(sig.Paths)
	sig.Texts = append([]SVGText(nil), sig.Texts...)
	sig.Images = append([]SVGImage(nil), sig.Images...)
	sig.Elements = cloneSVGElements(sig.Elements)
	return sig
}

func cloneSVGSegmentGroups(groups [][]SVGSegment) [][]SVGSegment {
	if len(groups) == 0 {
		return nil
	}
	out := make([][]SVGSegment, len(groups))
	for i, group := range groups {
		out[i] = append([]SVGSegment(nil), group...)
	}
	return out
}

func cloneSVGPaths(paths []SVGPath) []SVGPath {
	if len(paths) == 0 {
		return nil
	}
	out := make([]SVGPath, len(paths))
	for i, path := range paths {
		out[i] = cloneSVGPath(path)
	}
	return out
}

func cloneSVGPath(path SVGPath) SVGPath {
	path.Segments = append([]SVGSegment(nil), path.Segments...)
	path.Style = cloneSVGStyle(path.Style)
	return path
}

func cloneSVGStyle(style SVGStyle) SVGStyle {
	style.ClipPath = append([]SVGSegment(nil), style.ClipPath...)
	style.StrokeDashArray = append([]float64(nil), style.StrokeDashArray...)
	style.FillGradient.Stops = append([]SVGGradientStop(nil), style.FillGradient.Stops...)
	style.FillPattern.Elements = cloneSVGElements(style.FillPattern.Elements)
	return style
}

func cloneSVGElements(elements []SVGElement) []SVGElement {
	if len(elements) == 0 {
		return nil
	}
	out := make([]SVGElement, len(elements))
	for i, element := range elements {
		out[i] = element
		out[i].Path = cloneSVGPath(element.Path)
		out[i].Text.widthCache = svgTextWidthCache{}
		out[i].Image.Data = append([]byte(nil), element.Image.Data...)
	}
	return out
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

type svgPathNormalizer struct {
	segs                 []SVGSegment
	x, y                 float64
	startX, startY       float64
	cubicCtrlX           float64
	cubicCtrlY           float64
	quadCtrlX, quadCtrlY float64
	prevCmd              byte
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

func (p *svgPathNormalizer) point(args []float64, pos int, relative bool) (float64, float64) {
	x, y := args[pos], args[pos+1]
	if relative {
		x += p.x
		y += p.y
	}
	return x, y
}

func (p *svgPathNormalizer) append(rawCmd byte, args []float64) error {
	cmd := svgPathCommandUpper(rawCmd)
	relative := svgPathCommandRelative(rawCmd)
	switch cmd {
	case 'M':
		p.x, p.y = p.point(args, 0, relative)
		p.startX, p.startY = p.x, p.y
		p.segs = append(p.segs, svgSegment('M', p.x, p.y))
	case 'L':
		p.x, p.y = p.point(args, 0, relative)
		p.segs = append(p.segs, svgSegment('L', p.x, p.y))
	case 'H':
		if relative {
			p.x += args[0]
		} else {
			p.x = args[0]
		}
		p.segs = append(p.segs, svgSegment('H', p.x))
	case 'V':
		if relative {
			p.y += args[0]
		} else {
			p.y = args[0]
		}
		p.segs = append(p.segs, svgSegment('V', p.y))
	case 'C':
		x0, y0 := p.point(args, 0, relative)
		x1, y1 := p.point(args, 2, relative)
		p.x, p.y = p.point(args, 4, relative)
		p.cubicCtrlX, p.cubicCtrlY = x1, y1
		p.segs = append(p.segs, svgSegment('C', x0, y0, x1, y1, p.x, p.y))
	case 'S':
		x0, y0 := p.x, p.y
		if p.prevCmd == 'C' || p.prevCmd == 'S' {
			x0 = 2*p.x - p.cubicCtrlX
			y0 = 2*p.y - p.cubicCtrlY
		}
		x1, y1 := p.point(args, 0, relative)
		p.x, p.y = p.point(args, 2, relative)
		p.cubicCtrlX, p.cubicCtrlY = x1, y1
		p.segs = append(p.segs, svgSegment('C', x0, y0, x1, y1, p.x, p.y))
	case 'Q':
		x0, y0 := p.point(args, 0, relative)
		p.x, p.y = p.point(args, 2, relative)
		p.quadCtrlX, p.quadCtrlY = x0, y0
		p.segs = append(p.segs, svgSegment('Q', x0, y0, p.x, p.y))
	case 'T':
		x0, y0 := p.x, p.y
		if p.prevCmd == 'Q' || p.prevCmd == 'T' {
			x0 = 2*p.x - p.quadCtrlX
			y0 = 2*p.y - p.quadCtrlY
		}
		p.x, p.y = p.point(args, 0, relative)
		p.quadCtrlX, p.quadCtrlY = x0, y0
		p.segs = append(p.segs, svgSegment('Q', x0, y0, p.x, p.y))
	case 'A':
		largeArc, err := svgArcFlag(args[3], "large-arc")
		if err != nil {
			return err
		}
		sweep, err := svgArcFlag(args[4], "sweep")
		if err != nil {
			return err
		}
		endX, endY := p.point(args, 5, relative)
		p.segs, err = appendSVGArcSegments(p.segs, p.x, p.y, args[0], args[1], args[2], largeArc, sweep, endX, endY)
		if err != nil {
			return err
		}
		p.x, p.y = endX, endY
	case 'Z':
		p.segs = append(p.segs, svgSegment('Z'))
		p.x, p.y = p.startX, p.startY
	}
	p.prevCmd = cmd
	return nil
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
	return appendSVGArcSegments(nil, x1, y1, rx, ry, xAxisRotation, largeArc, sweep, x2, y2)
}

func appendSVGArcSegments(segs []SVGSegment, x1, y1, rx, ry, xAxisRotation float64, largeArc, sweep bool, x2, y2 float64) ([]SVGSegment, error) {
	if !svgFinite(rx) || !svgFinite(ry) || !svgFinite(xAxisRotation) || !svgFinite(x2) || !svgFinite(y2) {
		return nil, errors.New("invalid SVG arc: non-finite value")
	}
	rx, ry = math.Abs(rx), math.Abs(ry)
	if x1 == x2 && y1 == y2 {
		return segs, nil
	}
	if rx == 0 || ry == 0 {
		return append(segs, svgSegment('L', x2, y2)), nil
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
		return append(segs, svgSegment('L', x2, y2)), nil
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
	startLen := len(segs)
	if cap(segs)-len(segs) < pieces {
		grown := make([]SVGSegment, len(segs), len(segs)+pieces)
		copy(grown, segs)
		segs = grown
	}
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
	if len(segs) == startLen {
		return segs, nil
	}
	return segs, nil
}

func pathParse(pathStr string) (segs []SVGSegment, err error) {
	var args [7]float64
	argLen := 0
	var cmd byte
	var argCount int
	normalizer := svgPathNormalizer{segs: make([]SVGSegment, 0, svgPathSegmentCapacity(pathStr))}
	tokenIndex := 0
	for i := 0; i < len(pathStr); {
		for i < len(pathStr) && (isASCIISpace(pathStr[i]) || pathStr[i] == ',') {
			i++
		}
		if i >= len(pathStr) {
			break
		}
		start := i
		var token string
		c := pathStr[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			token = pathStr[i : i+1]
			i++
		} else if pathArgStart(c) {
			i = svgScanNumberEnd(pathStr, i)
			token = pathStr[start:i]
		} else {
			for i < len(pathStr) && !isASCIISpace(pathStr[i]) && pathStr[i] != ',' {
				i++
			}
			token = pathStr[start:i]
		}
		if token == "" {
			continue
		}
		if !pathArgStart(token[0]) {
			if argLen > 0 {
				return nil, fmt.Errorf("expecting additional (%d) numeric arguments", argCount-argLen)
			}
			cmd = token[0]
			var ok bool
			argCount, ok = svgPathCommandArgCount(cmd)
			if !ok {
				return nil, fmt.Errorf("expecting SVG path command at position %d, got %s", tokenIndex, token)
			}
			if argCount == 0 {
				if err := normalizer.append(cmd, nil); err != nil {
					return nil, err
				}
				cmd = 0
			}
			tokenIndex++
			continue
		}
		if cmd == 0 || argCount == 0 {
			return nil, fmt.Errorf("expecting SVG path command at position %d, got %s", tokenIndex, token)
		}
		n, err := svgParseNumber(token, "path")
		if err != nil {
			return nil, err
		}
		args[argLen] = n
		argLen++
		if argLen == argCount {
			if err := normalizer.append(cmd, args[:argCount]); err != nil {
				return nil, err
			}
			argLen = 0
			switch cmd {
			case 'M':
				cmd = 'L'
			case 'm':
				cmd = 'l'
			default:
				argCount, _ = svgPathCommandArgCount(cmd)
			}
		}
		tokenIndex++
	}
	if argLen > 0 {
		return nil, fmt.Errorf("expecting additional (%d) numeric arguments", argCount-argLen)
	}
	return normalizer.segs, nil
}

func svgPathSegmentCapacity(pathStr string) int {
	count := 0
	for i := 0; i < len(pathStr); i++ {
		c := pathStr[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			switch c {
			case 'e', 'E':
				if i > 0 && i+1 < len(pathStr) && pathArgStart(pathStr[i-1]) && (pathStr[i+1] == '-' || pathStr[i+1] == '+' || (pathStr[i+1] >= '0' && pathStr[i+1] <= '9')) {
					continue
				}
			case 'A', 'a':
				count += 2
				continue
			}
			if _, ok := svgPathCommandArgCount(c); ok {
				count++
			}
		}
	}
	if count < 8 {
		return 8
	}
	return count
}

func svgScanNumberEnd(s string, i int) int {
	start := i
	if s[i] == '-' || s[i] == '+' {
		i++
	}
	digits := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
		digits++
	}
	if i < len(s) && s[i] == '.' {
		i++
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
			digits++
		}
	}
	if digits > 0 && i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		exp := i + 1
		if exp < len(s) && (s[exp] == '-' || s[exp] == '+') {
			exp++
		}
		expDigits := exp
		for exp < len(s) && s[exp] >= '0' && s[exp] <= '9' {
			exp++
		}
		if exp > expDigits {
			i = exp
		}
	}
	if i == start || (i == start+1 && (s[start] == '-' || s[start] == '+')) {
		for i < len(s) && !isASCIISpace(s[i]) && s[i] != ',' {
			i++
		}
	}
	return i
}

type svgNode struct {
	XMLName  xml.Name
	Attrs    []xml.Attr      `xml:",any,attr"`
	Text     string          `xml:",chardata"`
	Children []svgNode       `xml:",any"`
	html     HTMLSegmentType `xml:"-"`
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
		less := func(i, j int) bool {
			return stops[i].Offset < stops[j].Offset
		}
		if !sort.SliceIsSorted(stops, less) {
			sort.SliceStable(stops, less)
		}
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

func svgResolvePattern(id string, refs map[string]svgNode, gradients map[string]SVGGradient, cache map[string]SVGPattern, clipCache map[svgClipCacheKey]svgClipCacheEntry, rules []htmlCSSRule, ancestors []HTMLSegmentType, depth int, seen map[string]bool) (SVGPattern, bool, error) {
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
		if base, ok, err := svgResolvePattern(refID, refs, gradients, cache, clipCache, rules, ancestors, depth+1, seen); err != nil {
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
	elements, err := svgPatternElements(node, pattern, refs, gradients, clipCache, rules, ancestors, depth+1)
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

func svgPatternElements(node svgNode, pattern SVGPattern, refs map[string]svgNode, gradients map[string]SVGGradient, clipCache map[svgClipCacheKey]svgClipCacheEntry, rules []htmlCSSRule, ancestors []HTMLSegmentType, depth int) ([]SVGElement, error) {
	transform, err := svgViewBoxTransform(node.attr("viewBox"), node.attr("preserveAspectRatio"), pattern.Wd, pattern.Ht)
	if err != nil {
		return nil, err
	}
	style := svgNodeStyle(node, SVGStyle{}, rules, ancestors)
	sig := SVG{}
	childAncestors := append(ancestors, svgHTMLSegment(node))
	for _, child := range node.Children {
		if err := svgCollectDepth(child, style, transform, &sig, refs, gradients, nil, clipCache, rules, childAncestors, depth+1, false); err != nil {
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
	clipCache := map[svgClipCacheKey]svgClipCacheEntry{}
	rules := svgCollectStyleRules(node)
	return svgCollectDepth(node, style, transform, sig, refs, gradients, patterns, clipCache, rules, nil, 0, false)
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
	if node.html.Str != "" || node.html.Attr != nil {
		return node.html
	}
	attrs := map[string]string{}
	for _, attr := range node.Attrs {
		attrs[strings.ToLower(attr.Name.Local)] = attr.Value
	}
	return HTMLSegmentType{Cat: 'O', Str: strings.ToLower(node.XMLName.Local), Attr: attrs}
}

func svgPrepareNodes(node *svgNode) {
	attrs := make(map[string]string, len(node.Attrs))
	for _, attr := range node.Attrs {
		attrs[strings.ToLower(attr.Name.Local)] = attr.Value
	}
	node.html = HTMLSegmentType{Cat: 'O', Str: strings.ToLower(node.XMLName.Local), Attr: attrs}
	for i := range node.Children {
		svgPrepareNodes(&node.Children[i])
	}
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

type svgClipCacheKey struct {
	id        string
	transform svgMatrix
}

type svgClipCacheEntry struct {
	segments []SVGSegment
	rule     string
}

func svgResolveStyleRefs(style SVGStyle, transform svgMatrix, refs map[string]svgNode, gradients map[string]SVGGradient, patterns map[string]SVGPattern, clipCache map[svgClipCacheKey]svgClipCacheEntry, rules []htmlCSSRule, ancestors []HTMLSegmentType, depth int) (SVGStyle, error) {
	if style.FillRef != "" {
		gradient, ok, err := svgResolveGradient(style.FillRef, refs, gradients, map[string]bool{})
		if err != nil {
			return style, err
		}
		if ok {
			style.FillGradient = gradient
		} else if patterns != nil {
			pattern, ok, err := svgResolvePattern(style.FillRef, refs, gradients, patterns, clipCache, rules, ancestors, depth+1, map[string]bool{})
			if err != nil {
				return style, err
			}
			if ok {
				style.FillPattern = pattern
			}
		}
	}
	if style.ClipRef != "" {
		clip, rule, err := svgResolveClipPath(style.ClipRef, refs, transform, clipCache, rules, ancestors, depth+1)
		if err != nil {
			return style, err
		}
		style.ClipPath = clip
		style.ClipRule = rule
	}
	return style, nil
}

func svgResolveClipPath(id string, refs map[string]svgNode, transform svgMatrix, cache map[svgClipCacheKey]svgClipCacheEntry, rules []htmlCSSRule, ancestors []HTMLSegmentType, depth int) ([]SVGSegment, string, error) {
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
	key := svgClipCacheKey{id: id, transform: transform}
	if cache != nil {
		if cached, ok := cache[key]; ok {
			return cached.segments, cached.rule, nil
		}
	}
	rule := parseSVGFillRule(firstNonEmpty(node.attr("clip-rule"), node.attr("fill-rule")))
	segs, err := svgCollectClipSegments(node, SVGStyle{}, transform, refs, cache, rules, ancestors, depth+1)
	if err == nil && cache != nil {
		cache[key] = svgClipCacheEntry{segments: segs, rule: rule}
	}
	return segs, rule, err
}

func svgCollectClipSegments(node svgNode, style SVGStyle, transform svgMatrix, refs map[string]svgNode, cache map[svgClipCacheKey]svgClipCacheEntry, rules []htmlCSSRule, ancestors []HTMLSegmentType, depth int) ([]SVGSegment, error) {
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
	childAncestors := append(ancestors, el)
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
		return svgCollectClipSegments(ref, style, useTransform, refs, cache, rules, childAncestors, depth+1)
	}
	out := []SVGSegment{}
	if path, ok, err := svgElementPath(node, style, transform); err != nil {
		return nil, err
	} else if ok {
		out = append(out, path.Segments...)
	}
	for _, child := range node.Children {
		childSegs, err := svgCollectClipSegments(child, style, transform, refs, cache, rules, childAncestors, depth+1)
		if err != nil {
			return nil, err
		}
		out = append(out, childSegs...)
	}
	return out, nil
}

func svgCollectDepth(node svgNode, style SVGStyle, transform svgMatrix, sig *SVG, refs map[string]svgNode, gradients map[string]SVGGradient, patterns map[string]SVGPattern, clipCache map[svgClipCacheKey]svgClipCacheEntry, rules []htmlCSSRule, ancestors []HTMLSegmentType, depth int, renderingRef bool) error {
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
	childAncestors := append(ancestors, el)
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
		return svgCollectDepth(ref, style, useTransform, sig, refs, gradients, patterns, clipCache, rules, childAncestors, depth+1, true)
	}
	style, err = svgResolveStyleRefs(style, transform, refs, gradients, patterns, clipCache, rules, ancestors, depth)
	if err != nil {
		return err
	}
	path, ok, err := svgElementPath(node, style, transform)
	if err != nil {
		return err
	}
	if ok {
		if !svgPathFinite(path) {
			return errors.New("invalid SVG path: non-finite value")
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
			return errors.New("invalid SVG text: non-finite value")
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
			return errors.New("invalid SVG image: non-finite value")
		}
		sig.Images = append(sig.Images, image)
		sig.Elements = append(sig.Elements, SVGElement{Kind: "image", Image: image})
	}
	for _, child := range node.Children {
		if err := svgCollectDepth(child, style, transform, sig, refs, gradients, patterns, clipCache, rules, childAncestors, depth+1, renderingRef); err != nil {
			return err
		}
	}
	return nil
}

// SVGParse parses a Scalable Vector Graphics (SVG) buffer into a descriptor.
// Paths, lines, rectangles, circles, ellipses, polylines, polygons, text, and
// inherited presentation attributes are converted to data that SVGWrite can
// render.
func SVGParse(buf []byte) (sig SVG, err error) {
	if len(buf) > maxSVGSourceBytes {
		return SVG{}, errors.New("SVG source exceeds maximum size")
	}
	cacheKey := svgParseCacheKey{size: len(buf), sum: sha256.Sum256(buf)}
	if cached, ok := svgParseCacheGet(cacheKey); ok {
		return cached, nil
	}
	var src svgNode
	err = xml.Unmarshal(buf, &src)
	if err == nil {
		svgPrepareNodes(&src)
	}
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
	} else {
		svgParseCachePut(cacheKey, sig)
	}
	return
}

// SVGFileParse parses an SVG file into a descriptor that SVGWrite can render.
func SVGFileParse(svgFileStr string) (sig SVG, err error) {
	var buf []byte
	buf, err = readSVGFile(svgFileStr)
	if err == nil {
		sig, err = SVGParse(buf)
	}
	return
}

func readSVGFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	if info, err := file.Stat(); err == nil && info.Mode().IsRegular() && info.Size() > maxSVGSourceBytes {
		return nil, errors.New("SVG source exceeds maximum size")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxSVGSourceBytes+1))
	if err == nil && len(data) > maxSVGSourceBytes {
		err = errors.New("SVG source exceeds maximum size")
	}
	return data, err
}
