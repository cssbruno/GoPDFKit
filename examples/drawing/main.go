// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package main

import (
	"log"
	"math"

	"github.com/cssbruno/paperrune/document"
	"github.com/cssbruno/paperrune/examples/internal/outpath"
)

func main() {
	pdf := document.MustNew()
	pdf.AddPage()
	pdf.SetFont("Helvetica", "B", 16)
	pdf.Cell(40, 10, "Drawing primitives")

	pdf.SetDrawColor(25, 90, 170)
	pdf.SetFillColor(228, 238, 248)
	pdf.RoundedRect(20, 30, 55, 28, 4, "1234", "DF")
	pdf.Text(26, 47, "RoundedRect")

	pdf.SetDrawColor(170, 60, 35)
	pdf.Circle(115, 44, 15, "D")
	pdf.Line(100, 44, 130, 44)
	pdf.Line(115, 29, 115, 59)

	pdf.SetDrawColor(30, 120, 75)
	pdf.SetFillColor(220, 242, 229)
	pdf.Polygon(starPoints(60, 95, 20, 9), "DF")

	pdf.LinearGradient(100, 80, 70, 30, 30, 80, 170, 235, 245, 255, 0, 0, 1, 0)
	pdf.Rect(100, 80, 70, 30, "D")
	pdf.Text(108, 98, "LinearGradient")

	if err := pdf.OutputFileAndClose(outpath.File("drawing.pdf")); err != nil {
		log.Fatal(err)
	}
}

func starPoints(cx, cy, radius float64, count int) []document.Point {
	points := make([]document.Point, 0, count)
	for i := range count {
		angle := -math.Pi/2 + float64(i)*2*math.Pi/float64(count)
		r := radius
		if i%2 == 1 {
			r *= 0.48
		}
		points = append(points, document.Point{
			X: cx + math.Cos(angle)*r,
			Y: cy + math.Sin(angle)*r,
		})
	}
	return points
}
