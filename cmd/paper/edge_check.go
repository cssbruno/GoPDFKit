// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/cssbruno/gopdfkit/document"
	"github.com/cssbruno/gopdfkit/internal/papercompile"
	"github.com/cssbruno/gopdfkit/internal/paperedge"
	"github.com/cssbruno/gopdfkit/internal/paperlang"
)

type edgeCheckRequest struct {
	file      string
	source    string
	schema    string
	locale    string
	count     uint
	seed      int64
	maxItems  uint
	outputDir string
	assets    document.PaperAssetCatalog
	jsonMode  bool
}

type edgeCheckCaseResult struct {
	Name        string                     `json:"name"`
	SHA256      string                     `json:"sha256"`
	OK          bool                       `json:"ok"`
	Pages       int                        `json:"pages,omitempty"`
	PlanHash    string                     `json:"plan_hash,omitempty"`
	PDFBytes    int                        `json:"pdf_bytes,omitempty"`
	JSONFile    string                     `json:"json_file,omitempty"`
	PDFFile     string                     `json:"pdf_file,omitempty"`
	Error       string                     `json:"error,omitempty"`
	Diagnostics []document.PaperDiagnostic `json:"diagnostics,omitempty"`
}

type edgeCheckResult struct {
	OK     bool                  `json:"ok"`
	Schema string                `json:"schema"`
	Seed   int64                 `json:"seed"`
	Cases  []edgeCheckCaseResult `json:"cases"`
}

func checkGeneratedEdgeCases(request edgeCheckRequest, stdout, stderr io.Writer) int {
	if request.count > uint(^uint32(0)) || request.maxItems > uint(^uint32(0)) {
		return commandError(request.jsonMode, stdout, stderr, "check", errors.New("edge-case bounds must fit uint32"))
	}
	parsed := paperlang.Parse(request.file, request.source)
	if !parsed.OK() {
		return languageDiagnostics(request.jsonMode, stdout, stderr, "check", parsed.Diagnostics)
	}
	extracted := papercompile.ExtractSchemasWithResolver(parsed.AST, papercompile.ImportResolver(paperFileImportResolver()))
	if !extracted.OK() {
		return languageDiagnostics(request.jsonMode, stdout, stderr, "check", extracted.Diagnostics)
	}
	schema, err := selectEdgeSchema(extracted.Schemas, request.schema)
	if err != nil {
		return commandError(request.jsonMode, stdout, stderr, "check", err)
	}
	cases, err := paperedge.Generate(schema, paperedge.Options{Count: uint32(request.count), Seed: request.seed, MaxListItems: uint32(request.maxItems)}) // #nosec G115 -- checked above.
	if err != nil {
		return commandError(request.jsonMode, stdout, stderr, "check", err)
	}
	if request.outputDir != "" {
		if err := os.MkdirAll(request.outputDir, 0o750); err != nil {
			return commandError(request.jsonMode, stdout, stderr, "check", err)
		}
	}

	report := edgeCheckResult{OK: true, Schema: schema.Name, Seed: request.seed, Cases: make([]edgeCheckCaseResult, 0, len(cases))}
	for index, generated := range cases {
		caseResult := edgeCheckCaseResult{Name: generated.Name, SHA256: generated.Digest}
		if request.outputDir != "" {
			caseResult.JSONFile = filepath.Join(request.outputDir, fmt.Sprintf("%03d-%s.json", index+1, generated.Name))
			if err := atomicWrite(caseResult.JSONFile, generated.JSON, 0o644); err != nil {
				caseResult.Error = err.Error()
				report.OK = false
				report.Cases = append(report.Cases, caseResult)
				continue
			}
		}
		plan, planned, planErr := planPaperJSON(request.file, request.source, generated.JSON, schema.Name, request.locale, "edge-"+generated.Name, request.assets)
		caseResult.Pages, caseResult.PlanHash, caseResult.Diagnostics = planned.Pages, planned.Hash, planned.Diagnostics
		if planErr != nil {
			caseResult.Error = planErr.Error()
			report.OK = false
			report.Cases = append(report.Cases, caseResult)
			continue
		}
		pdf, renderErr := renderEdgeCasePDF(plan)
		if renderErr != nil {
			caseResult.Error = renderErr.Error()
			report.OK = false
			report.Cases = append(report.Cases, caseResult)
			continue
		}
		caseResult.PDFBytes = len(pdf)
		if request.outputDir != "" {
			caseResult.PDFFile = filepath.Join(request.outputDir, fmt.Sprintf("%03d-%s.pdf", index+1, generated.Name))
			if err := atomicWrite(caseResult.PDFFile, pdf, 0o644); err != nil {
				caseResult.Error = err.Error()
				report.OK = false
				report.Cases = append(report.Cases, caseResult)
				continue
			}
		}
		caseResult.OK = true
		report.Cases = append(report.Cases, caseResult)
	}

	if request.jsonMode {
		if writeJSON(stdout, stderr, report) != exitOK || !report.OK {
			return exitFailure
		}
		return exitOK
	}
	for _, checked := range report.Cases {
		status := "ok"
		if !checked.OK {
			status = "failed"
		}
		_, _ = fmt.Fprintf(stdout, "%s\t%s\tpages=%d\tbytes=%d\n", status, checked.Name, checked.Pages, checked.PDFBytes)
		if checked.Error != "" {
			_, _ = fmt.Fprintf(stderr, "paper check: edge case %s: %s\n", checked.Name, checked.Error)
		}
		writePaperDiagnostics(stderr, checked.Diagnostics)
	}
	if !report.OK {
		return exitFailure
	}
	_, _ = fmt.Fprintf(stdout, "ok edge-cases=%d seed=%d schema=%s\n", len(report.Cases), report.Seed, report.Schema)
	return exitOK
}

func selectEdgeSchema(schemas []papercompile.SchemaDescriptor, requested string) (papercompile.SchemaDescriptor, error) {
	requested = strings.TrimSpace(requested)
	if requested != "" && !strings.HasPrefix(requested, "@") {
		requested = "@" + requested
	}
	if requested != "" {
		for _, schema := range schemas {
			if schema.Name == requested {
				return schema, nil
			}
		}
		return papercompile.SchemaDescriptor{}, fmt.Errorf("schema %s is not declared", requested)
	}
	if len(schemas) == 0 {
		return papercompile.SchemaDescriptor{}, errors.New("document declares no schema")
	}
	if len(schemas) > 1 {
		return papercompile.SchemaDescriptor{}, errors.New("document declares multiple schemas; select one with --schema")
	}
	return schemas[0], nil
}

func renderEdgeCasePDF(plan document.PaperPlan) ([]byte, error) {
	pdf, err := document.NewDocument(document.WithUnit(document.UnitPoint), document.WithDeterministicOutput())
	if err != nil {
		return nil, err
	}
	rendered, err := pdf.WritePaperPlan(plan)
	if err != nil {
		return nil, err
	}
	if !rendered.OK() {
		return nil, errors.New("edge-case PDF paint produced no pages")
	}
	var encoded bytes.Buffer
	limited := &limitWriter{w: &encoded, remaining: maxPDFBytes}
	if err := pdf.OutputWithOptions(limited, document.OutputOptions{Deterministic: true}); err != nil {
		return nil, err
	}
	if !bytes.HasPrefix(encoded.Bytes(), []byte("%PDF-")) || !bytes.Contains(encoded.Bytes(), []byte("%%EOF")) {
		return nil, errors.New("edge-case output is not a complete PDF")
	}
	return encoded.Bytes(), nil
}
