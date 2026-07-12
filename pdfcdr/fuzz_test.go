// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package pdfcdr

import (
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

func TestSanitizePDFObjectNestingLimit(t *testing.T) {
	input := strings.Repeat("[", maxPDFValueNesting+1) + strings.Repeat("]", maxPDFValueNesting+1)
	if _, err := sanitizePDFObject([]byte(input)); err == nil {
		t.Fatal("sanitizePDFObject accepted excessive nesting")
	}
}
