// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package pdfcdr

import (
	"bytes"
	"fmt"
	"testing"
)

func TestSanitizePDFObjectPreservesRenderingResourceNames(t *testing.T) {
	for container := range resourceNameDictionaryKeys {
		for resourceName := range removedPDFKeys {
			name := container + "/" + resourceName
			t.Run(name, func(t *testing.T) {
				input := []byte(fmt.Sprintf(
					"<< /%s << /%s 9 0 R >> /%s 10 0 R >>",
					container, resourceName, resourceName,
				))
				got, err := sanitizePDFObject(input)
				if err != nil {
					t.Fatalf("sanitizePDFObject() error = %v", err)
				}
				want := []byte(fmt.Sprintf("<< /%s << /%s 9 0 R >> >>", container, resourceName))
				if !bytes.Equal(got, want) {
					t.Fatalf("sanitizePDFObject() = %q, want %q", got, want)
				}
			})
		}
	}
}

func TestSanitizePDFObjectDropsInlineActionFromResourceDictionary(t *testing.T) {
	input := []byte("<< /Font << /Safe 9 0 R /Evil << /Type /Action /S /JavaScript /JS (app.alert) >> >> >>")
	got, err := sanitizePDFObject(input)
	if err != nil {
		t.Fatalf("sanitizePDFObject() error = %v", err)
	}
	if !bytes.Contains(got, []byte("/Safe 9 0 R")) {
		t.Fatalf("sanitizePDFObject() = %q, want safe resource", got)
	}
	for _, forbidden := range [][]byte{[]byte("/Evil"), []byte("/Action"), []byte("/JavaScript"), []byte("/JS")} {
		if bytes.Contains(got, forbidden) {
			t.Fatalf("sanitizePDFObject() = %q, contains %q", got, forbidden)
		}
	}
}
