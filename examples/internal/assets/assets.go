// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

// Package assets resolves repository fixture files for examples.
package assets

import (
	"path/filepath"
	"runtime"
)

// File returns a path under assets/static.
func File(parts ...string) string {
	_, caller, _, ok := runtime.Caller(0)
	if !ok {
		panic("could not resolve example asset path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(caller), "..", "..", ".."))
	path := append([]string{root, "assets", "static"}, parts...)
	return filepath.Join(path...)
}
