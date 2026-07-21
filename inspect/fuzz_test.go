// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package inspect

import (
	"strings"
	"testing"
)

func FuzzDecodedStreamsAndText(f *testing.F) {
	f.Add([]byte("%PDF-1.4\n1 0 obj\n<</Length 12>>\nstream\nBT (Hi) Tj\nendstream\nendobj\n%%EOF"))
	f.Add([]byte("not a pdf"))
	f.Fuzz(func(t *testing.T, input []byte) {
		_, _ = DecodedStreams(input)
		text, err := Text(input)
		if err != nil {
			return
		}
		pages, err := PageCount(input)
		if err != nil {
			t.Fatalf("Text succeeded but PageCount failed: %v", err)
		}
		var pageText strings.Builder
		for page := 1; page <= pages; page++ {
			text, err := PageText(input, page)
			if err != nil {
				t.Fatalf("Text succeeded but PageText(%d) failed: %v", page, err)
			}
			pageText.WriteString(text)
		}
		if text != pageText.String() {
			t.Fatalf("Text = %q, concatenated PageText = %q", text, pageText.String())
		}
	})
}
