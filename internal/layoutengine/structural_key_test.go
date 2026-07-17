// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import "testing"

func TestDeriveAnonymousStructuralKeyIsDeterministicAndRevisionScoped(t *testing.T) {
	revision := mustSourceRevisionID(t, "11")
	input := AnonymousStructuralKeyInput{
		Revision: revision, Parent: NodeKey("document/body"), Kind: "paragraph", Ordinal: 2,
		Fingerprint: repeatedDigest("22"),
	}
	first, err := DeriveAnonymousStructuralKey(input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := DeriveAnonymousStructuralKey(input)
	if err != nil || first != second || len(first) != len("anon/")+64 {
		t.Fatalf("keys = %q, %q, %v", first, second, err)
	}
	changes := []AnonymousStructuralKeyInput{input, input, input, input, input}
	changes[0].Revision = mustSourceRevisionID(t, "33")
	changes[1].Parent = "document/header"
	changes[2].Kind = "heading"
	changes[3].Ordinal++
	changes[4].Fingerprint = repeatedDigest("44")
	for index, changed := range changes {
		key, err := DeriveAnonymousStructuralKey(changed)
		if err != nil || key == first {
			t.Fatalf("change %d key = %q, %v", index, key, err)
		}
	}
}

func TestDeriveAnonymousStructuralKeyRejectsInvalidInputs(t *testing.T) {
	valid := AnonymousStructuralKeyInput{Revision: mustSourceRevisionID(t, "11"), Kind: "text", Fingerprint: repeatedDigest("22")}
	tests := []AnonymousStructuralKeyInput{valid, valid, valid, valid}
	tests[0].Revision = SourceRevisionID{}
	tests[1].Kind = "Text"
	tests[2].Parent = " bad"
	tests[3].Fingerprint = "not-a-digest"
	for index, input := range tests {
		if key, err := DeriveAnonymousStructuralKey(input); err == nil || key != "" {
			t.Fatalf("input %d = (%q, %v)", index, key, err)
		}
	}
}

func mustSourceRevisionID(t *testing.T, pair string) SourceRevisionID {
	t.Helper()
	id, err := ParseSourceRevisionID(repeatedDigest(pair))
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func repeatedDigest(pair string) string {
	value := ""
	for len(value) < 64 {
		value += pair
	}
	return value[:64]
}
