// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
)

func TestWebDisplayRenderPayloadRoundTripsThroughSharedRasterizer(t *testing.T) {
	plan, sources := rasterFixture(t)
	request := rasterRequest()
	payload, err := EncodeWebDisplayRenderPayload(plan, sources, request)
	if err != nil {
		t.Fatal(err)
	}
	web, err := RenderWebDisplayPayload(t.Context(), payload)
	if err != nil {
		t.Fatal(err)
	}
	direct, err := CaptureDisplayPlanPNG(plan, sources, request)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(web.PNG(), direct.PNG()) || web.Manifest().PNGSHA256 != direct.Manifest().PNGSHA256 || web.Manifest().PlanHash != direct.Manifest().PlanHash {
		t.Fatal("web payload did not produce the exact direct display-list raster")
	}
	var decoded WebDisplayRenderPayload
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.FormatVersion != WebDisplayRenderPayloadVersion || decoded.Renderer != DisplayRasterRendererVersion || len(decoded.Bindings) != 1 || len(decoded.Blobs) != 1 {
		t.Fatalf("payload contract = %+v", decoded)
	}
}

func TestWebDisplayRenderCachePreservesPixelsAndClearsOnPlanChange(t *testing.T) {
	plan, sources := rasterFixture(t)
	payload, err := EncodeWebDisplayRenderPayload(plan, sources, rasterRequest())
	if err != nil {
		t.Fatal(err)
	}
	var cache WebDisplayRenderCache
	first, err := RenderWebDisplayPayloadCached(t.Context(), payload, &cache)
	if err != nil {
		t.Fatal(err)
	}
	second, err := RenderWebDisplayPayloadCached(t.Context(), payload, &cache)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.PNG(), second.PNG()) || len(cache.fonts) != 1 {
		t.Fatal("cached render changed pixels or failed to retain the validated font")
	}
	cache.prepare("next-plan")
	if len(cache.fonts) != 0 || len(cache.images) != 0 {
		t.Fatal("cache retained resources after plan identity changed")
	}
}

func TestWebDisplayRenderPayloadRejectsTamperingAndUnknownFields(t *testing.T) {
	plan, sources := rasterFixture(t)
	payload, err := EncodeWebDisplayRenderPayload(plan, sources, rasterRequest())
	if err != nil {
		t.Fatal(err)
	}
	var decoded WebDisplayRenderPayload
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	decoded.PlanHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	tampered, err := json.Marshal(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RenderWebDisplayPayload(t.Context(), tampered); !errors.Is(err, ErrWebDisplayRenderPayload) {
		t.Fatalf("tampered plan hash error = %v", err)
	}
	unknown := append(append([]byte(nil), payload[:len(payload)-1]...), []byte(`,"injected":true}`)...)
	if _, err := RenderWebDisplayPayload(t.Context(), unknown); !errors.Is(err, ErrWebDisplayRenderPayload) {
		t.Fatalf("unknown field error = %v", err)
	}
	decoded.PlanHash = planHashString(t, plan)
	decoded.Blobs[0].Bytes[0] ^= 0xff
	tampered, err = json.Marshal(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RenderWebDisplayPayload(t.Context(), tampered); !errors.Is(err, ErrWebDisplayRenderPayload) {
		t.Fatalf("tampered resource error = %v", err)
	}
}

func planHashString(t *testing.T, plan LayoutPlan) string {
	t.Helper()
	hash, err := plan.Hash()
	if err != nil {
		t.Fatal(err)
	}
	return hash.String()
}
