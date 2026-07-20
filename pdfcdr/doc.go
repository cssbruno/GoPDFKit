// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

// Package pdfcdr provides PDF Content Disarm and Reconstruction (CDR).
//
// CDR parses a supported PDF, keeps its page content and reachable page
// resources, removes interactive and non-rendering document structures, and
// writes a new classic-xref PDF. The output does not retain the source
// catalog, page tree, annotations, attachments, metadata, or actions.
//
// The package uses the same intentionally narrow parser as importpdf: classic
// xref-table, unencrypted PDFs with unfiltered or FlateDecode page content.
// It is a reconstruction boundary, not a PDF renderer or a guarantee against
// vulnerabilities in a downstream PDF viewer or in embedded image/font
// decoders.
package pdfcdr
