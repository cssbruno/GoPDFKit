// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperorg

import (
	"errors"
	"strings"
	"testing"
)

func hash(character string) string { return strings.Repeat(character, 64) }

func validCatalog() Catalog {
	return Catalog{SchemaVersion: CatalogSchemaVersion, Organization: "harbor-health", Revision: "governance-v7", Teams: []TeamRelease{{
		TeamID: "clinical-docs", Revision: "release-42",
		Theme:                 PackageRelease{Name: "harbor-theme", Version: "v3.2.0", Digest: hash("a")},
		Policy:                PackageRelease{Name: "clinical-policy", Version: "v7.0.1", Digest: hash("b")},
		ApprovedResources:     []ResourceApproval{{Kind: "font", Name: "inter-regular", Digest: hash("c")}, {Kind: "image", Name: "clinic-mark", Digest: hash("d")}},
		ScenarioLibrary:       []LibraryEntry{{Name: "prescription-matrix", Version: "v2", Digest: hash("e")}},
		VisualBaselineLibrary: []LibraryEntry{{Name: "prescription-a4", Version: "v5", Digest: hash("f")}},
		ComplianceProfiles:    []ComplianceProfile{CompliancePDFA4, CompliancePDFUA2},
	}}}
}

func TestCatalogCanonicalIdentityAndExactGovernanceAuthorization(t *testing.T) {
	catalog := validCatalog()
	first, err := catalog.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	second, _ := catalog.CanonicalJSON()
	if string(first) != string(second) || !strings.Contains(string(first), `"clinical-docs"`) {
		t.Fatalf("canonical catalog = %s", first)
	}
	team := catalog.Teams[0]
	request := ReleaseRequest{TeamID: team.TeamID, TeamRevision: team.Revision, Theme: team.Theme, Policy: team.Policy,
		Resources: []ResourceApproval{team.ApprovedResources[1], team.ApprovedResources[0]},
		Scenarios: team.ScenarioLibrary, VisualBaselines: team.VisualBaselineLibrary,
		ComplianceProfiles: []ComplianceProfile{CompliancePDFUA2, CompliancePDFA4}}
	receipt, err := catalog.AuthorizeRelease(request)
	if err != nil || receipt.CatalogDigest == "" || receipt.ThemeDigest != hash("a") || receipt.PolicyDigest != hash("b") {
		t.Fatalf("authorization = %+v, %v", receipt, err)
	}
	again, err := catalog.AuthorizeRelease(request)
	if err != nil || again != receipt {
		t.Fatalf("repeated authorization = %+v, %v", again, err)
	}
}

func TestCatalogRejectsNoncanonicalAndUnapprovedReleaseInputs(t *testing.T) {
	invalid := validCatalog()
	invalid.Teams[0].ApprovedResources[0], invalid.Teams[0].ApprovedResources[1] = invalid.Teams[0].ApprovedResources[1], invalid.Teams[0].ApprovedResources[0]
	if err := invalid.Validate(); !errors.Is(err, ErrInvalidCatalog) {
		t.Fatalf("unordered catalog = %v", err)
	}
	catalog := validCatalog()
	team := catalog.Teams[0]
	base := ReleaseRequest{TeamID: team.TeamID, TeamRevision: team.Revision, Theme: team.Theme, Policy: team.Policy,
		Resources: team.ApprovedResources, Scenarios: team.ScenarioLibrary, VisualBaselines: team.VisualBaselineLibrary,
		ComplianceProfiles: team.ComplianceProfiles}
	tests := []struct {
		name   string
		mutate func(*ReleaseRequest)
	}{
		{"team revision", func(value *ReleaseRequest) { value.TeamRevision = "stale" }},
		{"theme", func(value *ReleaseRequest) { value.Theme.Digest = hash("9") }},
		{"policy", func(value *ReleaseRequest) { value.Policy.Version = "v8" }},
		{"resource", func(value *ReleaseRequest) {
			value.Resources = []ResourceApproval{{Kind: "font", Name: "unapproved", Digest: hash("8")}}
		}},
		{"scenario", func(value *ReleaseRequest) {
			value.Scenarios = []LibraryEntry{{Name: "other", Version: "v1", Digest: hash("7")}}
		}},
		{"baseline", func(value *ReleaseRequest) {
			value.VisualBaselines = []LibraryEntry{{Name: "other", Version: "v1", Digest: hash("6")}}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := base
			request.Resources = append([]ResourceApproval(nil), base.Resources...)
			test.mutate(&request)
			if receipt, err := catalog.AuthorizeRelease(request); !errors.Is(err, ErrGovernanceDenied) || receipt != (AuthorizationReceipt{}) {
				t.Fatalf("AuthorizeRelease() = %+v, %v", receipt, err)
			}
		})
	}
}
