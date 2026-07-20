// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cssbruno/paperrune/internal/paperedge"
)

func loadEdgeInputCases(files []string) ([]paperedge.Case, error) {
	cases := make([]paperedge.Case, 0, len(files))
	for _, file := range files {
		if file == "-" {
			return nil, fmt.Errorf("--edge-input does not accept stdin; use a named JSON file")
		}
		payload, err := readBoundedFile(file, nil, maxSourceBytes, "edge JSON input")
		if err != nil {
			return nil, fmt.Errorf("read edge input %s: %w", file, err)
		}
		if _, err := inspectEdgeCaseInput(payload); err != nil {
			return nil, fmt.Errorf("inspect edge input %s: %w", file, err)
		}
		name := "user-" + edgeCaseName(strings.TrimSuffix(filepath.Base(file), filepath.Ext(file)))
		cases = append(cases, paperedge.Case{Name: name, Digest: edgeSHA256(payload), JSON: payload})
	}
	return cases, nil
}

func edgeCaseName(value string) string {
	var name strings.Builder
	separator := false
	for _, char := range strings.ToLower(strings.TrimSpace(value)) {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' {
			name.WriteRune(char)
			separator = false
			continue
		}
		if !separator && name.Len() != 0 {
			name.WriteByte('-')
			separator = true
		}
	}
	cleaned := strings.Trim(name.String(), "-")
	if cleaned == "" {
		return "case"
	}
	if len(cleaned) > 80 {
		cleaned = strings.TrimRight(cleaned[:80], "-")
	}
	return cleaned
}

func validateUniqueEdgeCaseNames(cases []paperedge.Case) error {
	seen := make(map[string]struct{}, len(cases))
	for _, item := range cases {
		if _, exists := seen[item.Name]; exists {
			return fmt.Errorf("duplicate edge-case name %q; rename the corresponding input file", item.Name)
		}
		seen[item.Name] = struct{}{}
	}
	return nil
}

func evaluateEdgeThresholds(inspection edgeCheckPDFInspection, request edgeCheckRequest) error {
	if uint64(inspection.PageIssueCount) > uint64(request.maxPageIssues) {
		return fmt.Errorf("page issue count %d exceeds maximum %d", inspection.PageIssueCount, request.maxPageIssues)
	}
	if uint64(inspection.ExtractedTextRunes) < uint64(request.minTextRunes) {
		return fmt.Errorf("extracted text runes %d is below minimum %d", inspection.ExtractedTextRunes, request.minTextRunes)
	}
	if uint64(inspection.ParsedPages) > uint64(request.maxPages) {
		return fmt.Errorf("PDF page count %d exceeds maximum %d", inspection.ParsedPages, request.maxPages)
	}
	return nil
}

type edgeBaselineFingerprint struct {
	OK                  bool
	Stage               string
	Error               string
	InputSHA256         string
	Pages               int
	PlanHash            string
	PDFSHA256           string
	ExtractedTextSHA256 string
	PageIssueCount      uint32
}

func edgeCaseFingerprint(item edgeCheckCaseResult) edgeBaselineFingerprint {
	fingerprint := edgeBaselineFingerprint{
		OK: item.OK, Stage: item.Stage, Error: item.Error, InputSHA256: item.SHA256,
		Pages: item.Pages, PlanHash: item.PlanHash,
	}
	if item.Inspection != nil {
		fingerprint.PDFSHA256 = item.Inspection.SHA256
		fingerprint.ExtractedTextSHA256 = item.Inspection.ExtractedTextSHA256
		fingerprint.PageIssueCount = item.Inspection.PageIssueCount
	}
	return fingerprint
}

func compareEdgeBaseline(file string, current edgeCheckResult) (edgeBaselineComparison, error) {
	payload, err := readBoundedFile(file, nil, maxSourceBytes, "edge baseline")
	if err != nil {
		return edgeBaselineComparison{}, fmt.Errorf("read edge baseline: %w", err)
	}
	var baseline edgeCheckResult
	if err := json.Unmarshal(payload, &baseline); err != nil {
		return edgeBaselineComparison{}, fmt.Errorf("decode edge baseline: %w", err)
	}
	if baseline.FormatVersion < 2 || baseline.FormatVersion > 3 {
		return edgeBaselineComparison{}, fmt.Errorf("unsupported edge baseline format %d", baseline.FormatVersion)
	}

	comparison := edgeBaselineComparison{File: file}
	oldCases := make(map[string]edgeCheckCaseResult, len(baseline.Cases))
	for _, item := range baseline.Cases {
		if _, exists := oldCases[item.Name]; exists {
			return edgeBaselineComparison{}, fmt.Errorf("edge baseline contains duplicate case %q", item.Name)
		}
		oldCases[item.Name] = item
	}
	for _, item := range current.Cases {
		previous, exists := oldCases[item.Name]
		if !exists {
			comparison.Added++
			comparison.Changes = append(comparison.Changes, edgeBaselineChange{Name: item.Name, Status: "added"})
			continue
		}
		delete(oldCases, item.Name)
		if edgeCaseFingerprint(previous) == edgeCaseFingerprint(item) {
			comparison.Unchanged++
			continue
		}
		comparison.Changed++
		comparison.Changes = append(comparison.Changes, edgeBaselineChange{Name: item.Name, Status: "changed"})
	}
	for name := range oldCases {
		comparison.Missing++
		comparison.Changes = append(comparison.Changes, edgeBaselineChange{Name: name, Status: "missing"})
	}
	sort.Slice(comparison.Changes, func(i, j int) bool {
		return comparison.Changes[i].Name < comparison.Changes[j].Name
	})
	return comparison, nil
}
