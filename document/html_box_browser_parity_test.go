// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"encoding/json"
	"errors"
	"image/color"
	"image/png"
	"math"
	"testing"
	"time"

	"github.com/cssbruno/paperrune/internal/browseroracle"
)

func TestHTMLUnifiedRoundedShadowPinnedBrowserGeometryAndPixels(t *testing.T) {
	compiled, err := CompileHTML(htmlUnifiedRoundedShadowFixture)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := htmlUnifiedFlexTestPlanner().PlanCompiledHTML(12, compiled)
	if err != nil {
		t.Fatal(err)
	}
	browser := `<style>html,body{margin:0;width:240px;height:160px;background:#fff}.body{position:absolute;left:20px;top:20px;width:200px;height:120px}` +
		`.box{font:12px/12px sans-serif;margin:4px;padding:5px;background-color:#eef4fa;border:2px solid #28445f;border-radius:7px;box-shadow:3px 4px 0 2px #6c7884}</style>` +
		`<div class="body"><p class="box">Rounded shadow</p></div>`
	capture, err := browseroracle.CaptureFirefox(t.Context(), browser,
		`(()=>{const r=document.querySelector(".box").getBoundingClientRect();return{x:r.x,y:r.y,width:r.width,height:r.height}})()`,
		browseroracle.Options{Width: 240, Height: 160, Timeout: 15 * time.Second})
	if errors.Is(err, browseroracle.ErrBrowserUnavailable) {
		t.Skipf("pinned external browser oracle unavailable: %v", err)
	}
	if err != nil {
		t.Fatal(err)
	}
	var got struct{ X, Y, Width, Height float64 }
	if err := json.Unmarshal(capture.DOMRects, &got); err != nil {
		t.Fatal(err)
	}
	box := plan.plan.Projection().Fragments[0].BorderBox
	want := []float64{box.X.Points(), box.Y.Points(), box.Width.Points(), box.Height.Points()}
	actual := []float64{got.X, got.Y, got.Width, got.Height}
	for axis := range want {
		if math.Abs(actual[axis]-want[axis]) > 1.0/1024.0 {
			t.Fatalf("Firefox %s rounded axis %d = %.6f, plan %.6f; DOM=%s", capture.Version, axis, actual[axis], want[axis], capture.DOMRects)
		}
	}
	image, err := png.Decode(bytes.NewReader(capture.PNG))
	if err != nil {
		t.Fatal(err)
	}
	assertPixel := func(x, y int, want color.RGBA) {
		t.Helper()
		if got := color.RGBAModel.Convert(image.At(x, y)).(color.RGBA); got != want {
			t.Fatalf("Firefox rounded pixel(%d,%d)=%+v want %+v", x, y, got, want)
		}
	}
	assertPixel(int(got.X+got.Width/2), int(got.Y), color.RGBA{40, 68, 95, 255})
	assertPixel(int(got.X+got.Width/2), int(got.Y+5), color.RGBA{238, 244, 250, 255})
	assertPixel(int(got.X+got.Width+3), int(got.Y+got.Height/2), color.RGBA{108, 120, 132, 255})
	assertPixel(int(got.X), int(got.Y), color.RGBA{255, 255, 255, 255})
}

func TestHTMLUnifiedBoxModelPinnedBrowserGeometry(t *testing.T) {
	const paperStyle = `margin:5%;padding:5%;border:1pt solid #445566;width:50%;min-width:40%;max-width:60%;height:40pt;min-height:30pt;max-height:50pt;box-sizing:border-box;overflow:hidden;background-color:#ddeeff`
	compiled, err := CompileHTML(`<p style="` + paperStyle + `">Box</p>`)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := htmlUnifiedFlexTestPlanner().PlanCompiledHTML(12, compiled)
	if err != nil {
		t.Fatal(err)
	}
	const browserStyle = `margin:5%;padding:5%;border:1px solid #445566;width:50%;min-width:40%;max-width:60%;height:40px;min-height:30px;max-height:50px;box-sizing:border-box;overflow:hidden;background-color:#ddeeff`
	browser := `<style>html,body{margin:0;width:240px;height:160px}.body{position:absolute;left:20px;top:20px;width:200px;height:120px}.box{` + browserStyle + `}</style><div class="body"><p class="box">Box</p></div>`
	capture, err := browseroracle.CaptureFirefox(t.Context(), browser,
		`(()=>{const r=document.querySelector(".box").getBoundingClientRect();return{x:r.x,y:r.y,width:r.width,height:r.height}})()`,
		browseroracle.Options{Width: 240, Height: 160, Timeout: 15 * time.Second})
	if errors.Is(err, browseroracle.ErrBrowserUnavailable) {
		t.Skipf("pinned external browser oracle unavailable: %v", err)
	}
	if err != nil {
		t.Fatal(err)
	}
	var got struct{ X, Y, Width, Height float64 }
	if err := json.Unmarshal(capture.DOMRects, &got); err != nil {
		t.Fatal(err)
	}
	box := plan.plan.Projection().Fragments[0].BorderBox
	want := []float64{box.X.Points(), box.Y.Points(), box.Width.Points(), box.Height.Points()}
	actual := []float64{got.X, got.Y, got.Width, got.Height}
	for axis := range want {
		if math.Abs(actual[axis]-want[axis]) > 1.0/1024.0 {
			t.Fatalf("Firefox %s box axis %d = %.6f, plan %.6f; DOM=%s", capture.Version, axis, actual[axis], want[axis], capture.DOMRects)
		}
	}
}
