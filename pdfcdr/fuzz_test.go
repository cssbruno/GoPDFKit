// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package pdfcdr

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func FuzzSanitizePDFObject(f *testing.F) {
	for _, seed := range []string{
		"<< /Font << /F1 6 0 R >> /AA 7 0 R >>",
		"[<< /Type /Action /S /JavaScript /JS (x) >>]",
		"<< /Length 3 >>\nstream\nabc\nendstream",
		"(literal (nested) string)",
		"<48656c6c6f>",
		"\x00\v%",
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, input []byte) {
		output, err := sanitizePDFObject(input)
		if err != nil {
			return
		}
		if _, err := sanitizePDFObject(output); err != nil {
			t.Fatalf("sanitized output is not stable: %v", err)
		}
	})
}

func FuzzSanitizePDF(f *testing.F) {
	f.Add(activeSourcePDF(f))
	f.Add([]byte("not a pdf"))
	options := Options{
		MaxSourceBytes:       2 * 1024 * 1024,
		MaxReferencedObjects: 4096,
		MaxPages:             256,
		MaxDecodedBytes:      2 * 1024 * 1024,
		MaxResourceBytes:     2 * 1024 * 1024,
		MaxOutputBytes:       2 * 1024 * 1024,
		MaxObjects:           4096,
	}
	f.Fuzz(func(t *testing.T, input []byte) {
		output, err := SanitizeContext(context.Background(), input, options)
		if err != nil {
			return
		}
		again, err := SanitizeContext(context.Background(), output, options)
		if err != nil {
			t.Fatalf("sanitized PDF cannot be sanitized again: %v", err)
		}
		if !bytes.Equal(output, again) {
			t.Fatal("sanitization is not idempotent")
		}
	})
}

func TestSanitizePDFObjectNestingLimit(t *testing.T) {
	input := strings.Repeat("[", maxPDFValueNesting+1) + strings.Repeat("]", maxPDFValueNesting+1)
	if _, err := sanitizePDFObject([]byte(input)); err == nil {
		t.Fatal("sanitizePDFObject accepted excessive nesting")
	}
}
