// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

// Package paperorg defines a deterministic organization-governance catalog.
// It stores public identities only: source, fixtures, credentials, signing
// material, and artifact bytes remain in their owning systems.
package paperorg

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

const CatalogSchemaVersion uint16 = 1

var (
	ErrInvalidCatalog   = errors.New("paperorg: invalid governance catalog")
	ErrGovernanceDenied = errors.New("paperorg: governance authorization denied")
)

type PackageRelease struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Digest  string `json:"digest"`
}

type ResourceApproval struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Digest string `json:"digest"`
}

type LibraryEntry struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Digest  string `json:"digest"`
}

type ComplianceProfile string

const (
	CompliancePDFA4  ComplianceProfile = "pdfa-4"
	CompliancePDFUA2 ComplianceProfile = "pdfua-2"
)

type TeamRelease struct {
	TeamID                string              `json:"team_id"`
	Revision              string              `json:"revision"`
	Theme                 PackageRelease      `json:"theme"`
	Policy                PackageRelease      `json:"policy"`
	ApprovedResources     []ResourceApproval  `json:"approved_resources"`
	ScenarioLibrary       []LibraryEntry      `json:"scenario_library"`
	VisualBaselineLibrary []LibraryEntry      `json:"visual_baseline_library"`
	ComplianceProfiles    []ComplianceProfile `json:"compliance_profiles"`
}

type Catalog struct {
	SchemaVersion uint16        `json:"schema_version"`
	Organization  string        `json:"organization"`
	Revision      string        `json:"revision"`
	Teams         []TeamRelease `json:"teams"`
}

type ReleaseRequest struct {
	TeamID             string
	TeamRevision       string
	Theme              PackageRelease
	Policy             PackageRelease
	Resources          []ResourceApproval
	Scenarios          []LibraryEntry
	VisualBaselines    []LibraryEntry
	ComplianceProfiles []ComplianceProfile
}

type AuthorizationReceipt struct {
	CatalogDigest   string `json:"catalog_digest"`
	Organization    string `json:"organization"`
	CatalogRevision string `json:"catalog_revision"`
	TeamID          string `json:"team_id"`
	TeamRevision    string `json:"team_revision"`
	ThemeDigest     string `json:"theme_digest"`
	PolicyDigest    string `json:"policy_digest"`
}

func (catalog Catalog) Validate() error {
	if catalog.SchemaVersion != CatalogSchemaVersion || !validLabel(catalog.Organization) || !validLabel(catalog.Revision) || catalog.Teams == nil {
		return ErrInvalidCatalog
	}
	previous := ""
	for index, team := range catalog.Teams {
		if !validLabel(team.TeamID) || !validLabel(team.Revision) || index > 0 && team.TeamID <= previous ||
			!validPackage(team.Theme) || !validPackage(team.Policy) || team.ApprovedResources == nil || team.ScenarioLibrary == nil ||
			team.VisualBaselineLibrary == nil || team.ComplianceProfiles == nil ||
			!sortedUniqueResources(team.ApprovedResources) || !sortedUniqueLibrary(team.ScenarioLibrary) ||
			!sortedUniqueLibrary(team.VisualBaselineLibrary) || !sortedUniqueProfiles(team.ComplianceProfiles) {
			return fmt.Errorf("%w: teams[%d]", ErrInvalidCatalog, index)
		}
		previous = team.TeamID
	}
	return nil
}

func (catalog Catalog) CanonicalJSON() ([]byte, error) {
	if err := catalog.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(catalog)
}

func (catalog Catalog) Digest() (string, error) {
	encoded, err := catalog.CanonicalJSON()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

// AuthorizeRelease verifies an exact team release against canonical approved
// sets. Requests are sets: their order and duplicate spelling cannot change a
// decision or receipt.
func (catalog Catalog) AuthorizeRelease(request ReleaseRequest) (AuthorizationReceipt, error) {
	if err := catalog.Validate(); err != nil {
		return AuthorizationReceipt{}, err
	}
	index := sort.Search(len(catalog.Teams), func(index int) bool { return catalog.Teams[index].TeamID >= request.TeamID })
	if index == len(catalog.Teams) || catalog.Teams[index].TeamID != request.TeamID {
		return AuthorizationReceipt{}, ErrGovernanceDenied
	}
	team := catalog.Teams[index]
	if request.TeamRevision != team.Revision || request.Theme != team.Theme || request.Policy != team.Policy ||
		!resourceSubset(request.Resources, team.ApprovedResources) || !librarySubset(request.Scenarios, team.ScenarioLibrary) ||
		!librarySubset(request.VisualBaselines, team.VisualBaselineLibrary) || !profileSubset(request.ComplianceProfiles, team.ComplianceProfiles) {
		return AuthorizationReceipt{}, ErrGovernanceDenied
	}
	digest, err := catalog.Digest()
	if err != nil {
		return AuthorizationReceipt{}, err
	}
	return AuthorizationReceipt{CatalogDigest: digest, Organization: catalog.Organization, CatalogRevision: catalog.Revision,
		TeamID: team.TeamID, TeamRevision: team.Revision, ThemeDigest: team.Theme.Digest, PolicyDigest: team.Policy.Digest}, nil
}

func validLabel(value string) bool {
	if value == "" || len(value) > 256 || !utf8.ValidString(value) || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if character <= 0x20 || character > 0x7e || strings.ContainsRune("/?#", character) {
			return false
		}
	}
	return true
}

func validDigest(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validPackage(value PackageRelease) bool {
	return validLabel(value.Name) && validLabel(value.Version) && validDigest(value.Digest)
}

func resourceKey(value ResourceApproval) string { return value.Kind + "\x00" + value.Name }
func libraryKey(value LibraryEntry) string      { return value.Name + "\x00" + value.Version }

func sortedUniqueResources(values []ResourceApproval) bool {
	previous := ""
	for index, value := range values {
		key := resourceKey(value)
		if !validLabel(value.Kind) || !validLabel(value.Name) || !validDigest(value.Digest) || index > 0 && key <= previous {
			return false
		}
		previous = key
	}
	return true
}

func sortedUniqueLibrary(values []LibraryEntry) bool {
	previous := ""
	for index, value := range values {
		key := libraryKey(value)
		if !validLabel(value.Name) || !validLabel(value.Version) || !validDigest(value.Digest) || index > 0 && key <= previous {
			return false
		}
		previous = key
	}
	return true
}

func sortedUniqueProfiles(values []ComplianceProfile) bool {
	previous := ComplianceProfile("")
	for index, value := range values {
		if value != CompliancePDFA4 && value != CompliancePDFUA2 || index > 0 && value <= previous {
			return false
		}
		previous = value
	}
	return true
}

func resourceSubset(request, approved []ResourceApproval) bool {
	return setSubset(request, approved, resourceKey, func(value ResourceApproval) bool {
		return validLabel(value.Kind) && validLabel(value.Name) && validDigest(value.Digest)
	})
}

func librarySubset(request, approved []LibraryEntry) bool {
	return setSubset(request, approved, libraryKey, func(value LibraryEntry) bool {
		return validLabel(value.Name) && validLabel(value.Version) && validDigest(value.Digest)
	})
}

func setSubset[T comparable](request, approved []T, key func(T) string, valid func(T) bool) bool {
	allowed := make(map[string]T, len(approved))
	for _, value := range approved {
		allowed[key(value)] = value
	}
	seen := make(map[string]struct{}, len(request))
	for _, value := range request {
		identity := key(value)
		if !valid(value) {
			return false
		}
		if _, duplicate := seen[identity]; duplicate {
			continue
		}
		seen[identity] = struct{}{}
		if approvedValue, ok := allowed[identity]; !ok || approvedValue != value {
			return false
		}
	}
	return true
}

func profileSubset(request, approved []ComplianceProfile) bool {
	allowed := make(map[ComplianceProfile]struct{}, len(approved))
	for _, value := range approved {
		allowed[value] = struct{}{}
	}
	for _, value := range request {
		if _, ok := allowed[value]; !ok {
			return false
		}
	}
	return true
}
