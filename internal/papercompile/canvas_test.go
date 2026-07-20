package papercompile

import (
	"testing"

	"github.com/cssbruno/paperrune/internal/paperlang"
	"github.com/cssbruno/paperrune/layout"
)

func TestCompileCanvasAnchorsPreserveReadableConstraints(t *testing.T) {
	source := "document @d:\n  page @p:\n    size: \"A4\"\n    body @b:\n      canvas @diagram:\n        width: 160pt\n        height: 80pt\n        anchor @base:\n          width: 40pt\n          height: 20pt\n          left: \"canvas.left + 8pt\"\n          top: \"canvas.top + 8pt\"\n          background: \"#336699\"\n        anchor @badge:\n          width: 24pt\n          height: 12pt\n          left: \"@base.right + 6pt\"\n          top: \"@base.top\"\n          alt: \"Status badge\"\n"
	parsed := paperlang.Parse("canvas.paper", source)
	if !parsed.OK() {
		t.Fatalf("parse diagnostics = %#v", parsed.Diagnostics)
	}
	compiled := Compile(parsed.AST)
	if !compiled.OK() {
		t.Fatalf("compile diagnostics = %#v", compiled.Diagnostics)
	}
	canvas, ok := compiled.Document.Body[0].(layout.CanvasBlock)
	if !ok || len(canvas.Items) != 2 || canvas.Items[1].Constraints[0].Target != "@base" || canvas.Items[1].Constraints[0].Offset != 6 {
		t.Fatalf("canvas = %#v", compiled.Document.Body[0])
	}
	if compiled.Mapping.Nodes[len(compiled.Mapping.Nodes)-1].SegmentIndex != 1 {
		t.Fatalf("mapping = %#v", compiled.Mapping.Nodes)
	}
}
