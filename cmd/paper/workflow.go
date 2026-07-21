// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/cssbruno/paperrune/document"
	"github.com/cssbruno/paperrune/internal/paperd"
	"github.com/cssbruno/paperrune/internal/paperscenario"
)

func runWorkflow(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	set := flags("workflow", stderr)
	target := set.String("target", "", "exact readable source ID to edit")
	literalFile := set.String("literal-file", "", "read replacement literal from FILE")
	fontFile := set.String("font", "", "font program used for deterministic review rasterization")
	output := set.String("o", "", "write the approved PDF atomically to FILE")
	actor := set.String("actor", "", "bounded authenticated actor identity")
	scenario := set.String("scenario", "typical", "required scenario identity")
	scenarioResult := set.String("scenario-result-hash", "", "SHA-256 of the passed scenario report")
	validatorProfile := set.String("validator-profile", "layout", "required validator profile")
	validatorVersion := set.String("validator-version", "1", "required validator version")
	validatorHash := set.String("validator-hash", "", "SHA-256 of the passed validator report")
	approvalNonce := set.String("approval-nonce", "", "unique reviewer nonce of at least 16 bytes")
	approve := set.Bool("approve", false, "explicitly approve the exact review manifest")
	file, code := parseOneFile(set, args)
	if code >= 0 {
		return code
	}
	if *target == "" || *literalFile == "" || *fontFile == "" || *output == "" || *actor == "" ||
		*scenarioResult == "" || *validatorHash == "" || *approvalNonce == "" || !*approve {
		return commandError(true, stdout, stderr, "workflow", errors.New("--target, --literal-file, --font, -o, --actor, evidence hashes, --approval-nonce, and --approve are required"))
	}
	if file == "-" && *literalFile == "-" {
		return commandError(true, stdout, stderr, "workflow", errors.New("document source and literal cannot both use stdin"))
	}
	source, err := readSource(file, stdin)
	if err != nil {
		return commandError(true, stdout, stderr, "workflow", err)
	}
	literal, err := readWorkflowFile(*literalFile, stdin, maxSourceBytes)
	if err != nil {
		return commandError(true, stdout, stderr, "workflow", fmt.Errorf("literal: %w", err))
	}
	font, err := readWorkflowFile(*fontFile, stdin, maxSourceBytes)
	if err != nil {
		return commandError(true, stdout, stderr, "workflow", fmt.Errorf("font: %w", err))
	}
	policy := paperd.CandidateAcceptancePolicy{RequiredScenarios: []string{*scenario},
		RequiredValidators:      []paperd.CandidateValidatorRequirement{{Profile: *validatorProfile, Version: *validatorVersion}},
		RequiredReviewArtifacts: []string{"review_manifest"}}
	workspace, err := paperd.NewWorkspaceWithOptions(paperd.WorkspaceOptions{ProjectID: "paper-cli", PolicyRevision: "paper-cli-policy-v1",
		DisclosureDomain: paperd.DisclosureRestricted, RequireMutationAuthority: true, ProtectedNodeIDs: []string{*target}, CandidateAcceptance: policy})
	if err != nil {
		return commandError(true, stdout, stderr, "workflow", err)
	}
	ctx := context.Background()
	candidate, err := workspace.BeginHeadlessLiteralWorkflow(ctx, paperd.HeadlessLiteralRequest{File: displayFile(file), Source: string(source),
		Target: *target, Literal: string(literal), Actor: *actor, IdempotencyKey: "paper-cli-edit-v1", ProtectedNodes: []string{*target}})
	if err != nil {
		return commandError(true, stdout, stderr, "workflow", err)
	}
	reviewRequest := document.DefaultPaperReviewRequest()
	reviewRequest.CoreFontProgram = font
	reviewRequest.MaxArtifactBytes, reviewRequest.MaxTotalBytes, reviewRequest.MaxManifestBytes = 4<<20, 12<<20, 1<<20
	review, err := workspace.ReviewHeadlessCandidate(ctx, paperd.HeadlessReviewRequest{Candidate: candidate,
		Selectors: []document.PaperPlanSelector{{Key: *target, MaxResults: 16}}, MaxExplainBytes: maxExplain, Review: reviewRequest})
	if err != nil {
		return commandError(true, stdout, stderr, "workflow", err)
	}
	scenarioRevision, err := workspace.CreateScenarioRevision([]paperscenario.Scenario{{Name: *scenario}}, paperscenario.Limits{})
	if err != nil {
		return commandError(true, stdout, stderr, "workflow", err)
	}
	acceptance, err := workspace.AcceptHeadlessCandidate(ctx, paperd.HeadlessAcceptanceRequest{Review: review, Actor: *actor,
		IdempotencyKey:  "paper-cli-accept-v1",
		Scenarios:       []paperd.ScenarioAcceptanceEvidence{{Revision: scenarioRevision.Handle, Name: *scenario, Digest: scenarioRevision.Digest, ResultHash: *scenarioResult, Passed: true}},
		Validators:      []paperd.ValidatorAcceptanceEvidence{{Profile: *validatorProfile, Version: *validatorVersion, Hash: *validatorHash, Passed: true}},
		ReviewArtifacts: []paperd.ReviewAcceptanceEvidence{{Kind: "review_manifest", Hash: review.ReviewManifestHash, Approved: true}},
		ApprovalNonce:   *approvalNonce + ":accept", ApprovalTTL: 5 * time.Minute})
	if err != nil {
		return commandError(true, stdout, stderr, "workflow", err)
	}
	prepared, err := workspace.PrepareHeadlessExport(paperd.HeadlessExportRequest{Review: review, Acceptance: acceptance,
		Actor: *actor, Target: *output,
		ApprovalNonce: *approvalNonce + ":export", ApprovalTTL: 5 * time.Minute})
	if err != nil {
		return commandError(true, stdout, stderr, "workflow", err)
	}
	executed, err := workspace.ExecuteHeadlessExport(ctx, prepared, func(_ context.Context, input paperd.SensitiveOperationInput) (paperd.SensitiveExecutionOutcome, error) {
		if err := atomicWrite(*output, input.Payload, 0o600); err != nil {
			return paperd.SensitiveExecutionOutcome{}, err
		}
		return paperd.SensitiveExecutionOutcome{ExternalID: "local-file", ResultHash: workflowHash(input.Payload), Bytes: int64(len(input.Payload))}, nil
	})
	if err != nil {
		return commandError(true, stdout, stderr, "workflow", err)
	}
	return writeJSON(stdout, stderr, struct {
		OK                 bool   `json:"ok"`
		CandidateRevision  string `json:"candidate_revision"`
		PlanHash           string `json:"plan_hash"`
		ReviewManifestHash string `json:"review_manifest_hash"`
		AcceptanceHash     string `json:"acceptance_hash"`
		PDFSHA256          string `json:"pdf_sha256"`
		PDFBytes           int    `json:"pdf_bytes"`
		ExportAuditHash    string `json:"export_audit_hash"`
	}{true, candidate.HeadDigest, candidate.HeadPlanHash, review.ReviewManifestHash, acceptance.AcceptanceHash,
		prepared.PDFSHA256, prepared.PDFBytes, executed.ExecutionAuditHash})
}

func readWorkflowFile(name string, stdin io.Reader, limit int) ([]byte, error) {
	if name == "-" {
		data, err := io.ReadAll(io.LimitReader(stdin, int64(limit)+1))
		if err != nil {
			return nil, err
		}
		if len(data) > limit {
			return nil, errors.New("input exceeds limit")
		}
		return data, nil
	}
	info, err := os.Stat(name)
	if err != nil {
		return nil, err
	}
	if info.Size() < 0 || info.Size() > int64(limit) {
		return nil, errors.New("input exceeds limit")
	}
	return os.ReadFile(name)
}

func workflowHash(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}
