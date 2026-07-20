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
	"html/template"
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
	file      string
	source    string
	schema    string
	locale    string
	count     uint
	seed      int64
	maxItems  uint
	outputDir string
	visual    bool
	assets    document.PaperAssetCatalog
	jsonMode  bool
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
	PreviewFile     string                     `json:"preview_file,omitempty"`
	PreviewSHA256   string                     `json:"preview_sha256,omitempty"`
	InputInspection *edgeCheckInputInspection  `json:"input_inspection,omitempty"`
	Inspection      *edgeCheckPDFInspection    `json:"inspection,omitempty"`
	Error           string                     `json:"error,omitempty"`
	Diagnostics     []document.PaperDiagnostic `json:"diagnostics,omitempty"`
}

type edgeCheckResult struct {
	FormatVersion uint16                `json:"format_version"`
	OK            bool                  `json:"ok"`
	Schema        string                `json:"schema"`
	Seed          int64                 `json:"seed"`
	ReportFile    string                `json:"report_file,omitempty"`
	GalleryFile   string                `json:"gallery_file,omitempty"`
	Cases         []edgeCheckCaseResult `json:"cases"`
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

	report := edgeCheckResult{FormatVersion: 2, OK: true, Schema: schema.Name, Seed: request.seed, Cases: make([]edgeCheckCaseResult, 0, len(cases))}
	if request.outputDir != "" {
		report.ReportFile = "edge-report.json"
		if request.visual {
			report.GalleryFile = "edge-gallery.html"
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
				preview, previewErr := renderEdgeCasePreview(plan)
				if previewErr != nil {
					caseResult.Stage = "render-preview"
					caseResult.Error = previewErr.Error()
					report.OK = false
					report.Cases = append(report.Cases, caseResult)
					continue
				}
				caseResult.PreviewFile = baseName + ".svg"
				caseResult.PreviewSHA256 = edgeSHA256(preview)
				if err := atomicWrite(filepath.Join(request.outputDir, caseResult.PreviewFile), preview, 0o644); err != nil {
					caseResult.Stage = "write-preview"
					caseResult.Error = err.Error()
					report.OK = false
					report.Cases = append(report.Cases, caseResult)
					continue
				}
			}
		}
		caseResult.OK = true
		report.Cases = append(report.Cases, caseResult)
	}
	if request.outputDir != "" {
		if err := writeEdgeReport(request.outputDir, report); err != nil {
			return commandError(request.jsonMode, stdout, stderr, "check", err)
		}
		if request.visual {
			if err := writeEdgeGallery(request.outputDir, report); err != nil {
				return commandError(request.jsonMode, stdout, stderr, "check", err)
			}
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

func renderEdgeCasePreview(plan document.PaperPlan) ([]byte, error) {
	if plan.PageCount() <= 0 || plan.PageCount() > 64 {
		return nil, fmt.Errorf("edge-case preview supports 1..64 pages, got %d", plan.PageCount())
	}
	columns := uint32(2)
	if plan.PageCount() == 1 {
		columns = 1
	}
	capture, err := plan.Capture(document.PaperPlanCaptureRequest{
		Mode: "core_text_svg", IncludeContactSheet: true, ContactSheetColumns: columns,
		MaxPages: uint32(plan.PageCount()), MaxCrops: 1,
		MaxArtifactBytes: 32 << 20, MaxTotalBytes: 32 << 20, MaxManifestBytes: 1 << 20,
	}) // #nosec G115 -- the page count is checked against the 64-page capture bound above.
	if err != nil {
		return nil, err
	}
	if len(capture.Artifacts) != 1 || len(capture.Artifacts[0].SVG) == 0 {
		return nil, fmt.Errorf("edge-case preview produced %d artifacts instead of one contact sheet", len(capture.Artifacts))
	}
	return append([]byte(nil), capture.Artifacts[0].SVG...), nil
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

type edgeGalleryData struct {
	Report edgeCheckResult
	Passed int
	Failed int
}

func writeEdgeGallery(outputDir string, report edgeCheckResult) error {
	data := edgeGalleryData{Report: report}
	for _, checked := range report.Cases {
		if checked.OK {
			data.Passed++
		} else {
			data.Failed++
		}
	}
	tmpl, err := template.New("edge-gallery").Parse(edgeGalleryTemplate)
	if err != nil {
		return fmt.Errorf("parse edge gallery template: %w", err)
	}
	var encoded bytes.Buffer
	if err := tmpl.Execute(&encoded, data); err != nil {
		return fmt.Errorf("render edge gallery: %w", err)
	}
	if err := atomicWrite(filepath.Join(outputDir, report.GalleryFile), encoded.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write edge gallery: %w", err)
	}
	return nil
}

func edgeSHA256(payload []byte) string {
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

const edgeGalleryTemplate = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>PDF edge-case gallery</title>
<style>
:root{color-scheme:light;font-family:Inter,ui-sans-serif,system-ui,sans-serif;background:#edf2f5;color:#16303d}
body{margin:0;padding:32px}.shell{max-width:1500px;margin:auto}header{display:flex;gap:24px;align-items:end;justify-content:space-between;margin-bottom:24px}
h1{margin:0;font-size:clamp(28px,4vw,52px);letter-spacing:-.04em}.summary{color:#526a76}.summary a{color:#0c6570}
.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(340px,1fr));gap:18px}.case{background:#fff;border:1px solid #d4e0e5;border-radius:14px;overflow:hidden;box-shadow:0 8px 24px #173c4b12}
.case.pass{border-top:5px solid #16866f}.case.fail{border-top:5px solid #c95050}.meta{padding:16px 18px}.meta h2{font-size:18px;margin:0 0 8px;overflow-wrap:anywhere}
.facts{display:flex;gap:12px;flex-wrap:wrap;color:#59717c;font-size:13px}.status{font-weight:700}.pass .status{color:#08705c}.fail .status{color:#ad3030}
.preview{display:block;width:100%;height:420px;object-fit:contain;background:#d9e2e6;border-block:1px solid #d4e0e5}.missing{height:180px;display:grid;place-items:center;background:#f7e8e8;color:#922;padding:20px;text-align:center}
.links{display:flex;gap:12px;padding:14px 18px}.links a{color:#0c6570;font-weight:650;text-decoration:none}.error{white-space:pre-wrap;overflow-wrap:anywhere;font:12px/1.45 ui-monospace,monospace;background:#fff3f3;color:#8b2525;padding:12px;border-radius:8px}
</style>
</head>
<body><main class="shell">
<header><div><h1>PDF edge-case gallery</h1><div class="summary">Schema {{.Report.Schema}} · seed {{.Report.Seed}} · {{.Passed}} passed · {{.Failed}} failed</div></div><div class="summary"><a href="{{.Report.ReportFile}}">machine-readable report</a></div></header>
<section class="grid">{{range .Report.Cases}}
<article class="case {{if .OK}}pass{{else}}fail{{end}}">
<div class="meta"><h2>{{.Name}}</h2><div class="facts"><span class="status">{{if .OK}}PASS{{else}}FAIL{{end}}</span><span>{{.Pages}} pages</span><span>{{.InputBytes}} input bytes</span><span>{{.PDFBytes}} PDF bytes</span>{{if .InputInspection}}<span>max string {{.InputInspection.MaxStringRunes}} runes at {{.InputInspection.MaxStringRunesPath}}</span><span>largest list {{.InputInspection.MaxListItems}} items</span>{{end}}{{if .Inspection}}<span>{{.Inspection.ExtractedTextRunes}} extracted runes</span><span>{{.Inspection.PageIssueCount}} plan issues</span>{{end}}</div>{{if .Error}}<p class="error">{{.Stage}}: {{.Error}}</p>{{end}}</div>
{{if .PreviewFile}}<a href="{{.PDFFile}}"><img class="preview" loading="lazy" src="{{.PreviewFile}}" alt="Visual contact sheet for {{.Name}}"></a>{{else}}<div class="missing">No visual preview was produced for this case.</div>{{end}}
<div class="links">{{if .JSONFile}}<a href="{{.JSONFile}}">Input JSON</a>{{end}}{{if .PDFFile}}<a href="{{.PDFFile}}">PDF</a>{{end}}{{if .PreviewFile}}<a href="{{.PreviewFile}}">SVG preview</a>{{end}}</div>
</article>{{end}}</section>
</main></body></html>
`

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
