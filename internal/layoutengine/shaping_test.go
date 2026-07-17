// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

type fixedASCIIProvider struct{ advance Fixed }

func (provider fixedASCIIProvider) CoreASCIIAdvances(_ CoreFontFace, text string, _ Fixed) ([]Fixed, error) {
	result := make([]Fixed, len(text))
	for index := range result {
		result[index] = provider.advance
	}
	return result, nil
}

func TestCoreASCIIShaperPreservesByteClustersAndMetrics(t *testing.T) {
	input := shapingFixture("ABC")
	result, err := ShapeText(context.Background(), CoreASCIIShaper{Metrics: fixedASCIIProvider{advance: 7}}, input, ShapingLimits{}, nil)
	if err != nil {
		t.Fatalf("ShapeText() = %v", err)
	}
	for index, glyph := range result.Glyphs {
		if glyph.ID != uint32(input.Text[index]) || glyph.Advance != 7 || glyph.Offset != (Point{}) ||
			glyph.Cluster != (UTF8Cluster{Start: uint32(index), End: uint32(index + 1)}) {
			t.Fatalf("glyph %d = %+v", index, glyph)
		}
	}
	if len(result.FontRuns) != 1 || result.FontRuns[0].Font != input.Font || result.Direction != DirectionLTR {
		t.Fatalf("shape result = %+v", result)
	}
	first, err := result.CanonicalJSON()
	if err != nil {
		t.Fatalf("CanonicalJSON() = %v", err)
	}
	second, _ := result.CanonicalJSON()
	if !reflect.DeepEqual(first, second) {
		t.Fatal("canonical shaping serialization is unstable")
	}
}

func TestCoreASCIIShaperTruthfullyRejectsCombiningAndRTL(t *testing.T) {
	for _, mutate := range []func(*ShapingInput){
		func(input *ShapingInput) { input.Text = "e\u0301" },
		func(input *ShapingInput) { input.Text, input.Direction = "abc", DirectionRTL },
	} {
		input := shapingFixture("abc")
		mutate(&input)
		_, err := ShapeText(context.Background(), CoreASCIIShaper{Metrics: fixedASCIIProvider{advance: 7}}, input, ShapingLimits{}, nil)
		if !errors.Is(err, ErrShapingUnsupported) {
			t.Fatalf("ShapeText() = %v, want unsupported", err)
		}
		var planning *PlanningError
		if !errors.As(err, &planning) || planning.Diagnostic.Code != DiagnosticGlyphMissing {
			t.Fatalf("unsupported diagnostic = %#v", err)
		}
	}
}

type characterizedExternalShaper struct{ calls atomic.Uint64 }

func (*characterizedExternalShaper) StableID() string { return "characterized-external-v1" }

func (shaper *characterizedExternalShaper) Shape(_ context.Context, input ShapingInput, budget *ShapingBudget) (ShapedText, error) {
	shaper.calls.Add(1)
	if err := budget.Charge(2); err != nil {
		return ShapedText{}, err
	}
	fallback := input.Fallbacks[0]
	return ShapedText{
		Text: input.Text, Language: input.Language, Direction: input.Direction, Source: input.Source,
		Glyphs: []ShapedGlyph{
			{ID: 20, Advance: 6, Cluster: UTF8Cluster{Start: 2, End: 4}, FontRun: 0},
			{ID: 10, Advance: 6, Cluster: UTF8Cluster{Start: 0, End: 2}, FontRun: 0},
		},
		FontRuns: []ShapedFontRun{{Font: fallback, GlyphCount: 2, TextEnd: 4, Fallback: true}},
		Diagnostics: []Diagnostic{{
			Code: DiagnosticFontMissing, Severity: SeverityWarning, Stage: StageLayout,
			Message:  "primary font lacked the requested script; deterministic fallback selected",
			Evidence: []DiagnosticEvidence{{Key: "fallback", Value: fallback.Name}},
		}},
	}, nil
}

func TestExternalShaperContractAcceptsRTLClustersAndFallbackProvenance(t *testing.T) {
	input := shapingFixture("אב")
	input.Direction = DirectionRTL
	input.Language = "he"
	input.Fallbacks = []ShapeFont{{Name: "fallback-hebrew", Digest: digestOf("2")}}
	shaper := &characterizedExternalShaper{}
	result, err := ShapeText(context.Background(), shaper, input, ShapingLimits{}, nil)
	if err != nil {
		t.Fatalf("ShapeText() = %v", err)
	}
	if result.Glyphs[0].Cluster.Start != 2 || result.Glyphs[1].Cluster.Start != 0 || !result.FontRuns[0].Fallback ||
		result.FontRuns[0].Font != input.Fallbacks[0] || result.Diagnostics[0].Code != DiagnosticFontMissing {
		t.Fatalf("external shaped result = %+v", result)
	}
}

func TestShapeCacheIsByteBoundedAndConcurrent(t *testing.T) {
	cache, err := NewShapeCache(ShapeCacheLimits{MaxEntries: 2, MaxBytes: 2048})
	if err != nil {
		t.Fatalf("NewShapeCache() = %v", err)
	}
	shaper := &characterizedExternalShaper{}
	input := shapingFixture("אב")
	input.Direction, input.Language = DirectionRTL, "he"
	input.Fallbacks = []ShapeFont{{Name: "fallback-hebrew", Digest: digestOf("2")}}
	if _, err := ShapeText(context.Background(), shaper, input, ShapingLimits{}, cache); err != nil {
		t.Fatalf("warm ShapeText() = %v", err)
	}
	var wait sync.WaitGroup
	for index := 0; index < 32; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if _, err := ShapeText(context.Background(), shaper, input, ShapingLimits{}, cache); err != nil {
				t.Errorf("cached ShapeText() = %v", err)
			}
		}()
	}
	wait.Wait()
	stats := cache.Stats()
	if shaper.calls.Load() != 1 || stats.Hits != 32 || stats.Entries > 2 || stats.Bytes > 2048 {
		t.Fatalf("cache stats = %+v, shaper calls = %d", stats, shaper.calls.Load())
	}
	returned, _ := ShapeText(context.Background(), shaper, input, ShapingLimits{}, cache)
	returned.Glyphs[0].ID = 999
	again, _ := ShapeText(context.Background(), shaper, input, ShapingLimits{}, cache)
	if again.Glyphs[0].ID == 999 {
		t.Fatal("cached result aliases caller-owned glyphs")
	}
	returned.Diagnostics[0].Evidence[0].Value = "mutated"
	again, _ = ShapeText(context.Background(), shaper, input, ShapingLimits{}, cache)
	if again.Diagnostics[0].Evidence[0].Value != "fallback-hebrew" {
		t.Fatal("cached result aliases nested diagnostic evidence")
	}
}

func TestShapedFontRunsMustExactlyPartitionGlyphsAndText(t *testing.T) {
	input := shapingFixture("ABC")
	result, err := ShapeText(context.Background(), CoreASCIIShaper{Metrics: fixedASCIIProvider{advance: 1}}, input, ShapingLimits{}, nil)
	if err != nil {
		t.Fatalf("ShapeText() = %v", err)
	}
	tests := []struct {
		name   string
		mutate func(*ShapedText)
	}{
		{"glyph overlap", func(value *ShapedText) {
			value.FontRuns = []ShapedFontRun{
				{Font: input.Font, GlyphCount: 2, TextEnd: 2},
				{Font: input.Font, GlyphStart: 1, GlyphCount: 1, TextStart: 2, TextEnd: 3},
			}
			value.Glyphs[2].FontRun = 1
		}},
		{"text overlap", func(value *ShapedText) {
			value.FontRuns = []ShapedFontRun{
				{Font: input.Font, GlyphCount: 2, TextEnd: 2},
				{Font: input.Font, GlyphStart: 2, GlyphCount: 1, TextStart: 1, TextEnd: 3},
			}
			value.Glyphs[2].FontRun = 1
		}},
		{"glyph gap", func(value *ShapedText) {
			value.FontRuns[0].GlyphStart = 1
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			invalid := cloneShapedText(result)
			test.mutate(&invalid)
			if err := invalid.Validate(); !errors.Is(err, ErrShapingInvalid) {
				t.Fatalf("Validate() = %v", err)
			}
		})
	}
}

func TestShapingLimitsRejectValuesAboveHardCaps(t *testing.T) {
	limits := DefaultShapingLimits()
	limits.MaxGlyphs = hardMaxShapingGlyphs + 1
	_, err := ShapeText(context.Background(), CoreASCIIShaper{Metrics: fixedASCIIProvider{advance: 1}}, shapingFixture("A"), limits, nil)
	if !errors.Is(err, ErrShapingInvalid) && err == nil {
		t.Fatalf("oversized limits ShapeText() = %v", err)
	}
}

type shapedRecordingSink struct{ glyphs []ShapedGlyph }

func (sink *shapedRecordingSink) PaintShapedFontRun(_ ShapedFontRun, glyphs []ShapedGlyph) error {
	sink.glyphs = append(sink.glyphs, glyphs...)
	glyphs[0].ID = 999
	return nil
}

func TestReplayShapedTextUsesExactGlyphsWithoutReshaping(t *testing.T) {
	input := shapingFixture("ABC")
	shaper := CoreASCIIShaper{Metrics: fixedASCIIProvider{advance: 9}}
	result, err := ShapeText(context.Background(), shaper, input, ShapingLimits{}, nil)
	if err != nil {
		t.Fatalf("ShapeText() = %v", err)
	}
	sink := &shapedRecordingSink{}
	if err := ReplayShapedText(result, sink); err != nil {
		t.Fatalf("ReplayShapedText() = %v", err)
	}
	if len(sink.glyphs) != 3 || sink.glyphs[1] != result.Glyphs[1] || result.Glyphs[0].ID != 'A' {
		t.Fatalf("replayed glyphs = %+v, result = %+v", sink.glyphs, result.Glyphs)
	}
}

func TestShapeTextHonorsCancellationAndWorkLimits(t *testing.T) {
	input := shapingFixture("ABC")
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := ShapeText(canceled, CoreASCIIShaper{Metrics: fixedASCIIProvider{advance: 1}}, input, ShapingLimits{}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled ShapeText() = %v", err)
	}
	limits := DefaultShapingLimits()
	limits.MaxWork = uint64(len(input.Text))
	_, err = ShapeText(context.Background(), CoreASCIIShaper{Metrics: fixedASCIIProvider{advance: 1}}, input, limits, nil)
	if !errors.Is(err, ErrShapingWorkLimit) {
		t.Fatalf("work-limited ShapeText() = %v", err)
	}
}

func shapingFixture(text string) ShapingInput {
	return ShapingInput{
		Text:     text,
		Font:     ShapeFont{Name: "courier", Digest: digestOf("1"), CoreFace: CoreFontCourier},
		FontSize: Fixed(10 * FixedScale), Language: "en", Direction: DirectionLTR,
	}
}

func digestOf(value string) ShapeFontDigest {
	return ShapeFontDigest(strings.Repeat(value, 64))
}
