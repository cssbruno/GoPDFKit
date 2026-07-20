// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

// Command lab-hemograma-html renders a patient-friendly Brazilian hemogram
// from an HTML template. All people, identifiers, results, addresses, and
// credentials in this example are fictional.
package main

import (
	"fmt"
	stdhtml "html"
	"log"
	"os"
	"strings"

	"github.com/cssbruno/paperrune/document"
	"github.com/cssbruno/paperrune/examples/internal/assets"
	"github.com/cssbruno/paperrune/examples/internal/outpath"
)

type reportData struct {
	PatientName string
	PatientID   string
	BirthSex    string
	Doctor      string
	Collection  string
	Released    string
	Protocol    string
	Erythrogram []resultRow
	Leukogram   []resultRow
	Platelets   []resultRow
}

type resultRow struct {
	Name      string
	Result    string
	Unit      string
	Reference string
	Previous  string
	Flag      string
}

func main() {
	pdf := document.MustNew()
	pdf.SetTitle("Hemograma HTML - modelo brasileiro contemporâneo", false)
	pdf.SetCreator("examples/lab-hemograma-html", false)
	addUTF8Fonts(pdf)
	pdf.SetMargins(10, 10, 10)
	pdf.SetAutoPageBreak(true, 10)
	pdf.AddPage()

	data := sampleData()
	drawLabHeader(pdf, data)
	drawBodyTitle(pdf)
	drawSummaryCards(pdf)
	pdf.SetY(145)
	// The strict unified HTML table cohort uses core-font metrics for intrinsic
	// sizing. The drawn page chrome still uses the embedded UTF-8 family.
	pdf.SetFont("Helvetica", "", 6.4)

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
	drawTechnicalNote(pdf)
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
		"erythrogram_rows": document.HTMLTemplateRaw(resultRowsHTML(data.Erythrogram)),
		"leukogram_rows":   document.HTMLTemplateRaw(resultRowsHTML(data.Leukogram)),
		"platelet_rows":    document.HTMLTemplateRaw(resultRowsHTML(data.Platelets)),
	})
}

func sampleData() reportData {
	return reportData{
		PatientName: "ANA CLARA ALMEIDA",
		PatientID:   "DEMO-260718-0042",
		BirthSex:    "14/03/1988  |  Feminino",
		Doctor:      "Dra. Helena Modelo  |  CRM-CE 00000",
		Collection:  "18/07/2026 às 08:05",
		Released:    "18/07/2026 às 10:14",
		Protocol:    "DEMO-2026-0042",
		Erythrogram: []resultRow{
			{"Hemacias", "4,08", "milhoes/uL", "3,90 - 5,20", "4,21", ""},
			{"Hemoglobina", "11,2", "g/dL", "12,0 - 15,5", "11,7", "BAIXO"},
			{"Hematocrito", "34,7", "%", "36,0 - 46,0", "36,2", "BAIXO"},
			{"VCM", "85,0", "fL", "80,0 - 100,0", "86,0", ""},
			{"HCM", "27,5", "pg", "27,0 - 33,0", "27,8", ""},
			{"CHCM", "32,3", "g/dL", "31,0 - 36,0", "32,4", ""},
			{"RDW", "15,6", "%", "11,5 - 14,5", "15,1", "ALTO"},
		},
		Leukogram: []resultRow{
			{"Leucocitos", "8.740", "/uL", "4.000 - 10.000", "7.960", ""},
			{"Neutrofilos  59,8%", "5.227", "/uL", "1.800 - 7.700", "4.744", ""},
			{"Linfocitos  29,0%", "2.535", "/uL", "1.000 - 4.000", "2.388", ""},
			{"Monocitos  6,9%", "603", "/uL", "200 - 1.000", "549", ""},
			{"Eosinofilos  3,6%", "315", "/uL", "20 - 500", "223", ""},
			{"Basofilos  0,7%", "60", "/uL", "0 - 100", "56", ""},
		},
		Platelets: []resultRow{
			{"Plaquetas", "428.000", "/uL", "150.000 - 450.000", "391.000", ""},
			{"Volume plaquetario medio", "11,8", "fL", "7,5 - 11,5", "11,2", "ALTO"},
		},
	}
}

func resultRowsHTML(rows []resultRow) string {
	var b strings.Builder
	for _, row := range rows {
		resultClass := "result"
		statusClass := "status normal-status"
		status := "NA FAIXA"
		if row.Flag != "" {
			resultClass += " abnormal-result"
			statusClass = "status flag-status"
			status = row.Flag
		}
		fmt.Fprintf(&b, `<tr>
			<td width="29%%" class="exam-name">%s</td>
			<td width="14%%" class="%s">%s</td>
			<td width="11%%" class="unit">%s</td>
			<td width="22%%" class="reference">%s</td>
			<td width="11%%" class="previous">%s</td>
			<td width="13%%" class="%s">%s</td>
		</tr>`, esc(strings.ToUpper(row.Name)), resultClass, esc(row.Result), esc(row.Unit), esc(row.Reference), esc(row.Previous), statusClass, esc(status))
	}
	return b.String()
}

func esc(s string) string {
	return stdhtml.EscapeString(s)
}

func drawLabHeader(pdf *document.Document, data reportData) {
	navy := color{18, 48, 71}
	teal := color{0, 157, 157}
	muted := color{96, 117, 130}
	ink := color{31, 45, 57}
	line := color{214, 224, 228}
	pale := color{244, 248, 249}
	paleAlert := color{252, 238, 239}
	alert := color{186, 55, 64}
	white := color{255, 255, 255}

	fill(pdf, navy)
	pdf.Rect(0, 0, 210, 5, "F")
	fill(pdf, teal)
	pdf.Circle(21, 21, 10, "F")
	set(pdf, "B", 14.5, white)
	centerText(pdf, 11, 15.4, 20, 9, "A+")
	set(pdf, "B", 16, navy)
	pdf.Text(37, 17.5, "AURORA")
	set(pdf, "", 6.7, color{0, 117, 124})
	pdf.Text(37.2, 23.3, "MEDICINA DIAGNÓSTICA")
	set(pdf, "", 5.4, muted)
	pdf.Text(37.2, 29, "Unidade Fortaleza  |  CNES 0000000 (fictício)")
	pdf.Text(37.2, 33.4, "(85) 3000-0000  |  laudos@aurora-demo.test")

	set(pdf, "B", 5, muted)
	rightText(pdf, 145, 11, 53, 3.5, "RESULTADO LABORATORIAL")
	set(pdf, "B", 9.6, navy)
	rightText(pdf, 132, 17, 66, 5, "HEMOGRAMA COMPLETO")
	set(pdf, "", 5.4, muted)
	rightText(pdf, 145, 25, 53, 3.5, "Laudo "+data.Protocol)
	rightText(pdf, 145, 29.5, 53, 3.5, "Emitido "+data.Released)

	fill(pdf, paleAlert)
	pdf.Rect(0, 39, 210, 7, "F")
	set(pdf, "B", 5.8, alert)
	centerText(pdf, 0, 40.5, 210, 4, "MODELO FICTÍCIO  |  DADOS SINTÉTICOS  |  SEM VALIDADE CLÍNICA")

	fill(pdf, white)
	draw(pdf, line)
	pdf.RoundedRect(10, 50, 190, 31, 2.4, "1234", "DF")
	headerField(pdf, 15, 57, "PACIENTE", data.PatientName, 59)
	headerField(pdf, 79, 57, "NASCIMENTO / SEXO", data.BirthSex, 51)
	headerField(pdf, 139, 57, "ID DO PACIENTE", data.PatientID, 56)
	pdf.Line(15, 66.5, 195, 66.5)
	headerField(pdf, 15, 72, "SOLICITANTE", data.Doctor, 59)
	headerField(pdf, 79, 72, "COLETA", data.Collection, 51)
	headerField(pdf, 139, 72, "LIBERAÇÃO", data.Released, 56)

	fill(pdf, pale)
	pdf.RoundedRect(10, 86, 190, 19, 2.4, "1234", "DF")
	headerField(pdf, 15, 93, "AMOSTRA", "Sangue total com EDTA", 47)
	headerField(pdf, 67, 93, "MÉTODO", "Impedância, citometria de fluxo e espectrofotometria", 79)
	headerField(pdf, 151, 93, "QUALIDADE", "Amostra íntegra", 44)
	set(pdf, "", 5.2, ink)
}

func drawFixedFooter(pdf *document.Document) {
	navy := color{18, 48, 71}
	muted := color{96, 117, 130}
	ink := color{31, 45, 57}
	line := color{214, 224, 228}
	white := color{255, 255, 255}

	pdf.SetPage(1)
	// Unified HTML requires the standard automatic page-break policy while it
	// plans. Fixed page chrome below the body is positioned manually.
	pdf.SetAutoPageBreak(false, 0)
	draw(pdf, line)
	pdf.Line(10, 257, 200, 257)
	set(pdf, "B", 5.1, muted)
	pdf.Text(12, 263, "RESPONSÁVEL TÉCNICA E PROFISSIONAL SIGNATÁRIA")
	set(pdf, "B", 7, navy)
	pdf.Text(12, 269, "Dra. Marina Campos")
	set(pdf, "", 5.6, ink)
	pdf.Text(12, 274, "Biomédica  |  CRBM-6 00000 (fictício)")
	set(pdf, "", 4.9, muted)
	pdf.Text(12, 279, "Assinatura demonstrativa: 84C2-A7F1-0D42-2026")

	set(pdf, "B", 5.1, muted)
	pdf.Text(120, 263, "VALIDAÇÃO DO DOCUMENTO")
	set(pdf, "", 5.5, ink)
	pdf.Text(120, 269, "aurora-demo.test/validar")
	pdf.Text(120, 274, "Código DEMO-2026-0042")
	set(pdf, "I", 4.8, muted)
	pdf.Text(120, 279, "Endereço sem função real")

	fill(pdf, navy)
	pdf.Rect(0, 286, 210, 11, "F")
	set(pdf, "", 5, white)
	pdf.Text(10, 291, "AURORA DIAGNÓSTICOS DEMO  |  Rua Exemplo, 120 - Fortaleza/CE  |  CNES 0000000")
	set(pdf, "B", 5, white)
	rightText(pdf, 170, 288.5, 30, 3.5, "Página 1 de 1")
	set(pdf, "", 4.7, color{190, 213, 224})
	pdf.Text(10, 295, "Documento demonstrativo; nenhum dado pertence a paciente ou laboratório real.")
}

func drawTechnicalNote(pdf *document.Document) {
	fill(pdf, color{252, 238, 239})
	draw(pdf, color{239, 199, 202})
	pdf.RoundedRect(10, 237, 190, 15, 2, "1234", "DF")
	fill(pdf, color{186, 55, 64})
	pdf.RoundedRectExt(10, 237, 3, 15, 2, 0, 0, 2, "F")
	set(pdf, "B", 5.1, color{186, 55, 64})
	pdf.Text(17, 242.3, "OBSERVAÇÃO TÉCNICA")
	set(pdf, "", 5.7, color{31, 45, 57})
	pdf.Text(17, 247.1, "Discreta anisocitose. Resultados sinalizados devem ser interpretados com o contexto clínico.")
	set(pdf, "I", 4.7, color{96, 117, 130})
	pdf.Text(17, 250.7, "Faixas de referência variam conforme método, população e laboratório executor.")
}

func drawBodyTitle(pdf *document.Document) {
	set(pdf, "B", 13.5, color{18, 48, 71})
	pdf.Text(10, 116, "Seu hemograma")
	set(pdf, "", 5.6, color{96, 117, 130})
	pdf.Text(10, 122, "Leitura visual com resultado atual, faixa de referência e histórico.")
	rightText(pdf, 122, 118.5, 78, 3.5, "Verde: na faixa  |  Vermelho: sinalizado")
}

func drawSummaryCards(pdf *document.Document) {
	drawSummaryCard(pdf, 10, 127, 59, "15 parâmetros analisados", false)
	drawSummaryCard(pdf, 75.5, 127, 59, "4 resultados sinalizados", true)
	drawSummaryCard(pdf, 141, 127, 59, "Histórico comparativo", false)
}

func drawSummaryCard(pdf *document.Document, x, y, w float64, text string, warning bool) {
	background := color{244, 248, 249}
	border := color{214, 224, 228}
	foreground := color{18, 48, 71}
	if warning {
		background = color{252, 238, 239}
		border = color{239, 199, 202}
		foreground = color{186, 55, 64}
	}
	fill(pdf, background)
	draw(pdf, border)
	pdf.RoundedRect(x, y, w, 13, 2, "1234", "DF")
	set(pdf, "B", 6.2, foreground)
	centerText(pdf, x+2, y+4.7, w-4, 3.5, text)
}

func headerField(pdf *document.Document, x, y float64, label, value string, width float64) {
	set(pdf, "B", 4.9, color{96, 117, 130})
	pdf.Text(x, y, label)
	set(pdf, "B", 6.1, color{31, 45, 57})
	pdf.SetXY(x, y+1.5)
	pdf.CellFormat(width, 4.8, value, "", 0, "L", false, 0, "")
}

func hemogramaTemplate() string {
	return `
		<style>
			p { margin:0; color:#1f2d39; }
			table { width:100%; border-collapse:collapse; }
			th { background-color:#123047; color:#ffffff; border:none; padding:3pt; font-size:5.2pt; text-align:left; }
			th.center { text-align:center; }
			td { color:#1f2d39; border:none; border-bottom:1px solid #d6e0e4; padding:2.2pt; font-size:5.9pt; vertical-align:middle; }
			.section-cell { background-color:#e8f7f6; color:#00757c; font-size:5.9pt; font-weight:bold; padding:3pt; }
			.exam-name { font-size:5.7pt; }
			.result { color:#1f2d39; text-align:right; font-size:6.4pt; font-weight:bold; }
			.abnormal-result { color:#ba3740; }
			.unit { color:#607582; font-size:5.2pt; }
			.reference { color:#526b78; font-size:5.4pt; text-align:center; }
			.previous { color:#607582; font-size:5.3pt; text-align:center; }
			.status { font-size:4.7pt; text-align:center; font-weight:bold; }
			.normal-status { color:#4c7e6f; }
			.flag-status { color:#ba3740; background-color:#fceeed; }
		</style>

		<table>
			<thead><tr>
				<th width="29%">EXAME</th><th width="14%" class="center">SEU RESULTADO</th><th width="11%">UNIDADE</th>
				<th width="22%" class="center">REFERENCIA</th><th width="11%" class="center">ANTERIOR</th><th width="13%" class="center">STATUS</th>
			</tr></thead>
			<tbody>
				<tr><td colspan="6" class="section-cell">ERITROGRAMA  |  SERIE VERMELHA</td></tr>
				{{erythrogram_rows}}
				<tr><td colspan="6" class="section-cell">LEUCOGRAMA  |  CONTAGEM ABSOLUTA E DIFERENCIAL</td></tr>
				{{leukogram_rows}}
				<tr><td colspan="6" class="section-cell">PLAQUETAS  |  CONTAGEM E VOLUME MEDIO</td></tr>
				{{platelet_rows}}
			</tbody>
		</table>

	`
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
