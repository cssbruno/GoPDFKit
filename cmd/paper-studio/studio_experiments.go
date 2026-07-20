// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package main

import (
	"context"
	"errors"
	"net/http"

	"github.com/cssbruno/paperrune/document"
)

type studioTypedExperimentsResponse struct {
	FormatVersion  uint16                                   `json:"format_version"`
	Revision       string                                   `json:"revision"`
	SourceRevision string                                   `json:"source_revision"`
	PlanHash       string                                   `json:"plan_hash,omitempty"`
	Scenario       string                                   `json:"scenario,omitempty"`
	Projection     document.TypedCharacterizationProjection `json:"projection"`
}

// handleTypedExperiments exposes the bounded typed compatibility corpus as a
// read-only, revision-bound Studio artifact. The corpus is compiler-owned and
// never uses the browser document or JavaScript layout as an experiment.
func (s *studioServer) handleTypedExperiments(w http.ResponseWriter, r *http.Request) {
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
		writeStudioError(w, http.StatusConflict, errors.New("paper-studio: stale typed experiment revision"))
		return
	}

	snapshot.typedMu.Lock()
	if snapshot.typed == nil {
		projection, runErr := document.RunTypedCharacterization(ctx, document.TypedCharacterizationLimits{})
		if runErr != nil {
			snapshot.typedMu.Unlock()
			writeStudioError(w, http.StatusUnprocessableEntity, runErr)
			return
		}
		snapshot.typed = &projection
	}
	projection := *snapshot.typed
	snapshot.typedMu.Unlock()

	writeStudioJSON(w, http.StatusOK, studioTypedExperimentsResponse{
		FormatVersion: 1, Revision: snapshot.revision,
		SourceRevision: studioSourceRevision(snapshot.source), PlanHash: snapshot.plan.Hash(),
		Scenario: snapshot.scenario, Projection: projection,
	})
}
