// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package sign

import "testing"

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
		_, _ = VerifyCMS(input, nil)
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
