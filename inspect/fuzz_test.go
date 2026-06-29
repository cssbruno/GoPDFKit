// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package inspect

import "testing"

func FuzzDecodedStreamsAndText(f *testing.F) {
	f.Add([]byte("%PDF-1.4\n1 0 obj\n<</Length 12>>\nstream\nBT (Hi) Tj\nendstream\nendobj\n%%EOF"))
	f.Add([]byte("not a pdf"))
	f.Fuzz(func(t *testing.T, input []byte) {
		_, _ = DecodedStreams(input)
		_, _ = Text(input)
	})
}
