// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"image"
	"image/color"
	"image/png"
	"strings"
	"sync"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
	"github.com/cssbruno/gopdfkit/layout"
)

func TestLayoutDocumentPlanLowersNestedSectionClauseAndNoteToExactPDF(t *testing.T) {
	source := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 220, Ht: 160}), WithNoCompression())
	source.SetMargins(18, 18, 18)
	source.SetAutoPageBreak(true, 18)
	doc := &layout.LayoutDocument{
		Title: "Typed exact plan", Language: "en-US",
		Metadata: layout.DocumentMetadata{Subject: "Cutover", Author: "GoPDFKit"},
		Body: []layout.Block{
			layout.SectionBlock{Title: "Overview", Blocks: []layout.Block{
				layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "Section paragraph"}}},
				layout.ClauseBlock{Number: "1.", Title: "Terms", Blocks: []layout.Block{
					layout.ListBlock{Ordered: true, Items: []layout.ListItem{
						{Blocks: []layout.Block{layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "First item"}}}}},
						{Blocks: []layout.Block{layout.HeadingBlock{Level: 4, Segments: []layout.TextSegment{{Text: "Second item"}}}}},
					}},
					layout.NoteBoxBlock{Title: "Notice", Body: []layout.Block{
						layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "Nested note body"}}},
					}},
				}},
			}},
			layout.PageBreakBlock{After: true},
			layout.ClauseBlock{Number: "2.", Title: "After break", Blocks: []layout.Block{
				layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "Second page text"}}},
			}},
		},
	}

	plan, err := source.PlanLayoutDocument(doc)
	if err != nil || plan.PageCount() != 2 || plan.Hash() == "" || source.PageCount() != 0 {
		t.Fatalf("PlanLayoutDocument() = pages %d hash %q source pages %d, %v", plan.PageCount(), plan.Hash(), source.PageCount(), err)
	}
	target := MustNew(WithUnit(UnitPoint), WithNoCompression(), WithDeterministicOutput())
	pages, err := target.WriteLayoutDocumentPlan(plan)
	if err != nil || pages != 2 || target.PageCount() != 2 {
		t.Fatalf("WriteLayoutDocumentPlan() = %d, pages %d, %v", pages, target.PageCount(), err)
	}
	projection := plan.plan.Projection()
	var plannedText strings.Builder
	for _, run := range projection.GlyphRuns {
		plannedText.WriteString(run.Codes)
		plannedText.WriteByte('\n')
	}
	for _, text := range []string{"Overview", "Section paragraph", "1. Terms", "1. First item", "2. Second item", "Notice", "Nested note body", "2. After break", "Second page text"} {
		if !strings.Contains(plannedText.String(), text) {
			t.Fatalf("exact plan lacks text %q:\n%s", text, plannedText.String())
		}
	}
	var output bytes.Buffer
	if err := target.OutputWithOptions(&output, OutputOptions{Deterministic: true}); err != nil {
		t.Fatal(err)
	}
	if output.Len() == 0 || target.title != utf8toutf16("Typed exact plan") ||
		target.subject != utf8toutf16("Cutover") || target.author != utf8toutf16("GoPDFKit") ||
		target.compliance.Lang != "en-US" {
		t.Fatalf("painted plan metadata/output = %d bytes, title %q subject %q author %q lang %q",
			output.Len(), target.title, target.subject, target.author, target.compliance.Lang)
	}
}

func TestLayoutDocumentPlanUnsupportedContainerDiagnosticIsStableAndAtomic(t *testing.T) {
	source := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 180, Ht: 120}))
	doc := &layout.LayoutDocument{Body: []layout.Block{
		layout.SectionBlock{Blocks: []layout.Block{
			layout.NoteBoxBlock{Body: []layout.Block{layout.TableBlock{Caption: "not yet"}}},
		}},
	}}
	plan, err := source.PlanLayoutDocument(doc)
	want := "body[0].columns: at least one column is required"
	if !errors.Is(err, ErrLayoutDocumentPlanUnsupported) || !strings.Contains(err.Error(), want) || plan.Hash() != "" || source.PageCount() != 0 {
		t.Fatalf("unsupported = plan %#v, pages %d, %q; want %q", plan, source.PageCount(), err, want)
	}

	target := MustNew(WithUnit(UnitPoint))
	pages, paintErr := target.WriteLayoutDocumentPlan(plan)
	if paintErr == nil || pages != 0 || target.PageCount() != 0 || !strings.Contains(paintErr.Error(), "plan is empty") {
		t.Fatalf("zero plan paint = %d, pages %d, %v", pages, target.PageCount(), paintErr)
	}
}

func TestLayoutDocumentPlanRetainsCanonicalTypedTreeBeforePainting(t *testing.T) {
	planner := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 180, Ht: 120}))
	doc := &layout.LayoutDocument{Title: "Tree", Body: []layout.Block{layout.SectionBlock{Title: "Section", Blocks: []layout.Block{
		layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "body"}}, Style: layout.TextStyle{FontFamily: "Helvetica", FontSize: 9, LineHeight: 10}},
	}}}}
	plan, err := planner.PlanLayoutDocument(doc)
	if err != nil || plan.Hash() == "" || planner.PageCount() != 0 {
		t.Fatalf("typed tree plan = hash %q pages %d, %v", plan.Hash(), planner.PageCount(), err)
	}
	projection := plan.tree.Projection()
	if len(projection.Nodes) < 5 || len(projection.Children) == 0 || len(projection.Semantics) == 0 {
		t.Fatalf("typed canonical tree = nodes %d edges %d semantics %d", len(projection.Nodes), len(projection.Children), len(projection.Semantics))
	}
	doc.Title = "mutated"
	doc.Body = nil
	if len(plan.tree.Projection().Nodes) != len(projection.Nodes) {
		t.Fatal("typed plan tree aliases caller model")
	}
}

func TestLayoutDocumentPlanRejectsUnrepresentedContainerPolicies(t *testing.T) {
	tests := []struct {
		name string
		body layout.Block
		want string
	}{
		{"note box", layout.NoteBoxBlock{Box: layout.BoxStyle{Padding: layout.Spacing{Left: 2}}}, "body[0]: visual box styling is unsupported"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := MustNew(WithUnit(UnitPoint))
			_, err := source.PlanLayoutDocument(&layout.LayoutDocument{Body: []layout.Block{test.body}})
			if !errors.Is(err, ErrLayoutDocumentPlanUnsupported) || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want unsupported containing %q", err, test.want)
			}
		})
	}
}

func TestLayoutDocumentPlanReportsCustomBlockDeterministically(t *testing.T) {
	source := MustNew(WithUnit(UnitPoint))
	_, err := source.PlanLayoutDocument(&layout.LayoutDocument{Body: []layout.Block{
		layout.SectionBlock{Blocks: []layout.Block{typedPlanCustomBlock{}}},
	}})
	want := "document: layout document plan unsupported: block_kind: body[0].blocks[0] is custom-widget"
	if !errors.Is(err, ErrLayoutDocumentPlanUnsupported) || err.Error() != want {
		t.Fatalf("error = %q, want %q", err, want)
	}
}

func TestLayoutDocumentPlanMetadataAndSignaturePreserveOrderedSemanticsCaptureAndPDF(t *testing.T) {
	source := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 240, Ht: 220}), WithNoCompression())
	source.SetMargins(20, 20, 20)
	doc := &layout.LayoutDocument{
		Body: []layout.Block{layout.MetadataGridBlock{
			Columns: 1,
			Fields:  []layout.MetadataField{{Label: "Account", Value: "A-17"}, {Label: "Region", Value: "North"}},
			Style:   layout.TextStyle{FontFamily: "Helvetica", FontSize: 10, LineHeight: 11},
		}},
		Signature: &layout.SignatureBlock{
			PlaceholderReference: " ApprovalSignature ",
			Rows: []layout.SignatureRowBlock{{Columns: []layout.SignatureColumn{{
				Label: "Approved by", Name: "Ada Example", Role: "Director",
				Metadata: []layout.MetadataField{{Label: "Employee", Value: "42"}},
			}}}},
		},
	}
	plan, err := source.PlanLayoutDocument(doc)
	if err != nil || plan.PageCount() != 1 || plan.Hash() == "" {
		t.Fatalf("PlanLayoutDocument(metadata/signature) = %#v, %v", plan, err)
	}
	projection := plan.plan.Projection()
	actual := make([]string, 0)
	for _, node := range projection.SemanticNodes {
		if node.Role == "cell" {
			actual = append(actual, node.Attributes.ActualText)
		}
	}
	wantActual := []string{"Account: A-17", "Region: North", "Approved by; Ada Example; Director; Employee: 42"}
	if strings.Join(actual, "|") != strings.Join(wantActual, "|") {
		t.Fatalf("ordered cell semantics = %#v, want %#v", actual, wantActual)
	}

	capture, err := plan.CaptureDisplayPage(1)
	if err != nil || capture.PlanHash != plan.Hash() {
		t.Fatalf("typed display capture = hash %q, %v", capture.PlanHash, err)
	}
	if svg := capture.SVG(); !bytes.Contains(svg, []byte(">A</text>")) || !bytes.Contains(svg, []byte(">4</text>")) || !bytes.Contains(svg, []byte("<path")) {
		t.Fatalf("typed capture lacks metadata/signature display commands: %v\n%s", err, svg)
	}

	target := MustNew(WithUnit(UnitPoint), WithNoCompression(), WithDeterministicOutput())
	pages, err := target.WriteLayoutDocumentPlan(plan)
	if err != nil || pages != 1 || target.signatureFieldName != "ApprovalSignature" {
		t.Fatalf("WriteLayoutDocumentPlan() = pages %d field %q, %v", pages, target.signatureFieldName, err)
	}
	var pdf bytes.Buffer
	if err := target.OutputWithOptions(&pdf, OutputOptions{Deterministic: true}); err != nil || pdf.Len() == 0 {
		t.Fatalf("typed metadata/signature PDF = %d bytes, %v", pdf.Len(), err)
	}
}

func TestLayoutDocumentPlanImagesUseExactDisplayPlanCaptureAndResourceReuse(t *testing.T) {
	data, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==")
	if err != nil {
		t.Fatal(err)
	}
	source := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 180, Ht: 180}), WithNoCompression())
	source.SetMargins(20, 20, 20)
	doc := &layout.LayoutDocument{Body: []layout.Block{
		layout.ImageBlock{Data: data, Format: "png", Alt: "Red status pixel", Width: 60, Height: 30, Fit: layout.ImageFitContain, Align: "center",
			Caption: []layout.TextSegment{{Text: "Status image"}}},
		layout.ImageBlock{DataRef: &data, Format: "png", Width: 40, Height: 20, Fit: layout.ImageFitCover, Align: "right"},
	}}
	plan, err := source.PlanLayoutDocument(doc)
	if err != nil || plan.PageCount() != 1 || plan.Hash() == "" || source.PageCount() != 0 {
		t.Fatalf("PlanLayoutDocument(images) = %#v source pages %d, %v", plan, source.PageCount(), err)
	}
	projection := plan.plan.Projection()
	if len(projection.ImageResources) != 1 || len(projection.Images) != 2 || len(projection.Commands) < 3 {
		t.Fatalf("display image cardinality = resources %d images %d commands %d", len(projection.ImageResources), len(projection.Images), len(projection.Commands))
	}
	digest := sha256.Sum256(data)
	if got, want := string(projection.ImageResources[0].Digest), hex.EncodeToString(digest[:]); got != want ||
		projection.ImageResources[0].PixelWidth != 1 || projection.ImageResources[0].PixelHeight != 1 {
		t.Fatalf("image resource = %#v, want digest %s and 1x1", projection.ImageResources[0], want)
	}
	if projection.Images[0].Crop != nil || projection.Images[0].Bounds.Width != projection.Images[0].Bounds.Height || projection.Images[1].Crop == nil {
		t.Fatalf("contain/cover placements = %#v / %#v", projection.Images[0], projection.Images[1])
	}
	var figure, artifact bool
	for _, node := range projection.SemanticNodes {
		switch node.Role {
		case "figure":
			figure = node.Attributes.AlternateText == "Red status pixel"
		case "artifact":
			artifact = true
		}
	}
	if !figure || !artifact || len(projection.ReadingOrder) != len(projection.Fragments)-1 {
		t.Fatalf("image semantics = %#v reading=%d fragments=%d", projection.SemanticNodes, len(projection.ReadingOrder), len(projection.Fragments))
	}
	// Command order must retain image, caption glyphs, then decorative image.
	if projection.Commands[0].Kind != "image" || projection.Commands[len(projection.Commands)-1].Kind != "image" {
		t.Fatalf("display command order = %#v", projection.Commands)
	}

	capture, err := plan.Capture(PaperPlanCaptureRequest{
		Mode: "geometry_svg", IncludeContactSheet: true, ContactSheetColumns: 1,
		MaxPages: 1, MaxCrops: 4, MaxArtifactBytes: 1 << 20, MaxTotalBytes: 4 << 20, MaxManifestBytes: 1 << 20,
	})
	if err != nil || capture.PlanHash != plan.Hash() || len(capture.Artifacts) != 1 ||
		!bytes.Contains(capture.Artifacts[0].SVG, []byte("<image")) {
		t.Fatalf("typed image capture = artifacts %d hash %q, %v\n%s", len(capture.Artifacts), capture.PlanHash, err, firstArtifactSVG(capture.Artifacts))
	}
	again, err := plan.Capture(PaperPlanCaptureRequest{
		Mode: "geometry_svg", IncludeContactSheet: true, ContactSheetColumns: 1,
		MaxPages: 1, MaxCrops: 4, MaxArtifactBytes: 1 << 20, MaxTotalBytes: 4 << 20, MaxManifestBytes: 1 << 20,
	})
	if err != nil || len(again.Artifacts) != 1 || !bytes.Equal(capture.ManifestJSON, again.ManifestJSON) ||
		!bytes.Equal(capture.Artifacts[0].SVG, again.Artifacts[0].SVG) {
		t.Fatalf("typed image capture is not deterministic: %v", err)
	}
	display, err := plan.CaptureDisplayPage(1)
	displayAgain, againErr := plan.CaptureDisplayPage(1)
	if err != nil || againErr != nil || display.PlanHash != plan.Hash() || display.Page != 1 ||
		!bytes.Equal(display.SVG(), displayAgain.SVG()) ||
		!bytes.Contains(display.SVG(), []byte(`class="planned-image"`)) ||
		!bytes.Contains(display.SVG(), []byte("data:image/png;base64,")) {
		t.Fatalf("exact display capture = page %d hash %q, %v / %v\n%s", display.Page, display.PlanHash, err, againErr, display.SVG())
	}
	detached := display.SVG()
	detached[0] = 'x'
	if display.SVG()[0] == 'x' {
		t.Fatal("display capture SVG was not detached")
	}

	data[0] ^= 0xff // The immutable plan owns a detached source-byte snapshot.
	target := MustNew(WithUnit(UnitPoint), WithNoCompression(), WithDeterministicOutput())
	pages, err := target.WriteLayoutDocumentPlan(plan)
	if err != nil || pages != 1 || target.PageCount() != 1 {
		t.Fatalf("WriteLayoutDocumentPlan(images) = %d pages %d, %v", pages, target.PageCount(), err)
	}
	var pdf bytes.Buffer
	if err := target.OutputWithOptions(&pdf, OutputOptions{Deterministic: true}); err != nil ||
		target.resources == nil || len(target.resources.images) != 1 || bytes.Count(pdf.Bytes(), []byte("/Subtype /Image")) == 0 {
		t.Fatalf("image PDF resource reuse = resources %d image objects %d, %v", len(target.resources.images), bytes.Count(pdf.Bytes(), []byte("/Subtype /Image")), err)
	}
}

func TestLayoutDocumentPlanStyledImageBoxHasExactGeometryGraphicsSemanticsAndPDF(t *testing.T) {
	data, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==")
	red := layout.DocumentColor{R: 210, G: 20, B: 30, Set: true}
	blue := layout.DocumentColor{R: 10, G: 30, B: 220, Set: true}
	box := layout.BoxStyle{
		Padding:         layout.Spacing{Top: 2, Right: 4, Bottom: 8, Left: 6},
		BackgroundColor: red,
		Border: layout.BorderStyle{
			Top:    layout.BorderSide{Width: 1, Style: "solid", Color: blue},
			Right:  layout.BorderSide{Width: 1, Style: "solid", Color: blue},
			Bottom: layout.BorderSide{Width: 2, Style: "solid", Color: blue},
			Left:   layout.BorderSide{Width: 3, Style: "solid", Color: blue},
		},
	}
	planner := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 180, Ht: 120}), WithNoCompression(), WithDeterministicOutput())
	planner.SetMargins(20, 20, 20)
	plan, err := planner.PlanLayoutDocument(&layout.LayoutDocument{Body: []layout.Block{layout.ImageBlock{
		Data: data, Format: "png", Alt: "Decorated proof", Width: 60, Height: 30,
		Fit: layout.ImageFitContain, Align: "center", BoxRef: &box,
	}}})
	if err != nil || plan.PageCount() != 1 {
		t.Fatalf("PlanLayoutDocument(styled image) = pages %d, %v", plan.PageCount(), err)
	}
	projection := plan.plan.Projection()
	if len(projection.Fragments) != 1 || len(projection.Images) != 1 || len(projection.Paths) != 5 ||
		len(projection.Fills) != 1 || len(projection.Strokes) != 4 || len(projection.Commands) != 6 {
		t.Fatalf("styled image cardinality = fragments/images/paths/fills/strokes/commands %d/%d/%d/%d/%d/%d",
			len(projection.Fragments), len(projection.Images), len(projection.Paths), len(projection.Fills), len(projection.Strokes), len(projection.Commands))
	}
	fragment := projection.Fragments[0]
	if fragment.BorderBox.X.Points() != 53 || fragment.BorderBox.Y.Points() != 20 ||
		fragment.BorderBox.Width.Points() != 74 || fragment.BorderBox.Height.Points() != 43 ||
		fragment.ContentBox.X.Points() != 62 || fragment.ContentBox.Y.Points() != 23 ||
		fragment.ContentBox.Width.Points() != 60 || fragment.ContentBox.Height.Points() != 30 {
		t.Fatalf("styled image boxes = border %#v content %#v", fragment.BorderBox, fragment.ContentBox)
	}
	if projection.Commands[0].Kind != layoutengine.CommandFillPath || projection.Commands[1].Kind != layoutengine.CommandImage {
		t.Fatalf("styled image paint order = %#v", projection.Commands)
	}
	for _, command := range projection.Commands[2:] {
		if command.Kind != layoutengine.CommandStrokePath {
			t.Fatalf("styled image border command = %#v", command)
		}
	}
	if len(projection.SemanticNodes) < 2 || projection.SemanticNodes[1].Role != layoutengine.SemanticRoleFigure ||
		projection.SemanticNodes[1].Attributes.AlternateText != "Decorated proof" {
		t.Fatalf("styled image semantics = %#v", projection.SemanticNodes)
	}

	before := plan.Hash()
	box.Padding.Left = 99
	box.BackgroundColor = blue
	if plan.Hash() != before || plan.plan.Projection().Fragments[0].ContentBox != fragment.ContentBox {
		t.Fatal("styled image plan aliases BoxRef")
	}
	target := MustNew(WithUnit(UnitPoint), WithNoCompression(), WithDeterministicOutput())
	if pages, err := target.WriteLayoutDocumentPlan(plan); err != nil || pages != 1 {
		t.Fatalf("WriteLayoutDocumentPlan(styled image) = %d, %v", pages, err)
	}
	content := target.pages[1].Bytes()
	if !bytes.Contains(content, []byte(" rg")) || !bytes.Contains(content, []byte(" RG")) ||
		bytes.Count(content, []byte(" S")) != 4 || !bytes.Contains(content, []byte("/I")) {
		t.Fatalf("styled image PDF graphics are incomplete:\n%s", content)
	}
}

func TestLayoutDocumentPlanImageLimitsAndUnsupportedInputsAreAtomic(t *testing.T) {
	data, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==")
	doc := &layout.LayoutDocument{Body: []layout.Block{layout.ImageBlock{Data: data, Format: "png", Width: 20, Height: 20}}}
	limited := MustNew(WithUnit(UnitPoint), WithLimits(Limits{MaxImageSourceBytes: 8}))
	plan, err := limited.PlanLayoutDocument(doc)
	if err == nil || plan.Hash() != "" || limited.PageCount() != 0 {
		t.Fatalf("source-limit plan = %#v pages %d, %v", plan, limited.PageCount(), err)
	}

	source := MustNew(WithUnit(UnitPoint))
	plan, err = source.PlanLayoutDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	target := MustNew(WithUnit(UnitPoint), WithLimits(Limits{MaxImageSourceBytes: 8}))
	pages, err := target.WriteLayoutDocumentPlan(plan)
	if err == nil || pages != 0 || target.PageCount() != 0 {
		t.Fatalf("target-limit paint = pages %d target pages %d, %v", pages, target.PageCount(), err)
	}

	unsupported := []struct {
		name  string
		image layout.ImageBlock
		want  string
	}{
		{"ambient source", layout.ImageBlock{Source: "logo.png", Format: "png"}, "body[0].source"},
		{"format", layout.ImageBlock{Data: data, Format: "gif"}, "body[0].format"},
		{"alignment", layout.ImageBlock{Data: data, Format: "png", Align: "justify"}, "body[0].align"},
		{"dpi", layout.ImageBlock{Data: data, Format: "png", DPI: 144}, "body[0].dpi"},
		{"margin", layout.ImageBlock{Data: data, Format: "png", Box: layout.BoxStyle{Margin: layout.Spacing{Left: 1}}}, "body[0].box.margin"},
		{"border style", layout.ImageBlock{Data: data, Format: "png", Box: layout.BoxStyle{Border: layout.BorderStyle{Top: layout.BorderSide{Width: 1, Style: "dashed"}}}}, "body[0].box.border"},
		{"background color", layout.ImageBlock{Data: data, Format: "png", Box: layout.BoxStyle{BackgroundColor: layout.DocumentColor{R: 999, Set: true}}}, "body[0].box.background"},
	}
	for _, test := range unsupported {
		t.Run(test.name, func(t *testing.T) {
			planner := MustNew(WithUnit(UnitPoint))
			_, err := planner.PlanLayoutDocument(&layout.LayoutDocument{Body: []layout.Block{test.image}})
			if !errors.Is(err, ErrLayoutDocumentPlanUnsupported) || !strings.Contains(err.Error(), test.want) || planner.PageCount() != 0 {
				t.Fatalf("unsupported image = pages %d, %v, want %q", planner.PageCount(), err, test.want)
			}
		})
	}
}

func TestLayoutDocumentPlanResourceResolutionContextAndCumulativeBounds(t *testing.T) {
	encode := func(c color.NRGBA) []byte {
		value := image.NewNRGBA(image.Rect(0, 0, 2, 2))
		for y := 0; y < 2; y++ {
			for x := 0; x < 2; x++ {
				value.SetNRGBA(x, y, c)
			}
		}
		var out bytes.Buffer
		if err := png.Encode(&out, value); err != nil {
			t.Fatal(err)
		}
		return out.Bytes()
	}
	first, second := encode(color.NRGBA{R: 255, A: 255}), encode(color.NRGBA{B: 255, A: 255})
	source := MustNew(WithUnit(UnitPoint))
	plan, err := source.PlanLayoutDocument(&layout.LayoutDocument{Body: []layout.Block{layout.ImageBlock{Data: first, Format: "png", Width: 10, Height: 10}, layout.ImageBlock{Data: second, Format: "png", Width: 10, Height: 10}}})
	if err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if capture, err := plan.CaptureDisplayPageContext(canceled, 1); !errors.Is(err, context.Canceled) || len(capture.SVG()) != 0 {
		t.Fatalf("canceled display capture=%d err=%v", len(capture.SVG()), err)
	}
	target := MustNew(WithUnit(UnitPoint))
	if pages, err := target.WriteLayoutDocumentPlanContext(canceled, plan); !errors.Is(err, context.Canceled) || pages != 0 || target.PageCount() != 0 {
		t.Fatalf("canceled write pages=%d target=%d err=%v", pages, target.PageCount(), err)
	}
	limit := max(len(first), len(second)) + 1
	if limit >= len(first)+len(second) {
		t.Fatalf("fixture cannot distinguish cumulative limit")
	}
	bounded := MustNew(WithUnit(UnitPoint), WithLimits(Limits{MaxImageSourceBytes: int64(limit)}))
	beforeResources := bounded.resources
	pages, err := bounded.WriteLayoutDocumentPlan(plan)
	if err == nil || pages != 0 || bounded.PageCount() != 0 || bounded.resources != beforeResources || !strings.Contains(err.Error(), "cumulative planned image source") {
		t.Fatalf("cumulative write pages=%d target=%d err=%v", pages, bounded.PageCount(), err)
	}
	want, err := plan.CaptureDisplayPage(1)
	if err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	failures := make(chan error, 8)
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			got, err := plan.CaptureDisplayPageContext(context.Background(), 1)
			if err != nil {
				failures <- err
				return
			}
			if !bytes.Equal(got.SVG(), want.SVG()) {
				failures <- errors.New("concurrent preview differs")
			}
		}()
	}
	group.Wait()
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
}

func firstArtifactSVG(artifacts []PaperPlanArtifact) []byte {
	if len(artifacts) == 0 {
		return nil
	}
	return artifacts[0].SVG
}

type typedPlanCustomBlock struct{}

func (typedPlanCustomBlock) DocumentBlockKind() layout.BlockKind { return "custom-widget" }
