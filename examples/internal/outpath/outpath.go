// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

// Package outpath resolves generated PDF output paths for examples.
package outpath

import (
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/cssbruno/gopdfkit/document"
)

func init() {
	fixed := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	document.SetDefaultCreationDate(fixed)
	document.SetDefaultModificationDate(fixed)
}

// File returns name under assets/generated/pdf/examples.
func File(name string) string {
	_, caller, _, ok := runtime.Caller(0)
	if !ok {
		panic("could not resolve example output path")
	}
	dir := filepath.Clean(filepath.Join(filepath.Dir(caller), "..", "..", "..", "assets", "generated", "pdf", "examples"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		panic(err)
	}
	return filepath.Join(dir, name)
}
