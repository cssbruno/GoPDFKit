// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package browseroracle

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image/png"
	"testing"
	"time"
)

func TestPinnedFirefoxCapturesCanonicalDOMRectsAndScreenshot(t *testing.T) {
	source := `<!doctype html><meta charset="utf-8"><style>
html,body{margin:0;width:240px;height:160px;background:white}#row{display:flex;width:200px;height:40px;gap:10px;align-items:flex-start;background:#eee}.item{flex:0 0 40px;height:20px;background:#123456}
</style><div id="row"><div class="item" id="a"></div><div class="item" id="b"></div></div>`
	expression := `(()=>["row","a","b"].map(id=>{const r=document.getElementById(id).getBoundingClientRect();return{id,x:r.x,y:r.y,width:r.width,height:r.height}}))()`
	capture, err := CaptureFirefox(context.Background(), source, expression, Options{Width: 240, Height: 160, Timeout: 15 * time.Second})
	if errors.Is(err, ErrBrowserUnavailable) {
		t.Skipf("external browser oracle unavailable: %v", err)
	}
	if err != nil {
		t.Fatal(err)
	}
	var rects []struct {
		ID                  string
		X, Y, Width, Height float64
	}
	if err := json.Unmarshal(capture.DOMRects, &rects); err != nil {
		t.Fatal(err)
	}
	want := []struct {
		ID                  string
		X, Y, Width, Height float64
	}{{"row", 0, 0, 200, 40}, {"a", 0, 0, 40, 20}, {"b", 50, 0, 40, 20}}
	if len(rects) != len(want) {
		t.Fatalf("DOMRects = %s", capture.DOMRects)
	}
	for index := range want {
		if rects[index] != want[index] {
			t.Fatalf("DOMRect %d = %+v, want %+v", index, rects[index], want[index])
		}
	}
	image, err := png.Decode(bytes.NewReader(capture.PNG))
	if err != nil || image.Bounds().Dx() != 240 || image.Bounds().Dy() != 160 {
		t.Fatalf("screenshot bounds = %v err=%v", image.Bounds(), err)
	}
}
