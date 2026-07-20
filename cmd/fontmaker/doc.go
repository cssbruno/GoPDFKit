// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

/*
Command fontmaker generates JSON font definition files for PaperRune.

The command accepts TrueType, OpenType, and binary Type 1 font files and writes
the files needed by PaperRune.AddFont. Type 1 input also requires a sibling AFM
metrics file.
*/
package main
