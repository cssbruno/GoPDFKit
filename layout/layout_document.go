// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package layout

import (
	"fmt"
	"strings"
	"time"
)

// DocumentKind identifies the high-level purpose of a generated document.
type DocumentKind string

const (
	// DocumentKindGeneric is the fallback kind for documents without a more
	// specific category.
	DocumentKindGeneric DocumentKind = "generic"
	// DocumentKindReport identifies report-style documents.
	DocumentKindReport DocumentKind = "report"
	// DocumentKindForm identifies forms and questionnaires.
	DocumentKindForm DocumentKind = "form"
	// DocumentKindLetter identifies letter-style correspondence.
	DocumentKindLetter DocumentKind = "letter"
	// DocumentKindTransactional identifies invoices, receipts, and similar
	// transactional documents.
	DocumentKindTransactional DocumentKind = "transactional"
	// DocumentKindAttestation identifies certificates and attestations.
	DocumentKindAttestation DocumentKind = "attestation"
	// DocumentKindStatement identifies account or status statements.
	DocumentKindStatement DocumentKind = "statement"
	// DocumentKindLongForm identifies contract-like long-form documents.
	DocumentKindLongForm DocumentKind = "long-form"
)

// LayoutDocument is the shared model that document builders and HTML parsers can
// produce before PDF layout and drawing.
type LayoutDocument struct {
	Kind         DocumentKind      // High-level document category.
	Title        string            // Human-readable document title.
	Language     string            // Optional BCP 47 language tag.
	Metadata     DocumentMetadata  // Document metadata and summary fields.
	PageTemplate PageTemplate      // Page margins, headers, footers, and numbering.
	Body         []Block           // Main document body blocks in render order.
	Signature    *SignatureBlock   // Optional signature block.
	QR           *QRBlock          // Optional standalone QR block.
	Attachments  []AttachmentBlock // Files embedded during document output.
}

// NewLayoutDocument creates a document model with a generic kind when kind is empty.
func NewLayoutDocument(kind DocumentKind) *LayoutDocument {
	if kind == "" {
		kind = DocumentKindGeneric
	}
	return &LayoutDocument{Kind: kind}
}

// AddBlock appends a non-nil block to the document body.
func (d *LayoutDocument) AddBlock(block Block) {
	if block == nil {
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
)

// Block is implemented by every shared document block.
type Block interface {
	DocumentBlockKind() BlockKind
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
}

// BoxStyle describes common block styling independent of a renderer.
type BoxStyle struct {
	Margin          Spacing       // Space outside the block.
	Padding         Spacing       // Space inside the block.
	Border          BorderStyle   // Per-side border settings.
	BackgroundColor DocumentColor // Optional background color.
	KeepTogether    bool          // Prefer not to split this block across pages.
	KeepWithNext    bool          // Prefer to keep this block with the next one.
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
	Text  string    // Segment text.
	Style TextStyle // Style applied to this segment.
	Link  string    // Optional link target.
}

// ParagraphBlock represents a paragraph of styled text.
type ParagraphBlock struct {
	Segments []TextSegment // Paragraph text segments.
	Style    TextStyle     // Paragraph text style.
	Box      BoxStyle      // Paragraph box style.
}

// DocumentBlockKind returns BlockKindParagraph.
func (ParagraphBlock) DocumentBlockKind() BlockKind { return BlockKindParagraph }

// HeadingBlock represents a section heading.
type HeadingBlock struct {
	Level    int           // Heading level, where 1 is the highest.
	Segments []TextSegment // Heading text segments.
	Style    TextStyle     // Heading text style.
	Box      BoxStyle      // Heading box style.
}

// DocumentBlockKind returns BlockKindHeading.
func (HeadingBlock) DocumentBlockKind() BlockKind { return BlockKindHeading }

// ListBlock represents an ordered or unordered list.
type ListBlock struct {
	Ordered     bool       // Whether the list is ordered.
	MarkerStyle string     // Marker style, such as decimal or bullet.
	Items       []ListItem // List items.
	Style       TextStyle  // List text style.
	Box         BoxStyle   // List box style.
}

// DocumentBlockKind returns BlockKindList.
func (ListBlock) DocumentBlockKind() BlockKind { return BlockKindList }

// ListItem stores the blocks that belong to one list item.
type ListItem struct {
	Blocks []Block // Blocks that make up the list item.
}

// TableBlock represents a table split into header, body, and footer rows.
type TableBlock struct {
	Caption string        // Optional table caption.
	Columns []TableColumn // Column width constraints.
	Header  []TableRow    // Header rows.
	Body    []TableRow    // Body rows.
	Footer  []TableRow    // Footer rows.
	Style   TableStyle    // Table layout options.
	Box     BoxStyle      // Table box style.
}

// DocumentBlockKind returns BlockKindTable.
func (TableBlock) DocumentBlockKind() BlockKind { return BlockKindTable }

// TableColumn describes a table column width constraint.
type TableColumn struct {
	Width    float64 // Preferred column width.
	MinWidth float64 // Minimum column width.
	MaxWidth float64 // Maximum column width.
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
}

// TableCell stores table cell content and layout attributes.
type TableCell struct {
	Blocks        []Block   // Cell content blocks.
	ColSpan       int       // Number of columns spanned by the cell.
	RowSpan       int       // Number of rows spanned by the cell.
	Align         string    // Horizontal cell alignment.
	VerticalAlign string    // Vertical cell alignment.
	Style         TextStyle // Cell text style.
	Box           BoxStyle  // Cell box style.
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
	Source    string        // Image file path or registered image name.
	Data      []byte        // Inline image bytes.
	Format    string        // Image format, such as png or jpg.
	Alt       string        // Alternative text used by fallback rendering.
	Caption   []TextSegment // Optional caption text.
	Width     float64       // Requested image width.
	Height    float64       // Requested image height.
	MaxWidth  float64       // Maximum rendered width.
	MaxHeight float64       // Maximum rendered height.
	Fit       ImageFitMode  // How the image fits inside its target box.
	Align     string        // Horizontal alignment.
	DPI       float64       // Optional image DPI override.
	Box       BoxStyle      // Image box style.
}

// DocumentBlockKind returns BlockKindImage.
func (ImageBlock) DocumentBlockKind() BlockKind { return BlockKindImage }

// SignatureBlock groups one or more signature rows.
type SignatureBlock struct {
	Rows                 []SignatureRowBlock // Signature rows.
	KeepTogether         bool                // Whether rows should stay together.
	PlaceholderReference string              // Preferred PAdES signature field name.
}

// PAdESFieldName returns the signature field name to use with PAdES signing.
func (s SignatureBlock) PAdESFieldName() string {
	name := strings.TrimSpace(s.PlaceholderReference)
	if name == "" {
		return "Signature1"
	}
	return name
}

// SignatureRowBlock represents one row of signature columns.
type SignatureRowBlock struct {
	Columns      []SignatureColumn // Signature columns.
	KeepTogether bool              // Whether the row should stay on one page.
	Box          BoxStyle          // Row box style.
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
	Fields  []MetadataField // Metadata fields to render.
	Columns int             // Number of grid columns.
	Style   TextStyle       // Grid text style.
	Box     BoxStyle        // Grid box style.
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
	QR    QRBlock       // QR code configuration.
	Text  []TextSegment // Verification text.
	Style TextStyle     // Verification text style.
	Box   BoxStyle      // Verification box style.
}

// DocumentBlockKind returns BlockKindQRVerification.
func (QRVerificationBlock) DocumentBlockKind() BlockKind { return BlockKindQRVerification }

// NoteBoxBlock represents a callout, warning, or highlighted note.
type NoteBoxBlock struct {
	Title string    // Note title.
	Body  []Block   // Note body blocks.
	Style TextStyle // Note text style.
	Box   BoxStyle  // Note box style.
}

// DocumentBlockKind returns BlockKindNoteBox.
func (NoteBoxBlock) DocumentBlockKind() BlockKind { return BlockKindNoteBox }

// SectionBlock groups related blocks under an optional title.
type SectionBlock struct {
	Title             string   // Optional section title.
	Blocks            []Block  // Section body blocks.
	KeepTitleWithBody bool     // Prefer to keep title and first body block together.
	Box               BoxStyle // Section box style.
}

// DocumentBlockKind returns BlockKindSection.
func (SectionBlock) DocumentBlockKind() BlockKind { return BlockKindSection }

// ClauseBlock represents a numbered or named clause in long-form documents.
type ClauseBlock struct {
	Number       string   // Clause number or label.
	Title        string   // Clause title.
	Blocks       []Block  // Clause body blocks.
	BreakBefore  bool     // Insert a page break before the clause.
	BreakAfter   bool     // Insert a page break after the clause.
	KeepTogether bool     // Prefer to keep the clause on one page.
	Box          BoxStyle // Clause box style.
}

// DocumentBlockKind returns BlockKindClause.
func (ClauseBlock) DocumentBlockKind() BlockKind { return BlockKindClause }

// PageBreakBlock represents an explicit page break.
type PageBreakBlock struct {
	Before bool // Insert a page break before following content.
	After  bool // Insert a page break after preceding content.
}

// DocumentBlockKind returns BlockKindPageBreak.
func (PageBreakBlock) DocumentBlockKind() BlockKind { return BlockKindPageBreak }

// HeaderBlock stores reusable header content.
type HeaderBlock struct {
	Blocks []Block  // Header content blocks.
	Height float64  // Reserved header height.
	Box    BoxStyle // Header box style.
}

// FooterBlock stores reusable footer content.
type FooterBlock struct {
	Blocks          []Block  // Footer content blocks.
	Height          float64  // Reserved footer height.
	ReservePageArea bool     // Whether body layout reserves footer height.
	Box             BoxStyle // Footer box style.
}

// AttachmentBlock describes a PDF attachment that can be added during output.
type AttachmentBlock struct {
	Name        string // Attachment filename.
	MIMEType    string // Attachment MIME type.
	Description string // Attachment description.
	Data        []byte // Attachment bytes.
}
