// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package paperedit

import (
	"strconv"
	"strings"
	"testing"
)

var paperEditBenchmarkSink Result

// BenchmarkPaperEditIncrementalReplaceText measures the complete bounded
// semantic-edit transaction, including exact revision verification, lossless
// parse, target lookup, minimal patch planning, candidate reparse, and detached
// evidence construction. The source is intentionally large enough to expose
// accidental whole-document quadratic behavior while retaining one local edit.
func BenchmarkPaperEditIncrementalReplaceText(b *testing.B) {
	source := paperEditBenchmarkSource(200)
	transaction := Transaction{
		File:             "benchmark.paper",
		Source:           source,
		ExpectedRevision: SourceRevision(source),
		Operations:       []Operation{ReplaceText{Target: "@text-100", Text: "Updated local value"}},
	}
	result, err := Apply(transaction)
	if err != nil || !result.Applied || !strings.Contains(result.Source, `text @text-100: "Updated local value"`) {
		b.Fatalf("validate incremental edit: applied=%t err=%v", result.Applied, err)
	}
	b.ReportAllocs()
	b.SetBytes(int64(len(source)))
	b.ResetTimer()
	for range b.N {
		result, err := Apply(transaction)
		if err != nil {
			b.Fatal(err)
		}
		paperEditBenchmarkSink = result
	}
}

func paperEditBenchmarkSource(nodes int) string {
	var source strings.Builder
	source.Grow(nodes * 90)
	source.WriteString("document @benchmark:\n  page @page:\n    body @body:\n")
	for index := 0; index < nodes; index++ {
		value := strconv.Itoa(index)
		source.WriteString("      paragraph @paragraph-")
		source.WriteString(value)
		source.WriteString(":\n        text @text-")
		source.WriteString(value)
		source.WriteString(": \"Row ")
		source.WriteString(value)
		source.WriteString("\"\n")
	}
	return source.String()
}
