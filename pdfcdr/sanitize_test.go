// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package pdfcdr

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestSanitizePDFObjectPreservesRenderingResourceNames(t *testing.T) {
	for container := range resourceNameDictionaryKeys {
		for resourceName := range removedPDFKeys {
			name := container + "/" + resourceName
			t.Run(name, func(t *testing.T) {
				input := []byte(fmt.Sprintf("<< /%s << /%s 9 0 R >> >>", container, resourceName))
				want := input
				if !rejectedExternalKeys[resourceName] {
					input = []byte(fmt.Sprintf(
						"<< /%s << /%s 9 0 R >> /%s 10 0 R >>",
						container, resourceName, resourceName,
					))
					want = []byte(fmt.Sprintf("<< /%s << /%s 9 0 R >> >>", container, resourceName))
				}
				got, err := sanitizePDFObject(input)
				if err != nil {
					t.Fatalf("sanitizePDFObject() error = %v", err)
				}
				if !bytes.Equal(got, want) {
					t.Fatalf("sanitizePDFObject() = %q, want %q", got, want)
				}
			})
		}
	}
}

func TestSanitizePDFObjectPreservesEscapedRenderingResourceIdentity(t *testing.T) {
	input := []byte("<< /Font << /F#31 9 0 R >> >>")
	got, err := sanitizePDFObject(input)
	if err != nil {
		t.Fatalf("sanitizePDFObject() error = %v", err)
	}
	if !bytes.Contains(got, []byte("/F#31 9 0 R")) {
		t.Fatalf("sanitizePDFObject() = %q, changed escaped resource name", got)
	}
}

func TestSanitizePDFObjectCanonicalizesNamesForPolicyChecks(t *testing.T) {
	input := []byte("<< /O#70enAction << /Type /Action /S /Java#53cript /J#53 (x) >> /Safe true >>")
	got, err := sanitizePDFObject(input)
	if err != nil {
		t.Fatalf("sanitizePDFObject() error = %v", err)
	}
	if bytes.Contains(got, []byte("O#70enAction")) || bytes.Contains(got, []byte("Java#53cript")) {
		t.Fatalf("sanitizePDFObject() = %q, retained escaped active content", got)
	}
	if !bytes.Contains(got, []byte("/Safe true")) {
		t.Fatalf("sanitizePDFObject() = %q, removed safe value", got)
	}
}

func TestSanitizePDFObjectRejectsExternalAndPostScriptConstructs(t *testing.T) {
	for name, input := range map[string]string{
		"external file":       "<< /#46 (external.pdf) >>",
		"external filter":     "<< /F#46ilter /FlateDecode >>",
		"reference XObject":   "<< /R#65f << /F (external.pdf) >> >>",
		"PostScript XObject":  "<< /Subtype /P#53 >>",
		"indirect subtype":    "<< /Subtype 9 0 R >>",
		"indirect filter":     "<< /Filter 9 0 R >>",
		"indirect parameters": "<< /DecodeParms [9 0 R] >>",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := sanitizePDFObject([]byte(input)); err == nil {
				t.Fatalf("sanitizePDFObject(%q) error = nil, want policy rejection", input)
			}
		})
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

func TestSanitizePDFObjectRejectsTokensAfterDeclaredStream(t *testing.T) {
	for name, length := range map[string]string{
		"direct length":   "3",
		"indirect length": "9 0 R",
	} {
		t.Run(name, func(t *testing.T) {
			input := []byte(fmt.Sprintf(
				"<< /Length %s >>\nstream\nabc\nendstream\n/JavaScript true\nendstream",
				length,
			))
			_, err := sanitizePDFObject(input)
			if err == nil || !strings.Contains(err.Error(), "unexpected bytes after PDF endstream") {
				t.Fatalf("sanitizePDFObject() error = %v, want trailing-token rejection", err)
			}
		})
	}
}

func TestSanitizePDFObjectUsesDeclaredStreamLength(t *testing.T) {
	payload := []byte("abc\nendstream\nxyz")
	input := []byte(fmt.Sprintf(
		"<< /Length %d >>\nstream\n%s\nendstream",
		len(payload),
		payload,
	))
	got, err := sanitizePDFObject(input)
	if err != nil {
		t.Fatalf("sanitizePDFObject() error = %v", err)
	}
	if !bytes.Equal(got, input) {
		t.Fatalf("sanitizePDFObject() = %q, want %q", got, input)
	}
}

func TestSanitizePDFObjectAllowsSingleEndstreamWithIndirectLength(t *testing.T) {
	input := []byte("<< /Length 9 0 R >>\nstream\nabc\nendstream")
	got, err := sanitizePDFObject(input)
	if err != nil {
		t.Fatalf("sanitizePDFObject() error = %v", err)
	}
	if !bytes.Equal(got, input) {
		t.Fatalf("sanitizePDFObject() = %q, want %q", got, input)
	}
}
