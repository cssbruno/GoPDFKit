// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package pdfcdr

import (
	"os"
	"testing"
)

func BenchmarkSanitizeActiveFixture(b *testing.B) {
	source := activeSourcePDF(b)
	benchmarkSanitize(b, source)
}

func BenchmarkSanitizeDocumentMultiCell(b *testing.B) {
	source, err := os.ReadFile("../assets/generated/pdf/Document_MultiCell.pdf")
	if err != nil {
		b.Fatal(err)
	}
	benchmarkSanitize(b, source)
}

func benchmarkSanitize(b *testing.B, source []byte) {
	b.Helper()
	b.ReportAllocs()
	b.SetBytes(int64(len(source)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Sanitize(source); err != nil {
			b.Fatal(err)
		}
	}
}
