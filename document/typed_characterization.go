// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"

	"github.com/cssbruno/gopdfkit/inspect"
	"github.com/cssbruno/gopdfkit/internal/layoutengine"
	"github.com/cssbruno/gopdfkit/layout"
)

const TypedCharacterizationSchemaVersion uint16 = 1
const TypedCharacterizationProjectionSchemaVersion uint16 = 3

type TypedBehaviorStatus string

const (
	TypedBehaviorDocumented  TypedBehaviorStatus = "documented"
	TypedBehaviorAccidental  TypedBehaviorStatus = "accidental"
	TypedBehaviorDeprecated  TypedBehaviorStatus = "deprecated"
	TypedBehaviorUnsupported TypedBehaviorStatus = "unsupported"
)

type TypedEntryPointInventory struct {
	Package   string              `json:"package"`
	Receiver  string              `json:"receiver,omitempty"`
	Name      string              `json:"name"`
	Signature string              `json:"signature"`
	Status    TypedBehaviorStatus `json:"status"`
	Notes     string              `json:"notes,omitempty"`
}

type TypedFieldInventory struct {
	Name   string              `json:"name"`
	Type   string              `json:"type"`
	Status TypedBehaviorStatus `json:"status"`
}

type TypedBlockInventory struct {
	Kind       layout.BlockKind      `json:"kind"`
	GoType     string                `json:"go_type"`
	Status     TypedBehaviorStatus   `json:"status"`
	PlanStatus TypedBehaviorStatus   `json:"exact_plan_status"`
	Fields     []TypedFieldInventory `json:"fields"`
}

type TypedBehaviorInventory struct {
	Scope  string              `json:"scope"`
	Name   string              `json:"name"`
	Status TypedBehaviorStatus `json:"status"`
	Notes  string              `json:"notes,omitempty"`
}

type TypedFixtureInventory struct {
	Name     string             `json:"name"`
	Coverage []string           `json:"coverage"`
	Blocks   []layout.BlockKind `json:"blocks,omitempty"`
}

type TypedCharacterizationInventory struct {
	SchemaVersion uint16                     `json:"schema_version"`
	EntryPoints   []TypedEntryPointInventory `json:"entry_points"`
	Blocks        []TypedBlockInventory      `json:"blocks"`
	Behaviors     []TypedBehaviorInventory   `json:"behaviors"`
	Fixtures      []TypedFixtureInventory    `json:"fixtures"`
}

var typedBlockTypes = []struct {
	kind   layout.BlockKind
	typeOf reflect.Type
	plan   TypedBehaviorStatus
}{
	{layout.BlockKindParagraph, reflect.TypeOf(layout.ParagraphBlock{}), TypedBehaviorDocumented},
	{layout.BlockKindHeading, reflect.TypeOf(layout.HeadingBlock{}), TypedBehaviorDocumented},
	{layout.BlockKindList, reflect.TypeOf(layout.ListBlock{}), TypedBehaviorDocumented},
	{layout.BlockKindTable, reflect.TypeOf(layout.TableBlock{}), TypedBehaviorDocumented},
	{layout.BlockKindImage, reflect.TypeOf(layout.ImageBlock{}), TypedBehaviorDocumented},
	{layout.BlockKindSignatureRow, reflect.TypeOf(layout.SignatureRowBlock{}), TypedBehaviorDocumented},
	{layout.BlockKindMetadataGrid, reflect.TypeOf(layout.MetadataGridBlock{}), TypedBehaviorDocumented},
	{layout.BlockKindQRVerification, reflect.TypeOf(layout.QRVerificationBlock{}), TypedBehaviorDocumented},
	{layout.BlockKindNoteBox, reflect.TypeOf(layout.NoteBoxBlock{}), TypedBehaviorDocumented},
	{layout.BlockKindSection, reflect.TypeOf(layout.SectionBlock{}), TypedBehaviorDocumented},
	{layout.BlockKindClause, reflect.TypeOf(layout.ClauseBlock{}), TypedBehaviorDocumented},
	{layout.BlockKindPageBreak, reflect.TypeOf(layout.PageBreakBlock{}), TypedBehaviorDocumented},
	{layout.BlockKindRowColumn, reflect.TypeOf(layout.RowColumnBlock{}), TypedBehaviorDocumented},
	// Canvas is a documented local, bounded anchor DAG. It deliberately does
	// not imply support for document-wide constraints or intrinsic item sizing.
	{layout.BlockKindCanvas, reflect.TypeOf(layout.CanvasBlock{}), TypedBehaviorDocumented},
}

func TypedLayoutInventory() TypedCharacterizationInventory {
	entries := []TypedEntryPointInventory{
		{Package: "layout", Name: "NewLayoutDocument", Signature: "func() *LayoutDocument", Status: TypedBehaviorDocumented},
		{Package: "layout", Name: "NewDocumentModel", Signature: "func(string, ...Block) *LayoutDocument", Status: TypedBehaviorDocumented},
		{Package: "layout", Receiver: "*LayoutDocument", Name: "AddBlock", Signature: "func(Block)", Status: TypedBehaviorDocumented},
		{Package: "layout", Name: "NormalizeBlock", Signature: "func(Block) (Block, bool)", Status: TypedBehaviorDocumented},
		{Package: "layout", Name: "NormalizeBlocks", Signature: "func([]Block) []Block", Status: TypedBehaviorDocumented},
		{Package: "document", Receiver: "*Document", Name: "WriteDocument", Signature: "func(*layout.LayoutDocument)", Status: TypedBehaviorDocumented, Notes: "unified immutable-plan lowering adapter; unsupported receiver/model contracts fail without a legacy renderer"},
		{Package: "document", Receiver: "*Document", Name: "PlanLayoutDocument", Signature: "func(*layout.LayoutDocument) (LayoutDocumentPlan, error)", Status: TypedBehaviorDocumented},
		{Package: "document", Receiver: "*Document", Name: "PlanLayoutDocumentContext", Signature: "func(context.Context, *layout.LayoutDocument) (LayoutDocumentPlan, error)", Status: TypedBehaviorDocumented},
		{Package: "document", Receiver: "*Document", Name: "WriteLayoutDocumentPlan", Signature: "func(LayoutDocumentPlan) (int, error)", Status: TypedBehaviorDocumented},
		{Package: "document", Receiver: "*Document", Name: "WriteLayoutDocumentPlanContext", Signature: "func(context.Context, LayoutDocumentPlan) (int, error)", Status: TypedBehaviorDocumented},
	}
	blocks := make([]TypedBlockInventory, 0, len(typedBlockTypes))
	for _, item := range typedBlockTypes {
		fields := make([]TypedFieldInventory, item.typeOf.NumField())
		for index := range fields {
			field := item.typeOf.Field(index)
			fields[index] = TypedFieldInventory{Name: field.Name, Type: field.Type.String(), Status: TypedBehaviorDocumented}
		}
		blocks = append(blocks, TypedBlockInventory{Kind: item.kind, GoType: item.typeOf.Name(),
			Status: TypedBehaviorDocumented, PlanStatus: item.plan, Fields: fields})
	}
	behaviors := []TypedBehaviorInventory{
		{Scope: "PageTemplate.Header", Name: "default odd-page header with exact measured region height", Status: TypedBehaviorDocumented},
		{Scope: "PageTemplate.FirstPageHeader", Name: "first-page header overrides the default header", Status: TypedBehaviorDocumented},
		{Scope: "PageTemplate.Footer", Name: "default odd-page footer with exact measured region height", Status: TypedBehaviorDocumented},
		{Scope: "PageTemplate.EvenPageFooter", Name: "even-page footer overrides the default footer", Status: TypedBehaviorDocumented},
		{Scope: "PageTemplate.PageNumbers", Name: "bounded corrected current/total page counters", Status: TypedBehaviorDocumented},
		{Scope: "TableBlock.Columns", Name: "exact mixed fixed and bounded intrinsic/min/max tracks with colspan contributions", Status: TypedBehaviorDocumented},
		{Scope: "TableBlock.CellSpans", Name: "bounded rectangular colspan and rowspan occupancy", Status: TypedBehaviorDocumented},
		{Scope: "TableBlock.Header", Name: "header rows repeat on continuation pages", Status: TypedBehaviorDocumented},
		{Scope: "TableBlock.Pagination", Name: "rowspan groups, causal breaks, and oversized-row progress", Status: TypedBehaviorDocumented},
		{Scope: "Text.CoreFonts", Name: "deterministic core-font shaping, wrapping, links, and reading order", Status: TypedBehaviorDocumented},
		{Scope: "Text.UnicodeCoreFonts", Name: "characters outside the selected core-font repertoire", Status: TypedBehaviorUnsupported},
		{Scope: "TextSegment.Link", Name: "canonical external http, https, and mailto links", Status: TypedBehaviorDocumented},
		{Scope: "TextSegment.Destination", Name: "canonical named internal destinations and #name links resolved from finalized glyph geometry", Status: TypedBehaviorDocumented},
		{Scope: "ImageBlock", Name: "bounded content-addressed inline PNG and JPEG", Status: TypedBehaviorDocumented},
		{Scope: "HTML.SVG", Name: "HTML inline SVG uses bounded unified display-plan lowering; unsupported SVG contracts are rejected", Status: TypedBehaviorDocumented},
		{Scope: "HTML.Forms", Name: "HTML form controls are rejected by strict unified planning", Status: TypedBehaviorUnsupported},
		{Scope: "QRVerificationBlock", Name: "bounded content-addressed QR image plus verification text and link", Status: TypedBehaviorDocumented},
		{Scope: "HeadingBlock.Level", Name: "out-of-range levels are currently accepted by exact planning", Status: TypedBehaviorAccidental},
		{Scope: "ParagraphBlock", Name: "an indivisible line over an empty body is emitted once with oversized-line evidence", Status: TypedBehaviorDocumented},
		{Scope: "TableBlock", Name: "sparse rows materialize deterministic empty cells while bounded colspan and rowspan occupancy remains exact", Status: TypedBehaviorDocumented},
		{Scope: "WriteDocument", Name: "supported fresh documents use immutable unified planning; unsupported receiver or model contracts store an error without a legacy renderer", Status: TypedBehaviorDocumented},
	}
	return TypedCharacterizationInventory{SchemaVersion: TypedCharacterizationSchemaVersion,
		EntryPoints: entries, Blocks: blocks, Behaviors: behaviors, Fixtures: typedFixtureInventory()}
}

func (inventory TypedCharacterizationInventory) CanonicalJSON() ([]byte, error) {
	if inventory.SchemaVersion != TypedCharacterizationSchemaVersion {
		return nil, errors.New("document: typed characterization schema is invalid")
	}
	return json.Marshal(inventory)
}

func (inventory TypedCharacterizationInventory) Hash() (string, error) {
	encoded, err := inventory.CanonicalJSON()
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

type TypedCharacterizationLimits struct {
	MaxFixtures uint32
	MaxWork     uint64
}

func DefaultTypedCharacterizationLimits() TypedCharacterizationLimits {
	return TypedCharacterizationLimits{MaxFixtures: 64, MaxWork: 1 << 20}
}

type TypedFixtureResult struct {
	Name         string                          `json:"name"`
	Outcome      string                          `json:"outcome"`
	Pages        int                             `json:"pages"`
	PlanHash     string                          `json:"plan_hash,omitempty"`
	BreakLedger  []layoutengine.BreakDecision    `json:"break_ledger,omitempty"`
	ReadingRoles []layoutengine.SemanticRole     `json:"reading_roles"`
	PDF          *CharacterizationPDFEvidence    `json:"pdf,omitempty"`
	RasterStatus string                          `json:"raster_status"`
	Raster       *CharacterizationRasterEvidence `json:"raster,omitempty"`
}

type CharacterizationPDFEvidence struct {
	SHA256          string   `json:"sha256"`
	Bytes           uint64   `json:"bytes"`
	Text            string   `json:"text"`
	PageText        []string `json:"page_text"`
	Links           uint32   `json:"links"`
	Destinations    uint32   `json:"destinations"`
	Widgets         uint32   `json:"widgets"`
	Attachments     uint32   `json:"attachments"`
	StructureTrees  uint32   `json:"structure_trees"`
	MarkedContent   uint32   `json:"marked_content"`
	HasAcroForm     bool     `json:"has_acro_form"`
	HasTaggedMarker bool     `json:"has_tagged_marker"`
}

type TypedCharacterizationProjection struct {
	SchemaVersion uint16               `json:"schema_version"`
	InventoryHash string               `json:"inventory_hash"`
	Fixtures      []TypedFixtureResult `json:"fixtures"`
}

// RunTypedCharacterization executes the bounded exact-planner corpus and
// paints successful immutable plans into independent deterministic PDFs.
// Unsupported/rejected fixtures are recorded as outcomes rather than hidden.
func RunTypedCharacterization(ctx context.Context, limits TypedCharacterizationLimits) (TypedCharacterizationProjection, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if limits == (TypedCharacterizationLimits{}) {
		limits = DefaultTypedCharacterizationLimits()
	}
	fixtures := typedCharacterizationFixtures()
	if limits.MaxFixtures == 0 || limits.MaxWork == 0 || uint64(len(fixtures)) > uint64(limits.MaxFixtures) {
		return TypedCharacterizationProjection{}, errors.New("document: typed characterization limit exceeded")
	}
	inventoryHash, err := TypedLayoutInventory().Hash()
	if err != nil {
		return TypedCharacterizationProjection{}, err
	}
	result := TypedCharacterizationProjection{SchemaVersion: TypedCharacterizationProjectionSchemaVersion, InventoryHash: inventoryHash,
		Fixtures: make([]TypedFixtureResult, 0, len(fixtures))}
	var work uint64
	rasterBudget := characterizationRasterBudget{}
	for _, fixture := range fixtures {
		if err := ctx.Err(); err != nil {
			return TypedCharacterizationProjection{}, err
		}
		work += typedCharacterizationDocumentWork(fixture.doc)
		if work > limits.MaxWork {
			return TypedCharacterizationProjection{}, errors.New("document: typed characterization work limit exceeded")
		}
		planner := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 200, Ht: fixture.pageHeight}), WithNoCompression())
		fixtureContext := ctx
		if fixture.mode == "cancel" {
			canceled, cancel := context.WithCancel(ctx)
			cancel()
			fixtureContext = canceled
		}
		if fixture.mode == "limit" {
			bounded, limitErr := WithPlanningWorkLimit(ctx, 1)
			if limitErr != nil {
				return TypedCharacterizationProjection{}, limitErr
			}
			fixtureContext = bounded
		}
		plan, planErr := planner.PlanLayoutDocumentContext(fixtureContext, fixture.doc)
		entry := TypedFixtureResult{Name: fixture.inventory.Name, RasterStatus: "not-applicable"}
		switch {
		case fixture.mode == "cancel" && errors.Is(planErr, context.Canceled):
			entry.Outcome = "canceled"
		case fixture.mode == "limit" && planErr != nil:
			entry.Outcome = "resource-limit"
		case fixture.mode == "malformed" && planErr != nil:
			entry.Outcome = "rejected"
		case fixture.mode == "malformed" && planErr == nil:
			entry.Outcome, entry.Pages, entry.PlanHash = "accepted-malformed", plan.PageCount(), plan.Hash()
		case planErr == nil:
			entry.Outcome, entry.Pages, entry.PlanHash = "planned", plan.PageCount(), plan.Hash()
		case errors.Is(planErr, ErrLayoutDocumentPlanUnsupported):
			entry.Outcome = "unsupported"
		default:
			entry.Outcome = "rejected"
		}
		if planErr == nil {
			entry.BreakLedger = append([]layoutengine.BreakDecision(nil), plan.plan.Projection().Breaks...)
			evidence, roles, evidenceErr := typedCharacterizationPDFEvidence(plan, fixture.pageHeight)
			if evidenceErr != nil {
				return TypedCharacterizationProjection{}, evidenceErr
			}
			entry.PDF, entry.ReadingRoles = &evidence, roles
			raster, rasterStatus, rasterErr := captureCharacterizationRaster(ctx, fixture.inventory.Name, plan, &rasterBudget)
			if rasterErr != nil {
				return TypedCharacterizationProjection{}, rasterErr
			}
			entry.Raster, entry.RasterStatus = raster, rasterStatus
		}
		if fixture.mode == "concurrent" && planErr == nil {
			var wait sync.WaitGroup
			failures := make(chan error, 16)
			for worker := 0; worker < 16; worker++ {
				wait.Add(1)
				go func() {
					defer wait.Done()
					target := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 200, Ht: fixture.pageHeight}), WithNoCompression())
					if _, err := target.WriteLayoutDocumentPlan(plan); err != nil {
						failures <- err
					}
				}()
			}
			wait.Wait()
			close(failures)
			if err := <-failures; err != nil {
				entry.Outcome, entry.Pages, entry.PlanHash = "rejected", 0, ""
				entry.BreakLedger = nil
			}
		}
		result.Fixtures = append(result.Fixtures, entry)
	}
	return result, nil
}

func typedCharacterizationPDFEvidence(plan LayoutDocumentPlan, pageHeight float64) (CharacterizationPDFEvidence, []layoutengine.SemanticRole, error) {
	target := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 200, Ht: pageHeight}), WithNoCompression(), WithDeterministicOutput())
	if _, err := target.WriteLayoutDocumentPlan(plan); err != nil {
		return CharacterizationPDFEvidence{}, nil, err
	}
	var output bytes.Buffer
	if err := target.OutputWithOptions(&output, OutputOptions{Deterministic: true}); err != nil {
		return CharacterizationPDFEvidence{}, nil, err
	}
	pdf := output.Bytes()
	evidence, err := characterizationPDFOutputEvidence(pdf, plan.PageCount())
	if err != nil {
		return CharacterizationPDFEvidence{}, nil, err
	}
	projection := plan.plan.Projection()
	roleByID := make(map[layoutengine.SemanticNodeID]layoutengine.SemanticRole, len(projection.SemanticNodes))
	for _, semantic := range projection.SemanticNodes {
		roleByID[semantic.ID] = semantic.Role
	}
	roles := make([]layoutengine.SemanticRole, len(projection.ReadingOrder))
	for index, occurrence := range projection.ReadingOrder {
		roles[index] = roleByID[occurrence.Semantic]
	}
	return evidence, roles, nil
}

func characterizationPDFOutputEvidence(pdf []byte, pages int) (CharacterizationPDFEvidence, error) {
	pageText := make([]string, pages)
	var text strings.Builder
	for page := 1; page <= pages; page++ {
		value, err := inspect.PageText(pdf, page)
		if err != nil {
			return CharacterizationPDFEvidence{}, err
		}
		pageText[page-1] = value
		text.WriteString(value)
	}
	digest := sha256.Sum256(pdf)
	count := func(token string) uint32 { return uint32(bytes.Count(pdf, []byte(token))) } // #nosec G115 -- low-width representation is explicitly normalized before packing
	return CharacterizationPDFEvidence{SHA256: hex.EncodeToString(digest[:]), Bytes: uint64(len(pdf)), Text: text.String(), PageText: pageText,
		Links: count("/Subtype /Link"), Destinations: count("/Dest "), Widgets: count("/Subtype /Widget"),
		Attachments: count("/Filespec"), StructureTrees: count("/StructTreeRoot"), MarkedContent: count(" BDC"),
		HasAcroForm: bytes.Contains(pdf, []byte("/AcroForm")), HasTaggedMarker: bytes.Contains(pdf, []byte("/Marked true"))}, nil
}

func (projection TypedCharacterizationProjection) CanonicalJSON() ([]byte, error) {
	return json.Marshal(projection)
}
