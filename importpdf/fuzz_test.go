// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package importpdf

import "testing"

func FuzzOpenBytes(f *testing.F) {
	f.Add([]byte("%PDF-1.4\n%%EOF"))
	f.Add([]byte("not a pdf"))
	f.Fuzz(func(t *testing.T, input []byte) {
		_, _ = OpenBytes(input)
	})
}
