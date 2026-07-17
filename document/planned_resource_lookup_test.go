// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
	"github.com/cssbruno/gopdfkit/layout"
)

func TestPlannedImageLookupIsCancellationAwareAndBounded(t *testing.T) {
	digest := layoutengine.ImageContentDigest(strings.Repeat("a", 64))
	sources := plannedImageSources{digest: []byte("four")}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	budget, err := newPlannedImageLookupBudget(1, 4)
	if err != nil {
		t.Fatal(err)
	}
	if data, err := lookupPlannedImageSourceContext(canceled, sources, digest, &budget); !errors.Is(err, context.Canceled) || data != nil {
		t.Fatalf("canceled lookup = %q, %v", data, err)
	}

	budget, _ = newPlannedImageLookupBudget(1, 3)
	if data, err := lookupPlannedImageSourceContext(context.Background(), sources, digest, &budget); err == nil || data != nil || !strings.Contains(err.Error(), "source bytes exceed limit") {
		t.Fatalf("byte-bounded lookup = %q, %v", data, err)
	}

	budget, _ = newPlannedImageLookupBudget(1, 4)
	if data, err := lookupPlannedImageSourceContext(context.Background(), sources, digest, &budget); err != nil || string(data) != "four" {
		t.Fatalf("exact-limit lookup = %q, %v", data, err)
	}
	if data, err := lookupPlannedImageSourceContext(context.Background(), sources, digest, &budget); err == nil || data != nil || !strings.Contains(err.Error(), "lookup count exceeds limit") {
		t.Fatalf("count-bounded lookup = %q, %v", data, err)
	}
}

func TestTypedPlanBuildsDetachedCumulativeBoundedImageCatalog(t *testing.T) {
	first := encodeResourceLookupPNG(t, color.NRGBA{R: 255, A: 255})
	second := encodeResourceLookupPNG(t, color.NRGBA{B: 255, A: 255})
	limit := len(first)
	if len(second) > limit {
		limit = len(second)
	}
	doc := &layout.LayoutDocument{Body: []layout.Block{
		layout.ImageBlock{Data: first, Format: "png", Width: 10, Height: 10},
		layout.ImageBlock{Data: second, Format: "png", Width: 10, Height: 10},
	}}
	planner := MustNew(WithUnit(UnitPoint), WithLimits(Limits{MaxImageSourceBytes: int64(limit)}))
	if plan, err := planner.PlanLayoutDocumentContext(context.Background(), doc); err == nil || plan.PageCount() != 0 || !strings.Contains(err.Error(), "cumulative planned image source bytes exceed limit") {
		t.Fatalf("cumulative catalog plan pages=%d err=%v", plan.PageCount(), err)
	}

	sources, err := typedLayoutImageSourcesContext(context.Background(), doc, uint64(len(first)+len(second)))
	if err != nil || len(sources) != 2 {
		t.Fatalf("exact-limit catalog resources=%d err=%v", len(sources), err)
	}
	for digest, encoded := range sources {
		before := append([]byte(nil), encoded...)
		first[0] ^= 0xff
		second[0] ^= 0xff
		if !bytes.Equal(encoded, before) {
			t.Fatalf("catalog resource %s aliases caller bytes", digest)
		}
		first[0] ^= 0xff
		second[0] ^= 0xff
	}
}

func TestTypedImageCatalogHonorsCanceledContext(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	doc := &layout.LayoutDocument{Body: []layout.Block{layout.ImageBlock{Data: []byte("image"), Format: "png"}}}
	if sources, err := typedLayoutImageSourcesContext(canceled, doc, 64); !errors.Is(err, context.Canceled) || sources != nil {
		t.Fatalf("canceled catalog = %#v, %v", sources, err)
	}
}

func encodeResourceLookupPNG(t *testing.T, value color.NRGBA) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.SetNRGBA(0, 0, value)
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
