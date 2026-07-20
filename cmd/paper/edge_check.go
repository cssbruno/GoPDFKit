// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/cssbruno/paperrune/document"
	"github.com/cssbruno/paperrune/inspect"
	"github.com/cssbruno/paperrune/internal/papercompile"
	"github.com/cssbruno/paperrune/internal/paperedge"
	"github.com/cssbruno/paperrune/internal/paperlang"
)

type edgeCheckRequest struct {
	file                string
	source              string
	schema              string
	locale              string
	count               uint
	seed                int64
	maxItems            uint
	outputDir           string
	visual              bool
	visualDPI           uint
	inputFiles          []string
	maxPageIssues       uint
	minTextRunes        uint
	maxPages            uint
	baseline            string
	allowBaselineChange bool
	assets              document.PaperAssetCatalog
	jsonMode            bool
}

type edgeCheckPDFInspection struct {
	StructureOK         bool                            `json:"structure_ok"`
	SHA256              string                          `json:"sha256"`
	ParsedPages         int                             `json:"parsed_pages"`
	ExtractedTextBytes  int                             `json:"extracted_text_bytes"`
	ExtractedTextRunes  int                             `json:"extracted_text_runes"`
	ExtractedTextSHA256 string                          `json:"extracted_text_sha256"`
	PageIssueCount      uint32                          `json:"page_issue_count"`
	PageText            []edgeCheckPageTextInspection   `json:"page_text"`
	PageSummaries       []document.PaperPlanPageSummary `json:"page_summaries"`
}

type edgeCheckPageTextInspection struct {
	Page   int    `json:"page"`
	Bytes  int    `json:"bytes"`
	Runes  int    `json:"runes"`
	SHA256 string `json:"sha256"`
}

type edgeCheckInputInspection struct {
	StringCount          int    `json:"string_count"`
	EmptyStringCount     int    `json:"empty_string_count"`
	WhitespaceOnlyCount  int    `json:"whitespace_only_string_count"`
	MultilineStringCount int    `json:"multiline_string_count"`
	MaxStringBytes       int    `json:"max_string_bytes"`
	MaxStringBytesPath   string `json:"max_string_bytes_path,omitempty"`
	MaxStringRunes       int    `json:"max_string_runes"`
	MaxStringRunesPath   string `json:"max_string_runes_path,omitempty"`
	NumberCount          int    `json:"number_count"`
	BooleanCount         int    `json:"boolean_count"`
	NullCount            int    `json:"null_count"`
	ObjectCount          int    `json:"object_count"`
	ListCount            int    `json:"list_count"`
	MaxListItems         int    `json:"max_list_items"`
	MaxListPath          string `json:"max_list_path,omitempty"`
	MaxDepth             int    `json:"max_depth"`
}

type edgeCheckCaseResult struct {
	Name            string                     `json:"name"`
	SHA256          string                     `json:"sha256"`
	InputBytes      int                        `json:"input_bytes"`
	OK              bool                       `json:"ok"`
	Stage           string                     `json:"stage,omitempty"`
	Pages           int                        `json:"pages,omitempty"`
	PlanHash        string                     `json:"plan_hash,omitempty"`
	PDFBytes        int                        `json:"pdf_bytes,omitempty"`
	JSONFile        string                     `json:"json_file,omitempty"`
	PDFFile         string                     `json:"pdf_file,omitempty"`
	RasterPages     []edgeCheckRasterPage      `json:"raster_pages,omitempty"`
	InputInspection *edgeCheckInputInspection  `json:"input_inspection,omitempty"`
	Inspection      *edgeCheckPDFInspection    `json:"inspection,omitempty"`
	Error           string                     `json:"error,omitempty"`
	Diagnostics     []document.PaperDiagnostic `json:"diagnostics,omitempty"`
}

type edgeCheckResult struct {
	FormatVersion    uint16                  `json:"format_version"`
	OK               bool                    `json:"ok"`
	Schema           string                  `json:"schema"`
	Seed             int64                   `json:"seed"`
	ReportFile       string                  `json:"report_file,omitempty"`
	VisualReviewFile string                  `json:"visual_review_file,omitempty"`
	Thresholds       edgeCheckThresholds     `json:"thresholds"`
	Baseline         *edgeBaselineComparison `json:"baseline,omitempty"`
	Cases            []edgeCheckCaseResult   `json:"cases"`
}

type edgeCheckThresholds struct {
	MaxPageIssues uint `json:"max_page_issues"`
	MinTextRunes  uint `json:"min_text_runes"`
	MaxPages      uint `json:"max_pages"`
}

type edgeCheckRasterPage struct {
	Page   int    `json:"page"`
	File   string `json:"file"`
	SHA256 string `json:"sha256"`
	Bytes  int    `json:"bytes"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

type edgeBaselineComparison struct {
	File      string               `json:"file"`
	Unchanged int                  `json:"unchanged"`
	Changed   int                  `json:"changed"`
	Missing   int                  `json:"missing"`
	Added     int                  `json:"added"`
	Changes   []edgeBaselineChange `json:"changes,omitempty"`
}

type edgeBaselineChange struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

func checkGeneratedEdgeCases(request edgeCheckRequest, stdout, stderr io.Writer) int {
	if request.count > uint(^uint32(0)) || request.maxItems > uint(^uint32(0)) || request.maxPageIssues > uint(^uint32(0)) {
		return commandError(request.jsonMode, stdout, stderr, "check", errors.New("edge-case bounds must fit uint32"))
	}
	if request.maxPages == 0 || request.maxPages > 1000 {
		return commandError(request.jsonMode, stdout, stderr, "check", errors.New("--edge-max-pages must be between 1 and 1000"))
	}
	if request.visual && (request.visualDPI < 36 || request.visualDPI > 300) {
		return commandError(request.jsonMode, stdout, stderr, "check", errors.New("--edge-visual-dpi must be between 36 and 300"))
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
	cases := make([]paperedge.Case, 0, int(request.count)+len(request.inputFiles))
	if request.count != 0 {
		generated, generateErr := paperedge.Generate(schema, paperedge.Options{Count: uint32(request.count), Seed: request.seed, MaxListItems: uint32(request.maxItems)}) // #nosec G115 -- checked above.
		if generateErr != nil {
			return commandError(request.jsonMode, stdout, stderr, "check", generateErr)
		}
		cases = append(cases, generated...)
	}
	custom, err := loadEdgeInputCases(request.inputFiles)
	if err != nil {
		return commandError(request.jsonMode, stdout, stderr, "check", err)
	}
	cases = append(cases, custom...)
	if err := validateUniqueEdgeCaseNames(cases); err != nil {
		return commandError(request.jsonMode, stdout, stderr, "check", err)
	}
	if request.outputDir != "" {
		if err := os.MkdirAll(request.outputDir, 0o750); err != nil {
			return commandError(request.jsonMode, stdout, stderr, "check", err)
		}
	}
	var rasterizer *edgePDFRasterizer
	if request.visual {
		rasterizer, err = newEdgePDFRasterizer()
		if err != nil {
			return commandError(request.jsonMode, stdout, stderr, "check", err)
		}
		defer rasterizer.Close()
	}

	report := edgeCheckResult{
		FormatVersion: 3, OK: true, Schema: schema.Name, Seed: request.seed,
		Thresholds: edgeCheckThresholds{MaxPageIssues: request.maxPageIssues, MinTextRunes: request.minTextRunes, MaxPages: request.maxPages},
		Cases:      make([]edgeCheckCaseResult, 0, len(cases)),
	}
	if request.outputDir != "" {
		report.ReportFile = "edge-report.json"
		if request.visual {
			report.VisualReviewFile = "edge-visual-review.pdf"
		}
	}
	for index, generated := range cases {
		baseName := fmt.Sprintf("%03d-%s", index+1, generated.Name)
		caseResult := edgeCheckCaseResult{Name: generated.Name, SHA256: generated.Digest, InputBytes: len(generated.JSON)}
		inputInspection, inputErr := inspectEdgeCaseInput(generated.JSON)
		if inputErr != nil {
			caseResult.Stage = "inspect-input"
			caseResult.Error = inputErr.Error()
			report.OK = false
			report.Cases = append(report.Cases, caseResult)
			continue
		}
		caseResult.InputInspection = &inputInspection
		if request.outputDir != "" {
			caseResult.JSONFile = baseName + ".json"
			if err := atomicWrite(filepath.Join(request.outputDir, caseResult.JSONFile), generated.JSON, 0o644); err != nil {
				caseResult.Stage = "write-input"
				caseResult.Error = err.Error()
				report.OK = false
				report.Cases = append(report.Cases, caseResult)
				continue
			}
		}
		plan, planned, planErr := planPaperJSON(request.file, request.source, generated.JSON, schema.Name, request.locale, "edge-"+generated.Name, request.assets)
		caseResult.Pages, caseResult.PlanHash, caseResult.Diagnostics = planned.Pages, planned.Hash, planned.Diagnostics
		if planErr != nil {
			caseResult.Stage = "plan"
			caseResult.Error = planErr.Error()
			report.OK = false
			report.Cases = append(report.Cases, caseResult)
			continue
		}
		pageSummaries, summaryErr := plan.PageSummaries()
		if summaryErr != nil {
			caseResult.Stage = "inspect-plan"
			caseResult.Error = summaryErr.Error()
			report.OK = false
			report.Cases = append(report.Cases, caseResult)
			continue
		}
		pdf, renderErr := renderEdgeCasePDF(plan)
		if renderErr != nil {
			caseResult.Stage = "render-pdf"
			caseResult.Error = renderErr.Error()
			report.OK = false
			report.Cases = append(report.Cases, caseResult)
			continue
		}
		caseResult.PDFBytes = len(pdf)
		inspection, inspectErr := inspectEdgeCasePDF(pdf, caseResult.Pages, pageSummaries)
		if inspectErr != nil {
			caseResult.Stage = "inspect-pdf"
			caseResult.Error = inspectErr.Error()
			report.OK = false
			report.Cases = append(report.Cases, caseResult)
			continue
		}
		caseResult.Inspection = &inspection
		if thresholdErr := evaluateEdgeThresholds(inspection, request); thresholdErr != nil {
			caseResult.Stage = "threshold"
			caseResult.Error = thresholdErr.Error()
			report.OK = false
		}
		if request.outputDir != "" {
			caseResult.PDFFile = baseName + ".pdf"
			if err := atomicWrite(filepath.Join(request.outputDir, caseResult.PDFFile), pdf, 0o644); err != nil {
				caseResult.Stage = "write-pdf"
				caseResult.Error = err.Error()
				report.OK = false
				report.Cases = append(report.Cases, caseResult)
				continue
			}
			if request.visual {
				rasterPages, rasterErr := rasterizer.Rasterize(filepath.Join(request.outputDir, caseResult.PDFFile), request.outputDir, baseName, inspection.ParsedPages, request.visualDPI)
				if rasterErr != nil {
					caseResult.Stage = "rasterize-pdf"
					caseResult.Error = rasterErr.Error()
					report.OK = false
					report.Cases = append(report.Cases, caseResult)
					continue
				}
				caseResult.RasterPages = rasterPages
			}
		}
		caseResult.OK = caseResult.Error == ""
		report.Cases = append(report.Cases, caseResult)
	}
	if request.baseline != "" {
		comparison, compareErr := compareEdgeBaseline(request.baseline, report)
		if compareErr != nil {
			return commandError(request.jsonMode, stdout, stderr, "check", compareErr)
		}
		report.Baseline = &comparison
		if len(comparison.Changes) != 0 && !request.allowBaselineChange {
			report.OK = false
		}
	}
	if request.outputDir != "" {
		if request.visual {
			if err := writeEdgeVisualReview(request.outputDir, report); err != nil {
				return commandError(request.jsonMode, stdout, stderr, "check", err)
			}
		}
		if err := writeEdgeReport(request.outputDir, report); err != nil {
			return commandError(request.jsonMode, stdout, stderr, "check", err)
		}
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

func inspectEdgeCasePDF(pdf []byte, plannedPages int, summaries []document.PaperPlanPageSummary) (edgeCheckPDFInspection, error) {
	if err := inspect.ValidateStructure(pdf); err != nil {
		return edgeCheckPDFInspection{}, fmt.Errorf("validate PDF structure: %w", err)
	}
	parsedPages, err := inspect.PageCount(pdf)
	if err != nil {
		return edgeCheckPDFInspection{}, fmt.Errorf("count PDF pages: %w", err)
	}
	if parsedPages != plannedPages || parsedPages != len(summaries) {
		return edgeCheckPDFInspection{}, fmt.Errorf("page count mismatch: planned=%d parsed=%d summaries=%d", plannedPages, parsedPages, len(summaries))
	}
	extracted, err := inspect.Text(pdf)
	if err != nil {
		return edgeCheckPDFInspection{}, fmt.Errorf("extract PDF text: %w", err)
	}
	issueCount := uint32(0)
	for _, summary := range summaries {
		issueCount += summary.IssueCount
	}
	pageText := make([]edgeCheckPageTextInspection, 0, parsedPages)
	var combined strings.Builder
	for page := 1; page <= parsedPages; page++ {
		extractedPage, pageErr := inspect.PageText(pdf, page)
		if pageErr != nil {
			return edgeCheckPDFInspection{}, fmt.Errorf("extract PDF page %d text: %w", page, pageErr)
		}
		combined.WriteString(extractedPage)
		pageText = append(pageText, edgeCheckPageTextInspection{
			Page: page, Bytes: len(extractedPage), Runes: utf8.RuneCountInString(extractedPage),
			SHA256: edgeSHA256([]byte(extractedPage)),
		})
	}
	if combined.String() != extracted {
		return edgeCheckPDFInspection{}, errors.New("whole-document text does not match concatenated per-page extraction")
	}
	return edgeCheckPDFInspection{
		StructureOK: true, SHA256: edgeSHA256(pdf), ParsedPages: parsedPages,
		ExtractedTextBytes: len(extracted), ExtractedTextRunes: utf8.RuneCountInString(extracted),
		ExtractedTextSHA256: edgeSHA256([]byte(extracted)), PageIssueCount: issueCount,
		PageText:      pageText,
		PageSummaries: append([]document.PaperPlanPageSummary(nil), summaries...),
	}, nil
}

func inspectEdgeCaseInput(payload []byte) (edgeCheckInputInspection, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return edgeCheckInputInspection{}, fmt.Errorf("decode generated JSON: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		if err == nil {
			return edgeCheckInputInspection{}, errors.New("generated JSON contains multiple values")
		}
		return edgeCheckInputInspection{}, fmt.Errorf("finish generated JSON: %w", err)
	}
	var result edgeCheckInputInspection
	inspectEdgeInputValue(value, "", 0, &result)
	return result, nil
}

func inspectEdgeInputValue(value any, path string, depth int, result *edgeCheckInputInspection) {
	if depth > result.MaxDepth {
		result.MaxDepth = depth
	}
	switch typed := value.(type) {
	case map[string]any:
		result.ObjectCount++
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			inspectEdgeInputValue(typed[key], path+"/"+escapeJSONPointerToken(key), depth+1, result)
		}
	case []any:
		result.ListCount++
		if len(typed) > result.MaxListItems || len(typed) == result.MaxListItems && (result.MaxListPath == "" || path < result.MaxListPath) {
			result.MaxListItems = len(typed)
			result.MaxListPath = edgeDisplayJSONPointer(path)
		}
		for index, item := range typed {
			inspectEdgeInputValue(item, fmt.Sprintf("%s/%d", path, index), depth+1, result)
		}
	case string:
		result.StringCount++
		if typed == "" {
			result.EmptyStringCount++
		} else if strings.TrimSpace(typed) == "" {
			result.WhitespaceOnlyCount++
		}
		if strings.ContainsAny(typed, "\r\n") {
			result.MultilineStringCount++
		}
		bytes := len(typed)
		runes := utf8.RuneCountInString(typed)
		displayPath := edgeDisplayJSONPointer(path)
		if bytes > result.MaxStringBytes || bytes == result.MaxStringBytes && (result.MaxStringBytesPath == "" || displayPath < result.MaxStringBytesPath) {
			result.MaxStringBytes = bytes
			result.MaxStringBytesPath = displayPath
		}
		if runes > result.MaxStringRunes || runes == result.MaxStringRunes && (result.MaxStringRunesPath == "" || displayPath < result.MaxStringRunesPath) {
			result.MaxStringRunes = runes
			result.MaxStringRunesPath = displayPath
		}
	case json.Number:
		result.NumberCount++
	case bool:
		result.BooleanCount++
	case nil:
		result.NullCount++
	}
}

func edgeDisplayJSONPointer(path string) string {
	if path == "" {
		return "/"
	}
	return path
}

func escapeJSONPointerToken(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "~", "~0"), "/", "~1")
}

func writeEdgeReport(outputDir string, report edgeCheckResult) error {
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode edge report: %w", err)
	}
	encoded = append(encoded, '\n')
	if err := atomicWrite(filepath.Join(outputDir, report.ReportFile), encoded, 0o644); err != nil {
		return fmt.Errorf("write edge report: %w", err)
	}
	return nil
}

func edgeSHA256(payload []byte) string {
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
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
