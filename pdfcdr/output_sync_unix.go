// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package pdfcdr

import "os"

func syncOutputDirectory(path string) error {
	dir, err := os.Open(path) // #nosec G304 -- path is the parent of the caller-selected output file.
	if err != nil {
		return err
	}
	defer func() { _ = dir.Close() }()
	return dir.Sync()
}
