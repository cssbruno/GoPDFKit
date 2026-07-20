// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

//go:build !js || !wasm

// Command paper-studio-wasm is built for js/wasm by the Paper Studio asset
// pipeline. The host stub keeps repository-wide native build tooling complete.
package main

import "fmt"

func main() {
	fmt.Println("paper-studio-wasm: build with GOOS=js GOARCH=wasm")
}
