// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
)

func TestHTMLCharacterizationBaselineProjection(t *testing.T) {
	first, err := HTMLCharacterizationJSON()
	if err != nil {
		t.Fatal(err)
	}
	second, _ := HTMLCharacterizationJSON()
	if !reflect.DeepEqual(first, second) {
		t.Fatal("inventory JSON is nondeterministic")
	}
	sum := sha256.Sum256(first)
	got := hex.EncodeToString(sum[:])
	const want = "38863521a3a93b19c0f6bfd84e65a6c0a3eb3df688762bffeae30ea93c94edaa"
	if got != want {
		t.Fatalf("HTML characterization drift: hash=%s\n%s", got, first)
	}
	inventory := HTMLCharacterization()
	if !sort.StringsAreSorted(inventory.EntryPoints) || !sort.StringsAreSorted(inventory.RecognizedTags) || !sort.StringsAreSorted(inventory.RecognizedCSSProperties) {
		t.Fatal("inventory slices are not canonical")
	}
}

func TestHTMLCharacterizationPublicEntryPointsMatchAST(t *testing.T) {
	want := HTMLCharacterization().EntryPoints
	set := token.NewFileSet()
	packages, err := parser.ParseDir(set, ".", func(info fs.FileInfo) bool {
		return strings.HasPrefix(info.Name(), "html") && strings.HasSuffix(info.Name(), ".go") && !strings.HasSuffix(info.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0)
	for _, file := range packages["document"].Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || !fn.Name.IsExported() {
				continue
			}
			receiver := ""
			if fn.Recv != nil {
				receiver = htmlCharacterizationReceiver(fn.Recv.List[0].Type)
			}
			include := receiver == "HTML" || receiver == "CompiledHTML" || (receiver == "Document" && strings.Contains(fn.Name.Name, "HTML")) || receiver == "" && (strings.Contains(fn.Name.Name, "HTML") || fn.Name.Name == "RenderHTMLTemplate")
			if !include {
				continue
			}
			name := fn.Name.Name
			if receiver != "" {
				name = "(*" + receiver + ")." + name
			}
			got = append(got, name)
		}
	}
	sort.Strings(got)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("public HTML AST drift\ngot  %v\nwant %v", got, want)
	}
}

func htmlCharacterizationReceiver(expr ast.Expr) string {
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	if name, ok := expr.(*ast.Ident); ok {
		return name.Name
	}
	return ""
}

func TestHTMLCharacterizationFixturesExerciseEveryClassification(t *testing.T) {
	seen := map[string]bool{}
	for _, fixture := range HTMLCharacterization().Fixtures {
		if len(fixture.Source) > 4096 {
			t.Fatalf("fixture %s is unbounded", fixture.Name)
		}
		seen[fixture.Classification] = true
		compiled, err := CompileHTML(fixture.Source)
		if err != nil {
			t.Fatalf("%s compile: %v", fixture.Name, err)
		}
		switch fixture.Classification {
		case "malformed-recovered":
			if len(compiled.RecoveryIssues()) == 0 {
				t.Fatalf("%s has no recovery evidence", fixture.Name)
			}
		case "diagnostic-unsupported":
			pdf := characterizationPDF()
			html := pdf.HTMLNew()
			messages := html.ValidateHTML(fixture.Source)
			if len(messages) == 0 {
				t.Fatalf("%s has no unsupported diagnostic", fixture.Name)
			}
		case "rejected-by-policy":
			pdf := characterizationPDF()
			html := pdf.HTMLNew()
			if err := html.WriteContext(context.Background(), 10, fixture.Source); err == nil {
				t.Fatalf("%s was not rejected", fixture.Name)
			}
		case "strict-unified-plannable":
			planner := MustNew(WithUnit(UnitPoint))
			if plan, err := planner.PlanCompiledHTML(10, compiled); err != nil || plan.Hash() == "" {
				t.Fatalf("%s strict plan=%q %v", fixture.Name, plan.Hash(), err)
			}
		default:
			pdf := characterizationPDF()
			html := pdf.HTMLNew()
			if err := html.WriteContext(context.Background(), 10, fixture.Source); err != nil {
				t.Fatalf("%s render: %v", fixture.Name, err)
			}
		}
	}
	for _, class := range HTMLCharacterization().BehaviorClasses {
		if !seen[class] {
			t.Fatalf("classification %q has no fixture", class)
		}
	}
}

func TestRunHTMLCharacterizationRecordsDeterministicPDFEvidence(t *testing.T) {
	first, err := RunHTMLCharacterization(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	second, err := RunHTMLCharacterization(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	a, _ := first.CanonicalJSON()
	b, _ := second.CanonicalJSON()
	if !reflect.DeepEqual(a, b) || len(first.Fixtures) != len(HTMLCharacterization().Fixtures) {
		t.Fatalf("HTML evidence is incomplete or nondeterministic:\n%s\n%s", a, b)
	}
	for _, fixture := range first.Fixtures {
		rendered := fixture.Outcome == "rendered" || fixture.Outcome == "planned"
		if rendered && (fixture.Pages == 0 || fixture.PDF == nil || fixture.PDF.SHA256 == "" ||
			fixture.PDF.Bytes == 0 || len(fixture.PDF.PageText) != fixture.Pages) {
			t.Fatalf("rendered fixture lacks PDF evidence: %+v", fixture)
		}
		if !rendered && (fixture.Pages != 0 || fixture.PDF != nil) {
			t.Fatalf("non-rendered fixture fabricated PDF evidence: %+v", fixture)
		}
		if fixture.Outcome == "planned" && len(fixture.ReadingRoles) == 0 {
			t.Fatalf("unified fixture lacks reading order: %+v", fixture)
		}
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := RunHTMLCharacterization(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled HTML characterization = %v", err)
	}
}

func TestHTMLCharacterizationCursorLimitsCancellationAndConcurrentReuse(t *testing.T) {
	pdf := characterizationPDF()
	pdf.SetXY(24, 30)
	startPage, startY := pdf.PageCount(), pdf.GetY()
	html := pdf.HTMLNew()
	if err := html.WriteContext(context.Background(), 10, "<p>cursor integration</p>"); err != nil || pdf.PageCount() < startPage || pdf.GetY() == startY {
		t.Fatalf("cursor pages=%d y=%g err=%v", pdf.PageCount(), pdf.GetY(), err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	other := characterizationPDF()
	otherHTML := other.HTMLNew()
	if err := otherHTML.WriteContext(canceled, 10, "<p>cancel</p>"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation=%v", err)
	}
	limited := characterizationPDF()
	limitedHTML := limited.HTMLNew()
	limitedHTML.MaxHTMLBytes = 8
	if err := limitedHTML.WriteContext(context.Background(), 10, "<p>too much content</p>"); !errors.Is(err, ErrHTMLLimitExceeded) {
		t.Fatalf("limit=%v", err)
	}
	tableLimited := characterizationPDF()
	tableHTML := tableLimited.HTMLNew()
	tableHTML.MaxTableRows = 1
	if err := tableHTML.WriteContext(context.Background(), 10, "<table><tr><td>1</td></tr><tr><td>2</td></tr></table>"); err == nil || !strings.Contains(err.Error(), "row count exceeds") {
		t.Fatalf("table limit=%v", err)
	}
	compiled, err := CompileHTML("<section><p>concurrent compiled reuse</p></section>")
	if err != nil {
		t.Fatal(err)
	}
	template, err := CompileHTMLTemplate("<p>Hello {{name}}</p>")
	if err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	failures := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			target := characterizationPDF()
			renderer := target.HTMLNew()
			renderer.WriteCompiled(10, compiled)
			if err := renderer.WriteTemplateContext(context.Background(), 10, template, HTMLTemplateValues{"name": string(rune('A' + index%26))}); err != nil {
				failures <- err
				return
			}
			if err := target.Error(); err != nil {
				failures <- err
			}
		}(i)
	}
	wait.Wait()
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
}

func characterizationPDF() *Document {
	pdf := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 200, Ht: 160}), WithNoCompression())
	pdf.SetMargins(18, 18, 18)
	pdf.SetAutoPageBreak(true, 18)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 10)
	return pdf
}
