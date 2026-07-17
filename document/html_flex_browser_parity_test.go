// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image/png"
	"math"
	"testing"
	"time"

	"github.com/cssbruno/gopdfkit/internal/browseroracle"
	"github.com/cssbruno/gopdfkit/internal/layoutengine"
	"golang.org/x/image/font/gofont/goregular"
)

type browserFlexRect struct {
	ID                  string  `json:"id"`
	X, Y, Width, Height float64 `json:",omitempty"`
}

func TestHTMLUnifiedFlexPinnedBrowserGeometryAndRasterCorpus(t *testing.T) {
	const browserReset = `<style>html,body{margin:0;width:240px;height:160px;overflow:hidden;background:#fff}p,h2,section,div{box-sizing:border-box;margin:0;padding:0}</style>`
	fixtures := []struct {
		name, paper, browser, expression string
		planFragments                    []int
	}{
		{
			name: "nowrap-align-justify",
			paper: `<style>.row{display:flex;gap:10pt;justify-content:space-between;align-items:flex-start}.a{flex:0 0 40pt;height:20pt;line-height:12pt}.b{flex:0 0 40pt;height:30pt;line-height:12pt}</style>` +
				`<div class="row"><p class="a">.</p><p class="b">.</p></div>`,
			browser: browserReset + `<style>.row{position:absolute;left:20px;top:20px;width:200px;display:flex;gap:10px;justify-content:space-between;align-items:flex-start}.a{flex:0 0 40px;height:20px;line-height:12px}.b{flex:0 0 40px;height:30px;line-height:12px}</style>` +
				`<div class="row"><p class="a">.</p><p class="b">.</p></div>`,
			expression:    `(()=>["a","b"].map(id=>{const r=document.querySelector("."+id).getBoundingClientRect();return{id,x:r.x,y:r.y,width:r.width,height:r.height}}))()`,
			planFragments: []int{0, 1},
		},
		{
			name: "nested-column",
			paper: `<style>.outer{display:flex;gap:10pt;align-items:flex-start}.stack{display:flex;flex-direction:column;flex:0 0 90pt;gap:4pt;align-items:stretch}.n1,.n2{flex:0 0 18pt;height:18pt;line-height:12pt}.peer{flex:1 1 0;height:12pt;line-height:12pt}</style>` +
				`<div class="outer"><section class="stack"><p class="n1">.</p><h2 class="n2">.</h2></section><p class="peer">.</p></div>`,
			browser: browserReset + `<style>.outer{position:absolute;left:20px;top:20px;width:200px;display:flex;gap:10px;align-items:flex-start}.stack{display:flex;flex-direction:column;flex:0 0 90px;gap:4px;align-items:stretch}.n1,.n2{flex:0 0 18px;height:18px;line-height:12px}.peer{flex:1 1 0;height:12px;line-height:12px}</style>` +
				`<div class="outer"><section class="stack"><p class="n1">.</p><h2 class="n2">.</h2></section><p class="peer">.</p></div>`,
			expression:    `(()=>["stack","peer","n1","n2"].map(id=>{const r=document.querySelector("."+id).getBoundingClientRect();return{id,x:r.x,y:r.y,width:r.width,height:r.height}}))()`,
			planFragments: []int{0, 1, 2, 3},
		},
		{
			name: "wrapped-row",
			paper: `<style>.wrap{display:flex;flex-wrap:wrap;height:50pt;gap:5pt 10pt;align-content:flex-start;align-items:flex-start}.w1,.w2,.w3{flex:0 0 80pt;height:18pt;line-height:12pt}</style>` +
				`<div class="wrap"><p class="w1">.</p><p class="w2">.</p><p class="w3">.</p></div>`,
			browser: browserReset + `<style>.wrap{position:absolute;left:20px;top:20px;width:200px;height:50px;display:flex;flex-wrap:wrap;gap:5px 10px;align-content:flex-start;align-items:flex-start}.w1,.w2,.w3{flex:0 0 80px;height:18px;line-height:12px}</style>` +
				`<div class="wrap"><p class="w1">.</p><p class="w2">.</p><p class="w3">.</p></div>`,
			expression:    `(()=>["w1","w2","w3"].map(id=>{const r=document.querySelector("."+id).getBoundingClientRect();return{id,x:r.x,y:r.y,width:r.width,height:r.height}}))()`,
			planFragments: []int{0, 1, 2},
		},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			compiled, err := CompileHTML(fixture.paper)
			if err != nil {
				t.Fatal(err)
			}
			plan, err := htmlUnifiedFlexTestPlanner().PlanCompiledHTMLContext(context.Background(), 12, compiled)
			if err != nil {
				t.Fatal(err)
			}
			capture, err := browseroracle.CaptureFirefox(t.Context(), fixture.browser, fixture.expression, browseroracle.Options{
				Width: 240, Height: 160, Timeout: 15 * time.Second,
			})
			if errors.Is(err, browseroracle.ErrBrowserUnavailable) {
				t.Skipf("pinned external browser oracle unavailable: %v", err)
			}
			if err != nil {
				t.Fatal(err)
			}
			var rects []browserFlexRect
			if err := json.Unmarshal(capture.DOMRects, &rects); err != nil || len(rects) != len(fixture.planFragments) {
				t.Fatalf("browser DOMRects = %s err=%v", capture.DOMRects, err)
			}
			projection := plan.plan.Projection()
			for index, fragmentIndex := range fixture.planFragments {
				if fragmentIndex < 0 || fragmentIndex >= len(projection.Fragments) {
					t.Fatalf("plan fragment index %d is absent", fragmentIndex)
				}
				box := projection.Fragments[fragmentIndex].BorderBox
				want := []float64{box.X.Points(), box.Y.Points(), box.Width.Points(), box.Height.Points()}
				got := []float64{rects[index].X, rects[index].Y, rects[index].Width, rects[index].Height}
				for axis := range want {
					if math.Abs(got[axis]-want[axis]) > 1.0/1024.0 {
						t.Fatalf("DOMRect %s axis %d = %.6f, plan %.6f; all=%s", rects[index].ID, axis, got[axis], want[axis], capture.DOMRects)
					}
				}
			}
			planPNG := captureFlexPlanPNG(t, plan)
			changed, total, maxDelta, mean := compareFlexPNGs(t, planPNG, capture.PNG)
			t.Logf("Firefox %s raster changed=%d/%d max_channel_delta=%d mean_channel_delta=%.4f", capture.Version, changed, total, maxDelta, mean)
			if changed*1000 > total*8 || maxDelta > 255 || mean > 1.5 {
				t.Fatalf("browser raster parity changed=%d/%d max=%d mean=%.4f", changed, total, maxDelta, mean)
			}
		})
	}
}

func captureFlexPlanPNG(t *testing.T, plan LayoutDocumentPlan) []byte {
	t.Helper()
	projection := plan.plan.Projection()
	fonts := make(map[layoutengine.CoreFontMetricsDigest][]byte, len(projection.Fonts))
	for _, font := range projection.Fonts {
		fonts[font.MetricsDigest] = goregular.TTF
	}
	profile := layoutengine.DefaultDisplayRasterProfile()
	profile.DPI = 72
	request := layoutengine.DisplayRasterRequest{
		Page: 1, Profile: profile, Limits: layoutengine.DefaultDisplayRasterLimits(), PageProfile: characterizationDigest("browser-flex-page-profile"),
		Revisions: layoutengine.ViewerRevisionIdentityInput{SourceRevision: characterizationDigest("browser-flex-source"), ScenarioRevision: characterizationDigest("browser-flex-scenario"), PolicyRevision: characterizationDigest("browser-flex-policy")},
	}
	artifact, err := layoutengine.CaptureDisplayPlanPNGContext(t.Context(), plan.plan, layoutengine.DisplayRasterSources{FontPrograms: fonts}, request)
	if err != nil {
		t.Fatal(err)
	}
	return artifact.PNG()
}

func compareFlexPNGs(t *testing.T, leftBytes, rightBytes []byte) (changed, total, maxDelta int, mean float64) {
	t.Helper()
	left, err := png.Decode(bytes.NewReader(leftBytes))
	if err != nil {
		t.Fatal(err)
	}
	right, err := png.Decode(bytes.NewReader(rightBytes))
	if err != nil {
		t.Fatal(err)
	}
	if left.Bounds() != right.Bounds() {
		t.Fatalf("raster dimensions = %v / %v", left.Bounds(), right.Bounds())
	}
	var sum uint64
	for y := left.Bounds().Min.Y; y < left.Bounds().Max.Y; y++ {
		for x := left.Bounds().Min.X; x < left.Bounds().Max.X; x++ {
			total++
			lr, lg, lb, _ := left.At(x, y).RGBA()
			rr, rg, rb, _ := right.At(x, y).RGBA()
			pixelChanged := false
			for _, delta := range []int{absByte(int(lr>>8) - int(rr>>8)), absByte(int(lg>>8) - int(rg>>8)), absByte(int(lb>>8) - int(rb>>8))} {
				sum += uint64(delta)
				if delta > maxDelta {
					maxDelta = delta
				}
				pixelChanged = pixelChanged || delta != 0
			}
			if pixelChanged {
				changed++
			}
		}
	}
	return changed, total, maxDelta, float64(sum) / float64(total*3)
}

func absByte(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
