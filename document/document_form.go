// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	stdhtml "html"
	"strings"

	"github.com/cssbruno/paperrune/layout"
)

// FormDocument describes a generic form that can be rendered as supported HTML
// or converted into shared document blocks.
type FormDocument struct {
	Title    string        // Form title.
	Sections []FormSection // Form sections in display order.
}

// FormSection groups related form questions.
type FormSection struct {
	Title        string         // Section title.
	Questions    []FormQuestion // Questions in this section.
	BreakBefore  bool           // Insert a page break before this section.
	BreakAfter   bool           // Insert a page break after this section.
	KeepTogether bool           // Prefer to keep the section on one page.
}

// FormQuestion stores one question and its answer.
type FormQuestion struct {
	Label    string     // Question label.
	Answer   FormAnswer // Question answer.
	Required bool       // Whether the question is required.
}

// FormAnswer stores a plain, list, or table answer.
type FormAnswer struct {
	Text  string     // Plain text answer.
	Items []string   // List answer items.
	Table [][]string // Table answer rows.
}

// FormDocumentHTML returns the canonical supported-HTML representation of a
// form document.
func FormDocumentHTML(form FormDocument) string {
	out := newFormHTMLWriter(estimateFormHTMLSize(form))
	if strings.TrimSpace(form.Title) != "" {
		out.elementText("h1", form.Title)
	}
	for _, section := range form.Sections {
		writeFormSectionHTML(out, section)
	}
	return out.string()
}

// ValidateFormDocumentHTML validates the canonical form HTML against the
// supported HTML subset.
func ValidateFormDocumentHTML(form FormDocument) []string {
	pdf := MustNew()
	html := pdf.HTMLNew()
	return html.ValidateHTML(FormDocumentHTML(form))
}

// FormDocumentBlocks converts a form into shared document blocks.
func FormDocumentBlocks(form FormDocument) []layout.Block {
	blocks := make([]layout.Block, 0, 1+len(form.Sections))
	if strings.TrimSpace(form.Title) != "" {
		blocks = append(blocks, layout.HeadingBlock{Level: 1, Segments: []layout.TextSegment{{Text: form.Title}}})
	}
	for _, section := range form.Sections {
		sectionBlock := layout.SectionBlock{
			Title:             section.Title,
			KeepTitleWithBody: true,
			Box:               layout.BoxStyle{KeepTogether: section.KeepTogether},
		}
		if section.BreakBefore || section.BreakAfter {
			sectionBlock.Blocks = append(sectionBlock.Blocks, layout.PageBreakBlock{Before: section.BreakBefore})
		}
		for _, question := range section.Questions {
			sectionBlock.Blocks = append(sectionBlock.Blocks, formQuestionBlocks(question)...)
		}
		if section.BreakAfter {
			sectionBlock.Blocks = append(sectionBlock.Blocks, layout.PageBreakBlock{After: true})
		}
		blocks = append(blocks, sectionBlock)
	}
	return blocks
}

// FormDocumentModel converts a form into a shared Document.
func FormDocumentModel(form FormDocument) *layout.LayoutDocument {
	doc := layout.NewLayoutDocument()
	doc.Title = form.Title
	doc.Body = FormDocumentBlocks(form)
	return doc
}

type formHTMLWriter struct {
	out strings.Builder
}

func newFormHTMLWriter(capacity int) *formHTMLWriter {
	w := &formHTMLWriter{}
	if capacity > 0 {
		w.out.Grow(capacity)
	}
	return w
}

func (w *formHTMLWriter) string() string {
	return w.out.String()
}

func (w *formHTMLWriter) raw(value string) {
	w.out.WriteString(value)
}

func (w *formHTMLWriter) byte(value byte) {
	w.out.WriteByte(value)
}

func (w *formHTMLWriter) text(value string) {
	w.out.WriteString(escapeFormHTML(value))
}

func (w *formHTMLWriter) elementText(tag, value string) {
	w.byte('<')
	w.raw(tag)
	w.byte('>')
	w.text(value)
	w.raw("</")
	w.raw(tag)
	w.byte('>')
}

func writeFormSectionHTML(out *formHTMLWriter, section FormSection) {
	style := formSectionStyle(section)
	out.raw(`<section class="form-section"`)
	if style != "" {
		out.raw(` style="`)
		out.raw(style)
		out.byte('"')
	}
	out.byte('>')
	if strings.TrimSpace(section.Title) != "" {
		out.elementText("h2", section.Title)
	}
	out.raw(`<dl class="form-qa">`)
	for _, question := range section.Questions {
		out.raw("<dt>")
		out.text(question.Label)
		if question.Required {
			out.raw(" *")
		}
		out.raw("</dt><dd>")
		writeFormAnswerHTML(out, question.Answer)
		out.raw("</dd>")
	}
	out.raw("</dl></section>")
}

func writeFormAnswerHTML(out *formHTMLWriter, answer FormAnswer) {
	switch {
	case len(answer.Table) > 0:
		out.raw(`<table class="form-answer-table"><tbody>`)
		for _, row := range answer.Table {
			out.raw("<tr>")
			for _, cell := range row {
				out.elementText("td", cell)
			}
			out.raw("</tr>")
		}
		out.raw("</tbody></table>")
	case len(answer.Items) > 0:
		out.raw(`<ul class="form-answer-list">`)
		for _, item := range answer.Items {
			out.elementText("li", item)
		}
		out.raw("</ul>")
	default:
		out.elementText("p", answer.Text)
	}
}

func estimateFormHTMLSize(form FormDocument) int {
	size := len(form.Title) + 16
	for _, section := range form.Sections {
		size += len(section.Title) + 80
		for _, question := range section.Questions {
			size += len(question.Label) + len(question.Answer.Text) + 32
			for _, item := range question.Answer.Items {
				size += len(item) + 16
			}
			for _, row := range question.Answer.Table {
				size += 16
				for _, cell := range row {
					size += len(cell) + 16
				}
			}
		}
	}
	return size
}

func formQuestionBlocks(question FormQuestion) []layout.Block {
	label := question.Label
	if question.Required {
		label += " *"
	}
	blocks := []layout.Block{
		layout.ParagraphBlock{
			Segments: []layout.TextSegment{{Text: label}},
			Style:    layout.TextStyle{Bold: true},
			Box:      layout.BoxStyle{KeepWithNext: true},
		},
	}
	blocks = append(blocks, formAnswerBlocks(question.Answer)...)
	return blocks
}

func formAnswerBlocks(answer FormAnswer) []layout.Block {
	switch {
	case len(answer.Table) > 0:
		rows := make([]layout.TableRow, 0, len(answer.Table))
		for _, inputRow := range answer.Table {
			row := layout.TableRow{KeepTogether: true}
			for _, cell := range inputRow {
				row.Cells = append(row.Cells, layout.TableCell{
					Blocks: []layout.Block{layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: cell}}}},
				})
			}
			rows = append(rows, row)
		}
		return []layout.Block{layout.TableBlock{Body: rows}}
	case len(answer.Items) > 0:
		items := make([]layout.ListItem, 0, len(answer.Items))
		for _, item := range answer.Items {
			items = append(items, layout.ListItem{Blocks: []layout.Block{layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: item}}}}})
		}
		return []layout.Block{layout.ListBlock{Items: items}}
	default:
		return []layout.Block{layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: answer.Text}}}}
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
