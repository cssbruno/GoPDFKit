// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

// Package paperlang parses the small, human-readable .paper source language.
//
// The package deliberately stops at syntax. It preserves source spelling,
// interpolation text, indentation structure, readable IDs, and exact source
// spans without importing document renderers or layout engines. Later compiler
// stages may resolve interpolation and lower the deterministic AST into layout.
// ParseLossless provides a separate exact-source CST/trivia snapshot for
// editors and future-syntax round trips; canonical AST formatting remains an
// explicit operation.
package paperlang
