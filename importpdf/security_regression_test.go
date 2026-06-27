// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package importpdf

import (
	"bytes"
	"compress/zlib"
	"strings"
	"testing"
)

func TestSecurityDecodedStreamLimit(t *testing.T) {
	compressed := zlibBytes(bytes.Repeat([]byte{'A'}, MaxDecodedStreamBytes+1))
	_, err := decodePDFStream(pdfDict{"Filter": pdfValue{kind: pdfValueName, name: "FlateDecode"}}, compressed)
	if err == nil || !strings.Contains(err.Error(), "uncompressed data exceeds expected size") {
		t.Fatalf("decodePDFStream() error = %v, want decoded stream size limit", err)
	}
}

func TestSecurityValueArrayLimit(t *testing.T) {
	var input strings.Builder
	input.WriteByte('[')
	for range MaxArrayItems + 1 {
		input.WriteString("0 ")
	}
	input.WriteByte(']')

	_, err := newPDFValueParser([]byte(input.String())).parseValue()
	if err == nil || !strings.Contains(err.Error(), "PDF array exceeds maximum size") {
		t.Fatalf("parseValue() error = %v, want array size limit", err)
	}
}

func zlibBytes(data []byte) []byte {
	var out bytes.Buffer
	writer := zlib.NewWriter(&out)
	_, _ = writer.Write(data)
	_ = writer.Close()
	return out.Bytes()
}
