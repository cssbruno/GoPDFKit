// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os/exec"
	"strings"
	"testing"

	"github.com/cssbruno/paperrune/document"
	"github.com/cssbruno/paperrune/internal/pdfverify"
	"golang.org/x/image/font/gofont/goregular"
)

type finalRasterizerFunc func(context.Context, []byte, uint32, []image.Point, pdfverify.Limits) (pdfverify.RasterOutput, error)

func (function finalRasterizerFunc) Rasterize(ctx context.Context, pdf []byte, dpi uint32, dimensions []image.Point, limits pdfverify.Limits) (pdfverify.RasterOutput, error) {
	return function(ctx, pdf, dpi, dimensions, limits)
}

func TestVerifyFinalPlanPDFUsesSameRetainedPlanAndReturnsDetachedEvidence(t *testing.T) {
	workspace, created, opened := sensitiveFixture(t, WorkspaceOptions{PolicyRevision: "policy-v7"})
	plan, planned, err := workspace.CreatePlan(created.Revision.Handle)
	if err != nil {
		t.Fatal(err)
	}
	if !planned.OK() {
		t.Fatalf("plan result = %#v", planned)
	}
	raster := document.DefaultPaperPlanRasterRequest()
	raster.CoreFontProgram = goregular.TTF
	raster.MaxSourceBytes, raster.MaxPNGBytes = uint64(workspace.limits.MaxRenderBytes), uint64(workspace.limits.MaxRenderBytes)
	preview, err := workspace.plans[plan.Handle.value.serial].plan.CaptureRasterPages(context.Background(), raster)
	if err != nil {
		t.Fatal(err)
	}
	actualPages := make([][]byte, len(preview.Pages))
	for index := range preview.Pages {
		actualPages[index] = append([]byte(nil), preview.Pages[index].PNG...)
	}
	verifier := finalRasterizerFunc(func(_ context.Context, pdf []byte, dpi uint32, dimensions []image.Point, limits pdfverify.Limits) (pdfverify.RasterOutput, error) {
		if len(pdf) == 0 || dpi != raster.DPI || len(dimensions) != plan.Pages || limits.MaxPDFBytes > uint64(workspace.limits.MaxRenderBytes) {
			t.Fatalf("verification input mismatch")
		}
		return pdfverify.RasterOutput{Renderer: "test-final-pdf-consumer", Version: "1.0", Pages: actualPages}, nil
	})
	request := FinalPDFVerificationRequest{Plan: plan.Handle, Raster: raster}
	authorizeFinalVerification(t, workspace, created, opened, &request, "final-verification-0001")
	result, err := workspace.VerifyFinalPlanPDF(context.Background(), request, verifier)
	if err != nil || !result.Report.Passed || result.Report.PlanHash != plan.Hash || result.Report.PageCount != uint32(plan.Pages) || len(result.ReportJSON) == 0 {
		t.Fatalf("VerifyFinalPlanPDF = %#v, %v", result, err)
	}
	result.ReportJSON[0] ^= 0xff
	authorizeFinalVerification(t, workspace, created, opened, &request, "final-verification-0002")
	again, err := workspace.VerifyFinalPlanPDF(context.Background(), request, verifier)
	if err != nil || len(again.ReportJSON) == 0 || again.ReportJSON[0] == result.ReportJSON[0] {
		t.Fatal("verification report aliases prior result")
	}
}

func solidPNG(t *testing.T, width, height int, black bool) []byte {
	t.Helper()
	canvas := image.NewNRGBA(image.Rect(0, 0, width, height))
	value := color.White
	if black {
		value = color.Black
	}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			canvas.Set(x, y, value)
		}
	}
	var output bytes.Buffer
	if err := png.Encode(&output, canvas); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func TestVerifyFinalPlanPDFReturnsFailureReportForPixelMismatch(t *testing.T) {
	workspace, created, opened := sensitiveFixture(t, WorkspaceOptions{PolicyRevision: "policy-v7"})
	plan, planned, err := workspace.CreatePlan(created.Revision.Handle)
	if err != nil {
		t.Fatal(err)
	}
	if !planned.OK() {
		t.Fatalf("plan result = %#v", planned)
	}
	raster := document.DefaultPaperPlanRasterRequest()
	raster.CoreFontProgram = goregular.TTF
	raster.MaxSourceBytes, raster.MaxPNGBytes = uint64(workspace.limits.MaxRenderBytes), uint64(workspace.limits.MaxRenderBytes)
	verifier := finalRasterizerFunc(func(_ context.Context, _ []byte, _ uint32, dimensions []image.Point, _ pdfverify.Limits) (pdfverify.RasterOutput, error) {
		pages := make([][]byte, len(dimensions))
		for index, dimension := range dimensions {
			pages[index] = solidPNG(t, dimension.X, dimension.Y, true)
		}
		return pdfverify.RasterOutput{Renderer: "wrong-consumer", Version: "1", Pages: pages}, nil
	})
	request := FinalPDFVerificationRequest{Plan: plan.Handle, Raster: raster}
	authorizeFinalVerification(t, workspace, created, opened, &request, "final-verification-fail")
	result, err := workspace.VerifyFinalPlanPDF(context.Background(), request, verifier)
	if errorCode(err) != "FINAL_PDF_VERIFICATION_FAILED" || result.Report.Passed || len(result.Report.Failures) == 0 || len(result.ReportJSON) == 0 {
		t.Fatalf("mismatch result = %#v, %v", result, err)
	}
}

func TestVerifyFinalPlanPDFWithPinnedPopplerConsumer(t *testing.T) {
	binary, err := exec.LookPath("pdftoppm")
	if err != nil {
		t.Skip("pdftoppm is not installed")
	}
	workspace, created, opened := sensitiveFixture(t, WorkspaceOptions{PolicyRevision: "policy-v7"})
	plan, planned, err := workspace.CreatePlan(created.Revision.Handle)
	if err != nil || !planned.OK() {
		t.Fatalf("CreatePlan = %#v, %v", planned, err)
	}
	raster := document.DefaultPaperPlanRasterRequest()
	raster.CoreFontProgram = goregular.TTF
	raster.MaxSourceBytes, raster.MaxPNGBytes = uint64(workspace.limits.MaxRenderBytes), uint64(workspace.limits.MaxRenderBytes)
	request := FinalPDFVerificationRequest{Plan: plan.Handle, Raster: raster, Tolerance: pdfverify.RasterTolerance{MaxChangedPixelsPPM: 1_000, MaxChannelDelta: 255, MaxMeanChannelDeltaMilli: 100}}
	authorizeFinalVerification(t, workspace, created, opened, &request, "poppler-final-verify")
	result, err := workspace.VerifyFinalPlanPDF(context.Background(), request, pdfverify.PopplerRasterizer{Binary: binary, Version: "26.05.0", TempRoot: t.TempDir()})
	if err != nil || !result.Report.Passed || result.Report.Renderer != "poppler/pdftoppm" || len(result.Report.Pages) != plan.Pages || result.ExecutionAuditHash == "" {
		t.Fatalf("Poppler final verification = %#v, %v", result, err)
	}
	for _, page := range result.Report.Pages {
		if page.ChangedPixelsPPM > request.Tolerance.MaxChangedPixelsPPM || page.MaximumChannelDelta > request.Tolerance.MaxChannelDelta || page.MeanChannelDeltaMilli > request.Tolerance.MaxMeanChannelDeltaMilli {
			t.Fatalf("page exceeds pinned Poppler tolerance: %#v", page)
		}
	}
}

func TestFinalPDFVerificationInputHashIsCanonicalAndResourceCausal(t *testing.T) {
	workspace, created, _ := sensitiveFixture(t, WorkspaceOptions{PolicyRevision: "policy-v7"})
	plan, planned, err := workspace.CreatePlan(created.Revision.Handle)
	if err != nil || !planned.OK() {
		t.Fatalf("CreatePlan = %#v, %v", planned, err)
	}
	raster := document.DefaultPaperPlanRasterRequest()
	raster.MaxSourceBytes, raster.MaxPNGBytes = uint64(workspace.limits.MaxRenderBytes), uint64(workspace.limits.MaxRenderBytes)
	raster.FontPrograms = map[string][]byte{"b": []byte("font-b"), "a": []byte("font-a")}
	raster.Images = map[string][]byte{"z": []byte("image-z")}
	request := FinalPDFVerificationRequest{Plan: plan.Handle, Raster: raster, RequiredCompliance: []string{"pdfua", "layout"}, Compliance: []pdfverify.ComplianceEvidence{
		{Profile: "pdfua", Tool: "validator", Version: "2", PDFSHA256: strings.Repeat("a", 64), ReportHash: strings.Repeat("b", 64), Passed: true},
		{Profile: "layout", Tool: "validator", Version: "1", PDFSHA256: strings.Repeat("a", 64), ReportHash: strings.Repeat("c", 64), Passed: true},
	}}
	first, err := workspace.FinalPDFVerificationInputHash(request)
	if err != nil {
		t.Fatal(err)
	}
	request.RequiredCompliance[0], request.RequiredCompliance[1] = request.RequiredCompliance[1], request.RequiredCompliance[0]
	request.Compliance[0], request.Compliance[1] = request.Compliance[1], request.Compliance[0]
	request.Raster.FontPrograms = map[string][]byte{"a": []byte("font-a"), "b": []byte("font-b")}
	second, err := workspace.FinalPDFVerificationInputHash(request)
	if err != nil || first != second {
		t.Fatalf("canonical hashes = %q / %q, %v", first, second, err)
	}
	request.Raster.FontPrograms["a"][0] = 'X'
	changed, err := workspace.FinalPDFVerificationInputHash(request)
	if err != nil || changed == first {
		t.Fatalf("resource-causal hash = %q, %v", changed, err)
	}
}

func authorizeFinalVerification(t *testing.T, workspace *Workspace, created PaperCreateResult, opened PaperOpenSnapshot, request *FinalPDFVerificationRequest, nonce string) {
	t.Helper()
	authority := grantSensitiveForTest(t, workspace, opened, SensitiveProductionCapture)
	inputHash, err := workspace.FinalPDFVerificationInputHash(*request)
	if err != nil {
		t.Fatal(err)
	}
	evidence := completeSensitiveEvidence(string(created.Revision.Revision))
	evidence.OperationInputHash = inputHash
	approval := grantApprovalForTest(t, workspace, created, authority, evidence, nonce+"-approval", workspace.handleTTL)
	request.Authorization = SensitiveOperationRequest{Authority: authority.Handle, Approval: approval.Handle, Operation: SensitiveProductionCapture,
		ExpectedHead: created.Revision.Handle, PolicyRevision: "policy-v7", Evidence: evidence}
}
