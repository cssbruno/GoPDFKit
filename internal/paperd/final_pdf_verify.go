// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package paperd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"

	"github.com/cssbruno/gopdfkit/document"
	"github.com/cssbruno/gopdfkit/internal/pdfverify"
)

type FinalPDFVerificationRequest struct {
	Plan               PlanHandle
	Authorization      SensitiveOperationRequest
	Raster             document.PaperPlanRasterRequest
	Tolerance          pdfverify.RasterTolerance
	Structure          pdfverify.StructuralExpectation
	RequiredCompliance []string
	Compliance         []pdfverify.ComplianceEvidence
	Limits             pdfverify.Limits
}

type FinalPDFVerificationResult struct {
	Plan               PlanSnapshot              `json:"plan"`
	Authorization      SensitiveOperationReceipt `json:"authorization"`
	ExecutionAuditHash string                    `json:"execution_audit_hash,omitempty"`
	Report             pdfverify.Report          `json:"report"`
	ReportJSON         []byte                    `json:"-"`
}

type finalPDFResourceDigest struct{ Kind, ID, SHA256 string }
type finalPDFVerificationBinding struct {
	PlanHash string
	Raster   struct {
		PageProfile                            string
		DPI                                    uint32
		MaxPixels, MaxSourceBytes, MaxPNGBytes uint64
		CoreFontSHA256                         string
		Resources                              []finalPDFResourceDigest
	}
	Tolerance          pdfverify.RasterTolerance
	Structure          pdfverify.StructuralExpectation
	RequiredCompliance []string
	Compliance         []pdfverify.ComplianceEvidence
	Limits             pdfverify.Limits
}

// FinalPDFVerificationInputHash returns the exact production-capture input
// hash that approval evidence must bind. Raw fonts, images, PDF bytes, targets,
// and handles never enter approval or audit state.
func (w *Workspace) FinalPDFVerificationInputHash(request FinalPDFVerificationRequest) (string, error) {
	if w == nil {
		return "", workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	record, raster, limits, err := w.normalizeFinalPDFVerification(request)
	if err != nil {
		return "", err
	}
	binding := finalPDFVerificationBinding{PlanHash: record.plan.Hash(), Tolerance: request.Tolerance, Structure: request.Structure,
		RequiredCompliance: append([]string(nil), request.RequiredCompliance...), Compliance: append([]pdfverify.ComplianceEvidence(nil), request.Compliance...), Limits: limits}
	binding.Structure.Pages = uint32(record.plan.PageCount())
	binding.Raster.PageProfile, binding.Raster.DPI, binding.Raster.MaxPixels, binding.Raster.MaxSourceBytes, binding.Raster.MaxPNGBytes = raster.PageProfile, raster.DPI, raster.MaxPixels, raster.MaxSourceBytes, raster.MaxPNGBytes
	binding.Raster.CoreFontSHA256 = digestBytes(raster.CoreFontProgram)
	for id, value := range raster.FontPrograms {
		binding.Raster.Resources = append(binding.Raster.Resources, finalPDFResourceDigest{Kind: "font", ID: id, SHA256: digestBytes(value)})
	}
	for id, value := range raster.Images {
		binding.Raster.Resources = append(binding.Raster.Resources, finalPDFResourceDigest{Kind: "image", ID: id, SHA256: digestBytes(value)})
	}
	sort.Slice(binding.Raster.Resources, func(i, j int) bool {
		if binding.Raster.Resources[i].Kind != binding.Raster.Resources[j].Kind {
			return binding.Raster.Resources[i].Kind < binding.Raster.Resources[j].Kind
		}
		return binding.Raster.Resources[i].ID < binding.Raster.Resources[j].ID
	})
	sort.Strings(binding.RequiredCompliance)
	sort.Slice(binding.Compliance, func(i, j int) bool {
		left, right := binding.Compliance[i], binding.Compliance[j]
		if left.Profile != right.Profile {
			return left.Profile < right.Profile
		}
		if left.Tool != right.Tool {
			return left.Tool < right.Tool
		}
		if left.Version != right.Version {
			return left.Version < right.Version
		}
		if left.PDFSHA256 != right.PDFSHA256 {
			return left.PDFSHA256 < right.PDFSHA256
		}
		if left.ReportHash != right.ReportHash {
			return left.ReportHash < right.ReportHash
		}
		return !left.Passed && right.Passed
	})
	payload, err := json.Marshal(binding)
	if err != nil {
		return "", err
	}
	return SensitiveOperationInputHash(SensitiveProductionCapture, SensitiveOperationInput{Target: "paperd:final-pdf-verification:" + record.plan.Hash(), MediaType: "application/vnd.gopdfkit.final-pdf-verification+json", Payload: payload})
}

// VerifyFinalPlanPDF independently verifies PDF bytes painted from one exact
// retained plan against direct display-list pages from that same plan. The
// external renderer is explicit and versioned; no browser layout or ambient
// screenshot is accepted as final-PDF evidence.
func (w *Workspace) VerifyFinalPlanPDF(ctx context.Context, request FinalPDFVerificationRequest, rasterizer pdfverify.Rasterizer) (FinalPDFVerificationResult, error) {
	if w == nil {
		return FinalPDFVerificationResult{}, workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	if ctx == nil || rasterizer == nil {
		return FinalPDFVerificationResult{}, workspaceError("INVALID_FINAL_PDF_VERIFICATION", "context and rasterizer are required", ErrInvalidQuery)
	}
	record, rasterRequest, limits, err := w.normalizeFinalPDFVerification(request)
	if err != nil {
		return FinalPDFVerificationResult{}, err
	}
	request.Raster = rasterRequest
	request.Structure.Pages = uint32(record.plan.PageCount())
	inputHash, err := w.FinalPDFVerificationInputHash(request)
	if err != nil {
		return FinalPDFVerificationResult{}, err
	}
	if request.Authorization.Operation != SensitiveProductionCapture || request.Authorization.Evidence.OperationInputHash != inputHash {
		w.auditSensitiveExecutionDenial(request.Authorization, "SENSITIVE_INPUT_BINDING_MISMATCH")
		return FinalPDFVerificationResult{}, workspaceError("SENSITIVE_INPUT_BINDING_MISMATCH", "final PDF verification is not bound to exact production-capture approval", ErrSensitiveOperationDenied)
	}
	authorization, err := w.AuthorizeSensitiveOperation(request.Authorization)
	if err != nil {
		return FinalPDFVerificationResult{}, err
	}
	result := FinalPDFVerificationResult{Plan: snapshotPlan(record), Authorization: authorization}
	preview, err := record.plan.CaptureRasterPages(ctx, request.Raster)
	if err != nil {
		result.ExecutionAuditHash = w.auditSensitiveExecutionOutcome(request.Authorization, authorization, false, "FINAL_PDF_PLAN_RASTER")
		return result, workspaceError("FINAL_PDF_PLAN_RASTER", "direct retained-plan raster failed", err)
	}
	rendered, err := w.RenderPlan(request.Plan)
	if err != nil {
		result.ExecutionAuditHash = w.auditSensitiveExecutionOutcome(request.Authorization, authorization, false, "FINAL_PDF_RENDER_FAILED")
		return result, err
	}
	expected := make([]pdfverify.ExpectedRasterPage, len(preview.Pages))
	for index, page := range preview.Pages {
		expected[index] = pdfverify.ExpectedRasterPage{Page: page.Page, PNG: append([]byte(nil), page.PNG...), PlanRasterManifest: page.ManifestSHA256}
	}
	report, verifyErr := pdfverify.Verify(ctx, pdfverify.Request{PDF: rendered.PDF, PlanHash: preview.PlanHash, DPI: request.Raster.DPI,
		ExpectedPages: expected, Tolerance: request.Tolerance, Structure: request.Structure,
		RequiredCompliance: append([]string(nil), request.RequiredCompliance...), Compliance: append([]pdfverify.ComplianceEvidence(nil), request.Compliance...), Limits: limits}, rasterizer)
	result.Report = report
	if report.Version != 0 {
		encoded, encodeErr := report.CanonicalJSON(limits.MaxJSONBytes)
		if encodeErr != nil {
			result.ExecutionAuditHash = w.auditSensitiveExecutionOutcome(request.Authorization, authorization, false, "FINAL_PDF_REPORT_FAILED")
			return result, encodeErr
		}
		result.ReportJSON = append([]byte(nil), encoded...)
	}
	if verifyErr != nil {
		result.ExecutionAuditHash = w.auditSensitiveExecutionOutcome(request.Authorization, authorization, false, "FINAL_PDF_VERIFICATION_FAILED")
		if errors.Is(verifyErr, pdfverify.ErrVerificationFailed) {
			return result, workspaceError("FINAL_PDF_VERIFICATION_FAILED", "serialized PDF did not satisfy exact verification evidence", verifyErr)
		}
		return result, verifyErr
	}
	result.ExecutionAuditHash = w.auditSensitiveExecutionOutcome(request.Authorization, authorization, true, "final PDF verification completed")
	return result, nil
}

func (w *Workspace) normalizeFinalPDFVerification(request FinalPDFVerificationRequest) (*planRecord, document.PaperPlanRasterRequest, pdfverify.Limits, error) {
	record, err := w.plan(request.Plan)
	if err != nil {
		return nil, document.PaperPlanRasterRequest{}, pdfverify.Limits{}, err
	}
	raster := request.Raster
	if raster.PageProfile == "" && raster.DPI == 0 && raster.MaxPixels == 0 && raster.MaxSourceBytes == 0 && raster.MaxPNGBytes == 0 && len(raster.CoreFontProgram) == 0 && len(raster.FontPrograms) == 0 && len(raster.Images) == 0 {
		raster = document.DefaultPaperPlanRasterRequest()
	}
	if raster.DPI == 0 || raster.MaxPixels == 0 || raster.MaxSourceBytes == 0 || raster.MaxPNGBytes == 0 || raster.MaxPNGBytes > uint64(w.limits.MaxRenderBytes) || raster.MaxSourceBytes > uint64(w.limits.MaxRenderBytes) {
		return nil, document.PaperPlanRasterRequest{}, pdfverify.Limits{}, workspaceError("FINAL_PDF_VERIFICATION_LIMIT", "plan raster limits are incomplete or exceed workspace bounds", ErrLimit)
	}
	limits := request.Limits
	if limits == (pdfverify.Limits{}) {
		limits = pdfverify.DefaultLimits()
		workspaceBytes := uint64(w.limits.MaxRenderBytes)
		limits.MaxPDFBytes = min(limits.MaxPDFBytes, workspaceBytes)
		limits.MaxRasterBytesPage = min(limits.MaxRasterBytesPage, workspaceBytes)
		limits.MaxTotalRasterBytes = min(limits.MaxTotalRasterBytes, workspaceBytes)
	}
	if request.Structure.Pages != 0 && request.Structure.Pages != uint32(record.plan.PageCount()) {
		return nil, document.PaperPlanRasterRequest{}, pdfverify.Limits{}, workspaceError("FINAL_PDF_PLAN_MISMATCH", "structural page expectation differs from retained plan", ErrInvalidQuery)
	}
	if err := pdfverify.ValidateConfiguration(raster.DPI, request.Tolerance, limits, request.Structure, request.RequiredCompliance, request.Compliance); err != nil {
		return nil, document.PaperPlanRasterRequest{}, pdfverify.Limits{}, workspaceError("INVALID_FINAL_PDF_VERIFICATION", "verification configuration is invalid", err)
	}
	return record, raster, limits, nil
}

func digestBytes(value []byte) string { sum := sha256.Sum256(value); return hex.EncodeToString(sum[:]) }
