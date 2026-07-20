// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package sign

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
)

var benchmarkXrefOffsetsSink map[int]int
var benchmarkXrefOffsetSink int
var benchmarkSignatureCountSink int

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

func BenchmarkXrefResolverLookupLargeClassic(b *testing.B) {
	const entries = 10000
	input, xrefOffset := benchmarkClassicXrefPDF(entries)
	resolver, err := newPDFXrefResolverContext(context.Background(), input, xrefOffset, 1, entries)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ReportMetric(entries, "lookups/op")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		total := 0
		for object := 1; object < entries; object++ {
			offset, err := resolver.objectOffsetContext(context.Background(), pdfRef{Object: object})
			if err != nil {
				b.Fatal(err)
			}
			total += offset
		}
		benchmarkXrefOffsetSink = total
	}
}

func BenchmarkSignatureDiscoveryLargeFieldTree(b *testing.B) {
	const fields = 2000
	input := benchmarkSignatureFieldTreePDF(fields)
	if got := SignatureCount(input); got != 1 {
		b.Fatalf("SignatureCount() = %d, want 1", got)
	}
	b.ReportAllocs()
	b.ReportMetric(fields, "fields/op")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkSignatureCountSink = SignatureCount(input)
	}
}

func BenchmarkSignatureDiscoverySharedLargeValue(b *testing.B) {
	const (
		fields  = 2000
		padding = 2 << 20
	)
	input := benchmarkSharedSignatureValuePDF(fields, padding)
	if got := SignatureCount(input); got != 1 {
		b.Fatalf("SignatureCount() = %d, want 1", got)
	}
	b.SetBytes(int64(len(input)))
	b.ReportAllocs()
	b.ReportMetric(fields, "fields/op")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkSignatureCountSink = SignatureCount(input)
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
	fmt.Fprintf(&output, "trailer\n<< /Size %d /Root 1 0 R >>\n", entries)
	return output.Bytes(), xrefOffset
}

func benchmarkSignatureFieldTreePDF(fields int) []byte {
	objects := make([]string, 4+fields+1)
	objects[0] = "<< /Type /Catalog /Pages 2 0 R /AcroForm 4 0 R >>"
	objects[1] = "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"
	objects[2] = "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] >>"
	var refs strings.Builder
	for i := 0; i < fields; i++ {
		fmt.Fprintf(&refs, "%d 0 R ", 5+i)
		objects[4+i] = "<< /FT /Sig /V null >>"
	}
	signatureObject := 5 + fields
	objects[3] = "<< /Fields [" + refs.String() + "] >>"
	objects[4+fields-1] = fmt.Sprintf("<< /FT /Sig /V %d 0 R >>", signatureObject)
	objects[4+fields] = "<< /ByteRange [0 1 2 0] /Contents <00> >>"
	return minimalClassicPDF(objects...)
}

func benchmarkSharedSignatureValuePDF(fields, padding int) []byte {
	objects := make([]string, 5+fields)
	objects[0] = "<< /Type /Catalog /Pages 2 0 R /AcroForm 4 0 R >>"
	objects[1] = "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"
	objects[2] = "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] >>"
	var refs strings.Builder
	for i := 0; i < fields; i++ {
		object := 5 + i
		fmt.Fprintf(&refs, "%d 0 R ", object)
		objects[4+i] = fmt.Sprintf("<< /FT /Sig /V %d 0 R >>", 5+fields)
	}
	objects[3] = "<< /Fields [" + refs.String() + "] >>"
	objects[4+fields] = "<< /ByteRange [0 1 2 0] /Contents <00> /Reason (" + strings.Repeat("x", padding) + ") >>"
	return minimalClassicPDF(objects...)
}
