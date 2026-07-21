// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package sign

import (
	"context"
	"testing"
)

func FuzzReadDER(f *testing.F) {
	f.Add([]byte{0x30, 0x00})
	f.Add([]byte{0x04, 0x01, 0x00})
	f.Fuzz(func(t *testing.T, input []byte) {
		_, _, _ = readDER(input)
	})
}

func FuzzInspectCMS(f *testing.F) {
	f.Add([]byte{0x30, 0x00})
	f.Add(derSequence(derOID(oidSignedData), der(0xa0, derSequence())))
	f.Add([]byte("not cms"))
	f.Fuzz(func(t *testing.T, input []byte) {
		_, _ = InspectCMS(input)
	})
}

func FuzzVerifyCMS(f *testing.F) {
	f.Add([]byte{0x30, 0x00})
	f.Add(derSequence(derOID(oidSignedData), der(0xa0, derSequence())))
	f.Add([]byte("not cms"))
	f.Fuzz(func(t *testing.T, input []byte) {
		_, _ = VerifyCMSIntegrity(input)
	})
}

func FuzzPDFString(f *testing.F) {
	f.Add("plain")
	f.Add("(paren) \\ slash")
	f.Add("\x00\x01\n\r")
	f.Fuzz(func(t *testing.T, input string) {
		_ = pdfString(input)
	})
}

func FuzzPDFSignatureScanner(f *testing.F) {
	f.Add([]byte("<< /Type /Sig /ByteRange [0 1 2 0] /Contents <00> >>"))
	f.Add([]byte("<< /Length 57 >>\nstream\n<< /Type /Sig /ByteRange [0 1 2 3] /Contents <00> >>\nendstream"))
	f.Add(minimalPDFBytes())
	f.Fuzz(func(t *testing.T, input []byte) {
		_ = SignatureCount(input)
		_, _ = ExtractByteRange(input)
		extracted, err := ExtractSignature(input)
		if err == nil {
			if len(extracted.ByteRange) != byteRangeLength {
				t.Fatalf("successful extraction returned %d ByteRange values", len(extracted.ByteRange))
			}
			if extracted.ContentsStart < 0 || extracted.ContentsEnd > len(input) || extracted.ContentsStart >= extracted.ContentsEnd {
				t.Fatalf("successful extraction returned invalid Contents range [%d,%d)", extracted.ContentsStart, extracted.ContentsEnd)
			}
		}
	})
}

func FuzzAnalyzePDF(f *testing.F) {
	f.Add(minimalPDFBytes())
	f.Add([]byte("%PDF-1.7\nstartxref\n0\n%%EOF\n"))
	f.Fuzz(func(t *testing.T, input []byte) {
		_, _ = analyzePDFContext(context.Background(), input, DefaultMaxXrefChainLength, DefaultMaxXrefEntries)
	})
}
