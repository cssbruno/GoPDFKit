// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"bytes"
	"errors"
	"testing"
)

func TestDiagnosticCodecRoundTripsCanonicallyAndDetached(t *testing.T) {
	diagnostic := testDiagnostic()
	diagnostic.Evidence = []DiagnosticEvidence{{Key: "z", Value: "last"}, {Key: "a", Value: "first"}}
	encoded, err := EncodeDiagnosticSet([]Diagnostic{diagnostic}, DiagnosticCodecLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Index(encoded, []byte(`"key":"a"`)) > bytes.Index(encoded, []byte(`"key":"z"`)) {
		t.Fatalf("evidence is not canonical: %s", encoded)
	}
	decoded, err := DecodeDiagnosticSet(encoded, DiagnosticCodecLimits{})
	if err != nil || len(decoded.Diagnostics) != 1 {
		t.Fatalf("DecodeDiagnosticSet() = %#v, %v", decoded, err)
	}
	decoded.Diagnostics[0].Evidence[0].Value = "mutated"
	again, err := DecodeDiagnosticSet(encoded, DiagnosticCodecLimits{})
	if err != nil || again.Diagnostics[0].Evidence[0].Value != "first" {
		t.Fatal("decoded diagnostics alias caller-owned state")
	}
	reencoded, err := EncodeDiagnosticSet(again.Diagnostics, DiagnosticCodecLimits{})
	if err != nil || !bytes.Equal(encoded, reencoded) {
		t.Fatalf("canonical round trip changed bytes: %s != %s (%v)", encoded, reencoded, err)
	}
}

func TestDiagnosticCodecRejectsUnknownTrailingInvalidAndLimits(t *testing.T) {
	valid, err := EncodeDiagnosticSet([]Diagnostic{testDiagnostic()}, DiagnosticCodecLimits{})
	if err != nil {
		t.Fatal(err)
	}
	unknown := bytes.Replace(valid, []byte(`"diagnostics"`), []byte(`"unknown":1,"diagnostics"`), 1)
	if _, err := DecodeDiagnosticSet(unknown, DiagnosticCodecLimits{}); err == nil {
		t.Fatal("unknown field unexpectedly decoded")
	}
	if _, err := DecodeDiagnosticSet(append(valid, []byte(` {}`)...), DiagnosticCodecLimits{}); err == nil {
		t.Fatal("trailing JSON unexpectedly decoded")
	}
	if _, err := DecodeDiagnosticSet(valid, DiagnosticCodecLimits{MaxDiagnostics: 1, MaxBytes: 8}); !errors.Is(err, ErrDiagnosticCodecLimit) {
		t.Fatalf("byte limit error = %v", err)
	}
	bad := testDiagnostic()
	bad.Code = "bad"
	if _, err := EncodeDiagnosticSet([]Diagnostic{bad}, DiagnosticCodecLimits{}); err == nil {
		t.Fatal("invalid diagnostic unexpectedly encoded")
	}
}
