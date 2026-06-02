/****************************************************************************
 * Software: GoPDFKit                                                         *
 * License:  MIT License                                                    *
 *                                                                          *
 * Copyright (c) 2026 cssBruno                                              *
 ****************************************************************************/

package gopdfkit

import (
	"fmt"
	"strings"
	"time"
)

// DocumentKind identifies the high-level purpose of a generated document.
type DocumentKind string

const (
	DocumentKindGeneric       DocumentKind = "generic"
	DocumentKindReport        DocumentKind = "report"
	DocumentKindForm          DocumentKind = "form"
	DocumentKindLetter        DocumentKind = "letter"
	DocumentKindTransactional DocumentKind = "transactional"
	DocumentKindAttestation   DocumentKind = "attestation"
	DocumentKindStatement     DocumentKind = "statement"
	DocumentKindLongForm      DocumentKind = "long-form"
)

// Document is the shared model that document builders and HTML parsers can
// produce before PDF layout and drawing.
type Document struct {
	Kind        DocumentKind
	Title       string
	Language    string
	Metadata    DocumentMetadata
	Chrome      *PageChrome
	Header      *HeaderBlock
	Footer      *FooterBlock
	Body        []Block
	Signature   *SignatureBlock
	QR          *QRBlock
	Attachments []AttachmentBlock
}

// NewDocument creates a document model with a generic kind when kind is empty.
func NewDocument(kind DocumentKind) *Document {
	if kind == "" {
		kind = DocumentKindGeneric
	}
	return &Document{Kind: kind}
}

// AddBlock appends a non-nil block to the document body.
func (d *Document) AddBlock(block Block) {
	if block == nil {
		return
	}
	d.Body = append(d.Body, block)
}

// PageChrome returns normalized page-level header and footer configuration.
func (d *Document) PageChrome() PageChrome {
	if d == nil {
		return PageChrome{}
	}
	chrome := PageChrome{}
	if d.Chrome != nil {
		chrome = *d.Chrome
	}
	if chrome.Header == nil {
		chrome.Header = d.Header
	}
	if chrome.Footer == nil {
		chrome.Footer = d.Footer
	}
	return chrome
}

// DocumentMetadata holds common metadata used by headers, footers,
// verification blocks, PDF metadata, and structured document summaries.
type DocumentMetadata struct {
	Subject         string
	Author          string
	Organization    string
	DocumentID      string
	ExternalID      string
	VerificationURL string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	Parties         []DocumentParty
	Fields          []MetadataField
}

// DocumentParty describes a named person, company, or role shown in metadata.
type DocumentParty struct {
	Role       string
	Name       string
	Identifier string
	Email      string
	Phone      string
	Address    string
	Fields     []MetadataField
}

// MetadataField is a label/value pair used by metadata grids and summaries.
type MetadataField struct {
	Label string
	Value string
}

// BlockKind identifies a shared layout block.
type BlockKind string

const (
	BlockKindParagraph      BlockKind = "paragraph"
	BlockKindHeading        BlockKind = "heading"
	BlockKindList           BlockKind = "list"
	BlockKindTable          BlockKind = "table"
	BlockKindImage          BlockKind = "image"
	BlockKindSignatureRow   BlockKind = "signature-row"
	BlockKindMetadataGrid   BlockKind = "metadata-grid"
	BlockKindQRVerification BlockKind = "qr-verification"
	BlockKindNoteBox        BlockKind = "note-box"
	BlockKindSection        BlockKind = "section"
	BlockKindClause         BlockKind = "clause"
	BlockKindPageBreak      BlockKind = "page-break"
)

// Block is implemented by every shared document block.
type Block interface {
	DocumentBlockKind() BlockKind
}

// DocumentColor stores an optional RGB color.
type DocumentColor struct {
	R   int
	G   int
	B   int
	Set bool
}

// TextStyle describes common text styling independent of a renderer.
type TextStyle struct {
	FontFamily    string
	FontSize      float64
	Bold          bool
	Italic        bool
	Underline     bool
	StrikeThrough bool
	Color         DocumentColor
	Align         string
	LineHeight    float64
}

// BoxStyle describes common block styling independent of a renderer.
type BoxStyle struct {
	Margin          Spacing
	Padding         Spacing
	Border          BorderStyle
	BackgroundColor DocumentColor
	KeepTogether    bool
	KeepWithNext    bool
}

// Spacing stores top, right, bottom, and left measurements in document units.
type Spacing struct {
	Top    float64
	Right  float64
	Bottom float64
	Left   float64
}

// BorderStyle stores a simple per-side border model.
type BorderStyle struct {
	Top    BorderSide
	Right  BorderSide
	Bottom BorderSide
	Left   BorderSide
}

// BorderSide describes one border edge.
type BorderSide struct {
	Width float64
	Style string
	Color DocumentColor
}

// TextSegment is a styled text run.
type TextSegment struct {
	Text  string
	Style TextStyle
	Link  string
}

// ParagraphBlock represents a paragraph of styled text.
type ParagraphBlock struct {
	Segments []TextSegment
	Style    TextStyle
	Box      BoxStyle
}

func (ParagraphBlock) DocumentBlockKind() BlockKind { return BlockKindParagraph }

// HeadingBlock represents a section heading.
type HeadingBlock struct {
	Level    int
	Segments []TextSegment
	Style    TextStyle
	Box      BoxStyle
}

func (HeadingBlock) DocumentBlockKind() BlockKind { return BlockKindHeading }

// ListBlock represents an ordered or unordered list.
type ListBlock struct {
	Ordered     bool
	MarkerStyle string
	Items       []ListItem
	Style       TextStyle
	Box         BoxStyle
}

func (ListBlock) DocumentBlockKind() BlockKind { return BlockKindList }

// ListItem stores the blocks that belong to one list item.
type ListItem struct {
	Blocks []Block
}

// TableBlock represents a table split into header, body, and footer rows.
type TableBlock struct {
	Caption string
	Columns []TableColumn
	Header  []TableRow
	Body    []TableRow
	Footer  []TableRow
	Style   TableStyle
	Box     BoxStyle
}

func (TableBlock) DocumentBlockKind() BlockKind { return BlockKindTable }

// TableColumn describes a table column width constraint.
type TableColumn struct {
	Width    float64
	MinWidth float64
	MaxWidth float64
}

// TableStyle stores renderer-independent table layout options.
type TableStyle struct {
	BorderCollapse bool
	RepeatHeader   bool
	KeepRows       bool
}

// TableRow stores table cells and row-level pagination hints.
type TableRow struct {
	Cells        []TableCell
	KeepTogether bool
}

// TableCell stores table cell content and layout attributes.
type TableCell struct {
	Blocks        []Block
	ColSpan       int
	RowSpan       int
	Align         string
	VerticalAlign string
	Style         TextStyle
	Box           BoxStyle
}

// ImageFitMode identifies how an image is fitted into its target box.
type ImageFitMode string

const (
	ImageFitAuto    ImageFitMode = ""
	ImageFitContain ImageFitMode = "contain"
	ImageFitCover   ImageFitMode = "cover"
)

// ImageBlock represents an image and optional caption.
type ImageBlock struct {
	Source    string
	Data      []byte
	Format    string
	Alt       string
	Caption   []TextSegment
	Width     float64
	Height    float64
	MaxWidth  float64
	MaxHeight float64
	Fit       ImageFitMode
	Align     string
	DPI       float64
	Box       BoxStyle
}

func (ImageBlock) DocumentBlockKind() BlockKind { return BlockKindImage }

// SignatureBlock groups one or more signature rows.
type SignatureBlock struct {
	Rows                 []SignatureRowBlock
	KeepTogether         bool
	PlaceholderReference string
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
	Columns      []SignatureColumn
	KeepTogether bool
	Box          BoxStyle
}

func (SignatureRowBlock) DocumentBlockKind() BlockKind { return BlockKindSignatureRow }

// SignatureColumn describes a single signature line and its metadata.
type SignatureColumn struct {
	Label    string
	Name     string
	Role     string
	Metadata []MetadataField
	Width    float64
}

// MetadataGridBlock represents label/value metadata in a grid.
type MetadataGridBlock struct {
	Fields  []MetadataField
	Columns int
	Style   TextStyle
	Box     BoxStyle
}

func (MetadataGridBlock) DocumentBlockKind() BlockKind { return BlockKindMetadataGrid }

// QRBlock describes a standalone QR code.
type QRBlock struct {
	Value        string
	Label        string
	URL          string
	Size         float64
	Align        string
	KeepTogether bool
}

// PageChrome describes reusable page-level header, footer, and margin options.
type PageChrome struct {
	Header                *HeaderBlock
	Footer                *FooterBlock
	FirstPageHeader       *HeaderBlock
	FirstPageFooter       *FooterBlock
	AlternateFooter       *FooterBlock
	Margins               Spacing
	PageNumberFormat      string
	TotalPageAlias        string
	ReserveFooterHeight   float64
	AlternateFooterHeight float64
}

// FooterReservedHeight returns the body-layout space reserved for the footer.
func (pc PageChrome) FooterReservedHeight() float64 {
	return pc.FooterReservedHeightForPage(0)
}

// FooterReservedHeightForPage returns the body-layout footer space for a page.
func (pc PageChrome) FooterReservedHeightForPage(page int) float64 {
	footer := pc.FooterForPage(page)
	if page > 0 && page%2 == 0 && pc.AlternateFooterHeight > 0 {
		return pc.AlternateFooterHeight
	}
	if pc.ReserveFooterHeight > 0 {
		return pc.ReserveFooterHeight
	}
	if footer != nil && footer.ReservePageArea {
		return footer.Height
	}
	return 0
}

// FooterForPage returns the footer block selected for a page.
func (pc PageChrome) FooterForPage(page int) *FooterBlock {
	if page == 1 && pc.FirstPageFooter != nil {
		return pc.FirstPageFooter
	}
	if page > 0 && page%2 == 0 && pc.AlternateFooter != nil {
		return pc.AlternateFooter
	}
	return pc.Footer
}

// PageNumberText formats the footer page number label when enabled.
func (pc PageChrome) PageNumberText(page int) string {
	if page <= 0 {
		return ""
	}
	footer := pc.FooterForPage(page)
	if pc.PageNumberFormat == "" && (footer == nil || !footer.ShowPageNumber) {
		return ""
	}
	format := strings.TrimSpace(pc.PageNumberFormat)
	if format == "" {
		format = "Page %d"
		if alias := pc.pageTotalAlias(); alias != "" {
			format += " / " + alias
		}
	}
	return fmt.Sprintf(format, page)
}

func (pc PageChrome) pageTotalAlias() string {
	if pc.TotalPageAlias != "" {
		return pc.TotalPageAlias
	}
	if footer := pc.FooterForPage(0); footer != nil {
		return footer.TotalPageAlias
	}
	return ""
}

// QRVerificationBlock combines a QR code with verification text.
type QRVerificationBlock struct {
	QR    QRBlock
	Text  []TextSegment
	Style TextStyle
	Box   BoxStyle
}

func (QRVerificationBlock) DocumentBlockKind() BlockKind { return BlockKindQRVerification }

// NoteBoxBlock represents a callout, warning, or highlighted note.
type NoteBoxBlock struct {
	Title string
	Body  []Block
	Style TextStyle
	Box   BoxStyle
}

func (NoteBoxBlock) DocumentBlockKind() BlockKind { return BlockKindNoteBox }

// SectionBlock groups related blocks under an optional title.
type SectionBlock struct {
	Title             string
	Blocks            []Block
	KeepTitleWithBody bool
	Box               BoxStyle
}

func (SectionBlock) DocumentBlockKind() BlockKind { return BlockKindSection }

// ClauseBlock represents a numbered or named clause in long-form documents.
type ClauseBlock struct {
	Number       string
	Title        string
	Blocks       []Block
	BreakBefore  bool
	BreakAfter   bool
	KeepTogether bool
	Box          BoxStyle
}

func (ClauseBlock) DocumentBlockKind() BlockKind { return BlockKindClause }

// PageBreakBlock represents an explicit page break.
type PageBreakBlock struct {
	Before bool
	After  bool
}

func (PageBreakBlock) DocumentBlockKind() BlockKind { return BlockKindPageBreak }

// HeaderBlock stores reusable header content.
type HeaderBlock struct {
	Blocks []Block
	Height float64
	Box    BoxStyle
}

// FooterBlock stores reusable footer content.
type FooterBlock struct {
	Blocks          []Block
	Height          float64
	ShowPageNumber  bool
	TotalPageAlias  string
	ReservePageArea bool
	Box             BoxStyle
}

// AttachmentBlock describes a PDF attachment that can be added during output.
type AttachmentBlock struct {
	Name        string
	MIMEType    string
	Description string
	Data        []byte
}
