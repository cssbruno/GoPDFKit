/****************************************************************************
 * Software: GoPDFKit                                                         *
 * License:  MIT License                                                    *
 *                                                                          *
 *                *
 * Copyright (c) 2026 cssBruno                                              *
 ****************************************************************************/

package gopdfkit

import "io"

// FontLoader is used to read fonts (JSON font specification and zlib compressed font binaries)
// from arbitrary locations (e.g. files, zip files, embedded font resources).
//
// Open provides an io.Reader for the specified font file (.json or .z). The file name
// never includes a path. Open returns an error if the specified file cannot be opened.
type FontLoader interface {
	Open(name string) (io.Reader, error)
}
