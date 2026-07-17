// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/cssbruno/gopdfkit/internal/pdfverify"
)

type studioDeliveryResponse struct {
	FormatVersion  uint16 `json:"format_version"`
	Revision       string `json:"revision"`
	SourceRevision string `json:"source_revision"`
	PlanHash       string `json:"plan_hash,omitempty"`
	Scenario       string `json:"scenario,omitempty"`
	Preflight      struct {
		Status     string `json:"status"`
		IssueCount int    `json:"issue_count"`
		PageCount  int    `json:"page_count"`
		Failure    string `json:"failure,omitempty"`
	} `json:"preflight"`
	PDFVerification struct {
		Status            string   `json:"status"`
		SHA256            string   `json:"sha256,omitempty"`
		StructureElements uint32   `json:"structure_elements,omitempty"`
		ContentMarked     uint32   `json:"content_marked,omitempty"`
		Failures          []string `json:"failures,omitempty"`
	} `json:"pdf_verification"`
	Export struct {
		Status   string `json:"status"`
		Endpoint string `json:"endpoint,omitempty"`
		Failure  string `json:"failure,omitempty"`
	} `json:"export"`
	Publish struct {
		Status string `json:"status"`
		Reason string `json:"reason"`
	} `json:"publish"`
}

func (s *studioServer) handleDelivery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), studioAPITimeout)
	defer cancel()
	snapshot, err := s.current(ctx, r.URL.Query().Get("scenario"))
	if err != nil {
		writeStudioError(w, http.StatusUnprocessableEntity, err)
		return
	}
	if r.URL.Query().Get("revision") != snapshot.revision {
		writeStudioError(w, http.StatusConflict, errors.New("paper-studio: stale delivery revision"))
		return
	}
	response := buildStudioDeliveryResponse(snapshot)
	if snapshot.pages > 0 && response.Preflight.Status == "ready" {
		pdf, renderErr := renderStudioTaggedPDF(ctx, snapshot.plan)
		if renderErr != nil {
			response.PDFVerification.Status = "failed"
			response.PDFVerification.Failures = []string{renderErr.Error()}
		} else if report, inspectErr := pdfverify.InspectTags(ctx, pdf, pdfverify.TagInspectionLimits{}); inspectErr != nil {
			response.PDFVerification.Status = "failed"
			response.PDFVerification.Failures = []string{inspectErr.Error()}
		} else {
			response.PDFVerification.SHA256 = report.PDFSHA256
			response.PDFVerification.StructureElements = report.StructureElements
			response.PDFVerification.ContentMarked = report.ContentMarked
			response.PDFVerification.Failures = append([]string(nil), report.Failures...)
			if report.Passed {
				response.PDFVerification.Status = "verified"
				response.Export.Status = "ready"
			} else {
				response.PDFVerification.Status = "failed"
			}
		}
	}
	if response.PDFVerification.Status != "verified" {
		response.Export.Status = "blocked"
		response.Export.Failure = "export requires a verified final PDF"
	}
	writeStudioJSON(w, http.StatusOK, response)
}

func buildStudioDeliveryResponse(snapshot *studioSnapshot) studioDeliveryResponse {
	result := studioDeliveryResponse{FormatVersion: 1, Revision: snapshot.revision,
		SourceRevision: studioSourceRevision(snapshot.source), PlanHash: snapshot.plan.Hash(), Scenario: snapshot.scenario}
	result.Preflight.PageCount = snapshot.pages
	result.Preflight.IssueCount = len(snapshot.diagnostics)
	if snapshot.pages == 0 {
		result.Preflight.Status = "unavailable"
		result.Preflight.Failure = "a valid plan is required"
	} else if len(snapshot.diagnostics) != 0 {
		result.Preflight.Status = "blocked"
		result.Preflight.Failure = "current source has diagnostics"
	} else {
		result.Preflight.Status = "ready"
	}
	result.PDFVerification.Status = "pending"
	result.Export.Status = "pending"
	result.Publish.Status = "separate_authorized_capability"
	result.Publish.Reason = "publish is never implied by local export or PDF verification"
	return result
}

func (s *studioServer) handleExportPDF(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), studioAPITimeout)
	defer cancel()
	snapshot, err := s.current(ctx, r.URL.Query().Get("scenario"))
	if err != nil {
		writeStudioError(w, http.StatusUnprocessableEntity, err)
		return
	}
	if snapshot.pages == 0 || r.URL.Query().Get("revision") != snapshot.revision {
		writeStudioError(w, http.StatusConflict, errors.New("paper-studio: export requires the exact current plan revision"))
		return
	}
	if len(snapshot.diagnostics) != 0 {
		writeStudioError(w, http.StatusUnprocessableEntity, errors.New("paper-studio: export is blocked by current diagnostics"))
		return
	}
	pdf, err := renderStudioTaggedPDF(ctx, snapshot.plan)
	if err != nil {
		writeStudioError(w, http.StatusUnprocessableEntity, err)
		return
	}
	report, err := pdfverify.InspectTags(ctx, pdf, pdfverify.TagInspectionLimits{})
	if err != nil || !report.Passed {
		if err == nil {
			err = fmt.Errorf("final PDF verification failed: %s", strings.Join(report.Failures, "; "))
		}
		writeStudioError(w, http.StatusUnprocessableEntity, err)
		return
	}
	name := strings.TrimSuffix(filepath.Base(snapshot.file), filepath.Ext(snapshot.file)) + ".pdf"
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pdf)
}
