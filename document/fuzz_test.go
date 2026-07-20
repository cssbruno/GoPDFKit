// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func FuzzCompileHTML(f *testing.F) {
	f.Add("<p>Hello</p>")
	f.Add(`<table><tr><td>A</td></tr></table>`)
	f.Add(`<img src="data:image/png;base64,AAAA">`)
	f.Fuzz(func(t *testing.T, input string) {
		_, _ = CompileHTML(input)
	})
}

func FuzzCompileHTMLDataImageSource(f *testing.F) {
	f.Add("data:image/png;base64,AAAA")
	f.Add("data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///ywAAAAAAQABAAACAUwAOw==")
	f.Add("not a data URI")
	f.Fuzz(func(t *testing.T, input string) {
		_, _, _ = compileHTMLDataImageSource(input, 1024)
	})
}

func FuzzHTMLTokenize(f *testing.F) {
	f.Add("<section><p>Hello</p></section>")
	f.Fuzz(func(t *testing.T, input string) {
		_ = HTMLTokenize(input)
	})
}

func FuzzSVGParse(f *testing.F) {
	f.Add([]byte(`<svg width="1" height="1"><path d="M0 0L1 1"/></svg>`))
	f.Fuzz(func(t *testing.T, input []byte) {
		_, _ = SVGParse(input)
	})
}

func FuzzDeserializeTemplate(f *testing.F) {
	tpl := CreateTpl(Point{0, 0}, Size{Wd: 10, Ht: 10}, "P", "mm", ".", func(t *Tpl) {
		t.RawWriteStr("0 0 m")
	})
	if tpl != nil {
		if encoded, err := tpl.Serialize(); err == nil {
			f.Add(encoded)
		}
	}
	f.Add([]byte("not-a-template"))
	f.Fuzz(func(t *testing.T, input []byte) {
		_, _ = DeserializeTemplate(input)
	})
}

func FuzzParseImageOptionsReader(f *testing.F) {
	f.Add([]byte{0xff, 0xd8, 0xff, 0xd9}, "jpg")
	if png, err := hex.DecodeString("89504e470d0a1a0a0000000d49484452000000010000000108060000001f15c4890000000a49444154789c6360000002000100ffff03000006000557bfab0d0000000049454e44ae426082"); err == nil {
		f.Add(png, "png")
	}
	f.Add([]byte("GIF89a\x01\x00\x01\x00\x80\x00\x00\x00\x00\x00\xff\xff\xff,\x00\x00\x00\x00\x01\x00\x01\x00\x00\x02\x02D\x01\x00;"), "gif")
	f.Add([]byte("RIFF\x1a\x00\x00\x00WEBPVP8 \x0e\x00\x00\x00\x10\x00\x00\x9d\x01\x2a\x01\x00\x01\x00\x02\x00\x34\x25\xa4\x00\x03p\x00\xfe\xfb\xfdP\x00"), "webp")
	f.Add([]byte("not an image"), "png")
	f.Fuzz(func(t *testing.T, input []byte, imageType string) {
		_, _, _ = parseImageOptionsReader(ImageOptions{ImageType: imageType}, bytes.NewReader(input), 1, defaultCompressionLevel(), "")
	})
}

func FuzzAppendEscapedPDFCellText(f *testing.F) {
	f.Add("plain text")
	f.Add("(paren) \\ slash")
	f.Add("\x00\x01\n\r")
	f.Fuzz(func(t *testing.T, input string) {
		_ = appendEscapedPDFCellText(nil, input)
	})
}
