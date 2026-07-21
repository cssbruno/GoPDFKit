// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sort"

	"github.com/cssbruno/paperrune/internal/layoutengine"
)

// HTMLCharacterizationInventory is the deterministic Stage 0 description of
// the public HTML surface and its currently recognized rendering vocabulary.
// Recognition is deliberately distinct from browser parity.
type HTMLCharacterizationInventory struct {
	SchemaVersion           uint16                        `json:"schema_version"`
	EntryPoints             []string                      `json:"entry_points"`
	RecognizedTags          []string                      `json:"recognized_tags"`
	RecognizedCSSProperties []string                      `json:"recognized_css_properties"`
	CSSValueFamilies        []HTMLCSSValueFamily          `json:"css_value_families"`
	Cursor                  HTMLCursorContract            `json:"cursor"`
	BehaviorClasses         []string                      `json:"behavior_classes"`
	Fixtures                []HTMLCharacterizationFixture `json:"fixtures"`
}

type HTMLCSSValueFamily struct {
	Property string   `json:"property"`
	Values   []string `json:"values"`
}
type HTMLCursorContract struct {
	Entry      string `json:"entry"`
	Exit       string `json:"exit"`
	Pagination string `json:"pagination"`
	Failure    string `json:"failure"`
}
type HTMLCharacterizationFixture struct {
	Name           string `json:"name"`
	Cohort         string `json:"cohort"`
	Classification string `json:"classification"`
	Source         string `json:"source"`
}

// HTMLCharacterization returns a detached, sorted inventory projection.
func HTMLCharacterization() HTMLCharacterizationInventory {
	tags := sortedHTMLCharacterizationKeys(htmlSupportedTags)
	properties := sortedHTMLCharacterizationKeys(htmlSupportedCSSProperties)
	return HTMLCharacterizationInventory{SchemaVersion: 1,
		EntryPoints:    sortedHTMLCharacterizationStrings([]string{"CompileHTML", "CompileHTMLContext", "CompileHTMLTemplate", "CompileHTMLTemplateContext", "HTMLCharacterization", "HTMLCharacterizationJSON", "HTMLTokenize", "HTMLTokenizeContext", "RenderHTMLTemplate", "RunHTMLCharacterization", "(*CompiledHTML).DebugDump", "(*CompiledHTML).RecoveryIssues", "(*CompiledHTML).Stats", "(*CompiledHTML).Tokens", "(*Document).HTMLNew", "(*Document).PlanCompiledHTML", "(*Document).PlanCompiledHTMLContext", "(*HTML).ValidateHTML", "(*HTML).Write", "(*HTML).WriteCompiled", "(*HTML).WriteContext", "(*HTML).WriteTemplate", "(*HTML).WriteTemplateContext"}),
		RecognizedTags: tags, RecognizedCSSProperties: properties,
		CSSValueFamilies: []HTMLCSSValueFamily{{"border-collapse", []string{"collapse", "separate"}}, {"break", []string{"always", "auto", "avoid", "page"}}, {"display", []string{"block", "flex", "inline", "inline-block", "inline-flex"}}, {"flex-direction", []string{"column", "column-reverse", "row", "row-reverse"}}, {"object-fit", []string{"contain", "cover", "fill"}}, {"text-align", []string{"center", "justify", "left", "right"}}, {"vertical-align", []string{"bottom", "middle", "top"}}, {"white-space", []string{"break-spaces", "normal", "nowrap", "pre", "pre-line", "pre-wrap"}}},
		Cursor:           HTMLCursorContract{Entry: "uses the document's current page and XY position", Exit: "leaves XY after the final rendered content", Pagination: "may append bounded pages and continues in the active body region", Failure: "compile/validation failures are pre-render; render-time failures may retain already emitted legacy output"},
		BehaviorClasses:  []string{"recognized-rendered", "recognized-ignored-metadata", "diagnostic-unsupported", "malformed-recovered", "rejected-by-policy", "strict-unified-plannable"},
		Fixtures:         htmlCharacterizationFixtures()}
}

func HTMLCharacterizationJSON() ([]byte, error) { return json.Marshal(HTMLCharacterization()) }

type HTMLFixtureResult struct {
	Name         string                          `json:"name"`
	Outcome      string                          `json:"outcome"`
	Pages        int                             `json:"pages"`
	ReadingRoles []layoutengine.SemanticRole     `json:"reading_roles"`
	PDF          *CharacterizationPDFEvidence    `json:"pdf,omitempty"`
	RasterStatus string                          `json:"raster_status"`
	Raster       *CharacterizationRasterEvidence `json:"raster,omitempty"`
}

type HTMLCharacterizationProjection struct {
	SchemaVersion uint16              `json:"schema_version"`
	Fixtures      []HTMLFixtureResult `json:"fixtures"`
}

func (projection HTMLCharacterizationProjection) CanonicalJSON() ([]byte, error) {
	return json.Marshal(projection)
}

// RunHTMLCharacterization executes every bounded Stage 0 HTML fixture and
// records deterministic PDF evidence for every renderable outcome. Policy,
// unsupported, and recovered-but-unrenderable outcomes remain explicit.
func RunHTMLCharacterization(ctx context.Context) (HTMLCharacterizationProjection, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	projection := HTMLCharacterizationProjection{SchemaVersion: 2,
		Fixtures: make([]HTMLFixtureResult, 0, len(htmlCharacterizationFixtures()))}
	var totalPDFBytes uint64
	rasterBudget := characterizationRasterBudget{}
	for _, fixture := range htmlCharacterizationFixtures() {
		if err := ctx.Err(); err != nil {
			return HTMLCharacterizationProjection{}, err
		}
		compiled, err := CompileHTMLContext(ctx, fixture.Source)
		if err != nil {
			return HTMLCharacterizationProjection{}, err
		}
		entry := HTMLFixtureResult{Name: fixture.Name, RasterStatus: "not-applicable"}
		var output []byte
		var rasterPlan LayoutDocumentPlan
		hasRasterPlan := false
		switch fixture.Classification {
		case "diagnostic-unsupported":
			pdf := newHTMLCharacterizationDocument(true)
			html := pdf.HTMLNew()
			if len(html.ValidateHTML(fixture.Source)) == 0 {
				return HTMLCharacterizationProjection{}, errors.New("document: HTML characterization expected unsupported diagnostic")
			}
			entry.Outcome = "unsupported"
		case "rejected-by-policy":
			pdf := newHTMLCharacterizationDocument(true)
			html := pdf.HTMLNew()
			if err := html.WriteContext(ctx, 10, fixture.Source); err == nil {
				return HTMLCharacterizationProjection{}, errors.New("document: HTML characterization expected policy rejection")
			}
			entry.Outcome = "rejected-by-policy"
		case "strict-unified-plannable":
			planner := newHTMLCharacterizationDocument(false)
			plan, err := planner.PlanCompiledHTMLContext(ctx, 10, compiled)
			if err != nil {
				return HTMLCharacterizationProjection{}, err
			}
			target := newHTMLCharacterizationDocument(false)
			if _, err := target.WriteLayoutDocumentPlanContext(ctx, plan); err != nil {
				return HTMLCharacterizationProjection{}, err
			}
			output, err = htmlCharacterizationOutput(target)
			if err != nil {
				return HTMLCharacterizationProjection{}, err
			}
			entry.Outcome, entry.Pages = "planned", plan.PageCount()
			entry.ReadingRoles = characterizationReadingRoles(plan.plan)
			rasterPlan, hasRasterPlan = plan, true
		default:
			pdf := newHTMLCharacterizationDocument(true)
			html := pdf.HTMLNew()
			if err := html.WriteContext(ctx, 10, fixture.Source); err != nil {
				return HTMLCharacterizationProjection{}, err
			}
			output, err = htmlCharacterizationOutput(pdf)
			if err != nil {
				return HTMLCharacterizationProjection{}, err
			}
			entry.Outcome, entry.Pages = "rendered", pdf.PageCount()
		}
		if len(output) != 0 {
			totalPDFBytes += uint64(len(output))
			if totalPDFBytes > 128<<20 {
				return HTMLCharacterizationProjection{}, ErrHTMLLimitExceeded
			}
			evidence, err := characterizationPDFOutputEvidence(output, entry.Pages)
			if err != nil {
				return HTMLCharacterizationProjection{}, err
			}
			entry.PDF = &evidence
		}
		if hasRasterPlan {
			raster, rasterStatus, rasterErr := captureCharacterizationRaster(ctx, fixture.Name, rasterPlan, &rasterBudget)
			if rasterErr != nil {
				return HTMLCharacterizationProjection{}, rasterErr
			}
			entry.Raster, entry.RasterStatus = raster, rasterStatus
		}
		projection.Fixtures = append(projection.Fixtures, entry)
	}
	return projection, nil
}

func newHTMLCharacterizationDocument(addPage bool) *Document {
	pdf := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 200, Ht: 160}), WithNoCompression(), WithDeterministicOutput())
	pdf.SetMargins(18, 18, 18)
	pdf.SetAutoPageBreak(true, 18)
	if addPage {
		pdf.AddPage()
		pdf.SetFont("Helvetica", "", 10)
	}
	return pdf
}

func htmlCharacterizationOutput(pdf *Document) ([]byte, error) {
	var output bytes.Buffer
	err := pdf.OutputWithOptions(&output, OutputOptions{Deterministic: true})
	return output.Bytes(), err
}

func characterizationReadingRoles(plan layoutengine.LayoutPlan) []layoutengine.SemanticRole {
	projection := plan.Projection()
	byID := make(map[layoutengine.SemanticNodeID]layoutengine.SemanticRole, len(projection.SemanticNodes))
	for _, node := range projection.SemanticNodes {
		byID[node.ID] = node.Role
	}
	roles := make([]layoutengine.SemanticRole, len(projection.ReadingOrder))
	for index, occurrence := range projection.ReadingOrder {
		roles[index] = byID[occurrence.Semantic]
	}
	return roles
}

func sortedHTMLCharacterizationKeys(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for value, enabled := range values {
		if enabled {
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

func sortedHTMLCharacterizationStrings(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}

func htmlCharacterizationFixtures() []HTMLCharacterizationFixture {
	return []HTMLCharacterizationFixture{
		{"text-lists-nested", "text_lists", "recognized-rendered", `<article><h2>Title</h2><p>Hello <strong>world</strong><br>next</p><ul><li>One</li></ul><dl><dt>Term</dt><dd>Definition</dd></dl></article>`},
		{"mixed-flex", "mixed_nested", "recognized-rendered", `<div style="display:flex;gap:2pt"><section><p>A</p></section><div><p>B</p></div></div>`},
		{"table-spans", "tables", "recognized-rendered", `<table><caption>Grid</caption><thead><tr><th colspan="2">Head</th></tr></thead><tbody><tr><td rowspan="2">A</td><td>B</td></tr><tr><td>C</td></tr></tbody><tfoot><tr><td colspan="2">Foot</td></tr></tfoot></table>`},
		{"svg", "svg", "recognized-rendered", `<svg width="12" height="12" viewBox="0 0 12 12"><rect x="1" y="1" width="10" height="10" fill="#123456"/></svg>`},
		{"data-image", "images", "recognized-rendered", `<img width="1" height="1" alt="pixel" src="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==">`},
		{"link", "links", "recognized-rendered", `<p><a href="https://example.test/stage0">link</a></p>`},
		{"forms", "forms", "diagnostic-unsupported", `<form><label>Name<input name="name"></label></form>`},
		{"metadata", "metadata", "recognized-ignored-metadata", `<head><title>Ignored</title><script>ignored()</script></head><p>Body</p>`},
		{"malformed", "recovery", "malformed-recovered", `<div><p>open</div>`},
		{"unsafe-link", "policy", "rejected-by-policy", `<p><a href="javascript:alert(1)">unsafe</a></p>`},
		{"strict-plan", "unified", "strict-unified-plannable", `<main><h1>Plan</h1><p>Exact</p></main>`},
	}
}
