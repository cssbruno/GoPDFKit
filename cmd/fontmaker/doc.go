// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

/*
Command fontmaker generates JSON font definition files for GoPDFKit.

The command accepts TrueType, OpenType, and binary Type 1 font files and writes
the files needed by GoPDFKit.AddFont. Type 1 input also requires a sibling AFM
metrics file.
*/
package main
