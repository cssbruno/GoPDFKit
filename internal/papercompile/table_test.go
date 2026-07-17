package papercompile

import (
	"github.com/cssbruno/gopdfkit/internal/paperlang"
	"github.com/cssbruno/gopdfkit/layout"
	"testing"
)

const paperTableSource = "document @report:\n  page @sheet:\n    body @body:\n      table @ledger:\n        caption: \"Ledger\"\n        repeat-header: true\n        split: \"rows\"\n        table-track @name-track:\n          width: 60pt\n        table-track @value-track:\n          width: 40pt\n        table-header @head:\n          table-row @head-row:\n            cell @name-head:\n              text: \"Name\"\n            cell @value-head:\n              text: \"Value\"\n        table-row @body-row:\n          cell @name:\n            text: \"Alpha\"\n          cell @value:\n            colspan: 1\n            paragraph:\n              text: \"10\"\n"

func TestCompileReadableTable(t *testing.T) {
	parsed := paperlang.Parse("table.paper", paperTableSource)
	result := Compile(parsed.AST)
	if !parsed.OK() || !result.OK() {
		t.Fatalf("diagnostics=%#v/%#v", parsed.Diagnostics, result.Diagnostics)
	}
	table := result.Document.Body[0].(layout.TableBlock)
	if table.Caption != "Ledger" || !table.Style.RepeatHeader || len(table.Columns) != 2 || len(table.Header) != 1 || len(table.Body) != 1 || !table.Header[0].Cells[0].Header {
		t.Fatalf("table=%#v", table)
	}
}
