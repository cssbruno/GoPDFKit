// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"fmt"
	"strings"
	"testing"
)

func BenchmarkHTMLTokenizeSmallAttributes(b *testing.B) {
	const fragment = `<p id="target" class="note" style="color:#123456">Token text</p>`
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if tokens := HTMLTokenize(fragment); len(tokens) == 0 {
			b.Fatal("HTMLTokenize returned no tokens")
		}
	}
}

func BenchmarkHTMLTokenizeLargeFragment(b *testing.B) {
	fragment := benchmarkLargeHTMLFragment()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if tokens := HTMLTokenize(fragment); len(tokens) == 0 {
			b.Fatal("HTMLTokenize returned no tokens")
		}
	}
}

func BenchmarkCompileHTMLLargeFragment(b *testing.B) {
	fragment := benchmarkLargeHTMLFragment()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		compiled, err := CompileHTML(fragment)
		if err != nil {
			b.Fatalf("CompileHTML() error = %v", err)
		}
		if stats := compiled.Stats(); stats.Tokens == 0 || stats.Nodes == 0 {
			b.Fatalf("CompileHTML stats = %#v, want populated tokens and nodes", stats)
		}
	}
}

func benchmarkLargeHTMLFragment() string {
	var out strings.Builder
	out.WriteString(`<style>.report p.note{color:#123456}.report .num{text-align:right}</style><section class="report">`)
	for section := 0; section < 24; section++ {
		fmt.Fprintf(&out, `<article id="section-%02d"><h2>Section %02d</h2>`, section, section)
		for row := 0; row < 32; row++ {
			fmt.Fprintf(&out, `<p class="note">Large fragment section %02d row %03d <strong>value</strong> <span class="num">%0.2f</span></p>`, section, row, float64(section*row)*1.25)
		}
		out.WriteString(`</article>`)
	}
	out.WriteString(`</section>`)
	return out.String()
}
