// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"sort"
	"strings"
)

const planIdentityDomain = "gopdfkit.plan-id.v1"

const MaxPlanIdentityCompatibilityFlags = 1024

var ErrPlanIdentityInputs = errors.New("layoutengine: plan identity inputs are invalid")
var ErrRenderIdentityInputs = errors.New("layoutengine: render identity inputs are invalid")

// PlanIdentityInputs are every non-layout-plan input that can change planned
// geometry or visible content. The resulting PlanID is an internal cache and
// evidence identity; public tools expose opaque scoped handles instead of this
// potentially sensitive digest.
type PlanIdentityInputs struct {
	Template             SemanticTemplateID
	Scenario             ScenarioRevisionID
	Resources            ResourceCatalogID
	Locale               string
	Timezone             string
	UnicodeVersion       string
	CLDRVersion          string
	HyphenationVersion   string
	CompatibilityProfile string
	PageProfile          string
	PlannerVersion       string
	CompatibilityFlags   []string
}

// DerivePlanID validates and hashes canonical PlanID inputs. Compatibility
// flags are set-like: order is ignored, duplicates are rejected, and every
// string is length-prefixed so no concatenation ambiguity is possible.
func DerivePlanID(input PlanIdentityInputs) (PlanID, error) {
	if !input.Template.Valid() || !input.Scenario.Valid() || !input.Resources.Valid() {
		return PlanID{}, fmt.Errorf("%w: template, scenario, and resource identities are required", ErrPlanIdentityInputs)
	}
	fields := []struct {
		name  string
		value string
	}{
		{"locale", input.Locale},
		{"timezone", input.Timezone},
		{"unicode_version", input.UnicodeVersion},
		{"cldr_version", input.CLDRVersion},
		{"hyphenation_version", input.HyphenationVersion},
		{"compatibility_profile", input.CompatibilityProfile},
		{"page_profile", input.PageProfile},
		{"planner_version", input.PlannerVersion},
	}
	for _, field := range fields {
		if err := validateTextIdentity("plan identity "+field.name, field.value); err != nil {
			return PlanID{}, fmt.Errorf("%w: %v", ErrPlanIdentityInputs, err)
		}
	}
	if !validCanonicalLocale(input.Locale) {
		return PlanID{}, fmt.Errorf("%w: locale is not a canonical explicit language tag", ErrPlanIdentityInputs)
	}
	if !validCanonicalTimezone(input.Timezone) {
		return PlanID{}, fmt.Errorf("%w: timezone is not canonical UTC, a fixed UTC offset, or an explicit IANA-style name", ErrPlanIdentityInputs)
	}
	if len(input.CompatibilityFlags) > MaxPlanIdentityCompatibilityFlags {
		return PlanID{}, fmt.Errorf("%w: compatibility flag count exceeds %d", ErrPlanIdentityInputs, MaxPlanIdentityCompatibilityFlags)
	}
	flags := append([]string(nil), input.CompatibilityFlags...)
	for index, flag := range flags {
		if err := validateTextIdentity("plan compatibility flag", flag); err != nil {
			return PlanID{}, fmt.Errorf("%w: flag %d: %v", ErrPlanIdentityInputs, index, err)
		}
	}
	sort.Strings(flags)
	for index := 1; index < len(flags); index++ {
		if flags[index] == flags[index-1] {
			return PlanID{}, fmt.Errorf("%w: duplicate compatibility flag %q", ErrPlanIdentityInputs, flags[index])
		}
	}

	digest := sha256.New()
	writePlanIdentityField(digest, "domain", planIdentityDomain)
	writePlanIdentityField(digest, "template", input.Template.String())
	writePlanIdentityField(digest, "scenario", input.Scenario.String())
	writePlanIdentityField(digest, "resources", input.Resources.String())
	for _, field := range fields {
		writePlanIdentityField(digest, field.name, field.value)
	}
	for _, flag := range flags {
		writePlanIdentityField(digest, "compatibility_flag", flag)
	}
	var sum [sha256.Size]byte
	copy(sum[:], digest.Sum(nil))
	return PlanID{digestID{sum: sum}}, nil
}

func validCanonicalLocale(value string) bool {
	parts := strings.Split(value, "-")
	if len(parts) == 0 || len(parts) > 16 || len(parts[0]) < 2 || len(parts[0]) > 8 || !asciiLower(parts[0]) {
		return false
	}
	extension := false
	for _, part := range parts[1:] {
		if len(part) == 0 || len(part) > 8 || !asciiAlphaNumeric(part) {
			return false
		}
		switch {
		case len(part) == 1:
			if !asciiLower(part) && !asciiDigits(part) {
				return false
			}
			extension = true
		case extension:
			for index := range part {
				if part[index] >= 'A' && part[index] <= 'Z' {
					return false
				}
			}
		case len(part) == 4 && asciiAlpha(part):
			if part[0] < 'A' || part[0] > 'Z' || !asciiLower(part[1:]) {
				return false
			}
		case len(part) == 2 && asciiAlpha(part):
			if !asciiUpper(part) {
				return false
			}
		case len(part) == 3 && asciiDigits(part):
		default:
			for index := range part {
				if part[index] >= 'A' && part[index] <= 'Z' {
					return false
				}
			}
		}
	}
	return true
}

func validCanonicalTimezone(value string) bool {
	if value == "UTC" {
		return true
	}
	if strings.HasPrefix(value, "UTC+") || strings.HasPrefix(value, "UTC-") {
		if len(value) != len("UTC+00:00") || value[6] != ':' || !asciiDigits(value[4:6]) || !asciiDigits(value[7:9]) {
			return false
		}
		hours := int(value[4]-'0')*10 + int(value[5]-'0')
		minutes := int(value[7]-'0')*10 + int(value[8]-'0')
		return hours <= 23 && minutes <= 59 && !(value[3] == '-' && hours == 0 && minutes == 0)
	}
	parts := strings.Split(value, "/")
	if len(parts) < 2 || len(parts) > 8 {
		return false
	}
	for _, part := range parts {
		if part == "" || len(part) > 64 || part == "." || part == ".." {
			return false
		}
		for index := range part {
			character := part[index]
			if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || character == '_' || character == '-' || character == '+' {
				continue
			}
			return false
		}
	}
	return value != "Etc/Localtime"
}

func asciiAlpha(value string) bool {
	for index := range value {
		if (value[index] < 'a' || value[index] > 'z') && (value[index] < 'A' || value[index] > 'Z') {
			return false
		}
	}
	return value != ""
}

func asciiLower(value string) bool {
	for index := range value {
		if value[index] < 'a' || value[index] > 'z' {
			return false
		}
	}
	return value != ""
}

func asciiUpper(value string) bool {
	for index := range value {
		if value[index] < 'A' || value[index] > 'Z' {
			return false
		}
	}
	return value != ""
}

func asciiDigits(value string) bool {
	for index := range value {
		if value[index] < '0' || value[index] > '9' {
			return false
		}
	}
	return value != ""
}

func asciiAlphaNumeric(value string) bool {
	for index := range value {
		character := value[index]
		if (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') && (character < '0' || character > '9') {
			return false
		}
	}
	return value != ""
}

func writePlanIdentityField(destination hash.Hash, name, value string) {
	var length [8]byte
	binary.BigEndian.PutUint32(length[:4], uint32(len(name)))
	binary.BigEndian.PutUint32(length[4:], uint32(len(value)))
	_, _ = destination.Write(length[:])
	_, _ = destination.Write([]byte(name))
	_, _ = destination.Write([]byte(value))
}

// RenderIdentityInputs pin every presentation/disclosure choice layered on an
// immutable plan. CropProfile is a canonical named or serialized crop policy;
// the render artifact itself remains accessible through an opaque capability.
type RenderIdentityInputs struct {
	Plan             PlanID
	RendererVersion  string
	ColorProfile     string
	DPI              uint32
	CropProfile      string
	DisclosureDomain string
}

// DeriveRenderID produces a domain-separated artifact identity. It is not a
// public authorization token and must not be substituted for a scoped handle.
func DeriveRenderID(input RenderIdentityInputs) (RenderID, error) {
	if !input.Plan.Valid() || input.DPI == 0 || input.DPI > 19_200 {
		return RenderID{}, fmt.Errorf("%w: plan and DPI are outside valid bounds", ErrRenderIdentityInputs)
	}
	fields := []struct {
		name  string
		value string
	}{
		{"renderer_version", input.RendererVersion},
		{"color_profile", input.ColorProfile},
		{"crop_profile", input.CropProfile},
		{"disclosure_domain", input.DisclosureDomain},
	}
	for _, field := range fields {
		if err := validateTextIdentity("render identity "+field.name, field.value); err != nil {
			return RenderID{}, fmt.Errorf("%w: %v", ErrRenderIdentityInputs, err)
		}
	}
	digest := sha256.New()
	writePlanIdentityField(digest, "domain", "gopdfkit.render-id.v1")
	writePlanIdentityField(digest, "plan", input.Plan.String())
	for _, field := range fields {
		writePlanIdentityField(digest, field.name, field.value)
	}
	writePlanIdentityField(digest, "dpi", fmt.Sprintf("%d", input.DPI))
	var sum [sha256.Size]byte
	copy(sum[:], digest.Sum(nil))
	return RenderID{digestID{sum: sum}}, nil
}
