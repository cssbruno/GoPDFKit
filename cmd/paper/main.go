// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

// Command paper provides deterministic, bounded tools for .paper documents.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cssbruno/gopdfkit/document"
	"github.com/cssbruno/gopdfkit/internal/paperassets"
	"github.com/cssbruno/gopdfkit/internal/papercompile"
	"github.com/cssbruno/gopdfkit/internal/paperlang"
	"github.com/cssbruno/gopdfkit/internal/paperscenario"
)

const (
	exitOK      = 0
	exitFailure = 1
	exitUsage   = 2

	maxSourceBytes = 8 << 20
	maxPDFBytes    = 64 << 20
	maxExplain     = 4 << 20
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return exitUsage
	}
	if args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		printUsage(stdout)
		return exitOK
	}
	commands := map[string]func([]string, io.Reader, io.Writer, io.Writer) int{
		"fmt":      runFmt,
		"check":    runCheck,
		"render":   runRender,
		"capture":  runCapture,
		"explain":  runExplain,
		"scenario": runScenario,
		"workflow": runWorkflow,
	}
	command, ok := commands[args[0]]
	if !ok {
		_, _ = fmt.Fprintf(stderr, "paper: unknown command %q\n", args[0])
		printUsage(stderr)
		return exitUsage
	}
	return command(args[1:], stdin, stdout, stderr)
}

func printUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "usage: paper <fmt|check|render|capture|explain|scenario|workflow> [options] FILE")
}

func flags(name string, stderr io.Writer) *flag.FlagSet {
	set := flag.NewFlagSet("paper "+name, flag.ContinueOnError)
	set.SetOutput(stderr)
	return set
}

func parseOneFile(set *flag.FlagSet, args []string) (string, int) {
	if err := set.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return "", exitOK
		}
		return "", exitUsage
	}
	if set.NArg() != 1 {
		_, _ = fmt.Fprintf(set.Output(), "paper %s: expected exactly one FILE (use - for stdin)\n", set.Name()[len("paper "):])
		return "", exitUsage
	}
	return set.Arg(0), -1
}

type assetOptions struct {
	manifest *string
	root     *string
}

func addAssetFlags(set *flag.FlagSet) assetOptions {
	return assetOptions{
		manifest: set.String("assets", "", "load an explicit content-addressed asset manifest"),
		root:     set.String("asset-root", "", "resolve manifest asset paths under this explicit root"),
	}
}

func (options assetOptions) load() (document.PaperAssetCatalog, error) {
	if *options.root != "" && *options.manifest == "" {
		return document.PaperAssetCatalog{}, errors.New("--asset-root requires --assets")
	}
	if *options.manifest == "" {
		return document.NewPaperAssetCatalog(nil)
	}
	loaded, err := paperassets.LoadManifestResources(*options.manifest, *options.root)
	if err != nil {
		return document.PaperAssetCatalog{}, err
	}
	resources := make([]document.PaperAssetResource, len(loaded))
	for index, resource := range loaded {
		resources[index] = document.PaperAssetResource{
			Name: resource.Name, MediaType: resource.MediaType, Digest: resource.Digest, Data: resource.Data,
		}
	}
	return document.NewPaperAssetCatalog(resources)
}

func runFmt(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	set := flags("fmt", stderr)
	write := set.Bool("w", false, "atomically replace FILE with canonical formatting")
	jsonMode := set.Bool("json", false, "write a JSON result")
	file, code := parseOneFile(set, args)
	if code >= 0 {
		return code
	}
	if *write && file == "-" {
		return commandError(*jsonMode, stdout, stderr, "fmt", errors.New("-w cannot be used with stdin"))
	}
	source, err := readSource(file, stdin)
	if err != nil {
		return commandError(*jsonMode, stdout, stderr, "fmt", err)
	}
	parsed := paperlang.Parse(displayFile(file), string(source))
	if !parsed.OK() {
		return languageDiagnostics(*jsonMode, stdout, stderr, "fmt", parsed.Diagnostics)
	}
	formatted, err := paperlang.FormatWithOptions(parsed.AST, paperlang.FormatOptions{MaxBytes: maxSourceBytes})
	if err != nil {
		return commandError(*jsonMode, stdout, stderr, "fmt", err)
	}
	changed := !bytes.Equal(source, formatted)
	if *write && changed {
		mode := os.FileMode(0o644)
		if info, statErr := os.Stat(file); statErr == nil {
			mode = info.Mode().Perm()
		}
		if err := atomicWrite(file, formatted, mode); err != nil {
			return commandError(*jsonMode, stdout, stderr, "fmt", err)
		}
	}
	if *jsonMode {
		result := struct {
			OK        bool   `json:"ok"`
			Changed   bool   `json:"changed"`
			Formatted string `json:"formatted,omitempty"`
		}{OK: true, Changed: changed}
		if !*write {
			result.Formatted = string(formatted)
		}
		return writeJSON(stdout, stderr, result)
	}
	if !*write {
		if _, err := stdout.Write(formatted); err != nil {
			_, _ = fmt.Fprintf(stderr, "paper fmt: %v\n", err)
			return exitFailure
		}
	}
	return exitOK
}

func runCheck(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	set := flags("check", stderr)
	jsonMode := set.Bool("json", false, "write a JSON result")
	scenario := set.String("scenario", "", "plan with the selected scenario")
	assets := addAssetFlags(set)
	file, code := parseOneFile(set, args)
	if code >= 0 {
		return code
	}
	catalog, err := assets.load()
	if err != nil {
		return commandError(*jsonMode, stdout, stderr, "check", err)
	}
	source, err := readSource(file, stdin)
	if err != nil {
		return commandError(*jsonMode, stdout, stderr, "check", err)
	}
	_, result, planErr := planPaper(displayFile(file), string(source), *scenario, catalog)
	if *jsonMode {
		out := struct {
			OK          bool                       `json:"ok"`
			Pages       int                        `json:"pages"`
			Hash        string                     `json:"hash,omitempty"`
			Diagnostics []document.PaperDiagnostic `json:"diagnostics,omitempty"`
		}{OK: planErr == nil, Pages: result.Pages, Hash: result.Hash, Diagnostics: result.Diagnostics}
		if writeJSON(stdout, stderr, out) != exitOK || planErr != nil {
			return exitFailure
		}
		return exitOK
	}
	writePaperDiagnostics(stderr, result.Diagnostics)
	if planErr != nil {
		return exitFailure
	}
	_, _ = fmt.Fprintf(stdout, "ok pages=%d hash=%s\n", result.Pages, result.Hash)
	return exitOK
}

func runRender(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	set := flags("render", stderr)
	output := set.String("o", "", "write PDF atomically to FILE (default: stdout)")
	jsonMode := set.Bool("json", false, "write a JSON result; requires -o")
	scenario := set.String("scenario", "", "plan with the selected scenario")
	assets := addAssetFlags(set)
	file, code := parseOneFile(set, args)
	if code >= 0 {
		return code
	}
	if *jsonMode && *output == "" {
		return commandError(true, stdout, stderr, "render", errors.New("--json requires -o so JSON and PDF do not share stdout"))
	}
	catalog, err := assets.load()
	if err != nil {
		return commandError(*jsonMode, stdout, stderr, "render", err)
	}
	source, err := readSource(file, stdin)
	if err != nil {
		return commandError(*jsonMode, stdout, stderr, "render", err)
	}
	plan, planned, err := planPaper(displayFile(file), string(source), *scenario, catalog)
	if err != nil {
		return paperDiagnostics(*jsonMode, stdout, stderr, "render", planned.Diagnostics)
	}
	pdf, err := document.NewDocument(document.WithUnit(document.UnitPoint), document.WithDeterministicOutput())
	if err != nil {
		return commandError(*jsonMode, stdout, stderr, "render", err)
	}
	painted, err := pdf.WritePaperPlan(plan)
	if err != nil {
		return paperDiagnostics(*jsonMode, stdout, stderr, "render", painted.Diagnostics)
	}
	var encoded bytes.Buffer
	limited := &limitWriter{w: &encoded, remaining: maxPDFBytes}
	if err := pdf.OutputWithOptions(limited, document.OutputOptions{Deterministic: true}); err != nil {
		return commandError(*jsonMode, stdout, stderr, "render", err)
	}
	if *output != "" {
		if err := atomicWrite(*output, encoded.Bytes(), 0o644); err != nil {
			return commandError(*jsonMode, stdout, stderr, "render", err)
		}
	} else if _, err := stdout.Write(encoded.Bytes()); err != nil {
		_, _ = fmt.Fprintf(stderr, "paper render: %v\n", err)
		return exitFailure
	}
	if *jsonMode {
		return writeJSON(stdout, stderr, struct {
			OK    bool   `json:"ok"`
			Pages int    `json:"pages"`
			Hash  string `json:"hash"`
			Bytes int    `json:"bytes"`
			File  string `json:"file"`
		}{true, painted.Pages, plan.Hash(), encoded.Len(), *output})
	}
	return exitOK
}

func runExplain(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	set := flags("explain", stderr)
	node := set.Uint("node", 0, "select a node ID")
	key := set.String("key", "", "select a readable key")
	instance := set.String("instance", "", "select an instance ID")
	fragment := set.Uint("fragment", 0, "select a fragment ID")
	page := set.Uint("page", 0, "select a one-based page")
	maxResults := set.Uint("max-results", 64, "maximum structural results")
	jsonMode := set.Bool("json", false, "write JSON (accepted for command consistency)")
	scenario := set.String("scenario", "", "plan with the selected scenario")
	assets := addAssetFlags(set)
	file, code := parseOneFile(set, args)
	if code >= 0 {
		return code
	}
	if *node > uint(^uint32(0)) || *fragment > uint(^uint32(0)) || *page > uint(^uint32(0)) || *maxResults == 0 || *maxResults > uint(^uint32(0)) {
		return commandError(*jsonMode, stdout, stderr, "explain", errors.New("selectors and --max-results must fit positive uint32 values"))
	}
	selector := document.PaperPlanSelector{Node: uint32(*node), Key: *key, Instance: *instance, Fragment: uint32(*fragment), Page: uint32(*page), MaxResults: uint32(*maxResults)}
	if selector.Node == 0 && selector.Key == "" && selector.Instance == "" && selector.Fragment == 0 && selector.Page == 0 {
		return commandError(*jsonMode, stdout, stderr, "explain", errors.New("provide at least one selector: --node, --key, --instance, --fragment, or --page"))
	}
	catalog, err := assets.load()
	if err != nil {
		return commandError(*jsonMode, stdout, stderr, "explain", err)
	}
	source, err := readSource(file, stdin)
	if err != nil {
		return commandError(*jsonMode, stdout, stderr, "explain", err)
	}
	plan, result, err := planPaper(displayFile(file), string(source), *scenario, catalog)
	if err != nil {
		return paperDiagnostics(*jsonMode, stdout, stderr, "explain", result.Diagnostics)
	}
	explanation, err := plan.Explain([]document.PaperPlanSelector{selector}, 1, maxExplain)
	if err != nil {
		return commandError(*jsonMode, stdout, stderr, "explain", err)
	}
	if _, err := stdout.Write(append(explanation.JSON(), '\n')); err != nil {
		_, _ = fmt.Fprintf(stderr, "paper explain: %v\n", err)
		return exitFailure
	}
	return exitOK
}

type uint32List []uint32

func (v *uint32List) String() string {
	parts := make([]string, len(*v))
	for i, value := range *v {
		parts[i] = strconv.FormatUint(uint64(value), 10)
	}
	return strings.Join(parts, ",")
}

func (v *uint32List) Set(raw string) error {
	value, err := strconv.ParseUint(raw, 10, 32)
	if err != nil || value == 0 {
		return fmt.Errorf("must be a positive uint32")
	}
	*v = append(*v, uint32(value))
	return nil
}

func runCapture(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	set := flags("capture", stderr)
	mode := set.String("mode", "geometry_svg", "geometry_svg or core_text_svg")
	contact := set.Bool("contact-sheet", false, "include a contact sheet in addition to selected crops")
	columns := set.Uint("columns", 1, "contact-sheet columns")
	maxPages := set.Uint("max-pages", 1, "maximum captured pages")
	maxCrops := set.Uint("max-crops", 32, "maximum captured crops")
	output := set.String("o", "", "write output atomically to FILE (default: stdout)")
	jsonMode := set.Bool("json", false, "write a JSON capture bundle instead of one SVG")
	scenario := set.String("scenario", "", "plan with the selected scenario")
	assets := addAssetFlags(set)
	var nodes, fragments uint32List
	set.Var(&nodes, "node", "capture node ID (repeatable)")
	set.Var(&fragments, "fragment", "capture fragment ID (repeatable)")
	file, code := parseOneFile(set, args)
	if code >= 0 {
		return code
	}
	if *mode != "geometry_svg" && *mode != "core_text_svg" {
		return commandError(*jsonMode, stdout, stderr, "capture", errors.New("--mode must be geometry_svg or core_text_svg"))
	}
	if *columns == 0 || *columns > uint(^uint32(0)) || *maxPages == 0 || *maxPages > uint(^uint32(0)) || *maxCrops == 0 || *maxCrops > uint(^uint32(0)) {
		return commandError(*jsonMode, stdout, stderr, "capture", errors.New("capture limits must be positive uint32 values"))
	}
	includeContactSheet := *contact || (len(nodes) == 0 && len(fragments) == 0)
	catalog, err := assets.load()
	if err != nil {
		return commandError(*jsonMode, stdout, stderr, "capture", err)
	}
	source, err := readSource(file, stdin)
	if err != nil {
		return commandError(*jsonMode, stdout, stderr, "capture", err)
	}
	plan, result, err := planPaper(displayFile(file), string(source), *scenario, catalog)
	if err != nil {
		return paperDiagnostics(*jsonMode, stdout, stderr, "capture", result.Diagnostics)
	}
	capture, err := plan.Capture(document.PaperPlanCaptureRequest{
		Mode: *mode, IncludeContactSheet: includeContactSheet, ContactSheetColumns: uint32(*columns),
		Nodes: nodes, Fragments: fragments, MaxPages: uint32(*maxPages), MaxCrops: uint32(*maxCrops),
		MaxArtifactBytes: 4 << 20, MaxTotalBytes: 32 << 20, MaxManifestBytes: 1 << 20,
	})
	if err != nil {
		return commandError(*jsonMode, stdout, stderr, "capture", err)
	}
	var payload []byte
	if *jsonMode {
		artifacts := make([]struct {
			Metadata json.RawMessage `json:"metadata"`
			SVG      string          `json:"svg"`
		}, len(capture.Artifacts))
		for i, artifact := range capture.Artifacts {
			artifacts[i].Metadata = json.RawMessage(artifact.MetadataJSON)
			artifacts[i].SVG = string(artifact.SVG)
		}
		payload, err = json.Marshal(struct {
			PlanHash  string          `json:"plan_hash"`
			Manifest  json.RawMessage `json:"manifest"`
			Artifacts any             `json:"artifacts"`
		}{capture.PlanHash, json.RawMessage(capture.ManifestJSON), artifacts})
		payload = append(payload, '\n')
	} else if len(capture.Artifacts) != 1 {
		return commandError(false, stdout, stderr, "capture", fmt.Errorf("capture produced %d artifacts; use --json to keep the complete bundle", len(capture.Artifacts)))
	} else {
		payload = capture.Artifacts[0].SVG
	}
	if err != nil {
		return commandError(*jsonMode, stdout, stderr, "capture", err)
	}
	if *output != "" {
		if err := atomicWrite(*output, payload, 0o644); err != nil {
			return commandError(*jsonMode, stdout, stderr, "capture", err)
		}
	} else if _, err := stdout.Write(payload); err != nil {
		_, _ = fmt.Fprintf(stderr, "paper capture: %v\n", err)
		return exitFailure
	}
	return exitOK
}

func runScenario(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	set := flags("scenario", stderr)
	selected := set.String("scenario", "", "select one scenario by exact name")
	jsonMode := set.Bool("json", false, "write full fixtures as JSON")
	file, code := parseOneFile(set, args)
	if code >= 0 {
		return code
	}
	source, err := readSource(file, stdin)
	if err != nil {
		return commandError(*jsonMode, stdout, stderr, "scenario", err)
	}
	compiled := papercompile.CompileScenarioSource(displayFile(file), string(source))
	if !compiled.OK() {
		return languageDiagnostics(*jsonMode, stdout, stderr, "scenario", compiled.Diagnostics)
	}
	fixtures, err := paperscenario.Resolve(compiled.Scenarios, paperscenario.Limits{})
	if err != nil {
		return commandError(*jsonMode, stdout, stderr, "scenario", err)
	}
	if *selected != "" {
		for _, fixture := range fixtures {
			if fixture.Name == *selected {
				if *jsonMode {
					return writeJSON(stdout, stderr, fixture)
				}
				_, _ = fmt.Fprintf(stdout, "%s\t%s\t%s\n", fixture.Name, fixture.Digest, fixture.Locale)
				return exitOK
			}
		}
		return commandError(*jsonMode, stdout, stderr, "scenario", fmt.Errorf("scenario %q not found", *selected))
	}
	if *jsonMode {
		return writeJSON(stdout, stderr, fixtures)
	}
	for _, fixture := range fixtures {
		_, _ = fmt.Fprintf(stdout, "%s\t%s\t%s\n", fixture.Name, fixture.Digest, fixture.Locale)
	}
	return exitOK
}

func planPaper(file, source, scenario string, assets document.PaperAssetCatalog) (document.PaperPlan, document.PaperPlanResult, error) {
	resolver := paperFileImportResolver()
	if scenario != "" {
		return document.PlanPaperScenarioWithAssetsAndImports(file, source, scenario, assets, resolver)
	}
	return document.PlanPaperWithAssetsAndImports(file, source, assets, resolver)
}

func paperFileImportResolver() document.PaperImportResolver {
	return func(importerFile, importPath string) (string, string, error) {
		base := filepath.Dir(importerFile)
		if importerFile == "" || importerFile == "stdin.paper" {
			base = "."
		}
		file := filepath.Clean(filepath.Join(base, filepath.FromSlash(importPath)))
		source, err := os.ReadFile(file)
		if err != nil {
			return "", "", err
		}
		if len(source) > maxSourceBytes {
			return "", "", fmt.Errorf("imported source exceeds %d bytes", maxSourceBytes)
		}
		return file, string(source), nil
	}
}

func displayFile(file string) string {
	if file == "-" {
		return "stdin.paper"
	}
	return file
}

func readSource(file string, stdin io.Reader) ([]byte, error) {
	reader := stdin
	var opened *os.File
	if file != "-" {
		var err error
		opened, err = os.Open(file) // #nosec G304 -- file is the explicit CLI input path; no shell is involved.
		if err != nil {
			return nil, err
		}
		defer func() { _ = opened.Close() }()
		reader = opened
	}
	data, err := io.ReadAll(io.LimitReader(reader, maxSourceBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxSourceBytes {
		return nil, fmt.Errorf("source exceeds %d-byte limit", maxSourceBytes)
	}
	return data, nil
}

func atomicWrite(name string, data []byte, mode os.FileMode) (err error) {
	dir := filepath.Dir(name)
	temporary, err := os.CreateTemp(dir, "."+filepath.Base(name)+".tmp-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryName)
	}()
	if err = temporary.Chmod(mode); err == nil {
		_, err = temporary.Write(data)
	}
	if err == nil {
		err = temporary.Sync()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(temporaryName, name)
	}
	return err
}

type limitWriter struct {
	w         io.Writer
	remaining int64
}

func (w *limitWriter) Write(data []byte) (int, error) {
	if int64(len(data)) > w.remaining {
		return 0, fmt.Errorf("output exceeds %d-byte limit", maxPDFBytes)
	}
	n, err := w.w.Write(data)
	w.remaining -= int64(n)
	return n, err
}

func writeJSON(stdout, stderr io.Writer, value any) int {
	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		_, _ = fmt.Fprintf(stderr, "paper: encode JSON: %v\n", err)
		return exitFailure
	}
	return exitOK
}

func commandError(jsonMode bool, stdout, stderr io.Writer, command string, err error) int {
	if jsonMode {
		_ = writeJSON(stdout, stderr, struct {
			OK    bool   `json:"ok"`
			Error string `json:"error"`
		}{false, err.Error()})
	} else {
		_, _ = fmt.Fprintf(stderr, "paper %s: %v\n", command, err)
	}
	return exitFailure
}

func languageDiagnostics(jsonMode bool, stdout, stderr io.Writer, command string, diagnostics []paperlang.Diagnostic) int {
	if jsonMode {
		_ = writeJSON(stdout, stderr, struct {
			OK          bool                   `json:"ok"`
			Diagnostics []paperlang.Diagnostic `json:"diagnostics"`
		}{false, diagnostics})
	} else {
		for _, diagnostic := range diagnostics {
			_, _ = fmt.Fprintf(stderr, "%s:%d:%d: %s %s: %s\n", diagnostic.Span.File, diagnostic.Span.Start.Line, diagnostic.Span.Start.Column, diagnostic.Severity, diagnostic.Code, diagnostic.Message)
		}
	}
	return exitFailure
}

func paperDiagnostics(jsonMode bool, stdout, stderr io.Writer, command string, diagnostics []document.PaperDiagnostic) int {
	if jsonMode {
		_ = writeJSON(stdout, stderr, struct {
			OK          bool                       `json:"ok"`
			Diagnostics []document.PaperDiagnostic `json:"diagnostics"`
		}{false, diagnostics})
	} else {
		writePaperDiagnostics(stderr, diagnostics)
		if len(diagnostics) == 0 {
			_, _ = fmt.Fprintf(stderr, "paper %s: operation failed\n", command)
		}
	}
	return exitFailure
}

func writePaperDiagnostics(w io.Writer, diagnostics []document.PaperDiagnostic) {
	for _, diagnostic := range diagnostics {
		_, _ = fmt.Fprintf(w, "%s:%d:%d: %s %s/%s: %s\n", diagnostic.File, diagnostic.StartLine, diagnostic.StartColumn, diagnostic.Severity, diagnostic.Stage, diagnostic.Code, diagnostic.Message)
	}
}
