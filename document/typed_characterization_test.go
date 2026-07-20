// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/cssbruno/paperrune/layout"
)

func TestTypedLayoutInventoryPinsEveryBlockFieldAndType(t *testing.T) {
	want := map[layout.BlockKind][]string{
		layout.BlockKindParagraph:      {"Segments:[]layout.TextSegment", "Style:layout.TextStyle", "StyleRef:*layout.TextStyle", "Box:layout.BoxStyle", "BoxRef:*layout.BoxStyle"},
		layout.BlockKindHeading:        {"Level:int", "Segments:[]layout.TextSegment", "Style:layout.TextStyle", "StyleRef:*layout.TextStyle", "Box:layout.BoxStyle", "BoxRef:*layout.BoxStyle"},
		layout.BlockKindList:           {"Ordered:bool", "MarkerStyle:string", "Start:int", "Items:[]layout.ListItem", "Style:layout.TextStyle", "StyleRef:*layout.TextStyle", "Box:layout.BoxStyle", "BoxRef:*layout.BoxStyle"},
		layout.BlockKindTable:          {"Caption:string", "CaptionSegments:[]layout.TextSegment", "Columns:[]layout.TableColumn", "Header:[]layout.TableRow", "Body:[]layout.TableRow", "Footer:[]layout.TableRow", "Style:layout.TableStyle", "Box:layout.BoxStyle", "BoxRef:*layout.BoxStyle"},
		layout.BlockKindImage:          {"Source:string", "Data:[]uint8", "DataRef:*[]uint8", "Format:string", "Alt:string", "Caption:[]layout.TextSegment", "CaptionStyle:layout.TextStyle", "Width:float64", "Height:float64", "MaxWidth:float64", "MaxHeight:float64", "WidthPercent:uint32", "MaxWidthPercent:uint32", "Fit:layout.ImageFitMode", "FocusX:float64", "FocusY:float64", "FocusSet:bool", "Align:string", "DPI:float64", "Decorative:bool", "Box:layout.BoxStyle", "BoxRef:*layout.BoxStyle"},
		layout.BlockKindSignatureRow:   {"Columns:[]layout.SignatureColumn", "Gap:float64", "KeepTogether:bool", "Box:layout.BoxStyle", "BoxRef:*layout.BoxStyle"},
		layout.BlockKindMetadataGrid:   {"Fields:[]layout.MetadataField", "Columns:int", "Gap:float64", "Style:layout.TextStyle", "StyleRef:*layout.TextStyle", "Box:layout.BoxStyle", "BoxRef:*layout.BoxStyle"},
		layout.BlockKindQRVerification: {"QR:layout.QRBlock", "Text:[]layout.TextSegment", "Style:layout.TextStyle", "StyleRef:*layout.TextStyle", "Box:layout.BoxStyle", "BoxRef:*layout.BoxStyle"},
		layout.BlockKindNoteBox:        {"Title:string", "Body:[]layout.Block", "Style:layout.TextStyle", "StyleRef:*layout.TextStyle", "Box:layout.BoxStyle", "BoxRef:*layout.BoxStyle"},
		layout.BlockKindSection:        {"Title:string", "Blocks:[]layout.Block", "KeepTitleWithBody:bool", "Box:layout.BoxStyle", "BoxRef:*layout.BoxStyle"},
		layout.BlockKindClause:         {"Number:string", "Title:string", "Blocks:[]layout.Block", "BreakBefore:bool", "BreakAfter:bool", "KeepTogether:bool", "Box:layout.BoxStyle", "BoxRef:*layout.BoxStyle"},
		layout.BlockKindPageBreak:      {"Before:bool", "After:bool"},
		layout.BlockKindRowColumn:      {"Direction:layout.RowColumnDirection", "Gap:float64", "CrossGap:float64", "CrossSize:float64", "Wrap:string", "MainAlign:string", "CrossAlign:string", "AlignContent:string", "ReverseMain:bool", "Items:[]layout.RowColumnItem"},
		layout.BlockKindCanvas:         {"Width:float64", "Height:float64", "DefaultHorizontal:string", "DefaultVertical:string", "Items:[]layout.CanvasItem"},
	}
	inventory := TypedLayoutInventory()
	if len(inventory.Blocks) != len(want) {
		t.Fatalf("block inventory count = %d, want %d", len(inventory.Blocks), len(want))
	}
	for _, block := range inventory.Blocks {
		fields, exists := want[block.Kind]
		if !exists {
			t.Fatalf("unexpected block kind %q", block.Kind)
		}
		got := make([]string, len(block.Fields))
		for index, field := range block.Fields {
			got[index] = field.Name + ":" + field.Type
			if field.Status != TypedBehaviorDocumented {
				t.Fatalf("%s.%s status = %q", block.GoType, field.Name, field.Status)
			}
		}
		if !reflect.DeepEqual(got, fields) {
			t.Fatalf("%s fields drifted:\ngot  %v\nwant %v", block.GoType, got, fields)
		}
		delete(want, block.Kind)
	}
	if len(want) != 0 {
		t.Fatalf("missing block kinds: %v", want)
	}
}

func TestTypedPublicEntryPointsAndBlockImplementationsASTDoNotDrift(t *testing.T) {
	wantSignatures := map[string]string{
		"layout.NewLayoutDocument":                            "func() *LayoutDocument",
		"layout.NewDocumentModel":                             "func(title string, blocks ...Block) *LayoutDocument",
		"layout.(*LayoutDocument).AddBlock":                   "func(block Block)",
		"layout.NormalizeBlock":                               "func(block Block) (_ Block, ok bool)",
		"layout.NormalizeBlocks":                              "func(blocks []Block) []Block",
		"document.(*Document).WriteDocument":                  "func(doc *layout.LayoutDocument)",
		"document.(*Document).PlanLayoutDocument":             "func(doc *layout.LayoutDocument) (LayoutDocumentPlan, error)",
		"document.(*Document).PlanLayoutDocumentContext":      "func(ctx context.Context, doc *layout.LayoutDocument) (LayoutDocumentPlan, error)",
		"document.(*Document).WriteLayoutDocumentPlan":        "func(plan LayoutDocumentPlan) (int, error)",
		"document.(*Document).WriteLayoutDocumentPlanContext": "func(ctx context.Context, plan LayoutDocumentPlan) (int, error)",
	}
	got := parseTypedAPISignatures(t)
	for name, signature := range wantSignatures {
		if got[name] != signature {
			t.Fatalf("%s signature = %q, want %q", name, got[name], signature)
		}
	}
	if len(got) != len(wantSignatures) {
		t.Fatalf("typed API set drifted: got %v want %v", got, wantSignatures)
	}
	inventoryKeys := make([]string, 0, len(TypedLayoutInventory().EntryPoints))
	for _, entry := range TypedLayoutInventory().EntryPoints {
		name := entry.Package + "." + entry.Name
		if entry.Receiver != "" {
			name = entry.Package + ".(" + entry.Receiver + ")." + entry.Name
		}
		inventoryKeys = append(inventoryKeys, name)
	}
	sort.Strings(inventoryKeys)
	wantKeys := make([]string, 0, len(wantSignatures))
	for name := range wantSignatures {
		wantKeys = append(wantKeys, name)
	}
	sort.Strings(wantKeys)
	if !reflect.DeepEqual(inventoryKeys, wantKeys) {
		t.Fatalf("machine entry inventory drifted: got %v want %v", inventoryKeys, wantKeys)
	}

	wantBlocks := make([]string, len(typedBlockTypes))
	for index, item := range typedBlockTypes {
		wantBlocks[index] = item.typeOf.Name()
	}
	sort.Strings(wantBlocks)
	gotBlocks := parseLayoutBlockImplementations(t)
	if !reflect.DeepEqual(gotBlocks, wantBlocks) {
		t.Fatalf("Block implementations drifted: got %v want %v", gotBlocks, wantBlocks)
	}
}

func parseTypedAPISignatures(t *testing.T) map[string]string {
	t.Helper()
	result := map[string]string{}
	fset := token.NewFileSet()
	files := typedCharacterizationSourceFiles(t)
	selected := map[string]bool{"NewLayoutDocument": true, "NewDocumentModel": true, "AddBlock": true, "NormalizeBlock": true, "NormalizeBlocks": true, "WriteDocument": true, "PlanLayoutDocument": true, "PlanLayoutDocumentContext": true, "WriteLayoutDocumentPlan": true, "WriteLayoutDocumentPlanContext": true}
	for _, source := range files {
		file, err := parser.ParseFile(fset, source.path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || !ast.IsExported(fn.Name.Name) {
				continue
			}
			include := selected[fn.Name.Name]
			if !include {
				var params, results bytes.Buffer
				_ = format.Node(&params, fset, fn.Type.Params)
				if fn.Type.Results != nil {
					_ = format.Node(&results, fset, fn.Type.Results)
				}
				if source.pkg == "layout" {
					include = fn.Recv == nil && (strings.Contains(params.String(), "Block") || strings.Contains(results.String(), "LayoutDocument"))
				} else if fn.Recv != nil && receiverText(t, fset, fn.Recv.List[0].Type) == "*Document" {
					include = strings.Contains(params.String(), "layout.LayoutDocument") || strings.Contains(params.String(), "LayoutDocumentPlan")
				}
			}
			if !include {
				continue
			}
			name := source.pkg + "." + fn.Name.Name
			if fn.Recv != nil {
				name = source.pkg + ".(" + receiverText(t, fset, fn.Recv.List[0].Type) + ")." + fn.Name.Name
			}
			var buf bytes.Buffer
			if err := format.Node(&buf, fset, fn.Type); err != nil {
				t.Fatal(err)
			}
			result[name] = buf.String()
		}
	}
	return result
}

func typedCharacterizationSourceFiles(t *testing.T) []struct{ pkg, path string } {
	t.Helper()
	result := make([]struct{ pkg, path string }, 0)
	for _, source := range []struct{ pkg, pattern string }{{"layout", "../layout/*.go"}, {"document", "*.go"}} {
		paths, err := filepath.Glob(source.pattern)
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range paths {
			base := filepath.Base(path)
			if strings.HasSuffix(base, "_test.go") || strings.HasPrefix(base, "html") || strings.HasPrefix(base, "typed_characterization") {
				continue
			}
			result = append(result, struct{ pkg, path string }{source.pkg, path})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].pkg != result[j].pkg {
			return result[i].pkg < result[j].pkg
		}
		return result[i].path < result[j].path
	})
	return result
}

func receiverText(t *testing.T, fset *token.FileSet, expression ast.Expr) string {
	t.Helper()
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, expression); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func parseLayoutBlockImplementations(t *testing.T) []string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "../layout/layout_document.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	result := []string{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "DocumentBlockKind" || fn.Recv == nil {
			continue
		}
		text := receiverText(t, fset, fn.Recv.List[0].Type)
		result = append(result, strings.TrimPrefix(text, "*"))
	}
	sort.Strings(result)
	return result
}

func TestTypedCharacterizationCorpusIsCompleteBoundedAndDeterministic(t *testing.T) {
	requireDarwinRasterBaseline(t)
	inventory := TypedLayoutInventory()
	covered := map[layout.BlockKind]bool{}
	categories := map[string]bool{}
	for _, fixture := range inventory.Fixtures {
		for _, kind := range fixture.Blocks {
			covered[kind] = true
		}
		for _, category := range fixture.Coverage {
			categories[category] = true
		}
	}
	for _, block := range inventory.Blocks {
		if !covered[block.Kind] {
			t.Fatalf("block %q has no fixture", block.Kind)
		}
	}
	for _, required := range []string{"nested", "mixed", "exact-fit", "one-unit-over", "table", "large", "wide", "rowspan", "page-regions", "first", "odd", "even", "malformed", "recovery", "cancellation", "limits", "concurrent-reuse"} {
		if !categories[required] {
			t.Fatalf("missing fixture category %q", required)
		}
	}
	first, err := RunTypedCharacterization(t.Context(), TypedCharacterizationLimits{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := RunTypedCharacterization(t.Context(), TypedCharacterizationLimits{})
	if err != nil {
		t.Fatal(err)
	}
	a, _ := first.CanonicalJSON()
	b, _ := second.CanonicalJSON()
	if !bytes.Equal(a, b) {
		t.Fatalf("runner is nondeterministic:\n%s\n%s", a, b)
	}
	digest := sha256.Sum256(a)
	if got := hex.EncodeToString(digest[:]); got != "0e980d26e744958b49b33ba0bc83d4257be969b9c7e434efff6f7ce35e00b953" {
		t.Fatalf("typed characterization golden drift: got %s", got)
	}
	if len(first.Fixtures) != len(inventory.Fixtures) {
		t.Fatalf("fixture results = %d, want %d", len(first.Fixtures), len(inventory.Fixtures))
	}
	outcomes := map[string]bool{}
	for _, fixture := range first.Fixtures {
		outcomes[fixture.Outcome] = true
		success := fixture.Outcome == "planned" || fixture.Outcome == "accepted-malformed"
		if success {
			if fixture.PlanHash == "" || fixture.Pages == 0 || len(fixture.ReadingRoles) == 0 {
				t.Fatalf("successful fixture lacks plan/semantic evidence: %+v", fixture)
			}
			if len(fixture.BreakLedger) >= fixture.Pages {
				t.Fatalf("fixture %q has too many break records for %d pages: %+v", fixture.Name, fixture.Pages, fixture.BreakLedger)
			}
			for index, decision := range fixture.BreakLedger {
				if decision.Reason == "" || decision.FromPage == 0 || decision.ToPage != decision.FromPage+1 ||
					!decision.Preceding.Valid() || !decision.Triggering.Valid() || decision.Preceding == decision.Triggering {
					t.Fatalf("fixture %q break ledger[%d] lacks causal evidence: %+v", fixture.Name, index, decision)
				}
			}
		} else if len(fixture.BreakLedger) != 0 {
			t.Fatalf("unsuccessful fixture published break evidence: %+v", fixture)
		}
		if success && (fixture.Pages == 0 || fixture.PDF == nil || fixture.PDF.SHA256 == "" ||
			fixture.PDF.Bytes == 0 || len(fixture.PDF.PageText) != fixture.Pages) {
			t.Fatalf("successful fixture lacks complete PDF evidence: %+v", fixture)
		}
		if !success && fixture.PDF != nil {
			t.Fatalf("unsuccessful fixture published PDF evidence: %+v", fixture)
		}
	}
	for _, want := range []string{"planned", "rejected", "accepted-malformed", "canceled", "resource-limit"} {
		if !outcomes[want] {
			t.Fatalf("missing runner outcome %q: %+v", want, first.Fixtures)
		}
	}
	limits := DefaultTypedCharacterizationLimits()
	limits.MaxFixtures = 1
	if _, err := RunTypedCharacterization(t.Context(), limits); err == nil {
		t.Fatal("fixture limit was not enforced")
	}
}

func TestTypedBehaviorInventoryClassifiesStageZeroDomains(t *testing.T) {
	want := map[string]TypedBehaviorStatus{
		"PageTemplate.Header": TypedBehaviorDocumented, "PageTemplate.FirstPageHeader": TypedBehaviorDocumented,
		"PageTemplate.Footer": TypedBehaviorDocumented, "PageTemplate.EvenPageFooter": TypedBehaviorDocumented,
		"PageTemplate.PageNumbers": TypedBehaviorDocumented, "TableBlock.Columns": TypedBehaviorDocumented,
		"TableBlock.CellSpans": TypedBehaviorDocumented, "TableBlock.Header": TypedBehaviorDocumented,
		"TableBlock.Pagination": TypedBehaviorDocumented, "Text.CoreFonts": TypedBehaviorDocumented,
		"Text.UnicodeCoreFonts": TypedBehaviorUnsupported, "TextSegment.Link": TypedBehaviorDocumented,
		"TextSegment.Destination": TypedBehaviorDocumented,
		"ImageBlock":              TypedBehaviorDocumented, "HTML.SVG": TypedBehaviorDocumented,
		"HTML.Forms": TypedBehaviorUnsupported, "QRVerificationBlock": TypedBehaviorDocumented,
		"HeadingBlock.Level": TypedBehaviorAccidental,
	}
	valid := map[TypedBehaviorStatus]bool{TypedBehaviorDocumented: true, TypedBehaviorAccidental: true,
		TypedBehaviorDeprecated: true, TypedBehaviorUnsupported: true}
	for _, behavior := range TypedLayoutInventory().Behaviors {
		if !valid[behavior.Status] || behavior.Scope == "" || behavior.Name == "" {
			t.Fatalf("invalid behavior classification: %+v", behavior)
		}
		if expected, ok := want[behavior.Scope]; ok {
			if behavior.Status != expected {
				t.Fatalf("%s status = %q, want %q", behavior.Scope, behavior.Status, expected)
			}
			delete(want, behavior.Scope)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing Stage 0 behavior classifications: %v", want)
	}
}
