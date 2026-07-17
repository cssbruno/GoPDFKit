// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
)

// PaperPlanPageIssue is one plan diagnostic that is positioned on an exact
// page. Parse/typecheck diagnostics without a retained page deliberately do
// not appear here: Studio must not guess their page from source text.
type PaperPlanPageIssue struct {
	Code        string   `json:"code"`
	Severity    string   `json:"severity"`
	Stage       string   `json:"stage"`
	Message     string   `json:"message"`
	Key         string   `json:"key,omitempty"`
	Instance    string   `json:"instance,omitempty"`
	Fragment    uint32   `json:"fragment,omitempty"`
	Region      string   `json:"region,omitempty"`
	StartLine   uint32   `json:"start_line,omitempty"`
	StartColumn uint32   `json:"start_column,omitempty"`
	Bounds      [4]int64 `json:"bounds,omitempty"`
	HasBounds   bool     `json:"has_bounds,omitempty"`
}

// PaperPlanPageSummary is a detached, bounded page-rail projection. Selector
// is the exact first/even/odd page-master selector state, not an invented
// authored master ID. Regions and repeated regions are derived only from
// retained fragments. ContentHash covers the page's geometry, display
// payloads, breaks, diagnostics, semantics, reading order, and provenance.
type PaperPlanPageSummary struct {
	Page            uint32               `json:"page"`
	Selector        string               `json:"selector"`
	Regions         []string             `json:"regions"`
	RepeatedRegions []string             `json:"repeated_regions,omitempty"`
	IssueCount      uint32               `json:"issue_count"`
	IssuesTruncated bool                 `json:"issues_truncated,omitempty"`
	Issues          []PaperPlanPageIssue `json:"issues,omitempty"`
	ContentHash     string               `json:"content_hash"`
}

// PaperPlanPageSummaryLimits bounds the detached page-rail projection while
// preserving exact issue counts and content hashes.
type PaperPlanPageSummaryLimits struct {
	MaxPages         uint32
	MaxIssuesPerPage uint32
}

type paperPlanPageCommandEvidence struct {
	Command     layoutengine.DisplayCommand      `json:"command"`
	GlyphRun    *layoutengine.CoreGlyphRun       `json:"glyph_run,omitempty"`
	Font        *layoutengine.CoreFontResource   `json:"font,omitempty"`
	Image       *layoutengine.PlannedImage       `json:"image,omitempty"`
	ImageSource *layoutengine.ImageResource      `json:"image_source,omitempty"`
	Link        *layoutengine.PlannedLink        `json:"link,omitempty"`
	Destination *layoutengine.PlannedDestination `json:"destination,omitempty"`
	Path        *layoutengine.PlannedPath        `json:"path,omitempty"`
	Transform   *layoutengine.Transform          `json:"transform,omitempty"`
	Clip        *layoutengine.PlannedClip        `json:"clip,omitempty"`
	Fill        *layoutengine.PlannedFill        `json:"fill,omitempty"`
	Stroke      *layoutengine.PlannedStroke      `json:"stroke,omitempty"`
}

type paperPlanPageFingerprint struct {
	Page              layoutengine.PlannedPage                   `json:"page"`
	Fragments         []layoutengine.Fragment                    `json:"fragments,omitempty"`
	Lines             []layoutengine.PlannedLine                 `json:"lines,omitempty"`
	Commands          []paperPlanPageCommandEvidence             `json:"commands,omitempty"`
	Breaks            []layoutengine.BreakDecision               `json:"breaks,omitempty"`
	Diagnostics       []layoutengine.Diagnostic                  `json:"diagnostics,omitempty"`
	Destinations      []layoutengine.PlannedDestination          `json:"destinations,omitempty"`
	SemanticNodes     []layoutengine.SemanticNode                `json:"semantic_nodes,omitempty"`
	SemanticFragments []layoutengine.SemanticFragmentAssociation `json:"semantic_fragments,omitempty"`
	ReadingOrder      []layoutengine.ReadingOccurrence           `json:"reading_order,omitempty"`
	Provenance        []layoutengine.ProvenanceEntry             `json:"provenance,omitempty"`
}

type paperPlanPageIndex struct {
	destinations      [][]layoutengine.PlannedDestination
	breaks            [][]layoutengine.BreakDecision
	diagnostics       [][]layoutengine.Diagnostic
	semanticFragments [][]layoutengine.SemanticFragmentAssociation
	readingOrder      [][]layoutengine.ReadingOccurrence
}

// PageSummaries returns one deterministic entry per immutable page. It is a
// read-only projection and never returns source bytes or plan-owned slices.
func (p PaperPlan) PageSummaries() ([]PaperPlanPageSummary, error) {
	return p.PageSummariesWithLimits(PaperPlanPageSummaryLimits{MaxPages: 10_000, MaxIssuesPerPage: 32})
}

// PageSummariesWithLimits applies explicit positive page and per-page issue
// bounds. A plan larger than MaxPages is rejected rather than partially
// represented; issues are truncated with their exact count retained.
func (p PaperPlan) PageSummariesWithLimits(limits PaperPlanPageSummaryLimits) ([]PaperPlanPageSummary, error) {
	if p.hash == "" || p.pages == 0 {
		return nil, errors.New("document: paper plan has no pages")
	}
	if limits.MaxPages == 0 || limits.MaxPages > 100_000 || limits.MaxIssuesPerPage == 0 || limits.MaxIssuesPerPage > 256 {
		return nil, errors.New("document: paper page summary limits are invalid")
	}
	projection := p.plan.Projection()
	if uint64(len(projection.Pages)) > uint64(limits.MaxPages) {
		return nil, fmt.Errorf("document: paper plan has %d pages, page summary limit is %d", len(projection.Pages), limits.MaxPages)
	}
	indexed := indexPaperPlanPages(projection)
	result := make([]PaperPlanPageSummary, 0, len(projection.Pages))
	for _, page := range projection.Pages {
		fingerprint, regions, repeated, issues, err := paperPageFingerprint(projection, indexed, page)
		if err != nil {
			return nil, err
		}
		encoded, err := json.Marshal(fingerprint)
		if err != nil {
			return nil, fmt.Errorf("document: encode page %d summary: %w", page.Number, err)
		}
		digest := sha256.Sum256(encoded)
		selector := "odd"
		if page.Number == 1 {
			selector = "first"
		} else if page.Number%2 == 0 {
			selector = "even"
		}
		issueCount := len(issues)
		if len(issues) > int(limits.MaxIssuesPerPage) {
			issues = issues[:limits.MaxIssuesPerPage]
		}
		result = append(result, PaperPlanPageSummary{Page: page.Number, Selector: selector,
			Regions: regions, RepeatedRegions: repeated, IssueCount: uint32(issueCount), IssuesTruncated: issueCount > len(issues),
			Issues: append([]PaperPlanPageIssue(nil), issues...), ContentHash: hex.EncodeToString(digest[:])})
	}
	return result, nil
}

func indexPaperPlanPages(projection layoutengine.LayoutPlanProjection) paperPlanPageIndex {
	count := len(projection.Pages) + 1
	indexed := paperPlanPageIndex{
		destinations: make([][]layoutengine.PlannedDestination, count), breaks: make([][]layoutengine.BreakDecision, count),
		diagnostics: make([][]layoutengine.Diagnostic, count), semanticFragments: make([][]layoutengine.SemanticFragmentAssociation, count),
		readingOrder: make([][]layoutengine.ReadingOccurrence, count),
	}
	for _, value := range projection.Destinations {
		indexed.destinations[value.Page] = append(indexed.destinations[value.Page], value)
	}
	for _, value := range projection.Breaks {
		indexed.breaks[value.FromPage] = append(indexed.breaks[value.FromPage], value)
		if value.ToPage != value.FromPage {
			indexed.breaks[value.ToPage] = append(indexed.breaks[value.ToPage], value)
		}
	}
	for _, value := range projection.Diagnostics {
		if value.Location.Page != 0 {
			indexed.diagnostics[value.Location.Page] = append(indexed.diagnostics[value.Location.Page], value)
		}
	}
	for _, value := range projection.SemanticFragments {
		indexed.semanticFragments[value.Page] = append(indexed.semanticFragments[value.Page], value)
	}
	for _, value := range projection.ReadingOrder {
		indexed.readingOrder[value.Page] = append(indexed.readingOrder[value.Page], value)
	}
	return indexed
}

func paperPageFingerprint(projection layoutengine.LayoutPlanProjection, indexed paperPlanPageIndex, page layoutengine.PlannedPage) (paperPlanPageFingerprint, []string, []string, []PaperPlanPageIssue, error) {
	fragmentStart, fragmentEnd, fragmentOK := paperPageSummaryRange(page.Fragments, len(projection.Fragments))
	lineStart, lineEnd, lineOK := paperPageSummaryRange(page.Lines, len(projection.Lines))
	commandStart, commandEnd, commandOK := paperPageSummaryRange(page.Commands, len(projection.Commands))
	if !fragmentOK || !lineOK || !commandOK {
		return paperPlanPageFingerprint{}, nil, nil, nil, fmt.Errorf("document: page %d summary ranges are invalid", page.Number)
	}
	fingerprint := paperPlanPageFingerprint{Page: page,
		Fragments: append([]layoutengine.Fragment(nil), projection.Fragments[fragmentStart:fragmentEnd]...),
		Lines:     append([]layoutengine.PlannedLine(nil), projection.Lines[lineStart:lineEnd]...)}
	regionSet, repeatedSet := map[string]bool{}, map[string]bool{}
	semanticIDs, provenanceIDs := map[layoutengine.SemanticNodeID]bool{}, map[layoutengine.ProvenanceID]bool{}
	for index, fragment := range fingerprint.Fragments {
		regionSet[string(fragment.Region)] = true
		if fragment.Repeated {
			repeatedSet[string(fragment.Region)] = true
		}
		if fragmentStart+index < len(projection.FragmentProvenance) {
			provenanceIDs[projection.FragmentProvenance[fragmentStart+index]] = true
		}
	}
	for index := range fingerprint.Lines {
		if lineStart+index < len(projection.LineProvenance) {
			provenanceIDs[projection.LineProvenance[lineStart+index]] = true
		}
	}
	orderedProvenance := make([]int, 0, len(provenanceIDs))
	for id := range provenanceIDs {
		orderedProvenance = append(orderedProvenance, int(id))
	}
	sort.Ints(orderedProvenance)
	for _, id := range orderedProvenance {
		if id > 0 && id <= len(projection.Provenance) {
			fingerprint.Provenance = append(fingerprint.Provenance, projection.Provenance[id-1])
		}
	}
	for _, command := range projection.Commands[commandStart:commandEnd] {
		evidence := paperPlanPageCommandEvidence{Command: command}
		switch command.Kind {
		case layoutengine.CommandGlyphRun:
			run := projection.GlyphRuns[command.Payload]
			evidence.GlyphRun = &run
			font := projection.Fonts[run.Font-1]
			evidence.Font = &font
		case layoutengine.CommandImage:
			placed := projection.Images[command.Payload]
			evidence.Image = &placed
			resource := projection.ImageResources[placed.Resource-1]
			evidence.ImageSource = &resource
		case layoutengine.CommandLink:
			link := projection.Links[command.Payload]
			evidence.Link = &link
			if link.Destination.Valid() {
				destination := projection.Destinations[link.Destination-1]
				evidence.Destination = &destination
			}
		case layoutengine.CommandTransform:
			value := projection.Transforms[command.Payload]
			evidence.Transform = &value
		case layoutengine.CommandClip:
			value := projection.Clips[command.Payload]
			evidence.Clip = &value
			path := projection.Paths[value.Path]
			evidence.Path = &path
		case layoutengine.CommandFillPath:
			value := projection.Fills[command.Payload]
			evidence.Fill = &value
			path := projection.Paths[value.Path]
			evidence.Path = &path
		case layoutengine.CommandStrokePath:
			value := projection.Strokes[command.Payload]
			evidence.Stroke = &value
			path := projection.Paths[value.Path]
			evidence.Path = &path
		}
		fingerprint.Commands = append(fingerprint.Commands, evidence)
	}
	fingerprint.Destinations = append(fingerprint.Destinations, indexed.destinations[page.Number]...)
	fingerprint.Breaks = append(fingerprint.Breaks, indexed.breaks[page.Number]...)
	issues := make([]PaperPlanPageIssue, 0)
	for _, diagnostic := range indexed.diagnostics[page.Number] {
		fingerprint.Diagnostics = append(fingerprint.Diagnostics, diagnostic)
		location := diagnostic.Location
		issue := PaperPlanPageIssue{Code: string(diagnostic.Code), Severity: string(diagnostic.Severity), Stage: string(diagnostic.Stage),
			Message: diagnostic.Message, Key: string(location.Key), Instance: string(location.Instance), Fragment: uint32(location.Fragment),
			Region: string(location.Region), StartLine: location.Source.Start.Line, StartColumn: location.Source.Start.Column, HasBounds: location.HasBounds}
		if location.HasBounds {
			issue.Bounds = [4]int64{int64(location.Bounds.X), int64(location.Bounds.Y), int64(location.Bounds.Width), int64(location.Bounds.Height)}
		}
		issues = append(issues, issue)
	}
	for _, association := range indexed.semanticFragments[page.Number] {
		fingerprint.SemanticFragments = append(fingerprint.SemanticFragments, association)
		semanticIDs[association.Semantic] = true
	}
	for _, occurrence := range indexed.readingOrder[page.Number] {
		fingerprint.ReadingOrder = append(fingerprint.ReadingOrder, occurrence)
		semanticIDs[occurrence.Semantic] = true
	}
	orderedSemantics := make([]int, 0, len(semanticIDs))
	for id := range semanticIDs {
		orderedSemantics = append(orderedSemantics, int(id))
	}
	sort.Ints(orderedSemantics)
	for _, id := range orderedSemantics {
		if id > 0 && id <= len(projection.SemanticNodes) {
			fingerprint.SemanticNodes = append(fingerprint.SemanticNodes, projection.SemanticNodes[id-1])
		}
	}
	return fingerprint, sortedPageRegions(regionSet), sortedPageRegions(repeatedSet), issues, nil
}

func paperPageSummaryRange(value layoutengine.IndexRange, limit int) (int, int, bool) {
	start, end := uint64(value.Start), uint64(value.Start)+uint64(value.Count)
	if start > uint64(limit) || end > uint64(limit) {
		return 0, 0, false
	}
	return int(start), int(end), true
}

func sortedPageRegions(values map[string]bool) []string {
	regions := make([]string, 0, len(values))
	for value := range values {
		if value != "" {
			regions = append(regions, value)
		}
	}
	order := map[string]int{"header": 0, "body": 1, "footer": 2}
	sort.Slice(regions, func(i, j int) bool {
		left, leftOK := order[regions[i]]
		right, rightOK := order[regions[j]]
		if leftOK != rightOK {
			return leftOK
		}
		if leftOK && left != right {
			return left < right
		}
		return regions[i] < regions[j]
	})
	return regions
}

// PaperPlanSelector is the stable document-level selector used by headless
// plan tools. Selectors combine with AND semantics; zero fields are absent.
type PaperPlanSelector struct {
	DiagnosticCode string `json:"diagnostic_code,omitempty"`
	Node           uint32 `json:"node,omitempty"`
	Key            string `json:"key,omitempty"`
	Instance       string `json:"instance,omitempty"`
	Fragment       uint32 `json:"fragment,omitempty"`
	Page           uint32 `json:"page,omitempty"`
	MaxResults     uint32 `json:"max_results"`
}

// PaperPlanJSON is a detached canonical tool result tied to an exact plan
// hash. JSON returns a fresh copy of the encoded payload.
type PaperPlanJSON struct {
	PlanHash string `json:"plan_hash"`
	payload  []byte
}

func (r PaperPlanJSON) JSON() []byte { return append([]byte(nil), r.payload...) }

// DeterministicInputManifest returns the exact pinned resources, locale,
// timezone, text-data versions, page profile, and internal plan identity bound
// during planning. The result is detached and tied to the public plan hash.
func (p PaperPlan) DeterministicInputManifest() (PaperPlanJSON, error) {
	manifest, ok := p.plan.DeterministicInputs()
	if !ok || p.hash == "" {
		return PaperPlanJSON{}, errors.New("document: paper plan has no deterministic input manifest")
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return PaperPlanJSON{}, fmt.Errorf("document: encode deterministic input manifest: %w", err)
	}
	return PaperPlanJSON{PlanHash: p.hash, payload: encoded}, nil
}

// Query executes a bounded structural query without exposing internal plan
// types or mutable plan storage.
func (p PaperPlan) Query(selector PaperPlanSelector) (PaperPlanJSON, error) {
	result, err := p.plan.QueryStructure(toStructuralQuery(selector))
	if err != nil {
		return PaperPlanJSON{}, err
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return PaperPlanJSON{}, fmt.Errorf("document: encode paper plan query: %w", err)
	}
	return PaperPlanJSON{PlanHash: p.hash, payload: encoded}, nil
}

// Explain returns bounded causal layout evidence for the supplied selectors.
func (p PaperPlan) Explain(selectors []PaperPlanSelector, maxSelectors, maxBytes uint32) (PaperPlanJSON, error) {
	queries := make([]layoutengine.StructuralQuery, len(selectors))
	for index, selector := range selectors {
		queries[index] = toStructuralQuery(selector)
	}
	explanation, err := p.plan.ExplainLayout(queries, layoutengine.ExplainLayoutLimits{
		MaxSelectors: maxSelectors, MaxCanonicalBytes: maxBytes,
	})
	if err != nil {
		return PaperPlanJSON{}, err
	}
	encoded, err := p.explainJSON(explanation.CanonicalJSON, maxBytes)
	if err != nil {
		return PaperPlanJSON{}, err
	}
	return PaperPlanJSON{PlanHash: p.hash, payload: encoded}, nil
}

// ExplainContext is the cancellation-aware and work-bounded explanation
// boundary used by retained-plan services.
func (p PaperPlan) ExplainContext(ctx context.Context, selectors []PaperPlanSelector, maxSelectors, maxBytes uint32, maxWork uint64) (PaperPlanJSON, error) {
	queries := make([]layoutengine.StructuralQuery, len(selectors))
	for index, selector := range selectors {
		queries[index] = toStructuralQuery(selector)
	}
	explanation, err := p.plan.ExplainLayoutContext(ctx, queries, layoutengine.ExplainLayoutLimits{
		MaxSelectors: maxSelectors, MaxCanonicalBytes: maxBytes,
	}, maxWork)
	if err != nil {
		return PaperPlanJSON{}, err
	}
	encoded, err := p.explainJSON(explanation.CanonicalJSON, maxBytes)
	if err != nil {
		return PaperPlanJSON{}, err
	}
	return PaperPlanJSON{PlanHash: p.hash, payload: encoded}, nil
}

// explainJSON adds source-level binding and style-token provenance to the
// exact layout explanation without making the layout engine depend on the
// .paper compiler. The layout explanation remains the authoritative geometry
// projection; provenance is a detached document-adapter field beside it.
func (p PaperPlan) explainJSON(encode func() ([]byte, error), maxBytes uint32) ([]byte, error) {
	encoded, err := encode()
	if err != nil {
		return nil, err
	}
	provenance, err := p.Provenance()
	if err != nil {
		return nil, err
	}
	var base struct {
		SchemaVersion uint16          `json:"schema_version"`
		PlanHash      string          `json:"plan_hash"`
		Limits        json.RawMessage `json:"limits"`
		Targets       json.RawMessage `json:"targets"`
	}
	if err := json.Unmarshal(encoded, &base); err != nil {
		return nil, fmt.Errorf("document: decode paper explanation: %w", err)
	}
	final, err := json.Marshal(struct {
		SchemaVersion uint16              `json:"schema_version"`
		PlanHash      string              `json:"plan_hash"`
		Limits        json.RawMessage     `json:"limits"`
		Targets       json.RawMessage     `json:"targets"`
		Provenance    PaperPlanProvenance `json:"provenance"`
	}{
		SchemaVersion: base.SchemaVersion, PlanHash: base.PlanHash,
		Limits: base.Limits, Targets: base.Targets, Provenance: provenance,
	})
	if err != nil {
		return nil, fmt.Errorf("document: encode paper explanation: %w", err)
	}
	if uint64(len(final)) > uint64(maxBytes) {
		return nil, fmt.Errorf("document: paper explanation exceeds byte limit: encoded=%d limit=%d", len(final), maxBytes)
	}
	return final, nil
}

// HitTest queries one exact fixed-point page coordinate. xFixed and yFixed
// use the plan's stable 1/1024-point unit, avoiding float rounding at an agent
// or editor boundary. Results are detached, bounded by the layout contract,
// and ordered from the visually latest candidate to the earliest.
func (p PaperPlan) HitTest(page uint32, xFixed, yFixed int64) (PaperPlanJSON, error) {
	hit, err := p.plan.HitTestPage(page, layoutengine.Point{
		X: layoutengine.Fixed(xFixed),
		Y: layoutengine.Fixed(yFixed),
	})
	if err != nil {
		return PaperPlanJSON{}, err
	}
	encoded, err := json.Marshal(hit)
	if err != nil {
		return PaperPlanJSON{}, fmt.Errorf("document: encode paper plan hit test: %w", err)
	}
	return PaperPlanJSON{PlanHash: p.hash, payload: encoded}, nil
}

// PaperPlanPixelHitTestRequest binds a zero-based raster pixel to the exact
// fixed page-coordinate crop represented by the raster. This makes clicks in
// screenshots/crops independent of CSS pixels, DPI, and browser scaling.
type PaperPlanPixelHitTestRequest struct {
	Page          uint32 `json:"page"`
	PixelX        uint32 `json:"pixel_x"`
	PixelY        uint32 `json:"pixel_y"`
	PixelWidth    uint32 `json:"pixel_width"`
	PixelHeight   uint32 `json:"pixel_height"`
	CaptureX      int64  `json:"capture_x"`
	CaptureY      int64  `json:"capture_y"`
	CaptureWidth  int64  `json:"capture_width"`
	CaptureHeight int64  `json:"capture_height"`
}

// HitTestPixel maps a declared raster pixel center to exact plan geometry and
// returns bounded source-bearing hits tied to this immutable plan hash.
func (p PaperPlan) HitTestPixel(request PaperPlanPixelHitTestRequest) (PaperPlanJSON, error) {
	hit, err := p.plan.HitTestRasterPixel(layoutengine.RasterPixelQuery{
		Page: request.Page, PixelX: request.PixelX, PixelY: request.PixelY,
		PixelWidth: request.PixelWidth, PixelHeight: request.PixelHeight,
		CaptureBounds: layoutengine.Rect{X: layoutengine.Fixed(request.CaptureX), Y: layoutengine.Fixed(request.CaptureY),
			Width: layoutengine.Fixed(request.CaptureWidth), Height: layoutengine.Fixed(request.CaptureHeight)},
	})
	if err != nil {
		return PaperPlanJSON{}, err
	}
	encoded, err := json.Marshal(hit)
	if err != nil {
		return PaperPlanJSON{}, fmt.Errorf("document: encode paper plan pixel hit test: %w", err)
	}
	return PaperPlanJSON{PlanHash: p.hash, payload: encoded}, nil
}

func toStructuralQuery(selector PaperPlanSelector) layoutengine.StructuralQuery {
	return layoutengine.StructuralQuery{
		DiagnosticCode: layoutengine.DiagnosticCode(selector.DiagnosticCode),
		Node:           layoutengine.NodeID(selector.Node), Key: layoutengine.NodeKey(selector.Key),
		Instance: layoutengine.InstanceID(selector.Instance), Fragment: layoutengine.FragmentID(selector.Fragment),
		Page: selector.Page, MaxResults: selector.MaxResults,
	}
}

// PaperPlanCaptureRequest selects deterministic SVG artifacts generated from
// exact planned coordinates. Mode is "geometry_svg" or "core_text_svg".
type PaperPlanCaptureRequest struct {
	Mode                  string   `json:"mode"`
	IncludeContactSheet   bool     `json:"include_contact_sheet"`
	IncludeCrossPageStrip bool     `json:"include_cross_page_strip"`
	ContactSheetColumns   uint32   `json:"contact_sheet_columns,omitempty"`
	Nodes                 []uint32 `json:"nodes,omitempty"`
	Fragments             []uint32 `json:"fragments,omitempty"`
	MaxPages              uint32   `json:"max_pages"`
	MaxCrops              uint32   `json:"max_crops"`
	MaxArtifactBytes      uint64   `json:"max_artifact_bytes"`
	MaxTotalBytes         uint64   `json:"max_total_bytes"`
	MaxManifestBytes      uint64   `json:"max_manifest_bytes"`
}

// PaperPlanArtifact is a detached visual artifact. MetadataJSON describes the
// exact crop/transform and SVG is a separately copied payload.
type PaperPlanArtifact struct {
	MetadataJSON []byte `json:"metadata_json"`
	SVG          []byte `json:"svg"`
}

type PaperPlanCapture struct {
	PlanHash     string              `json:"plan_hash"`
	ManifestJSON []byte              `json:"manifest_json"`
	Artifacts    []PaperPlanArtifact `json:"artifacts"`
}

// PaperPlanPageSVG is a detached, exact-coordinate page artifact for Studio
// and other read-only clients. Kind is "display" for shared display-list paint
// or "geometry" for the provenance/break overlay. SVG returns a fresh byte
// slice from the capture method; callers cannot mutate the retained plan.
type PaperPlanPageSVG struct {
	PlanHash      string `json:"plan_hash"`
	Kind          string `json:"kind"`
	FormatVersion uint16 `json:"format_version"`
	Page          uint32 `json:"page"`
	PageX         int64  `json:"page_x"`
	PageY         int64  `json:"page_y"`
	PageWidth     int64  `json:"page_width"`
	PageHeight    int64  `json:"page_height"`
	CanvasX       int64  `json:"canvas_x,omitempty"`
	CanvasY       int64  `json:"canvas_y,omitempty"`
	CanvasWidth   int64  `json:"canvas_width,omitempty"`
	CanvasHeight  int64  `json:"canvas_height,omitempty"`
	FixedScale    int64  `json:"fixed_scale"`
	SVG           []byte `json:"-"`
}

// CaptureDisplayPageSVG replays one immutable display-list page into bounded
// SVG without invoking frontend or browser layout. images is keyed by the
// lowercase SHA-256 content digest recorded in the plan; nil is valid for
// pages without image commands.
func (p PaperPlan) CaptureDisplayPageSVG(ctx context.Context, page uint32, images map[string][]byte) (PaperPlanPageSVG, error) {
	if ctx == nil {
		return PaperPlanPageSVG{}, errors.New("document: nil display-page capture context")
	}
	if p.hash == "" || p.pages <= 0 {
		return PaperPlanPageSVG{}, errors.New("document: empty paper plan")
	}
	sources := make(layoutengine.DisplaySVGImageSources, len(images)+len(p.imageSources))
	for digest, encoded := range p.imageSources {
		sources[digest] = append([]byte(nil), encoded...)
	}
	for digest, encoded := range images {
		sources[layoutengine.ImageContentDigest(digest)] = append([]byte(nil), encoded...)
	}
	capture, err := layoutengine.CaptureDisplayPlanSVGContext(ctx, p.plan, page, sources)
	if err != nil {
		return PaperPlanPageSVG{}, err
	}
	return PaperPlanPageSVG{PlanHash: p.hash, Kind: "display", FormatVersion: capture.FormatVersion, Page: capture.Page,
		PageX: int64(capture.PageBounds.X), PageY: int64(capture.PageBounds.Y), PageWidth: int64(capture.PageBounds.Width), PageHeight: int64(capture.PageBounds.Height),
		FixedScale: capture.FixedScale, SVG: append([]byte(nil), capture.SVG...)}, nil
}

// CaptureGeometryPageSVG returns the deterministic source/fragment/break
// overlay for one page. It contains no authored text and is not a substitute
// renderer for the display-list preview.
func (p PaperPlan) CaptureGeometryPageSVG(page uint32) (PaperPlanPageSVG, error) {
	if p.hash == "" || p.pages <= 0 {
		return PaperPlanPageSVG{}, errors.New("document: empty paper plan")
	}
	capture, err := p.plan.CaptureDebugGeometrySVGPage(page)
	if err != nil {
		return PaperPlanPageSVG{}, err
	}
	return PaperPlanPageSVG{PlanHash: p.hash, Kind: "geometry", FormatVersion: capture.FormatVersion, Page: capture.Page,
		PageX: int64(capture.PageBounds.X), PageY: int64(capture.PageBounds.Y), PageWidth: int64(capture.PageBounds.Width), PageHeight: int64(capture.PageBounds.Height),
		CanvasX: int64(capture.CanvasBounds.X), CanvasY: int64(capture.CanvasBounds.Y), CanvasWidth: int64(capture.CanvasBounds.Width), CanvasHeight: int64(capture.CanvasBounds.Height),
		FixedScale: capture.FixedScale, SVG: append([]byte(nil), capture.SVG...)}, nil
}

// Capture creates deterministic, bounded agent visual artifacts.
func (p PaperPlan) Capture(request PaperPlanCaptureRequest) (PaperPlanCapture, error) {
	nodes := make([]layoutengine.NodeID, len(request.Nodes))
	for index, node := range request.Nodes {
		nodes[index] = layoutengine.NodeID(node)
	}
	fragments := make([]layoutengine.FragmentID, len(request.Fragments))
	for index, fragment := range request.Fragments {
		fragments[index] = layoutengine.FragmentID(fragment)
	}
	bundle, err := layoutengine.CaptureAgentVisualArtifacts(p.plan, layoutengine.AgentVisualRequest{
		Mode: layoutengine.AICaptureMode(request.Mode), IncludeContactSheet: request.IncludeContactSheet,
		IncludeCrossPageStrip: request.IncludeCrossPageStrip,
		ContactSheetColumns:   request.ContactSheetColumns, Nodes: nodes, Fragments: fragments,
		Revisions: p.revisions,
		Limits: layoutengine.AgentVisualLimits{MaxPages: request.MaxPages, MaxCrops: request.MaxCrops,
			MaxArtifactBytes: request.MaxArtifactBytes, MaxTotalBytes: request.MaxTotalBytes,
			MaxManifestBytes: request.MaxManifestBytes},
	})
	if err != nil {
		return PaperPlanCapture{}, err
	}
	manifest, err := bundle.CanonicalJSON()
	if err != nil {
		return PaperPlanCapture{}, err
	}
	artifacts := bundle.Artifacts()
	result := PaperPlanCapture{PlanHash: p.hash, ManifestJSON: append([]byte(nil), manifest...), Artifacts: make([]PaperPlanArtifact, len(artifacts))}
	for index, artifact := range artifacts {
		metadata, marshalErr := json.Marshal(artifact.Metadata)
		if marshalErr != nil {
			return PaperPlanCapture{}, fmt.Errorf("document: encode paper capture metadata: %w", marshalErr)
		}
		result.Artifacts[index] = PaperPlanArtifact{MetadataJSON: metadata, SVG: append([]byte(nil), artifact.SVG...)}
	}
	return result, nil
}
