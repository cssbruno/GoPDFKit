// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package main

import (
	"log"
	"strings"

	"github.com/cssbruno/gopdfkit"
	"github.com/cssbruno/gopdfkit/document"
	"github.com/cssbruno/gopdfkit/examples/internal/outpath"
)

func main() {
	pdf := gopdfkit.New()
	pdf.SetTitle("HTML Flex Edge Cases", false)
	pdf.SetCreator("examples/html-flex-edge-cases", false)
	pdf.SetMargins(14, 16, 14)
	pdf.SetAutoPageBreak(true, 16)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 10)

	fragment := flexEdgeCaseHTML()
	html := pdf.HTMLNew()
	if messages := html.ValidateHTML(fragment); len(messages) > 0 {
		log.Fatalf("unsupported HTML/CSS in example: %s", strings.Join(messages, "; "))
	}
	compiled, err := document.CompileHTML(fragment)
	if err != nil {
		log.Fatal(err)
	}
	html.WriteCompiled(5.2, compiled)

	if err := pdf.OutputFileAndClose(outpath.File("html-flex-edge-cases.pdf")); err != nil {
		log.Fatal(err)
	}
}

func flexEdgeCaseHTML() string {
	return `
		<style>
			h1 { color:#1f3652; font-size:18pt; margin:0 0 3mm 0; }
			h2 { color:#1f3652; font-size:12pt; margin:5mm 0 2mm 0; }
			p { color:#3f4b59; line-height:1.32; margin:0 0 2.5mm 0; }
			.small { color:#687384; font-size:8pt; }
			.box {
				background-color:#f5f8fb;
				border:1px solid #b6c6d5;
				border-radius:2mm;
				padding:2.5mm;
			}
			.empty {
				display:flex;
				gap:2mm;
				border:1px dashed #b6c6d5;
				padding:2mm;
				margin:1mm 0 3mm 0;
			}
			.reverse {
				display:flex;
				flex-direction:row-reverse;
				gap:2mm;
				align-items:center;
				margin:2mm 0 4mm 0;
			}
			.reverse .box { flex:1 1 42mm; min-width:30mm; max-width:58mm; }
			.inline {
				display:inline-flex;
				flex-wrap:wrap;
				column-gap:2mm;
				row-gap:1.5mm;
				margin:2mm 0 4mm 0;
			}
			.inline .box { flex:0 1 48mm; min-width:28mm; max-width:60mm; }
			.board {
				display:flex;
				flex-wrap:wrap;
				row-gap:2mm;
				column-gap:3mm;
				justify-content:space-between;
				align-content:center;
				align-items:flex-start;
				height:72mm;
				border:1px solid #9fb0bf;
				padding:3mm;
				margin:2mm 0 4mm 0;
			}
			.card { flex:1 1 46mm; min-width:40mm; max-width:66mm; background-color:#eef6f1; border:1px solid #b7d1bf; padding:2mm; }
			.first { order:-1; }
			.last { order:5; }
			.center-card { align-self:center; min-height:26mm; }
			.fixed { flex:0 0 76mm; flex-shrink:0; }
			.stack { display:flex; flex-direction:column; justify-content:center; align-items:center; gap:1mm; }
			.pill { background-color:#ffffff; border:1px solid #b7d1bf; padding:1mm; max-width:34mm; }
			table { width:100%; border-collapse:collapse; margin:1.5mm 0 0 0; }
			td { border:1px solid #c8d2df; padding:1mm; font-size:8pt; }
			.page-flex {
				display:flex;
				flex-direction:column;
				justify-content:space-evenly;
				align-items:center;
				gap:2mm;
				height:52mm;
				page-break-before:always;
				border:1px solid #9fb0bf;
				padding:3mm;
			}
			.page-flex .box { width:70mm; }
			.align-end { align-self:flex-end; }
		</style>

		<h1>HTML Flex Edge Cases</h1>
		<p>This example shows the bounded flexbox subset used for PDF reports: row reverse order, inline flex, direct text flex items, wrapping, order, shrink constraints, nested flex, nested tables, align-self, and page-break handling.</p>

		<h2>Empty and Row Reverse</h2>
		<div class="empty"></div>
		<div class="reverse">
			<div class="box">Source A<br><span class="small">renders after C because row-reverse is active</span></div>
			<div class="box">Source B<br><span class="small">middle item</span></div>
			<div class="box">Source C<br><span class="small">first visual item</span></div>
		</div>

		<h2>Inline Flex and Direct Text</h2>
		<div class="inline">
			Direct text item
			<span class="box">Inline span flex item</span>
			<span class="box">Wrapped span item with <span class="small">nested span styling</span></span>
		</div>

		<h2>Wrapping, Ordering, Nested Content</h2>
		<div class="board">
			<div class="card last">Last ordered card<br><span class="small">source appears first</span></div>
			<div class="card first">First ordered card
				<div class="stack"><span class="pill">Nested flex A</span><span class="pill">Nested flex B</span></div>
			</div>
			<div class="card center-card">Centered card with a table
				<table><tr><td>Code</td><td>Meaning</td></tr><tr><td>OK</td><td>Nested table inside flex</td></tr></table>
			</div>
			<div class="card fixed">Fixed width card<br><span class="small">flex-shrink:0</span></div>
		</div>

		<div class="page-flex">
			<div class="box">Page-break flex item</div>
			<div class="box align-end">Aligned end item</div>
			<div class="box">Centered column item</div>
		</div>
	`
}
