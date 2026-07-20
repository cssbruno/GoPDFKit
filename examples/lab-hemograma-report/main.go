// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

// Command lab-hemograma-report draws a polished, single-page Brazilian
// laboratory report with the low-level Document API. All people, identifiers,
// results, addresses, and credentials in this example are fictional.
package main

import (
	"log"
	"os"

	"github.com/cssbruno/paperrune/document"
	"github.com/cssbruno/paperrune/examples/internal/assets"
	"github.com/cssbruno/paperrune/examples/internal/outpath"
)

type resultRow struct {
	name      string
	result    string
	unit      string
	reference string
	previous  string
	flag      string
}

var (
	navy        = color{18, 48, 71}
	teal        = color{0, 157, 157}
	tealDark    = color{0, 117, 124}
	ink         = color{31, 45, 57}
	muted       = color{96, 117, 130}
	line        = color{214, 224, 228}
	pale        = color{244, 248, 249}
	paleTeal    = color{232, 247, 246}
	alert       = color{186, 55, 64}
	paleAlert   = color{252, 238, 239}
	white       = color{255, 255, 255}
	erythrogram = []resultRow{
		{"Hemácias", "4,08", "milhões/µL", "3,90 - 5,20", "4,21", ""},
		{"Hemoglobina", "11,2", "g/dL", "12,0 - 15,5", "11,7", "BAIXO"},
		{"Hematócrito", "34,7", "%", "36,0 - 46,0", "36,2", "BAIXO"},
		{"VCM", "85,0", "fL", "80,0 - 100,0", "86,0", ""},
		{"HCM", "27,5", "pg", "27,0 - 33,0", "27,8", ""},
		{"CHCM", "32,3", "g/dL", "31,0 - 36,0", "32,4", ""},
		{"RDW", "15,6", "%", "11,5 - 14,5", "15,1", "ALTO"},
	}
	leukogram = []resultRow{
		{"Leucócitos", "8.740", "/µL", "4.000 - 10.000", "7.960", ""},
		{"Neutrófilos  59,8%", "5.227", "/µL", "1.800 - 7.700", "4.744", ""},
		{"Linfócitos  29,0%", "2.535", "/µL", "1.000 - 4.000", "2.388", ""},
		{"Monócitos  6,9%", "603", "/µL", "200 - 1.000", "549", ""},
		{"Eosinófilos  3,6%", "315", "/µL", "20 - 500", "223", ""},
		{"Basófilos  0,7%", "60", "/µL", "0 - 100", "56", ""},
	}
	platelets = []resultRow{
		{"Plaquetas", "428.000", "/µL", "150.000 - 450.000", "391.000", ""},
		{"Volume plaquetário médio", "11,8", "fL", "7,5 - 11,5", "11,2", "ALTO"},
	}
)

func main() {
	pdf := document.MustNew()
	pdf.SetTitle("Hemograma completo - modelo brasileiro contemporâneo", false)
	pdf.SetCreator("examples/lab-hemograma-report", false)
	addUTF8Fonts(pdf)
	pdf.SetMargins(0, 0, 0)
	pdf.SetAutoPageBreak(false, 0)
	pdf.AddPage()

	drawHeader(pdf)
	drawPatientCard(pdf)
	drawSpecimenCard(pdf)
	drawResults(pdf)
	drawSignoff(pdf)
	drawFooter(pdf)

	if err := pdf.OutputFileAndClose(outpath.File("lab-hemograma-report.pdf")); err != nil {
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

func drawHeader(pdf *document.Document) {
	fill(pdf, navy)
	pdf.Rect(0, 0, 210, 5, "F")

	fill(pdf, teal)
	pdf.Circle(22, 22, 11, "F")
	set(pdf, "B", 16, white)
	centerText(pdf, 11, 16.6, 22, 10, "A+")

	set(pdf, "B", 17.5, navy)
	pdf.Text(39, 18.5, "AURORA")
	set(pdf, "", 7.2, tealDark)
	pdf.Text(39.3, 24.3, "MEDICINA DIAGNÓSTICA")
	set(pdf, "", 5.8, muted)
	pdf.Text(39.3, 30.2, "Unidade Fortaleza  |  CNES 0000000 (fictício)")
	pdf.Text(39.3, 34.7, "(85) 3000-0000  |  laudos@aurora-demo.test")

	fill(pdf, paleTeal)
	draw(pdf, color{187, 224, 223})
	pdf.RoundedRect(143, 10, 57, 28, 2.8, "1234", "DF")
	set(pdf, "B", 5.4, tealDark)
	pdf.Text(148, 16, "RESULTADO LABORATORIAL")
	set(pdf, "B", 9.5, navy)
	pdf.Text(148, 23, "HEMOGRAMA COMPLETO")
	set(pdf, "", 5.8, muted)
	pdf.Text(148, 29.5, "Laudo DEMO-2026-0042")
	pdf.Text(148, 34, "Emitido em 18/07/2026 às 10:14")

	fill(pdf, paleAlert)
	pdf.Rect(0, 42, 210, 7, "F")
	set(pdf, "B", 6.1, alert)
	centerText(pdf, 0, 43.5, 210, 4, "MODELO FICTÍCIO  |  DADOS SINTÉTICOS  |  SEM VALIDADE CLÍNICA")
}

func drawPatientCard(pdf *document.Document) {
	card(pdf, 10, 53, 190, 31, white)
	field(pdf, 15, 59, "PACIENTE", "ANA CLARA ALMEIDA", 62)
	field(pdf, 83, 59, "NASCIMENTO / SEXO", "14/03/1988  |  Feminino", 46)
	field(pdf, 142, 59, "ID DO PACIENTE", "DEMO-260718-0042", 53)

	draw(pdf, line)
	pdf.Line(15, 69, 195, 69)
	field(pdf, 15, 74, "SOLICITANTE", "Dra. Helena Modelo  |  CRM-CE 00000", 62)
	field(pdf, 83, 74, "ATENDIMENTO", "Particular", 46)
	field(pdf, 142, 74, "UNIDADE", "Fortaleza - CE", 53)
}

func drawSpecimenCard(pdf *document.Document) {
	card(pdf, 10, 89, 190, 24, pale)
	field(pdf, 15, 95, "COLETA", "18/07/2026  08:05", 42)
	field(pdf, 62, 95, "RECEBIMENTO", "18/07/2026  08:31", 42)
	field(pdf, 109, 95, "LIBERAÇÃO", "18/07/2026  10:14", 42)
	field(pdf, 156, 95, "AMOSTRA", "Sangue total EDTA", 39)

	set(pdf, "B", 5.2, muted)
	pdf.Text(15, 106, "MÉTODO ANALÍTICO")
	set(pdf, "", 6.2, ink)
	pdf.Text(53, 106, "Impedância, citometria de fluxo e espectrofotometria")
	set(pdf, "", 5.4, muted)
	rightText(pdf, 155, 103.8, 40, 3.5, "Amostra íntegra")
}

func drawResults(pdf *document.Document) {
	set(pdf, "B", 13.5, navy)
	pdf.Text(10, 121, "Hemograma completo")
	set(pdf, "", 5.7, muted)
	rightText(pdf, 128, 117.5, 72, 4, "Intervalos demonstrativos para mulher adulta")

	drawTableHeader(pdf, 126)
	y := 133.0
	y = drawSection(pdf, y, "ERITROGRAMA", "Série vermelha")
	for i, row := range erythrogram {
		drawResultRow(pdf, y, row, i%2 == 1)
		y += 5.8
	}
	y += 1.4
	y = drawSection(pdf, y, "LEUCOGRAMA", "Contagem absoluta e diferencial")
	for i, row := range leukogram {
		drawResultRow(pdf, y, row, i%2 == 1)
		y += 5.8
	}
	y += 1.4
	y = drawSection(pdf, y, "PLAQUETAS", "Contagem e volume médio")
	for i, row := range platelets {
		drawResultRow(pdf, y, row, i%2 == 1)
		y += 5.8
	}

	fill(pdf, paleAlert)
	draw(pdf, color{239, 199, 202})
	pdf.RoundedRect(10, y+1.4, 190, 14, 2, "1234", "DF")
	fill(pdf, alert)
	pdf.RoundedRectExt(10, y+1.4, 3.2, 14, 2, 0, 0, 2, "F")
	set(pdf, "B", 5.4, alert)
	pdf.Text(17, y+6.2, "OBSERVAÇÃO TÉCNICA")
	set(pdf, "", 6.1, ink)
	pdf.Text(17, y+11.3, "Discreta anisocitose. Resultados assinalados devem ser interpretados com o contexto clínico.")
	set(pdf, "I", 5.1, muted)
	pdf.Text(17, y+15, "Faixas de referência variam conforme método, população e laboratório executor.")
}

func drawTableHeader(pdf *document.Document, y float64) {
	fill(pdf, navy)
	pdf.RoundedRectExt(10, y, 190, 7, 2, 2, 0, 0, "F")
	set(pdf, "B", 5.3, white)
	pdf.Text(14, y+4.7, "ANÁLISE")
	rightText(pdf, 72, y+1.7, 34, 3, "RESULTADO")
	pdf.Text(110, y+4.7, "UNIDADE")
	centerText(pdf, 133, y+1.7, 33, 3, "REFERÊNCIA")
	centerText(pdf, 169, y+1.7, 18, 3, "ANTERIOR")
	centerText(pdf, 188, y+1.7, 10, 3, "FLAG")
}

func drawSection(pdf *document.Document, y float64, title, subtitle string) float64 {
	fill(pdf, paleTeal)
	pdf.Rect(10, y, 190, 6.4, "F")
	set(pdf, "B", 6.4, tealDark)
	pdf.Text(14, y+4.4, title)
	set(pdf, "", 5.1, muted)
	pdf.Text(45, y+4.4, subtitle)
	return y + 6.4
}

func drawResultRow(pdf *document.Document, y float64, row resultRow, shaded bool) {
	if shaded {
		fill(pdf, pale)
		pdf.Rect(10, y, 190, 5.8, "F")
	}
	draw(pdf, line)
	pdf.Line(10, y+5.8, 200, y+5.8)

	set(pdf, "", 6.25, ink)
	pdf.Text(14, y+3.95, row.name)
	resultColor := ink
	if row.flag != "" {
		resultColor = alert
	}
	set(pdf, "B", 6.6, resultColor)
	rightText(pdf, 72, y+1.2, 34, 3.3, row.result)
	set(pdf, "", 5.7, muted)
	pdf.Text(110, y+3.95, row.unit)
	centerText(pdf, 133, y+1.2, 33, 3.3, row.reference)
	centerText(pdf, 169, y+1.2, 18, 3.3, row.previous)
	if row.flag != "" {
		fill(pdf, paleAlert)
		draw(pdf, color{232, 174, 179})
		pdf.RoundedRect(188.2, y+1.05, 10.5, 3.8, 1.2, "1234", "DF")
		set(pdf, "B", 4.2, alert)
		centerText(pdf, 188.2, y+1.25, 10.5, 2.8, row.flag)
	} else {
		fill(pdf, color{204, 224, 218})
		pdf.Circle(193.5, y+2.9, 1.05, "F")
	}
}

func drawSignoff(pdf *document.Document) {
	set(pdf, "B", 5.2, muted)
	pdf.Text(12, 266, "RESPONSÁVEL TÉCNICA E PROFISSIONAL SIGNATÁRIA")
	set(pdf, "B", 7, navy)
	pdf.Text(12, 272, "Dra. Marina Campos")
	set(pdf, "", 5.8, ink)
	pdf.Text(12, 277, "Biomédica  |  CRBM-6 00000 (fictício)")
	set(pdf, "", 5.1, muted)
	pdf.Text(12, 282, "Assinatura demonstrativa: 84C2-A7F1-0D42-2026")

	set(pdf, "B", 5.2, muted)
	pdf.Text(112, 266, "VALIDAÇÃO DO DOCUMENTO")
	set(pdf, "", 5.6, ink)
	pdf.Text(112, 272, "aurora-demo.test/validar")
	pdf.Text(112, 277, "Código: DEMO-2026-0042")
	set(pdf, "I", 4.9, muted)
	pdf.Text(112, 282, "QR e endereço sem função real")
	drawDemoQR(pdf, 181, 263, 17)
}

func drawFooter(pdf *document.Document) {
	fill(pdf, navy)
	pdf.Rect(0, 287, 210, 10, "F")
	set(pdf, "", 5.15, white)
	pdf.Text(10, 291.5, "AURORA DIAGNÓSTICOS DEMO  |  Rua Exemplo, 120 - Fortaleza/CE  |  CNES 0000000")
	set(pdf, "B", 5.15, white)
	rightText(pdf, 172, 288.8, 28, 3.5, "Página 1 de 1")
	set(pdf, "", 4.8, color{190, 213, 224})
	pdf.Text(10, 295.2, "Layout inspirado em convenções públicas brasileiras; nenhum dado pertence a paciente ou laboratório real.")
}

func drawDemoQR(pdf *document.Document, x, y, size float64) {
	fill(pdf, white)
	draw(pdf, line)
	pdf.Rect(x, y, size, size, "DF")
	module := size / 13
	pattern := []string{
		"1111100111111", "1000100100001", "1010100110101", "1000100100001",
		"1111100111111", "0000000000000", "1011011010110", "0110100101011",
		"1100111011001", "0000000010100", "1111101110111", "1000100101100", "1111101101011",
	}
	fill(pdf, navy)
	for row, bits := range pattern {
		for col, bit := range bits {
			if bit == '1' {
				pdf.Rect(x+float64(col)*module, y+float64(row)*module, module, module, "F")
			}
		}
	}
}

func card(pdf *document.Document, x, y, w, h float64, background color) {
	fill(pdf, background)
	draw(pdf, line)
	pdf.RoundedRect(x, y, w, h, 2.4, "1234", "DF")
}

func field(pdf *document.Document, x, y float64, label, value string, width float64) {
	set(pdf, "B", 5.1, muted)
	pdf.Text(x, y, label)
	set(pdf, "B", 6.4, ink)
	pdf.SetXY(x, y+1.7)
	pdf.CellFormat(width, 5, value, "", 0, "L", false, 0, "")
}

func fill(pdf *document.Document, c color) {
	pdf.SetFillColor(c.r, c.g, c.b)
}

func draw(pdf *document.Document, c color) {
	pdf.SetDrawColor(c.r, c.g, c.b)
	pdf.SetLineWidth(0.2)
}

func set(pdf *document.Document, style string, size float64, c color) {
	pdf.SetFont("dejavu", style, size)
	pdf.SetTextColor(c.r, c.g, c.b)
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
