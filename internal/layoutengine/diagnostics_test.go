// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import "testing"

func TestDiagnosticValidationAcceptsStructuredEvidenceAndFix(t *testing.T) {
	diagnostic := testDiagnostic()
	if err := diagnostic.Validate(); err != nil {
		t.Fatalf("Validate() = %v", err)
	}

	diagnostic.Evidence = append(diagnostic.Evidence, diagnostic.Evidence[0])
	if err := diagnostic.Validate(); err == nil {
		t.Fatal("duplicate evidence unexpectedly validated")
	}
}

func TestDiagnosticValidationRejectsNonCanonicalContracts(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Diagnostic)
	}{
		{"code", func(d *Diagnostic) { d.Code = "not-canonical" }},
		{"severity", func(d *Diagnostic) { d.Severity = "fatal" }},
		{"stage", func(d *Diagnostic) { d.Stage = "browser" }},
		{"message", func(d *Diagnostic) { d.Message = " " }},
		{"region", func(d *Diagnostic) { d.Location.Region = "Body" }},
		{"fix", func(d *Diagnostic) { d.Fixes[0].Property = "" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			diagnostic := testDiagnostic()
			test.mutate(&diagnostic)
			if err := diagnostic.Validate(); err == nil {
				t.Fatal("Validate() unexpectedly succeeded")
			}
		})
	}
}

func TestDiagnosticValidationRejectsUnrepresentableBounds(t *testing.T) {
	diagnostic := testDiagnostic()
	diagnostic.Location.Bounds = Rect{X: MaxFixed, Width: 1, Height: 1}
	if err := diagnostic.Validate(); err == nil {
		t.Fatal("unrepresentable diagnostic bounds unexpectedly validated")
	}
}

func TestDiagnosticValidationRejectsBoundsWithoutPresenceFlag(t *testing.T) {
	diagnostic := testDiagnostic()
	diagnostic.Location.HasBounds = false
	if err := diagnostic.Validate(); err == nil {
		t.Fatal("bounds without has_bounds unexpectedly validated")
	}
}

func TestDiagnosticValidationRejectsInvalidUTF8(t *testing.T) {
	diagnostic := testDiagnostic()
	diagnostic.Message = string([]byte{0xff})
	if err := diagnostic.Validate(); err == nil {
		t.Fatal("invalid UTF-8 message unexpectedly validated")
	}
}

func testDiagnostic() Diagnostic {
	return Diagnostic{
		Code:     DiagnosticTrackMinOverflow,
		Severity: SeverityWarning,
		Stage:    StageLayout,
		Message:  "minimum tracks exceed the page width",
		Location: DiagnosticLocation{
			Node:      7,
			Key:       "@lines",
			Instance:  "@lines",
			Fragment:  1,
			Page:      1,
			Region:    RegionBody,
			Bounds:    Rect{X: 10, Y: 20, Width: 30, Height: 40},
			HasBounds: true,
		},
		Evidence: []DiagnosticEvidence{{Key: "overflow", Value: "12pt"}},
		Related: []DiagnosticReference{{
			Code:     DiagnosticKeepTooLarge,
			Location: DiagnosticLocation{Key: "@lines"},
		}},
		Fixes: []DiagnosticFix{{
			Kind:     FixSetProperty,
			Target:   "@lines",
			Property: "columns",
			Value:    "1fr 20mm",
		}},
	}
}
