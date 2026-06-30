// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package main

import (
	"log"
	"os"

	"github.com/cssbruno/gopdfkit"
	"github.com/cssbruno/gopdfkit/document"
	"github.com/cssbruno/gopdfkit/examples/internal/assets"
	"github.com/cssbruno/gopdfkit/examples/internal/outpath"
)

type examRow struct {
	name   string
	result string
	unit   string
	men    string
	women  string
}

type diffRow struct {
	name     string
	percent  string
	absolute string
	unit     string
	ref      string
}

func main() {
	pdf := gopdfkit.New()
	pdf.SetTitle("Hemograma Modelo Brasileiro", false)
	pdf.SetCreator("examples/lab-hemograma-report", false)
	addUTF8Fonts(pdf)
	pdf.SetMargins(0, 0, 0)
	pdf.SetAutoPageBreak(false, 0)
	pdf.AddPage()

	drawHemograma(pdf)

	if err := pdf.OutputFileAndClose(outpath.File("lab-hemograma-report.pdf")); err != nil {
		log.Fatal(err)
	}
}

func addUTF8Fonts(pdf *gopdfkit.Document) {
	fonts := map[string]string{
		"":   "DejaVuSansCondensed.ttf",
		"B":  "DejaVuSansCondensed-Bold.ttf",
		"I":  "DejaVuSansCondensed-Oblique.ttf",
		"BI": "DejaVuSansCondensed-BoldOblique.ttf",
	}
	for style, name := range fonts {
		data, err := os.ReadFile(assets.File("font", name))
		if err != nil {
			log.Fatal(err)
		}
		pdf.AddUTF8FontFromBytes("dejavu", style, data)
	}
}

func drawHemograma(pdf *gopdfkit.Document) {
	drawHeader(pdf)
	drawPatientBand(pdf)
	drawExamBody(pdf)
	drawFooter(pdf)
}

func drawHeader(pdf *gopdfkit.Document) {
	blue := color{0, 166, 216}
	set(pdf, "B", 18, blue)
	pdf.Text(7, 16, "LAB")
	set(pdf, "B", 18, blue)
	pdf.Text(7, 25, "MODELO")
	set(pdf, "", 4.3, blue)
	pdf.Text(8, 31, "LABORATÓRIO DE ANÁLISES CLÍNICAS")

	pdf.SetFillColor(0, 166, 216)
	pdf.Polygon([]document.Point{
		{X: 42, Y: 12}, {X: 48, Y: 8}, {X: 55, Y: 15}, {X: 51, Y: 21}, {X: 43, Y: 20},
	}, "F")
	pdf.Circle(51, 20, 4.6, "F")

	pdf.SetDrawColor(0, 135, 170)
	pdf.SetLineWidth(0.2)
	pdf.Line(72, 25.4, 198, 25.4)

	set(pdf, "B", 6.8, blue)
	rightText(pdf, 151, 11, 47, 3.5, "Responsável Técnico:")
	set(pdf, "", 6.5, blue)
	rightText(pdf, 151, 16, 47, 3.5, "Dra. Camila Ribeiro")
	rightText(pdf, 151, 20.5, 47, 3.5, "CRBM 00000")
	pdf.SetDrawColor(0, 135, 170)
	pdf.Line(151, 25.4, 198, 25.4)
	set(pdf, "B", 6.5, blue)
	rightText(pdf, 151, 31.5, 47, 3.5, "E-mail: laudo@exemplo.test")
	rightText(pdf, 151, 36, 47, 3.5, "www.laboratorioexemplo.test")
}

func drawPatientBand(pdf *gopdfkit.Document) {
	pdf.SetDrawColor(30, 30, 30)
	pdf.SetLineWidth(0.22)
	pdf.Line(4, 43, 206, 43)
	pdf.Line(4, 62.5, 206, 62.5)

	meta(pdf, 5, 47, "Paciente....:", "ANA CLARA ALMEIDA")
	meta(pdf, 5, 51.5, "Médico......:", "Dra. Fernanda Matos")
	meta(pdf, 5, 56, "Convênio....:", "PARTICULAR")
	meta(pdf, 74, 56, "Destino:", "LABORATÓRIO")

	meta(pdf, 124, 47, "Idade.............:", "38 Ano(s)")
	meta(pdf, 124, 51.5, "Data Req..........:", "30/06/2026")
	meta(pdf, 124, 56, "Hora Req..........:", "08:12")
	meta(pdf, 124, 60.5, "Data Emissão...:", "30/06/2026")
	drawBarcode(pdf, 181, 44.4, 18, 9.5, "238759")

	set(pdf, "B", 5.7, color{150, 25, 25})
	centerText(pdf, 4, 66.3, 202, 3.5, "MODELO FICTÍCIO - SEM VALIDADE CLÍNICA OU DIAGNÓSTICA")
}

func drawExamBody(pdf *gopdfkit.Document) {
	y := 72.0
	set(pdf, "B", 9, black)
	pdf.Text(7, y, "Hemograma")
	set(pdf, "", 6.3, black)
	pdf.Text(7, y+4.8, "Material: Sangue")
	pdf.Text(58, y+4.8, "Método: Cell-Dyn 3700.")
	set(pdf, "I", 5.8, black)
	pdf.Text(123, y+4.8, "Fonte: Dacie and Lewis - Practical Haematology 2017.")

	y = drawEritrograma(pdf, y+13.5)
	y = drawLeucograma(pdf, y+4.5)
	drawPlaquetas(pdf, y+4)
}

func drawEritrograma(pdf *gopdfkit.Document, y float64) float64 {
	set(pdf, "BI", 8, black)
	pdf.Text(7, y, "Eritrograma")
	set(pdf, "BI", 5.7, black)
	centerText(pdf, 119, y+3.7, 20, 3, "Homens")
	centerText(pdf, 160, y+3.7, 20, 3, "Mulheres")

	rows := []examRow{
		{"Hemácias", "5,78", "milhões/mm³", "4,50 - 5,50 milhões/mm³", "3,80 - 4,80 milhões/mm³"},
		{"Hemoglobina", "15,0", "g/dL", "13,0 - 17,0 g/dL", "12,0 - 15,0 g/dL"},
		{"Hematócrito", "46,7", "%", "40,0 - 50,0 %", "36,0 - 46,0 %"},
		{"R.D.W.", "11,6", "%", "11,6 - 14,0 %", "11,6 - 14,0 %"},
		{"V.C.M.", "80,8", "fl", "83,0 - 101,0 fl", "83,0 - 101,0 fl"},
		{"H.C.M.", "26,0", "pg", "27,0 - 32,0 pg", "27,0 - 32,0 pg"},
		{"C.H.C.M.", "32,1", "g/dL", "31,5 - 34,5 g/dL", "31,5 - 34,5 g/dL"},
	}
	y += 9.5
	for _, row := range rows {
		drawExamRow(pdf, y, row)
		y += 5.4
	}
	set(pdf, "", 6.2, black)
	pdf.Text(7, y, "Observações:")
	set(pdf, "B", 6.2, black)
	pdf.Text(28, y, "- Discreta Anisocitose: Microcitose (Discreta)")
	return y + 4.8
}

func drawLeucograma(pdf *gopdfkit.Document, y float64) float64 {
	set(pdf, "BI", 8, black)
	pdf.Text(7, y, "Leucograma")
	set(pdf, "BI", 6, black)
	centerText(pdf, 111, y, 50, 3, "Valores de Referência")

	rows := []diffRow{
		{"Leucócitos", "", "6.980", "/mm³", "4.000 - 10.000/mm³"},
		{"Blastos", "0,0", "0", "/mm³", "0 /mm³"},
		{"Promielócitos", "0,0", "0", "/mm³", "0 /mm³"},
		{"Mielócitos", "0,0", "0", "/mm³", "0 /mm³"},
		{"Metamielócitos", "0,0", "0", "/mm³", "0 /mm³"},
		{"Bastonetes", "0,0", "0", "/mm³", "0 - 400 /mm³"},
		{"Segmentados", "59,6", "4.160", "/mm³", "1.800 - 7.500 /mm³"},
		{"Eosinófilos", "4,0", "279", "/mm³", "40 - 450 /mm³"},
		{"Basófilos", "1,3", "91", "/mm³", "0 - 100 /mm³"},
		{"Linfócitos Típicos", "28,5", "1.989", "/mm³", "1.200 - 5.200 /mm³"},
		{"Linfócitos Reativos", "0,0", "0", "/mm³", "0 /mm³"},
		{"Monócitos", "6,5", "454", "/mm³", "80 - 800 /mm³"},
	}

	y += 7
	for _, row := range rows {
		drawDiffRow(pdf, y, row)
		y += 5.3
	}
	set(pdf, "", 6.2, black)
	pdf.Text(7, y, "Observações:")
	set(pdf, "B", 6.2, black)
	pdf.Text(28, y, "Normal")
	return y + 5
}

func drawPlaquetas(pdf *gopdfkit.Document, y float64) {
	set(pdf, "BI", 8, black)
	pdf.Text(7, y, "Plaquetas")
	pdf.Text(42, y, ":")
	value(pdf, 55, y-3.2, 38, 4, "116.000/mm³", "BI", 8)
	set(pdf, "", 6.1, black)
	pdf.Text(113, y, "150.000 - 450.000/mm³")
	y += 6
	set(pdf, "", 6.2, black)
	pdf.Text(7, y, "Observações:")
	set(pdf, "B", 6.2, black)
	pdf.Text(28, y, "- Confirmado por microscopia.")
}

func drawFooter(pdf *gopdfkit.Document) {
	set(pdf, "", 5.2, black)
	pdf.Text(12, 272.5, "* O valor preditivo dos testes laboratoriais depende da análise dos seus resultados e dos dados clínicos-epidemiológicos do paciente.")
	pdf.Text(12, 277, "Unidade Matriz 4136-42-40 | Rua Exemplo, 158 - Centro | Unidade Lapa 4136-2534 | Rua Modelo, 229")

	pdf.SetDrawColor(0, 0, 0)
	pdf.SetLineWidth(0.22)
	pdf.Circle(8.5, 281.5, 4, "D")
	set(pdf, "B", 5.5, black)
	centerText(pdf, 3.8, 282.5, 9.4, 2.5, "PNCQ")

	pdf.Curve(158, 260, 163, 249, 168, 260, "D")
	pdf.Curve(165, 260, 171, 252, 177, 260, "D")
	pdf.Line(157, 260.5, 181, 260.5)
	set(pdf, "B", 6.2, black)
	centerText(pdf, 150, 266.5, 40, 3, "Dra. Camila Ribeiro")
	set(pdf, "", 5.7, black)
	centerText(pdf, 150, 270.4, 40, 3, "Biomédica")
	centerText(pdf, 150, 274, 40, 3, "CRBM 00000")

	set(pdf, "B", 5.3, color{145, 25, 25})
	centerText(pdf, 67, 287, 76, 3, "DOCUMENTO DE EXEMPLO - NÃO UTILIZAR COMO LAUDO REAL")
	pdf.Circle(198, 281.5, 4, "D")
	set(pdf, "B", 4.8, black)
	centerText(pdf, 193.5, 282, 9, 2.5, "ISO")
}

func drawExamRow(pdf *gopdfkit.Document, y float64, row examRow) {
	set(pdf, "", 7.2, black)
	pdf.Text(8, y, row.name)
	pdf.Text(42, y, ":")
	value(pdf, 53, y-3.2, 22, 4, row.result, "BI", 7.2)
	set(pdf, "BI", 6.8, black)
	pdf.Text(76, y, row.unit)
	set(pdf, "", 5.7, black)
	pdf.Text(113, y, row.men)
	pdf.Text(154, y, row.women)
}

func drawDiffRow(pdf *gopdfkit.Document, y float64, row diffRow) {
	set(pdf, "", 7.2, black)
	pdf.Text(8, y, row.name)
	pdf.Text(42, y, ":")
	if row.percent != "" {
		value(pdf, 50, y-3.2, 18, 4, row.percent+" %", "BI", 7.2)
	}
	value(pdf, 75, y-3.2, 22, 4, row.absolute, "BI", 7.2)
	set(pdf, "BI", 6.8, black)
	pdf.Text(98, y, row.unit)
	set(pdf, "", 5.7, black)
	centerText(pdf, 112, y-3, 38, 4, row.ref)
}

func drawBarcode(pdf *gopdfkit.Document, x, y, w, h float64, label string) {
	pattern := []float64{0.45, 0.25, 0.8, 0.25, 0.35, 0.6, 0.25, 0.25, 1.0, 0.35, 0.25, 0.7, 0.25, 0.45, 0.25, 0.9, 0.25, 0.35, 0.6, 0.25}
	pdf.SetFillColor(0, 0, 0)
	cursor := x
	for i, bar := range pattern {
		if i%2 == 0 {
			pdf.Rect(cursor, y, bar, h, "F")
		}
		cursor += bar
		if cursor > x+w {
			break
		}
	}
	set(pdf, "", 5.5, black)
	centerText(pdf, x, y+h+3, w, 3, label)
}

func meta(pdf *gopdfkit.Document, x, y float64, label, text string) {
	set(pdf, "", 5.6, black)
	pdf.Text(x, y, label)
	set(pdf, "B", 5.6, black)
	pdf.Text(x+18, y, text)
}

func value(pdf *gopdfkit.Document, x, y, w, h float64, text, style string, size float64) {
	set(pdf, style, size, black)
	pdf.SetXY(x, y)
	pdf.CellFormat(w, h, text, "", 0, "R", false, 0, "")
}

func rightText(pdf *gopdfkit.Document, x, y, w, h float64, text string) {
	pdf.SetXY(x, y)
	pdf.CellFormat(w, h, text, "", 0, "R", false, 0, "")
}

func centerText(pdf *gopdfkit.Document, x, y, w, h float64, text string) {
	pdf.SetXY(x, y)
	pdf.CellFormat(w, h, text, "", 0, "C", false, 0, "")
}

type color struct {
	r int
	g int
	b int
}

var black = color{0, 0, 0}

func set(pdf *gopdfkit.Document, style string, size float64, c color) {
	pdf.SetFont("dejavu", style, size)
	pdf.SetTextColor(c.r, c.g, c.b)
}
