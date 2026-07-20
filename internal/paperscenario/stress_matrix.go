// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperscenario

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type PageProfileVariant struct {
	Name              string `json:"name"`
	WidthMilliPoints  int64  `json:"width_millipoints"`
	HeightMilliPoints int64  `json:"height_millipoints"`
	MarginMilliPoints int64  `json:"margin_millipoints"`
}

type ResourceFaultKind string

const (
	ResourceMissing   ResourceFaultKind = "missing"
	ResourceTruncated ResourceFaultKind = "truncated"
	ResourceMalformed ResourceFaultKind = "malformed"
	ResourceOversized ResourceFaultKind = "oversized"
)

type ResourceFault struct {
	ResourceKind string            `json:"resource_kind"`
	ResourceID   string            `json:"resource_id"`
	Fault        ResourceFaultKind `json:"fault"`
}

type StressMatrixRequest struct {
	Base           Fixture
	Strategies     []StressStrategy
	OptionalPaths  []string
	DatePaths      []string
	PageProfiles   []PageProfileVariant
	ResourceFaults []ResourceFault
	Seed           uint64
	Limits         StressLimits
}

type StressMatrixCase struct {
	Name          string              `json:"name"`
	Strategy      StressStrategy      `json:"strategy,omitempty"`
	Fixture       Fixture             `json:"fixture"`
	PageProfile   *PageProfileVariant `json:"page_profile,omitempty"`
	ResourceFault *ResourceFault      `json:"resource_fault,omitempty"`
}

// GenerateStressMatrix combines value, schema-optionality, date, page-profile,
// and resource-fault axes without reading ambient resources. Resource faults
// are typed instructions consumed by the bounded asset/font resolver, never
// corrupt bytes inserted into fixture data.
func GenerateStressMatrix(ctx context.Context, request StressMatrixRequest) ([]StressMatrixCase, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	limits := request.Limits
	if limits == (StressLimits{}) {
		limits = DefaultStressLimits()
	}
	if !validStressLimits(limits) || !validName(request.Base.Name) {
		return nil, fmt.Errorf("%w: stress matrix request", ErrLimit)
	}
	result := make([]StressMatrixCase, 0)
	appendCase := func(current StressMatrixCase) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if uint32(len(result)) >= limits.MaxCandidates { // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
			return fmt.Errorf("%w: stress matrix candidates", ErrLimit)
		}
		result = append(result, current)
		return nil
	}
	generated, err := GenerateStressFixtures(ctx, request.Base, request.Strategies, request.Seed, limits)
	if err != nil && len(request.Strategies) != 0 {
		return nil, err
	}
	for _, item := range generated {
		if err := appendCase(StressMatrixCase{Name: item.Fixture.Name, Strategy: item.Strategy, Fixture: item.Fixture}); err != nil {
			return nil, err
		}
	}
	seenPaths := make(map[string]struct{}, len(request.OptionalPaths)+len(request.DatePaths))
	for index, path := range request.OptionalPaths {
		if !validStressPath(path) {
			return nil, fmt.Errorf("%w: optional path %q", ErrInvalid, path)
		}
		key := "optional\x00" + path
		if _, exists := seenPaths[key]; exists {
			return nil, fmt.Errorf("%w: duplicate optional path %q", ErrInvalid, path)
		}
		seenPaths[key] = struct{}{}
		values := cloneFields(request.Base.Values)
		values, exists := deleteFixturePath(values, path)
		if !exists {
			return nil, fmt.Errorf("%w: optional path %q is absent", ErrInvalid, path)
		}
		name := matrixFixtureName(request.Base.Name, "optional", index)
		fixture, err := normalizeStressFixture(name, request.Base.Locale, values)
		if err != nil {
			return nil, err
		}
		if err := appendCase(StressMatrixCase{Name: name, Strategy: "optional-omission", Fixture: fixture}); err != nil {
			return nil, err
		}
	}
	dateValues := []string{"0001-01-01T00:00:00Z", "1970-01-01T00:00:00-12:00", "9999-12-31T23:59:59+14:00"}
	for pathIndex, path := range request.DatePaths {
		if !validStressPath(path) {
			return nil, fmt.Errorf("%w: date path %q", ErrInvalid, path)
		}
		key := "date\x00" + path
		if _, exists := seenPaths[key]; exists {
			return nil, fmt.Errorf("%w: duplicate date path %q", ErrInvalid, path)
		}
		seenPaths[key] = struct{}{}
		value, exists := fixtureStringAtPath(request.Base.Values, path)
		if !exists {
			return nil, fmt.Errorf("%w: date path %q is not a string", ErrInvalid, path)
		}
		if _, err := time.Parse(time.RFC3339, value); err != nil {
			return nil, fmt.Errorf("%w: date path %q is not RFC3339", ErrInvalid, path)
		}
		for valueIndex, date := range dateValues {
			values := cloneFields(request.Base.Values)
			setFixtureStringAtPath(values, path, date)
			name := matrixFixtureName(request.Base.Name, "date", pathIndex*len(dateValues)+valueIndex)
			fixture, err := normalizeStressFixture(name, request.Base.Locale, values)
			if err != nil {
				return nil, err
			}
			if err := appendCase(StressMatrixCase{Name: name, Strategy: "date-boundary", Fixture: fixture}); err != nil {
				return nil, err
			}
		}
	}
	for index, profile := range request.PageProfiles {
		if !validMatrixLabel(profile.Name) || profile.WidthMilliPoints <= 0 || profile.HeightMilliPoints <= 0 || profile.MarginMilliPoints < 0 || 2*profile.MarginMilliPoints >= profile.WidthMilliPoints || 2*profile.MarginMilliPoints >= profile.HeightMilliPoints {
			return nil, fmt.Errorf("%w: page profile[%d]", ErrInvalid, index)
		}
		name := matrixFixtureName(request.Base.Name, "page", index)
		fixture, err := normalizeStressFixture(name, request.Base.Locale, cloneFields(request.Base.Values))
		if err != nil {
			return nil, err
		}
		copyProfile := profile
		if err := appendCase(StressMatrixCase{Name: name, Strategy: "page-profile", Fixture: fixture, PageProfile: &copyProfile}); err != nil {
			return nil, err
		}
	}
	for index, fault := range request.ResourceFaults {
		if (fault.ResourceKind != "asset" && fault.ResourceKind != "font") || !validMatrixLabel(fault.ResourceID) || !validResourceFault(fault.Fault) {
			return nil, fmt.Errorf("%w: resource fault[%d]", ErrInvalid, index)
		}
		name := matrixFixtureName(request.Base.Name, "resource", index)
		fixture, err := normalizeStressFixture(name, request.Base.Locale, cloneFields(request.Base.Values))
		if err != nil {
			return nil, err
		}
		copyFault := fault
		if err := appendCase(StressMatrixCase{Name: name, Strategy: "resource-fault", Fixture: fixture, ResourceFault: &copyFault}); err != nil {
			return nil, err
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("%w: empty stress matrix", ErrInvalid)
	}
	return result, nil
}

func matrixFixtureName(base, axis string, index int) string {
	return base + "-matrix-" + axis + "-" + strconv.Itoa(index+1)
}

func validStressPath(path string) bool {
	if path == "" || len(path) > 4096 || !utf8.ValidString(path) || strings.HasPrefix(path, ".") || strings.HasSuffix(path, ".") {
		return false
	}
	for _, part := range strings.Split(path, ".") {
		if !validName(part) {
			return false
		}
	}
	return true
}

func validMatrixLabel(value string) bool {
	return value != "" && len(value) <= 4096 && utf8.ValidString(value) && strings.TrimSpace(value) == value && !strings.ContainsAny(value, "\r\n")
}

func validResourceFault(value ResourceFaultKind) bool {
	return value == ResourceMissing || value == ResourceTruncated || value == ResourceMalformed || value == ResourceOversized
}

func deleteFixturePath(fields []Field, path string) ([]Field, bool) {
	parts := strings.Split(path, ".")
	if len(parts) == 1 {
		position := fieldIndex(fields, parts[0])
		if position < 0 {
			return fields, false
		}
		return append(fields[:position], fields[position+1:]...), true
	}
	current := fields
	for index, part := range parts {
		position := fieldIndex(current, part)
		if position < 0 {
			return fields, false
		}
		if current[position].Value.Kind != Object {
			return fields, false
		}
		if index == len(parts)-2 {
			nested := current[position].Value.Object
			nestedPosition := fieldIndex(nested, parts[index+1])
			if nestedPosition < 0 {
				return fields, false
			}
			current[position].Value.Object = append(nested[:nestedPosition], nested[nestedPosition+1:]...)
			return fields, true
		}
		current = current[position].Value.Object
	}
	return fields, false
}
