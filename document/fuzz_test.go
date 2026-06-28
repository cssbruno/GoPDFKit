// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
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
	f.Add([]byte("not an image"), "png")
	f.Fuzz(func(t *testing.T, input []byte, imageType string) {
		_, _, _ = parseImageOptionsReader(ImageOptions{ImageType: imageType}, bytes.NewReader(input), 1, defaultCompressionLevel(), "")
	})
}
