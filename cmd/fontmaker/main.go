// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package main

import (
	"io"
	"os"

	"github.com/cssbruno/paperrune/font"
)

type fontBuildFunc func(fontPath, encodingPath, outputDir string, log io.Writer, embed bool) error

func main() {
	cmd := newCommand(os.Args[0], os.Stdout, os.Stderr, font.Make)
	os.Exit(cmd.run(os.Args[1:]))
}
