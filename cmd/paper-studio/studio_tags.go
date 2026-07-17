// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/cssbruno/gopdfkit/document"
	"github.com/cssbruno/gopdfkit/internal/pdfverify"
)

const studioTaggedPDFLimit = 64 << 20

type studioPDFTagsResponse struct {
	FormatVersion  uint16              `json:"format_version"`
	Evidence       string              `json:"evidence"`
	SourceRevision string              `json:"source_revision"`
	PlanRevision   string              `json:"plan_revision"`
	Report         pdfverify.TagReport `json:"report"`
}

func (s *studioServer) handlePDFTags(w http.ResponseWriter, r *http.Request) {
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
	if snapshot.pages == 0 || r.URL.Query().Get("revision") == "" || r.URL.Query().Get("revision") != snapshot.revision {
		writeStudioError(w, http.StatusConflict, errors.New("paper-studio: tag inspection requires the exact current plan revision"))
		return
	}
	pdf, err := renderStudioTaggedPDF(ctx, snapshot.plan)
	if err != nil {
		writeStudioError(w, http.StatusUnprocessableEntity, err)
		return
	}
	report, err := pdfverify.InspectTags(ctx, pdf, pdfverify.TagInspectionLimits{})
	if err != nil {
		writeStudioError(w, http.StatusUnprocessableEntity, fmt.Errorf("paper-studio: inspect final PDF tags: %w", err))
		return
	}
	writeStudioJSON(w, http.StatusOK, studioPDFTagsResponse{
		FormatVersion: 1, Evidence: "final_serialized_pdf", SourceRevision: studioSourceRevision(snapshot.source),
		PlanRevision: snapshot.revision, Report: report,
	})
}

func renderStudioTaggedPDF(ctx context.Context, plan document.PaperPlan) ([]byte, error) {
	pdf, err := document.NewDocument(document.WithUnit(document.UnitPoint), document.WithDeterministicOutput())
	if err != nil {
		return nil, err
	}
	pdf.EnableTaggedPDF()
	rendered, err := pdf.WritePaperPlan(plan)
	if err != nil {
		return nil, fmt.Errorf("paper-studio: paint tagged final PDF: %w", err)
	}
	if rendered.Pages != plan.PageCount() {
		return nil, fmt.Errorf("paper-studio: tagged final PDF painted %d pages, want %d", rendered.Pages, plan.PageCount())
	}
	buffer := studioBoundedBuffer{limit: studioTaggedPDFLimit}
	if err := pdf.OutputWithOptionsContext(ctx, &buffer, document.OutputOptions{Deterministic: true}); err != nil {
		return nil, fmt.Errorf("paper-studio: serialize tagged final PDF: %w", err)
	}
	return append([]byte(nil), buffer.Bytes()...), nil
}

type studioBoundedBuffer struct {
	bytes.Buffer
	limit int
}

func (buffer *studioBoundedBuffer) Write(value []byte) (int, error) {
	if len(value) > buffer.limit-buffer.Len() {
		return 0, errors.New("paper-studio: tagged final PDF exceeds the inspection limit")
	}
	return buffer.Buffer.Write(value)
}
