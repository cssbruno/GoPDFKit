// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package importpdf

import (
	"bytes"
	"fmt"
	"testing"
)

func FuzzOpenBytes(f *testing.F) {
	f.Add([]byte("%PDF-1.4\n%%EOF"))
	f.Add(minimalFuzzPDF())
	f.Add([]byte("not a pdf"))
	f.Fuzz(func(t *testing.T, input []byte) {
		_, _ = OpenBytes(input)
	})
}

func minimalFuzzPDF() []byte {
	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n")
	offsets := []int{0}
	writeObj := func(body string) {
		offsets = append(offsets, out.Len())
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", len(offsets)-1, body)
	}
	writeObj("<</Type /Catalog /Pages 2 0 R>>")
	writeObj("<</Type /Pages /Kids [3 0 R] /Count 1>>")
	writeObj("<</Type /Page /Parent 2 0 R /MediaBox [0 0 10 10] /Contents 4 0 R /Resources <<>>>>")
	writeObj("<</Length 8>>\nstream\n0 0 m\nS\nendstream")
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n", len(offsets))
	out.WriteString("0000000000 65535 f \n")
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&out, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&out, "trailer\n<</Size %d /Root 1 0 R>>\nstartxref\n%d\n%%%%EOF\n", len(offsets), xref)
	return out.Bytes()
}
