// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
	"github.com/cssbruno/gopdfkit/internal/papercompile"
	"github.com/cssbruno/gopdfkit/internal/paperedit"
	"github.com/cssbruno/gopdfkit/internal/paperlang"
	"github.com/cssbruno/gopdfkit/layout"
)

// ErrPaperRender reports that a .paper pipeline failed. Inspect
// PaperRenderResult.Diagnostics for source-oriented details.
var ErrPaperRender = errors.New("document: paper render failed")

// PaperRenderStage identifies the pipeline stage that produced a diagnostic.
type PaperRenderStage string

const (
	PaperStageParse   PaperRenderStage = "parse"
	PaperStageCompile PaperRenderStage = "compile"
	PaperStagePlan    PaperRenderStage = "plan"
	PaperStagePaint   PaperRenderStage = "paint"
)

// PaperDiagnostic is a stable, editor-friendly pipeline diagnostic. Source
// positions are one-based. A zero end position means the failure applies to
// the complete input rather than one syntax token.
type PaperDiagnostic struct {
	Stage       PaperRenderStage `json:"stage"`
	Code        string           `json:"code"`
	Severity    string           `json:"severity"`
	Message     string           `json:"message"`
	Hint        string           `json:"hint,omitempty"`
	File        string           `json:"file,omitempty"`
	StartLine   uint32           `json:"start_line,omitempty"`
	StartColumn uint32           `json:"start_column,omitempty"`
	EndLine     uint32           `json:"end_line,omitempty"`
	EndColumn   uint32           `json:"end_column,omitempty"`
}

// PaperAssetResource binds a readable `asset:name` to verified immutable
// bytes. Digest is the lowercase SHA-256 of Data and is mandatory.
type PaperAssetResource struct {
	Name      string
	MediaType string
	Digest    string
	Data      []byte
	Family    string
	Style     string
	Weight    uint16
	License   string
}

// PaperAssetCatalog is an opaque, immutable content-addressed input. Construct
// it explicitly; planning never searches the filesystem or network.
type PaperAssetCatalog struct {
	compile papercompile.AssetCatalog
}

// PaperImportResolver is the explicit source boundary for reusable design
// imports. It receives the importing file and the authored relative path, and
// returns the imported file's stable name plus source bytes.
type PaperImportResolver func(importerFile, importPath string) (file, source string, err error)

// PaperJSONOptions selects the declared schema and locale for strict external
// JSON data. Schema may include or omit its leading @. Name is a stable,
// human-readable fixture identity used in diagnostics and deterministic input
// manifests; it defaults to external-data.
type PaperJSONOptions struct {
	Name   string
	Schema string
	Locale string
}

func NewPaperAssetCatalog(resources []PaperAssetResource) (PaperAssetCatalog, error) {
	compiled := make([]papercompile.AssetResource, len(resources))
	for index, resource := range resources {
		compiled[index] = papercompile.AssetResource{Name: resource.Name, MediaType: resource.MediaType, Digest: resource.Digest, Data: resource.Data, Family: resource.Family, Style: resource.Style, Weight: resource.Weight, License: resource.License}
	}
	catalog, err := papercompile.NewAssetCatalog(compiled)
	if err != nil {
		return PaperAssetCatalog{}, err
	}
	return PaperAssetCatalog{compile: catalog}, nil
}

func (catalog PaperAssetCatalog) ResourceCount() int { return catalog.compile.Len() }

// PaperRenderResult describes an attempted parse-to-PDF transaction. Pages is
// the number of pages committed by this call and is zero on every failure.
type PaperRenderResult struct {
	Pages       int               `json:"pages"`
	Diagnostics []PaperDiagnostic `json:"diagnostics,omitempty"`
}

// PaperPlan is an immutable, paint-independent result of parsing, compiling,
// measuring, wrapping, and paginating one .paper source. Its representation is
// intentionally private so the document API does not expose layoutengine's
// evolving plan schema.
type PaperPlan struct {
	plan         layoutengine.LayoutPlan
	mapping      papercompile.CompileMapping
	file         string
	title        string
	language     string
	root         paperlang.Span
	hash         string
	pages        int
	revisions    layoutengine.ViewerRevisionIdentityInput
	imageSources plannedImageSources
	fontSources  plannedFontSources
}

// PaperPlanResult describes a plan transaction without exposing internal
// layout tables. Pages and Hash are zero on failure.
type PaperPlanResult struct {
	Pages       int               `json:"pages"`
	Hash        string            `json:"hash,omitempty"`
	Diagnostics []PaperDiagnostic `json:"diagnostics,omitempty"`
}

// OK reports whether planning committed at least one immutable page without
// an error diagnostic.
func (r PaperPlanResult) OK() bool {
	if r.Pages == 0 {
		return false
	}
	for _, diagnostic := range r.Diagnostics {
		if diagnostic.Severity == string(paperlang.SeverityError) {
			return false
		}
	}
	return true
}

// PageCount returns the number of immutable pages in the plan.
func (p PaperPlan) PageCount() int { return p.pages }

// Hash returns the canonical SHA-256 layout-plan hash, or an empty string for
// the zero PaperPlan.
func (p PaperPlan) Hash() string { return p.hash }

// OK reports whether the pipeline committed its planned pages without an
// error diagnostic.
func (r PaperRenderResult) OK() bool {
	if r.Pages == 0 {
		return false
	}
	for _, diagnostic := range r.Diagnostics {
		if diagnostic.Severity == string(paperlang.SeverityError) {
			return false
		}
	}
	return true
}

// WritePaper parses, compiles, plans, and paints one .paper source directly
// through the typed layout path. It does not use HTML. The initial production
// contract accepts plain paragraphs, headings, and explicit page-break
// markers using PDF core fonts.
//
// Parsing, semantic compilation, line wrapping, pagination, glyph planning,
// and PDF painter preflight all complete before the target Document is
// changed. Failures in those stages leave f untouched. An unexpected failure
// after the preflight-approved paint commit starts may leave an opened target.
func (f *Document) WritePaper(file, source string) (PaperRenderResult, error) {
	plan, planned, err := PlanPaper(file, source)
	if err != nil {
		return PaperRenderResult{Diagnostics: planned.Diagnostics}, err
	}
	rendered, err := f.WritePaperPlan(plan)
	rendered.Diagnostics = append(append([]PaperDiagnostic(nil), planned.Diagnostics...), rendered.Diagnostics...)
	return rendered, err
}

// WritePaperWithAssets is WritePaper with an explicit immutable catalog for
// human-readable `asset:name` references.
func (f *Document) WritePaperWithAssets(file, source string, assets PaperAssetCatalog) (PaperRenderResult, error) {
	plan, planned, err := PlanPaperWithAssets(file, source, assets)
	if err != nil {
		return PaperRenderResult{Diagnostics: planned.Diagnostics}, err
	}
	rendered, err := f.WritePaperPlan(plan)
	rendered.Diagnostics = append(append([]PaperDiagnostic(nil), planned.Diagnostics...), rendered.Diagnostics...)
	return rendered, err
}

// WritePaperWithImports is WritePaper with an explicit resolver for reusable
// themes and styles declared in source-relative files.
func (f *Document) WritePaperWithImports(file, source string, resolver PaperImportResolver) (PaperRenderResult, error) {
	plan, planned, err := PlanPaperWithImports(file, source, resolver)
	if err != nil {
		return PaperRenderResult{Diagnostics: planned.Diagnostics}, err
	}
	rendered, err := f.WritePaperPlan(plan)
	rendered.Diagnostics = append(append([]PaperDiagnostic(nil), planned.Diagnostics...), rendered.Diagnostics...)
	return rendered, err
}

// WritePaperWithAssetsAndImports combines explicit asset and source-import
// boundaries for direct rendering.
func (f *Document) WritePaperWithAssetsAndImports(file, source string, assets PaperAssetCatalog, resolver PaperImportResolver) (PaperRenderResult, error) {
	plan, planned, err := PlanPaperWithAssetsAndImports(file, source, assets, resolver)
	if err != nil {
		return PaperRenderResult{Diagnostics: planned.Diagnostics}, err
	}
	rendered, err := f.WritePaperPlan(plan)
	rendered.Diagnostics = append(append([]PaperDiagnostic(nil), planned.Diagnostics...), rendered.Diagnostics...)
	return rendered, err
}

// WritePaperScenario is WritePaper with one explicitly selected source
// scenario. Bounded repeat nodes are expanded before planning.
func (f *Document) WritePaperScenario(file, source, scenario string) (PaperRenderResult, error) {
	plan, planned, err := PlanPaperScenario(file, source, scenario)
	if err != nil {
		return PaperRenderResult{Diagnostics: planned.Diagnostics}, err
	}
	rendered, err := f.WritePaperPlan(plan)
	rendered.Diagnostics = append(append([]PaperDiagnostic(nil), planned.Diagnostics...), rendered.Diagnostics...)
	return rendered, err
}

func (f *Document) WritePaperScenarioWithAssets(file, source, scenario string, assets PaperAssetCatalog) (PaperRenderResult, error) {
	plan, planned, err := PlanPaperScenarioWithAssets(file, source, scenario, assets)
	if err != nil {
		return PaperRenderResult{Diagnostics: planned.Diagnostics}, err
	}
	rendered, err := f.WritePaperPlan(plan)
	rendered.Diagnostics = append(append([]PaperDiagnostic(nil), planned.Diagnostics...), rendered.Diagnostics...)
	return rendered, err
}

// WritePaperScenarioWithImports renders one selected scenario with reusable
// themes and styles supplied by an explicit resolver.
func (f *Document) WritePaperScenarioWithImports(file, source, scenario string, resolver PaperImportResolver) (PaperRenderResult, error) {
	plan, planned, err := PlanPaperScenarioWithImports(file, source, scenario, resolver)
	if err != nil {
		return PaperRenderResult{Diagnostics: planned.Diagnostics}, err
	}
	rendered, err := f.WritePaperPlan(plan)
	rendered.Diagnostics = append(append([]PaperDiagnostic(nil), planned.Diagnostics...), rendered.Diagnostics...)
	return rendered, err
}

// WritePaperScenarioWithAssetsAndImports combines explicit assets and imports
// for one selected scenario.
func (f *Document) WritePaperScenarioWithAssetsAndImports(file, source, scenario string, assets PaperAssetCatalog, resolver PaperImportResolver) (PaperRenderResult, error) {
	plan, planned, err := PlanPaperScenarioWithAssetsAndImports(file, source, scenario, assets, resolver)
	if err != nil {
		return PaperRenderResult{Diagnostics: planned.Diagnostics}, err
	}
	rendered, err := f.WritePaperPlan(plan)
	rendered.Diagnostics = append(append([]PaperDiagnostic(nil), planned.Diagnostics...), rendered.Diagnostics...)
	return rendered, err
}

// WritePaperJSON validates one external JSON object against a declared schema,
// expands bounded repeats, and paints the resulting plan. It is the concise
// data-driven path for application-generated documents.
func (f *Document) WritePaperJSON(file, source string, data []byte) (PaperRenderResult, error) {
	return f.WritePaperJSONWithOptions(file, source, data, PaperJSONOptions{})
}

// WritePaperJSONWithOptions is WritePaperJSON with explicit schema, locale,
// and fixture identity selection.
func (f *Document) WritePaperJSONWithOptions(file, source string, data []byte, options PaperJSONOptions) (PaperRenderResult, error) {
	plan, planned, err := PlanPaperJSONWithOptions(file, source, data, options)
	if err != nil {
		return PaperRenderResult{Diagnostics: planned.Diagnostics}, err
	}
	rendered, err := f.WritePaperPlan(plan)
	rendered.Diagnostics = append(append([]PaperDiagnostic(nil), planned.Diagnostics...), rendered.Diagnostics...)
	return rendered, err
}

// WritePaperJSONWithAssetsAndImports combines the concise external JSON path
// with explicit immutable assets and reusable source imports.
func (f *Document) WritePaperJSONWithAssetsAndImports(file, source string, data []byte, options PaperJSONOptions, assets PaperAssetCatalog, resolver PaperImportResolver) (PaperRenderResult, error) {
	plan, planned, err := PlanPaperJSONWithAssetsAndImports(file, source, data, options, assets, resolver)
	if err != nil {
		return PaperRenderResult{Diagnostics: planned.Diagnostics}, err
	}
	rendered, err := f.WritePaperPlan(plan)
	rendered.Diagnostics = append(append([]PaperDiagnostic(nil), planned.Diagnostics...), rendered.Diagnostics...)
	return rendered, err
}

// PlanPaper performs the complete read-only .paper pipeline. It never paints
// and cannot mutate a caller-owned Document.
func PlanPaper(file, source string) (PaperPlan, PaperPlanResult, error) {
	return PlanPaperContext(context.Background(), file, source)
}

// PlanPaperContext propagates cancellation and one cumulative work budget
// through parsing-adjacent planning and every nested layout child.
func PlanPaperContext(ctx context.Context, file, source string) (PaperPlan, PaperPlanResult, error) {
	return planPaperSource(ctx, file, source, "", false, papercompile.AssetCatalog{}, nil)
}

func PlanPaperWithAssets(file, source string, assets PaperAssetCatalog) (PaperPlan, PaperPlanResult, error) {
	return PlanPaperWithAssetsContext(context.Background(), file, source, assets)
}

func PlanPaperWithAssetsContext(ctx context.Context, file, source string, assets PaperAssetCatalog) (PaperPlan, PaperPlanResult, error) {
	return planPaperSource(ctx, file, source, "", false, assets.compile, nil)
}

// PlanPaperWithImports is PlanPaper with an explicit resolver for source-
// relative reusable themes and styles.
func PlanPaperWithImports(file, source string, resolver PaperImportResolver) (PaperPlan, PaperPlanResult, error) {
	return PlanPaperWithImportsContext(context.Background(), file, source, resolver)
}

func PlanPaperWithImportsContext(ctx context.Context, file, source string, resolver PaperImportResolver) (PaperPlan, PaperPlanResult, error) {
	return planPaperSource(ctx, file, source, "", false, papercompile.AssetCatalog{}, papercompile.ImportResolver(resolver))
}

// PlanPaperWithAssetsAndImports combines explicit asset and source-import
// boundaries.
func PlanPaperWithAssetsAndImports(file, source string, assets PaperAssetCatalog, resolver PaperImportResolver) (PaperPlan, PaperPlanResult, error) {
	return PlanPaperWithAssetsAndImportsContext(context.Background(), file, source, assets, resolver)
}

func PlanPaperWithAssetsAndImportsContext(ctx context.Context, file, source string, assets PaperAssetCatalog, resolver PaperImportResolver) (PaperPlan, PaperPlanResult, error) {
	return planPaperSource(ctx, file, source, "", false, assets.compile, papercompile.ImportResolver(resolver))
}

// PlanPaperScenario plans one explicitly selected source scenario and expands
// its bounded keyed repeats. It performs no I/O and never consults ambient data.
func PlanPaperScenario(file, source, scenario string) (PaperPlan, PaperPlanResult, error) {
	return PlanPaperScenarioContext(context.Background(), file, source, scenario)
}

func PlanPaperScenarioContext(ctx context.Context, file, source, scenario string) (PaperPlan, PaperPlanResult, error) {
	return planPaperSource(ctx, file, source, scenario, true, papercompile.AssetCatalog{}, nil)
}

func PlanPaperScenarioWithAssets(file, source, scenario string, assets PaperAssetCatalog) (PaperPlan, PaperPlanResult, error) {
	return PlanPaperScenarioWithAssetsContext(context.Background(), file, source, scenario, assets)
}

func PlanPaperScenarioWithAssetsContext(ctx context.Context, file, source, scenario string, assets PaperAssetCatalog) (PaperPlan, PaperPlanResult, error) {
	return planPaperSource(ctx, file, source, scenario, true, assets.compile, nil)
}

// PlanPaperScenarioWithImports plans one explicitly selected scenario with an
// explicit resolver for reusable themes and styles.
func PlanPaperScenarioWithImports(file, source, scenario string, resolver PaperImportResolver) (PaperPlan, PaperPlanResult, error) {
	return PlanPaperScenarioWithImportsContext(context.Background(), file, source, scenario, resolver)
}

func PlanPaperScenarioWithImportsContext(ctx context.Context, file, source, scenario string, resolver PaperImportResolver) (PaperPlan, PaperPlanResult, error) {
	return planPaperSource(ctx, file, source, scenario, true, papercompile.AssetCatalog{}, papercompile.ImportResolver(resolver))
}

// PlanPaperScenarioWithAssetsAndImports combines explicit asset and source-
// import boundaries for one selected scenario.
func PlanPaperScenarioWithAssetsAndImports(file, source, scenario string, assets PaperAssetCatalog, resolver PaperImportResolver) (PaperPlan, PaperPlanResult, error) {
	return PlanPaperScenarioWithAssetsAndImportsContext(context.Background(), file, source, scenario, assets, resolver)
}

func PlanPaperScenarioWithAssetsAndImportsContext(ctx context.Context, file, source, scenario string, assets PaperAssetCatalog, resolver PaperImportResolver) (PaperPlan, PaperPlanResult, error) {
	return planPaperSource(ctx, file, source, scenario, true, assets.compile, papercompile.ImportResolver(resolver))
}

// PlanPaperJSON plans strict external JSON data using the document's only
// declared schema. It performs no I/O and does not mutate a Document.
func PlanPaperJSON(file, source string, data []byte) (PaperPlan, PaperPlanResult, error) {
	return PlanPaperJSONWithOptions(file, source, data, PaperJSONOptions{})
}

// PlanPaperJSONWithOptions is PlanPaperJSON with explicit schema, locale, and
// fixture identity selection.
func PlanPaperJSONWithOptions(file, source string, data []byte, options PaperJSONOptions) (PaperPlan, PaperPlanResult, error) {
	return planPaperJSONSource(context.Background(), file, source, data, options, papercompile.AssetCatalog{}, nil)
}

// PlanPaperJSONWithAssetsAndImports combines strict external JSON data with
// explicit immutable asset and source-import boundaries.
func PlanPaperJSONWithAssetsAndImports(file, source string, data []byte, options PaperJSONOptions, assets PaperAssetCatalog, resolver PaperImportResolver) (PaperPlan, PaperPlanResult, error) {
	return planPaperJSONSource(context.Background(), file, source, data, options, assets.compile, papercompile.ImportResolver(resolver))
}

type paperJSONSelection struct {
	data    []byte
	options PaperJSONOptions
}

func planPaperSource(ctx context.Context, file, source, scenario string, selectScenario bool, assets papercompile.AssetCatalog, resolver papercompile.ImportResolver) (PaperPlan, PaperPlanResult, error) {
	return planPaperSourceSelection(ctx, file, source, scenario, selectScenario, nil, assets, resolver)
}

func planPaperJSONSource(ctx context.Context, file, source string, data []byte, options PaperJSONOptions, assets papercompile.AssetCatalog, resolver papercompile.ImportResolver) (PaperPlan, PaperPlanResult, error) {
	return planPaperSourceSelection(ctx, file, source, "", true, &paperJSONSelection{data: data, options: options}, assets, resolver)
}

func planPaperSourceSelection(ctx context.Context, file, source, scenario string, selectScenario bool, data *paperJSONSelection, assets papercompile.AssetCatalog, resolver papercompile.ImportResolver) (PaperPlan, PaperPlanResult, error) {
	var budgetErr error
	ctx, budgetErr = ensureDocumentPlanningBudget(ctx)
	if budgetErr != nil {
		return PaperPlan{}, PaperPlanResult{}, budgetErr
	}
	if err := layoutengine.ChargePlanningWork(ctx, ".paper document planning", uint64(len(source))+1); err != nil {
		return PaperPlan{}, PaperPlanResult{}, err
	}
	parsed := paperlang.Parse(file, source)
	result := PaperPlanResult{Diagnostics: paperDiagnostics(PaperStageParse, parsed.Diagnostics)}
	if !parsed.OK() {
		return paperPlanFailure(result, PaperStageParse, errors.New("source is invalid"))
	}

	compiled := papercompile.CompileWithAssetsAndResolver(parsed.AST, assets, resolver)
	if data != nil {
		compiled = papercompile.CompileJSONDataWithAssetsAndResolver(parsed.AST, data.data, papercompile.JSONDataOptions{
			Name: data.options.Name, Schema: data.options.Schema, Locale: data.options.Locale,
		}, assets, resolver)
	} else if selectScenario {
		compiled = papercompile.CompileScenarioWithAssetsAndResolver(parsed.AST, scenario, assets, resolver)
	}
	result.Diagnostics = append(result.Diagnostics, paperDiagnostics(PaperStageCompile, compiled.Diagnostics)...)
	if !compiled.OK() {
		return paperPlanFailure(result, PaperStageCompile, errors.New("semantic compilation failed"))
	}
	compiled.Mapping.SourceRevision = string(paperedit.SourceRevision(source))

	planner, err := newPaperPlanner(compiled.Page)
	if err != nil {
		return paperPlanStageFailure(result, PaperStagePlan, "PAPER_PLAN_PAGE", err, file, parsed.AST.Root)
	}
	if err := installPaperCatalogFonts(planner, assets); err != nil {
		return paperPlanStageFailure(result, PaperStagePlan, "PAPER_PLAN_FONT_RESOURCES", err, file, parsed.AST.Root)
	}
	var planned layoutengine.LayoutPlan
	blocks := layout.NormalizeBlocks(compiled.Document.Body)
	if len(blocks) == 1 && typedShadowTemplateHasOnlyMargins(compiled.Document.PageTemplate) {
		if canvas, ok := blocks[0].(layout.CanvasBlock); ok {
			planned, err = planner.planPaperCanvas(ctx, compiled.Document, compiled.Mapping, canvas)
		} else if typedBlocksContainTable(compiled.Document.Body) {
			planned, err = planner.planTypedMixedBodiesMapped(ctx, compiled.Document, compiled.Mapping, nil)
		} else {
			planned, err = planner.planPaperTextBlocksMappedContext(ctx, compiled.Document, compiled.Mapping)
		}
	} else if !typedShadowTemplateHasOnlyMargins(compiled.Document.PageTemplate) {
		planned, err = planner.planTypedPageTemplate(ctx, compiled.Document, compiled.Mapping)
	} else if typedBlocksContainTable(compiled.Document.Body) || (len(blocks) > 1 && typedBlocksContainRowColumn(compiled.Document.Body)) {
		planned, err = planner.planTypedMixedBodiesMapped(ctx, compiled.Document, compiled.Mapping, nil)
	} else {
		planned, err = planner.planPaperTextBlocksMappedContext(ctx, compiled.Document, compiled.Mapping)
	}
	if err != nil {
		code := "PAPER_PLAN_FAILED"
		hint := "use plain paragraphs or headings with printable ASCII text and PDF core fonts"
		if errors.Is(err, errTypedShadowUnsupported) {
			code = "PAPER_PLAN_UNSUPPORTED"
		}
		return paperPlanStageFailureWithHint(result, PaperStagePlan, code, err, hint, file, parsed.AST.Root)
	}
	selectionIdentity := scenario
	if data != nil {
		selectionIdentity = "data:" + compiled.ScenarioDigest
	}
	planned, err = bindPaperDeterministicInputs(planned, compiled, source, selectionIdentity, selectScenario)
	if err != nil {
		return paperPlanStageFailure(result, PaperStagePlan, "PAPER_PLAN_INPUT_IDENTITY", err, file, parsed.AST.Root)
	}
	imageSources, err := typedLayoutImageSourcesContext(ctx, compiled.Document, 8<<20)
	if err != nil {
		return paperPlanStageFailure(result, PaperStagePlan, "PAPER_PLAN_IMAGE_RESOURCES", err, file, parsed.AST.Root)
	}
	fontSources, err := planner.typedLayoutFontSourcesContext(ctx, planned)
	if err != nil {
		return paperPlanStageFailure(result, PaperStagePlan, "PAPER_PLAN_FONT_RESOURCES", err, file, parsed.AST.Root)
	}
	hash, err := planned.Hash()
	if err != nil {
		return paperPlanStageFailure(result, PaperStagePlan, "PAPER_PLAN_HASH", err, file, parsed.AST.Root)
	}
	root := paperlang.Span{File: file}
	if parsed.AST.Root != nil {
		root = parsed.AST.Root.HeaderSpan
	}
	plan := PaperPlan{plan: planned, file: file, title: compiled.Document.Title,
		language: strings.TrimSpace(compiled.Document.Language), root: root, hash: hash.String(),
		pages: len(planned.Projection().Pages), revisions: paperViewerRevisions(source, selectionIdentity, selectScenario),
		mapping:      clonePaperCompileMapping(compiled.Mapping),
		imageSources: imageSources, fontSources: fontSources}
	result.Pages, result.Hash = plan.PageCount(), plan.Hash()
	return plan, result, nil
}

func installPaperCatalogFonts(planner *Document, assets papercompile.AssetCatalog) error {
	if planner == nil {
		return errors.New("paper font planner is nil")
	}
	for _, resource := range assets.FontResources() {
		family := resource.Name
		style := ""
		if resource.Weight >= 600 {
			style = "B"
		}
		if resource.Style == "italic" || resource.Style == "oblique" {
			if style == "B" {
				style = "BI"
			} else {
				style = "I"
			}
		}
		if err := planner.AddUTF8FontFromBytesError(family, style, resource.Data); err != nil {
			return fmt.Errorf("install project font %q: %w", resource.Name, err)
		}
	}
	return nil
}

func bindPaperDeterministicInputs(plan layoutengine.LayoutPlan, compiled papercompile.Result, source, scenario string, selected bool) (layoutengine.LayoutPlan, error) {
	templateHash, err := compiled.Tree.SemanticHash()
	if err != nil {
		return layoutengine.LayoutPlan{}, fmt.Errorf("derive semantic template identity: %w", err)
	}
	template, err := layoutengine.ParseSemanticTemplateID(templateHash)
	if err != nil {
		return layoutengine.LayoutPlan{}, err
	}
	revisions := paperViewerRevisions(source, scenario, selected)
	scenarioRevision, err := layoutengine.ParseScenarioRevisionID(revisions.ScenarioRevision)
	if err != nil {
		return layoutengine.LayoutPlan{}, err
	}
	resources, err := layoutengine.ResourceCatalogFromPlan(plan)
	if err != nil {
		return layoutengine.LayoutPlan{}, fmt.Errorf("derive resource catalog: %w", err)
	}
	width, err := layoutengine.FixedFromPoints(compiled.Page.Width)
	if err != nil {
		return layoutengine.LayoutPlan{}, err
	}
	height, err := layoutengine.FixedFromPoints(compiled.Page.Height)
	if err != nil {
		return layoutengine.LayoutPlan{}, err
	}
	page, err := layoutengine.NewPageProfileManifest("paper-point-size", width, height)
	if err != nil {
		return layoutengine.LayoutPlan{}, err
	}
	locale := strings.TrimSpace(compiled.Locale)
	if locale == "" {
		locale = strings.TrimSpace(compiled.Document.Language)
	}
	if locale == "" {
		locale = "und"
	}
	manifest, err := layoutengine.NewDeterministicInputManifest(
		template, scenarioRevision, resources, locale, "UTC", layoutengine.BuiltinTextDataVersions(),
		"paper-language/0.1", []string{"core-font-ascii", "no-cldr", "no-hyphenation"}, page,
		layoutengine.PlannerVersion,
	)
	if err != nil {
		return layoutengine.LayoutPlan{}, err
	}
	return plan.WithDeterministicInputs(manifest)
}

func paperViewerRevisions(source, scenario string, selected bool) layoutengine.ViewerRevisionIdentityInput {
	sourceRevision := string(paperedit.SourceRevision(source))
	scenarioName := "default"
	if selected {
		scenarioName = strings.TrimPrefix(strings.TrimSpace(scenario), "@")
	}
	scenarioDigest := sha256.Sum256([]byte("gopdfkit.paper-scenario-revision.v1\x00" + sourceRevision + "\x00" + scenarioName))
	policyDigest := sha256.Sum256([]byte("gopdfkit.paper-planning-policy.v1"))
	return layoutengine.ViewerRevisionIdentityInput{
		SourceRevision: sourceRevision, ScenarioRevision: hex.EncodeToString(scenarioDigest[:]),
		PolicyRevision: hex.EncodeToString(policyDigest[:]),
	}
}

// WritePaperPlan preflights and paints an already immutable PaperPlan. It
// never reparses, recompiles, measures, wraps, or paginates.
func (f *Document) WritePaperPlan(plan PaperPlan) (PaperRenderResult, error) {
	result := PaperRenderResult{}
	if plan.hash == "" || plan.PageCount() == 0 {
		return paperStageFailureWithSpan(result, PaperStagePaint, "PAPER_PLAN_INVALID",
			errors.New("paper plan is empty"), "create the plan with document.PlanPaper", plan.root)
	}
	projection := plan.plan.Projection()
	needsDisplayPainter := f.tagged.enabled || len(projection.ImageResources) != 0 || layoutPlanHasMultipleGlyphRunsPerLine(projection)
	for _, font := range projection.Fonts {
		if font.EmbeddedUTF8 != nil {
			needsDisplayPainter = true
			break
		}
	}
	for _, command := range projection.Commands {
		if command.Kind != layoutengine.CommandGlyphRun {
			needsDisplayPainter = true
			break
		}
	}
	if needsDisplayPainter {
		prepared, err := f.preflightDisplayLayoutPlanPDFResourcesContextForTarget(context.Background(), plan.plan, plan.imageSources, plan.fontSources, false)
		if err != nil {
			return paperStageFailureWithSpan(result, PaperStagePaint, "PAPER_PAINT_PREFLIGHT", err,
				"render into a fresh document with compatible image limits", plan.root)
		}
		pageStart := f.PageCount()
		if err := f.paintPreparedDisplayLayoutPlanPDF(prepared); err != nil {
			return paperStageFailureWithSpan(result, PaperStagePaint, "PAPER_PAINT_FAILED", err, "", plan.root)
		}
		if plan.title != "" {
			f.SetTitle(plan.title, true)
		}
		if plan.language != "" {
			f.compliance.Lang = plan.language
		}
		result.Pages = f.PageCount() - pageStart
		return result, nil
	}

	// This preflight checks the live target's state, policies, limits, fonts,
	// and the complete paint event recording without installing a resource or
	// opening a page. The painter repeats this read-only validation defensively.
	if _, err := f.preflightCoreLayoutPlanPDF(plan.plan); err != nil {
		return paperStageFailureWithSpan(result, PaperStagePaint, "PAPER_PAINT_PREFLIGHT", err,
			"render into a fresh document with compatible core fonts and page limits", plan.root)
	}

	pageStart := f.PageCount()
	if err := f.paintCoreLayoutPlanPDF(plan.plan); err != nil {
		return paperStageFailureWithSpan(result, PaperStagePaint, "PAPER_PAINT_FAILED", err, "", plan.root)
	}
	if plan.title != "" {
		f.SetTitle(plan.title, true)
	}
	if plan.language != "" {
		f.compliance.Lang = plan.language
	}
	result.Pages = f.PageCount() - pageStart
	return result, nil
}

type paperMeasuredBlock struct {
	explicitBreak     bool
	lines             []layoutengine.ParagraphLineInput
	runs              map[uint32][]layoutengine.CoreGlyphRun
	decorations       map[uint32][]paperMeasuredDecoration
	fontIDs           map[layoutengine.FontResourceID]layoutengine.FontResourceID
	node              layoutengine.NodeID
	key               layoutengine.NodeKey
	instance          layoutengine.InstanceID
	source            layoutengine.SourceSpan
	semanticRole      layoutengine.SemanticRole
	semanticText      string
	headingLevel      uint8
	segments          []layout.TextSegment
	keepTogether      bool
	keepWithNext      bool
	orphans           uint32
	widows            uint32
	keepGroups        []uint32
	image             *paperMeasuredImage
	gridRow           *paperMeasuredGridRow
	canvas            *paperMeasuredCanvas
	semanticAncestors []typedSemanticAncestor
	box               paperMeasuredBox
}

type paperMeasuredDecoration struct {
	path       layoutengine.PlannedPath
	stroke     layoutengine.PlannedStroke
	sourceLine layoutengine.PlannedLine
}

type paperPlanningBlock struct {
	bodyIndex         int
	segmentIndex      int
	nestedIndex       int
	path              string
	explicitBreak     bool
	paragraph         layout.ParagraphBlock
	semanticRole      layoutengine.SemanticRole
	semanticText      string
	headingLevel      uint8
	keepTogether      bool
	keepWithNext      bool
	orphans           uint32
	widows            uint32
	keepGroups        []uint32
	image             *layout.ImageBlock
	gridRow           *paperPlanningGridRow
	canvas            *layout.CanvasBlock
	semanticAncestors []typedSemanticAncestor
	box               layout.BoxStyle
}

type typedSemanticAncestor struct {
	path string
	role layoutengine.SemanticRole
	text string
}

type paperPlanningGridCell struct {
	paragraph                        layout.ParagraphBlock
	image                            *layout.ImageBlock
	path                             string
	semanticText                     string
	semanticRole                     layoutengine.SemanticRole
	requestedWidth                   float64
	segmentIndex                     int
	topInsetPoints                   float64
	topInsetInDocumentUnits          bool
	compactLineHeight                float64
	compactLineHeightInDocumentUnits bool
	artifactOnly                     bool
}

type paperPlanningGridRow struct {
	cells                        []paperPlanningGridCell
	columnCount                  int
	gapPoints                    float64
	gapInDocumentUnits           bool
	minimumHeightPoints          float64
	minimumHeightInDocumentUnits bool
	lineOffsetPoints             float64
	lineOffsetInDocumentUnits    bool
}

type paperMeasuredGridCell struct {
	measurement  paperRowColumnMeasurement
	image        *paperMeasuredImage
	node         layoutengine.NodeID
	semanticText string
	semanticRole layoutengine.SemanticRole
	segments     []layout.TextSegment
	offsetX      layoutengine.Fixed
	width        layoutengine.Fixed
	topInset     layoutengine.Fixed
	artifactOnly bool
}

type paperMeasuredGridRow struct {
	cells       []paperMeasuredGridCell
	trackWidths []layoutengine.Fixed
	gap         layoutengine.Fixed
	height      layoutengine.Fixed
	lineOffset  layoutengine.Fixed
}

type paperBodySelector func(page uint32, base layoutengine.Rect) (layoutengine.Rect, error)

// planPaperTextBlocks measures each source block through the existing exact
// core-font line-shadow bridge, then flows all measured lines through one
// resumable layoutengine plan. It does not paint or mutate either Document.
func (f *Document) planPaperTextBlocks(doc *layout.LayoutDocument) (layoutengine.LayoutPlan, error) {
	return f.planPaperTextBlocksContext(context.Background(), doc)
}

func (f *Document) planPaperTextBlocksMapped(doc *layout.LayoutDocument, mapping papercompile.CompileMapping) (layoutengine.LayoutPlan, error) {
	return f.planPaperTextBlocksMappedContext(context.Background(), doc, mapping)
}

func (f *Document) planPaperTextBlocksContext(ctx context.Context, doc *layout.LayoutDocument) (layoutengine.LayoutPlan, error) {
	return f.planPaperTextBlocksMappedContext(ctx, doc, papercompile.CompileMapping{})
}

func (f *Document) planPaperTextBlocksMappedContext(ctx context.Context, doc *layout.LayoutDocument, mapping papercompile.CompileMapping) (layoutengine.LayoutPlan, error) {
	return f.planPaperTextBlocksMappedBodiesContext(ctx, doc, mapping, nil)
}

func (f *Document) planPaperTextBlocksMappedBodiesContext(ctx context.Context, doc *layout.LayoutDocument, mapping papercompile.CompileMapping, selectBody paperBodySelector) (layoutengine.LayoutPlan, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if doc == nil {
		return layoutengine.LayoutPlan{}, newTypedShadowUnsupported(typedShadowDocumentEnvelope, "layout document is nil")
	}
	blocks := layout.NormalizeBlocks(doc.Body)
	if len(blocks) == 0 {
		return layoutengine.LayoutPlan{}, newTypedShadowUnsupported(typedShadowDocumentEnvelope, "requires at least one text block")
	}
	for bodyIndex, block := range blocks {
		if container, ok := block.(layout.RowColumnBlock); ok {
			if len(blocks) != 1 {
				return layoutengine.LayoutPlan{}, newTypedShadowUnsupported(typedShadowBlockKind, "initial row/column planning requires the container to be the only body block")
			}
			return f.planPaperRowColumnMapped(ctx, doc, mapping, bodyIndex, container, selectBody)
		}
	}

	left, top, right, bottom := typedShadowMargins(f, doc.PageTemplate.Margins)
	contentWidth := f.w - left - right
	bodyHeight := f.h - top - bottom
	if contentWidth <= 0 || bodyHeight <= 0 {
		return layoutengine.LayoutPlan{}, newTypedShadowUnsupported(typedShadowGeometry, "page margins leave no body area")
	}
	pageSize, baseBody, err := typedShadowFixedGeometry(f, left, top, contentWidth, bodyHeight)
	if err != nil {
		return layoutengine.LayoutPlan{}, newTypedShadowUnsupported(typedShadowGeometry, err.Error())
	}
	resolveBody := func(page uint32) (layoutengine.Rect, error) {
		body := baseBody
		if selectBody != nil {
			var selectErr error
			body, selectErr = selectBody(page, baseBody)
			if selectErr != nil {
				return layoutengine.Rect{}, selectErr
			}
		}
		baseBottom, _ := baseBody.Bottom()
		baseRight, _ := baseBody.Right()
		bodyBottom, bottomErr := body.Bottom()
		bodyRight, rightErr := body.Right()
		if bottomErr != nil || rightErr != nil || body.Width <= 0 || body.Height <= 0 ||
			body.X < baseBody.X || bodyRight > baseRight || body.Y < baseBody.Y || bodyBottom > baseBottom {
			return layoutengine.Rect{}, fmt.Errorf("document: page %d selected body region is invalid or outside the page margins", page)
		}
		return body, nil
	}
	body, err := resolveBody(1)
	if err != nil {
		return layoutengine.LayoutPlan{}, err
	}

	if err := ctx.Err(); err != nil {
		return layoutengine.LayoutPlan{}, err
	}
	planningBlocks, err := paperExpandPlanningBlocks(ctx, blocks)
	if err != nil {
		var planning *layoutengine.PlanningError
		if errors.As(err, &planning) {
			return layoutengine.LayoutPlan{}, err
		}
		return layoutengine.LayoutPlan{}, newTypedShadowUnsupported(typedShadowBlockKind, err.Error())
	}
	if len(planningBlocks) > 100_000 {
		return layoutengine.LayoutPlan{}, fmt.Errorf("document: typed planner block limit exceeded: %d > 100000", len(planningBlocks))
	}
	fontIndex := make(map[paperCoreFontIdentity]layoutengine.FontResourceID)
	fonts := make([]layoutengine.CoreFontResource, 0)
	measured := make([]paperMeasuredBlock, 0, len(planningBlocks))
	var measuredLines uint64
	var nextNode layoutengine.NodeID
	fallbackIdentity := len(planningBlocks)
	for index, block := range planningBlocks {
		if err := layoutengine.ChargePlanningWork(ctx, "typed document block planning", 1); err != nil {
			return layoutengine.LayoutPlan{}, err
		}
		if block.explicitBreak {
			measured = append(measured, paperMeasuredBlock{explicitBreak: true})
			continue
		}
		identity := paperBlockIdentity(mapping, block.bodyIndex, block.segmentIndex, block.nestedIndex, index)
		if block.gridRow != nil {
			measuredBox, boxErr := f.paperMeasureBox(block.box, block.path)
			if boxErr != nil {
				return layoutengine.LayoutPlan{}, boxErr
			}
			rowWidth := baseBody.Width
			for _, inset := range []layoutengine.Fixed{measuredBox.style.Margin.Left, measuredBox.style.Border.Left, measuredBox.style.Padding.Left,
				measuredBox.style.Padding.Right, measuredBox.style.Border.Right, measuredBox.style.Margin.Right} {
				rowWidth, boxErr = rowWidth.Sub(inset)
				if boxErr != nil || rowWidth <= 0 {
					return layoutengine.LayoutPlan{}, fmt.Errorf("%s: box horizontal edges leave no grid content width", block.path)
				}
			}
			row, rowErr := f.measurePaperGridRow(ctx, doc, mapping, block, rowWidth, left, top, right, bottom, &nextNode, &fallbackIdentity)
			if rowErr != nil {
				return layoutengine.LayoutPlan{}, rowErr
			}
			for cellIndex := range row.cells {
				measurement := &row.cells[cellIndex].measurement
				localFonts := make(map[layoutengine.FontResourceID]layoutengine.FontResourceID)
				for _, font := range measurement.plan.Fonts {
					localID := font.ID
					fontIdentity := paperFontIdentity(font)
					globalID, exists := fontIndex[fontIdentity]
					if !exists {
						globalID = layoutengine.FontResourceID(len(fonts) + 1) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
						font.ID = globalID
						fonts = append(fonts, font)
						fontIndex[fontIdentity] = globalID
					}
					localFonts[localID] = globalID
				}
				for runIndex := range measurement.plan.GlyphRuns {
					measurement.plan.GlyphRuns[runIndex].Font = localFonts[measurement.plan.GlyphRuns[runIndex].Font]
				}
				measuredLines += uint64(len(measurement.plan.Lines))
			}
			if measuredLines > 1_000_000 {
				return layoutengine.LayoutPlan{}, fmt.Errorf("document: typed planner line limit exceeded: %d > 1000000", measuredLines)
			}
			first := row.cells[0]
			measured = append(measured, paperMeasuredBlock{node: first.node, key: first.measurement.identity.key,
				instance: first.measurement.identity.instance, source: first.measurement.identity.source,
				keepTogether: true, keepWithNext: block.keepWithNext,
				keepGroups: append([]uint32(nil), block.keepGroups...), gridRow: &row,
				semanticAncestors: append([]typedSemanticAncestor(nil), block.semanticAncestors...), box: measuredBox})
			continue
		}
		if block.image != nil {
			nextNode++
			imageBlock := *block.image
			if htmlUnifiedVisualBox(block.box) {
				merged, mergeErr := paperMergeOuterBox(imageBlock.EffectiveBox(), block.box, block.path)
				if mergeErr != nil {
					return layoutengine.LayoutPlan{}, mergeErr
				}
				imageBlock.Box, imageBlock.BoxRef = merged, nil
			}
			image, imageErr := f.measureTypedPlanningImageContext(ctx, imageBlock, contentWidth)
			if imageErr != nil {
				return layoutengine.LayoutPlan{}, fmt.Errorf("%s: %w", block.path, imageErr)
			}
			measured = append(measured, paperMeasuredBlock{
				node: nextNode, key: identity.key, instance: identity.instance,
				source: identity.source, semanticRole: block.semanticRole, semanticText: block.semanticText,
				keepTogether: true, keepWithNext: block.keepWithNext,
				keepGroups: append([]uint32(nil), block.keepGroups...), image: &image,
				semanticAncestors: append([]typedSemanticAncestor(nil), block.semanticAncestors...),
			})
			continue
		}
		if block.canvas != nil {
			nextNode++
			containerNode := nextNode
			canvasMapping := paperCanvasMappingForBody(mapping, block.bodyIndex)
			canvasPlan, canvasErr := f.planPaperCanvas(ctx, doc, canvasMapping, *block.canvas)
			if canvasErr != nil {
				return layoutengine.LayoutPlan{}, fmt.Errorf("%s: %w", block.path, canvasErr)
			}
			projection := canvasPlan.Projection()
			items := make([]paperMeasuredCanvasItem, 0, len(projection.Fragments))
			semanticByFragment := make(map[layoutengine.FragmentID]layoutengine.SemanticNode)
			for _, association := range projection.SemanticFragments {
				semanticByFragment[association.Fragment] = projection.SemanticNodes[association.Semantic-1]
			}
			for index := range projection.Fragments {
				oldNode := projection.Fragments[index].Node
				nextNode++
				projection.Fragments[index].Node = nextNode
				semantic := semanticByFragment[projection.Fragments[index].ID]
				items = append(items, paperMeasuredCanvasItem{node: nextNode, key: projection.Fragments[index].Key,
					instance: projection.Fragments[index].Instance, source: projection.Fragments[index].Source,
					role: semantic.Role, alt: semantic.Attributes.AlternateText})
				for diagnosticIndex := range projection.Diagnostics {
					if projection.Diagnostics[diagnosticIndex].Location.Node == oldNode {
						projection.Diagnostics[diagnosticIndex].Location.Node = nextNode
					}
				}
			}
			identity := paperBlockIdentity(mapping, block.bodyIndex, -1, -1, index)
			if len(projection.Fragments) == 0 {
				return layoutengine.LayoutPlan{}, fmt.Errorf("%s: canvas produced no positioned items", block.path)
			}
			measured = append(measured, paperMeasuredBlock{node: containerNode, key: identity.key, instance: identity.instance,
				source: identity.source, semanticRole: layoutengine.SemanticRoleSection, keepTogether: true,
				semanticAncestors: append([]typedSemanticAncestor(nil), block.semanticAncestors...),
				canvas:            &paperMeasuredCanvas{projection: projection, height: layoutFixedFromDocumentUnits(f, block.canvas.Height), items: items}})
			continue
		}
		measuredBox, boxErr := f.paperMeasureBox(block.box, block.path)
		if boxErr != nil {
			return layoutengine.LayoutPlan{}, boxErr
		}
		single := *doc
		paragraph := block.paragraph
		authoredSegments := append([]layout.TextSegment(nil), paragraph.Segments...)
		mixedCoreShadow := typedParagraphNeedsMixedCoreShadow(paragraph, f)
		if !mixedCoreShadow {
			paragraph.Segments = make([]layout.TextSegment, len(authoredSegments))
			for index, segment := range authoredSegments {
				paragraph.Segments[index] = layout.TextSegment{Text: segment.Text, Link: segment.Link, Destination: segment.Destination}
			}
		}
		paragraph.Box, paragraph.BoxRef = layout.BoxStyle{}, nil
		single.Body = []layout.Block{paragraph}
		boxContentWidth, widthErr := measuredBox.contentWidth(baseBody.Width)
		if widthErr != nil {
			return layoutengine.LayoutPlan{}, fmt.Errorf("%s: %w", block.path, widthErr)
		}
		contentX, widthErr := baseBody.X.Add(measuredBox.style.Margin.Left)
		for _, inset := range []layoutengine.Fixed{measuredBox.style.Border.Left, measuredBox.style.Padding.Left} {
			if widthErr == nil {
				contentX, widthErr = contentX.Add(inset)
			}
		}
		contentRight, rightErr := contentX.Add(boxContentWidth)
		if widthErr != nil || rightErr != nil || contentRight > pageSize.Width {
			return layoutengine.LayoutPlan{}, fmt.Errorf("%s: resolved box content width overflows the page", block.path)
		}
		single.PageTemplate = layout.PageTemplate{Margins: layout.Spacing{
			Left: f.PointConvert(contentX.Points()), Top: top,
			Right: f.w - f.PointConvert(contentRight.Points()), Bottom: bottom,
		}}
		shadow, err := f.planTypedParagraphLineShadowContext(ctx, &single)
		if err != nil {
			path := block.path
			if path == "" {
				path = fmt.Sprintf("body[%d]", block.bodyIndex)
			}
			return layoutengine.LayoutPlan{}, fmt.Errorf("%s: %w", path, err)
		}
		measurement := paperRowColumnMeasurement{plan: shadow.Plan.Projection()}
		if !mixedCoreShadow {
			var restyleErr error
			measurement, restyleErr = f.restylePaperMeasurement(measurement, paragraph.Style, authoredSegments)
			if restyleErr != nil {
				return layoutengine.LayoutPlan{}, fmt.Errorf("%s: %w", block.path, restyleErr)
			}
		}
		projection := measurement.plan
		nextNode++
		blockPlan := paperMeasuredBlock{
			lines:        make([]layoutengine.ParagraphLineInput, len(projection.Lines)),
			runs:         make(map[uint32][]layoutengine.CoreGlyphRun),
			fontIDs:      make(map[layoutengine.FontResourceID]layoutengine.FontResourceID),
			node:         nextNode,
			key:          identity.key,
			instance:     identity.instance,
			source:       identity.source,
			semanticRole: block.semanticRole, semanticText: block.semanticText,
			headingLevel: block.headingLevel,
			segments:     append([]layout.TextSegment(nil), block.paragraph.Segments...),
			keepTogether: block.keepTogether, keepWithNext: block.keepWithNext,
			orphans: block.orphans, widows: block.widows,
			keepGroups:        append([]uint32(nil), block.keepGroups...),
			semanticAncestors: append([]typedSemanticAncestor(nil), block.semanticAncestors...),
			box:               measuredBox,
		}
		measuredLines += uint64(len(projection.Lines))
		if measuredLines > 1_000_000 {
			return layoutengine.LayoutPlan{}, fmt.Errorf("document: typed planner line limit exceeded: %d > 1000000", measuredLines)
		}
		for _, font := range projection.Fonts {
			localID := font.ID
			identity := paperFontIdentity(font)
			globalID, exists := fontIndex[identity]
			if !exists {
				globalID = layoutengine.FontResourceID(len(fonts) + 1) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
				font.ID = globalID
				fonts = append(fonts, font)
				fontIndex[identity] = globalID
			}
			blockPlan.fontIDs[localID] = globalID
		}
		for lineIndex, line := range projection.Lines {
			offset, err := line.Bounds.X.Sub(body.X)
			if err != nil {
				return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] line %d x offset: %w", block.bodyIndex, lineIndex, err)
			}
			baseline, err := line.Baseline.Sub(line.Bounds.Y)
			if err != nil {
				return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] line %d baseline: %w", block.bodyIndex, lineIndex, err)
			}
			blockPlan.lines[lineIndex] = layoutengine.ParagraphLineInput{
				OffsetX: offset, Width: line.Bounds.Width, Height: line.Bounds.Height,
				Baseline: baseline, Source: identity.source,
			}
		}
		for _, run := range projection.GlyphRuns {
			run.Origin.X, err = run.Origin.X.Sub(projection.Lines[run.Line].Bounds.X)
			if err != nil {
				return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] glyph run offset: %w", block.bodyIndex, err)
			}
			run.Origin.Y = 0
			blockPlan.runs[run.Line] = append(blockPlan.runs[run.Line], run)
		}
		if len(projection.Strokes) != 0 {
			blockPlan.decorations = make(map[uint32][]paperMeasuredDecoration)
			for _, stroke := range projection.Strokes {
				if uint64(stroke.Path) >= uint64(len(projection.Paths)) {
					return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] decoration references a missing path", block.bodyIndex)
				}
				path := projection.Paths[stroke.Path]
				lineIndex := 0
				found := false
				bestDistance := int64(^uint64(0) >> 1)
				for candidate, line := range projection.Lines {
					bottom, bottomErr := line.Bounds.Bottom()
					if bottomErr != nil {
						return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] decoration line bounds: %w", block.bodyIndex, bottomErr)
					}
					delta := int64(path.Bounds.Y - line.Baseline)
					if delta < 0 {
						delta = -delta
					}
					if delta < bestDistance {
						bestDistance = delta
						lineIndex = candidate
					}
					if path.Bounds.Y >= line.Bounds.Y && path.Bounds.Y <= bottom {
						lineIndex, found = candidate, true
						break
					}
				}
				if !found && len(projection.Lines) == 0 {
					return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] decoration has no owning line", block.bodyIndex)
				}
				blockPlan.decorations[uint32(lineIndex)] = append(blockPlan.decorations[uint32(lineIndex)], paperMeasuredDecoration{
					path: path, stroke: stroke, sourceLine: projection.Lines[lineIndex],
				})
			}
		}
		measured = append(measured, blockPlan)
	}

	geometry := layoutengine.LayoutPlanInput{}
	var gridGroup uint32
	runs := make([]layoutengine.CoreGlyphRun, 0)
	imageResources := make([]layoutengine.ImageResource, 0)
	images := make([]layoutengine.PlannedImage, 0)
	imageResourceIDs := make(map[layoutengine.ImageContentDigest]layoutengine.ImageResourceID)
	paths := make([]layoutengine.PlannedPath, 0)
	fills := make([]layoutengine.PlannedFill, 0)
	strokes := make([]layoutengine.PlannedStroke, 0)
	displayItems := make([]layoutengine.DisplayItem, 0)
	appendMeasuredDecorations := func(block paperMeasuredBlock, sourceLine uint32, line layoutengine.PlannedLine, fragmentID layoutengine.FragmentID) error {
		for _, decoration := range block.decorations[sourceLine] {
			dx, dxErr := line.Bounds.X.Sub(decoration.sourceLine.Bounds.X)
			dy, dyErr := line.Bounds.Y.Sub(decoration.sourceLine.Bounds.Y)
			if dxErr != nil || dyErr != nil {
				return fmt.Errorf("decoration translation offset: %w %w", dxErr, dyErr)
			}
			path, pathErr := translatePaperNestedPath(decoration.path, dx, dy)
			if pathErr != nil {
				return pathErr
			}
			paths = append(paths, path)
			stroke := decoration.stroke
			stroke.Path = uint32(len(paths) - 1) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
			stroke.Fragment = fragmentID
			strokes = append(strokes, stroke)
			displayItems = append(displayItems, layoutengine.DisplayItem{Kind: layoutengine.CommandStrokePath, Payload: uint32(len(strokes) - 1)}) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
		}
		return nil
	}
	explicitDecorationBoxes := make(map[layoutengine.NodeID]layoutengine.Rect)
	pageNumber := uint32(1)
	cursorY := body.Y
	available := body.Height
	regionEmpty := true
	type pendingPaperBreak struct {
		reason    layoutengine.BreakReason
		fromPage  uint32
		preceding layoutengine.FragmentID
		required  layoutengine.Fixed
		available layoutengine.Fixed
	}
	var pendingBreak *pendingPaperBreak
	explicitBreakPending := false
	diagnostics := make([]layoutengine.Diagnostic, 0)
	type paperKeepRange struct{ first, last int }
	keepRanges := make(map[uint32]paperKeepRange)
	for index, block := range measured {
		for _, group := range block.keepGroups {
			rangeValue, exists := keepRanges[group]
			if !exists {
				rangeValue = paperKeepRange{first: index, last: index}
			}
			rangeValue.last = index
			keepRanges[group] = rangeValue
		}
	}
	blockHeight := func(block paperMeasuredBlock) (layoutengine.Fixed, error) {
		if block.canvas != nil {
			return block.canvas.height, nil
		}
		if block.gridRow != nil {
			return block.box.outerHeight(block.gridRow.height)
		}
		if block.image != nil {
			return block.image.height, nil
		}
		height, err := block.box.contentHeight(block.lines)
		if err != nil {
			return 0, err
		}
		return block.box.outerHeight(height)
	}
	advancePage := func() error {
		if f.limits.MaxPages > 0 && int(pageNumber) >= f.limits.MaxPages {
			return fmt.Errorf("%w: typed plan requires more than %d pages", ErrPageLimitExceeded, f.limits.MaxPages)
		}
		pageNumber++
		return nil
	}
	advanceBodyPage := func() error {
		if err := advancePage(); err != nil {
			return err
		}
		var err error
		body, err = resolveBody(pageNumber)
		if err != nil {
			return err
		}
		cursorY, available, regionEmpty = body.Y, body.Height, true
		return nil
	}
	startBodyPage := func() {
		geometry.Pages = append(geometry.Pages, layoutengine.PlannedPage{Number: pageNumber, Size: pageSize,
			Fragments: layoutengine.IndexRange{Start: uint32(len(geometry.Fragments))}, Lines: layoutengine.IndexRange{Start: uint32(len(geometry.Lines))}}) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
		geometry.PageRegions = append(geometry.PageRegions, layoutengine.PlannedPageRegion{Page: pageNumber, Region: layoutengine.RegionBody, Bounds: body})
	}
	for blockIndex, block := range measured {
		if err := layoutengine.ChargePlanningWork(ctx, "typed document pagination", 1); err != nil {
			return layoutengine.LayoutPlan{}, err
		}
		if block.explicitBreak {
			// A page-break is a source-order boundary, not a blank-page
			// command. Leading, trailing, and repeated markers therefore do
			// not synthesize empty pages; one pending marker is consumed only
			// when later content exists to receive it.
			if !regionEmpty {
				explicitBreakPending = true
			}
			continue
		}
		if block.canvas != nil {
			if explicitBreakPending {
				pendingBreak = &pendingPaperBreak{reason: layoutengine.BreakExplicitPageBreak, fromPage: pageNumber,
					preceding: geometry.Fragments[len(geometry.Fragments)-1].ID}
				if err := advanceBodyPage(); err != nil {
					return layoutengine.LayoutPlan{}, err
				}
				explicitBreakPending = false
			}
			if block.canvas.height > body.Height {
				return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] canvas is taller than an empty body page", blockIndex)
			}
			if block.canvas.height > available && !regionEmpty {
				pendingBreak = &pendingPaperBreak{reason: layoutengine.BreakInsufficientRemainingBodySpace, fromPage: pageNumber,
					preceding: geometry.Fragments[len(geometry.Fragments)-1].ID, required: block.canvas.height, available: available}
				if err := advanceBodyPage(); err != nil {
					return layoutengine.LayoutPlan{}, err
				}
			}
			if regionEmpty {
				startBodyPage()
			}
			page := &geometry.Pages[len(geometry.Pages)-1]
			dy, moveErr := cursorY.Sub(body.Y)
			if moveErr != nil {
				return layoutengine.LayoutPlan{}, moveErr
			}
			fragmentMap := make(map[layoutengine.FragmentID]layoutengine.FragmentID, len(block.canvas.projection.Fragments))
			for _, local := range block.canvas.projection.Fragments {
				oldID := local.ID
				local.ID = layoutengine.FragmentID(len(geometry.Fragments) + 1) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
				local.Page, local.Region = pageNumber, layoutengine.RegionBody
				local.MarginBox, moveErr = translateTypedRect(local.MarginBox, 0, dy)
				if moveErr == nil {
					local.BorderBox, moveErr = translateTypedRect(local.BorderBox, 0, dy)
				}
				if moveErr == nil {
					local.PaddingBox, moveErr = translateTypedRect(local.PaddingBox, 0, dy)
				}
				if moveErr == nil {
					local.ContentBox, moveErr = translateTypedRect(local.ContentBox, 0, dy)
				}
				if moveErr != nil {
					return layoutengine.LayoutPlan{}, moveErr
				}
				geometry.Fragments = append(geometry.Fragments, local)
				fragmentMap[oldID] = local.ID
				page.Fragments.Count++
			}
			if pendingBreak != nil && len(block.canvas.projection.Fragments) != 0 {
				geometry.Breaks = append(geometry.Breaks, layoutengine.BreakDecision{Reason: pendingBreak.reason, FromPage: pendingBreak.fromPage,
					ToPage: pageNumber, Region: layoutengine.RegionBody, Preceding: pendingBreak.preceding,
					Triggering: fragmentMap[block.canvas.projection.Fragments[0].ID], Required: pendingBreak.required, Available: pendingBreak.available})
				pendingBreak = nil
			}
			for _, command := range block.canvas.projection.Commands {
				newFragment := fragmentMap[command.Fragment]
				switch command.Kind {
				case layoutengine.CommandFillPath:
					fill := block.canvas.projection.Fills[command.Payload]
					path, pathErr := translatePaperNestedPath(block.canvas.projection.Paths[fill.Path], 0, dy)
					if pathErr != nil {
						return layoutengine.LayoutPlan{}, pathErr
					}
					paths = append(paths, path)
					fill.Path, fill.Fragment = uint32(len(paths)-1), newFragment // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
					fills = append(fills, fill)
					displayItems = append(displayItems, layoutengine.DisplayItem{Kind: command.Kind, Payload: uint32(len(fills) - 1)}) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
				case layoutengine.CommandStrokePath:
					stroke := block.canvas.projection.Strokes[command.Payload]
					path, pathErr := translatePaperNestedPath(block.canvas.projection.Paths[stroke.Path], 0, dy)
					if pathErr != nil {
						return layoutengine.LayoutPlan{}, pathErr
					}
					paths = append(paths, path)
					stroke.Path, stroke.Fragment = uint32(len(paths)-1), newFragment // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
					strokes = append(strokes, stroke)
					displayItems = append(displayItems, layoutengine.DisplayItem{Kind: command.Kind, Payload: uint32(len(strokes) - 1)}) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
				}
			}
			for _, diagnostic := range block.canvas.projection.Diagnostics {
				diagnostic.Location.Page, diagnostic.Location.Region = pageNumber, layoutengine.RegionBody
				if diagnostic.Location.Fragment.Valid() {
					diagnostic.Location.Fragment = fragmentMap[diagnostic.Location.Fragment]
				}
				if diagnostic.Location.HasBounds {
					diagnostic.Location.Bounds, moveErr = translateTypedRect(diagnostic.Location.Bounds, 0, dy)
					if moveErr != nil {
						return layoutengine.LayoutPlan{}, moveErr
					}
				}
				diagnostics = append(diagnostics, diagnostic)
			}
			cursorY, err = cursorY.Add(block.canvas.height)
			if err != nil {
				return layoutengine.LayoutPlan{}, err
			}
			available, err = available.Sub(block.canvas.height)
			if err != nil {
				return layoutengine.LayoutPlan{}, err
			}
			regionEmpty = false
			continue
		}
		if explicitBreakPending {
			pendingBreak = &pendingPaperBreak{
				reason: layoutengine.BreakExplicitPageBreak, fromPage: pageNumber,
				preceding: geometry.Fragments[len(geometry.Fragments)-1].ID,
			}
			if err := advanceBodyPage(); err != nil {
				return layoutengine.LayoutPlan{}, err
			}
			explicitBreakPending = false
		}
		nextBody, err := resolveBody(pageNumber + 1)
		if err != nil {
			return layoutengine.LayoutPlan{}, err
		}
		constraintEnd := blockIndex
		constrained := block.keepTogether || block.keepWithNext
		for _, group := range block.keepGroups {
			rangeValue := keepRanges[group]
			if rangeValue.first == blockIndex {
				constrained = true
				if rangeValue.last > constraintEnd {
					constraintEnd = rangeValue.last
				}
			}
		}
		for constraintEnd < len(measured)-1 && measured[constraintEnd].keepWithNext && !measured[constraintEnd+1].explicitBreak {
			constrained = true
			constraintEnd++
		}
		if constrained {
			var required layoutengine.Fixed
			for index := blockIndex; index <= constraintEnd; index++ {
				height, err := blockHeight(measured[index])
				if err != nil {
					return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] keep height: %w", blockIndex, err)
				}
				required, err = required.Add(height)
				if err != nil {
					return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] keep height: %w", blockIndex, err)
				}
			}
			keepCapacity := body.Height
			if nextBody.Height > keepCapacity {
				keepCapacity = nextBody.Height
			}
			if required > keepCapacity {
				diagnostics = append(diagnostics, layoutengine.Diagnostic{
					Code: layoutengine.DiagnosticKeepTooLarge, Severity: layoutengine.SeverityWarning, Stage: layoutengine.StageLayout,
					Message: "preferred keep constraint exceeds an empty body and was relaxed to guarantee progress",
					Location: layoutengine.DiagnosticLocation{Node: block.node, Key: block.key, Instance: block.instance,
						Source: block.source, Page: pageNumber, Region: layoutengine.RegionBody},
					Evidence: []layoutengine.DiagnosticEvidence{
						{Key: "body_height", Value: fmt.Sprint(keepCapacity)},
						{Key: "required_height", Value: fmt.Sprint(required)},
					},
				})
			} else if required > available && !regionEmpty {
				pendingBreak = &pendingPaperBreak{reason: layoutengine.BreakPaginationConstraint, fromPage: pageNumber,
					preceding: geometry.Fragments[len(geometry.Fragments)-1].ID, required: required, available: available}
				if err := advanceBodyPage(); err != nil {
					return layoutengine.LayoutPlan{}, err
				}
			}
		}
		if block.gridRow != nil {
			row := block.gridRow
			rowOuterHeight, heightErr := block.box.outerHeight(row.height)
			if heightErr != nil {
				return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] grid row box height: %w", blockIndex, heightErr)
			}
			if rowOuterHeight > body.Height {
				return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] grid row is taller than an empty body page", blockIndex)
			}
			if rowOuterHeight > available && !regionEmpty {
				pendingBreak = &pendingPaperBreak{reason: layoutengine.BreakInsufficientRemainingBodySpace, fromPage: pageNumber,
					preceding: geometry.Fragments[len(geometry.Fragments)-1].ID, required: rowOuterHeight, available: available}
				if err := advanceBodyPage(); err != nil {
					return layoutengine.LayoutPlan{}, err
				}
			}
			if regionEmpty {
				startBodyPage()
			}
			page := &geometry.Pages[len(geometry.Pages)-1]
			contentX, contentY := body.X, cursorY
			if block.box.visual {
				borderX, addErr := body.X.Add(block.box.style.Margin.Left)
				borderY, addYErr := cursorY.Add(block.box.style.Margin.Top)
				borderWidth, widthErr := body.Width.Sub(block.box.style.Margin.Left)
				if widthErr == nil {
					borderWidth, widthErr = borderWidth.Sub(block.box.style.Margin.Right)
				}
				borderHeight, hErr := rowOuterHeight.Sub(block.box.style.Margin.Top)
				if hErr == nil {
					borderHeight, hErr = borderHeight.Sub(block.box.style.Margin.Bottom)
				}
				if addErr != nil || addYErr != nil || widthErr != nil || hErr != nil || borderWidth <= 0 || borderHeight <= 0 {
					return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] grid row box is invalid", blockIndex)
				}
				borderBox, rectErr := layoutengine.NewRect(borderX, borderY, borderWidth, borderHeight)
				if rectErr != nil {
					return layoutengine.LayoutPlan{}, rectErr
				}
				explicitDecorationBoxes[block.node] = borderBox
				contentX, addErr = borderX.Add(block.box.style.Border.Left)
				if addErr == nil {
					contentX, addErr = contentX.Add(block.box.style.Padding.Left)
				}
				contentY, addYErr = borderY.Add(block.box.style.Border.Top)
				if addYErr == nil {
					contentY, addYErr = contentY.Add(block.box.style.Padding.Top)
				}
				if addErr != nil || addYErr != nil {
					return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] grid row content origin is invalid", blockIndex)
				}
			}
			gridGroup++
			trackX := contentX
			for trackIndex, trackWidth := range row.trackWidths {
				trackBox, trackErr := layoutengine.NewRect(trackX, contentY, trackWidth, row.height)
				if trackErr != nil {
					return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] grid column track %d: %w", blockIndex, trackIndex, trackErr)
				}
				gapAfter := layoutengine.Fixed(0)
				if trackIndex+1 < len(row.trackWidths) {
					gapAfter = row.gap
				}
				geometry.GridTracks = append(geometry.GridTracks, layoutengine.PlannedGridTrack{Group: gridGroup, Page: pageNumber,
					Region: layoutengine.RegionBody, Axis: layoutengine.GridTrackColumn, Index: uint32(trackIndex), Bounds: trackBox, GapAfter: gapAfter})
				trackX, err = trackX.Add(trackWidth)
				if err == nil && gapAfter > 0 {
					trackX, err = trackX.Add(gapAfter)
				}
				if err != nil {
					return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] grid column track %d overflows", blockIndex, trackIndex)
				}
			}
			rowWidth, rowWidthErr := trackX.Sub(contentX)
			rowTrackBox, rowTrackErr := layoutengine.NewRect(contentX, contentY, rowWidth, row.height)
			if rowWidthErr != nil || rowTrackErr != nil {
				return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] grid row track is invalid", blockIndex)
			}
			geometry.GridTracks = append(geometry.GridTracks, layoutengine.PlannedGridTrack{Group: gridGroup, Page: pageNumber,
				Region: layoutengine.RegionBody, Axis: layoutengine.GridTrackRow, Bounds: rowTrackBox})
			for cellIndex, cell := range row.cells {
				x, err := contentX.Add(cell.offsetX)
				if err != nil {
					return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] cell %d x: %w", blockIndex, cellIndex, err)
				}
				box, err := layoutengine.NewRect(x, contentY, cell.width, row.height)
				if err != nil {
					return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] cell %d box: %w", blockIndex, cellIndex, err)
				}
				fragmentID := layoutengine.FragmentID(len(geometry.Fragments) + 1) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
				identity := cell.measurement.identity
				geometry.Fragments = append(geometry.Fragments, layoutengine.Fragment{ID: fragmentID, Node: cell.node,
					Key: identity.key, Instance: identity.instance, Page: pageNumber, Region: layoutengine.RegionBody,
					BorderBox: box, ContentBox: box, Continuation: layoutengine.ContinuationWhole, Source: identity.source})
				page.Fragments.Count++
				if pendingBreak != nil {
					geometry.Breaks = append(geometry.Breaks, layoutengine.BreakDecision{Reason: pendingBreak.reason,
						FromPage: pendingBreak.fromPage, ToPage: pageNumber, Region: layoutengine.RegionBody,
						Preceding: pendingBreak.preceding, Triggering: fragmentID, Required: pendingBreak.required, Available: pendingBreak.available})
					pendingBreak = nil
				}
				if row.lineOffset > 0 {
					lineY, lineErr := contentY.Add(row.lineOffset)
					rightX, rightErr := x.Add(cell.width)
					if lineErr != nil || rightErr != nil {
						return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] cell %d signature line overflows", blockIndex, cellIndex)
					}
					lineBounds, boundsErr := layoutengine.RectFromPoints(layoutengine.Point{X: x, Y: lineY}, layoutengine.Point{X: rightX, Y: lineY})
					if boundsErr != nil {
						return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] cell %d signature line: %w", blockIndex, cellIndex, boundsErr)
					}
					paths = append(paths, layoutengine.PlannedPath{Bounds: lineBounds, Segments: []layoutengine.PathSegment{
						{Kind: layoutengine.PathMoveTo, Point: layoutengine.Point{X: x, Y: lineY}},
						{Kind: layoutengine.PathLineTo, Point: layoutengine.Point{X: rightX, Y: lineY}},
					}})
					strokeWidth, _ := layoutengine.FixedFromPoints(0.567)
					strokes = append(strokes, layoutengine.PlannedStroke{Path: uint32(len(paths) - 1), // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
						Color: layoutengine.CoreRGBColor{Set: true}, Width: strokeWidth, Fragment: fragmentID})
					displayItems = append(displayItems, layoutengine.DisplayItem{Kind: layoutengine.CommandStrokePath, Payload: uint32(len(strokes) - 1)}) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
				}
				if cell.image != nil {
					imageBox, imageErr := layoutengine.NewRect(x, contentY, cell.image.width, cell.image.height)
					if imageErr != nil {
						return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] cell %d image box: %w", blockIndex, cellIndex, imageErr)
					}
					resourceID, exists := imageResourceIDs[cell.image.resource.Digest]
					if !exists {
						resourceID = layoutengine.ImageResourceID(len(imageResources) + 1) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
						resource := cell.image.resource
						resource.ID = resourceID
						imageResources = append(imageResources, resource)
						imageResourceIDs[resource.Digest] = resourceID
					}
					cell.image.resource.ID = resourceID
					placement, imageErr := cell.image.placement(fragmentID, imageBox)
					if imageErr != nil {
						return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] cell %d image placement: %w", blockIndex, cellIndex, imageErr)
					}
					images = append(images, placement)
					displayItems = append(displayItems, layoutengine.DisplayItem{Kind: layoutengine.CommandImage, Payload: uint32(len(images) - 1)}) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
				}
				lineMap := make(map[uint32]uint32, len(cell.measurement.plan.Lines))
				for localIndex, line := range cell.measurement.plan.Lines {
					xOffset, xErr := line.Bounds.X.Sub(cell.measurement.body.X)
					yOffset, yErr := line.Bounds.Y.Sub(cell.measurement.body.Y)
					if xErr != nil || yErr != nil {
						return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] cell %d line offset is invalid", blockIndex, cellIndex)
					}
					lineX, xErr := x.Add(xOffset)
					lineY, yErr := contentY.Add(cell.topInset)
					if yErr == nil {
						lineY, yErr = lineY.Add(yOffset)
					}
					if xErr != nil || yErr != nil {
						return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] cell %d line position overflows", blockIndex, cellIndex)
					}
					bounds, boundsErr := layoutengine.NewRect(lineX, lineY, line.Bounds.Width, line.Bounds.Height)
					baselineOffset, baselineErr := line.Baseline.Sub(line.Bounds.Y)
					baseline, addErr := lineY.Add(baselineOffset)
					if boundsErr != nil || baselineErr != nil || addErr != nil {
						return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] cell %d line geometry is invalid", blockIndex, cellIndex)
					}
					globalLine := uint32(len(geometry.Lines)) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
					geometry.Lines = append(geometry.Lines, layoutengine.PlannedLine{Fragment: fragmentID, Index: uint32(localIndex), Bounds: bounds, Baseline: baseline, Source: identity.source})
					page.Lines.Count++
					lineMap[uint32(localIndex)] = globalLine
				}
				for _, run := range cell.measurement.plan.GlyphRuns {
					run.Line = lineMap[run.Line]
					line := geometry.Lines[run.Line]
					run.Origin = layoutengine.Point{X: line.Bounds.X, Y: line.Baseline}
					run.Source = identity.source
					runs = append(runs, run)
					displayItems = append(displayItems, layoutengine.DisplayItem{Kind: layoutengine.CommandGlyphRun, Payload: uint32(len(runs) - 1)}) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
				}
			}
			cursorY, err = cursorY.Add(rowOuterHeight)
			if err != nil {
				return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] grid row cursor: %w", blockIndex, err)
			}
			available, err = available.Sub(rowOuterHeight)
			if err != nil {
				return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] grid row remaining height: %w", blockIndex, err)
			}
			regionEmpty = false
			continue
		}
		if block.image != nil {
			if block.image.height > body.Height {
				return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] image is taller than an empty body page", blockIndex)
			}
			if block.image.height > available && !regionEmpty {
				pendingBreak = &pendingPaperBreak{
					reason: layoutengine.BreakInsufficientRemainingBodySpace, fromPage: pageNumber,
					preceding: geometry.Fragments[len(geometry.Fragments)-1].ID,
					required:  block.image.height, available: available,
				}
				if err := advanceBodyPage(); err != nil {
					return layoutengine.LayoutPlan{}, err
				}
			}
			if regionEmpty {
				startBodyPage()
			}
			page := &geometry.Pages[len(geometry.Pages)-1]
			x, err := block.image.targetX(body)
			if err != nil {
				return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] image x: %w", blockIndex, err)
			}
			box, err := layoutengine.NewRect(x, cursorY, block.image.width, block.image.height)
			if err != nil {
				return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] image box: %w", blockIndex, err)
			}
			fragmentID := layoutengine.FragmentID(len(geometry.Fragments) + 1) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
			contentBox, err := block.image.contentBox(box)
			if err != nil {
				return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] image content box: %w", blockIndex, err)
			}
			geometry.Fragments = append(geometry.Fragments, layoutengine.Fragment{
				ID: fragmentID, Node: block.node, Key: block.key, Instance: block.instance,
				Page: pageNumber, Region: layoutengine.RegionBody, BorderBox: box, ContentBox: contentBox,
				Continuation: layoutengine.ContinuationWhole, Source: block.source,
			})
			page.Fragments.Count++
			if pendingBreak != nil {
				geometry.Breaks = append(geometry.Breaks, layoutengine.BreakDecision{
					Reason: pendingBreak.reason, FromPage: pendingBreak.fromPage, ToPage: pageNumber,
					Region: layoutengine.RegionBody, Preceding: pendingBreak.preceding, Triggering: fragmentID,
					Required: pendingBreak.required, Available: pendingBreak.available,
				})
				pendingBreak = nil
			}
			if block.image.background.Set {
				paths = append(paths, typedTableRectPath(box))
				fills = append(fills, layoutengine.PlannedFill{Path: uint32(len(paths) - 1), Rule: layoutengine.FillNonZero, Color: block.image.background, Fragment: fragmentID}) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
				displayItems = append(displayItems, layoutengine.DisplayItem{Kind: layoutengine.CommandFillPath, Payload: uint32(len(fills) - 1)})                                 // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
			}
			resourceID, exists := imageResourceIDs[block.image.resource.Digest]
			if !exists {
				resourceID = layoutengine.ImageResourceID(len(imageResources) + 1) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
				resource := block.image.resource
				resource.ID = resourceID
				imageResources = append(imageResources, resource)
				imageResourceIDs[resource.Digest] = resourceID
			}
			block.image.resource.ID = resourceID
			placement, err := block.image.placement(fragmentID, box)
			if err != nil {
				return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] image placement: %w", blockIndex, err)
			}
			placement.Source = block.source
			images = append(images, placement)
			displayItems = append(displayItems, layoutengine.DisplayItem{Kind: layoutengine.CommandImage, Payload: uint32(len(images) - 1)}) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
			for side, border := range block.image.borders {
				if border.width <= 0 {
					continue
				}
				path, pathErr := typedTableBorderPath(box, side)
				if pathErr != nil {
					return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] image border: %w", blockIndex, pathErr)
				}
				paths = append(paths, path)
				strokes = append(strokes, layoutengine.PlannedStroke{Path: uint32(len(paths) - 1), Color: border.color, Width: border.width, Fragment: fragmentID}) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
				displayItems = append(displayItems, layoutengine.DisplayItem{Kind: layoutengine.CommandStrokePath, Payload: uint32(len(strokes) - 1)})              // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
			}
			cursorY, err = cursorY.Add(block.image.height)
			if err != nil {
				return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] image cursor: %w", blockIndex, err)
			}
			available, err = available.Sub(block.image.height)
			if err != nil {
				return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] image remaining height: %w", blockIndex, err)
			}
			regionEmpty = false
			continue
		}
		if block.box.visual {
			contentHeight, heightErr := block.box.contentHeight(block.lines)
			if heightErr != nil {
				return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] box content height: %w", blockIndex, heightErr)
			}
			outerHeight, heightErr := block.box.outerHeight(contentHeight)
			if heightErr != nil {
				return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] box outer height: %w", blockIndex, heightErr)
			}
			if outerHeight > body.Height {
				return layoutengine.LayoutPlan{}, newTypedShadowUnsupported(typedShadowGeometry, fmt.Sprintf("body[%d] decorated box is taller than an empty body page", blockIndex))
			}
			if outerHeight > available && !regionEmpty {
				pendingBreak = &pendingPaperBreak{
					reason: layoutengine.BreakInsufficientRemainingBodySpace, fromPage: pageNumber,
					preceding: geometry.Fragments[len(geometry.Fragments)-1].ID, required: outerHeight, available: available,
				}
				if err := advanceBodyPage(); err != nil {
					return layoutengine.LayoutPlan{}, err
				}
			}
			if regionEmpty {
				startBodyPage()
			}
			page := &geometry.Pages[len(geometry.Pages)-1]
			borderX, boxErr := body.X.Add(block.box.style.Margin.Left)
			if boxErr != nil {
				return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] box x: %w", blockIndex, boxErr)
			}
			borderY, boxErr := cursorY.Add(block.box.style.Margin.Top)
			if boxErr != nil {
				return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] box y: %w", blockIndex, boxErr)
			}
			borderWidth, boxErr := block.box.borderWidth(body.Width)
			borderHeight, boxErr2 := outerHeight.Sub(block.box.style.Margin.Top)
			if boxErr2 == nil {
				borderHeight, boxErr2 = borderHeight.Sub(block.box.style.Margin.Bottom)
			}
			if boxErr != nil || boxErr2 != nil || borderWidth < 0 || borderHeight < 0 {
				return layoutengine.LayoutPlan{}, newTypedShadowUnsupported(typedShadowGeometry, fmt.Sprintf("body[%d] decorated box dimensions are invalid", blockIndex))
			}
			borderBox, boxErr := layoutengine.NewRect(borderX, borderY, borderWidth, borderHeight)
			if boxErr != nil {
				return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] border box: %w", blockIndex, boxErr)
			}
			marginBox, boxErr := layoutengine.NewRect(body.X, cursorY, body.Width, outerHeight)
			if boxErr != nil {
				return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] margin box: %w", blockIndex, boxErr)
			}
			paddingBox, boxErr := borderBox.Inset(block.box.style.Border)
			if boxErr != nil {
				return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] padding box: %w", blockIndex, boxErr)
			}
			contentX, boxErr := borderX.Add(block.box.style.Border.Left)
			if boxErr == nil {
				contentX, boxErr = contentX.Add(block.box.style.Padding.Left)
			}
			contentY, boxErr2 := borderY.Add(block.box.style.Border.Top)
			if boxErr2 == nil {
				contentY, boxErr2 = contentY.Add(block.box.style.Padding.Top)
			}
			contentWidth, widthErr := block.box.contentWidth(body.Width)
			contentBoxHeight, contentHeightErr := block.box.contentBoxHeight(contentHeight)
			if boxErr != nil || boxErr2 != nil || widthErr != nil || contentWidth < 0 {
				return layoutengine.LayoutPlan{}, newTypedShadowUnsupported(typedShadowGeometry, fmt.Sprintf("body[%d] decorated content box is invalid", blockIndex))
			}
			if contentHeightErr != nil {
				return layoutengine.LayoutPlan{}, newTypedShadowUnsupported(typedShadowGeometry, fmt.Sprintf("body[%d] decorated content height is invalid", blockIndex))
			}
			contentBox, boxErr := layoutengine.NewRect(contentX, contentY, contentWidth, contentBoxHeight)
			if boxErr != nil {
				return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] content box: %w", blockIndex, boxErr)
			}
			fragmentID := layoutengine.FragmentID(len(geometry.Fragments) + 1) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
			geometry.Fragments = append(geometry.Fragments, layoutengine.Fragment{
				ID: fragmentID, Node: block.node, Key: block.key, Instance: block.instance,
				Page: pageNumber, Region: layoutengine.RegionBody, MarginBox: marginBox, BorderBox: borderBox,
				PaddingBox: paddingBox, ContentBox: contentBox,
				Continuation: layoutengine.ContinuationWhole, Source: block.source,
			})
			page.Fragments.Count++
			if pendingBreak != nil {
				geometry.Breaks = append(geometry.Breaks, layoutengine.BreakDecision{
					Reason: pendingBreak.reason, FromPage: pendingBreak.fromPage, ToPage: pageNumber,
					Region: layoutengine.RegionBody, Preceding: pendingBreak.preceding, Triggering: fragmentID,
					Required: pendingBreak.required, Available: pendingBreak.available,
				})
				pendingBreak = nil
			}
			lineY := contentY
			for sourceLine, lineInput := range block.lines {
				x, lineErr := body.X.Add(lineInput.OffsetX)
				if lineErr != nil {
					return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] box line %d x: %w", blockIndex, sourceLine, lineErr)
				}
				bounds, lineErr := layoutengine.NewRect(x, lineY, lineInput.Width, lineInput.Height)
				baseline, baselineErr := lineY.Add(lineInput.Baseline)
				if lineErr != nil || baselineErr != nil {
					return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] box line %d geometry is invalid", blockIndex, sourceLine)
				}
				globalLine := uint32(len(geometry.Lines)) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
				geometry.Lines = append(geometry.Lines, layoutengine.PlannedLine{
					Fragment: fragmentID, Index: uint32(sourceLine), Bounds: bounds, Baseline: baseline, Source: lineInput.Source,
				})
				page.Lines.Count++
				for _, localRun := range block.runs[uint32(sourceLine)] {
					localRun.Line = globalLine
					localRun.Font = block.fontIDs[localRun.Font]
					localRun.Origin.X, err = bounds.X.Add(localRun.Origin.X)
					if err != nil {
						return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] box glyph run x: %w", blockIndex, err)
					}
					localRun.Origin.Y = baseline
					localRun.Source = block.source
					runs = append(runs, localRun)
					displayItems = append(displayItems, layoutengine.DisplayItem{Kind: layoutengine.CommandGlyphRun, Payload: uint32(len(runs) - 1)}) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
				}
				if err := appendMeasuredDecorations(block, uint32(sourceLine), layoutengine.PlannedLine{Fragment: fragmentID, Index: uint32(sourceLine), Bounds: bounds, Baseline: baseline, Source: lineInput.Source}, fragmentID); err != nil {
					return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] box line %d decoration: %w", blockIndex, sourceLine, err)
				}
				lineY, lineErr = lineY.Add(lineInput.Height)
				if lineErr != nil {
					return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] box line %d cursor: %w", blockIndex, sourceLine, lineErr)
				}
			}
			cursorY, boxErr = cursorY.Add(outerHeight)
			if boxErr != nil {
				return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] box cursor: %w", blockIndex, boxErr)
			}
			available, boxErr = available.Sub(outerHeight)
			if boxErr != nil {
				return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] box remaining height: %w", blockIndex, boxErr)
			}
			regionEmpty = false
			continue
		}
		orphans, widows := block.orphans, block.widows
		if orphans == 0 {
			orphans = 1
		}
		if widows == 0 {
			widows = 1
		}
		if block.keepTogether && len(block.lines) <= int(^uint32(0)) {
			orphans, widows = uint32(len(block.lines)), uint32(len(block.lines)) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
		}
		paragraph, err := layoutengine.NewParagraphLinePlanContext(ctx, layoutengine.ParagraphLinePlanInput{
			Node: block.node, Key: block.key, Instance: block.instance, Lines: block.lines,
			Orphans: orphans, Widows: widows, Mode: layoutengine.ParagraphBreakPrefer,
		})
		if err != nil {
			return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] paragraph plan: %w", blockIndex, err)
		}
		token := paragraph.Start()
		for {
			if err := ctx.Err(); err != nil {
				return layoutengine.LayoutPlan{}, err
			}
			fragmentNextBody, err := resolveBody(pageNumber + 1)
			if err != nil {
				return layoutengine.LayoutPlan{}, err
			}
			fragment, err := paragraph.FragmentContext(ctx, layoutengine.ParagraphFragmentSpace{
				Available: available, RegionEmpty: regionEmpty, NextRegionCapacity: fragmentNextBody.Height,
			}, token)
			if err != nil {
				return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] paragraph fragment: %w", blockIndex, err)
			}
			if fragment.Action == layoutengine.ParagraphDefer {
				nextLine := token.NextLine()
				if uint64(nextLine) >= uint64(len(block.lines)) || len(geometry.Fragments) == 0 {
					return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] paragraph deferral has no break evidence", blockIndex)
				}
				pendingBreak = &pendingPaperBreak{
					reason:   layoutengine.BreakInsufficientRemainingBodySpace,
					fromPage: pageNumber, preceding: geometry.Fragments[len(geometry.Fragments)-1].ID,
					required: block.lines[nextLine].Height, available: available,
				}
				if err := advanceBodyPage(); err != nil {
					return layoutengine.LayoutPlan{}, err
				}
				continue
			}
			if fragment.Action != layoutengine.ParagraphPlace || fragment.Lines.Count == 0 {
				return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] paragraph made no layout progress", blockIndex)
			}

			if regionEmpty {
				startBodyPage()
			}
			page := &geometry.Pages[len(geometry.Pages)-1]
			fragmentID := layoutengine.FragmentID(len(geometry.Fragments) + 1) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
			box, err := layoutengine.NewRect(body.X, cursorY, body.Width, fragment.Height)
			if err != nil {
				return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] fragment box: %w", blockIndex, err)
			}
			geometry.Fragments = append(geometry.Fragments, layoutengine.Fragment{
				ID: fragmentID, Node: block.node, Key: block.key, Instance: block.instance,
				Page: pageNumber, Region: layoutengine.RegionBody, BorderBox: box, ContentBox: box,
				Continuation: fragment.Continuation,
				Source:       block.source,
			})
			page.Fragments.Count++
			if fragment.RelaxedOrphans || fragment.RelaxedWidows {
				diagnostics = append(diagnostics, layoutengine.Diagnostic{
					Code: layoutengine.DiagnosticParagraphConstraintRelaxed, Severity: layoutengine.SeverityWarning, Stage: layoutengine.StageLayout,
					Message: "preferred paragraph pagination constraints were relaxed to guarantee progress",
					Location: layoutengine.DiagnosticLocation{Node: block.node, Key: block.key, Instance: block.instance,
						Source: block.source, Fragment: fragmentID, Page: pageNumber, Region: layoutengine.RegionBody,
						Bounds: box, HasBounds: true},
					Evidence: []layoutengine.DiagnosticEvidence{
						{Key: "orphans_applied", Value: fmt.Sprint(fragment.OrphansApplied)},
						{Key: "orphans_requested", Value: fmt.Sprint(orphans)},
						{Key: "widows_applied", Value: fmt.Sprint(fragment.WidowsApplied)},
						{Key: "widows_requested", Value: fmt.Sprint(widows)},
					},
				})
			}
			if pendingBreak != nil {
				geometry.Breaks = append(geometry.Breaks, layoutengine.BreakDecision{
					Reason: pendingBreak.reason, FromPage: pendingBreak.fromPage, ToPage: pageNumber,
					Region: layoutengine.RegionBody, Preceding: pendingBreak.preceding, Triggering: fragmentID,
					Required: pendingBreak.required, Available: pendingBreak.available,
				})
				pendingBreak = nil
			}

			lineY := cursorY
			for sourceLine := fragment.Lines.Start; sourceLine < fragment.Lines.Start+fragment.Lines.Count; sourceLine++ {
				lineInput := block.lines[sourceLine]
				x, err := body.X.Add(lineInput.OffsetX)
				if err != nil {
					return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] line %d x: %w", blockIndex, sourceLine, err)
				}
				bounds, err := layoutengine.NewRect(x, lineY, lineInput.Width, lineInput.Height)
				if err != nil {
					return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] line %d bounds: %w", blockIndex, sourceLine, err)
				}
				baseline, err := lineY.Add(lineInput.Baseline)
				if err != nil {
					return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] line %d baseline: %w", blockIndex, sourceLine, err)
				}
				globalLine := uint32(len(geometry.Lines)) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
				geometry.Lines = append(geometry.Lines, layoutengine.PlannedLine{
					Fragment: fragmentID, Index: sourceLine, Bounds: bounds, Baseline: baseline, Source: lineInput.Source,
				})
				page.Lines.Count++
				for _, localRun := range block.runs[sourceLine] {
					localRun.Line = globalLine
					localRun.Font = block.fontIDs[localRun.Font]
					localRun.Origin.X, err = bounds.X.Add(localRun.Origin.X)
					if err != nil {
						return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] glyph run x: %w", blockIndex, err)
					}
					localRun.Origin.Y = baseline
					localRun.Source = block.source
					runs = append(runs, localRun)
					displayItems = append(displayItems, layoutengine.DisplayItem{Kind: layoutengine.CommandGlyphRun, Payload: uint32(len(runs) - 1)}) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
				}
				if err := appendMeasuredDecorations(block, sourceLine, layoutengine.PlannedLine{Fragment: fragmentID, Index: sourceLine, Bounds: bounds, Baseline: baseline, Source: lineInput.Source}, fragmentID); err != nil {
					return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] line %d decoration: %w", blockIndex, sourceLine, err)
				}
				lineY, err = lineY.Add(lineInput.Height)
				if err != nil {
					return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] line %d cursor: %w", blockIndex, sourceLine, err)
				}
			}

			cursorY, err = cursorY.Add(fragment.Height)
			if err != nil {
				return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] fragment cursor: %w", blockIndex, err)
			}
			available, err = available.Sub(fragment.Height)
			if err != nil {
				return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] remaining body height: %w", blockIndex, err)
			}
			regionEmpty = false
			if fragment.Done {
				break
			}
			nextLine := fragment.Next.NextLine()
			if uint64(nextLine) >= uint64(len(block.lines)) {
				return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] continuation has no next line", blockIndex)
			}
			reason := layoutengine.BreakInsufficientRemainingBodySpace
			breakAvailable := available
			if fragment.OversizedLine {
				reason = layoutengine.BreakPreviousFragmentOverflow
				breakAvailable = 0
			} else if fragment.PolicyBreak {
				reason = layoutengine.BreakPaginationConstraint
			}
			required := block.lines[nextLine].Height
			if fragment.PolicyBreak && fragment.BreakRequired > required {
				required = fragment.BreakRequired
			}
			pendingBreak = &pendingPaperBreak{
				reason: reason, fromPage: pageNumber, preceding: fragmentID,
				required: required, available: breakAvailable,
			}
			if err := advanceBodyPage(); err != nil {
				return layoutengine.LayoutPlan{}, err
			}
			token = fragment.Next
		}
	}

	geometry.Diagnostics = diagnostics
	plan, err := layoutengine.NewLayoutPlan(geometry)
	if err != nil {
		return layoutengine.LayoutPlan{}, fmt.Errorf("document: paper text geometry: %w", err)
	}
	plan, err = layoutengine.AttachDisplayList(plan, layoutengine.DisplayListInput{
		Fonts: fonts, GlyphRuns: runs, ImageResources: imageResources, Images: images,
		Paths: paths, Fills: fills, Strokes: strokes, Items: displayItems,
	})
	if err != nil {
		return layoutengine.LayoutPlan{}, fmt.Errorf("document: attach paper display list: %w", err)
	}
	plan, err = paperAttachBoxOverflowClips(plan, measured)
	if err != nil {
		return layoutengine.LayoutPlan{}, fmt.Errorf("document: attach paper box overflow clips: %w", err)
	}
	plan, err = paperAttachBoxDecorations(plan, measured, explicitDecorationBoxes)
	if err != nil {
		return layoutengine.LayoutPlan{}, fmt.Errorf("document: attach paper box decorations: %w", err)
	}
	plan, err = attachTypedSegmentLinks(plan, typedMeasuredSegmentLinks(measured))
	if err != nil {
		return layoutengine.LayoutPlan{}, err
	}
	plan, err = attachPlanSemantics(plan, measured, strings.TrimSpace(doc.Language), mapping)
	if err != nil {
		return layoutengine.LayoutPlan{}, fmt.Errorf("document: attach plan semantics: %w", err)
	}
	return plan, nil
}

func attachPlanSemantics(plan layoutengine.LayoutPlan, measured []paperMeasuredBlock, language string, mapping papercompile.CompileMapping) (layoutengine.LayoutPlan, error) {
	projection := plan.Projection()
	rootKey := layoutengine.NodeKey("@typed-document")
	rootInstance := layoutengine.InstanceID("@typed-document")
	var rootSource layoutengine.SourceSpan
	for _, mapped := range mapping.Nodes {
		if mapped.Kind != paperlang.NodeDocument {
			continue
		}
		if mapped.ID != "" {
			rootKey = layoutengine.NodeKey(mapped.ID)
			rootInstance = layoutengine.InstanceID(mapped.ID)
			if mapped.InstancePath != "" {
				rootInstance = layoutengine.InstanceID(mapped.InstancePath + "/" + mapped.ID)
			}
		}
		rootSource = paperLayoutSourceSpan(mapped.Span)
		break
	}
	nodes := []layoutengine.SemanticNode{{
		ID: 1, Role: layoutengine.SemanticRoleDocument, Key: rootKey, Instance: rootInstance, Source: rootSource,
		Attributes: layoutengine.SemanticAttributes{Language: language},
	}}
	byNode := make(map[layoutengine.NodeID]layoutengine.SemanticNodeID, len(measured))
	containerByPath := make(map[string]layoutengine.SemanticNodeID)
	ensureAncestors := func(ancestors []typedSemanticAncestor) layoutengine.SemanticNodeID {
		parent := layoutengine.SemanticNodeID(1)
		for _, ancestor := range ancestors {
			if existing := containerByPath[ancestor.path]; existing.Valid() {
				parent = existing
				continue
			}
			id := layoutengine.SemanticNodeID(len(nodes) + 1) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
			identity := "@typed-container:" + ancestor.path
			nodes = append(nodes, layoutengine.SemanticNode{ID: id, Parent: parent, Role: ancestor.role,
				Key: layoutengine.NodeKey(identity), Instance: layoutengine.InstanceID(identity),
				Attributes: typedContainerSemanticAttributes(ancestor.role, ancestor.text)})
			containerByPath[ancestor.path] = id
			parent = id
		}
		return parent
	}
	appendNode := func(node layoutengine.NodeID, key layoutengine.NodeKey, instance layoutengine.InstanceID, source layoutengine.SourceSpan, role layoutengine.SemanticRole, text string, headingLevel uint8, ancestors []typedSemanticAncestor) {
		if !node.Valid() || byNode[node].Valid() {
			return
		}
		if role == "" {
			role = layoutengine.SemanticRoleParagraph
		}
		parent := ensureAncestors(ancestors)
		id := layoutengine.SemanticNodeID(len(nodes) + 1) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
		attributes := typedPlanSemanticAttributes(role, text)
		if role == layoutengine.SemanticRoleHeading {
			attributes.HeadingLevel = headingLevel
		}
		nodes = append(nodes, layoutengine.SemanticNode{ID: id, Parent: parent, Role: role, Key: key, Instance: instance,
			Source: source, Attributes: attributes})
		byNode[node] = id
	}
	for _, block := range measured {
		if block.explicitBreak {
			continue
		}
		if block.gridRow != nil {
			for _, cell := range block.gridRow.cells {
				identity := cell.measurement.identity
				role := cell.semanticRole
				if role == "" {
					role = layoutengine.SemanticRoleCell
				}
				if cell.artifactOnly {
					role = layoutengine.SemanticRoleArtifact
				}
				appendNode(cell.node, identity.key, identity.instance, identity.source, role, cell.semanticText, 0, block.semanticAncestors)
			}
			continue
		}
		if block.canvas != nil {
			parent := ensureAncestors(block.semanticAncestors)
			canvasSemantic := layoutengine.SemanticNodeID(len(nodes) + 1) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
			nodes = append(nodes, layoutengine.SemanticNode{ID: canvasSemantic, Parent: parent, Role: layoutengine.SemanticRoleSection,
				Key: block.key, Instance: block.instance, Source: block.source})
			byNode[block.node] = canvasSemantic
			for _, item := range block.canvas.items {
				role := item.role
				if role == "" {
					role = layoutengine.SemanticRoleArtifact
				}
				id := layoutengine.SemanticNodeID(len(nodes) + 1) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
				nodes = append(nodes, layoutengine.SemanticNode{ID: id, Parent: canvasSemantic, Role: role,
					Key: item.key, Instance: item.instance, Source: item.source,
					Attributes: typedPlanSemanticAttributes(role, item.alt)})
				byNode[item.node] = id
			}
			continue
		}
		appendNode(block.node, block.key, block.instance, block.source, block.semanticRole, block.semanticText, block.headingLevel, block.semanticAncestors)
	}
	associations := make([]layoutengine.SemanticFragmentAssociation, 0, len(projection.Fragments))
	reading := make([]layoutengine.ReadingOccurrence, 0, len(projection.Fragments))
	pageIndex := make(map[uint32]uint32)
	for _, fragment := range projection.Fragments {
		semantic := byNode[fragment.Node]
		if !semantic.Valid() {
			return layoutengine.LayoutPlan{}, fmt.Errorf("fragment %d has no typed semantic owner", fragment.ID)
		}
		associations = append(associations, layoutengine.SemanticFragmentAssociation{Semantic: semantic, Page: fragment.Page, Fragment: fragment.ID})
		if nodes[semantic-1].Role != layoutengine.SemanticRoleArtifact {
			reading = append(reading, layoutengine.ReadingOccurrence{Semantic: semantic, Page: fragment.Page, Fragment: fragment.ID, ReadingIndex: pageIndex[fragment.Page]})
			pageIndex[fragment.Page]++
		}
	}
	return layoutengine.AttachSemantics(plan, nodes, associations, reading)
}

func typedContainerSemanticAttributes(role layoutengine.SemanticRole, text string) layoutengine.SemanticAttributes {
	if role == layoutengine.SemanticRoleFigure {
		return layoutengine.SemanticAttributes{AlternateText: typedSemanticActualText(text)}
	}
	if role == layoutengine.SemanticRoleListItem && text != "" {
		return layoutengine.SemanticAttributes{ActualText: typedSemanticActualText(text)}
	}
	return layoutengine.SemanticAttributes{}
}

func typedPlanSemanticAttributes(role layoutengine.SemanticRole, text string) layoutengine.SemanticAttributes {
	switch role {
	case layoutengine.SemanticRoleFigure:
		return layoutengine.SemanticAttributes{AlternateText: text}
	case layoutengine.SemanticRoleArtifact:
		return layoutengine.SemanticAttributes{}
	default:
		return layoutengine.SemanticAttributes{ActualText: typedSemanticActualText(text)}
	}
}

func typedSemanticActualText(text string) string {
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, text))
}

type paperCoreFontIdentity struct {
	face     layoutengine.CoreFontFace
	digest   layoutengine.CoreFontMetricsDigest
	embedded layoutengine.CoreFontMetricsDigest
}

func paperFontIdentity(font layoutengine.CoreFontResource) paperCoreFontIdentity {
	identity := paperCoreFontIdentity{face: font.Face, digest: font.MetricsDigest}
	if font.EmbeddedUTF8 != nil {
		identity.embedded = font.EmbeddedUTF8.Digest
	}
	return identity
}

func paperParagraphBlock(block layout.Block) (layout.ParagraphBlock, bool) {
	switch block := block.(type) {
	case layout.ParagraphBlock:
		return block, true
	case layout.HeadingBlock:
		return layout.ParagraphBlock{
			Segments: block.Segments, Style: block.Style, StyleRef: block.StyleRef,
			Box: block.Box, BoxRef: block.BoxRef,
		}, true
	default:
		return layout.ParagraphBlock{}, false
	}
}

// paperExpandPlanningBlocks lowers the shared model's text-like body blocks
// into the one exact paragraph primitive measured by the core-font shadow.
// List markers become ordinary ASCII text before wrapping, so measurement,
// pagination, glyph planning, preview, and PDF painting all observe the same
// content rather than recreating markers during paint.
type paperPaginationPolicy struct {
	keepTogether bool
	keepWithNext bool
	orphans      uint32
	widows       uint32
	applyOrphans bool
	applyWidows  bool
}

func paperBoxPaginationPolicy(box layout.BoxStyle, _ *layout.BoxStyle, path string) (paperPaginationPolicy, error) {
	policy := paperPaginationPolicy{keepTogether: box.KeepTogether, keepWithNext: box.KeepWithNext,
		orphans: box.Orphans, widows: box.Widows, applyOrphans: box.Orphans != 0, applyWidows: box.Widows != 0}
	box.KeepTogether, box.KeepWithNext = false, false
	box.Orphans, box.Widows = 0, 0
	if box != (layout.BoxStyle{}) {
		return paperPaginationPolicy{}, fmt.Errorf("%s: visual box styling is unsupported by exact typed planning", path)
	}
	if policy.orphans == 0 {
		policy.orphans = 1
	}
	if policy.widows == 0 {
		policy.widows = 1
	}
	return policy, nil
}

func paperApplyPaginationPolicy(expanded *[]paperPlanningBlock, start int, policy paperPaginationPolicy, nextGroup *uint32, path string) error {
	end := len(*expanded)
	if start == end {
		return nil
	}
	if policy.keepTogether {
		*nextGroup = *nextGroup + 1
		group := *nextGroup
		for index := start; index < end; index++ {
			if (*expanded)[index].explicitBreak {
				return fmt.Errorf("%s: keep-together cannot cross an explicit page break", path)
			}
			(*expanded)[index].keepGroups = append((*expanded)[index].keepGroups, group)
		}
	}
	for index := start; index < end; index++ {
		if (*expanded)[index].image == nil && !(*expanded)[index].explicitBreak {
			if policy.applyOrphans {
				(*expanded)[index].orphans = policy.orphans
			}
			if policy.applyWidows {
				(*expanded)[index].widows = policy.widows
			}
		}
	}
	if policy.keepWithNext {
		for index := end - 1; index >= start; index-- {
			if !(*expanded)[index].explicitBreak {
				(*expanded)[index].keepWithNext = true
				break
			}
		}
	}
	return nil
}

func paperExpandPlanningBlocks(ctx context.Context, blocks []layout.Block) ([]paperPlanningBlock, error) {
	expanded := make([]paperPlanningBlock, 0, len(blocks))
	var nextGroup uint32
	for bodyIndex, block := range blocks {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := paperExpandPlanningBlock(ctx, &expanded, block, bodyIndex, -1, -1, fmt.Sprintf("body[%d]", bodyIndex), &nextGroup); err != nil {
			return nil, err
		}
	}
	applyTypedSemanticHierarchy(blocks, expanded)
	return expanded, nil
}

func applyTypedSemanticHierarchy(blocks []layout.Block, expanded []paperPlanningBlock) {
	byPath := make(map[string][]typedSemanticAncestor)
	cloneAppend := func(ancestors []typedSemanticAncestor, value typedSemanticAncestor) []typedSemanticAncestor {
		result := append([]typedSemanticAncestor(nil), ancestors...)
		return append(result, value)
	}
	var walk func(layout.Block, string, []typedSemanticAncestor)
	walk = func(candidate layout.Block, path string, ancestors []typedSemanticAncestor) {
		block, ok := layout.NormalizeBlock(candidate)
		if !ok {
			return
		}
		byPath[path] = append([]typedSemanticAncestor(nil), ancestors...)
		switch block := block.(type) {
		case layout.SectionBlock:
			nested := cloneAppend(ancestors, typedSemanticAncestor{path: path, role: layoutengine.SemanticRoleSection, text: strings.TrimSpace(block.Title)})
			byPath[path+".title"] = nested
			for index, child := range layout.NormalizeBlocks(block.Blocks) {
				walk(child, fmt.Sprintf("%s.blocks[%d]", path, index), nested)
			}
		case layout.ClauseBlock:
			nested := cloneAppend(ancestors, typedSemanticAncestor{path: path, role: layoutengine.SemanticRoleSection, text: strings.TrimSpace(block.Number + " " + block.Title)})
			byPath[path+".title"] = nested
			for index, child := range layout.NormalizeBlocks(block.Blocks) {
				walk(child, fmt.Sprintf("%s.blocks[%d]", path, index), nested)
			}
		case layout.NoteBoxBlock:
			nested := cloneAppend(ancestors, typedSemanticAncestor{path: path, role: layoutengine.SemanticRoleSection, text: strings.TrimSpace(block.Title)})
			byPath[path+".title"] = nested
			for index, child := range layout.NormalizeBlocks(block.Body) {
				walk(child, fmt.Sprintf("%s.body[%d]", path, index), nested)
			}
		case layout.ListBlock:
			list := cloneAppend(ancestors, typedSemanticAncestor{path: path, role: layoutengine.SemanticRoleList})
			for itemIndex, item := range block.Items {
				itemPath := fmt.Sprintf("%s.items[%d]", path, itemIndex)
				itemAncestors := cloneAppend(list, typedSemanticAncestor{path: itemPath, role: layoutengine.SemanticRoleListItem})
				for blockIndex, child := range layout.NormalizeBlocks(item.Blocks) {
					walk(child, fmt.Sprintf("%s.blocks[%d]", itemPath, blockIndex), itemAncestors)
				}
			}
		case layout.MetadataGridBlock:
			grid := cloneAppend(ancestors, typedSemanticAncestor{path: path, role: layoutengine.SemanticRoleSection, text: "metadata grid"})
			columns := block.Columns
			if columns <= 0 {
				columns = 2
			}
			for start, row := 0, 0; start < len(block.Fields); start, row = start+columns, row+1 {
				rowPath := fmt.Sprintf("%s.rows[%d]", path, row)
				byPath[rowPath] = cloneAppend(grid, typedSemanticAncestor{path: rowPath, role: layoutengine.SemanticRoleRow})
			}
		case layout.SignatureRowBlock:
			group := cloneAppend(ancestors, typedSemanticAncestor{path: path, role: layoutengine.SemanticRoleSection, text: "signature row"})
			byPath[path] = cloneAppend(group, typedSemanticAncestor{path: path + ".row", role: layoutengine.SemanticRoleRow})
		case layout.ImageBlock:
			byPath[path+".caption"] = cloneAppend(ancestors, typedSemanticAncestor{path: path, role: layoutengine.SemanticRoleFigure, text: strings.TrimSpace(block.Alt)})
		case layout.QRVerificationBlock:
			figure := cloneAppend(ancestors, typedSemanticAncestor{path: path, role: layoutengine.SemanticRoleFigure, text: typedQRAlternateText(block.QR)})
			byPath[path+".qr"], byPath[path+".text"] = figure, figure
		}
	}
	for index, block := range layout.NormalizeBlocks(blocks) {
		walk(block, fmt.Sprintf("body[%d]", index), nil)
	}
	for index := range expanded {
		expanded[index].semanticAncestors = append([]typedSemanticAncestor(nil), byPath[expanded[index].path]...)
		if strings.HasSuffix(expanded[index].path, ".title") {
			expanded[index].semanticRole = layoutengine.SemanticRoleHeading
		}
	}
}

func paperExpandPlanningBlock(ctx context.Context, expanded *[]paperPlanningBlock, candidate layout.Block, bodyIndex, segmentIndex, nestedIndex int, path string, nextGroup *uint32) error {
	if err := layoutengine.ChargePlanningWork(ctx, "typed document subtree expansion", 1); err != nil {
		return err
	}
	block, ok := layout.NormalizeBlock(candidate)
	if !ok {
		return nil
	}
	appendParagraph := func(paragraph layout.ParagraphBlock, path string) {
		*expanded = append(*expanded, paperPlanningBlock{
			bodyIndex: bodyIndex, segmentIndex: segmentIndex, nestedIndex: nestedIndex,
			path: path, paragraph: paragraph, semanticRole: layoutengine.SemanticRoleParagraph,
		})
	}
	appendSemanticParagraph := func(paragraph layout.ParagraphBlock, path string, role layoutengine.SemanticRole, actualText string, keepTogether bool) {
		appendParagraph(paragraph, path)
		last := &(*expanded)[len(*expanded)-1]
		last.semanticRole, last.semanticText, last.keepTogether = role, actualText, keepTogether
	}
	switch block := block.(type) {
	case layout.PageBreakBlock:
		if block.Before || block.After {
			*expanded = append(*expanded, paperPlanningBlock{
				bodyIndex: bodyIndex, segmentIndex: segmentIndex, nestedIndex: nestedIndex,
				path: path, explicitBreak: true,
			})
		}
		return nil
	case layout.ListBlock:
		policy, visualBox := paperContainerBoxPolicy(block.EffectiveBox())
		start := len(*expanded)
		listStyle := block.EffectiveStyle()
		for itemIndex, item := range block.Items {
			if err := ctx.Err(); err != nil {
				return err
			}
			itemBlocks := layout.NormalizeBlocks(item.Blocks)
			itemPath := fmt.Sprintf("%s.items[%d]", path, itemIndex)
			if len(itemBlocks) == 0 {
				return fmt.Errorf("%s has no text blocks", itemPath)
			}
			for itemBlockIndex, itemBlock := range itemBlocks {
				paragraph, ok := paperParagraphBlock(itemBlock)
				if !ok {
					if nested, nestedOK := itemBlock.(layout.ListBlock); nestedOK && itemBlockIndex > 0 {
						nested.Style = layout.MergedTextStyle(listStyle, nested.EffectiveStyle())
						nested.StyleRef = nil
						if err := paperExpandPlanningBlock(ctx, expanded, nested, bodyIndex, itemIndex, itemBlockIndex,
							fmt.Sprintf("%s.blocks[%d]", itemPath, itemBlockIndex), nextGroup); err != nil {
							return err
						}
						continue
					}
					return fmt.Errorf("%s.blocks[%d] is %s", itemPath, itemBlockIndex, itemBlock.DocumentBlockKind())
				}
				paragraph.Style = layout.MergedTextStyle(listStyle, paragraph.EffectiveStyle())
				paragraph.StyleRef = nil
				childPolicy, err := paperBoxPaginationPolicy(paragraph.EffectiveBox(), paragraph.BoxRef, fmt.Sprintf("%s.blocks[%d]", itemPath, itemBlockIndex))
				if err != nil {
					return err
				}
				paragraph.Box, paragraph.BoxRef = layout.BoxStyle{}, nil
				if itemBlockIndex == 0 {
					marker, err := paperListMarker(block, itemIndex)
					if err != nil {
						return fmt.Errorf("%s marker: %w", path, err)
					}
					segments := make([]layout.TextSegment, 0, len(paragraph.Segments)+1)
					segments = append(segments, layout.TextSegment{Text: marker + " "})
					segments = append(segments, paragraph.Segments...)
					paragraph.Segments = segments
				}
				*expanded = append(*expanded, paperPlanningBlock{
					bodyIndex: bodyIndex, segmentIndex: itemIndex, nestedIndex: itemBlockIndex,
					path: fmt.Sprintf("%s.blocks[%d]", itemPath, itemBlockIndex), paragraph: paragraph,
					keepTogether: childPolicy.keepTogether, keepWithNext: childPolicy.keepWithNext,
					orphans: childPolicy.orphans, widows: childPolicy.widows,
				})
			}
		}
		if err := paperApplyContainerBox(expanded, start, visualBox, path); err != nil {
			return err
		}
		return paperApplyPaginationPolicy(expanded, start, policy, nextGroup, path)
	case layout.MetadataGridBlock:
		policy, visualBox := paperContainerBoxPolicy(block.EffectiveBox())
		start := len(*expanded)
		columns := block.Columns
		if columns <= 0 {
			columns = 2
		}
		if columns > typedGridMaxColumns {
			return fmt.Errorf("%s: metadata-grid columns=%d exceeds the exact planner limit of %d", path, columns, typedGridMaxColumns)
		}
		if len(block.Fields) == 0 {
			return fmt.Errorf("%s: metadata-grid has no fields", path)
		}
		if !finiteNumbers(block.Gap) || block.Gap < 0 {
			return fmt.Errorf("%s: metadata-grid gap must be finite and non-negative", path)
		}
		style := block.EffectiveStyle()
		for rowStart := 0; rowStart < len(block.Fields); rowStart += columns {
			rowEnd := rowStart + columns
			if rowEnd > len(block.Fields) {
				rowEnd = len(block.Fields)
			}
			gap, gapInDocumentUnits := typedMetadataGridGapPoints, false
			if block.Gap > 0 {
				gap, gapInDocumentUnits = block.Gap, true
			}
			row := paperPlanningGridRow{columnCount: columns, gapPoints: gap, gapInDocumentUnits: gapInDocumentUnits,
				cells: make([]paperPlanningGridCell, 0, rowEnd-rowStart)}
			for fieldIndex := rowStart; fieldIndex < rowEnd; fieldIndex++ {
				text := metadataFieldText(block.Fields[fieldIndex])
				row.cells = append(row.cells, paperPlanningGridCell{
					paragraph:    layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: text}}, Style: style},
					path:         fmt.Sprintf("%s.fields[%d].column[%d]", path, fieldIndex, fieldIndex%columns),
					semanticText: text, segmentIndex: fieldIndex,
				})
			}
			*expanded = append(*expanded, paperPlanningBlock{bodyIndex: bodyIndex, segmentIndex: rowStart / columns,
				nestedIndex: -1, path: fmt.Sprintf("%s.rows[%d]", path, rowStart/columns), gridRow: &row, keepTogether: true})
		}
		if err := paperApplyContainerBox(expanded, start, visualBox, path); err != nil {
			return err
		}
		return paperApplyPaginationPolicy(expanded, start, policy, nextGroup, path)
	case layout.SignatureRowBlock:
		policy, visualBox := paperContainerBoxPolicy(block.EffectiveBox())
		policy.keepTogether = true
		start := len(*expanded)
		columns := append([]layout.SignatureColumn(nil), block.Columns...)
		if len(columns) == 0 {
			columns = []layout.SignatureColumn{{}}
		}
		if len(columns) > typedGridMaxColumns {
			return fmt.Errorf("%s: signature-row columns=%d exceeds the exact planner limit of %d", path, len(columns), typedGridMaxColumns)
		}
		if !finiteNumbers(block.Gap) || block.Gap < 0 {
			return fmt.Errorf("%s: signature-row gap must be finite and non-negative", path)
		}
		gap, gapInDocumentUnits := typedSignatureGridGapPoints, true
		if block.Gap > 0 {
			gap, gapInDocumentUnits = block.Gap, true
		}
		row := paperPlanningGridRow{columnCount: len(columns), gapPoints: gap, gapInDocumentUnits: gapInDocumentUnits,
			minimumHeightPoints: 24, minimumHeightInDocumentUnits: true,
			lineOffsetPoints: 12, lineOffsetInDocumentUnits: true,
			cells: make([]paperPlanningGridCell, 0, len(columns))}
		for columnIndex, column := range columns {
			if !finiteNumbers(column.Width) || column.Width < 0 {
				return fmt.Errorf("%s.columns[%d]: requested width must be finite and non-negative", path, columnIndex)
			}
			text := signatureColumnText(column)
			row.cells = append(row.cells, paperPlanningGridCell{
				paragraph: layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: text}},
					Style: layout.TextStyle{FontFamily: "Helvetica", FontSize: 9, Align: "C", LineHeight: 9}},
				path: fmt.Sprintf("%s.columns[%d]", path, columnIndex), semanticText: strings.ReplaceAll(text, "\n", "; "),
				semanticRole:   layoutengine.SemanticRoleCell,
				requestedWidth: column.Width, segmentIndex: columnIndex,
				topInsetPoints: 14, topInsetInDocumentUnits: true,
				compactLineHeight: 4, compactLineHeightInDocumentUnits: true, artifactOnly: text == "",
			})
		}
		*expanded = append(*expanded, paperPlanningBlock{bodyIndex: bodyIndex, segmentIndex: segmentIndex,
			nestedIndex: nestedIndex, path: path, gridRow: &row, keepTogether: true})
		if err := paperApplyContainerBox(expanded, start, visualBox, path); err != nil {
			return err
		}
		return paperApplyPaginationPolicy(expanded, start, policy, nextGroup, path)
	case layout.ImageBlock:
		box := block.EffectiveBox()
		policy := paperPaginationPolicy{keepTogether: box.KeepTogether, keepWithNext: box.KeepWithNext,
			orphans: box.Orphans, widows: box.Widows, applyOrphans: box.Orphans != 0, applyWidows: box.Widows != 0}
		if policy.applyOrphans || policy.applyWidows {
			return fmt.Errorf("%s: widow/orphan policy applies only to text", path)
		}
		box.KeepTogether, box.KeepWithNext, box.Orphans, box.Widows = false, false, 0, 0
		block.Box, block.BoxRef = box, nil
		if err := validateTypedPlanningImage(block, path); err != nil {
			return err
		}
		copy := block
		role := layoutengine.SemanticRoleFigure
		if block.Decorative || strings.TrimSpace(block.Alt) == "" {
			role = layoutengine.SemanticRoleArtifact
		}
		*expanded = append(*expanded, paperPlanningBlock{
			bodyIndex: bodyIndex, segmentIndex: segmentIndex, nestedIndex: nestedIndex,
			path: path, image: &copy, semanticRole: role,
			semanticText: strings.TrimSpace(block.Alt), keepTogether: true, keepWithNext: policy.keepWithNext,
		})
		if len(block.Caption) > 0 {
			(*expanded)[len(*expanded)-1].keepWithNext = true
			captionStyle := layout.MergedTextStyle(
				layout.TextStyle{FontFamily: "Helvetica", FontSize: 9, Italic: true, Align: "C", LineHeight: 10},
				block.CaptionStyle,
			)
			appendSemanticParagraph(layout.ParagraphBlock{
				Segments: block.Caption,
				Style:    captionStyle,
			}, path+".caption", layoutengine.SemanticRoleParagraph, layout.TextSegmentsPlainText(block.Caption), false)
		}
		return nil
	case layout.CanvasBlock:
		if block.Width <= 0 || block.Height <= 0 || len(block.Items) == 0 {
			return fmt.Errorf("%s: canvas requires positive dimensions and at least one item", path)
		}
		copy := block
		*expanded = append(*expanded, paperPlanningBlock{bodyIndex: bodyIndex, segmentIndex: segmentIndex,
			nestedIndex: nestedIndex, path: path, canvas: &copy, keepTogether: true})
		return nil
	case layout.QRVerificationBlock:
		return paperExpandQRVerification(ctx, expanded, block, bodyIndex, segmentIndex, nestedIndex, path, nextGroup)
	case layout.SectionBlock:
		sectionBox := block.EffectiveBox()
		policy, visualBox := paperContainerBoxPolicy(sectionBox)
		start := len(*expanded)
		if strings.TrimSpace(block.Title) != "" {
			appendParagraph(layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: block.Title}}}, path+".title")
			if block.KeepTitleWithBody {
				(*expanded)[len(*expanded)-1].keepWithNext = true
			}
		}
		if err := paperExpandPlanningChildren(ctx, expanded, block.Blocks, bodyIndex, path+".blocks", nextGroup); err != nil {
			return err
		}
		if err := paperApplyContainerBox(expanded, start, visualBox, path); err != nil {
			return err
		}
		return paperApplyPaginationPolicy(expanded, start, policy, nextGroup, path)
	case layout.ClauseBlock:
		policy, visualBox := paperContainerBoxPolicy(block.EffectiveBox())
		policy.keepTogether = policy.keepTogether || block.KeepTogether
		if block.BreakBefore {
			*expanded = append(*expanded, paperPlanningBlock{bodyIndex: bodyIndex, path: path + ".break-before", explicitBreak: true, segmentIndex: -1, nestedIndex: -1})
		}
		start := len(*expanded)
		title := strings.TrimSpace(block.Number + " " + block.Title)
		if title != "" {
			appendParagraph(layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: title}}}, path+".title")
		}
		if err := paperExpandPlanningChildren(ctx, expanded, block.Blocks, bodyIndex, path+".blocks", nextGroup); err != nil {
			return err
		}
		if err := paperApplyContainerBox(expanded, start, visualBox, path); err != nil {
			return err
		}
		if err := paperApplyPaginationPolicy(expanded, start, policy, nextGroup, path); err != nil {
			return err
		}
		if block.BreakAfter {
			*expanded = append(*expanded, paperPlanningBlock{bodyIndex: bodyIndex, path: path + ".break-after", explicitBreak: true, segmentIndex: -1, nestedIndex: -1})
		}
		return nil
	case layout.NoteBoxBlock:
		policy, visualBox := paperContainerBoxPolicy(block.EffectiveBox())
		start := len(*expanded)
		if block.StyleRef != nil {
			return fmt.Errorf("%s: note title style references are unsupported", path)
		}
		if strings.TrimSpace(block.Title) != "" {
			appendParagraph(layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: block.Title}}, Style: block.Style}, path+".title")
		}
		if err := paperExpandPlanningChildren(ctx, expanded, block.Body, bodyIndex, path+".body", nextGroup); err != nil {
			return err
		}
		if err := paperApplyContainerBox(expanded, start, visualBox, path); err != nil {
			return err
		}
		return paperApplyPaginationPolicy(expanded, start, policy, nextGroup, path)
	default:
		paragraph, ok := paperParagraphBlock(block)
		if !ok {
			return fmt.Errorf("%s is %s", path, block.DocumentBlockKind())
		}
		policy, visualBox, err := paperParagraphBoxPolicy(paragraph.EffectiveBox(), paragraph.BoxRef, path)
		if err != nil {
			return err
		}
		paragraph.Box, paragraph.BoxRef = layout.BoxStyle{}, nil
		role := layoutengine.SemanticRoleParagraph
		if heading, isHeading := block.(layout.HeadingBlock); isHeading {
			role = layoutengine.SemanticRoleHeading
			appendSemanticParagraph(paragraph, path, role, layout.TextSegmentsPlainText(heading.Segments), false)
			if heading.Level >= 1 && heading.Level <= 6 {
				(*expanded)[len(*expanded)-1].headingLevel = uint8(heading.Level)
			}
		} else {
			appendSemanticParagraph(paragraph, path, role, layout.TextSegmentsPlainText(paragraph.Segments), false)
		}
		last := &(*expanded)[len(*expanded)-1]
		last.keepTogether, last.keepWithNext = policy.keepTogether, policy.keepWithNext
		last.orphans, last.widows = policy.orphans, policy.widows
		last.box = visualBox
		if htmlUnifiedVisualBox(visualBox) {
			last.keepTogether = true
		}
		return nil
	}
}

func paperExpandPlanningChildren(ctx context.Context, expanded *[]paperPlanningBlock, children []layout.Block, bodyIndex int, prefix string, nextGroup *uint32) error {
	for index, child := range layout.NormalizeBlocks(children) {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := paperExpandPlanningBlock(ctx, expanded, child, bodyIndex, -1, index, fmt.Sprintf("%s[%d]", prefix, index), nextGroup); err != nil {
			return err
		}
	}
	return nil
}

type paperSourceIdentity struct {
	key      layoutengine.NodeKey
	instance layoutengine.InstanceID
	source   layoutengine.SourceSpan
}

func paperBlockIdentity(mapping papercompile.CompileMapping, bodyIndex, segmentIndex, nestedIndex, fallback int) paperSourceIdentity {
	fallbackID := fmt.Sprintf("@paper-block-%d", fallback+1)
	identity := paperSourceIdentity{key: layoutengine.NodeKey(fallbackID), instance: layoutengine.InstanceID(fallbackID)}
	var selected *papercompile.NodeMapping
	for index := range mapping.Nodes {
		candidate := &mapping.Nodes[index]
		if candidate.BodyIndex != bodyIndex || candidate.ID == "" {
			continue
		}
		if candidate.SegmentIndex == segmentIndex && candidate.NestedBlockIndex == nestedIndex {
			selected = candidate
			break
		}
		if selected == nil && candidate.SegmentIndex == segmentIndex && candidate.NestedBlockIndex == -1 {
			selected = candidate
		}
		if selected == nil && candidate.SegmentIndex == -1 && candidate.NestedBlockIndex == -1 {
			selected = candidate
		}
	}
	if selected == nil {
		for index := range mapping.AnonymousNodes {
			candidate := &mapping.AnonymousNodes[index]
			if candidate.BodyIndex != bodyIndex {
				continue
			}
			if candidate.SegmentIndex == segmentIndex && candidate.NestedBlockIndex == nestedIndex {
				selected = candidate
				break
			}
			if selected == nil && candidate.SegmentIndex == segmentIndex && candidate.NestedBlockIndex == -1 {
				selected = candidate
			}
			if selected == nil && candidate.SegmentIndex == -1 && candidate.NestedBlockIndex == -1 {
				selected = candidate
			}
		}
	}
	if selected == nil {
		return paperRevisionScopedFallbackIdentity(mapping.SourceRevision, identity, bodyIndex, segmentIndex, nestedIndex, fallback, paperlang.NodeKind("block"), paperlang.Span{})
	}
	if selected.ID == "" {
		return paperRevisionScopedFallbackIdentity(mapping.SourceRevision, identity, bodyIndex, segmentIndex, nestedIndex, fallback, selected.Kind, selected.Span)
	}
	readableID := selected.ID
	if segmentIndex >= 0 && selected.SegmentIndex != segmentIndex {
		readableID = fmt.Sprintf("%s#item-%d", readableID, segmentIndex+1)
	}
	if nestedIndex >= 0 && selected.NestedBlockIndex != nestedIndex {
		readableID = fmt.Sprintf("%s#block-%d", readableID, nestedIndex+1)
	}
	identity.key = layoutengine.NodeKey(readableID)
	identity.instance = layoutengine.InstanceID(readableID)
	if selected.InstancePath != "" {
		identity.instance = layoutengine.InstanceID(selected.InstancePath + "/" + readableID)
	}
	identity.source = paperLayoutSourceSpan(selected.Span)
	return identity
}

func paperRevisionScopedFallbackIdentity(revision string, fallback paperSourceIdentity, bodyIndex, segmentIndex, nestedIndex, ordinal int, kind paperlang.NodeKind, span paperlang.Span) paperSourceIdentity {
	sourceRevision, err := layoutengine.ParseSourceRevisionID(revision)
	if err != nil {
		return fallback
	}
	if ordinal < 0 {
		ordinal = 0
	}
	if uint64(ordinal) > uint64(^uint32(0)) {
		return fallback
	}
	fingerprintInput := fmt.Sprintf("%s\x00%d\x00%d\x00%d\x00%d\x00%d\x00%d", kind, bodyIndex, segmentIndex, nestedIndex, span.Start.Offset, span.End.Offset, ordinal)
	fingerprint := sha256.Sum256([]byte(fingerprintInput))
	key, err := layoutengine.DeriveAnonymousStructuralKey(layoutengine.AnonymousStructuralKeyInput{
		Revision: sourceRevision, Kind: "paper-block", Ordinal: uint32(ordinal), Fingerprint: hex.EncodeToString(fingerprint[:]),
	})
	if err != nil {
		return fallback
	}
	identity := paperSourceIdentity{key: key, instance: layoutengine.InstanceID(key), source: paperLayoutSourceSpan(span)}
	return identity
}

func paperLayoutSourceSpan(span paperlang.Span) layoutengine.SourceSpan {
	return layoutengine.SourceSpan{File: span.File,
		Start: layoutengine.SourcePosition{Offset: span.Start.Offset, Line: span.Start.Line, Column: span.Start.Column},
		End:   layoutengine.SourcePosition{Offset: span.End.Offset, Line: span.End.Line, Column: span.End.Column}}
}

func paperListMarker(block layout.ListBlock, itemIndex int) (string, error) {
	marker := strings.ToLower(strings.TrimSpace(block.MarkerStyle))
	if marker == "" {
		if block.Ordered {
			marker = "decimal"
		} else {
			marker = "dash"
		}
	}
	switch marker {
	case "decimal":
		if !block.Ordered {
			return "", errors.New("decimal marker requires an ordered list")
		}
		return fmt.Sprintf("%d.", paperListCounterValue(block, itemIndex)), nil
	case "lower-alpha", "upper-alpha":
		if !block.Ordered {
			return "", errors.New("alphabetic marker requires an ordered list")
		}
		value, err := paperAlphabeticCounter(paperListCounterValue(block, itemIndex))
		if err != nil {
			return "", err
		}
		if marker == "upper-alpha" {
			value = strings.ToUpper(value)
		}
		return value + ".", nil
	case "lower-roman", "upper-roman":
		if !block.Ordered {
			return "", errors.New("roman marker requires an ordered list")
		}
		value, err := paperRomanCounter(paperListCounterValue(block, itemIndex))
		if err != nil {
			return "", err
		}
		if marker == "lower-roman" {
			value = strings.ToLower(value)
		}
		return value + ".", nil
	case "dash":
		if block.Ordered {
			return "", errors.New("dash marker requires an unordered list")
		}
		return "-", nil
	case "asterisk":
		if block.Ordered {
			return "", errors.New("asterisk marker requires an unordered list")
		}
		return "*", nil
	case "none":
		return "", nil
	default:
		return "", fmt.Errorf("%q is unsupported; use decimal, lower/upper-alpha, lower/upper-roman, dash, asterisk, or none", block.MarkerStyle)
	}
}

func paperListCounterValue(block layout.ListBlock, itemIndex int) int {
	counter := block.Start
	if counter == 0 {
		counter = 1
	}
	for index := 0; index <= itemIndex && index < len(block.Items); index++ {
		if block.Items[index].ValueSet {
			counter = block.Items[index].Value
		}
		if index == itemIndex {
			return counter
		}
		counter++
	}
	return counter
}

func paperAlphabeticCounter(value int) (string, error) {
	if value < 1 || value > 18278 {
		return "", errors.New("alphabetic list counter must be from 1 through 18278")
	}
	var reversed [3]byte
	length := 0
	for value > 0 {
		value--
		reversed[length] = byte('a' + value%26)
		length++
		value /= 26
	}
	result := make([]byte, length)
	for index := range result {
		result[index] = reversed[length-index-1]
	}
	return string(result), nil
}

func paperRomanCounter(value int) (string, error) {
	if value < 1 || value > 3999 {
		return "", errors.New("roman list counter must be from 1 through 3999")
	}
	var out strings.Builder
	for _, part := range []struct {
		value int
		text  string
	}{{1000, "M"}, {900, "CM"}, {500, "D"}, {400, "CD"}, {100, "C"}, {90, "XC"}, {50, "L"}, {40, "XL"}, {10, "X"}, {9, "IX"}, {5, "V"}, {4, "IV"}, {1, "I"}} {
		for value >= part.value {
			out.WriteString(part.text)
			value -= part.value
		}
	}
	return out.String(), nil
}

func newPaperPlanner(page papercompile.PageSpec) (*Document, error) {
	if page.Width <= 0 || page.Height <= 0 || !isFiniteFloat(page.Width) || !isFiniteFloat(page.Height) {
		return nil, errors.New("compiled page size is invalid")
	}
	planner := documentNew("P", "pt", "", ".", Size{Wd: page.Width, Ht: page.Height})
	if planner.err != nil {
		return nil, planner.err
	}
	// Compiled margins are point values. A zero edge is meaningful, so the
	// planner fallback margins must also be zero.
	planner.SetMargins(0, 0, 0)
	planner.SetAutoPageBreak(true, 0)
	return planner, nil
}

func paperDiagnostics(stage PaperRenderStage, diagnostics []paperlang.Diagnostic) []PaperDiagnostic {
	result := make([]PaperDiagnostic, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		result = append(result, PaperDiagnostic{
			Stage: stage, Code: diagnostic.Code, Severity: string(diagnostic.Severity),
			Message: diagnostic.Message, Hint: diagnostic.Hint, File: diagnostic.Span.File,
			StartLine: diagnostic.Span.Start.Line, StartColumn: diagnostic.Span.Start.Column,
			EndLine: diagnostic.Span.End.Line, EndColumn: diagnostic.Span.End.Column,
		})
	}
	return result
}

func paperStageFailure(result PaperRenderResult, stage PaperRenderStage, code string, cause error, file string, root *paperlang.Node) (PaperRenderResult, error) {
	return paperStageFailureWithHint(result, stage, code, cause, "", file, root)
}

func paperStageFailureWithHint(result PaperRenderResult, stage PaperRenderStage, code string, cause error, hint, file string, root *paperlang.Node) (PaperRenderResult, error) {
	span := paperlang.Span{File: file}
	if root != nil {
		span = root.HeaderSpan
	}
	result.Diagnostics = append(result.Diagnostics, paperDiagnostics(stage, []paperlang.Diagnostic{{
		Code: code, Severity: paperlang.SeverityError, Message: cause.Error(), Hint: hint, Span: span,
	}})...)
	return paperRenderFailure(result, stage, cause)
}

func paperStageFailureWithSpan(result PaperRenderResult, stage PaperRenderStage, code string, cause error, hint string, span paperlang.Span) (PaperRenderResult, error) {
	result.Diagnostics = append(result.Diagnostics, paperDiagnostics(stage, []paperlang.Diagnostic{{
		Code: code, Severity: paperlang.SeverityError, Message: cause.Error(), Hint: hint, Span: span,
	}})...)
	return paperRenderFailure(result, stage, cause)
}

func paperPlanStageFailure(result PaperPlanResult, stage PaperRenderStage, code string, cause error, file string, root *paperlang.Node) (PaperPlan, PaperPlanResult, error) {
	return paperPlanStageFailureWithHint(result, stage, code, cause, "", file, root)
}

func paperPlanStageFailureWithHint(result PaperPlanResult, stage PaperRenderStage, code string, cause error, hint, file string, root *paperlang.Node) (PaperPlan, PaperPlanResult, error) {
	span := paperlang.Span{File: file}
	if root != nil {
		span = root.HeaderSpan
	}
	result.Diagnostics = append(result.Diagnostics, paperDiagnostics(stage, []paperlang.Diagnostic{{
		Code: code, Severity: paperlang.SeverityError, Message: cause.Error(), Hint: hint, Span: span,
	}})...)
	return paperPlanFailure(result, stage, cause)
}

func paperPlanFailure(result PaperPlanResult, stage PaperRenderStage, cause error) (PaperPlan, PaperPlanResult, error) {
	result.Pages, result.Hash = 0, ""
	return PaperPlan{}, result, fmt.Errorf("%w at %s stage: %w", ErrPaperRender, stage, cause)
}

func paperRenderFailure(result PaperRenderResult, stage PaperRenderStage, cause error) (PaperRenderResult, error) {
	result.Pages = 0
	return result, fmt.Errorf("%w at %s stage: %w", ErrPaperRender, stage, cause)
}
