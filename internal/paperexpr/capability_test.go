// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperexpr

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestExpressionAndControlFlowRuntimeHasNoAmbientAuthority is deliberately
// repository-grounded. It audits every production source file in paperexpr and
// each compiler file that evaluates expressions/control flow. A new ambient
// capability therefore fails in CI at the import that introduced it.
func TestExpressionAndControlFlowRuntimeHasNoAmbientAuthority(t *testing.T) {
	t.Parallel()
	forbidden := map[string]string{
		"os": "filesystem/environment", "io/fs": "filesystem", "path/filepath": "filesystem",
		"net": "network", "net/http": "network", "net/url": "network", "os/exec": "process",
		"reflect": "reflection", "plugin": "runtime module loading", "unsafe": "unsafe runtime access",
		"syscall": "host process", "time": "ambient time", "math/rand": "randomness", "crypto/rand": "randomness",
	}
	allowed := map[string]bool{
		"context": true, "encoding/json": true, "errors": true, "fmt": true, "math": true,
		"sort": true, "strconv": true, "strings": true, "unicode/utf8": true,
		"github.com/cssbruno/paperrune/internal/paperexpr":     true,
		"github.com/cssbruno/paperrune/internal/paperlang":     true,
		"github.com/cssbruno/paperrune/internal/paperrepeat":   true,
		"github.com/cssbruno/paperrune/internal/paperscenario": true,
	}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		auditCapabilitySource(t, file, forbidden, allowed)
	}
	for _, file := range []string{"../papercompile/scenario_loop.go", "../papercompile/scenario_repeat.go", "../papercompile/conditions.go"} {
		auditCapabilitySource(t, file, forbidden, allowed)
	}
}

func auditCapabilitySource(t *testing.T, file string, forbidden map[string]string, allowed map[string]bool) {
	t.Helper()
	source, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(source), "//go:linkname") || strings.Contains(string(source), `import "C"`) {
		t.Fatalf("%s introduces an external runtime capability", file)
	}
	parsed, err := parser.ParseFile(token.NewFileSet(), file, source, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	for _, imported := range parsed.Imports {
		path, err := strconv.Unquote(imported.Path.Value)
		if err != nil {
			t.Fatal(err)
		}
		if capability, denied := forbidden[path]; denied {
			t.Fatalf("%s imports %q and exposes %s capability", file, path, capability)
		}
		if !allowed[path] {
			t.Fatalf("%s imports non-whitelisted runtime dependency %q; audit it before extending authority", file, path)
		}
	}
	// The parser is intentionally used rather than textual import matching so
	// aliases and grouped imports cannot bypass this audit.
}
