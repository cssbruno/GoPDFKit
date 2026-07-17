// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"unicode"
	"unicode/utf8"
)

var (
	ErrShapedLineInvalid     = errors.New("layoutengine: shaped line layout is invalid")
	ErrShapedLineUnbreakable = errors.New("layoutengine: one shaped cluster exceeds the line width")
	ErrShapedLineLimit       = errors.New("layoutengine: shaped line breaking limit exceeded")
)

type ShapedLineBreakKind string

const (
	ShapedLineWrap    ShapedLineBreakKind = "wrap"
	ShapedLineNewline ShapedLineBreakKind = "newline"
	ShapedLineEnd     ShapedLineBreakKind = "end"
)

func (kind ShapedLineBreakKind) valid() bool {
	return kind == ShapedLineWrap || kind == ShapedLineNewline || kind == ShapedLineEnd
}

// ShapedLine retains a logical UTF-8 text range and the matching glyphs in
// their original visual order. A space chosen as a wrap opportunity remains
// at the end of the line and contributes to Width; explicit newline bytes are
// excluded from both adjacent line ranges.
type ShapedLine struct {
	Index     uint32              `json:"index"`
	TextStart uint32              `json:"text_start"`
	TextEnd   uint32              `json:"text_end"`
	Width     Fixed               `json:"width"`
	Break     ShapedLineBreakKind `json:"break"`
	Glyphs    []ShapedGlyph       `json:"glyphs,omitempty"`
}

type ShapedLineLayout struct {
	Text      string          `json:"text"`
	Language  string          `json:"language"`
	Direction TextDirection   `json:"direction"`
	MaxWidth  Fixed           `json:"max_width"`
	FontRuns  []ShapedFontRun `json:"font_runs"`
	Lines     []ShapedLine    `json:"lines"`
	Source    SourceSpan      `json:"source"`
}

func (layout ShapedLineLayout) CanonicalJSON() ([]byte, error) {
	if err := layout.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(layout)
}

func (layout ShapedLineLayout) Validate() error {
	if layout.Text == "" || !utf8.ValidString(layout.Text) || !layout.Direction.valid() || !validShapeLanguage(layout.Language) ||
		layout.MaxWidth <= 0 || len(layout.FontRuns) == 0 || len(layout.Lines) == 0 || layout.Source.Validate() != nil {
		return ErrShapedLineInvalid
	}
	for _, run := range layout.FontRuns {
		if err := validateShapeFont(run.Font); err != nil {
			return ErrShapedLineInvalid
		}
	}
	for index, line := range layout.Lines {
		if line.Index != uint32(index) || !line.Break.valid() || line.TextStart > line.TextEnd ||
			uint64(line.TextEnd) > uint64(len(layout.Text)) || !utf8Boundary(layout.Text, line.TextStart) || !utf8Boundary(layout.Text, line.TextEnd) ||
			line.Width < 0 || line.Width > layout.MaxWidth {
			return fmt.Errorf("%w: line %d metadata", ErrShapedLineInvalid, index)
		}
		var width Fixed
		for glyphIndex, glyph := range line.Glyphs {
			if glyph.ID == 0 || glyph.Advance < 0 || uint64(glyph.FontRun) >= uint64(len(layout.FontRuns)) ||
				glyph.Cluster.Start >= glyph.Cluster.End || glyph.Cluster.Start < line.TextStart || glyph.Cluster.End > line.TextEnd ||
				!utf8Boundary(layout.Text, glyph.Cluster.Start) || !utf8Boundary(layout.Text, glyph.Cluster.End) {
				return fmt.Errorf("%w: line %d glyph %d", ErrShapedLineInvalid, index, glyphIndex)
			}
			var err error
			width, err = width.Add(glyph.Advance)
			if err != nil {
				return err
			}
		}
		if width != line.Width {
			return fmt.Errorf("%w: line %d width", ErrShapedLineInvalid, index)
		}
		if !shapedLineCoversRange(line) {
			return fmt.Errorf("%w: line %d cluster coverage", ErrShapedLineInvalid, index)
		}
		if index+1 == len(layout.Lines) {
			if line.Break != ShapedLineEnd || line.TextEnd != uint32(len(layout.Text)) {
				return fmt.Errorf("%w: final line", ErrShapedLineInvalid)
			}
			continue
		}
		next := layout.Lines[index+1]
		switch line.Break {
		case ShapedLineWrap:
			if next.TextStart != line.TextEnd {
				return fmt.Errorf("%w: wrapped line range", ErrShapedLineInvalid)
			}
		case ShapedLineNewline:
			if !isExactNewline(layout.Text[line.TextEnd:next.TextStart]) {
				return fmt.Errorf("%w: explicit newline range", ErrShapedLineInvalid)
			}
		default:
			return fmt.Errorf("%w: premature end", ErrShapedLineInvalid)
		}
	}
	return nil
}

func shapedLineCoversRange(line ShapedLine) bool {
	if line.TextStart == line.TextEnd {
		return len(line.Glyphs) == 0
	}
	if len(line.Glyphs) == 0 {
		return false
	}
	clusters := make([]UTF8Cluster, len(line.Glyphs))
	for index := range line.Glyphs {
		clusters[index] = line.Glyphs[index].Cluster
	}
	sort.Slice(clusters, func(i, j int) bool {
		if clusters[i].Start != clusters[j].Start {
			return clusters[i].Start < clusters[j].Start
		}
		return clusters[i].End < clusters[j].End
	})
	position := line.TextStart
	var previous UTF8Cluster
	for index, cluster := range clusters {
		if index != 0 && cluster == previous {
			continue
		}
		if cluster.Start != position {
			return false
		}
		position, previous = cluster.End, cluster
	}
	return position == line.TextEnd
}

type ShapedLineLimits struct {
	MaxTextBytes uint64
	MaxGlyphs    uint64
	MaxLines     uint64
	MaxWork      uint64
}

const (
	hardMaxShapedLineTextBytes uint64 = 16 << 20
	hardMaxShapedLineGlyphs    uint64 = 1 << 22
	hardMaxShapedLines         uint64 = 1 << 20
	hardMaxShapedLineWork      uint64 = 64 << 20
)

func DefaultShapedLineLimits() ShapedLineLimits {
	return ShapedLineLimits{MaxTextBytes: 1 << 20, MaxGlyphs: 1 << 20, MaxLines: 1 << 16, MaxWork: 8 << 20}
}

type shapedLineBudget struct {
	ctx   context.Context
	limit uint64
	used  uint64
}

func (budget *shapedLineBudget) charge(amount uint64) error {
	if err := ChargePlanningWork(budget.ctx, "shaped line planning", amount); err != nil {
		return err
	}
	if err := budget.ctx.Err(); err != nil {
		return newPlanningError(err, Diagnostic{Code: DiagnosticCanceled, Severity: SeverityError, Stage: StageLayout, Message: "shaped line breaking was canceled"})
	}
	if amount > budget.limit-budget.used {
		return newPlanningError(ErrShapedLineLimit, Diagnostic{Code: DiagnosticWorkLimit, Severity: SeverityError, Stage: StageLayout, Message: "shaped line breaking exceeded its deterministic work limit"})
	}
	budget.used += amount
	return nil
}

type shapedClusterUnit struct {
	cluster UTF8Cluster
	width   Fixed
	space   bool
}

type shapedParagraph struct {
	textStart uint32
	textEnd   uint32
	unitStart int
	unitEnd   int
	newline   bool
}

// BreakShapedText greedily breaks an immutable shaping result without
// measuring or reshaping. Spaces are preferred, then any complete cluster.
func BreakShapedText(ctx context.Context, shaped ShapedText, maxWidth Fixed, limits ShapedLineLimits) (ShapedLineLayout, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	limits, err := normalizeShapedLineLimits(limits)
	if err != nil {
		return ShapedLineLayout{}, err
	}
	if maxWidth <= 0 || shaped.Validate() != nil {
		return ShapedLineLayout{}, ErrShapedLineInvalid
	}
	if uint64(len(shaped.Text)) > limits.MaxTextBytes || uint64(len(shaped.Glyphs)) > limits.MaxGlyphs {
		return ShapedLineLayout{}, newPlanningError(ErrShapedLineLimit, Diagnostic{Code: DiagnosticResourceLimit, Severity: SeverityError, Stage: StageLayout, Message: "shaped line input exceeds resource limits"})
	}
	budget := &shapedLineBudget{ctx: ctx, limit: limits.MaxWork}
	if err := budget.charge(uint64(len(shaped.Text)) + uint64(len(shaped.Glyphs))); err != nil {
		return ShapedLineLayout{}, err
	}
	units, err := collectShapedClusterUnits(shaped, budget)
	if err != nil {
		return ShapedLineLayout{}, err
	}
	paragraphs, err := splitShapedParagraphs(shaped.Text, units, budget)
	if err != nil {
		return ShapedLineLayout{}, err
	}
	layout := ShapedLineLayout{
		Text: shaped.Text, Language: shaped.Language, Direction: shaped.Direction,
		MaxWidth: maxWidth, FontRuns: cloneSlice(shaped.FontRuns), Source: shaped.Source,
	}
	clusterLines := make(map[UTF8Cluster]int, len(units))
	for _, paragraph := range paragraphs {
		if err := breakShapedParagraph(&layout, units, paragraph, maxWidth, limits, budget, clusterLines); err != nil {
			return ShapedLineLayout{}, err
		}
	}
	for _, glyph := range shaped.Glyphs {
		lineIndex, exists := clusterLines[glyph.Cluster]
		if !exists {
			return ShapedLineLayout{}, ErrShapedLineInvalid
		}
		layout.Lines[lineIndex].Glyphs = append(layout.Lines[lineIndex].Glyphs, glyph)
	}
	if err := layout.Validate(); err != nil {
		return ShapedLineLayout{}, err
	}
	return cloneShapedLineLayout(layout), nil
}

func normalizeShapedLineLimits(limits ShapedLineLimits) (ShapedLineLimits, error) {
	if limits == (ShapedLineLimits{}) {
		return DefaultShapedLineLimits(), nil
	}
	if limits.MaxTextBytes == 0 || limits.MaxGlyphs == 0 || limits.MaxLines == 0 || limits.MaxWork == 0 ||
		limits.MaxTextBytes > hardMaxShapedLineTextBytes || limits.MaxGlyphs > hardMaxShapedLineGlyphs ||
		limits.MaxLines > hardMaxShapedLines || limits.MaxWork > hardMaxShapedLineWork {
		return ShapedLineLimits{}, errors.New("layoutengine: shaped line limits must be positive and within hard caps")
	}
	return limits, nil
}

func collectShapedClusterUnits(shaped ShapedText, budget *shapedLineBudget) ([]shapedClusterUnit, error) {
	glyphs := cloneSlice(shaped.Glyphs)
	sort.SliceStable(glyphs, func(i, j int) bool {
		if glyphs[i].Cluster.Start != glyphs[j].Cluster.Start {
			return glyphs[i].Cluster.Start < glyphs[j].Cluster.Start
		}
		return glyphs[i].Cluster.End < glyphs[j].Cluster.End
	})
	units := make([]shapedClusterUnit, 0, len(glyphs))
	for _, glyph := range glyphs {
		if err := budget.charge(1); err != nil {
			return nil, err
		}
		if len(units) != 0 && glyph.Cluster.Start < units[len(units)-1].cluster.End && glyph.Cluster != units[len(units)-1].cluster {
			return nil, fmt.Errorf("%w: partially overlapping clusters", ErrShapedLineInvalid)
		}
		if len(units) != 0 && glyph.Cluster == units[len(units)-1].cluster {
			width, err := units[len(units)-1].width.Add(glyph.Advance)
			if err != nil {
				return nil, err
			}
			units[len(units)-1].width = width
			continue
		}
		clusterText := shaped.Text[glyph.Cluster.Start:glyph.Cluster.End]
		if containsNewline(clusterText) {
			return nil, fmt.Errorf("%w: newline belongs to a shaped cluster", ErrShapedLineInvalid)
		}
		units = append(units, shapedClusterUnit{cluster: glyph.Cluster, width: glyph.Advance, space: isBreakSpace(clusterText)})
	}
	return units, nil
}

func splitShapedParagraphs(text string, units []shapedClusterUnit, budget *shapedLineBudget) ([]shapedParagraph, error) {
	paragraphs := make([]shapedParagraph, 0, 1)
	position, paragraphStart, unitIndex, paragraphUnitStart := uint32(0), uint32(0), 0, 0
	for position < uint32(len(text)) {
		if err := budget.charge(1); err != nil {
			return nil, err
		}
		newline := newlineLength(text[position:])
		if newline != 0 {
			if unitIndex < len(units) && units[unitIndex].cluster.Start < position+newline {
				return nil, ErrShapedLineInvalid
			}
			paragraphs = append(paragraphs, shapedParagraph{paragraphStart, position, paragraphUnitStart, unitIndex, true})
			position += newline
			paragraphStart, paragraphUnitStart = position, unitIndex
			continue
		}
		if unitIndex >= len(units) || units[unitIndex].cluster.Start != position {
			return nil, fmt.Errorf("%w: non-newline text byte is not covered by a cluster", ErrShapedLineInvalid)
		}
		position = units[unitIndex].cluster.End
		unitIndex++
	}
	if unitIndex != len(units) {
		return nil, ErrShapedLineInvalid
	}
	paragraphs = append(paragraphs, shapedParagraph{paragraphStart, uint32(len(text)), paragraphUnitStart, unitIndex, false})
	return paragraphs, nil
}

func breakShapedParagraph(layout *ShapedLineLayout, units []shapedClusterUnit, paragraph shapedParagraph, maxWidth Fixed, limits ShapedLineLimits, budget *shapedLineBudget, clusterLines map[UTF8Cluster]int) error {
	if paragraph.unitStart == paragraph.unitEnd {
		kind := ShapedLineEnd
		if paragraph.newline {
			kind = ShapedLineNewline
		}
		return appendShapedLine(layout, ShapedLine{TextStart: paragraph.textStart, TextEnd: paragraph.textEnd, Break: kind}, limits)
	}
	prefix := make([]Fixed, paragraph.unitEnd-paragraph.unitStart+1)
	for index := paragraph.unitStart; index < paragraph.unitEnd; index++ {
		var err error
		prefix[index-paragraph.unitStart+1], err = prefix[index-paragraph.unitStart].Add(units[index].width)
		if err != nil {
			return err
		}
	}
	start := paragraph.unitStart
	for start < paragraph.unitEnd {
		if err := budget.charge(1); err != nil {
			return err
		}
		low, high := start, paragraph.unitEnd+1
		for low+1 < high {
			if err := budget.charge(1); err != nil {
				return err
			}
			middle := low + (high-low)/2
			width, err := prefix[middle-paragraph.unitStart].Sub(prefix[start-paragraph.unitStart])
			if err != nil {
				return err
			}
			if width <= maxWidth {
				low = middle
			} else {
				high = middle
			}
		}
		end := low
		if end == start {
			return ErrShapedLineUnbreakable
		}
		if end < paragraph.unitEnd {
			for candidate := end - 1; candidate >= start; candidate-- {
				if err := budget.charge(1); err != nil {
					return err
				}
				if units[candidate].space {
					end = candidate + 1
					break
				}
			}
		}
		width, _ := prefix[end-paragraph.unitStart].Sub(prefix[start-paragraph.unitStart])
		kind := ShapedLineWrap
		if end == paragraph.unitEnd {
			kind = ShapedLineEnd
			if paragraph.newline {
				kind = ShapedLineNewline
			}
		}
		line := ShapedLine{TextStart: units[start].cluster.Start, TextEnd: units[end-1].cluster.End, Width: width, Break: kind}
		lineIndex := len(layout.Lines)
		if err := appendShapedLine(layout, line, limits); err != nil {
			return err
		}
		for index := start; index < end; index++ {
			clusterLines[units[index].cluster] = lineIndex
		}
		start = end
	}
	return nil
}

func appendShapedLine(layout *ShapedLineLayout, line ShapedLine, limits ShapedLineLimits) error {
	if uint64(len(layout.Lines)) >= limits.MaxLines {
		return newPlanningError(ErrShapedLineLimit, Diagnostic{Code: DiagnosticResourceLimit, Severity: SeverityError, Stage: StageLayout, Message: "shaped line count exceeds its limit"})
	}
	line.Index = uint32(len(layout.Lines))
	layout.Lines = append(layout.Lines, line)
	return nil
}

func newlineLength(text string) uint32 {
	if len(text) >= 2 && text[0] == '\r' && text[1] == '\n' {
		return 2
	}
	if len(text) != 0 && (text[0] == '\n' || text[0] == '\r') {
		return 1
	}
	return 0
}

func isExactNewline(text string) bool { return int(newlineLength(text)) == len(text) && len(text) != 0 }

func containsNewline(text string) bool {
	for _, value := range []byte(text) {
		if value == '\n' || value == '\r' {
			return true
		}
	}
	return false
}

func isBreakSpace(text string) bool {
	if text == "" {
		return false
	}
	for _, value := range text {
		if !unicode.IsSpace(value) || value == '\n' || value == '\r' {
			return false
		}
	}
	return true
}

func cloneShapedLineLayout(layout ShapedLineLayout) ShapedLineLayout {
	layout.FontRuns = cloneSlice(layout.FontRuns)
	layout.Lines = cloneSlice(layout.Lines)
	for index := range layout.Lines {
		layout.Lines[index].Glyphs = cloneSlice(layout.Lines[index].Glyphs)
	}
	return layout
}

type ShapedLinePaintSink interface {
	BeginShapedLine(ShapedLine) error
	PaintShapedFontRun(ShapedFontRun, []ShapedGlyph) error
	EndShapedLine(ShapedLine) error
}

// ReplayShapedLineLayout replays stored visual-order glyphs exactly. It never
// calls a shaper or computes advances, offsets, clusters, or line breaks.
func ReplayShapedLineLayout(layout ShapedLineLayout, sink ShapedLinePaintSink) error {
	if sink == nil {
		return errors.New("layoutengine: shaped line paint sink is nil")
	}
	if err := layout.Validate(); err != nil {
		return err
	}
	for _, line := range layout.Lines {
		lineCopy := line
		lineCopy.Glyphs = cloneSlice(line.Glyphs)
		if err := sink.BeginShapedLine(lineCopy); err != nil {
			return err
		}
		for start := 0; start < len(line.Glyphs); {
			fontRun := line.Glyphs[start].FontRun
			end := start + 1
			for end < len(line.Glyphs) && line.Glyphs[end].FontRun == fontRun {
				end++
			}
			if err := sink.PaintShapedFontRun(layout.FontRuns[fontRun], cloneSlice(line.Glyphs[start:end])); err != nil {
				return err
			}
			start = end
		}
		if err := sink.EndShapedLine(lineCopy); err != nil {
			return err
		}
	}
	return nil
}
