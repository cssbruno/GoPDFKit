// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"reflect"
	"strings"
	"testing"
)

func TestSourceNodeIDRequiresReadableSlug(t *testing.T) {
	id, err := NewSourceNodeID("@Invoice_lines-2")
	if err != nil || !id.Valid() || id.String() != "@Invoice_lines-2" {
		t.Fatalf("NewSourceNodeID() = %q, %v", id, err)
	}
	for _, value := range []string{"", "invoice", "@2invoice", "@invoice/lines", "@invoice lines"} {
		if _, err := NewSourceNodeID(value); err == nil {
			t.Fatalf("NewSourceNodeID(%q) unexpectedly succeeded", value)
		}
	}
	if (SourceNodeID{}).Valid() {
		t.Fatal("zero SourceNodeID must be absent")
	}
}

func TestRegionIDRequiresCanonicalSlug(t *testing.T) {
	if !RegionBody.Valid() || RegionBody != RegionID("body") {
		t.Fatalf("RegionBody = %q, want body", RegionBody)
	}
	region, err := NewRegionID("sidebar-2")
	if err != nil || region != "sidebar-2" {
		t.Fatalf("NewRegionID() = %q, %v", region, err)
	}
	for _, value := range []string{"", "Body", "2body", "body region", "body/aside"} {
		if _, err := NewRegionID(value); err == nil {
			t.Fatalf("NewRegionID(%q) unexpectedly succeeded", value)
		}
	}
}

func TestDigestIdentitiesValidateCanonicalHexAndRemainDistinct(t *testing.T) {
	digest := strings.Repeat("ab", 32)
	source, err := ParseSourceRevisionID(digest)
	if err != nil || !source.Valid() || source.String() != digest {
		t.Fatalf("ParseSourceRevisionID() = %q, %v", source, err)
	}
	semantic, err := ParseSemanticTemplateID(digest)
	if err != nil || semantic.String() != digest {
		t.Fatalf("ParseSemanticTemplateID() = %q, %v", semantic, err)
	}
	if reflect.TypeOf(source) == reflect.TypeOf(semantic) {
		t.Fatal("source and semantic revision identities share a Go type")
	}

	parsers := []func(string) error{
		func(value string) error { _, err := ParseSourceRevisionID(value); return err },
		func(value string) error { _, err := ParseSemanticTemplateID(value); return err },
		func(value string) error { _, err := ParseScenarioRevisionID(value); return err },
		func(value string) error { _, err := ParsePolicyRevisionID(value); return err },
		func(value string) error { _, err := ParsePlanID(value); return err },
		func(value string) error { _, err := ParseRenderID(value); return err },
	}
	for _, parser := range parsers {
		if err := parser(digest); err != nil {
			t.Fatalf("parse canonical digest = %v", err)
		}
		for _, invalid := range []string{digest[:62], strings.ToUpper(digest), strings.Repeat("00", 32), strings.Repeat("gg", 32)} {
			if err := parser(invalid); err == nil {
				t.Fatalf("parse digest %q unexpectedly succeeded", invalid)
			}
		}
	}
}

func TestIdentityKindsRemainDistinctAndValidated(t *testing.T) {
	key, err := NewNodeKey("@invoice-lines")
	if err != nil || key != NodeKey("@invoice-lines") {
		t.Fatalf("NewNodeKey() = %q, %v", key, err)
	}
	instance, err := NewInstanceID(`@invoice-lines/row[key="SKU-187"]`)
	if err != nil || !instance.Valid() {
		t.Fatalf("NewInstanceID() = %q, %v", instance, err)
	}
	if NodeID(0).Valid() || FragmentID(0).Valid() {
		t.Fatal("zero numeric identities must be absent")
	}
	if !NodeID(1).Valid() || !FragmentID(1).Valid() {
		t.Fatal("non-zero numeric identities must be valid")
	}
	for _, value := range []string{"", " key", "key\n"} {
		if _, err := NewNodeKey(value); err == nil {
			t.Fatalf("NewNodeKey(%q) unexpectedly succeeded", value)
		}
	}
}

func TestSourceSpanValidation(t *testing.T) {
	span := SourceSpan{
		File:  "invoice.paper",
		Start: SourcePosition{Offset: 12, Line: 2, Column: 3},
		End:   SourcePosition{Offset: 19, Line: 2, Column: 10},
	}
	if err := span.Validate(); err != nil {
		t.Fatalf("Validate() = %v", err)
	}
	span.End.Offset = 11
	if err := span.Validate(); err == nil {
		t.Fatal("reversed source span unexpectedly validated")
	}
	if err := (SourceSpan{}).Validate(); err != nil {
		t.Fatalf("zero SourceSpan.Validate() = %v", err)
	}
}
