// Copyright (c) 2026 cssBruno

package gopdfkit

import (
	stdhtml "html"
	"strings"
)

// FormDocument describes a generic form that can be rendered as supported HTML
// or converted into shared document blocks.
type FormDocument struct {
	Title    string
	Sections []FormSection
}

// FormSection groups related form questions.
type FormSection struct {
	Title        string
	Questions    []FormQuestion
	BreakBefore  bool
	BreakAfter   bool
	KeepTogether bool
}

// FormQuestion stores one question and its answer.
type FormQuestion struct {
	Label    string
	Answer   FormAnswer
	Required bool
}

// FormAnswer stores a plain, list, or table answer.
type FormAnswer struct {
	Text  string
	Items []string
	Table [][]string
}

// FormDocumentHTML returns the canonical supported-HTML representation of a
// form document.
func FormDocumentHTML(form FormDocument) string {
	var out strings.Builder
	if strings.TrimSpace(form.Title) != "" {
		out.WriteString("<h1>")
		out.WriteString(escapeFormHTML(form.Title))
		out.WriteString("</h1>")
	}
	for _, section := range form.Sections {
		writeFormSectionHTML(&out, section)
	}
	return out.String()
}

// ValidateFormDocumentHTML validates the canonical form HTML against the
// supported HTML subset.
func ValidateFormDocumentHTML(form FormDocument) []string {
	pdf := New("P", "mm", "A4", "")
	html := pdf.HTMLNew()
	return html.ValidateHTML(FormDocumentHTML(form))
}

// FormDocumentBlocks converts a form into shared document blocks.
func FormDocumentBlocks(form FormDocument) []Block {
	blocks := make([]Block, 0, 1+len(form.Sections))
	if strings.TrimSpace(form.Title) != "" {
		blocks = append(blocks, HeadingBlock{Level: 1, Segments: []TextSegment{{Text: form.Title}}})
	}
	for _, section := range form.Sections {
		sectionBlock := SectionBlock{
			Title:             section.Title,
			KeepTitleWithBody: true,
			Box:               BoxStyle{KeepTogether: section.KeepTogether},
		}
		if section.BreakBefore || section.BreakAfter {
			sectionBlock.Blocks = append(sectionBlock.Blocks, PageBreakBlock{Before: section.BreakBefore})
		}
		for _, question := range section.Questions {
			sectionBlock.Blocks = append(sectionBlock.Blocks, formQuestionBlocks(question)...)
		}
		if section.BreakAfter {
			sectionBlock.Blocks = append(sectionBlock.Blocks, PageBreakBlock{After: true})
		}
		blocks = append(blocks, sectionBlock)
	}
	return blocks
}

// FormDocumentModel converts a form into a shared Document.
func FormDocumentModel(form FormDocument) *Document {
	doc := NewDocument(DocumentKindForm)
	doc.Title = form.Title
	doc.Body = FormDocumentBlocks(form)
	return doc
}

func writeFormSectionHTML(out *strings.Builder, section FormSection) {
	style := formSectionStyle(section)
	out.WriteString(`<section class="form-section"`)
	if style != "" {
		out.WriteString(` style="`)
		out.WriteString(style)
		out.WriteByte('"')
	}
	out.WriteByte('>')
	if strings.TrimSpace(section.Title) != "" {
		out.WriteString("<h2>")
		out.WriteString(escapeFormHTML(section.Title))
		out.WriteString("</h2>")
	}
	out.WriteString(`<dl class="form-qa">`)
	for _, question := range section.Questions {
		out.WriteString("<dt>")
		out.WriteString(escapeFormHTML(question.Label))
		if question.Required {
			out.WriteString(" *")
		}
		out.WriteString("</dt><dd>")
		writeFormAnswerHTML(out, question.Answer)
		out.WriteString("</dd>")
	}
	out.WriteString("</dl></section>")
}

func writeFormAnswerHTML(out *strings.Builder, answer FormAnswer) {
	switch {
	case len(answer.Table) > 0:
		out.WriteString(`<table class="form-answer-table"><tbody>`)
		for _, row := range answer.Table {
			out.WriteString("<tr>")
			for _, cell := range row {
				out.WriteString("<td>")
				out.WriteString(escapeFormHTML(cell))
				out.WriteString("</td>")
			}
			out.WriteString("</tr>")
		}
		out.WriteString("</tbody></table>")
	case len(answer.Items) > 0:
		out.WriteString(`<ul class="form-answer-list">`)
		for _, item := range answer.Items {
			out.WriteString("<li>")
			out.WriteString(escapeFormHTML(item))
			out.WriteString("</li>")
		}
		out.WriteString("</ul>")
	default:
		out.WriteString("<p>")
		out.WriteString(escapeFormHTML(answer.Text))
		out.WriteString("</p>")
	}
}

func formQuestionBlocks(question FormQuestion) []Block {
	label := question.Label
	if question.Required {
		label += " *"
	}
	blocks := []Block{
		ParagraphBlock{
			Segments: []TextSegment{{Text: label}},
			Style:    TextStyle{Bold: true},
			Box:      BoxStyle{KeepWithNext: true},
		},
	}
	blocks = append(blocks, formAnswerBlocks(question.Answer)...)
	return blocks
}

func formAnswerBlocks(answer FormAnswer) []Block {
	switch {
	case len(answer.Table) > 0:
		rows := make([]TableRow, 0, len(answer.Table))
		for _, inputRow := range answer.Table {
			row := TableRow{KeepTogether: true}
			for _, cell := range inputRow {
				row.Cells = append(row.Cells, TableCell{
					Blocks: []Block{ParagraphBlock{Segments: []TextSegment{{Text: cell}}}},
				})
			}
			rows = append(rows, row)
		}
		return []Block{TableBlock{Body: rows}}
	case len(answer.Items) > 0:
		items := make([]ListItem, 0, len(answer.Items))
		for _, item := range answer.Items {
			items = append(items, ListItem{Blocks: []Block{ParagraphBlock{Segments: []TextSegment{{Text: item}}}}})
		}
		return []Block{ListBlock{Items: items}}
	default:
		return []Block{ParagraphBlock{Segments: []TextSegment{{Text: answer.Text}}}}
	}
}

func formSectionStyle(section FormSection) string {
	styles := make([]string, 0, 3)
	if section.BreakBefore {
		styles = append(styles, "break-before: page")
	}
	if section.BreakAfter {
		styles = append(styles, "break-after: page")
	}
	if section.KeepTogether {
		styles = append(styles, "break-inside: avoid")
	}
	return strings.Join(styles, "; ")
}

func escapeFormHTML(value string) string {
	return stdhtml.EscapeString(strings.TrimSpace(value))
}
