// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

// Package outpath resolves generated PDF output paths for examples.
package outpath

import (
	"os"
	"path/filepath"
	"runtime"
)

// File returns name under assets/generated/pdf/examples.
func File(name string) string {
	_, caller, _, ok := runtime.Caller(0)
	if !ok {
		panic("could not resolve example output path")
	}
	dir := filepath.Clean(filepath.Join(filepath.Dir(caller), "..", "..", "..", "assets", "generated", "pdf", "examples"))
	// #nosec G301 -- example PDFs are intentionally readable by the user who runs the example.
	if err := os.MkdirAll(dir, 0o755); err != nil {
		panic(err)
	}
	return filepath.Join(dir, name)
}
