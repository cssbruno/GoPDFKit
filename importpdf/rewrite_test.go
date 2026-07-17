// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package importpdf

import (
	"bytes"
	"testing"
)

func TestRewriteIndirectRefsInsideDictionary(t *testing.T) {
	input := []byte("<< /Font << /F1 9 0 R >> /Label (9 0 R) >>")
	got := RewriteIndirectRefs(input, map[ObjRef]int{{num: 9, gen: 0}: 5})
	if !bytes.Contains(got, []byte("/F1 5 0 R")) {
		t.Fatalf("RewriteIndirectRefs() = %q, want dictionary reference rewritten", got)
	}
	if !bytes.Contains(got, []byte("/Label (9 0 R)")) {
		t.Fatalf("RewriteIndirectRefs() = %q, want literal string unchanged", got)
	}
}

func TestRewriteIndirectRefsDoesNotTreatStringOrNameAsStream(t *testing.T) {
	for name, marker := range map[string]string{
		"literal string": "(stream)",
		"PDF name":       "/stream",
	} {
		t.Run(name, func(t *testing.T) {
			input := []byte("<< /Marker " + marker + " /Font 9 0 R >>")
			got := RewriteIndirectRefs(input, map[ObjRef]int{{num: 9, gen: 0}: 5})
			if !bytes.Contains(got, []byte("/Font 5 0 R")) {
				t.Fatalf("RewriteIndirectRefs() = %q, want reference after %s rewritten", got, marker)
			}
		})
	}
}

func TestRewriteIndirectRefsLeavesStreamBodyUnchanged(t *testing.T) {
	for name, separator := range map[string]string{
		"after line break":           "\nstream\n",
		"after dictionary delimiter": "stream\n",
	} {
		t.Run(name, func(t *testing.T) {
			input := []byte("<< /Child 9 0 R /Length 6 >>" + separator + "9 0 R\nendstream")
			got := RewriteIndirectRefs(input, map[ObjRef]int{{num: 9, gen: 0}: 5})
			if !bytes.Contains(got, []byte("/Child 5 0 R")) {
				t.Fatalf("RewriteIndirectRefs() = %q, want stream dictionary reference rewritten", got)
			}
			if !bytes.Contains(got, []byte(separator+"9 0 R\nendstream")) {
				t.Fatalf("RewriteIndirectRefs() = %q, want stream body unchanged", got)
			}
		})
	}
}
