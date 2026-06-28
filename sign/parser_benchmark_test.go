// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package sign

import (
	"bytes"
	"fmt"
	"testing"
)

var benchmarkXrefOffsetsSink map[int]int

func BenchmarkParseXrefTableLargeClassic(b *testing.B) {
	const entries = 80000
	input, xrefOffset := benchmarkClassicXrefPDF(entries)

	b.SetBytes(int64(len(input) - xrefOffset))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		offsets, err := parseXrefTable(input, xrefOffset)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkXrefOffsetsSink = offsets
	}
}

func benchmarkClassicXrefPDF(entries int) ([]byte, int) {
	var output bytes.Buffer
	output.WriteString("%PDF-1.7\n")
	xrefOffset := output.Len()
	fmt.Fprintf(&output, "xref\n0 %d\n", entries)
	output.WriteString("0000000000 65535 f \n")
	for i := 1; i < entries; i++ {
		fmt.Fprintf(&output, "%010d 00000 n \n", i*17)
	}
	output.WriteString("trailer\n<< /Size 80000 /Root 1 0 R >>\n")
	return output.Bytes(), xrefOffset
}
