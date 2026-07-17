// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package pdfcdr

import "testing"

func BenchmarkSanitizeActiveFixture(b *testing.B) {
	source := activeSourcePDF(b)
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
