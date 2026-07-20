// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/cssbruno/paperrune/document"
	"github.com/cssbruno/paperrune/examples/internal/outpath"
)

func main() {
	pdf := document.MustNew()
	pdf.SetTitle("Project Status Report", false)
	pdf.SetCreator("examples/project-status-report", false)
	pdf.SetMargins(14, 24, 14)
	pdf.SetAutoPageBreak(true, 16)
	pdf.AliasNbPages("{total}")
	setHeaderFooter(pdf)

	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 10)

	fragment := statusHTML()
	html := pdf.HTMLNew()
	if messages := html.ValidateHTML(fragment); len(messages) > 0 {
		log.Fatalf("unsupported HTML/CSS in example: %s", strings.Join(messages, "; "))
	}
	compiled, err := document.CompileHTML(fragment)
	if err != nil {
		log.Fatal(err)
	}
	html.WriteCompiled(5.2, compiled)

	if err := pdf.OutputFileAndClose(outpath.File("project-status-report.pdf")); err != nil {
		log.Fatal(err)
	}
}

func setHeaderFooter(pdf *document.Document) {
	pdf.SetHeaderFunc(func() {
		pdf.SetFillColor(31, 54, 82)
		pdf.Rect(0, 0, 210, 18, "F")
		pdf.SetTextColor(255, 255, 255)
		pdf.SetFont("Helvetica", "B", 9)
		pdf.SetXY(14, 6)
		pdf.CellFormat(100, 5, "Project Phoenix - Status Report", "", 0, "L", false, 0, "")
		pdf.SetFont("Helvetica", "", 8)
		pdf.SetXY(122, 6)
		pdf.CellFormat(74, 5, "Week ending June 30, 2026", "", 0, "R", false, 0, "")
		pdf.SetTextColor(0, 0, 0)
		pdf.SetY(24)
	})

	pdf.SetFooterFunc(func() {
		pdf.SetY(-13)
		pdf.SetDrawColor(205, 213, 222)
		pdf.Line(14, 282, 196, 282)
		pdf.SetFont("Helvetica", "", 8)
		pdf.SetTextColor(95, 105, 115)
		pdf.CellFormat(90, 6, "Internal delivery update", "", 0, "L", false, 0, "")
		pdf.CellFormat(0, 6, fmt.Sprintf("Page %d / {total}", pdf.PageNo()), "", 0, "R", false, 0, "")
		pdf.SetTextColor(0, 0, 0)
	})
}

func statusHTML() string {
	return `
		<style>
			h1 { color:#1f3652; font-size:18pt; margin:0 0 3mm 0; }
			h2 { color:#1f3652; font-size:12.5pt; margin:5mm 0 2mm 0; }
			p { color:#3f4b59; line-height:1.32; margin:0 0 2.5mm 0; }
			.small { color:#687384; font-size:8pt; }
			.green { color:#166944; font-weight:bold; }
			.orange { color:#9b5b00; font-weight:bold; }
			.hero { display:flex; gap:3mm; align-items:stretch; margin:1mm 0 4mm 0; }
			.hero-card { flex:1; background-color:#f8fafc; border:1px solid #c8d2df; border-radius:2mm; padding:3mm; }
			.metric { display:flex; gap:3mm; align-items:stretch; margin:2mm 0 4mm 0; }
			.metric-card { flex:1; background-color:#eef6f1; border:1px solid #b7d1bf; border-radius:2mm; padding:3mm; }
			.metric-value { color:#166944; font-size:16pt; font-weight:bold; }
			.callout { background-color:#fff8e7; border-left:3px solid #d69222; border-top:1px solid #ead8ad; border-right:1px solid #ead8ad; border-bottom:1px solid #ead8ad; padding:3mm; margin:3mm 0; }
			table { width:100%; border-collapse:collapse; margin:2mm 0 4mm 0; }
			th { background-color:#e8eef5; color:#1f3652; border:1px solid #aebcce; font-weight:bold; padding:2mm; text-align:left; }
			td { border:1px solid #c8d2df; color:#394654; padding:2mm; vertical-align:top; }
			td.center { text-align:center; }
		</style>

		<h1>Project Phoenix Status</h1>
		<div class="hero">
			<div class="hero-card"><strong>Sponsor</strong><br>Operations Leadership<br><span class="small">Executive review: Jul 08, 2026</span></div>
			<div class="hero-card"><strong>Delivery window</strong><br>May 06 - Aug 19, 2026<br><span class="small">Sprint 5 of 8 in progress</span></div>
			<div class="hero-card"><strong>Overall status</strong><br><span class="green">On track</span><br><span class="small">Scope stable, one dependency at risk</span></div>
		</div>

		<p>Project Phoenix upgrades the customer onboarding workflow, consolidates manual spreadsheet steps, and adds account-level audit reporting for enterprise customers.</p>

		<div class="metric">
			<div class="metric-card"><span class="metric-value">74%</span><br><strong>Milestones complete</strong><br><span class="small">18 of 24 accepted</span></div>
			<div class="metric-card"><span class="metric-value">3</span><br><strong>Open risks</strong><br><span class="small">One high priority</span></div>
			<div class="metric-card"><span class="metric-value">6.2%</span><br><strong>Budget variance</strong><br><span class="small">Within approved range</span></div>
		</div>

		<h2>Work Completed This Week</h2>
		<table>
			<thead><tr><th width="22%">Area</th><th width="42%">Result</th><th width="18%">Owner</th><th width="18%">Status</th></tr></thead>
			<tbody>
				<tr><td>Data import</td><td>Mapped all legacy customer columns into the new canonical schema.</td><td>Data Team</td><td><span class="green">Accepted</span></td></tr>
				<tr><td>Approval flow</td><td>Completed manager approval queue and audit event capture.</td><td>Workflow</td><td><span class="green">Accepted</span></td></tr>
				<tr><td>Reporting</td><td>Delivered draft PDF scorecard and weekly variance table.</td><td>Reporting</td><td>Review</td></tr>
				<tr><td>Training</td><td>Recorded first enablement session for customer operations staff.</td><td>Enablement</td><td><span class="green">Done</span></td></tr>
			</tbody>
		</table>

		<p class="callout"><strong>Decision needed: </strong>Confirm whether the first release includes account-level exports or keeps exports behind the admin role until Sprint 7.</p>

		<div style="page-break-before:always"></div>
		<h1>Risks and Milestones</h1>
		<h2>Risk Register</h2>
		<table>
			<thead><tr><th width="24%">Risk</th><th width="40%">Impact</th><th width="18%">Owner</th><th width="18%">Plan</th></tr></thead>
			<tbody>
				<tr><td>CRM API quota</td><td>Bulk customer imports may throttle during month-end onboarding.</td><td>Platform</td><td><span class="orange">Mitigate</span></td></tr>
				<tr><td>Training attendance</td><td>Two regional teams have not attended enablement.</td><td>Enablement</td><td>Schedule</td></tr>
				<tr><td>Legacy IDs</td><td>Duplicate identifiers require manual approval before import.</td><td>Data Team</td><td>Monitor</td></tr>
			</tbody>
		</table>

		<h2>Upcoming Milestones</h2>
		<table>
			<thead><tr><th width="34%">Milestone</th><th width="22%">Target</th><th width="22%">Owner</th><th width="22%">Exit Criteria</th></tr></thead>
			<tbody>
				<tr><td>UAT packet release</td><td>Jul 03, 2026</td><td>QA</td><td>20 scripted flows pass</td></tr>
				<tr><td>Regional training</td><td>Jul 09, 2026</td><td>Enablement</td><td>All teams attend</td></tr>
				<tr><td>Go-live readiness review</td><td>Jul 17, 2026</td><td>PMO</td><td>No open high risks</td></tr>
			</tbody>
		</table>
	`
}
