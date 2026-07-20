// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cssbruno/paperrune/internal/browseroracle"
)

func TestHTMLUnifiedTextWhitespaceAndInlineStylePinnedBrowserOracle(t *testing.T) {
	const fragment = `<p id="plain">alpha   <strong>beta</strong><br>gamma</p><p id="lines" style="white-space:pre-line">one   two
three</p>`
	compiled, err := CompileHTML(`<style>p{font-family:Courier;font-size:10pt;line-height:12pt}</style>` + fragment)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := paginationTestDocument(t, 100).PlanCompiledHTML(12, compiled)
	if err != nil {
		t.Fatal(err)
	}
	var planned strings.Builder
	projection := plan.plan.Projection()
	for lineIndex := range projection.Lines {
		if planned.Len() != 0 {
			planned.WriteByte('\n')
		}
		for _, run := range projection.GlyphRuns {
			if int(run.Line) == lineIndex {
				planned.WriteString(run.Codes)
			}
		}
	}
	for _, want := range []string{"alpha beta", "gamma", "one two", "three"} {
		if !strings.Contains(planned.String(), want) {
			t.Fatalf("planned browser cohort lacks %q: %q", want, planned.String())
		}
	}

	browserSource := `<style>html,body{margin:0}p{margin:0;font:10px/12px monospace;width:140px}</style>` + fragment
	capture, err := browseroracle.CaptureFirefox(t.Context(), browserSource,
		`(()=>({plain:document.querySelector("#plain").innerText,lines:document.querySelector("#lines").innerText,weight:getComputedStyle(document.querySelector("strong")).fontWeight,whiteSpace:getComputedStyle(document.querySelector("#lines")).whiteSpace}))()`,
		browseroracle.Options{Width: 180, Height: 100, Timeout: 15 * time.Second})
	if errors.Is(err, browseroracle.ErrBrowserUnavailable) {
		t.Skipf("pinned external browser oracle unavailable: %v", err)
	}
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Plain, Lines, Weight, WhiteSpace string
	}
	if err := json.Unmarshal(capture.DOMRects, &got); err != nil {
		t.Fatal(err)
	}
	if got.Plain != "alpha beta\ngamma" || got.Lines != "one two\nthree" || got.Weight != "700" || got.WhiteSpace != "pre-line" {
		t.Fatalf("Firefox %s text/style oracle = %+v", capture.Version, got)
	}
}
