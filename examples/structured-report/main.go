// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package main

import (
	"time"

	"github.com/cssbruno/gopdfkit"
	"github.com/cssbruno/gopdfkit/document"
	"github.com/cssbruno/gopdfkit/examples/internal/outpath"
)

func main() {
	report := document.NewReportDocument(
		"Operations Report",
		document.MetadataGridBlock{Fields: []document.MetadataField{
			{Label: "Report ID", Value: "OPS-2026-001"},
			{Label: "Status", Value: "Ready"},
			{Label: "Owner", Value: "Platform Team"},
		}},
		document.ParagraphBlock{Segments: []document.TextSegment{{
			Text: "This report is assembled from structured document blocks rather than direct drawing calls.",
		}}},
		document.TableBlock{
			Caption: "Service Metrics",
			Header: []document.TableRow{{Cells: []document.TableCell{
				{Blocks: []document.Block{document.ParagraphBlock{Segments: []document.TextSegment{{Text: "Metric"}}}}},
				{Blocks: []document.Block{document.ParagraphBlock{Segments: []document.TextSegment{{Text: "Value"}}}}},
			}}},
			Body: []document.TableRow{
				{Cells: []document.TableCell{
					{Blocks: []document.Block{document.ParagraphBlock{Segments: []document.TextSegment{{Text: "Uptime"}}}}},
					{Blocks: []document.Block{document.ParagraphBlock{Segments: []document.TextSegment{{Text: "99.95%"}}}}},
				}},
				{Cells: []document.TableCell{
					{Blocks: []document.Block{document.ParagraphBlock{Segments: []document.TextSegment{{Text: "Requests"}}}}},
					{Blocks: []document.Block{document.ParagraphBlock{Segments: []document.TextSegment{{Text: "1.2M"}}}}},
				}},
			},
		},
	)
	report.Metadata.CreatedAt = time.Now().UTC()
	report.Footer = &document.FooterBlock{ShowPageNumber: true, ReservePageArea: true}

	pdf := gopdfkit.New()
	pdf.WriteDocument(report)

	if err := pdf.OutputFileAndClose(outpath.File("structured-report.pdf")); err != nil {
		panic(err)
	}
}
