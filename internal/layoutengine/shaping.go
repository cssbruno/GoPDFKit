// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

var (
	ErrShapingUnsupported = errors.New("layoutengine: Unicode shaping requires an external shaper")
	ErrShapingInvalid     = errors.New("layoutengine: shaped text is invalid")
	ErrShapingWorkLimit   = errors.New("layoutengine: shaping work limit exceeded")
	ErrShapingCacheLimit  = errors.New("layoutengine: shaping cache limits are invalid")
)

type TextDirection string

const (
	DirectionLTR TextDirection = "ltr"
	DirectionRTL TextDirection = "rtl"
)

func (direction TextDirection) valid() bool {
	return direction == DirectionLTR || direction == DirectionRTL
}

// ShapeFont is an immutable font-program identity. CoreFace is set only for a
// standard PDF core font. Digest identifies the exact metrics/font bytes.
type ShapeFont struct {
	Name     string          `json:"name"`
	Digest   ShapeFontDigest `json:"digest"`
	CoreFace CoreFontFace    `json:"core_face,omitempty"`
}

// ShapeFontDigest identifies the exact external font program plus shaping
// metrics. It remains distinct from the core-font-only metrics digest.
type ShapeFontDigest string

func (digest ShapeFontDigest) validate() error { return CoreFontMetricsDigest(digest).validate() }

type ShapeFeature struct {
	Tag   string `json:"tag"`
	Value uint32 `json:"value"`
}

type ShapingInput struct {
	Text      string         `json:"text"`
	Font      ShapeFont      `json:"font"`
	Fallbacks []ShapeFont    `json:"fallbacks,omitempty"`
	FontSize  Fixed          `json:"font_size"`
	Language  string         `json:"language"`
	Direction TextDirection  `json:"direction"`
	Features  []ShapeFeature `json:"features,omitempty"`
	Source    SourceSpan     `json:"source"`
}

type UTF8Cluster struct {
	Start uint32 `json:"start"`
	End   uint32 `json:"end"`
}

type ShapedGlyph struct {
	ID      uint32      `json:"id"`
	Advance Fixed       `json:"advance"`
	Offset  Point       `json:"offset"`
	Cluster UTF8Cluster `json:"cluster"`
	FontRun uint32      `json:"font_run"`
}

type ShapedFontRun struct {
	Font       ShapeFont `json:"font"`
	GlyphStart uint32    `json:"glyph_start"`
	GlyphCount uint32    `json:"glyph_count"`
	TextStart  uint32    `json:"text_start"`
	TextEnd    uint32    `json:"text_end"`
	Fallback   bool      `json:"fallback,omitempty"`
}

type ShapedText struct {
	Text        string          `json:"text"`
	Language    string          `json:"language"`
	Direction   TextDirection   `json:"direction"`
	Glyphs      []ShapedGlyph   `json:"glyphs"`
	FontRuns    []ShapedFontRun `json:"font_runs"`
	Diagnostics []Diagnostic    `json:"diagnostics,omitempty"`
	Source      SourceSpan      `json:"source"`
}

func (result ShapedText) CanonicalJSON() ([]byte, error) {
	if err := result.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

func (result ShapedText) Validate() error {
	if !utf8.ValidString(result.Text) || result.Text == "" || !result.Direction.valid() || !validShapeLanguage(result.Language) {
		return ErrShapingInvalid
	}
	if err := result.Source.Validate(); err != nil {
		return fmt.Errorf("%w: source: %v", ErrShapingInvalid, err)
	}
	if len(result.Glyphs) == 0 || len(result.FontRuns) == 0 {
		return fmt.Errorf("%w: empty glyph or font run output", ErrShapingInvalid)
	}
	textBytes := uint32(len(result.Text))
	var expectedGlyphStart uint32
	textRanges := make([]UTF8Cluster, len(result.FontRuns))
	for index, run := range result.FontRuns {
		if err := validateShapeFont(run.Font); err != nil || run.GlyphCount == 0 || run.TextStart >= run.TextEnd || run.TextEnd > textBytes ||
			run.GlyphStart != expectedGlyphStart || uint64(run.GlyphStart)+uint64(run.GlyphCount) > uint64(len(result.Glyphs)) {
			return fmt.Errorf("%w: font run %d", ErrShapingInvalid, index)
		}
		expectedGlyphStart += run.GlyphCount
		textRanges[index] = UTF8Cluster{Start: run.TextStart, End: run.TextEnd}
	}
	if expectedGlyphStart != uint32(len(result.Glyphs)) {
		return fmt.Errorf("%w: font runs do not cover every glyph", ErrShapingInvalid)
	}
	sort.Slice(textRanges, func(i, j int) bool { return textRanges[i].Start < textRanges[j].Start })
	var expectedTextStart uint32
	for _, textRange := range textRanges {
		if textRange.Start != expectedTextStart || !utf8Boundary(result.Text, textRange.Start) || !utf8Boundary(result.Text, textRange.End) {
			return fmt.Errorf("%w: font runs do not partition UTF-8 text", ErrShapingInvalid)
		}
		expectedTextStart = textRange.End
	}
	if expectedTextStart != textBytes {
		return fmt.Errorf("%w: font runs do not cover every text byte", ErrShapingInvalid)
	}
	for index, glyph := range result.Glyphs {
		if glyph.ID == 0 || glyph.Advance < 0 || uint64(glyph.FontRun) >= uint64(len(result.FontRuns)) ||
			glyph.Cluster.Start >= glyph.Cluster.End || glyph.Cluster.End > textBytes ||
			!utf8Boundary(result.Text, glyph.Cluster.Start) || !utf8Boundary(result.Text, glyph.Cluster.End) {
			return fmt.Errorf("%w: glyph %d", ErrShapingInvalid, index)
		}
		run := result.FontRuns[glyph.FontRun]
		if uint32(index) < run.GlyphStart || uint32(index) >= run.GlyphStart+run.GlyphCount ||
			glyph.Cluster.Start < run.TextStart || glyph.Cluster.End > run.TextEnd {
			return fmt.Errorf("%w: glyph %d is outside its font run", ErrShapingInvalid, index)
		}
	}
	for _, diagnostic := range result.Diagnostics {
		if err := diagnostic.Validate(); err != nil {
			return fmt.Errorf("%w: diagnostic: %v", ErrShapingInvalid, err)
		}
	}
	return nil
}

func utf8Boundary(text string, offset uint32) bool {
	if uint64(offset) > uint64(len(text)) {
		return false
	}
	return offset == 0 || offset == uint32(len(text)) || utf8.RuneStart(text[offset])
}

type ShapingLimits struct {
	MaxTextBytes uint64
	MaxGlyphs    uint64
	MaxFontRuns  uint64
	MaxFeatures  uint64
	MaxWork      uint64
}

func DefaultShapingLimits() ShapingLimits {
	return ShapingLimits{MaxTextBytes: 1 << 20, MaxGlyphs: 1 << 20, MaxFontRuns: 1 << 16, MaxFeatures: 256, MaxWork: 4 << 20}
}

const (
	hardMaxShapingTextBytes uint64 = 16 << 20
	hardMaxShapingGlyphs    uint64 = 1 << 22
	hardMaxShapingFontRuns  uint64 = 1 << 20
	hardMaxShapingFeatures  uint64 = 1 << 12
	hardMaxShapingWork      uint64 = 64 << 20
)

type ShapingBudget struct {
	ctx   context.Context
	limit uint64
	used  uint64
}

func (budget *ShapingBudget) Charge(amount uint64) error {
	if err := ChargePlanningWork(budget.ctx, "text shaping", amount); err != nil {
		return err
	}
	if err := budget.ctx.Err(); err != nil {
		return newPlanningError(err, Diagnostic{Code: DiagnosticCanceled, Severity: SeverityError, Stage: StageLayout, Message: "text shaping was canceled"})
	}
	if amount > budget.limit-budget.used {
		return newPlanningError(ErrShapingWorkLimit, Diagnostic{
			Code: DiagnosticWorkLimit, Severity: SeverityError, Stage: StageLayout,
			Message:  "text shaping exceeded its deterministic work limit",
			Evidence: []DiagnosticEvidence{{Key: "work_limit", Value: strconv.FormatUint(budget.limit, 10)}},
		})
	}
	budget.used += amount
	return nil
}

// UnicodeShaper is the injectable boundary for real script shaping. StableID
// must change when the shaping implementation, Unicode data, or policy changes.
type UnicodeShaper interface {
	StableID() string
	Shape(context.Context, ShapingInput, *ShapingBudget) (ShapedText, error)
}

type CoreASCIIAdvanceProvider interface {
	CoreASCIIAdvances(CoreFontFace, string, Fixed) ([]Fixed, error)
}

// CoreASCIIShaper is the characterized fast path for printable ASCII core-font
// codes. It never claims to shape combining marks, bidi text, or Unicode.
type CoreASCIIShaper struct{ Metrics CoreASCIIAdvanceProvider }

func (CoreASCIIShaper) StableID() string { return "core-ascii-v1" }

func (shaper CoreASCIIShaper) Shape(ctx context.Context, input ShapingInput, budget *ShapingBudget) (ShapedText, error) {
	if shaper.Metrics == nil || input.Font.CoreFace == "" || !input.Font.CoreFace.valid() || input.Direction != DirectionLTR {
		return ShapedText{}, unsupportedShapingError(input, 0)
	}
	glyphs := make([]ShapedGlyph, len(input.Text))
	for index := range []byte(input.Text) {
		if err := budget.Charge(1); err != nil {
			return ShapedText{}, err
		}
		code := input.Text[index]
		if code < 0x20 || code > 0x7e {
			return ShapedText{}, unsupportedShapingError(input, uint32(index))
		}
		glyphs[index] = ShapedGlyph{ID: uint32(code), Cluster: UTF8Cluster{Start: uint32(index), End: uint32(index + 1)}}
	}
	advances, err := shaper.Metrics.CoreASCIIAdvances(input.Font.CoreFace, input.Text, input.FontSize)
	if err != nil || len(advances) != len(glyphs) {
		return ShapedText{}, missingGlyphError(input, 0, 0)
	}
	for index, advance := range advances {
		if advance < 0 {
			return ShapedText{}, missingGlyphError(input, uint32(index), input.Text[index])
		}
		glyphs[index].Advance = advance
	}
	return ShapedText{
		Text: input.Text, Language: input.Language, Direction: input.Direction, Glyphs: glyphs,
		FontRuns: []ShapedFontRun{{Font: input.Font, GlyphCount: uint32(len(glyphs)), TextEnd: uint32(len(input.Text))}},
		Source:   input.Source,
	}, nil
}

func ShapeText(ctx context.Context, shaper UnicodeShaper, input ShapingInput, limits ShapingLimits, cache *ShapeCache) (ShapedText, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if limits == (ShapingLimits{}) {
		limits = DefaultShapingLimits()
	}
	if err := validateShapingInput(input, limits); err != nil {
		return ShapedText{}, err
	}
	if shaper == nil || strings.TrimSpace(shaper.StableID()) == "" {
		return ShapedText{}, unsupportedShapingError(input, firstNonASCII(input.Text))
	}
	key, err := shapingCacheKey(shaper.StableID(), input)
	if err != nil {
		return ShapedText{}, err
	}
	if cache != nil {
		if value, ok := cache.Get(key); ok {
			return value, nil
		}
	}
	budget := &ShapingBudget{ctx: ctx, limit: limits.MaxWork}
	if err := budget.Charge(uint64(len(input.Text)) + uint64(len(input.Features))); err != nil {
		return ShapedText{}, err
	}
	result, err := shaper.Shape(ctx, input, budget)
	if err != nil {
		return ShapedText{}, err
	}
	if err := result.Validate(); err != nil {
		return ShapedText{}, err
	}
	if result.Text != input.Text || result.Language != input.Language || result.Direction != input.Direction || result.Source != input.Source {
		return ShapedText{}, fmt.Errorf("%w: output identity does not match input", ErrShapingInvalid)
	}
	if uint64(len(result.Glyphs)) > limits.MaxGlyphs || uint64(len(result.FontRuns)) > limits.MaxFontRuns {
		return ShapedText{}, newPlanningError(ErrShapingWorkLimit, Diagnostic{Code: DiagnosticResourceLimit, Severity: SeverityError, Stage: StageLayout, Message: "shaped output exceeds resource limits"})
	}
	if err := validateShapedFontProvenance(input, result); err != nil {
		return ShapedText{}, err
	}
	if cache != nil {
		cache.Put(key, result)
	}
	return cloneShapedText(result), nil
}

func validateShapingInput(input ShapingInput, limits ShapingLimits) error {
	if limits.MaxTextBytes == 0 || limits.MaxGlyphs == 0 || limits.MaxFontRuns == 0 || limits.MaxFeatures == 0 || limits.MaxWork == 0 {
		return errors.New("layoutengine: shaping limits must be positive")
	}
	if limits.MaxTextBytes > hardMaxShapingTextBytes || limits.MaxGlyphs > hardMaxShapingGlyphs ||
		limits.MaxFontRuns > hardMaxShapingFontRuns || limits.MaxFeatures > hardMaxShapingFeatures || limits.MaxWork > hardMaxShapingWork {
		return errors.New("layoutengine: shaping limits exceed implementation hard caps")
	}
	if input.Text == "" || !utf8.ValidString(input.Text) || uint64(len(input.Text)) > limits.MaxTextBytes || input.FontSize <= 0 ||
		!input.Direction.valid() || !validShapeLanguage(input.Language) || uint64(len(input.Features)) > limits.MaxFeatures {
		return ErrShapingInvalid
	}
	if err := validateShapeFont(input.Font); err != nil {
		return err
	}
	for _, font := range input.Fallbacks {
		if err := validateShapeFont(font); err != nil {
			return err
		}
	}
	previous := ""
	for _, feature := range input.Features {
		if len(feature.Tag) != 4 || feature.Tag <= previous {
			return ErrShapingInvalid
		}
		for _, value := range []byte(feature.Tag) {
			if value < 'a' || value > 'z' {
				return ErrShapingInvalid
			}
		}
		previous = feature.Tag
	}
	return input.Source.Validate()
}

func validateShapedFontProvenance(input ShapingInput, result ShapedText) error {
	hasFallbackDiagnostic := false
	for _, diagnostic := range result.Diagnostics {
		hasFallbackDiagnostic = hasFallbackDiagnostic || diagnostic.Code == DiagnosticFontMissing
	}
	usedFallback := false
	for _, run := range result.FontRuns {
		if run.Font == input.Font {
			if run.Fallback {
				return fmt.Errorf("%w: primary font run marked as fallback", ErrShapingInvalid)
			}
			continue
		}
		found := false
		for _, fallback := range input.Fallbacks {
			found = found || run.Font == fallback
		}
		if !found || !run.Fallback {
			return fmt.Errorf("%w: output font run has no input provenance", ErrShapingInvalid)
		}
		usedFallback = true
	}
	if usedFallback && !hasFallbackDiagnostic {
		return fmt.Errorf("%w: fallback use has no diagnostic", ErrShapingInvalid)
	}
	return nil
}

func validateShapeFont(font ShapeFont) error {
	if strings.TrimSpace(font.Name) == "" || strings.TrimSpace(font.Name) != font.Name || font.Digest.validate() != nil {
		return ErrShapingInvalid
	}
	if font.CoreFace != "" && !font.CoreFace.valid() {
		return ErrShapingInvalid
	}
	return nil
}

func validShapeLanguage(language string) bool {
	if language == "und" {
		return true
	}
	if language == "" || strings.ToLower(language) != language {
		return false
	}
	for _, value := range []byte(language) {
		if (value < 'a' || value > 'z') && value != '-' && (value < '0' || value > '9') {
			return false
		}
	}
	return true
}

func unsupportedShapingError(input ShapingInput, offset uint32) error {
	return newPlanningError(ErrShapingUnsupported, Diagnostic{
		Code: DiagnosticGlyphMissing, Severity: SeverityError, Stage: StageLayout,
		Message:  "text requires a Unicode shaper that is not configured",
		Location: DiagnosticLocation{Source: input.Source},
		Evidence: []DiagnosticEvidence{{Key: "byte_offset", Value: strconv.FormatUint(uint64(offset), 10)}, {Key: "direction", Value: string(input.Direction)}},
	})
}

func missingGlyphError(input ShapingInput, offset uint32, code byte) error {
	return newPlanningError(ErrShapingUnsupported, Diagnostic{
		Code: DiagnosticGlyphMissing, Severity: SeverityError, Stage: StageLayout,
		Message: "core font metrics do not contain the requested glyph", Location: DiagnosticLocation{Source: input.Source},
		Evidence: []DiagnosticEvidence{{Key: "byte_offset", Value: strconv.FormatUint(uint64(offset), 10)}, {Key: "code", Value: strconv.FormatUint(uint64(code), 10)}},
	})
}

func firstNonASCII(text string) uint32 {
	for index := range []byte(text) {
		if text[index] < 0x20 || text[index] > 0x7e {
			return uint32(index)
		}
	}
	return 0
}

func shapingCacheKey(shaperID string, input ShapingInput) (string, error) {
	encoded, err := json.Marshal(struct {
		Shaper string       `json:"shaper"`
		Input  ShapingInput `json:"input"`
	}{shaperID, input})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func cloneShapedText(value ShapedText) ShapedText {
	value.Glyphs = cloneSlice(value.Glyphs)
	value.FontRuns = cloneSlice(value.FontRuns)
	if len(value.Diagnostics) != 0 {
		diagnostics := make([]Diagnostic, len(value.Diagnostics))
		for index := range value.Diagnostics {
			diagnostics[index] = cloneDiagnostic(value.Diagnostics[index])
		}
		value.Diagnostics = diagnostics
	}
	return value
}

type ShapeCacheLimits struct {
	MaxEntries uint64
	MaxBytes   uint64
}

type ShapeCacheStats struct {
	Entries uint64
	Bytes   uint64
	Hits    uint64
	Misses  uint64
}

type shapeCacheEntry struct {
	key   string
	value ShapedText
	bytes uint64
}

// ShapeCache is a concurrency-safe byte-accounted FIFO cache. Oversized
// values are returned to the caller but never retained.
type ShapeCache struct {
	mu     sync.Mutex
	limits ShapeCacheLimits
	items  map[string]shapeCacheEntry
	order  []string
	bytes  uint64
	hits   uint64
	misses uint64
}

func NewShapeCache(limits ShapeCacheLimits) (*ShapeCache, error) {
	if limits.MaxEntries == 0 || limits.MaxBytes == 0 || limits.MaxEntries > 1<<20 || limits.MaxBytes > 1<<30 {
		return nil, ErrShapingCacheLimit
	}
	return &ShapeCache{limits: limits, items: make(map[string]shapeCacheEntry)}, nil
}

func (cache *ShapeCache) Get(key string) (ShapedText, bool) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	entry, ok := cache.items[key]
	if !ok {
		cache.misses++
		return ShapedText{}, false
	}
	cache.hits++
	return cloneShapedText(entry.value), true
}

func (cache *ShapeCache) Put(key string, value ShapedText) {
	encoded, err := value.CanonicalJSON()
	if err != nil {
		return
	}
	size := uint64(len(key) + len(encoded))
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if _, exists := cache.items[key]; exists || size > cache.limits.MaxBytes {
		return
	}
	for uint64(len(cache.items)) >= cache.limits.MaxEntries || size > cache.limits.MaxBytes-cache.bytes {
		oldest := cache.order[0]
		cache.order = cache.order[1:]
		cache.bytes -= cache.items[oldest].bytes
		delete(cache.items, oldest)
	}
	cache.items[key] = shapeCacheEntry{key: key, value: cloneShapedText(value), bytes: size}
	cache.order = append(cache.order, key)
	cache.bytes += size
}

func (cache *ShapeCache) Stats() ShapeCacheStats {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	return ShapeCacheStats{Entries: uint64(len(cache.items)), Bytes: cache.bytes, Hits: cache.hits, Misses: cache.misses}
}

type ShapedTextPaintSink interface {
	PaintShapedFontRun(ShapedFontRun, []ShapedGlyph) error
}

// ReplayShapedText reuses immutable glyph IDs/positions without invoking a
// shaper. A painter is responsible only for mapping font resources to output.
func ReplayShapedText(result ShapedText, sink ShapedTextPaintSink) error {
	if sink == nil {
		return errors.New("layoutengine: shaped text paint sink is nil")
	}
	if err := result.Validate(); err != nil {
		return err
	}
	for _, run := range result.FontRuns {
		glyphs := cloneSlice(result.Glyphs[run.GlyphStart : run.GlyphStart+run.GlyphCount])
		if err := sink.PaintShapedFontRun(run, glyphs); err != nil {
			return err
		}
	}
	return nil
}

func sortedShapeFeatures(features []ShapeFeature) []ShapeFeature {
	result := cloneSlice(features)
	sort.Slice(result, func(i, j int) bool { return result[i].Tag < result[j].Tag })
	return result
}
