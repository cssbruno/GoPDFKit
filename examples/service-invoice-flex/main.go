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
	pdf.SetTitle("Service Invoice", false)
	pdf.SetCreator("examples/service-invoice-flex", false)
	pdf.SetMargins(14, 16, 14)
	pdf.SetAutoPageBreak(true, 8)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 10)

	fragment := invoiceHTML()
	html := pdf.HTMLNew()
	if messages := html.ValidateHTML(fragment); len(messages) > 0 {
		log.Fatalf("unsupported HTML/CSS in example: %s", strings.Join(messages, "; "))
	}
	compiled, err := document.CompileHTML(fragment)
	if err != nil {
		log.Fatal(err)
	}
	html.WriteCompiled(5.2, compiled)

	if err := pdf.OutputFileAndClose(outpath.File("service-invoice-flex.pdf")); err != nil {
		log.Fatal(err)
	}
}

func invoiceHTML() string {
	return `
		<style>
			h1 { color:#1f3652; font-size:22pt; margin:0 0 3mm 0; }
			h2 { color:#1f3652; font-size:12pt; margin:4mm 0 2mm 0; }
			p { color:#3f4b59; line-height:1.3; margin:0 0 2mm 0; }
			.small { color:#687384; font-size:8pt; }
			.total { color:#166944; font-size:18pt; font-weight:bold; }
			.header { display:flex; gap:4mm; align-items:flex-start; margin:0 0 5mm 0; }
			.company { flex:1; }
			.invoice-box { flex:0 0 58mm; background-color:#f4f8fb; border:1px solid #b8c7d8; border-radius:2mm; padding:3mm; }
			.addresses { display:flex; gap:3mm; align-items:stretch; margin:3mm 0 4mm 0; }
			.address-card { flex:1; background-color:#fbfcfd; border:1px solid #c8d2df; border-radius:2mm; padding:3mm; }
			.summary { display:flex; gap:3mm; margin:2mm 0 4mm 0; }
			.summary-card { flex:1; background-color:#eef6f1; border:1px solid #b7d1bf; border-radius:2mm; padding:3mm; }
			table { width:100%; border-collapse:collapse; margin:2mm 0 4mm 0; }
			th { background-color:#e8eef5; color:#1f3652; border:1px solid #aebcce; font-weight:bold; padding:2mm; text-align:left; }
			td { border:1px solid #c8d2df; color:#394654; padding:2mm; vertical-align:top; }
			td.money { text-align:right; }
			tfoot td { background-color:#f4f7fb; font-weight:bold; }
			.note { background-color:#fff8e7; border-left:3px solid #d69222; border-top:1px solid #ead8ad; border-right:1px solid #ead8ad; border-bottom:1px solid #ead8ad; padding:3mm; margin:3mm 0; }
		</style>

		<div class="header">
			<div class="company">
				<h1>Acme Field Services</h1>
				<p>118 Market Street<br>Denver, CO 80202<br><span class="small">billing@example.test - +1 555 0199</span></p>
			</div>
			<div class="invoice-box">
				<strong>Invoice INV-2026-0630</strong><br>
				<span class="small">Issued: Jun 30, 2026<br>Due: Jul 15, 2026<br>Terms: Net 15</span><br><br>
				<span class="total">$8,472.50</span>
			</div>
		</div>

		<div class="addresses">
			<div class="address-card"><strong>Bill to</strong><br>Northstar Retail Group<br>41 Harbor Avenue<br>Boston, MA 02110<br><span class="small">finance@northstar.example</span></div>
			<div class="address-card"><strong>Service location</strong><br>Northstar Distribution Center<br>900 Industrial Loop<br>Worcester, MA 01608<br><span class="small">Dock 14, east entrance</span></div>
		</div>

		<div class="summary">
			<div class="summary-card"><span class="total">42.5</span><br><strong>Service hours</strong><br><span class="small">Three field visits</span></div>
			<div class="summary-card"><span class="total">6</span><br><strong>Assets serviced</strong><br><span class="small">Conveyors and scanners</span></div>
			<div class="summary-card"><span class="total">100%</span><br><strong>QA pass</strong><br><span class="small">No reopen items</span></div>
		</div>

		<h2>Line Items</h2>
		<table>
			<thead><tr><th width="14%">Date</th><th width="42%">Description</th><th width="12%" class="money">Qty</th><th width="16%" class="money">Rate</th><th width="16%" class="money">Amount</th></tr></thead>
			<tbody>
				<tr><td>Jun 21</td><td>Preventive maintenance and scanner calibration</td><td class="money">12.0</td><td class="money">$145.00</td><td class="money">$1,740.00</td></tr>
				<tr><td>Jun 24</td><td>Emergency conveyor belt alignment and test cycle</td><td class="money">18.5</td><td class="money">$165.00</td><td class="money">$3,052.50</td></tr>
				<tr><td>Jun 28</td><td>Weekend support coverage and spare sensor replacement</td><td class="money">12.0</td><td class="money">$185.00</td><td class="money">$2,220.00</td></tr>
				<tr><td>Jun 28</td><td>Parts: two optical sensors, bracket kit, cable harness</td><td class="money">1.0</td><td class="money">$980.00</td><td class="money">$980.00</td></tr>
			</tbody>
			<tfoot>
				<tr><td colspan="4">Subtotal</td><td class="money">$7,992.50</td></tr>
				<tr><td colspan="4">Sales tax</td><td class="money">$480.00</td></tr>
				<tr><td colspan="4">Amount due</td><td class="money">$8,472.50</td></tr>
			</tfoot>
		</table>

		<p class="note"><strong>Payment note: </strong>Please include invoice INV-2026-0630 with ACH remittance. Late invoices may pause non-emergency field dispatch.</p>
	`
}
