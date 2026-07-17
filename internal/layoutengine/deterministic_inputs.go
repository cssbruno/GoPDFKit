// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"sort"
	"unicode"
)

const (
	MaxResourceCatalogEntries       = 1 << 18
	MaxResourceCatalogIdentityBytes = 64 << 20
)

// TextDataVersions identifies every versioned text dataset consulted by a
// planner. "none" is an explicit, stable value for a capability that is not
// installed; it must not be confused with an ambient host dependency.
type TextDataVersions struct {
	Unicode     string `json:"unicode"`
	CLDR        string `json:"cldr"`
	Hyphenation string `json:"hyphenation"`
}

// BuiltinTextDataVersions describes the data used by the current built-in
// core-font path. It uses Go's pinned Unicode tables and does not claim to use
// CLDR or hyphenation dictionaries.
func BuiltinTextDataVersions() TextDataVersions {
	return TextDataVersions{Unicode: unicode.Version, CLDR: "none", Hyphenation: "none"}
}

// ContentAddressedResource is one exact immutable planner resource. Digest is
// the lowercase SHA-256 of the bytes or canonical metrics payload actually
// used by planning, rather than a filename or mutable font-family name.
type ContentAddressedResource struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Digest string `json:"digest"`
}

// ResourceCatalogManifest is sorted canonical resource evidence. ID covers
// the complete ordered entry list and is therefore safe to use in PlanID.
type ResourceCatalogManifest struct {
	ID        string                     `json:"id"`
	Resources []ContentAddressedResource `json:"resources,omitempty"`
}

// NewResourceCatalogManifest validates, sorts, detaches, and hashes an exact
// resource set. The caller's order has no identity significance.
func NewResourceCatalogManifest(resources []ContentAddressedResource) (ResourceCatalogManifest, error) {
	if len(resources) > MaxResourceCatalogEntries {
		return ResourceCatalogManifest{}, fmt.Errorf("layoutengine: resource catalog exceeds %d entries", MaxResourceCatalogEntries)
	}
	canonical := append([]ContentAddressedResource(nil), resources...)
	identityBytes := 0
	for index, resource := range canonical {
		if err := validateTextIdentity("resource kind", resource.Kind); err != nil {
			return ResourceCatalogManifest{}, fmt.Errorf("resource %d: %w", index, err)
		}
		if err := validateTextIdentity("resource name", resource.Name); err != nil {
			return ResourceCatalogManifest{}, fmt.Errorf("resource %d: %w", index, err)
		}
		if err := validateSHA256("resource digest", resource.Digest); err != nil {
			return ResourceCatalogManifest{}, fmt.Errorf("resource %d: %w", index, err)
		}
		cost := len(resource.Kind) + len(resource.Name) + len(resource.Digest) + 24
		if identityBytes > MaxResourceCatalogIdentityBytes-cost {
			return ResourceCatalogManifest{}, fmt.Errorf("layoutengine: resource catalog identity exceeds %d bytes", MaxResourceCatalogIdentityBytes)
		}
		identityBytes += cost
	}
	sort.Slice(canonical, func(i, j int) bool {
		if canonical[i].Kind != canonical[j].Kind {
			return canonical[i].Kind < canonical[j].Kind
		}
		if canonical[i].Name != canonical[j].Name {
			return canonical[i].Name < canonical[j].Name
		}
		return canonical[i].Digest < canonical[j].Digest
	})
	for index := 1; index < len(canonical); index++ {
		if canonical[index].Kind == canonical[index-1].Kind && canonical[index].Name == canonical[index-1].Name {
			return ResourceCatalogManifest{}, fmt.Errorf("duplicate resource identity %q/%q", canonical[index].Kind, canonical[index].Name)
		}
	}
	digest := sha256.New()
	writePlanIdentityField(digest, "domain", "gopdfkit.resource-catalog.v1")
	for _, resource := range canonical {
		writePlanIdentityField(digest, "kind", resource.Kind)
		writePlanIdentityField(digest, "name", resource.Name)
		writePlanIdentityField(digest, "digest", resource.Digest)
	}
	return ResourceCatalogManifest{ID: hex.EncodeToString(digest.Sum(nil)), Resources: canonical}, nil
}

// PageProfileManifest content-addresses the physical page dimensions selected
// for layout. Width and Height use the engine's exact fixed-point point unit.
type PageProfileManifest struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Width  Fixed  `json:"width"`
	Height Fixed  `json:"height"`
}

// NewPageProfileManifest derives an identity from the canonical name and
// exact dimensions. A named profile therefore cannot silently change size.
func NewPageProfileManifest(name string, width, height Fixed) (PageProfileManifest, error) {
	if err := validateTextIdentity("page profile name", name); err != nil {
		return PageProfileManifest{}, err
	}
	size := Size{Width: width, Height: height}
	if err := size.Validate(); err != nil || size.IsEmpty() {
		return PageProfileManifest{}, errors.New("layoutengine: page profile dimensions must be positive and valid")
	}
	digest := sha256.New()
	writePlanIdentityField(digest, "domain", "gopdfkit.page-profile.v1")
	writePlanIdentityField(digest, "name", name)
	writePlanIdentityFixed(digest, "width", width)
	writePlanIdentityFixed(digest, "height", height)
	return PageProfileManifest{ID: hex.EncodeToString(digest.Sum(nil)), Name: name, Width: width, Height: height}, nil
}

func writePlanIdentityFixed(destination hash.Hash, name string, value Fixed) {
	writePlanIdentityField(destination, name, fmt.Sprintf("%d", value))
}

// DeterministicInputManifest is the inspectable canonical environment bound
// to one plan. PlanID is recomputed during validation, preventing a manifest
// from claiming inputs that its identity does not cover.
type DeterministicInputManifest struct {
	PlanID               string                  `json:"plan_id"`
	SemanticTemplateID   string                  `json:"semantic_template_id"`
	ScenarioRevisionID   string                  `json:"scenario_revision_id"`
	ResourceCatalog      ResourceCatalogManifest `json:"resource_catalog"`
	Locale               string                  `json:"locale"`
	Timezone             string                  `json:"timezone"`
	TextData             TextDataVersions        `json:"text_data"`
	CompatibilityProfile string                  `json:"compatibility_profile"`
	CompatibilityFlags   []string                `json:"compatibility_flags,omitempty"`
	PageProfile          PageProfileManifest     `json:"page_profile"`
	PlannerVersion       string                  `json:"planner_version"`
}

// NewDeterministicInputManifest constructs canonical, detached plan inputs.
func NewDeterministicInputManifest(template SemanticTemplateID, scenario ScenarioRevisionID,
	resources ResourceCatalogManifest, locale, timezone string, textData TextDataVersions,
	compatibilityProfile string, compatibilityFlags []string, page PageProfileManifest,
	plannerVersion string) (DeterministicInputManifest, error) {
	manifest := DeterministicInputManifest{
		SemanticTemplateID: template.String(), ScenarioRevisionID: scenario.String(),
		ResourceCatalog: cloneResourceCatalog(resources), Locale: locale, Timezone: timezone,
		TextData: textData, CompatibilityProfile: compatibilityProfile,
		CompatibilityFlags: append([]string(nil), compatibilityFlags...), PageProfile: page,
		PlannerVersion: plannerVersion,
	}
	planID, err := manifest.derivePlanID()
	if err != nil {
		return DeterministicInputManifest{}, err
	}
	manifest.PlanID = planID.String()
	sort.Strings(manifest.CompatibilityFlags)
	return manifest, nil
}

func (manifest DeterministicInputManifest) derivePlanID() (PlanID, error) {
	template, err := ParseSemanticTemplateID(manifest.SemanticTemplateID)
	if err != nil {
		return PlanID{}, err
	}
	scenario, err := ParseScenarioRevisionID(manifest.ScenarioRevisionID)
	if err != nil {
		return PlanID{}, err
	}
	resources, err := ParseResourceCatalogID(manifest.ResourceCatalog.ID)
	if err != nil {
		return PlanID{}, err
	}
	return DerivePlanID(PlanIdentityInputs{
		Template: template, Scenario: scenario, Resources: resources,
		Locale: manifest.Locale, Timezone: manifest.Timezone,
		UnicodeVersion: manifest.TextData.Unicode, CLDRVersion: manifest.TextData.CLDR,
		HyphenationVersion:   manifest.TextData.Hyphenation,
		CompatibilityProfile: manifest.CompatibilityProfile,
		PageProfile:          manifest.PageProfile.ID, PlannerVersion: manifest.PlannerVersion,
		CompatibilityFlags: manifest.CompatibilityFlags,
	})
}

func (manifest DeterministicInputManifest) validate() error {
	canonicalResources, err := NewResourceCatalogManifest(manifest.ResourceCatalog.Resources)
	if err != nil || canonicalResources.ID != manifest.ResourceCatalog.ID {
		return errors.New("layoutengine: deterministic resource catalog is not canonical")
	}
	canonicalPage, err := NewPageProfileManifest(manifest.PageProfile.Name, manifest.PageProfile.Width, manifest.PageProfile.Height)
	if err != nil || canonicalPage.ID != manifest.PageProfile.ID {
		return errors.New("layoutengine: deterministic page profile is not canonical")
	}
	if manifest.PlannerVersion != PlannerVersion {
		return errors.New("layoutengine: deterministic planner version does not match this planner")
	}
	derived, err := manifest.derivePlanID()
	if err != nil || derived.String() != manifest.PlanID {
		return errors.New("layoutengine: deterministic PlanID does not cover its manifest")
	}
	flags := append([]string(nil), manifest.CompatibilityFlags...)
	sort.Strings(flags)
	for index := range flags {
		if flags[index] != manifest.CompatibilityFlags[index] {
			return errors.New("layoutengine: deterministic compatibility flags are not sorted")
		}
	}
	return nil
}

func cloneResourceCatalog(catalog ResourceCatalogManifest) ResourceCatalogManifest {
	catalog.Resources = append([]ContentAddressedResource(nil), catalog.Resources...)
	return catalog
}

func cloneDeterministicInputs(manifest DeterministicInputManifest) DeterministicInputManifest {
	manifest.ResourceCatalog = cloneResourceCatalog(manifest.ResourceCatalog)
	manifest.CompatibilityFlags = append([]string(nil), manifest.CompatibilityFlags...)
	return manifest
}

func validateSHA256(name, value string) error {
	if len(value) != sha256.Size*2 {
		return fmt.Errorf("%s is not a lowercase SHA-256 digest", name)
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || hex.EncodeToString(decoded) != value {
		return fmt.Errorf("%s is not a lowercase SHA-256 digest", name)
	}
	allZero := true
	for _, b := range decoded {
		allZero = allZero && b == 0
	}
	if allZero {
		return fmt.Errorf("%s is zero", name)
	}
	return nil
}

// ResourceCatalogFromPlan derives content identities from the resources the
// painter will consume. Core-font entries cover exact canonical metrics;
// image entries cover exact encoded bytes through their existing digest.
func ResourceCatalogFromPlan(plan LayoutPlan) (ResourceCatalogManifest, error) {
	projection := plan.Projection()
	return resourceCatalogFromProjection(projection.Fonts, projection.ImageResources)
}

func resourceCatalogFromProjection(fonts []CoreFontResource, images []ImageResource) (ResourceCatalogManifest, error) {
	resources := make([]ContentAddressedResource, 0, len(fonts)+len(images))
	for _, font := range fonts {
		if font.EmbeddedUTF8 != nil {
			resources = append(resources, ContentAddressedResource{Kind: "embedded-utf8-font", Name: font.EmbeddedUTF8.Name, Digest: string(font.EmbeddedUTF8.Digest)})
		} else {
			resources = append(resources, ContentAddressedResource{Kind: "core-font-metrics", Name: string(font.Face), Digest: string(font.MetricsDigest)})
		}
	}
	for _, image := range images {
		resources = append(resources, ContentAddressedResource{Kind: "image-" + string(image.Format), Name: string(image.Digest), Digest: string(image.Digest)})
	}
	return NewResourceCatalogManifest(resources)
}

// WithDeterministicInputs returns a detached plan whose canonical projection
// and hash include the complete validated input manifest.
func (plan LayoutPlan) WithDeterministicInputs(manifest DeterministicInputManifest) (LayoutPlan, error) {
	if err := plan.Validate(); err != nil {
		return LayoutPlan{}, err
	}
	if err := manifest.validate(); err != nil {
		return LayoutPlan{}, err
	}
	catalog, err := resourceCatalogFromProjection(plan.fonts, plan.imageResources)
	if err != nil || catalog.ID != manifest.ResourceCatalog.ID {
		return LayoutPlan{}, errors.New("layoutengine: deterministic resource catalog does not match exact plan resources")
	}
	result := plan
	result.deterministicInputs = cloneDeterministicInputs(manifest)
	result.hasDeterministicInputs = true
	if err := result.Validate(); err != nil {
		return LayoutPlan{}, err
	}
	return result, nil
}

func rebindDeterministicResources(plan LayoutPlan, prior *DeterministicInputManifest) (LayoutPlan, error) {
	if prior == nil {
		return plan, nil
	}
	template, err := ParseSemanticTemplateID(prior.SemanticTemplateID)
	if err != nil {
		return LayoutPlan{}, err
	}
	scenario, err := ParseScenarioRevisionID(prior.ScenarioRevisionID)
	if err != nil {
		return LayoutPlan{}, err
	}
	resources, err := ResourceCatalogFromPlan(plan)
	if err != nil {
		return LayoutPlan{}, err
	}
	manifest, err := NewDeterministicInputManifest(template, scenario, resources, prior.Locale, prior.Timezone, prior.TextData,
		prior.CompatibilityProfile, prior.CompatibilityFlags, prior.PageProfile, prior.PlannerVersion)
	if err != nil {
		return LayoutPlan{}, err
	}
	return plan.WithDeterministicInputs(manifest)
}

// DeterministicInputs returns a detached input manifest when one was bound.
func (plan LayoutPlan) DeterministicInputs() (DeterministicInputManifest, bool) {
	if !plan.hasDeterministicInputs {
		return DeterministicInputManifest{}, false
	}
	return cloneDeterministicInputs(plan.deterministicInputs), true
}
