// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layout

import (
	"fmt"
	"reflect"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// LayoutDocument is the shared model that document assembly helpers and HTML
// parsers can produce before PDF layout and drawing.
type LayoutDocument struct {
	Title        string            // Human-readable document title.
	Language     string            // Optional BCP 47 language tag.
	Metadata     DocumentMetadata  // Document metadata and summary fields.
	PageTemplate PageTemplate      // Page margins, headers, footers, and numbering.
	Body         []Block           // Main document body blocks in render order.
	Signature    *SignatureBlock   // Optional signature block.
	QR           *QRBlock          // Optional standalone QR block.
	Attachments  []AttachmentBlock // Files embedded during document output.
}

// NewLayoutDocument creates an empty renderer-independent document model.
func NewLayoutDocument() *LayoutDocument {
	return &LayoutDocument{}
}

// NewDocumentModel creates a renderer-independent document model with an
// optional title heading followed by the supplied body blocks.
func NewDocumentModel(title string, blocks ...Block) *LayoutDocument {
	doc := NewLayoutDocument()
	doc.Title = title
	if title != "" {
		doc.Body = append(doc.Body, HeadingBlock{Level: 1, Segments: []TextSegment{{Text: title}}})
	}
	doc.Body = append(doc.Body, blocks...)
	return doc
}

// AddBlock appends a non-nil block to the document body. Built-in block
// pointers are stored in their canonical value form; typed nils are ignored.
func (d *LayoutDocument) AddBlock(block Block) {
	block, ok := NormalizeBlock(block)
	if !ok {
		return
	}
	d.Body = append(d.Body, block)
}

// DocumentMetadata holds common metadata used by headers, footers,
// verification blocks, PDF metadata, and structured document summaries.
type DocumentMetadata struct {
	Subject         string          // Short document subject.
	Author          string          // Document author name.
	Organization    string          // Issuing or owning organization.
	DocumentID      string          // Internal document identifier.
	ExternalID      string          // External system identifier.
	VerificationURL string          // URL used to verify the document.
	CreatedAt       time.Time       // Document creation timestamp.
	UpdatedAt       time.Time       // Last document update timestamp.
	Parties         []DocumentParty // People, companies, or roles in the document.
	Fields          []MetadataField // Additional label/value metadata.
}

// DocumentParty describes a named person, company, or role shown in metadata.
type DocumentParty struct {
	Role       string          // Party role, such as issuer or recipient.
	Name       string          // Party display name.
	Identifier string          // Tax, account, or other party identifier.
	Email      string          // Party email address.
	Phone      string          // Party phone number.
	Address    string          // Party postal address.
	Fields     []MetadataField // Additional party metadata.
}

// MetadataField is a label/value pair used by metadata grids and summaries.
type MetadataField struct {
	Label string // Field label.
	Value string // Field value.
}

// BlockKind identifies a shared layout block.
type BlockKind string

const (
	// BlockKindParagraph identifies paragraph blocks.
	BlockKindParagraph BlockKind = "paragraph"
	// BlockKindHeading identifies heading blocks.
	BlockKindHeading BlockKind = "heading"
	// BlockKindList identifies list blocks.
	BlockKindList BlockKind = "list"
	// BlockKindTable identifies table blocks.
	BlockKindTable BlockKind = "table"
	// BlockKindImage identifies image blocks.
	BlockKindImage BlockKind = "image"
	// BlockKindSignatureRow identifies signature-row blocks.
	BlockKindSignatureRow BlockKind = "signature-row"
	// BlockKindMetadataGrid identifies metadata-grid blocks.
	BlockKindMetadataGrid BlockKind = "metadata-grid"
	// BlockKindQRVerification identifies QR-verification blocks.
	BlockKindQRVerification BlockKind = "qr-verification"
	// BlockKindNoteBox identifies note-box blocks.
	BlockKindNoteBox BlockKind = "note-box"
	// BlockKindSection identifies section blocks.
	BlockKindSection BlockKind = "section"
	// BlockKindClause identifies clause blocks.
	BlockKindClause BlockKind = "clause"
	// BlockKindPageBreak identifies explicit page-break blocks.
	BlockKindPageBreak BlockKind = "page-break"
	// BlockKindRowColumn identifies a fixed-point row or column container.
	BlockKindRowColumn BlockKind = "row-column"
	// BlockKindCanvas identifies a local explicit anchor/constraint container.
	BlockKindCanvas BlockKind = "canvas"
)

// Block identifies a supported shared document block. Rendering and
// measurement accept the concrete block values declared by this package and
// non-nil pointers to them. Implementations from other packages are reported
// as unsupported by the built-in renderer.
type Block interface {
	DocumentBlockKind() BlockKind
}

// NormalizeBlock returns the canonical value form of a block. The built-in
// block pointers are accepted for caller convenience and copied to values;
// slice, byte, and style-reference fields retain their documented ownership
// semantics. Nil interfaces and typed nils are treated alike and return
// ok=false. Implementations from other packages pass through unchanged and
// remain subject to renderer support.
func NormalizeBlock(block Block) (_ Block, ok bool) {
	if block == nil {
		return nil, false
	}
	value := reflect.ValueOf(block)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if value.IsNil() {
			return nil, false
		}
	}

	switch block := block.(type) {
	case *ParagraphBlock:
		return *block, true
	case *HeadingBlock:
		return *block, true
	case *ListBlock:
		return *block, true
	case *TableBlock:
		return *block, true
	case *ImageBlock:
		return *block, true
	case *SignatureRowBlock:
		return *block, true
	case *MetadataGridBlock:
		return *block, true
	case *QRVerificationBlock:
		return *block, true
	case *NoteBoxBlock:
		return *block, true
	case *SectionBlock:
		return *block, true
	case *ClauseBlock:
		return *block, true
	case *PageBreakBlock:
		return *block, true
	case *RowColumnBlock:
		return *block, true
	case *CanvasBlock:
		return *block, true
	default:
		return block, true
	}
}

// NormalizeBlocks removes nil blocks and converts built-in block pointers to
// value snapshots while preserving order.
func NormalizeBlocks(blocks []Block) []Block {
	normalized := make([]Block, 0, len(blocks))
	for _, block := range blocks {
		if block, ok := NormalizeBlock(block); ok {
			normalized = append(normalized, block)
		}
	}
	return normalized
}

// DocumentColor stores an optional RGB color.
type DocumentColor struct {
	R   int  // Red component, 0-255.
	G   int  // Green component, 0-255.
	B   int  // Blue component, 0-255.
	Set bool // Whether this color should be applied.
}

// TextStyle describes common text styling independent of a renderer.
type TextStyle struct {
	FontFamily    string        // Font family name.
	FontSize      float64       // Font size in points.
	Bold          bool          // Whether text is bold.
	Italic        bool          // Whether text is italic.
	Underline     bool          // Whether text is underlined.
	StrikeThrough bool          // Whether text has a strike-through line.
	Color         DocumentColor // Optional text color.
	Align         string        // Horizontal alignment, such as L, C, R, or J.
	LineHeight    float64       // Line height in document units.
	WhiteSpace    string        // Optional resolved whitespace mode: normal, nowrap, pre, pre-wrap, pre-line, or break-spaces.
	TabSize       uint8         // Number of spaces per tab stop; zero uses 8.
}

// BoxStyle describes common block styling independent of a renderer.
type BoxStyle struct {
	Margin          Spacing        // Space outside the block.
	Padding         Spacing        // Space inside the block.
	Border          BorderStyle    // Per-side border settings.
	BackgroundColor DocumentColor  // Optional background color.
	Width           float64        // Optional resolved border-box width; zero uses the containing width.
	Height          float64        // Optional resolved border-box height; zero uses intrinsic content height.
	MinWidth        float64        // Optional resolved minimum border-box width.
	MinHeight       float64        // Optional resolved minimum border-box height.
	MaxWidth        float64        // Optional resolved maximum border-box width.
	MaxHeight       float64        // Optional resolved maximum border-box height.
	Overflow        string         // Overflow policy: empty/visible or hidden.
	BorderRadius    float64        // Optional uniform circular corner radius in document units.
	Shadow          BoxShadowStyle // Optional bounded outer shadow; blur and inset shadows are not represented.
	KeepTogether    bool           // Prefer not to split this block across pages.
	KeepWithNext    bool           // Prefer to keep this block with the next one.
	Orphans         uint32         // Minimum lines preferred at the bottom of a page; zero uses 1.
	Widows          uint32         // Minimum lines preferred at the top of a continuation page; zero uses 1.
}

// BoxShadowStyle is the renderer-independent subset used by the unified
// planner: one outer, solid-color shadow with fixed offsets and spread. Its
// zero value paints no shadow. Keeping blur and inset out of this contract
// prevents painters from inventing renderer-specific raster effects.
type BoxShadowStyle struct {
	OffsetX float64
	OffsetY float64
	Spread  float64
	Color   DocumentColor
}

// Spacing stores top, right, bottom, and left measurements in document units.
type Spacing struct {
	Top    float64 // Top spacing.
	Right  float64 // Right spacing.
	Bottom float64 // Bottom spacing.
	Left   float64 // Left spacing.
}

// BorderStyle stores a simple per-side border model.
type BorderStyle struct {
	Top    BorderSide // Top border.
	Right  BorderSide // Right border.
	Bottom BorderSide // Bottom border.
	Left   BorderSide // Left border.
}

// BorderSide describes one border edge.
type BorderSide struct {
	Width float64       // Border width in document units.
	Style string        // Border style name.
	Color DocumentColor // Optional border color.
}

// TextSegment is a styled text run.
type TextSegment struct {
	Text        string     // Segment text.
	Style       TextStyle  // Style applied to this segment.
	StyleRef    *TextStyle // Optional shared segment style.
	Link        string     // Optional external URI or #name internal link target.
	Destination string     // Optional named internal destination at this segment's first glyph.
}

// CanvasConstraint is one readable same-axis equality inside a local canvas.
// Target is "canvas" or an authored sibling ID. Offset uses document units.
type CanvasConstraint struct {
	Anchor       string
	Target       string
	TargetAnchor string
	Offset       float64
}

// CanvasItem is an explicitly measured visual node positioned by local
// anchors. Box currently supplies its deterministic fill and border styling.
type CanvasItem struct {
	ID          string
	Width       float64
	Height      float64
	Constraints []CanvasConstraint
	Box         BoxStyle
	Alt         string
}

// CanvasBlock is a bounded local anchor DAG. It is intentionally not a
// document-wide constraint system.
type CanvasBlock struct {
	Width             float64
	Height            float64
	DefaultHorizontal string
	DefaultVertical   string
	Items             []CanvasItem
}

func (CanvasBlock) DocumentBlockKind() BlockKind { return BlockKindCanvas }

// ParagraphBlock represents a paragraph of styled text.
type ParagraphBlock struct {
	Segments []TextSegment // Paragraph text segments.
	Style    TextStyle     // Paragraph text style.
	StyleRef *TextStyle    // Optional shared paragraph text style.
	Box      BoxStyle      // Paragraph box style.
	BoxRef   *BoxStyle     // Optional shared paragraph box style.
}

// DocumentBlockKind returns BlockKindParagraph.
func (ParagraphBlock) DocumentBlockKind() BlockKind { return BlockKindParagraph }

// HeadingBlock represents a section heading.
type HeadingBlock struct {
	Level    int           // Heading level, where 1 is the highest.
	Segments []TextSegment // Heading text segments.
	Style    TextStyle     // Heading text style.
	StyleRef *TextStyle    // Optional shared heading text style.
	Box      BoxStyle      // Heading box style.
	BoxRef   *BoxStyle     // Optional shared heading box style.
}

// DocumentBlockKind returns BlockKindHeading.
func (HeadingBlock) DocumentBlockKind() BlockKind { return BlockKindHeading }

// RowColumnDirection selects the main axis of a readable row/column block.
type RowColumnDirection string

const (
	RowDirection    RowColumnDirection = "row"
	ColumnDirection RowColumnDirection = "column"
)

// RowColumnTrackKind selects exact, intrinsic, or weighted main-axis sizing.
type RowColumnTrackKind string

const (
	RowColumnTrackFixed    RowColumnTrackKind = "fixed"
	RowColumnTrackAuto     RowColumnTrackKind = "auto"
	RowColumnTrackFraction RowColumnTrackKind = "fraction"
	RowColumnTrackFlex     RowColumnTrackKind = "flex"
)

type RowColumnFlexBasisKind string

const (
	RowColumnFlexBasisFixed   RowColumnFlexBasisKind = "fixed"
	RowColumnFlexBasisPercent RowColumnFlexBasisKind = "percent"
	RowColumnFlexBasisContent RowColumnFlexBasisKind = "content"
)

// RowColumnTrack is a renderer-independent point-based child constraint.
// Flex tracks use Basis/BasisPercent with integral Grow/Shrink factors and
// optional fixed Min/Max constraints; legacy fixed/auto/fraction fields retain
// their existing contracts.
type RowColumnTrack struct {
	Kind         RowColumnTrackKind
	Size         float64
	Min          float64
	Max          float64
	Weight       uint32
	BasisKind    RowColumnFlexBasisKind
	Basis        float64
	BasisPercent uint32
	Grow         uint32
	Shrink       uint32
	// GrowFactor and ShrinkFactor are millionth-scale CSS factors. A zero
	// value falls back to the corresponding integral Grow/Shrink field so the
	// original typed API remains source compatible.
	GrowFactor   uint64
	ShrinkFactor uint64
	MinPercent   uint32
	MaxPercent   uint32
}

// RowColumnItem owns one supported text block and its placement constraints.
type RowColumnItem struct {
	Block           Block
	Track           RowColumnTrack
	CrossSize       float64
	CrossAlign      string
	CrossMin        float64
	CrossMax        float64
	CrossMinPercent uint32
	CrossMaxPercent uint32
}

// RowColumnBlock is the readable source model for the fixed-point primitive.
// Initial document planning accepts paragraph and heading item blocks.
type RowColumnBlock struct {
	Direction    RowColumnDirection
	Gap          float64
	CrossGap     float64
	CrossSize    float64
	Wrap         string
	MainAlign    string
	CrossAlign   string
	AlignContent string
	ReverseMain  bool
	Items        []RowColumnItem
}

// DocumentBlockKind returns BlockKindRowColumn.
func (RowColumnBlock) DocumentBlockKind() BlockKind { return BlockKindRowColumn }

// ListBlock represents an ordered or unordered list.
type ListBlock struct {
	Ordered     bool       // Whether the list is ordered.
	MarkerStyle string     // Marker style, such as decimal or bullet.
	Start       int        // First ordered marker value; zero uses 1.
	Items       []ListItem // List items.
	Style       TextStyle  // List text style.
	StyleRef    *TextStyle // Optional shared list text style.
	Box         BoxStyle   // List box style.
	BoxRef      *BoxStyle  // Optional shared list box style.
}

// DocumentBlockKind returns BlockKindList.
func (ListBlock) DocumentBlockKind() BlockKind { return BlockKindList }

// ListItem stores the blocks that belong to one list item.
type ListItem struct {
	Blocks   []Block // Blocks that make up the list item.
	Value    int     // Explicit ordered marker value.
	ValueSet bool    // Whether Value overrides the current ordered counter.
}

// TableBlock represents a table split into header, body, and footer rows.
type TableBlock struct {
	Caption         string        // Optional plain table caption.
	CaptionSegments []TextSegment // Optional structured caption; mutually exclusive with Caption.
	Columns         []TableColumn // Column width constraints.
	Header          []TableRow    // Header rows.
	Body            []TableRow    // Body rows.
	Footer          []TableRow    // Footer rows.
	Style           TableStyle    // Table layout options.
	Box             BoxStyle      // Table box style.
	BoxRef          *BoxStyle     // Optional shared table box style.
}

// DocumentBlockKind returns BlockKindTable.
func (TableBlock) DocumentBlockKind() BlockKind { return BlockKindTable }

// TableColumn describes a table column width constraint.
type TableColumn struct {
	Width           float64 // Preferred fixed column width in document units.
	MinWidth        float64 // Minimum fixed column width in document units.
	MaxWidth        float64 // Maximum fixed column width in document units.
	WidthPercent    uint32  // Preferred container-relative width; 100% is 100_000_000.
	MinWidthPercent uint32  // Minimum container-relative width; 100% is 100_000_000.
	MaxWidthPercent uint32  // Maximum container-relative width; 100% is 100_000_000.
}

// TableStyle stores renderer-independent table layout options.
type TableStyle struct {
	BorderCollapse bool // Whether adjacent cell borders collapse.
	RepeatHeader   bool // Whether header rows repeat after page breaks.
	KeepRows       bool // Whether rows should stay together when possible.
}

// TableRow stores table cells and row-level pagination hints.
type TableRow struct {
	Cells        []TableCell // Cells in this row.
	KeepTogether bool        // Whether the row should stay on one page.
	KeepWithNext bool        // Whether this row should stay with the following row when possible.
	Orphans      uint32      // Minimum authored rows retained at the start table-page boundary.
	Widows       uint32      // Minimum authored rows retained at the final table-page boundary.
}

// TableCell stores table cell content and layout attributes.
type TableCell struct {
	Blocks        []Block    // Cell content blocks.
	Header        bool       // Whether this cell carries header semantics.
	Scope         string     // Optional header scope: row, column, or both.
	ColSpan       int        // Number of columns spanned by the cell.
	RowSpan       int        // Number of rows spanned by the cell.
	Align         string     // Horizontal cell alignment.
	VerticalAlign string     // Vertical cell alignment.
	Style         TextStyle  // Cell text style.
	StyleRef      *TextStyle // Optional shared cell text style.
	Box           BoxStyle   // Cell box style.
	BoxRef        *BoxStyle  // Optional shared cell box style.
}

// ImageFitMode identifies how an image is fitted into its target box.
type ImageFitMode string

const (
	// ImageFitAuto preserves the renderer's default image fitting behavior.
	ImageFitAuto ImageFitMode = ""
	// ImageFitContain scales the image to fit entirely inside the target box.
	ImageFitContain ImageFitMode = "contain"
	// ImageFitCover scales the image to cover the target box, cropping if needed.
	ImageFitCover ImageFitMode = "cover"
)

// ImageBlock represents an image and optional caption.
type ImageBlock struct {
	Source       string        // Image file path or registered image name.
	Data         []byte        // Inline image bytes.
	DataRef      *[]byte       // Optional shared inline image bytes.
	Format       string        // Image format, such as png or jpg.
	Alt          string        // Alternative text used by fallback rendering.
	Caption      []TextSegment // Optional caption text.
	CaptionStyle TextStyle     // Optional caption style; unset values use the canonical caption defaults.
	Width        float64       // Requested fixed image width.
	Height       float64       // Requested image height.
	MaxWidth     float64       // Maximum fixed rendered width.
	MaxHeight    float64       // Maximum rendered height.
	WidthPercent uint32        // Container-relative width; 100% is 100_000_000.
	// MaxWidthPercent is resolved against the image's containing content box.
	// Percentage height is intentionally absent because ordinary flow has no
	// definite containing height; Height=0 retains intrinsic/automatic sizing.
	MaxWidthPercent uint32
	Fit             ImageFitMode // How the image fits inside its target box.
	FocusX          float64      // Horizontal crop focus from 0 (left) through 1 (right).
	FocusY          float64      // Vertical crop focus from 0 (top) through 1 (bottom).
	FocusSet        bool         // Whether FocusX and FocusY are explicitly authored.
	Align           string       // Horizontal alignment.
	DPI             float64      // Optional image DPI override.
	Decorative      bool         // Whether the image is an accessibility artifact rather than a figure.
	Box             BoxStyle     // Image box style.
	BoxRef          *BoxStyle    // Optional shared image box style.
}

// DocumentBlockKind returns BlockKindImage.
func (ImageBlock) DocumentBlockKind() BlockKind { return BlockKindImage }

func effectiveTextStyle(style TextStyle, ref *TextStyle) TextStyle {
	if ref != nil {
		return *ref
	}
	return style
}

func effectiveBoxStyle(box BoxStyle, ref *BoxStyle) BoxStyle {
	if ref != nil {
		return *ref
	}
	return box
}

// EffectiveStyle returns the shared style reference when one is configured.
func (s TextSegment) EffectiveStyle() TextStyle { return effectiveTextStyle(s.Style, s.StyleRef) }

// EffectiveStyle returns the shared paragraph style when one is configured.
func (b ParagraphBlock) EffectiveStyle() TextStyle { return effectiveTextStyle(b.Style, b.StyleRef) }

// EffectiveBox returns the shared paragraph box when one is configured.
func (b ParagraphBlock) EffectiveBox() BoxStyle { return effectiveBoxStyle(b.Box, b.BoxRef) }

// EffectiveStyle returns the shared heading style when one is configured.
func (b HeadingBlock) EffectiveStyle() TextStyle { return effectiveTextStyle(b.Style, b.StyleRef) }

// EffectiveBox returns the shared heading box when one is configured.
func (b HeadingBlock) EffectiveBox() BoxStyle { return effectiveBoxStyle(b.Box, b.BoxRef) }

// EffectiveStyle returns the shared list style when one is configured.
func (b ListBlock) EffectiveStyle() TextStyle { return effectiveTextStyle(b.Style, b.StyleRef) }

// EffectiveBox returns the shared list box when one is configured.
func (b ListBlock) EffectiveBox() BoxStyle { return effectiveBoxStyle(b.Box, b.BoxRef) }

// EffectiveBox returns the shared table box when one is configured.
func (b TableBlock) EffectiveBox() BoxStyle { return effectiveBoxStyle(b.Box, b.BoxRef) }

// EffectiveStyle returns the shared cell style when one is configured.
func (c TableCell) EffectiveStyle() TextStyle { return effectiveTextStyle(c.Style, c.StyleRef) }

// EffectiveBox returns the shared cell box when one is configured.
func (c TableCell) EffectiveBox() BoxStyle { return effectiveBoxStyle(c.Box, c.BoxRef) }

// ImageData returns shared inline image bytes when configured.
func (b ImageBlock) ImageData() []byte {
	if b.DataRef != nil {
		return *b.DataRef
	}
	return b.Data
}

// EffectiveBox returns the shared image box when one is configured.
func (b ImageBlock) EffectiveBox() BoxStyle { return effectiveBoxStyle(b.Box, b.BoxRef) }

// EffectiveBox returns the shared signature-row box when one is configured.
func (b SignatureRowBlock) EffectiveBox() BoxStyle { return effectiveBoxStyle(b.Box, b.BoxRef) }

// EffectiveStyle returns the shared metadata-grid style when one is configured.
func (b MetadataGridBlock) EffectiveStyle() TextStyle { return effectiveTextStyle(b.Style, b.StyleRef) }

// EffectiveBox returns the shared metadata-grid box when one is configured.
func (b MetadataGridBlock) EffectiveBox() BoxStyle { return effectiveBoxStyle(b.Box, b.BoxRef) }

// SignatureBlock groups one or more signature rows.
type SignatureBlock struct {
	Rows                 []SignatureRowBlock // Signature rows.
	KeepTogether         bool                // Whether rows should stay together.
	PlaceholderReference string              // Preferred PAdES signature field name.
}

// PAdESFieldName returns the signature field name to use with PAdES signing.
func (s SignatureBlock) PAdESFieldName() string {
	name := s.PlaceholderReference
	if needsTrimSpace(name) {
		name = strings.TrimSpace(name)
	}
	if name == "" {
		return "Signature1"
	}
	return name
}

func needsTrimSpace(s string) bool {
	if s == "" {
		return false
	}
	first, _ := utf8.DecodeRuneInString(s)
	if unicode.IsSpace(first) {
		return true
	}
	last, _ := utf8.DecodeLastRuneInString(s)
	return unicode.IsSpace(last)
}

// SignatureRowBlock represents one row of signature columns.
type SignatureRowBlock struct {
	Columns      []SignatureColumn // Signature columns.
	Gap          float64           // Optional gap between columns; zero uses the compatibility default.
	KeepTogether bool              // Whether the row should stay on one page.
	Box          BoxStyle          // Row box style.
	BoxRef       *BoxStyle         // Optional shared row box style.
}

// DocumentBlockKind returns BlockKindSignatureRow.
func (SignatureRowBlock) DocumentBlockKind() BlockKind { return BlockKindSignatureRow }

// SignatureColumn describes a single signature line and its metadata.
type SignatureColumn struct {
	Label    string          // Display label for the signature line.
	Name     string          // Signer name.
	Role     string          // Signer role or title.
	Metadata []MetadataField // Additional signer metadata.
	Width    float64         // Requested column width.
}

// MetadataGridBlock represents label/value metadata in a grid.
type MetadataGridBlock struct {
	Fields   []MetadataField // Metadata fields to render.
	Columns  int             // Number of grid columns.
	Gap      float64         // Optional gap between columns; zero uses the compatibility default.
	Style    TextStyle       // Grid text style.
	StyleRef *TextStyle      // Optional shared grid text style.
	Box      BoxStyle        // Grid box style.
	BoxRef   *BoxStyle       // Optional shared grid box style.
}

// DocumentBlockKind returns BlockKindMetadataGrid.
func (MetadataGridBlock) DocumentBlockKind() BlockKind { return BlockKindMetadataGrid }

// QRBlock describes a standalone QR code.
type QRBlock struct {
	Value        string  // Encoded QR value.
	Label        string  // Optional label shown with the QR code.
	URL          string  // Optional verification URL.
	Size         float64 // Requested QR size.
	Align        string  // Horizontal alignment.
	KeepTogether bool    // Whether the QR block should stay on one page.
}

// PageTemplate describes the reusable page shell around body content.
type PageTemplate struct {
	Margins              Spacing           // Page margins.
	Header               *HeaderBlock      // Default page header.
	Footer               *FooterBlock      // Default page footer.
	FirstPageHeader      *HeaderBlock      // Header used only on page one.
	FirstPageFooter      *FooterBlock      // Footer used only on page one.
	EvenPageFooter       *FooterBlock      // Footer used on even pages.
	PageNumbers          PageNumberOptions // Automatic page-number rendering.
	ReserveFooterHeight  float64           // Body space reserved for the default footer.
	EvenPageFooterHeight float64           // Body space reserved for even-page footers.
}

// FooterReservedHeight returns the body-layout space reserved for the footer.
func (pt PageTemplate) FooterReservedHeight() float64 {
	return pt.FooterReservedHeightForPage(0)
}

// FooterReservedHeightForPage returns the body-layout footer space for a page.
func (pt PageTemplate) FooterReservedHeightForPage(page int) float64 {
	footer := pt.FooterForPage(page)
	if page > 0 && page%2 == 0 && pt.EvenPageFooterHeight > 0 {
		return pt.EvenPageFooterHeight
	}
	if pt.ReserveFooterHeight > 0 {
		return pt.ReserveFooterHeight
	}
	if footer != nil && footer.ReservePageArea {
		return footer.Height
	}
	return 0
}

// HeaderForPage returns the header block selected for a page.
func (pt PageTemplate) HeaderForPage(page int) *HeaderBlock {
	if page == 1 && pt.FirstPageHeader != nil {
		return pt.FirstPageHeader
	}
	return pt.Header
}

// FooterForPage returns the footer block selected for a page.
func (pt PageTemplate) FooterForPage(page int) *FooterBlock {
	if page == 1 && pt.FirstPageFooter != nil {
		return pt.FirstPageFooter
	}
	if page > 0 && page%2 == 0 && pt.EvenPageFooter != nil {
		return pt.EvenPageFooter
	}
	return pt.Footer
}

// PageNumberOptions controls automatic page-number text in page footers.
type PageNumberOptions struct {
	Enabled        bool   // Whether automatic page numbers are rendered.
	Format         string // fmt.Sprintf format for page numbers.
	TotalPageAlias string // Alias replaced with total page count.
}

// PageNumberText formats the footer page number label when enabled.
func (pt PageTemplate) PageNumberText(page int) string {
	if page <= 0 {
		return ""
	}
	format := strings.TrimSpace(pt.PageNumbers.Format)
	if !pt.PageNumbers.Enabled && format == "" {
		return ""
	}
	if format == "" {
		format = "Page %d"
		if alias := pt.pageTotalAlias(); alias != "" {
			format += " / " + alias
		}
	}
	return fmt.Sprintf(format, page)
}

// PageTotalAlias returns the alias replaced with the total page count.
func (pt PageTemplate) PageTotalAlias() string {
	return pt.pageTotalAlias()
}

func (pt PageTemplate) pageTotalAlias() string {
	return pt.PageNumbers.TotalPageAlias
}

// QRVerificationBlock combines a QR code with verification text.
type QRVerificationBlock struct {
	QR       QRBlock       // QR code configuration.
	Text     []TextSegment // Verification text.
	Style    TextStyle     // Verification text style.
	StyleRef *TextStyle    // Optional shared verification text style.
	Box      BoxStyle      // Verification box style.
	BoxRef   *BoxStyle     // Optional shared verification box style.
}

// DocumentBlockKind returns BlockKindQRVerification.
func (QRVerificationBlock) DocumentBlockKind() BlockKind { return BlockKindQRVerification }

// EffectiveStyle returns the shared QR-verification style when one is configured.
func (b QRVerificationBlock) EffectiveStyle() TextStyle {
	return effectiveTextStyle(b.Style, b.StyleRef)
}

// EffectiveBox returns the shared QR-verification box when one is configured.
func (b QRVerificationBlock) EffectiveBox() BoxStyle { return effectiveBoxStyle(b.Box, b.BoxRef) }

// NoteBoxBlock represents a callout, warning, or highlighted note.
type NoteBoxBlock struct {
	Title    string     // Note title.
	Body     []Block    // Note body blocks.
	Style    TextStyle  // Note text style.
	StyleRef *TextStyle // Optional shared note text style.
	Box      BoxStyle   // Note box style.
	BoxRef   *BoxStyle  // Optional shared note box style.
}

// DocumentBlockKind returns BlockKindNoteBox.
func (NoteBoxBlock) DocumentBlockKind() BlockKind { return BlockKindNoteBox }

// EffectiveStyle returns the shared note-box style when one is configured.
func (b NoteBoxBlock) EffectiveStyle() TextStyle { return effectiveTextStyle(b.Style, b.StyleRef) }

// EffectiveBox returns the shared note-box style when one is configured.
func (b NoteBoxBlock) EffectiveBox() BoxStyle { return effectiveBoxStyle(b.Box, b.BoxRef) }

// SectionBlock groups related blocks under an optional title.
type SectionBlock struct {
	Title             string    // Optional section title.
	Blocks            []Block   // Section body blocks.
	KeepTitleWithBody bool      // Prefer to keep title and first body block together.
	Box               BoxStyle  // Section box style.
	BoxRef            *BoxStyle // Optional shared section box style.
}

// DocumentBlockKind returns BlockKindSection.
func (SectionBlock) DocumentBlockKind() BlockKind { return BlockKindSection }

// EffectiveBox returns the shared section box when one is configured.
func (b SectionBlock) EffectiveBox() BoxStyle { return effectiveBoxStyle(b.Box, b.BoxRef) }

// ClauseBlock represents a numbered or named clause in long-form documents.
type ClauseBlock struct {
	Number       string    // Clause number or label.
	Title        string    // Clause title.
	Blocks       []Block   // Clause body blocks.
	BreakBefore  bool      // Insert a page break before the clause.
	BreakAfter   bool      // Insert a page break after the clause.
	KeepTogether bool      // Prefer to keep the clause on one page.
	Box          BoxStyle  // Clause box style.
	BoxRef       *BoxStyle // Optional shared clause box style.
}

// DocumentBlockKind returns BlockKindClause.
func (ClauseBlock) DocumentBlockKind() BlockKind { return BlockKindClause }

// EffectiveBox returns the shared clause box when one is configured.
func (b ClauseBlock) EffectiveBox() BoxStyle { return effectiveBoxStyle(b.Box, b.BoxRef) }

// PageBreakBlock represents an explicit page break.
type PageBreakBlock struct {
	Before bool // Insert a page break before following content.
	After  bool // Insert a page break after preceding content.
}

// DocumentBlockKind returns BlockKindPageBreak.
func (PageBreakBlock) DocumentBlockKind() BlockKind { return BlockKindPageBreak }

// HeaderBlock stores reusable header content.
type HeaderBlock struct {
	Blocks []Block   // Header content blocks.
	Height float64   // Reserved header height.
	Box    BoxStyle  // Header box style.
	BoxRef *BoxStyle // Optional shared header box style.
}

// EffectiveBox returns the shared header box when one is configured.
func (b HeaderBlock) EffectiveBox() BoxStyle { return effectiveBoxStyle(b.Box, b.BoxRef) }

// FooterBlock stores reusable footer content.
type FooterBlock struct {
	Blocks          []Block   // Footer content blocks.
	Height          float64   // Reserved footer height.
	ReservePageArea bool      // Whether body layout reserves footer height.
	Box             BoxStyle  // Footer box style.
	BoxRef          *BoxStyle // Optional shared footer box style.
}

// EffectiveBox returns the shared footer box when one is configured.
func (b FooterBlock) EffectiveBox() BoxStyle { return effectiveBoxStyle(b.Box, b.BoxRef) }

// AttachmentBlock describes a PDF attachment that can be added during output.
type AttachmentBlock struct {
	Name        string // Attachment filename.
	MIMEType    string // Attachment MIME type.
	Description string // Attachment description.
	Data        []byte // Attachment bytes.
}
