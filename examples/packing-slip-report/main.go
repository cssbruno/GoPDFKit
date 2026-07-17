// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package main

import (
	"log"
	"strings"

	"github.com/cssbruno/gopdfkit/document"
	"github.com/cssbruno/gopdfkit/examples/internal/outpath"
)

func main() {
	pdf := document.MustNew()
	pdf.SetTitle("Packing Slip", false)
	pdf.SetCreator("examples/packing-slip-report", false)
	pdf.SetMargins(14, 16, 14)
	pdf.SetAutoPageBreak(true, 8)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 10)

	fragment := packingSlipHTML()
	html := pdf.HTMLNew()
	if messages := html.ValidateHTML(fragment); len(messages) > 0 {
		log.Fatalf("unsupported HTML/CSS in example: %s", strings.Join(messages, "; "))
	}
	compiled, err := document.CompileHTML(fragment)
	if err != nil {
		log.Fatal(err)
	}
	html.WriteCompiled(5.2, compiled)

	if err := pdf.OutputFileAndClose(outpath.File("packing-slip-report.pdf")); err != nil {
		log.Fatal(err)
	}
}

func packingSlipHTML() string {
	return `
		<style>
			h1 { color:#1f3652; font-size:20pt; margin:0 0 3mm 0; }
			h2 { color:#1f3652; font-size:12pt; margin:5mm 0 2mm 0; }
			p { color:#3f4b59; line-height:1.3; margin:0 0 2mm 0; }
			.small { color:#687384; font-size:8pt; }
			.header { display:flex; gap:4mm; align-items:stretch; margin:0 0 4mm 0; }
			.title { flex:1; }
			.shipment { flex:0 0 62mm; background-color:#f4f8fb; border:1px solid #b8c7d8; border-radius:2mm; padding:3mm; }
			.addresses { display:flex; gap:3mm; align-items:stretch; margin:2mm 0 4mm 0; }
			.address-card { flex:1; background-color:#fbfcfd; border:1px solid #c8d2df; border-radius:2mm; padding:3mm; }
			.metrics { display:flex; gap:3mm; flex-wrap:wrap; margin:2mm 0 4mm 0; }
			.metric { flex:1 1 36mm; background-color:#eef6f1; border:1px solid #b7d1bf; border-radius:2mm; padding:2.5mm; }
			.metric-value { color:#166944; font-size:15pt; font-weight:bold; }
			table { width:100%; border-collapse:collapse; margin:2mm 0 4mm 0; }
			th { background-color:#e8eef5; color:#1f3652; border:1px solid #aebcce; font-weight:bold; padding:2mm; text-align:left; }
			td { border:1px solid #c8d2df; color:#394654; padding:2mm; vertical-align:top; }
			td.center { text-align:center; }
			.qa td { height:9mm; }
			.note { background-color:#fff8e7; border-left:3px solid #d69222; border-top:1px solid #ead8ad; border-right:1px solid #ead8ad; border-bottom:1px solid #ead8ad; padding:3mm; margin:3mm 0; }
		</style>

		<div class="header">
			<div class="title">
				<h1>Packing Slip</h1>
				<p>Outbound fulfillment packet for customer shipment.<br><span class="small">Generated June 30, 2026 - Warehouse BOS-03</span></p>
			</div>
			<div class="shipment">
				<strong>Shipment SHP-88241</strong><br>
				<span class="small">Order: SO-73418<br>Carrier: Ground Parcel<br>Service: 2-day commercial</span>
			</div>
		</div>

		<div class="addresses">
			<div class="address-card"><strong>Ship from</strong><br>Acme Fulfillment BOS-03<br>18 Dock Road<br>Worcester, MA 01608<br><span class="small">Dock 4 pickup</span></div>
			<div class="address-card"><strong>Ship to</strong><br>Northstar Retail - Receiving<br>41 Harbor Avenue<br>Boston, MA 02110<br><span class="small">Attention: Dana Miller</span></div>
		</div>

		<div class="metrics">
			<div class="metric"><span class="metric-value">8</span><br><strong>Cartons</strong><br><span class="small">All standard size</span></div>
			<div class="metric"><span class="metric-value">126 lb</span><br><strong>Total weight</strong><br><span class="small">Verified at dock scale</span></div>
			<div class="metric"><span class="metric-value">14</span><br><strong>Line items</strong><br><span class="small">No substitutions</span></div>
			<div class="metric"><span class="metric-value">2</span><br><strong>Hazmat labels</strong><br><span class="small">Battery handling</span></div>
		</div>

		<h2>Package Contents</h2>
		<table>
			<thead><tr><th width="14%">Carton</th><th width="18%">SKU</th><th width="40%">Description</th><th width="14%" class="center">Ordered</th><th width="14%" class="center">Packed</th></tr></thead>
			<tbody>
				<tr><td>1</td><td>SCN-204</td><td>Handheld scanner kit with charging base</td><td class="center">4</td><td class="center">4</td></tr>
				<tr><td>2</td><td>LBL-881</td><td>Thermal label rolls, 4x6 commercial pack</td><td class="center">12</td><td class="center">12</td></tr>
				<tr><td>3</td><td>BAT-110</td><td>Replacement battery pack, lithium ion</td><td class="center">8</td><td class="center">8</td></tr>
				<tr><td>4</td><td>CBL-451</td><td>USB-C rugged cable, 2 meter</td><td class="center">18</td><td class="center">18</td></tr>
				<tr><td>5-8</td><td>KIT-720</td><td>Warehouse workstation accessory kit</td><td class="center">4</td><td class="center">4</td></tr>
			</tbody>
		</table>

		<p class="note"><strong>Receiving note: </strong>Inspect battery cartons before opening. If tamper tape is broken, photograph the carton and notify support before signing.</p>

		<h2>Quality Checklist</h2>
		<table class="qa">
			<thead><tr><th width="42%">Check</th><th width="18%">Result</th><th width="40%">Initials / Notes</th></tr></thead>
			<tbody>
				<tr><td>Item count matches sales order</td><td>Pass</td><td>JD - Count verified at pack station 4</td></tr>
				<tr><td>Battery labels applied to required cartons</td><td>Pass</td><td>JD - Cartons 3 and 4</td></tr>
				<tr><td>Address and carrier service verified</td><td>Pass</td><td>MS - Manifest synced</td></tr>
			</tbody>
		</table>
	`
}
