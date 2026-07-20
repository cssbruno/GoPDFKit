// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

// Command lab-report-suite creates three polished, single-page Brazilian
// laboratory report examples: cardiometabolic biochemistry, urinalysis, and
// urine culture with an antibiogram. Every person, identifier, credential,
// address, result, and verification code in these examples is fictional.
package main

import (
	"log"
	"os"

	"github.com/cssbruno/paperrune/document"
	"github.com/cssbruno/paperrune/examples/internal/assets"
	"github.com/cssbruno/paperrune/examples/internal/outpath"
)

type color struct {
	r int
	g int
	b int
}

type theme struct {
	accent     color
	accentDark color
	pale       color
	border     color
}

type patient struct {
	name      string
	birthSex  string
	id        string
	requester string
	service   string
	unit      string
}

type specimen struct {
	collected string
	received  string
	released  string
	material  string
	method    string
	quality   string
}

type labRow struct {
	name      string
	result    string
	unit      string
	reference string
	previous  string
	status    string
}

var (
	navy  = color{18, 48, 71}
	ink   = color{31, 45, 57}
	muted = color{96, 117, 130}
	line  = color{214, 224, 228}
	soft  = color{246, 249, 250}
	white = color{255, 255, 255}
	alert = color{186, 55, 64}
	green = color{38, 126, 99}
	amber = color{173, 107, 16}

	biochemistryTheme = theme{
		accent:     color{33, 98, 135},
		accentDark: color{26, 75, 104},
		pale:       color{244, 247, 249},
		border:     color{205, 215, 221},
	}
	urinalysisTheme = theme{
		accent:     color{33, 98, 135},
		accentDark: color{26, 75, 104},
		pale:       color{244, 247, 249},
		border:     color{205, 215, 221},
	}
	microbiologyTheme = theme{
		accent:     color{33, 98, 135},
		accentDark: color{26, 75, 104},
		pale:       color{244, 247, 249},
		border:     color{205, 215, 221},
	}
)

func main() {
	buildBiochemistryReport()
	buildUrinalysisReport()
	buildMicrobiologyReport()
}

func buildBiochemistryReport() {
	pdf := newPDF("Painel cardiometabólico - modelo brasileiro contemporâneo", "examples/lab-report-suite")
	drawHeader(pdf, "PAINEL CARDIOMETABÓLICO", "DEMO-2026-0107", "18/07/2026 às 11:26", biochemistryTheme)
	drawPatientCard(pdf, patient{
		name: "ANA CLARA ALMEIDA", birthSex: "14/03/1988  |  Feminino", id: "DEMO-260718-0107",
		requester: "Dra. Helena Modelo  |  CRM-CE 00000", service: "Particular", unit: "Fortaleza - CE",
	})
	drawSpecimenCard(pdf, specimen{
		collected: "18/07/2026  07:42", received: "18/07/2026  08:06", released: "18/07/2026  11:26",
		material: "Soro e sangue total", method: "Métodos enzimáticos, ISE, imunoturbidimetria e cálculo", quality: "Jejum: 10 horas",
	})
	drawBiochemistryResults(pdf)
	drawSignoff(pdf, "DEMO-2026-0107")
	drawFooter(pdf)
	writePDF(pdf, "lab-bioquimica-report.pdf")
}

func buildUrinalysisReport() {
	pdf := newPDF("Urina tipo I - modelo brasileiro contemporâneo", "examples/lab-report-suite")
	drawHeader(pdf, "URINA TIPO I", "DEMO-2026-0218", "18/07/2026 às 15:08", urinalysisTheme)
	drawPatientCard(pdf, patient{
		name: "MARCOS VINÍCIUS DEMO", birthSex: "22/09/1975  |  Masculino", id: "DEMO-260718-0218",
		requester: "Dr. André Referência  |  CRM-CE 00000", service: "Ambulatorial", unit: "Fortaleza - CE",
	})
	drawSpecimenCard(pdf, specimen{
		collected: "18/07/2026  13:51", received: "18/07/2026  14:10", released: "18/07/2026  15:08",
		material: "Urina - jato médio", method: "Físico-química automatizada e citometria de fluxo", quality: "Frasco estéril",
	})
	drawUrinalysisResults(pdf)
	drawSignoff(pdf, "DEMO-2026-0218")
	drawFooter(pdf)
	writePDF(pdf, "lab-urinalise-report.pdf")
}

func buildMicrobiologyReport() {
	pdf := newPDF("Urocultura com antibiograma - modelo brasileiro contemporâneo", "examples/lab-report-suite")
	drawHeader(pdf, "UROCULTURA + ANTIBIOGRAMA", "DEMO-2026-0304", "18/07/2026 às 17:42", microbiologyTheme)
	drawPatientCard(pdf, patient{
		name: "JULIANA RIBEIRO MODELO", birthSex: "06/11/1992  |  Feminino", id: "DEMO-260716-0304",
		requester: "Dra. Laura Exemplo  |  CRM-CE 00000", service: "Pronto atendimento", unit: "Fortaleza - CE",
	})
	drawSpecimenCard(pdf, specimen{
		collected: "16/07/2026  09:18", received: "16/07/2026  09:43", released: "18/07/2026  17:42",
		material: "Urina - jato médio", method: "Cultura, identificação por MALDI-TOF e microdiluição", quality: "Frasco estéril",
	})
	drawMicrobiologyResults(pdf)
	drawSignoff(pdf, "DEMO-2026-0304")
	drawFooter(pdf)
	writePDF(pdf, "lab-microbiologia-report.pdf")
}

func newPDF(title, creator string) *document.Document {
	pdf := document.MustNew()
	pdf.SetTitle(title, true)
	pdf.SetCreator(creator, false)
	addUTF8Fonts(pdf)
	pdf.SetMargins(0, 0, 0)
	pdf.SetAutoPageBreak(false, 0)
	pdf.AddPage()
	return pdf
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

func writePDF(pdf *document.Document, name string) {
	if err := pdf.OutputFileAndClose(outpath.File(name)); err != nil {
		log.Fatal(err)
	}
}

func drawHeader(pdf *document.Document, reportTitle, reportID, issued string, t theme) {
	fill(pdf, t.accent)
	pdf.Rect(0, 0, 210, 2.2, "F")
	pdf.Rect(10, 8, 1.3, 18, "F")

	set(pdf, "B", 13.2, navy)
	pdf.Text(15, 14, "LABORATÓRIO AURORA")
	set(pdf, "", 6.4, t.accentDark)
	pdf.Text(15, 19.3, "ANÁLISES CLÍNICAS")
	set(pdf, "", 5.15, muted)
	pdf.Text(15, 24.8, "Unidade Fortaleza  |  CNES 0000000  |  CNPJ 00.000.000/0001-00 (dados fictícios)")

	set(pdf, "B", 5.4, muted)
	rightText(pdf, 132, 8.5, 68, 3.5, "LAUDO LABORATORIAL")
	set(pdf, "B", 8.5, navy)
	rightText(pdf, 122, 13, 78, 5, reportTitle)
	set(pdf, "", 5.2, muted)
	rightText(pdf, 132, 20, 68, 3.4, "Pedido: "+reportID)
	rightText(pdf, 122, 24.2, 78, 3.4, "Emissão: "+issued)

	draw(pdf, color{168, 184, 194})
	pdf.Line(10, 30.5, 200, 30.5)
	fill(pdf, color{250, 246, 246})
	pdf.Rect(10, 33, 190, 6, "F")
	set(pdf, "B", 5.4, alert)
	centerText(pdf, 10, 34.3, 190, 3.5, "DOCUMENTO DEMONSTRATIVO - DADOS FICTÍCIOS - SEM VALIDADE CLÍNICA")
}

func drawPatientCard(pdf *document.Document, p patient) {
	fill(pdf, soft)
	draw(pdf, line)
	pdf.Rect(10, 42, 190, 20, "DF")
	field(pdf, 13, 46, "PACIENTE", p.name, 69)
	field(pdf, 84, 46, "NASCIMENTO / SEXO", p.birthSex, 54)
	field(pdf, 141, 46, "REGISTRO", p.id, 56)
	field(pdf, 13, 55.5, "SOLICITANTE", p.requester, 69)
	field(pdf, 84, 55.5, "ATENDIMENTO", p.service, 54)
	field(pdf, 141, 55.5, "UNIDADE", p.unit, 56)
}

func drawSpecimenCard(pdf *document.Document, s specimen) {
	draw(pdf, line)
	pdf.Rect(10, 64.5, 190, 19, "D")
	field(pdf, 13, 68.5, "COLETA", s.collected, 43)
	field(pdf, 58, 68.5, "RECEBIMENTO", s.received, 43)
	field(pdf, 103, 68.5, "LIBERAÇÃO", s.released, 43)
	field(pdf, 148, 68.5, "MATERIAL", s.material, 49)
	set(pdf, "B", 4.9, muted)
	pdf.Text(13, 79.3, "MÉTODO")
	set(pdf, "", 5.6, ink)
	pdf.Text(31, 79.3, s.method)
	set(pdf, "", 4.9, muted)
	rightText(pdf, 167, 76.8, 30, 3.4, s.quality)
}

func drawBiochemistryResults(pdf *document.Document) {
	rows := []struct {
		title    string
		subtitle string
		values   []labRow
	}{
		{"METABOLISMO GLICÊMICO", "Rastreamento e controle", []labRow{
			{"Glicose em jejum", "108", "mg/dL", "70 - 99", "96", "ALTO"},
			{"Hemoglobina glicada (HbA1c)", "5,9", "%", "4,0 - 5,6", "5,6", "ALTO"},
			{"Glicose média estimada", "123", "mg/dL", "Informativo", "114", ""},
		}},
		{"FUNÇÃO RENAL E ELETRÓLITOS", "Soro", []labRow{
			{"Creatinina", "0,92", "mg/dL", "0,50 - 1,10", "0,88", ""},
			{"TFG estimada (CKD-EPI)", "82", "mL/min/1,73m²", "≥ 90", "86", "ABAIXO"},
			{"Ureia", "31", "mg/dL", "15 - 45", "29", ""},
			{"Sódio", "139", "mmol/L", "136 - 145", "140", ""},
			{"Potássio", "4,4", "mmol/L", "3,5 - 5,1", "4,2", ""},
		}},
		{"PERFIL LIPÍDICO", "Metas dependem do risco cardiovascular", []labRow{
			{"Colesterol total", "218", "mg/dL", "Desejável < 190", "204", "ALTO"},
			{"LDL-colesterol calculado", "142", "mg/dL", "Meta pelo risco", "131", "META"},
			{"HDL-colesterol", "49", "mg/dL", "> 40", "47", ""},
			{"Triglicerídeos", "136", "mg/dL", "< 150", "128", ""},
			{"Colesterol não HDL", "169", "mg/dL", "Meta pelo risco", "157", "META"},
		}},
		{"ENZIMAS HEPÁTICAS", "Atividade sérica", []labRow{
			{"AST (TGO)", "24", "U/L", "< 35", "22", ""},
			{"ALT (TGP)", "31", "U/L", "< 35", "27", ""},
			{"Gama-GT", "29", "U/L", "8 - 41", "26", ""},
		}},
	}

	set(pdf, "B", 13.5, navy)
	pdf.Text(10, 94, "Painel cardiometabólico")
	set(pdf, "", 5.2, muted)
	rightText(pdf, 128, 90.5, 72, 4, "Referências demonstrativas para pessoa adulta")
	drawLabTableHeader(pdf, 99)

	y := 106.0
	for sectionIndex, section := range rows {
		if sectionIndex > 0 {
			y += 0.7
		}
		y = drawLabSection(pdf, y, section.title, section.subtitle, biochemistryTheme)
		for i, row := range section.values {
			drawLabResultRow(pdf, y, row, i%2 == 1)
			y += 5.2
		}
	}

	fill(pdf, soft)
	pdf.Rect(10, 218, 190, 14, "F")
	fill(pdf, biochemistryTheme.accent)
	pdf.Rect(10, 218, 1.2, 14, "F")
	set(pdf, "B", 5.1, biochemistryTheme.accentDark)
	pdf.Text(14, 222.2, "NOTA INTERPRETATIVA")
	set(pdf, "", 5.35, ink)
	pdf.Text(14, 226.5, "Limiares glicêmicos seguem a Diretriz SBD 2025; diagnóstico exige correlação e confirmação clínica.")
	set(pdf, "I", 4.7, muted)
	pdf.Text(14, 230.1, "Metas lipídicas e TFG devem ser individualizadas; métodos e intervalos deste laudo são exclusivamente demonstrativos.")
}

func drawLabTableHeader(pdf *document.Document, y float64) {
	fill(pdf, navy)
	pdf.Rect(10, y, 190, 7, "F")
	set(pdf, "B", 5.2, white)
	pdf.Text(14, y+4.7, "ANÁLISE")
	rightText(pdf, 72, y+1.7, 34, 3, "RESULTADO")
	pdf.Text(110, y+4.7, "UNIDADE")
	centerText(pdf, 133, y+1.7, 33, 3, "REFERÊNCIA")
	centerText(pdf, 169, y+1.7, 18, 3, "ANTERIOR")
	centerText(pdf, 188, y+1.7, 10, 3, "STATUS")
}

func drawLabSection(pdf *document.Document, y float64, title, subtitle string, t theme) float64 {
	fill(pdf, t.pale)
	pdf.Rect(10, y, 190, 5.4, "F")
	set(pdf, "B", 6, t.accentDark)
	pdf.Text(14, y+3.8, title)
	set(pdf, "", 4.9, muted)
	pdf.Text(73, y+3.8, subtitle)
	return y + 5.4
}

func drawLabResultRow(pdf *document.Document, y float64, row labRow, shaded bool) {
	if shaded {
		fill(pdf, soft)
		pdf.Rect(10, y, 190, 5.2, "F")
	}
	draw(pdf, line)
	pdf.Line(10, y+5.2, 200, y+5.2)
	set(pdf, "", 5.9, ink)
	pdf.Text(14, y+3.6, row.name)
	resultColor := ink
	if row.status != "" {
		resultColor = alert
		if row.status == "META" {
			resultColor = amber
		}
	}
	set(pdf, "B", 6.2, resultColor)
	rightText(pdf, 72, y+0.9, 34, 3.2, row.result)
	set(pdf, "", 5.25, muted)
	pdf.Text(110, y+3.6, row.unit)
	centerText(pdf, 132, y+0.9, 35, 3.2, row.reference)
	centerText(pdf, 169, y+0.9, 18, 3.2, row.previous)
	if row.status == "" {
		return
	}
	badgeText := alert
	if row.status == "META" {
		badgeText = amber
	}
	set(pdf, "B", 4.1, badgeText)
	centerText(pdf, 187.5, y+1, 12, 2.7, row.status)
}

func drawUrinalysisResults(pdf *document.Document) {
	sections := []struct {
		title string
		rows  []labRow
	}{
		{"CARACTERES FÍSICOS", []labRow{
			{"Cor", "Amarelo-claro", "", "Amarelo", "", ""},
			{"Aspecto", "Ligeiramente turvo", "", "Límpido", "", "*"},
			{"Densidade", "1,018", "", "1,005 - 1,030", "", ""},
			{"pH", "5,5", "", "5,0 - 8,0", "", ""},
		}},
		{"ANÁLISE QUÍMICA", []labRow{
			{"Proteínas", "Negativo", "", "Negativo", "", ""},
			{"Glicose", "Negativo", "", "Negativo", "", ""},
			{"Corpos cetônicos", "Negativo", "", "Negativo", "", ""},
			{"Hemoglobina", "Traços", "", "Negativo", "", "*"},
			{"Nitrito", "Negativo", "", "Negativo", "", ""},
			{"Esterase leucocitária", "Traços", "", "Negativo", "", "*"},
			{"Bilirrubina", "Negativo", "", "Negativo", "", ""},
			{"Urobilinogênio", "Normal", "", "Normal", "", ""},
		}},
		{"SEDIMENTO URINÁRIO", []labRow{
			{"Leucócitos", "18", "/µL", "≤ 25", "", ""},
			{"Hemácias", "36", "/µL", "≤ 23", "", "*"},
			{"Células epiteliais", "7", "/µL", "≤ 15", "", ""},
			{"Bactérias", "Raras", "", "Ausentes a raras", "", ""},
			{"Muco", "Discreto", "", "Ausente a discreto", "", ""},
			{"Cilindros", "Ausentes", "", "Ausentes", "", ""},
			{"Cristais", "Ausentes", "", "Ausentes", "", ""},
		}},
	}

	set(pdf, "B", 13.5, navy)
	pdf.Text(10, 94, "Urina tipo I")
	set(pdf, "", 5.2, muted)
	rightText(pdf, 128, 90.5, 72, 4, "Análise físico-química e elementos figurados")
	drawUrinalysisTableHeader(pdf, 99)
	y := 106.0
	for sectionIndex, section := range sections {
		if sectionIndex > 0 {
			y += 0.7
		}
		y = drawLabSection(pdf, y, section.title, "", urinalysisTheme)
		for i, row := range section.rows {
			drawUrinalysisRow(pdf, y, row, i%2 == 1)
			y += 5.15
		}
	}

	fill(pdf, soft)
	pdf.Rect(10, 229, 190, 14, "F")
	fill(pdf, urinalysisTheme.accent)
	pdf.Rect(10, 229, 1.2, 14, "F")
	set(pdf, "B", 5.1, urinalysisTheme.accentDark)
	pdf.Text(14, 233.2, "OBSERVAÇÃO")
	set(pdf, "", 5.25, ink)
	pdf.Text(14, 237.4, "Resultados devem ser correlacionados com sintomas, técnica de coleta, transporte e conservação da amostra.")
	set(pdf, "I", 4.7, muted)
	pdf.Text(14, 241, "Método, unidades e intervalos deste documento são exemplos e variam conforme o sistema analítico.")
}

func drawUrinalysisTableHeader(pdf *document.Document, y float64) {
	fill(pdf, navy)
	pdf.Rect(10, y, 190, 7, "F")
	set(pdf, "B", 5.2, white)
	pdf.Text(14, y+4.7, "ANÁLISE")
	rightText(pdf, 88, y+1.7, 38, 3, "RESULTADO")
	pdf.Text(130, y+4.7, "UNIDADE")
	centerText(pdf, 148, y+1.7, 43, 3, "VALOR DE REFERÊNCIA")
	centerText(pdf, 193, y+1.7, 6, 3, "")
}

func drawUrinalysisRow(pdf *document.Document, y float64, row labRow, shaded bool) {
	if shaded {
		fill(pdf, soft)
		pdf.Rect(10, y, 190, 5.15, "F")
	}
	draw(pdf, line)
	pdf.Line(10, y+5.15, 200, y+5.15)
	set(pdf, "", 5.9, ink)
	pdf.Text(14, y+3.55, row.name)
	valueColor := ink
	if row.status != "" {
		valueColor = alert
	}
	set(pdf, "B", 6.1, valueColor)
	rightText(pdf, 88, y+0.85, 38, 3.2, row.result)
	set(pdf, "", 5.3, muted)
	pdf.Text(130, y+3.55, row.unit)
	centerText(pdf, 148, y+0.85, 43, 3.2, row.reference)
	if row.status != "" {
		set(pdf, "B", 7, alert)
		centerText(pdf, 193, y+0.7, 6, 3.5, "*")
	}
}

func drawMicrobiologyResults(pdf *document.Document) {
	antibiogram := []struct {
		drug     string
		category string
		meaning  string
	}{
		{"Amicacina", "S", "Sensível, dose padrão"},
		{"Amoxicilina/ácido clavulânico", "S", "Sensível, dose padrão"},
		{"Ampicilina", "R", "Resistente"},
		{"Ceftriaxona", "S", "Sensível, dose padrão"},
		{"Ciprofloxacino", "I", "Sensível, aumentando exposição"},
		{"Fosfomicina", "S", "Sensível, dose padrão"},
		{"Nitrofurantoína", "S", "Sensível, dose padrão"},
		{"Sulfametoxazol/trimetoprima", "R", "Resistente"},
	}

	set(pdf, "B", 13.5, navy)
	pdf.Text(10, 94, "Urocultura com antibiograma")
	set(pdf, "", 5.2, muted)
	rightText(pdf, 128, 90.5, 72, 4, "Cultura quantitativa - resultado demonstrativo")

	fill(pdf, microbiologyTheme.pale)
	pdf.Rect(10, 99, 190, 5.8, "F")
	set(pdf, "B", 5.8, microbiologyTheme.accentDark)
	pdf.Text(14, 103, "RESULTADO DA CULTURA")
	drawMicrobiologySummaryRow(pdf, 104.8, "Cultura", "POSITIVA", true)
	drawMicrobiologySummaryRow(pdf, 110.5, "Microrganismo isolado", "Escherichia coli", false)
	drawMicrobiologySummaryRow(pdf, 116.2, "Contagem", "≥ 100.000 UFC/mL", false)
	set(pdf, "", 5.1, ink)
	pdf.Text(14, 127.2, "Crescimento monomicrobiano. Correlacionar com sintomas, técnica de coleta e contexto clínico.")
	set(pdf, "I", 4.7, muted)
	pdf.Text(14, 131.2, "Isolado, contagem, categorias e interpretação são fictícios e não devem orientar terapia.")

	fill(pdf, microbiologyTheme.pale)
	pdf.Rect(10, 136, 190, 5.8, "F")
	set(pdf, "B", 5.8, microbiologyTheme.accentDark)
	pdf.Text(14, 140, "PERFIL DE SENSIBILIDADE ANTIMICROBIANA")
	fill(pdf, navy)
	pdf.Rect(10, 141.8, 190, 7, "F")
	set(pdf, "B", 5.2, white)
	pdf.Text(14, 146.5, "ANTIMICROBIANO")
	centerText(pdf, 119, 143.5, 23, 3, "CATEGORIA")
	pdf.Text(148, 146.5, "INTERPRETAÇÃO BrCAST")
	y := 148.8
	for i, row := range antibiogram {
		drawAntibiogramRow(pdf, y, row.drug, row.category, row.meaning, i%2 == 1)
		y += 5.8
	}
	fill(pdf, soft)
	pdf.Rect(10, 198, 190, 24, "F")
	fill(pdf, microbiologyTheme.accent)
	pdf.Rect(10, 198, 1.2, 24, "F")
	set(pdf, "B", 5.1, microbiologyTheme.accentDark)
	pdf.Text(14, 202.2, "LEGENDA BrCAST")
	set(pdf, "", 5.2, ink)
	pdf.Text(14, 206.5, "S - Sensível, dose padrão     I - Sensível, aumentando exposição     R - Resistente")
	set(pdf, "", 5.05, ink)
	pdf.Text(14, 212, "A escolha do antimicrobiano requer avaliação do sítio de infecção, dose, exposição e condição clínica.")
	set(pdf, "I", 4.65, muted)
	pdf.Text(14, 217.5, "Pontos de corte e resultados terapêuticos reais não são reproduzidos neste documento demonstrativo.")
}

func drawMicrobiologySummaryRow(pdf *document.Document, y float64, label, value string, highlight bool) {
	draw(pdf, line)
	pdf.Line(10, y+5.7, 200, y+5.7)
	set(pdf, "", 5.7, ink)
	pdf.Text(14, y+3.8, label)
	valueColor := ink
	if highlight {
		valueColor = alert
	}
	set(pdf, "B", 6.4, valueColor)
	pdf.Text(89, y+3.8, value)
}

func drawAntibiogramRow(pdf *document.Document, y float64, drug, category, meaning string, shaded bool) {
	if shaded {
		fill(pdf, soft)
		pdf.Rect(10, y, 190, 5.8, "F")
	}
	draw(pdf, line)
	pdf.Line(10, y+5.8, 200, y+5.8)
	set(pdf, "", 6, ink)
	pdf.Text(14, y+3.9, drug)
	badgeColor := green
	if category == "I" {
		badgeColor = amber
	}
	if category == "R" {
		badgeColor = alert
	}
	set(pdf, "B", 6, badgeColor)
	centerText(pdf, 125, y+0.9, 10, 3, category)
	set(pdf, "", 5.45, ink)
	pdf.Text(148, y+3.9, meaning)
}

func drawSignoff(pdf *document.Document, reportID string) {
	draw(pdf, color{168, 184, 194})
	pdf.Line(10, 249, 200, 249)
	set(pdf, "B", 5.2, muted)
	pdf.Text(12, 255, "RESPONSÁVEL TÉCNICA / PROFISSIONAL SIGNATÁRIA")
	set(pdf, "B", 6.8, navy)
	pdf.Text(12, 261, "Dra. Marina Campos")
	set(pdf, "", 5.6, ink)
	pdf.Text(12, 266, "Biomédica - CRBM-6 00000 (fictício)")
	set(pdf, "", 4.9, muted)
	pdf.Text(12, 271, "Assinado eletronicamente em 18/07/2026 - hash demonstrativo 84C2A7F10D42")

	set(pdf, "B", 5.2, muted)
	pdf.Text(119, 255, "AUTENTICIDADE DO LAUDO")
	set(pdf, "", 5.6, ink)
	pdf.Text(119, 261, "aurora-demo.test/validar")
	pdf.Text(119, 266, "Código: "+reportID)
	set(pdf, "I", 4.8, muted)
	pdf.Text(119, 271, "Endereço e código sem função real")
}

func drawFooter(pdf *document.Document) {
	fill(pdf, navy)
	pdf.Rect(0, 283, 210, 14, "F")
	set(pdf, "", 5.15, white)
	pdf.Text(10, 288.5, "LABORATÓRIO AURORA - Rua Exemplo, 120 - Fortaleza/CE - CEP 60000-000 - CNES 0000000")
	set(pdf, "B", 5.15, white)
	rightText(pdf, 172, 285.8, 28, 3.5, "Página 1 de 1")
	set(pdf, "", 4.8, color{190, 213, 224})
	pdf.Text(10, 293.5, "Documento fictício para demonstração de software. Não utilizar para diagnóstico, conduta ou identificação de paciente.")
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
