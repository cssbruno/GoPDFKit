// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

// Package document contains GoPDFKit's canonical high-level PDF API.
//
// New applications should import document directly. The root gopdfkit package
// remains a compatibility facade, and the layout aliases below remain for
// existing callers; neither surface accepts new aliases before a major release.
// Renderer-independent document model types live in the layout package.
package document
