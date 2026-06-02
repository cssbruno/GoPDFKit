// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package gopdfkit

import "io"

// FontLoader reads font resources from arbitrary locations, such as files, zip
// archives or embedded font resources.
//
// Open provides an io.Reader for the specified font file (.json or .z). The
// file name never includes a path. Open returns an error if the specified file
// cannot be opened.
type FontLoader interface {
	Open(name string) (io.Reader, error)
}
