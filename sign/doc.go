// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

// Package sign provides PDF signing and verification APIs.
//
// Source-PDF signing intentionally supports unencrypted classic-xref PDFs with
// no existing AcroForm and a direct page annotation array. Unsupported source
// structures return an error matching ErrUnsupportedPDF. Verification also
// requires a supported classic xref chain, follows the current catalog's
// AcroForm field tree, and ignores unreferenced signature dictionaries.
// ExtractSingleSignature is available when callers require exactly one
// reachable signature.
package sign
