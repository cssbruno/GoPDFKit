// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package main

import (
	"fmt"
	stdhtml "html"
	"log"
	"os"
	"strings"

	"github.com/cssbruno/gopdfkit/document"
	"github.com/cssbruno/gopdfkit/examples/internal/assets"
	"github.com/cssbruno/gopdfkit/examples/internal/outpath"
)

type reportData struct {
	PatientName string
	Doctor      string
	Plan        string
	Destination string
	Age         string
	RequestDate string
	RequestTime string
	IssueDate   string
	Protocol    string
	Erythrogram []erythrogramRow
	Leukogram   []leukogramRow
	Platelets   plateletRow
}

type erythrogramRow struct {
	Name   string
	Result string
	Unit   string
	Men    string
	Women  string
}

type leukogramRow struct {
	Name     string
	Percent  string
	Absolute string
	Unit     string
	Ref      string
}

type plateletRow struct {
	Result string
	Ref    string
	Note   string
}

func main() {
	pdf := document.MustNew()
	pdf.SetTitle("Hemograma HTML Modelo Brasileiro", false)
	pdf.SetCreator("examples/lab-hemograma-html", false)
	addUTF8Fonts(pdf)
	pdf.SetMargins(6, 7, 6)
	pdf.SetAutoPageBreak(true, 5)
	pdf.AddPage()
	pdf.SetFont("dejavu", "", 6.8)

	data := sampleData()
	drawLabHeader(pdf, data)
	pdf.SetY(67.4)

	fragment, err := renderHemogramaHTML(data)
	if err != nil {
		log.Fatal(err)
	}
	html := pdf.HTMLNew()
	if messages := html.ValidateHTML(fragment); len(messages) > 0 {
		log.Fatalf("unsupported HTML/CSS in example: %s", strings.Join(messages, "; "))
	}
	compiled, err := document.CompileHTML(fragment)
	if err != nil {
		log.Fatal(err)
	}
	html.WriteCompiled(3.05, compiled)
	drawFixedFooter(pdf)

	if err := pdf.OutputFileAndClose(outpath.File("lab-hemograma-html.pdf")); err != nil {
		log.Fatal(err)
	}
}

func addUTF8Fonts(pdf *document.Document) {
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

func renderHemogramaHTML(data reportData) (string, error) {
	return document.RenderHTMLTemplate(hemogramaTemplate(), document.HTMLTemplateValues{
		"erythrogram_rows": document.HTMLTemplateRaw(erythrogramRowsHTML(data.Erythrogram)),
		"leukogram_rows":   document.HTMLTemplateRaw(leukogramRowsHTML(data.Leukogram)),
		"platelet_result":  data.Platelets.Result,
		"platelet_ref":     data.Platelets.Ref,
		"platelet_note":    data.Platelets.Note,
	})
}

func sampleData() reportData {
	return reportData{
		PatientName: "ANA CLARA ALMEIDA",
		Doctor:      "Dra. Fernanda Matos",
		Plan:        "PARTICULAR",
		Destination: "LABORATÓRIO",
		Age:         "38 Ano(s)",
		RequestDate: "30/06/2026",
		RequestTime: "08:12",
		IssueDate:   "30/06/2026",
		Protocol:    "238759",
		Erythrogram: []erythrogramRow{
			{"Hemácias", "5,78", "milhões/mm³", "4,50 - 5,50 milhões/mm³", "3,80 - 4,80 milhões/mm³"},
			{"Hemoglobina", "15,0", "g/dL", "13,0 - 17,0 g/dL", "12,0 - 15,0 g/dL"},
			{"Hematócrito", "46,7", "%", "40,0 - 50,0 %", "36,0 - 46,0 %"},
			{"R.D.W.", "11,6", "%", "11,6 - 14,0 %", "11,6 - 14,0 %"},
			{"V.C.M.", "80,8", "fl", "83,0 - 101,0 fl", "83,0 - 101,0 fl"},
			{"H.C.M.", "26,0", "pg", "27,0 - 32,0 pg", "27,0 - 32,0 pg"},
			{"C.H.C.M.", "32,1", "g/dL", "31,5 - 34,5 g/dL", "31,5 - 34,5 g/dL"},
		},
		Leukogram: []leukogramRow{
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
		},
		Platelets: plateletRow{
			Result: "116.000/mm³",
			Ref:    "150.000 - 450.000/mm³",
			Note:   "- Confirmado por microscopia.",
		},
	}
}

func erythrogramRowsHTML(rows []erythrogramRow) string {
	var b strings.Builder
	for _, row := range rows {
		fmt.Fprintf(&b,
			`<tr><td width="25%%" class="exam-name">%s</td><td width="3%%" class="colon">:</td><td width="16%%" class="result">%s</td><td width="14%%" class="unit">%s</td><td width="21%%" class="ref">%s</td><td width="21%%" class="ref">%s</td></tr>`,
			esc(row.Name), esc(row.Result), esc(row.Unit), esc(row.Men), esc(row.Women),
		)
	}
	return b.String()
}

func leukogramRowsHTML(rows []leukogramRow) string {
	var b strings.Builder
	for _, row := range rows {
		percent := row.Percent
		if percent != "" {
			percent += " %"
		}
		fmt.Fprintf(&b,
			`<tr><td width="25%%" class="exam-name">%s</td><td width="3%%" class="colon">:</td><td width="14%%" class="result">%s</td><td width="16%%" class="result">%s</td><td width="10%%" class="unit">%s</td><td width="32%%" class="ref center">%s</td></tr>`,
			esc(row.Name), esc(percent), esc(row.Absolute), esc(row.Unit), esc(row.Ref),
		)
	}
	return b.String()
}

func esc(s string) string {
	return stdhtml.EscapeString(s)
}

func drawLabHeader(pdf *document.Document, data reportData) {
	blue := color{0, 166, 216}
	set(pdf, "B", 18, blue)
	pdf.Text(7, 16, "VOLPI")
	pdf.Text(7, 25, "BIANCARDI")
	set(pdf, "", 4.3, blue)
	pdf.Text(8, 31, "LABORATÓRIO DE ANÁLISES CLÍNICAS")

	pdf.SetFillColor(0, 166, 216)
	pdf.Polygon([]document.Point{
		{X: 48.2, Y: 20.4},
		{X: 50.2, Y: 10.4},
		{X: 54.4, Y: 15.1},
		{X: 58.5, Y: 10.4},
		{X: 61.0, Y: 20.4},
		{X: 59.4, Y: 24.0},
		{X: 55.0, Y: 26.0},
		{X: 50.6, Y: 24.0},
	}, "F")

	pdf.SetDrawColor(0, 135, 170)
	pdf.SetLineWidth(0.2)
	pdf.Line(72, 25.4, 198, 25.4)
	set(pdf, "B", 6.8, blue)
	rightText(pdf, 151, 11, 47, 3.5, "Responsáveis Técnicos:")
	set(pdf, "", 6.5, blue)
	rightText(pdf, 151, 16, 47, 3.5, "Dra. Camila Ribeiro")
	rightText(pdf, 151, 20.5, 47, 3.5, "CRBM 00000")
	pdf.SetDrawColor(0, 135, 170)
	pdf.Line(151, 25.4, 198, 25.4)
	set(pdf, "B", 6.5, blue)
	rightText(pdf, 151, 31.5, 47, 3.5, "E-mail: laudo@exemplo.test")
	rightText(pdf, 151, 36, 47, 3.5, "www.laboratorioexemplo.test")

	pdf.SetDrawColor(30, 30, 30)
	pdf.SetLineWidth(0.22)
	pdf.Line(4, 43, 206, 43)
	pdf.Line(4, 62.5, 206, 62.5)

	meta(pdf, 5, 47, "Paciente....:", data.PatientName)
	meta(pdf, 5, 51.5, "Médico......:", data.Doctor)
	meta(pdf, 5, 56, "Convênio....:", data.Plan)
	meta(pdf, 74, 56, "Destino:", data.Destination)
	meta(pdf, 124, 47, "Idade.............:", data.Age)
	meta(pdf, 124, 51.5, "Data Req..........:", data.RequestDate)
	meta(pdf, 124, 56, "Hora Req..........:", data.RequestTime)
	meta(pdf, 124, 60.5, "Data Emissão...:", data.IssueDate)
	drawBarcode(pdf, 181, 44.4, 18, 9.5, data.Protocol)

	set(pdf, "", 6.8, black)
}

func drawFixedFooter(pdf *document.Document) {
	pdf.SetPage(1)
	pdf.SetTextColor(0, 0, 0)
	pdf.SetFont("dejavu", "", 5.2)
	pdf.Text(14, 274.8, "* O valor preditivo dos testes laboratoriais depende da análise dos seus resultados e dos dados clínicos-epidemiológicos do paciente.")
	pdf.Text(14, 279, "Unidade Matriz 4136-42-40 | Rua Exemplo, 158 - Centro | Unidade Lapa 4136-2534 | Rua Modelo, 229")

	pdf.SetDrawColor(0, 0, 0)
	pdf.SetLineWidth(0.22)
	pdf.Circle(8.5, 281.5, 4, "D")
	pdf.Circle(198, 281.5, 4, "D")
	pdf.SetFont("dejavu", "B", 5.2)
	pdf.SetXY(4, 281)
	pdf.CellFormat(9, 3, "PNCQ", "", 0, "C", false, 0, "")
	pdf.SetXY(193.5, 281)
	pdf.CellFormat(9, 3, "ISO", "", 0, "C", false, 0, "")

	pdf.Curve(158, 238, 163, 227, 168, 238, "D")
	pdf.Curve(165, 238, 171, 230, 177, 238, "D")
	pdf.Line(154, 242, 190, 242)
	pdf.SetFont("dejavu", "B", 6)
	pdf.SetXY(154, 244)
	pdf.CellFormat(36, 3.5, "Dra. Camila Ribeiro", "", 1, "C", false, 0, "")
	pdf.SetFont("dejavu", "", 5.6)
	pdf.SetX(154)
	pdf.CellFormat(36, 3.5, "Biomédica", "", 1, "C", false, 0, "")
	pdf.SetX(154)
	pdf.CellFormat(36, 3.5, "CRBM 00000", "", 1, "C", false, 0, "")

	pdf.SetTextColor(155, 18, 18)
	pdf.SetFont("dejavu", "B", 5.4)
	pdf.SetXY(62, 286)
	pdf.CellFormat(86, 3.5, "DOCUMENTO DE EXEMPLO - NÃO UTILIZAR COMO LAUDO REAL", "", 0, "C", false, 0, "")
	pdf.SetTextColor(0, 0, 0)
}

func drawBarcode(pdf *document.Document, x, y, w, h float64, label string) {
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

func meta(pdf *document.Document, x, y float64, label, text string) {
	set(pdf, "", 5.6, black)
	pdf.Text(x, y, label)
	set(pdf, "B", 5.6, black)
	pdf.Text(x+18, y, text)
}

func rightText(pdf *document.Document, x, y, w, h float64, text string) {
	pdf.SetXY(x, y)
	pdf.CellFormat(w, h, text, "", 0, "R", false, 0, "")
}

func centerText(pdf *document.Document, x, y, w, h float64, text string) {
	pdf.SetXY(x, y)
	pdf.CellFormat(w, h, text, "", 0, "C", false, 0, "")
}

type color struct {
	r int
	g int
	b int
}

var black = color{0, 0, 0}

func set(pdf *document.Document, style string, size float64, c color) {
	pdf.SetFont("dejavu", style, size)
	pdf.SetTextColor(c.r, c.g, c.b)
}

func hemogramaTemplate() string {
	return `
		<style>
			p { margin:0; line-height:1.1; color:#000000; }
			.exam-head { display:flex; flex-direction:row; align-items:flex-end; gap:4mm; margin:0 0 1.4mm 0; }
			.exam-title { flex:0 0 43mm; color:#000000; font-size:8.9pt; font-weight:bold; }
			.meta-material { flex:0 0 32mm; color:#000000; font-size:5.85pt; }
			.meta-method { flex:0 0 43mm; color:#000000; font-size:5.85pt; }
			.exam-source { flex:1; text-align:right; font-size:5.55pt; font-style:italic; }
			.label { font-weight:bold; }
			table { width:100%; border-collapse:collapse; }
			td { border:0; padding:0.18mm 0.6mm; color:#000000; vertical-align:top; font-size:6.15pt; }
			.section-cell { font-size:7.4pt; font-weight:bold; font-style:italic; padding:0.75mm 0.6mm 0.45mm 0.6mm; }
			.ref-head { font-size:5.65pt; font-weight:bold; font-style:italic; text-align:center; padding:0.05mm 0.6mm 0.45mm 0.6mm; }
			.exam-name { font-size:6.45pt; }
			.colon { text-align:center; }
			.result { text-align:right; font-size:6.9pt; font-weight:bold; font-style:italic; }
			.unit { font-size:6.45pt; font-weight:bold; font-style:italic; }
			.ref { font-size:5.55pt; text-align:center; }
			.center { text-align:center; }
			.obs-cell { font-size:5.75pt; padding:0.65mm 0.6mm 1.1mm 0.6mm; }
			.platelet-title { font-size:7.4pt; font-weight:bold; font-style:italic; padding-top:0.8mm; }
			.platelet-value { font-size:7.4pt; font-weight:bold; font-style:italic; text-align:right; padding-top:0.8mm; }
			.platelet-ref { font-size:5.7pt; text-align:center; padding-top:1mm; }
		</style>

		<div class="exam-head">
			<div class="exam-title">Hemograma</div>
			<div class="meta-material"><span class="label">Material:</span> Sangue</div>
			<div class="meta-method"><span class="label">Método:</span> Cell-Dyn 3700.</div>
			<div class="exam-source">Fonte: Dacie and Lewis - Practical Haematology 2017.</div>
		</div>

		<table>
			<tr><td colspan="6" class="section-cell">Eritrograma</td></tr>
			<tr><td width="25%"></td><td width="3%"></td><td width="16%"></td><td width="14%"></td><td width="21%" class="ref-head">Homens</td><td width="21%" class="ref-head">Mulheres</td></tr>
			{{erythrogram_rows}}
			<tr><td colspan="6" class="obs-cell">Observações: - Discreta Anisocitose: Microcitose (Discreta)</td></tr>
			<tr><td colspan="6" class="section-cell">Leucograma</td></tr>
			<tr><td width="25%"></td><td width="3%"></td><td width="14%"></td><td width="16%"></td><td width="10%"></td><td width="32%" class="ref-head">Valores de Referência</td></tr>
			{{leukogram_rows}}
			<tr><td colspan="6" class="obs-cell">Observações: Normal</td></tr>
			<tr>
				<td width="25%" class="platelet-title">Plaquetas</td>
				<td width="3%" class="platelet-title">:</td>
				<td width="16%" class="platelet-value">{{platelet_result}}</td>
				<td width="14%"></td>
				<td width="42%" colspan="2" class="platelet-ref">{{platelet_ref}}</td>
			</tr>
			<tr><td colspan="6" class="obs-cell">Observações: {{platelet_note}}</td></tr>
		</table>
	`
}
