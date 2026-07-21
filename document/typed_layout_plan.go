// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cssbruno/paperrune/internal/layoutengine"
	"github.com/cssbruno/paperrune/internal/papercompile"
	"github.com/cssbruno/paperrune/layout"
)

// ErrLayoutDocumentPlanUnsupported reports that a typed LayoutDocument uses a
// contract which the unified exact planner does not yet represent. Callers can
// use errors.Is; the returned error text includes a deterministic reason and
// model path without exposing private migration implementation vocabulary.
var ErrLayoutDocumentPlanUnsupported = errors.New("document: layout document plan unsupported")

// LayoutDocumentPlan is an immutable, reusable exact plan for the supported
// typed LayoutDocument subset. Its representation stays private so the public
// document package does not expose layoutengine's evolving storage schema.
type LayoutDocumentPlan struct {
	plan         layoutengine.LayoutPlan
	tree         layoutengine.CanonicalTree
	hash         string
	pages        int
	imageSources plannedImageSources
	fontSources  plannedFontSources
	envelope     typedLayoutDocumentEnvelope
}

// typedLayoutDocumentEnvelope is deliberately private: layout nodes must not
// acquire PDF serialization, output, compliance, or signing concerns. Every
// reference-bearing value in this record is detached while planning so a plan
// can be reused after its source Document and LayoutDocument are mutated.
// Signing keys and certificates are intentionally absent; callers supply live
// credentials only to OutputSigned* after the plan has been painted.
type typedLayoutDocumentEnvelope struct {
	producer       string
	title          string
	subject        string
	author         string
	keywords       string
	creator        string
	creationDate   time.Time
	modDate        time.Time
	xmp            []byte
	compliance     ComplianceMetadata
	outputIntent   outputIntent
	outputPolicy   OutputPolicy
	pdfVersion     string
	catalogSort    bool
	tagged         bool
	signatureField string
	attachments    []Attachment
}

type typedLayoutEnvelopeHash struct {
	Schema         string             `json:"schema"`
	LayoutHash     string             `json:"layout_hash"`
	Producer       string             `json:"producer"`
	Title          string             `json:"title"`
	Subject        string             `json:"subject"`
	Author         string             `json:"author"`
	Keywords       string             `json:"keywords"`
	Creator        string             `json:"creator"`
	XMP            []byte             `json:"xmp"`
	Compliance     ComplianceMetadata `json:"compliance"`
	OutputIntent   typedOutputIntent  `json:"output_intent"`
	OutputPolicy   OutputPolicy       `json:"output_policy"`
	PDFVersion     string             `json:"pdf_version"`
	Tagged         bool               `json:"tagged"`
	SignatureField string             `json:"signature_field"`
	Attachments    []typedAttachment  `json:"attachments"`
}

type typedOutputIntent struct {
	ICCProfile []byte `json:"icc_profile"`
	Identifier string `json:"identifier"`
	Info       string `json:"info"`
}

type typedAttachment struct {
	Content        []byte `json:"content"`
	Filename       string `json:"filename"`
	Description    string `json:"description"`
	MIMEType       string `json:"mime_type"`
	AFRelationship string `json:"af_relationship"`
}

// PageCount returns the number of immutable pages in the plan.
func (p LayoutDocumentPlan) PageCount() int { return p.pages }

// Hash returns the canonical SHA-256 plan hash, or an empty string for a zero
// LayoutDocumentPlan.
func (p LayoutDocumentPlan) Hash() string { return p.hash }

// Capture creates the same deterministic, bounded SVG artifacts available for
// .paper plans while retaining the typed plan's exact hash.
func (p LayoutDocumentPlan) Capture(request PaperPlanCaptureRequest) (PaperPlanCapture, error) {
	return (PaperPlan{plan: p.plan, hash: p.hash}).Capture(request)
}

// LayoutDocumentDisplayCapture is a detached exact mixed-content page preview.
// SVG returns a fresh copy so callers cannot mutate a reusable plan result.
type LayoutDocumentDisplayCapture struct {
	PlanHash string `json:"plan_hash"`
	Page     uint32 `json:"page"`
	svg      []byte
}

func (capture LayoutDocumentDisplayCapture) SVG() []byte {
	return append([]byte(nil), capture.svg...)
}

// CaptureDisplayPage verifies content-addressed image bytes and emits the
// exact planned text/image display list for one page as bounded SVG. It never
// measures, fits, wraps, or paginates again.
func (p LayoutDocumentPlan) CaptureDisplayPage(page uint32) (LayoutDocumentDisplayCapture, error) {
	return p.CaptureDisplayPageContext(context.Background(), page)
}

func (p LayoutDocumentPlan) CaptureDisplayPageContext(ctx context.Context, page uint32) (LayoutDocumentDisplayCapture, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return LayoutDocumentDisplayCapture{}, err
	}
	sources := make(layoutengine.DisplaySVGImageSources, len(p.imageSources))
	for digest, encoded := range p.imageSources {
		if err := ctx.Err(); err != nil {
			return LayoutDocumentDisplayCapture{}, err
		}
		sources[digest] = encoded
	}
	capture, err := layoutengine.CaptureDisplayPlanSVGContext(ctx, p.plan, page, sources)
	if err != nil {
		return LayoutDocumentDisplayCapture{}, err
	}
	return LayoutDocumentDisplayCapture{PlanHash: p.hash, Page: capture.Page, svg: append([]byte(nil), capture.SVG...)}, nil
}

// PlanLayoutDocument lowers the supported typed LayoutDocument subset through
// the same exact planner used by .paper. Planning is read-only: it neither
// opens a page nor mutates the supplied model. The receiver supplies page,
// margin, core-font, and measurement configuration and must be fresh.
func (f *Document) PlanLayoutDocument(doc *layout.LayoutDocument) (LayoutDocumentPlan, error) {
	return f.PlanLayoutDocumentContext(context.Background(), doc)
}

// PlanLayoutDocumentContext is PlanLayoutDocument with cooperative
// cancellation for bounded planners such as tables.
func (f *Document) PlanLayoutDocumentContext(ctx context.Context, doc *layout.LayoutDocument) (LayoutDocumentPlan, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return LayoutDocumentPlan{}, err
		}
	}
	var budgetErr error
	ctx, budgetErr = ensureDocumentPlanningBudget(ctx)
	if budgetErr != nil {
		return LayoutDocumentPlan{}, budgetErr
	}
	if err := layoutengine.ChargePlanningWork(ctx, "typed document planning", 1); err != nil {
		return LayoutDocumentPlan{}, err
	}
	if err := f.validateLayoutDocumentPlanEnvelope(doc); err != nil {
		return LayoutDocumentPlan{}, layoutDocumentPlanError(err)
	}
	tree, treeErr := papercompile.LowerLayoutDocumentTreeContext(ctx, doc, layoutengine.CanonicalTreeLimits{})
	if treeErr != nil {
		return LayoutDocumentPlan{}, fmt.Errorf("document: lower typed canonical tree: %w", treeErr)
	}
	working := *doc
	working.Body = append([]layout.Block(nil), doc.Body...)
	envelope, err := f.snapshotLayoutDocumentEnvelope(doc)
	if err != nil {
		return LayoutDocumentPlan{}, layoutDocumentPlanError(err)
	}
	working.Attachments = nil
	if doc.Signature != nil {
		signatureRows := make([]layout.Block, 0, len(doc.Signature.Rows))
		for _, row := range doc.Signature.Rows {
			signatureRows = append(signatureRows, row)
		}
		if doc.Signature.KeepTogether {
			working.Body = append(working.Body, layout.SectionBlock{Blocks: signatureRows, Box: layout.BoxStyle{KeepTogether: true}})
		} else {
			working.Body = append(working.Body, signatureRows...)
		}
		working.Signature = nil
	}
	if doc.QR != nil {
		working.Body = append(working.Body, layout.QRVerificationBlock{QR: *doc.QR})
		working.QR = nil
	}
	var planned layoutengine.LayoutPlan
	blocks := layout.NormalizeBlocks(working.Body)
	hasTable := typedBlocksContainTable(blocks)
	needsMixedBox := typedBlocksNeedMixedBoxContainers(blocks)
	if len(blocks) == 1 && typedShadowTemplateHasOnlyMargins(working.PageTemplate) {
		if canvas, ok := blocks[0].(layout.CanvasBlock); ok {
			planned, err = f.planPaperCanvas(ctx, &working, papercompile.CompileMapping{}, canvas)
		} else if table, ok := blocks[0].(layout.TableBlock); ok {
			planned, err = f.planTypedTable(ctx, &working, table, "body[0]")
		} else if !hasTable && !needsMixedBox {
			planned, err = f.planPaperTextBlocksContext(ctx, &working)
		} else {
			planned, err = f.planTypedMixedBodies(ctx, &working, nil)
		}
	} else if !typedShadowTemplateHasOnlyMargins(working.PageTemplate) {
		planned, err = f.planTypedPageTemplate(ctx, &working, papercompile.CompileMapping{})
	} else {
		if hasTable {
			planned, err = f.planTypedMixedBodies(ctx, &working, nil)
		} else {
			planned, err = f.planPaperTextBlocksContext(ctx, &working)
		}
	}
	if err != nil {
		return LayoutDocumentPlan{}, layoutDocumentPlanError(err)
	}
	planned, err = bindTypedDeterministicInputs(planned, tree, doc)
	if err != nil {
		return LayoutDocumentPlan{}, fmt.Errorf("document: bind typed deterministic inputs: %w", err)
	}
	layoutHash, err := planned.Hash()
	if err != nil {
		return LayoutDocumentPlan{}, fmt.Errorf("document: hash typed layout plan: %w", err)
	}
	hash, err := hashTypedLayoutDocumentEnvelope(layoutHash.String(), envelope)
	if err != nil {
		return LayoutDocumentPlan{}, fmt.Errorf("document: hash typed document envelope: %w", err)
	}
	pages := len(planned.Projection().Pages)
	if pages == 0 {
		return LayoutDocumentPlan{}, newTypedShadowUnsupported(typedShadowDocumentEnvelope, "typed layout produced no pages")
	}
	imageSources, err := typedLayoutImageSourcesContext(ctx, &working, uint64(f.imageSourceLimit()))
	if err != nil {
		return LayoutDocumentPlan{}, fmt.Errorf("document: build bounded image resource catalog: %w", err)
	}
	fontSources, err := f.typedLayoutFontSourcesContext(ctx, planned)
	if err != nil {
		return LayoutDocumentPlan{}, fmt.Errorf("document: build bounded font resource catalog: %w", err)
	}
	return LayoutDocumentPlan{
		plan: planned, tree: tree, hash: hash, pages: pages,
		imageSources: imageSources, fontSources: fontSources, envelope: envelope,
	}, nil
}

func bindTypedDeterministicInputs(plan layoutengine.LayoutPlan, tree layoutengine.CanonicalTree, doc *layout.LayoutDocument) (layoutengine.LayoutPlan, error) {
	// The typed lowering contract guarantees empty SourceSpan values (the
	// model has no source-file locations), so the exact canonical tree hash is
	// already the semantic template identity. This avoids materializing a
	// second source-stripped tree projection for large tables.
	templateHash, err := tree.Hash()
	if err != nil {
		return layoutengine.LayoutPlan{}, fmt.Errorf("derive typed template identity: %w", err)
	}
	template, err := layoutengine.ParseSemanticTemplateID(templateHash)
	if err != nil {
		return layoutengine.LayoutPlan{}, err
	}
	scenarioDigest := sha256.Sum256([]byte("paperrune.typed-layout-scenario.v1\x00" + templateHash))
	scenario, err := layoutengine.ParseScenarioRevisionID(hex.EncodeToString(scenarioDigest[:]))
	if err != nil {
		return layoutengine.LayoutPlan{}, err
	}
	locale := strings.TrimSpace(doc.Language)
	if locale == "" {
		locale = "und"
	}
	flags := []string{"no-cldr", "no-hyphenation", "typed-layout/0.1"}
	if plan.HasEmbeddedUTF8Font() {
		flags = append(flags, "embedded-utf8-font")
	}
	return plan.BindDeterministicInputs(template, scenario, locale, "UTC", layoutengine.BuiltinTextDataVersions(),
		"typed-layout/0.1", flags, "typed-point-size")
}

func layoutDocumentPlanError(err error) error {
	if err == nil || !errors.Is(err, errTypedShadowUnsupported) {
		return err
	}
	var unsupported *typedShadowUnsupportedError
	errors.As(err, &unsupported)
	detail := err.Error()
	prefix := errTypedShadowUnsupported.Error()
	if index := strings.Index(detail, prefix); index >= 0 {
		detail = strings.TrimPrefix(detail[index+len(prefix):], ": ")
	}
	if detail == "" {
		return ErrLayoutDocumentPlanUnsupported
	}
	reason := typedShadowUnsupportedReason("")
	if unsupported != nil {
		reason = unsupported.Reason
	}
	return &layoutDocumentUnsupportedError{detail: detail, reason: reason}
}

type layoutDocumentUnsupportedError struct {
	detail string
	reason typedShadowUnsupportedReason
}

func (e *layoutDocumentUnsupportedError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%v: %s", ErrLayoutDocumentPlanUnsupported, e.detail)
}

func (e *layoutDocumentUnsupportedError) Unwrap() error { return ErrLayoutDocumentPlanUnsupported }

// WriteLayoutDocumentPlan preflights and paints an immutable typed plan. It
// does not measure or paginate again. On preflight failure the target remains
// unopened. The returned count is the number of pages committed by this call.
func (f *Document) WriteLayoutDocumentPlan(plan LayoutDocumentPlan) (int, error) {
	return f.WriteLayoutDocumentPlanContext(context.Background(), plan)
}

func (f *Document) WriteLayoutDocumentPlanContext(ctx context.Context, plan LayoutDocumentPlan) (int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if plan.hash == "" || plan.pages == 0 {
		return 0, errors.New("document: typed layout plan is empty; create it with PlanLayoutDocument")
	}
	if err := f.validateLayoutDocumentEnvelopeTarget(plan.envelope); err != nil {
		return 0, layoutDocumentPlanError(err)
	}
	projection := plan.plan.Projection()
	withDisplay := len(projection.ImageResources) != 0 || len(projection.Links) != 0 || len(projection.Destinations) != 0 || len(projection.Paths) != 0 || len(projection.Fills) != 0 || len(projection.Strokes) != 0 || len(projection.Clips) != 0 || len(projection.Transforms) != 0 || len(projection.SemanticNodes) != 0
	withDisplay = withDisplay || layoutPlanHasMultipleGlyphRunsPerLine(projection)
	for _, font := range projection.Fonts {
		withDisplay = withDisplay || font.EmbeddedUTF8 != nil
	}
	var display preparedDisplayPlanPDF
	var core preparedCorePlanPDF
	if withDisplay {
		var err error
		display, err = f.preflightDisplayLayoutPlanPDFResourcesContextForTarget(ctx, plan.plan, plan.imageSources, plan.fontSources, false)
		if err != nil {
			return 0, fmt.Errorf("document: preflight typed layout plan: %w", err)
		}
	} else {
		var err error
		core, err = f.preflightCoreLayoutPlanPDFContext(ctx, plan.plan)
		if err != nil {
			return 0, fmt.Errorf("document: preflight typed layout plan: %w", err)
		}
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	// Compliance and tagged-PDF state affect painting, so install the already
	// validated immutable envelope only after every fallible preflight succeeds.
	// The target is required to be fresh, which keeps all rejection paths atomic.
	f.installLayoutDocumentEnvelope(plan.envelope)
	pageStart := f.PageCount()
	if withDisplay {
		if err := f.paintPreparedDisplayLayoutPlanPDF(display); err != nil {
			return 0, fmt.Errorf("document: paint typed layout plan: %w", err)
		}
	} else if err := f.paintPreparedCoreLayoutPlanPDF(core); err != nil {
		return 0, fmt.Errorf("document: paint typed layout plan: %w", err)
	}
	return f.PageCount() - pageStart, nil
}

func (f *Document) validateLayoutDocumentPlanEnvelope(doc *layout.LayoutDocument) error {
	if f == nil || f.err != nil || f.page != 0 || f.state != documentStateUnopened ||
		f.clipNest != 0 || f.transformNest != 0 {
		return newTypedShadowUnsupported(typedShadowDocumentState, "requires a fresh error-free document")
	}
	if doc == nil {
		return newTypedShadowUnsupported(typedShadowDocumentEnvelope, "layout document is nil")
	}
	if !f.autoPageBreak || f.acceptPageBreakSet || f.headerFnc != nil ||
		f.footerFnc != nil || f.footerFncLpi != nil || f.pageAddGuard != nil || len(f.aliasMap) != 0 || f.aliasNbPagesStr != "" {
		return newTypedShadowUnsupported(typedShadowDocumentPolicy, "custom page lifecycle or deferred aliases are present")
	}
	if f.protect.encrypted {
		return newTypedShadowUnsupported(typedShadowDocumentEnvelope, "live encryption state cannot be retained in an immutable layout plan")
	}
	if f.tagged.nextText != (taggedContentOptions{}) || len(f.tagged.stack) != 0 ||
		f.tagged.pendingLinkElem != nil || f.tagged.pathArtifactOpen || f.tagged.artifactDepth != 0 {
		return newTypedShadowUnsupported(typedShadowDocumentEnvelope, "live tagged-content state cannot be retained in an immutable layout plan")
	}
	if doc.Signature != nil && strings.TrimSpace(f.signatureFieldName) != "" &&
		strings.TrimSpace(f.signatureFieldName) != doc.Signature.PAdESFieldName() {
		return newTypedShadowUnsupported(typedShadowDocumentEnvelope, "conflicting signing field identities cannot be retained safely")
	}
	return nil
}

func (f *Document) snapshotLayoutDocumentEnvelope(doc *layout.LayoutDocument) (typedLayoutDocumentEnvelope, error) {
	attachments := snapshotLayoutAttachments(doc.Attachments)
	if len(doc.Attachments) == 0 {
		attachments = snapshotDocumentAttachments(f.attachments)
	}
	envelope := typedLayoutDocumentEnvelope{
		producer: f.producer, title: f.title, subject: f.subject, author: f.author,
		keywords: f.keywords, creator: f.creator, creationDate: f.creationDate, modDate: f.modDate,
		xmp: append([]byte(nil), f.xmp...), compliance: f.compliance,
		outputIntent: outputIntent{
			iccProfile: append([]byte(nil), f.outputIntent.iccProfile...),
			identifier: f.outputIntent.identifier, info: f.outputIntent.info,
		},
		outputPolicy: f.outputPolicy, pdfVersion: f.pdfVersion, catalogSort: f.catalogSort,
		tagged: f.tagged.enabled, signatureField: strings.TrimSpace(f.signatureFieldName),
		attachments: attachments,
	}
	if doc.Title != "" {
		envelope.title = utf8toutf16(doc.Title)
	}
	if doc.Metadata.Subject != "" {
		envelope.subject = utf8toutf16(doc.Metadata.Subject)
	}
	if doc.Metadata.Author != "" {
		envelope.author = utf8toutf16(doc.Metadata.Author)
	}
	if !doc.Metadata.CreatedAt.IsZero() {
		envelope.creationDate = doc.Metadata.CreatedAt
	}
	if !doc.Metadata.UpdatedAt.IsZero() {
		envelope.modDate = doc.Metadata.UpdatedAt
	}
	if language := strings.TrimSpace(doc.Language); language != "" {
		envelope.compliance.Lang = language
	}
	if envelope.compliance.Title == "" && envelope.compliance.enabled() && doc.Title != "" {
		envelope.compliance.Title = doc.Title
	}
	if envelope.compliance.Identifier == "" && envelope.compliance.enabled() && doc.Metadata.DocumentID != "" {
		envelope.compliance.Identifier = doc.Metadata.DocumentID
	}
	if envelope.compliance.PDFUA2 {
		envelope.tagged = true
	}
	if doc.Signature != nil {
		envelope.signatureField = doc.Signature.PAdESFieldName()
	}
	if err := validateTypedLayoutDocumentEnvelope(envelope); err != nil {
		return typedLayoutDocumentEnvelope{}, err
	}
	return envelope, nil
}

func validateTypedLayoutDocumentEnvelope(envelope typedLayoutDocumentEnvelope) error {
	switch envelope.compliance.PDFA {
	case PDFAModeNone, PDFAMode4, PDFAMode4F, PDFAMode4E:
	default:
		return newTypedShadowUnsupported(typedShadowDocumentEnvelope, "unsupported PDF/A compliance profile")
	}
	if envelope.compliance.PDFUA2 && strings.TrimSpace(envelope.compliance.Lang) == "" {
		return newTypedShadowUnsupported(typedShadowDocumentEnvelope, "PDF/UA-2 compliance requires a document language")
	}
	if len(envelope.outputIntent.iccProfile) == 0 &&
		(envelope.outputIntent.identifier != "" || envelope.outputIntent.info != "") {
		return newTypedShadowUnsupported(typedShadowDocumentEnvelope, "output intent metadata has no ICC profile")
	}
	for index, attachment := range envelope.attachments {
		if attachment.FilePath != "" || attachment.Loader != nil {
			return newTypedShadowUnsupported(typedShadowDocumentEnvelope, fmt.Sprintf("attachments[%d] contains live external state", index))
		}
	}
	return nil
}

func (f *Document) validateLayoutDocumentEnvelopeTarget(envelope typedLayoutDocumentEnvelope) error {
	if f == nil || f.err != nil || f.page != 0 || f.state != documentStateUnopened ||
		f.clipNest != 0 || f.transformNest != 0 {
		return newTypedShadowUnsupported(typedShadowDocumentState, "document-envelope replay requires a fresh error-free target")
	}
	if f.protect.encrypted {
		return newTypedShadowUnsupported(typedShadowDocumentEnvelope, "live encryption state on the target cannot be combined with an immutable plan")
	}
	if current := strings.TrimSpace(f.signatureFieldName); current != "" &&
		envelope.signatureField != "" && current != envelope.signatureField {
		return newTypedShadowUnsupported(typedShadowDocumentEnvelope, "target has a conflicting signing field identity")
	}
	if err := validateTypedLayoutDocumentEnvelope(envelope); err != nil {
		return err
	}
	return nil
}

func (f *Document) installLayoutDocumentEnvelope(envelope typedLayoutDocumentEnvelope) {
	f.producer = envelope.producer
	f.title = envelope.title
	f.subject = envelope.subject
	f.author = envelope.author
	f.keywords = envelope.keywords
	f.creator = envelope.creator
	f.creationDate = envelope.creationDate
	f.modDate = envelope.modDate
	f.xmp = append([]byte(nil), envelope.xmp...)
	f.nXmp = 0
	f.compliance = envelope.compliance
	f.outputIntent = outputIntent{
		iccProfile: append([]byte(nil), envelope.outputIntent.iccProfile...),
		identifier: envelope.outputIntent.identifier, info: envelope.outputIntent.info,
	}
	f.nOutputIntentICC = 0
	f.outputPolicy = envelope.outputPolicy
	f.pdfVersion = envelope.pdfVersion
	f.catalogSort = envelope.catalogSort
	f.tagged = taggedPDFState{enabled: envelope.tagged}
	f.signatureFieldName = envelope.signatureField
	f.SetAttachments(envelope.attachments)
}

func hashTypedLayoutDocumentEnvelope(layoutHash string, envelope typedLayoutDocumentEnvelope) (string, error) {
	attachments := make([]typedAttachment, len(envelope.attachments))
	for index, attachment := range envelope.attachments {
		attachments[index] = typedAttachment{
			Content: append([]byte(nil), attachment.Content...), Filename: attachment.Filename,
			Description: attachment.Description, MIMEType: attachment.MIMEType,
			AFRelationship: attachment.AFRelationship,
		}
	}
	record := typedLayoutEnvelopeHash{
		Schema: "paperrune/typed-document-envelope/v1", LayoutHash: layoutHash,
		Producer: envelope.producer, Title: envelope.title, Subject: envelope.subject,
		Author: envelope.author, Keywords: envelope.keywords, Creator: envelope.creator,
		XMP: append([]byte(nil), envelope.xmp...), Compliance: envelope.compliance,
		OutputIntent: typedOutputIntent{
			ICCProfile: append([]byte(nil), envelope.outputIntent.iccProfile...),
			Identifier: envelope.outputIntent.identifier, Info: envelope.outputIntent.info,
		},
		OutputPolicy: envelope.outputPolicy, PDFVersion: envelope.pdfVersion, Tagged: envelope.tagged,
		SignatureField: envelope.signatureField, Attachments: attachments,
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func snapshotLayoutAttachments(blocks []layout.AttachmentBlock) []Attachment {
	attachments := documentAttachments(blocks)
	for index := range attachments {
		attachments[index].Content = append([]byte(nil), attachments[index].Content...)
	}
	return attachments
}

func snapshotDocumentAttachments(source []Attachment) []Attachment {
	attachments := make([]Attachment, len(source))
	for index := range source {
		attachments[index] = cloneAttachment(source[index])
		attachments[index].Content = append([]byte(nil), attachments[index].Content...)
	}
	return attachments
}
