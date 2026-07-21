// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package main

import (
	"fmt"
	"log"
	"path/filepath"

	"github.com/cssbruno/gopdfkit/document"
	"github.com/cssbruno/gopdfkit/examples/internal/assets"
	"github.com/cssbruno/gopdfkit/examples/internal/outpath"
)

func main() {
	pdf := document.MustNew()
	pdf.SetTitle("HTML Images", false)
	pdf.SetCreator("examples/html-images", false)
	pdf.SetMargins(16, 16, 16)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 10)

	imagePath := filepath.ToSlash(assets.File("image", "document.png"))
	html := pdf.HTMLNew()
	html.AllowLocalImages = true
	html.Write(5.5, fmt.Sprintf(`
		<style>
			h1 { color: #243447; margin-bottom: 6px; }
			p { color: #3c4652; line-height: 1.35; }
			figcaption {
				color: #5d6673;
				font-size: 9pt;
				text-align: center;
				margin-top: 4px;
			}
			.note {
				background-color: #eef7f2;
				border-left: 3px solid #2f8f5b;
				padding: 7px 9px;
				margin-top: 8px;
			}
		</style>
		<h1>HTML Images and SVG</h1>
		<p>This example enables local image rendering for trusted file paths, keeps images with captions, and renders a small inline SVG directly from the HTML fragment.</p>
		<figure>
			<img src="%s" alt="GoPDFKit logo" width="72mm" height="28mm" style="object-fit: contain; text-align: center">
			<figcaption>Local PNG loaded through HTMLNew with AllowLocalImages enabled.</figcaption>
		</figure>
		<figure>
			<svg width="72mm" height="25mm" viewBox="0 0 160 56">
				<rect x="1" y="1" width="158" height="54" rx="8" fill="#f0f4f8" stroke="#6f7d8c"/>
				<circle cx="30" cy="28" r="14" fill="#2f80ed"/>
				<path d="M58 18 L134 18 M58 28 L122 28 M58 38 L142 38" stroke="#243447" stroke-width="4" stroke-linecap="round"/>
			</svg>
			<figcaption>Inline SVG rendered as PDF drawing operations.</figcaption>
		</figure>
		<p class="note"><strong>Note:</strong> Use data URLs or opt in to local paths for deterministic PDF generation. Remote images are intentionally not loaded.</p>
	`, imagePath))

	if err := pdf.OutputFileAndClose(outpath.File("html-images.pdf")); err != nil {
		log.Fatal(err)
	}
}
