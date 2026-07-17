// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package paperlang

import "testing"

func TestASTProjectionPinsGrammarAndSchemaVersions(t *testing.T) {
	parsed := Parse("version.paper", "document:\n  page:\n    body:\n      paragraph:\n        text: \"versioned\"\n")
	if !parsed.OK() {
		t.Fatalf("Parse() diagnostics = %+v", parsed.Diagnostics)
	}
	projection := parsed.AST.Projection()
	if projection.SchemaVersion != ASTSchemaVersion || projection.GrammarVersion != GrammarVersion {
		t.Fatalf("Projection() versions = schema %d grammar %q", projection.SchemaVersion, projection.GrammarVersion)
	}
	if GrammarVersion == "" || ASTSchemaVersion == 0 {
		t.Fatal("language versions must be non-zero pinned constants")
	}
}
