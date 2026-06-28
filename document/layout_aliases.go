// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import "github.com/cssbruno/gopdfkit/layout"

// DocumentKind identifies the high-level purpose of a generated document.
type DocumentKind = layout.DocumentKind

const (
	DocumentKindGeneric       = layout.DocumentKindGeneric
	DocumentKindReport        = layout.DocumentKindReport
	DocumentKindForm          = layout.DocumentKindForm
	DocumentKindLetter        = layout.DocumentKindLetter
	DocumentKindTransactional = layout.DocumentKindTransactional
	DocumentKindAttestation   = layout.DocumentKindAttestation
	DocumentKindStatement     = layout.DocumentKindStatement
	DocumentKindLongForm      = layout.DocumentKindLongForm
)

// LayoutDocument is the shared model that document builders and HTML parsers can
// produce before PDF layout and drawing.
type LayoutDocument = layout.LayoutDocument

// NewLayoutDocument creates a document model with a generic kind when kind is empty.
func NewLayoutDocument(kind DocumentKind) *LayoutDocument {
	return layout.NewLayoutDocument(kind)
}

type DocumentMetadata = layout.DocumentMetadata
type DocumentParty = layout.DocumentParty
type MetadataField = layout.MetadataField

// BlockKind identifies a shared layout block.
type BlockKind = layout.BlockKind

const (
	BlockKindParagraph      = layout.BlockKindParagraph
	BlockKindHeading        = layout.BlockKindHeading
	BlockKindList           = layout.BlockKindList
	BlockKindTable          = layout.BlockKindTable
	BlockKindImage          = layout.BlockKindImage
	BlockKindSignatureRow   = layout.BlockKindSignatureRow
	BlockKindMetadataGrid   = layout.BlockKindMetadataGrid
	BlockKindQRVerification = layout.BlockKindQRVerification
	BlockKindNoteBox        = layout.BlockKindNoteBox
	BlockKindSection        = layout.BlockKindSection
	BlockKindClause         = layout.BlockKindClause
	BlockKindPageBreak      = layout.BlockKindPageBreak
)

type Block = layout.Block
type DocumentColor = layout.DocumentColor
type TextStyle = layout.TextStyle
type BoxStyle = layout.BoxStyle
type Spacing = layout.Spacing
type BorderStyle = layout.BorderStyle
type BorderSide = layout.BorderSide
type TextSegment = layout.TextSegment
type ParagraphBlock = layout.ParagraphBlock
type HeadingBlock = layout.HeadingBlock
type ListBlock = layout.ListBlock
type ListItem = layout.ListItem
type TableBlock = layout.TableBlock
type TableColumn = layout.TableColumn
type TableStyle = layout.TableStyle
type TableRow = layout.TableRow
type TableCell = layout.TableCell
type ImageFitMode = layout.ImageFitMode

const (
	ImageFitAuto    = layout.ImageFitAuto
	ImageFitContain = layout.ImageFitContain
	ImageFitCover   = layout.ImageFitCover
)

type ImageBlock = layout.ImageBlock
type SignatureBlock = layout.SignatureBlock
type SignatureRowBlock = layout.SignatureRowBlock
type SignatureColumn = layout.SignatureColumn
type MetadataGridBlock = layout.MetadataGridBlock
type QRBlock = layout.QRBlock
type PageTemplate = layout.PageTemplate
type PageNumberOptions = layout.PageNumberOptions
type QRVerificationBlock = layout.QRVerificationBlock
type NoteBoxBlock = layout.NoteBoxBlock
type SectionBlock = layout.SectionBlock
type ClauseBlock = layout.ClauseBlock
type PageBreakBlock = layout.PageBreakBlock
type HeaderBlock = layout.HeaderBlock
type FooterBlock = layout.FooterBlock
type AttachmentBlock = layout.AttachmentBlock
