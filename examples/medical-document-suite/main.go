// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

// Command medical-document-suite creates restrained Brazilian clinical-document
// examples at practical print sizes. Ordinary documents use ISO A5. Type A and
// B notification demonstrations use the compact 200 x 60 mm physical proportion
// of the current Anvisa forms. All people, credentials, identifiers, products,
// and verification values are fictional; controlled forms are unusable demos.
package main

import (
	"log"
	"os"

	"github.com/cssbruno/paperrune/document"
	"github.com/cssbruno/paperrune/examples/internal/assets"
	"github.com/cssbruno/paperrune/examples/internal/outpath"
)

type color struct{ r, g, b int }

type examCategory struct {
	title string
	items []string
}

type notificationTheme struct {
	letter     string
	stockName  string
	stockColor color
}

var (
	navy    = color{24, 48, 64}
	blue    = color{30, 91, 124}
	ink     = color{31, 39, 44}
	muted   = color{91, 104, 111}
	line    = color{174, 182, 187}
	soft    = color{245, 247, 248}
	white   = color{255, 255, 255}
	red     = color{166, 36, 44}
	paleRed = color{252, 244, 244}
	typeA   = color{255, 247, 191}
	typeB   = color{214, 235, 250}
)

func main() {
	buildExamRequestA5()
	buildStandardPrescriptionA5()
	buildControlledNotification(
		notificationTheme{letter: "A", stockName: "papel amarelo", stockColor: typeA},
		"notificacao-receita-a-demo.pdf",
		"notificacao-receita-a-preview-papel-amarelo.pdf",
	)
	buildControlledNotification(
		notificationTheme{letter: "B", stockName: "papel azul", stockColor: typeB},
		"notificacao-receita-b-demo.pdf",
		"notificacao-receita-b-preview-papel-azul.pdf",
	)
}

func buildExamRequestA5() {
	pdf := newDocument(148, 210, "Solicitação de exames A5 - demonstração")
	selected := map[string]bool{
		"Hemograma completo": true, "Ferritina": true, "Glicemia de jejum": true,
		"Hemoglobina glicada": true, "Ureia": true, "Creatinina + TFG": true,
		"Sódio": true, "Potássio": true, "Colesterol total": true,
		"HDL-colesterol": true, "LDL-colesterol": true, "Triglicerídeos": true,
		"AST (TGO)": true, "ALT (TGP)": true, "TSH": true, "T4 livre": true,
		"Vitamina B12": true, "Vitamina D": true, "Urina tipo I": true,
	}

	pageOne := []examCategory{
		{"HEMATOLOGIA E COAGULAÇÃO", []string{"Hemograma completo", "Reticulócitos", "Coagulograma", "TP / INR", "TTPa", "Fibrinogênio", "D-dímero", "VHS", "Tipagem ABO / Rh", "Coombs direto"}},
		{"METABOLISMO, RINS E ELETRÓLITOS", []string{"Glicemia de jejum", "Hemoglobina glicada", "Insulina", "HOMA-IR", "Ureia", "Creatinina + TFG", "Ácido úrico", "Sódio", "Potássio", "Cálcio total", "Magnésio", "Fósforo"}},
		{"LIPÍDIOS E MARCADORES CARDÍACOS", []string{"Colesterol total", "HDL-colesterol", "LDL-colesterol", "Triglicerídeos", "Colesterol não HDL", "Apolipoproteína A1", "Apolipoproteína B", "Lipoproteína(a)", "Troponina", "CK-MB"}},
		{"FÍGADO E PÂNCREAS", []string{"AST (TGO)", "ALT (TGP)", "Gama-GT", "Fosfatase alcalina", "Bilirrubinas", "Albumina", "Proteínas totais", "Amilase", "Lipase", "LDH"}},
		{"TIREOIDE E HORMÔNIOS", []string{"TSH", "T4 livre", "T3 total", "Anti-TPO", "Cortisol", "Prolactina", "FSH", "LH", "Estradiol", "Progesterona", "Testosterona total", "SHBG"}},
		{"FERRO, VITAMINAS E NUTRIÇÃO", []string{"Ferritina", "Ferro sérico", "Transferrina", "Saturação transferrina", "Vitamina B12", "Ácido fólico", "Vitamina D", "Zinco"}},
	}

	pageTwo := []examCategory{
		{"INFLAMAÇÃO E AUTOIMUNIDADE", []string{"PCR ultrassensível", "Fator reumatoide", "FAN", "Anti-CCP", "C3", "C4", "Anti-DNA", "ANCA", "Eletroforese proteínas", "Imunoglobulinas"}},
		{"SOROLOGIAS E INFECTOLOGIA", []string{"HIV 1/2", "HBsAg", "Anti-HBs", "Anti-HBc total", "Anti-HCV", "Sífilis", "Dengue", "Zika", "Chikungunya", "Chagas", "Toxoplasmose", "CMV", "Rubéola", "EBV"}},
		{"URINA, FEZES E MICROBIOLOGIA", []string{"Urina tipo I", "Urocultura", "Albumina/creatinina", "Proteinúria 24 h", "Parasitológico de fezes", "Sangue oculto", "Calprotectina fecal", "Coprocultura", "Cultura de secreção", "Bacterioscopia", "Antibiograma", "Teste molecular"}},
		{"MARCADORES E RASTREAMENTO", []string{"PSA total e livre", "CEA", "CA 125", "CA 19-9", "Alfafetoproteína", "Beta-hCG", "Citopatológico", "Pesquisa HPV", "Sangue oculto", "Eletroforese Hb"}},
		{"IMAGEM E EXAMES FUNCIONAIS", []string{"Radiografia", "Ultrassonografia", "Mamografia", "Densitometria óssea", "Tomografia", "Ressonância magnética", "Eletrocardiograma", "Ecocardiograma", "Holter", "MAPA", "Espirometria", "Endoscopia"}},
		{"OUTROS", []string{"Anatomopatológico", "Citometria de fluxo", "Teste alérgico", "Teste genético", "Avaliação auditiva", "Campimetria", "Eletroneuromiografia", "Polissonografia"}},
	}

	pdf.AddPage()
	drawA5Header(pdf, "SOLICITAÇÃO DE EXAMES", "PED-DEMO-0184", "1 / 2")
	drawDemoStrip(pdf, 29, "DOCUMENTO FICTÍCIO - SEM VALIDADE CLÍNICA")
	drawPatientSummary(pdf, 38)
	drawExamGrid(pdf, 8, 64, pageOne, selected)
	drawA5Footer(pdf, "Seleções marcadas são exemplos e não constituem solicitação médica.", "1 / 2")

	pdf.AddPage()
	drawA5Header(pdf, "SOLICITAÇÃO DE EXAMES", "PED-DEMO-0184", "2 / 2")
	drawDemoStrip(pdf, 29, "CONTINUAÇÃO - DOCUMENTO FICTÍCIO - SEM VALIDADE CLÍNICA")
	drawExamGrid(pdf, 8, 39, pageTwo, selected)
	drawExamSignoff(pdf)
	drawA5Footer(pdf, "Exemplo visual A5. A indicação e o preparo dependem de avaliação profissional.", "2 / 2")
	writePDF(pdf, "pedido-exames-a5.pdf")
}

func buildStandardPrescriptionA5() {
	pdf := newDocument(148, 210, "Receituário A5 - demonstração")
	pdf.AddPage()
	drawA5Header(pdf, "RECEITUÁRIO", "RX-DEMO-0048", "")
	drawDemoStrip(pdf, 29, "MODELO FICTÍCIO - PRODUTOS INEXISTENTES - NÃO UTILIZAR")
	drawPatientSummary(pdf, 38)

	set(pdf, "B", 6.2, navy)
	pdf.Text(9, 68, "PRESCRIÇÃO")
	drawMedicationA5(pdf, 9, 72, "01", "MEDICAMENTO DEMONSTRATIVO A - 500 mg", "Comprimidos - 9 (nove) unidades", "Uso oral. Tomar 1 comprimido a cada 8 horas por 3 dias.")
	drawMedicationA5(pdf, 9, 102, "02", "SOLUÇÃO DEMONSTRATIVA B - 0,9%", "Frasco com 100 mL - 1 (uma) unidade", "Uso tópico. Aplicar conforme orientação profissional demonstrativa.")
	drawMedicationA5(pdf, 9, 132, "03", "PRODUTO DEMONSTRATIVO C - 10 mg/mL", "Frasco conta-gotas - 1 (uma) unidade", "Uso demonstrativo. Este produto não existe e esta receita não tem validade.")
	drawA5Signature(pdf, 174)
	drawA5Footer(pdf, "Dados, assinatura, CRM e código são fictícios. Documento sem validade.", "")
	writePDF(pdf, "receituario-a5.pdf")
}

func buildControlledNotification(theme notificationTheme, printFilename, previewFilename string) {
	printPDF := newDocument(200, 60, "Notificação de Receita "+theme.letter+" - arte para "+theme.stockName)
	printPDF.AddPage()
	drawNotificationSlip(printPDF, theme, false)
	writePDF(printPDF, printFilename)

	previewPDF := newDocument(200, 60, "Notificação de Receita "+theme.letter+" - prévia em "+theme.stockName)
	previewPDF.AddPage()
	drawNotificationSlip(previewPDF, theme, true)
	writePDF(previewPDF, previewFilename)
}

func newDocument(width, height float64, title string) *document.Document {
	pdf := document.MustNew(
		document.WithUnit(document.UnitMillimeter),
		document.WithCustomPageSize(document.Size{Wd: width, Ht: height}),
	)
	pdf.SetTitle(title, true)
	pdf.SetCreator("examples/medical-document-suite", false)
	pdf.SetMargins(0, 0, 0)
	pdf.SetAutoPageBreak(false, 0)
	addUTF8Fonts(pdf)
	return pdf
}

func addUTF8Fonts(pdf *document.Document) {
	fonts := map[string]string{
		"": "DejaVuSansCondensed.ttf", "B": "DejaVuSansCondensed-Bold.ttf",
		"I": "DejaVuSansCondensed-Oblique.ttf", "BI": "DejaVuSansCondensed-BoldOblique.ttf",
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

func drawA5Header(pdf *document.Document, title, id, page string) {
	fill(pdf, blue)
	pdf.Rect(0, 0, 148, 1.3, "F")
	set(pdf, "B", 10.5, navy)
	pdf.Text(9, 10.5, "CLÍNICA AURORA")
	set(pdf, "", 4.5, muted)
	pdf.Text(9, 15, "MEDICINA INTEGRADA - FORTALEZA/CE")
	pdf.Text(9, 19, "CNES 0000000  |  CNPJ 00.000.000/0001-00  |  dados fictícios")
	set(pdf, "B", 7.2, navy)
	rightText(pdf, 78, 8, 61, 4, title)
	set(pdf, "", 4.6, muted)
	rightText(pdf, 78, 14, 61, 3.5, "Nº "+id)
	if page != "" {
		rightText(pdf, 111, 19, 28, 3.2, "PÁGINA "+page)
	}
	draw(pdf, line)
	pdf.Line(9, 25.5, 139, 25.5)
}

func drawDemoStrip(pdf *document.Document, y float64, label string) {
	fill(pdf, paleRed)
	pdf.Rect(9, y, 130, 5.8, "F")
	set(pdf, "B", 4.6, red)
	centerText(pdf, 9, y+1.2, 130, 3.2, label)
}

func drawPatientSummary(pdf *document.Document, y float64) {
	draw(pdf, line)
	fill(pdf, soft)
	pdf.Rect(9, y, 130, 22, "DF")
	compactField(pdf, 12, y+5, 70, "PACIENTE", "ANA CLARA EXEMPLO")
	compactField(pdf, 84, y+5, 52, "CPF", "000.000.000-00")
	compactField(pdf, 12, y+14.5, 82, "ENDEREÇO", "Rua Demonstração, 100 - Fortaleza/CE")
	compactField(pdf, 96, y+14.5, 40, "DATA", "18/07/2026")
}

func drawExamGrid(pdf *document.Document, x, y float64, categories []examCategory, selected map[string]bool) {
	const gap = 3.0
	columnWidth := (132 - gap) / 2
	yPos := [2]float64{y, y}
	for index, category := range categories {
		column := index % 2
		categoryX := x + float64(column)*(columnWidth+gap)
		yPos[column] = drawExamCategory(pdf, categoryX, yPos[column], columnWidth, category, selected) + 2
	}
}

func drawExamCategory(pdf *document.Document, x, y, width float64, category examCategory, selected map[string]bool) float64 {
	rows := (len(category.items) + 1) / 2
	height := 7.0 + float64(rows)*4.6
	fill(pdf, soft)
	pdf.Rect(x, y, width, 5.7, "F")
	draw(pdf, line)
	pdf.Rect(x, y, width, height, "D")
	set(pdf, "B", 4.4, navy)
	pdf.Text(x+2, y+3.9, category.title)
	itemWidth := (width - 4) / 2
	for index, item := range category.items {
		column := index % 2
		row := index / 2
		itemX := x + 2 + float64(column)*itemWidth
		itemY := y + 7.2 + float64(row)*4.6
		drawCheckbox(pdf, itemX, itemY-2.5, selected[item])
		set(pdf, "", 4.05, ink)
		pdf.Text(itemX+3.4, itemY, item)
	}
	return y + height
}

func drawCheckbox(pdf *document.Document, x, y float64, checked bool) {
	draw(pdf, color{124, 135, 141})
	pdf.Rect(x, y, 2.2, 2.2, "D")
	if checked {
		draw(pdf, blue)
		pdf.SetLineWidth(0.35)
		pdf.Line(x+0.35, y+1.15, x+0.9, y+1.75)
		pdf.Line(x+0.9, y+1.75, x+1.9, y+0.35)
	}
}

func drawExamSignoff(pdf *document.Document) {
	draw(pdf, line)
	pdf.Line(9, 166, 139, 166)
	set(pdf, "B", 4.5, muted)
	pdf.Text(10, 171, "SOLICITANTE")
	set(pdf, "B", 6.1, navy)
	pdf.Text(10, 177, "Dra. Helena Modelo")
	set(pdf, "", 4.8, ink)
	pdf.Text(10, 182, "CRM-CE 00000 - fictício")
	set(pdf, "", 4.5, muted)
	rightText(pdf, 78, 171, 60, 3.2, "ASSINATURA DEMONSTRATIVA")
	rightText(pdf, 78, 177, 60, 3.2, "sem certificado digital")
}

func drawMedicationA5(pdf *document.Document, x, y float64, number, name, quantity, directions string) {
	draw(pdf, line)
	pdf.Rect(x, y, 130, 27, "D")
	fill(pdf, blue)
	pdf.Rect(x, y, 7, 27, "F")
	set(pdf, "B", 6.5, white)
	centerText(pdf, x, y+3, 7, 4, number)
	set(pdf, "B", 5.8, navy)
	pdf.Text(x+11, y+6.2, name)
	set(pdf, "", 4.8, muted)
	pdf.Text(x+11, y+12, quantity)
	draw(pdf, line)
	pdf.Line(x+11, y+15, x+125, y+15)
	set(pdf, "", 5.15, ink)
	pdf.SetXY(x+11, y+17.2)
	pdf.MultiCell(114, 4.1, directions, "", "L", false)
}

func drawA5Signature(pdf *document.Document, y float64) {
	draw(pdf, line)
	pdf.Line(9, y, 139, y)
	set(pdf, "B", 4.4, muted)
	pdf.Text(10, y+5, "PRESCRITORA")
	set(pdf, "B", 5.9, navy)
	pdf.Text(10, y+10.5, "Dra. Helena Modelo - CRM-CE 00000")
	set(pdf, "", 4.4, muted)
	pdf.Text(10, y+15, "Assinatura demonstrativa - sem certificado")
	set(pdf, "B", 4.4, muted)
	rightText(pdf, 89, y+5, 49, 3, "AUTENTICIDADE")
	set(pdf, "", 4.4, ink)
	rightText(pdf, 89, y+10.5, 49, 3, "RX-DEMO-0048 - inválido")
}

func drawNotificationSlip(pdf *document.Document, theme notificationTheme, previewStock bool) {
	formInk := color{20, 20, 20}
	secondaryInk := color{78, 78, 78}
	if previewStock {
		fill(pdf, theme.stockColor)
		pdf.Rect(0, 0, 200, 60, "F")
	}
	draw(pdf, formInk)
	pdf.SetLineWidth(0.35)
	pdf.Rect(1.5, 1.5, 197, 57, "D")
	pdf.Line(44, 6.3, 44, 58.5)
	pdf.Line(118, 6.3, 118, 58.5)

	set(pdf, "B", 4.1, formInk)
	centerText(pdf, 2, 2.45, 196, 2.8, "MODELO FICTÍCIO - SEM NUMERAÇÃO - SEM VALIDADE - NÃO DISPENSAR")
	draw(pdf, formInk)
	pdf.Line(2, 6.3, 198, 6.3)

	set(pdf, "B", 5.0, formInk)
	centerText(pdf, 3, 8.2, 39, 3.2, "NOTIFICAÇÃO DE RECEITA")
	set(pdf, "B", 16, formInk)
	centerText(pdf, 3, 13, 39, 9, theme.letter)
	set(pdf, "B", 3.5, secondaryInk)
	centerText(pdf, 3, 23, 39, 2.8, "Nº SEM-NUMERAÇÃO-SNCR")
	draw(pdf, formInk)
	pdf.Line(4, 28, 41.5, 28)
	slipField(pdf, 4, 31, 37, "DATA", "18/07/2026", formInk, secondaryInk)
	slipField(pdf, 4, 39, 37, "PACIENTE", "ANA CLARA EXEMPLO", formInk, secondaryInk)
	slipField(pdf, 4, 47, 37, "CPF / PASSAPORTE", "000.000.000-00", formInk, secondaryInk)
	set(pdf, "", 3.25, secondaryInk)
	pdf.Text(4, 56, "Documento educacional - dados fictícios")

	drawSlipSection(pdf, 45.5, 8, 70.5, 22, "EMITENTE", formInk)
	set(pdf, "B", 4.0, formInk)
	pdf.Text(48, 15, "Dra. Helena Modelo - CRM-CE 00000")
	set(pdf, "", 3.45, formInk)
	pdf.Text(48, 19.5, "Clínica Aurora Demo - CNES 0000000")
	pdf.Text(48, 24, "Rua Exemplo, 120 - Fortaleza/CE")
	pdf.Text(48, 28, "CNPJ 00.000.000/0001-00 - fictício")

	drawSlipSection(pdf, 45.5, 32, 70.5, 24.5, "COMPRADOR", formInk)
	slipField(pdf, 48, 39, 64.5, "NOME", "NÃO PREENCHIDO - DEMO", formInk, secondaryInk)
	slipField(pdf, 48, 47, 38, "CPF", "SEM DADOS", formInk, secondaryInk)
	slipField(pdf, 89, 47, 23.5, "TELEFONE", "-", formInk, secondaryInk)
	set(pdf, "", 3.25, secondaryInk)
	pdf.Text(48, 54.5, "Endereço: campo não preenchido")

	drawSlipSection(pdf, 119.5, 8, 77, 48.5, "PRESCRIÇÃO", formInk)
	set(pdf, "B", 4.15, formInk)
	pdf.Text(122.5, 15, "NENHUM MEDICAMENTO PRESCRITO")
	slipField(pdf, 122.5, 22, 70.5, "SUBSTÂNCIA / MEDICAMENTO", "PRODUTO DEMONSTRATIVO INEXISTENTE", formInk, secondaryInk)
	slipField(pdf, 122.5, 31, 32.5, "CONCENTRAÇÃO", "-", formInk, secondaryInk)
	slipField(pdf, 158, 31, 35, "FORMA", "-", formInk, secondaryInk)
	slipField(pdf, 122.5, 40, 32.5, "QUANTIDADE", "00 (zero)", formInk, secondaryInk)
	slipField(pdf, 158, 40, 35, "POSOLOGIA", "SEM POSOLOGIA", formInk, secondaryInk)
	set(pdf, "", 3.3, secondaryInk)
	pdf.Text(122.5, 51.5, "Assinatura demonstrativa - sem certificado")
	draw(pdf, formInk)
	pdf.Line(122.5, 53.5, 193, 53.5)

	addInvalidWatermarkColor(pdf, 100, 33, 12, 8, formInk)
}

func drawSlipSection(pdf *document.Document, x, y, width, height float64, label string, accent color) {
	draw(pdf, accent)
	pdf.SetLineWidth(0.22)
	pdf.Rect(x, y, width, height, "D")
	set(pdf, "B", 3.8, accent)
	pdf.Text(x+2.5, y+4, label)
	pdf.Line(x+2, y+5.5, x+width-2, y+5.5)
}

func slipField(pdf *document.Document, x, y, width float64, label, value string, formInk, secondaryInk color) {
	set(pdf, "B", 3.05, secondaryInk)
	pdf.Text(x, y, label)
	set(pdf, "", 3.55, formInk)
	pdf.Text(x, y+3.4, value)
	draw(pdf, color{110, 110, 110})
	pdf.SetLineWidth(0.14)
	pdf.Line(x, y+4.2, x+width, y+4.2)
}

func compactField(pdf *document.Document, x, y, width float64, label, value string) {
	set(pdf, "B", 4.0, muted)
	pdf.Text(x, y, label)
	set(pdf, "B", 5.0, ink)
	pdf.SetXY(x, y+1.1)
	pdf.CellFormat(width, 4, value, "", 0, "L", false, 0, "")
}

func drawA5Footer(pdf *document.Document, note, page string) {
	draw(pdf, line)
	pdf.Line(9, 196, 139, 196)
	set(pdf, "", 3.9, muted)
	pdf.Text(9, 201, note)
	if page != "" {
		set(pdf, "B", 4.0, navy)
		rightText(pdf, 124, 198.5, 15, 3, page)
	}
	set(pdf, "", 3.65, muted)
	pdf.Text(9, 206, "Clínica Aurora Demo - Fortaleza/CE - dados integralmente fictícios")
}

func addInvalidWatermark(pdf *document.Document, centerX, centerY, size, angle float64) {
	addInvalidWatermarkColor(pdf, centerX, centerY, size, angle, red)
}

func addInvalidWatermarkColor(pdf *document.Document, centerX, centerY, size, angle float64, watermarkColor color) {
	pdf.SetAlpha(0.12, "Normal")
	set(pdf, "B", size, watermarkColor)
	pdf.TransformBegin()
	pdf.TransformRotate(angle, centerX, centerY)
	centerText(pdf, centerX-70, centerY-6, 140, 12, "MODELO FICTÍCIO - SEM VALIDADE")
	pdf.TransformEnd()
	pdf.SetAlpha(1, "Normal")
}

func fill(pdf *document.Document, value color) { pdf.SetFillColor(value.r, value.g, value.b) }

func draw(pdf *document.Document, value color) {
	pdf.SetDrawColor(value.r, value.g, value.b)
	pdf.SetLineWidth(0.2)
}

func set(pdf *document.Document, style string, size float64, value color) {
	pdf.SetFont("dejavu", style, size)
	pdf.SetTextColor(value.r, value.g, value.b)
}

func rightText(pdf *document.Document, x, y, width, height float64, text string) {
	pdf.SetXY(x, y)
	pdf.CellFormat(width, height, text, "", 0, "R", false, 0, "")
}

func centerText(pdf *document.Document, x, y, width, height float64, text string) {
	pdf.SetXY(x, y)
	pdf.CellFormat(width, height, text, "", 0, "C", false, 0, "")
}
