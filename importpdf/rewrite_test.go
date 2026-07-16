// SPDX-License-Identifier: MIT
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
