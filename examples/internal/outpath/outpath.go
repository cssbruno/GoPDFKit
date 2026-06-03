// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

// Package outpath resolves example output files beside the calling example.
package outpath

import (
	"path/filepath"
	"runtime"
)

// File returns name in the directory of the calling source file.
func File(name string) string {
	_, caller, _, ok := runtime.Caller(1)
	if !ok {
		panic("could not resolve example output path")
	}
	return filepath.Join(filepath.Dir(caller), name)
}
