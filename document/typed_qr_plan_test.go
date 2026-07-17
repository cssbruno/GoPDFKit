// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
	"github.com/cssbruno/gopdfkit/layout"
	"golang.org/x/image/font/gofont/goregular"
)

func TestLayoutDocumentPlanLowersQRToExactImageTextLinkSemanticsAndPDF(t *testing.T) {
	doc := &layout.LayoutDocument{
		Language: "en",
		Body: []layout.Block{
			layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "Before verification"}}},
			layout.QRVerificationBlock{QR: layout.QRBlock{
				URL: "https://example.test/verify/42", Label: "Verify record", Size: 28,
				Align: "center", KeepTogether: true,
			}},
		},
	}
	planner := MustNew(WithUnit(UnitPoint), WithNoCompression(), WithDeterministicOutput())
	plan, err := planner.PlanLayoutDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.plan.Projection()
	if len(projection.ImageResources) != 1 || len(projection.Images) != 1 || len(projection.Links) != 1 {
		t.Fatalf("QR resources/images/links = %d/%d/%d, want 1/1/1", len(projection.ImageResources), len(projection.Images), len(projection.Links))
	}
	if projection.Links[0].URI != "https://example.test/verify/42" || projection.Images[0].Bounds.Width != projection.Images[0].Bounds.Height {
		t.Fatalf("QR link/image = %#v / %#v", projection.Links[0], projection.Images[0])
	}
	var figure, linkedText bool
	for _, node := range projection.SemanticNodes {
		if node.Role == layoutengine.SemanticRoleFigure && node.Attributes.AlternateText == "Verify record QR code" {
			figure = true
		}
		if node.Role == layoutengine.SemanticRoleParagraph && strings.Contains(node.Attributes.ActualText, "https://example.test/verify/42") {
			linkedText = true
		}
	}
	if !figure || !linkedText {
		t.Fatalf("QR semantic figure/text = %t/%t; nodes %#v", figure, linkedText, projection.SemanticNodes)
	}
	capture, err := plan.CaptureDisplayPage(1)
	if err != nil || !bytes.Contains(capture.SVG(), []byte("<image")) || !bytes.Contains(capture.SVG(), []byte("data:image/png;base64,")) {
		t.Fatalf("QR display capture = %v, %s", err, capture.SVG())
	}
	rasterRequest := DefaultPaperPlanRasterRequest()
	rasterRequest.PageProfile = strings.Repeat("a", 64)
	rasterRequest.CoreFontProgram = goregular.TTF
	rasterRequest.Images = make(map[string][]byte, len(plan.imageSources))
	for digest, encoded := range plan.imageSources {
		rasterRequest.Images[string(digest)] = encoded
	}
	raster, err := (PaperPlan{plan: plan.plan, hash: plan.hash, pages: plan.pages}).CaptureRasterPages(context.Background(), rasterRequest)
	if err != nil || len(raster.Pages) != plan.PageCount() || len(raster.Pages[0].PNG) == 0 {
		t.Fatalf("QR raster capture = pages %d, %v", len(raster.Pages), err)
	}

	target := MustNew(WithUnit(UnitPoint), WithNoCompression(), WithDeterministicOutput())
	pages, err := target.WriteLayoutDocumentPlan(plan)
	if err != nil || pages != plan.PageCount() {
		t.Fatalf("WriteLayoutDocumentPlan(QR) = pages %d, %v", pages, err)
	}
	var pdf bytes.Buffer
	if err := target.OutputWithOptions(&pdf, OutputOptions{Deterministic: true}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(pdf.Bytes(), []byte("/Subtype /Image")) ||
		!bytes.Contains(pdf.Bytes(), []byte("/URI (https://example.test/verify/42)")) {
		t.Fatalf("QR PDF lacks image or exact URI annotation")
	}

	again, err := MustNew(WithUnit(UnitPoint)).PlanLayoutDocument(doc)
	if err != nil || again.Hash() != plan.Hash() {
		t.Fatalf("deterministic QR plan = %q, %v; want %q", again.Hash(), err, plan.Hash())
	}
}

func TestLayoutDocumentPlanQRIsDetachedAndReusableAcrossConcurrentWriters(t *testing.T) {
	model := &layout.LayoutDocument{Body: []layout.Block{layout.QRVerificationBlock{QR: layout.QRBlock{
		URL: "https://example.test/original", Label: "Original", Size: 24, Align: "right",
	}}}}
	plan, err := MustNew(WithUnit(UnitPoint), WithNoCompression(), WithDeterministicOutput()).PlanLayoutDocument(model)
	if err != nil {
		t.Fatal(err)
	}
	mutated := model.Body[0].(layout.QRVerificationBlock)
	mutated.QR.URL = "https://example.test/mutated"
	model.Body[0] = mutated
	const writers = 6
	errs := make(chan error, writers)
	var group sync.WaitGroup
	for index := 0; index < writers; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			target := MustNew(WithUnit(UnitPoint), WithNoCompression(), WithDeterministicOutput())
			if _, writeErr := target.WriteLayoutDocumentPlan(plan); writeErr != nil {
				errs <- writeErr
				return
			}
			var output bytes.Buffer
			if outputErr := target.OutputWithOptions(&output, OutputOptions{Deterministic: true}); outputErr != nil ||
				!bytes.Contains(output.Bytes(), []byte("https://example.test/original")) ||
				bytes.Contains(output.Bytes(), []byte("https://example.test/mutated")) {
				errs <- errors.New("concurrent QR writer observed mutable source state")
			}
		}()
	}
	group.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestLayoutDocumentPlanAppendsStandaloneQRAfterBodyAndSignature(t *testing.T) {
	doc := &layout.LayoutDocument{
		Body:      []layout.Block{layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "Body"}}}},
		Signature: &layout.SignatureBlock{Rows: []layout.SignatureRowBlock{{Columns: []layout.SignatureColumn{{Name: "Signer"}}}}},
		QR:        &layout.QRBlock{Value: "opaque-verification-value", Label: "Check", Size: 24},
	}
	plan, err := MustNew(WithUnit(UnitPoint)).PlanLayoutDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.plan.Projection()
	if len(projection.Images) != 1 {
		t.Fatalf("standalone QR images = %d, want 1", len(projection.Images))
	}
	var actual []string
	for _, node := range projection.SemanticNodes {
		if node.Attributes.ActualText != "" {
			actual = append(actual, node.Attributes.ActualText)
		}
	}
	joined := strings.Join(actual, "|")
	if body, signer, qr := strings.Index(joined, "Body"), strings.Index(joined, "Signer"), strings.Index(joined, "opaque-verification-value"); body < 0 || signer <= body || qr <= signer {
		t.Fatalf("standalone QR semantic order = %q", joined)
	}
}

func TestLayoutDocumentPlanQRAlignmentMatchesLegacySideBySideGeometry(t *testing.T) {
	for _, test := range []struct {
		align       string
		wantImageX  float64
		wantTextX   float64
		wantTextWid float64
	}{
		{align: "left", wantImageX: 10, wantTextX: 38, wantTextWid: 92},
		{align: "right", wantImageX: 106, wantTextX: 10, wantTextWid: 92},
	} {
		t.Run(test.align, func(t *testing.T) {
			planner := paginationTestDocument(t, 100)
			plan, err := planner.PlanLayoutDocument(&layout.LayoutDocument{Body: []layout.Block{
				layout.QRVerificationBlock{QR: layout.QRBlock{URL: "https://example.test/" + test.align, Size: 24, Align: test.align}},
			}})
			if err != nil {
				t.Fatal(err)
			}
			projection := plan.plan.Projection()
			if len(projection.Images) != 1 || len(projection.Links) == 0 {
				t.Fatalf("image/link count = %d/%d", len(projection.Images), len(projection.Links))
			}
			image := projection.Images[0].Bounds
			var text layoutengine.Rect
			for _, association := range projection.SemanticFragments {
				node := projection.SemanticNodes[association.Semantic-1]
				if node.Role == layoutengine.SemanticRoleParagraph {
					text = projection.Fragments[association.Fragment-1].BorderBox
				}
			}
			fixed := func(points float64) layoutengine.Fixed {
				value, _ := layoutengine.FixedFromPoints(points)
				return value
			}
			if image.X != fixed(test.wantImageX) || image.Width != fixed(24) || image.Height != fixed(24) ||
				text.X != fixed(test.wantTextX) || text.Width != fixed(test.wantTextWid) || text.Y != image.Y {
				t.Fatalf("%s geometry image=%+v text=%+v", test.align, image, text)
			}
		})
	}

	planner := paginationTestDocument(t, 120)
	center, err := planner.PlanLayoutDocument(&layout.LayoutDocument{Body: []layout.Block{
		layout.QRVerificationBlock{QR: layout.QRBlock{Value: "center", Size: 24, Align: "center"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	projection := center.plan.Projection()
	image := projection.Images[0].Bounds
	var text layoutengine.Rect
	for _, association := range projection.SemanticFragments {
		if projection.SemanticNodes[association.Semantic-1].Role == layoutengine.SemanticRoleParagraph {
			text = projection.Fragments[association.Fragment-1].BorderBox
		}
	}
	wantX, _ := layoutengine.FixedFromPoints(58)
	wantTextY, _ := image.Bottom()
	two, _ := layoutengine.FixedFromPoints(2)
	wantTextY, _ = wantTextY.Add(two)
	if image.X != wantX || text.Y != wantTextY {
		t.Fatalf("center geometry image=%+v text=%+v", image, text)
	}
}

func TestLayoutDocumentPlanRejectsInvalidQRAtomicallyAndHonorsCancellation(t *testing.T) {
	tests := []struct {
		name string
		qr   layout.QRBlock
		want string
	}{
		{name: "empty", qr: layout.QRBlock{}, want: "value or URL is required"},
		{name: "size", qr: layout.QRBlock{Value: "x", Size: -1}, want: "size"},
		{name: "align", qr: layout.QRBlock{Value: "x", Align: "justify"}, want: "align"},
		{name: "payload limit", qr: layout.QRBlock{Value: strings.Repeat("x", typedQRPayloadByteLimit+1)}, want: "payload exceeds"},
		{name: "unsafe URL", qr: layout.QRBlock{URL: "javascript:alert(1)"}, want: "unsupported"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			planner := MustNew(WithUnit(UnitPoint))
			plan, err := planner.PlanLayoutDocument(&layout.LayoutDocument{Body: []layout.Block{layout.QRVerificationBlock{QR: test.qr}}})
			if err == nil || !strings.Contains(err.Error(), test.want) || plan.Hash() != "" || planner.PageCount() != 0 {
				t.Fatalf("invalid QR = plan %q pages %d, %v; want %q", plan.Hash(), planner.PageCount(), err, test.want)
			}
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	planner := MustNew(WithUnit(UnitPoint))
	plan, err := planner.PlanLayoutDocumentContext(ctx, &layout.LayoutDocument{Body: []layout.Block{
		layout.QRVerificationBlock{QR: layout.QRBlock{Value: "cancelled"}},
	}})
	if !errors.Is(err, context.Canceled) || plan.Hash() != "" || planner.PageCount() != 0 {
		t.Fatalf("cancelled QR = plan %q pages %d, %v", plan.Hash(), planner.PageCount(), err)
	}
}
