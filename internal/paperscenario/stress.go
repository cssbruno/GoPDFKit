// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperscenario

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// StressStrategy is a deterministic, schema-preserving fixture mutation
// family. Asset, font, and page-profile strategies live at their respective
// resource boundaries rather than being smuggled into fixture values.
type StressStrategy string

const (
	StressEmptyCollections   StressStrategy = "empty-collections"
	StressMaximumCollections StressStrategy = "maximum-collections"
	StressLongLocalizedText  StressStrategy = "long-localized-text"
	StressUnbreakableText    StressStrategy = "unbreakable-text"
	StressComplexUnicode     StressStrategy = "complex-unicode"
	StressExtremeNumbers     StressStrategy = "extreme-numbers"
	StressNegativeNumbers    StressStrategy = "negative-numbers"
	StressDecimalPrecision   StressStrategy = "decimal-precision"
)

type StressLimits struct {
	MaxCandidates  uint32
	MaxWork        uint64
	MaxStringBytes uint32
	MaxListItems   uint32
	MaxEvaluations uint32
}

func DefaultStressLimits() StressLimits {
	return StressLimits{MaxCandidates: 32, MaxWork: 1_000_000, MaxStringBytes: 64 << 10, MaxListItems: 1024, MaxEvaluations: 4096}
}

func validStressLimits(l StressLimits) bool {
	h := DefaultStressLimits()
	return l.MaxCandidates > 0 && l.MaxCandidates <= h.MaxCandidates && l.MaxWork > 0 && l.MaxWork <= h.MaxWork &&
		l.MaxStringBytes > 0 && l.MaxStringBytes <= h.MaxStringBytes && l.MaxListItems > 0 && l.MaxListItems <= h.MaxListItems &&
		l.MaxEvaluations > 0 && l.MaxEvaluations <= h.MaxEvaluations
}

type GeneratedFixture struct {
	Strategy StressStrategy `json:"strategy"`
	Seed     uint64         `json:"seed"`
	Fixture  Fixture        `json:"fixture"`
}

// GenerateStressFixtures produces detached replayable fixtures in requested
// order. Seed participates in stable list keys and is recorded even though the
// generator deliberately uses no ambient randomness.
func GenerateStressFixtures(ctx context.Context, base Fixture, strategies []StressStrategy, seed uint64, limits StressLimits) ([]GeneratedFixture, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if limits == (StressLimits{}) {
		limits = DefaultStressLimits()
	}
	if !validStressLimits(limits) || len(strategies) == 0 || uint32(len(strategies)) > limits.MaxCandidates || !validName(base.Name) { // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
		return nil, fmt.Errorf("%w: stress request", ErrLimit)
	}
	result := make([]GeneratedFixture, 0, len(strategies))
	var work uint64
	seen := make(map[StressStrategy]bool, len(strategies))
	for index, strategy := range strategies {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if seen[strategy] {
			return nil, fmt.Errorf("%w: duplicate stress strategy %q", ErrInvalid, strategy)
		}
		seen[strategy] = true
		values := cloneFields(base.Values)
		changed, err := mutateStressFields(ctx, values, strategy, seed, limits, &work)
		if err != nil {
			return nil, err
		}
		if !changed {
			return nil, fmt.Errorf("%w: strategy %q has no compatible target", ErrInvalid, strategy)
		}
		name := base.Name + "-stress-" + strconv.Itoa(index+1)
		fixture, err := normalizeStressFixture(name, base.Locale, values)
		if err != nil {
			return nil, err
		}
		result = append(result, GeneratedFixture{Strategy: strategy, Seed: seed, Fixture: fixture})
	}
	return result, nil
}

func mutateStressFields(ctx context.Context, fields []Field, strategy StressStrategy, seed uint64, limits StressLimits, work *uint64) (bool, error) {
	changed := false
	for index := range fields {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		current, didChange, err := mutateStressValue(ctx, fields[index].Value, strategy, seed, limits, work)
		if err != nil {
			return false, err
		}
		fields[index].Value, changed = current, changed || didChange
	}
	return changed, nil
}

func mutateStressValue(ctx context.Context, value Value, strategy StressStrategy, seed uint64, limits StressLimits, work *uint64) (Value, bool, error) {
	*work++
	if *work > limits.MaxWork {
		return Value{}, false, fmt.Errorf("%w: stress work", ErrLimit)
	}
	switch value.Kind {
	case String:
		switch strategy {
		case StressLongLocalizedText:
			unit := value.String
			if unit == "" {
				unit = "localized"
			}
			for len(unit) < int(limits.MaxStringBytes)/2 {
				unit += " " + unit
			}
			if len(unit) > int(limits.MaxStringBytes) {
				unit = unit[:limits.MaxStringBytes]
			}
			value.String = unit
			return value, true, nil
		case StressUnbreakableText:
			value.String = strings.Repeat("W", int(limits.MaxStringBytes))
			return value, true, nil
		case StressComplexUnicode:
			unit := "漢字 العربية \u202eRTL\u202c e\u0301 👩🏽‍💻 "
			value.String = repeatWholeString(unit, int(limits.MaxStringBytes))
			return value, true, nil
		}
	case Number:
		switch strategy {
		case StressExtremeNumbers:
			value.Number = "999999999999999999999999.99"
			return value, true, nil
		case StressNegativeNumbers:
			value.Number = "-999999999999999999999999.99"
			return value, true, nil
		case StressDecimalPrecision:
			value.Number = "0.000000000000000000000001"
			return value, true, nil
		}
	case List:
		switch strategy {
		case StressEmptyCollections:
			value.List = nil
			return value, true, nil
		case StressMaximumCollections:
			if len(value.List) == 0 {
				return value, false, nil
			}
			base := append([]Item(nil), value.List...)
			for len(value.List) < int(limits.MaxListItems) {
				template := base[len(value.List)%len(base)]
				template.Key = "stress-" + strconv.FormatUint(seed, 10) + "-" + strconv.Itoa(len(value.List))
				template.Value = cloneValue(template.Value)
				value.List = append(value.List, template)
				*work++
				if *work > limits.MaxWork {
					return Value{}, false, fmt.Errorf("%w: stress work", ErrLimit)
				}
			}
			return value, true, nil
		}
		changed := false
		for index := range value.List {
			if err := ctx.Err(); err != nil {
				return Value{}, false, err
			}
			mutated, didChange, err := mutateStressValue(ctx, value.List[index].Value, strategy, seed, limits, work)
			if err != nil {
				return Value{}, false, err
			}
			value.List[index].Value, changed = mutated, changed || didChange
		}
		return value, changed, nil
	case Object:
		changed, err := mutateStressFields(ctx, value.Object, strategy, seed, limits, work)
		return value, changed, err
	}
	return value, false, nil
}

func repeatWholeString(unit string, maximum int) string {
	if unit == "" || maximum <= 0 {
		return ""
	}
	if maximum < len(unit) {
		end := 0
		for end < len(unit) {
			_, size := utf8.DecodeRuneInString(unit[end:])
			if size == 0 || end+size > maximum {
				break
			}
			end += size
		}
		return unit[:end]
	}
	var result strings.Builder
	result.Grow(maximum)
	for result.Len()+len(unit) <= maximum {
		result.WriteString(unit)
	}
	return result.String()
}

type LayoutObservation struct {
	PageCount   uint32 `json:"page_count"`
	BreakDigest string `json:"break_digest,omitempty"`
	Overflow    bool   `json:"overflow,omitempty"`
}

func (o LayoutObservation) Equal(other LayoutObservation) bool { return o == other }

type FixtureEvaluator func(context.Context, Fixture) (LayoutObservation, error)

type BoundaryRequest struct {
	Base     Fixture
	Path     string
	Minimum  uint32
	Maximum  uint32
	Evaluate FixtureEvaluator
	Limits   StressLimits
}

type BoundaryResult struct {
	Path              string            `json:"path"`
	Boundary          uint32            `json:"boundary"`
	Before            Fixture           `json:"before"`
	After             Fixture           `json:"after"`
	BeforeObservation LayoutObservation `json:"before_observation"`
	AfterObservation  LayoutObservation `json:"after_observation"`
	Evaluations       uint32            `json:"evaluations"`
}

// FindStringRepeatBoundary finds the first repeat count whose page/break/
// overflow observation differs from Minimum. The predicate is required to be
// monotonic over the requested range and the returned pair is replayable.
func FindStringRepeatBoundary(ctx context.Context, request BoundaryRequest) (BoundaryResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if request.Limits == (StressLimits{}) {
		request.Limits = DefaultStressLimits()
	}
	if !validStressLimits(request.Limits) || request.Evaluate == nil || request.Minimum >= request.Maximum || request.Maximum > request.Limits.MaxStringBytes {
		return BoundaryResult{}, fmt.Errorf("%w: boundary request", ErrLimit)
	}
	baseValue, ok := fixtureStringAtPath(request.Base.Values, request.Path)
	if !ok || baseValue == "" {
		return BoundaryResult{}, fmt.Errorf("%w: boundary path", ErrInvalid)
	}
	return findBoundary(ctx, request, func(count uint32) (Fixture, error) {
		values := cloneFields(request.Base.Values)
		if !setFixtureStringAtPath(values, request.Path, strings.Repeat(baseValue, int(count))) {
			return Fixture{}, ErrInvalid
		}
		return normalizeStressFixture(request.Base.Name, request.Base.Locale, values)
	})
}

// FindListLengthBoundary varies a stable-keyed list by cycling its first
// authored templates and deriving deterministic boundary-only keys.
func FindListLengthBoundary(ctx context.Context, request BoundaryRequest) (BoundaryResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if request.Limits == (StressLimits{}) {
		request.Limits = DefaultStressLimits()
	}
	value, ok := fixtureValueAtPath(request.Base.Values, request.Path)
	if !ok || value.Kind != List || len(value.List) == 0 || request.Maximum > request.Limits.MaxListItems {
		return BoundaryResult{}, fmt.Errorf("%w: list boundary path", ErrInvalid)
	}
	templates := append([]Item(nil), value.List...)
	return findBoundary(ctx, request, func(count uint32) (Fixture, error) {
		items := make([]Item, count)
		for index := range items {
			template := templates[index%len(templates)]
			template.Key = "boundary-" + strconv.Itoa(index)
			template.Value = cloneValue(template.Value)
			items[index] = template
		}
		values := cloneFields(request.Base.Values)
		if !setFixtureValueAtPath(values, request.Path, Value{Kind: List, List: items}) {
			return Fixture{}, ErrInvalid
		}
		return normalizeStressFixture(request.Base.Name, request.Base.Locale, values)
	})
}

// FindIntegerBoundary varies a canonical non-negative integer fixture value.
func FindIntegerBoundary(ctx context.Context, request BoundaryRequest) (BoundaryResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	value, ok := fixtureValueAtPath(request.Base.Values, request.Path)
	if !ok || value.Kind != Number {
		return BoundaryResult{}, fmt.Errorf("%w: integer boundary path", ErrInvalid)
	}
	return findBoundary(ctx, request, func(count uint32) (Fixture, error) {
		values := cloneFields(request.Base.Values)
		if !setFixtureValueAtPath(values, request.Path, Value{Kind: Number, Number: strconv.FormatUint(uint64(count), 10)}) {
			return Fixture{}, ErrInvalid
		}
		return normalizeStressFixture(request.Base.Name, request.Base.Locale, values)
	})
}

type boundaryBuilder func(uint32) (Fixture, error)

func findBoundary(ctx context.Context, request BoundaryRequest, build boundaryBuilder) (BoundaryResult, error) {
	if request.Limits == (StressLimits{}) {
		request.Limits = DefaultStressLimits()
	}
	if !validStressLimits(request.Limits) || request.Evaluate == nil || request.Minimum >= request.Maximum || build == nil {
		return BoundaryResult{}, fmt.Errorf("%w: boundary request", ErrLimit)
	}
	evaluations := uint32(0)
	evaluate := func(count uint32) (Fixture, LayoutObservation, error) {
		if err := ctx.Err(); err != nil {
			return Fixture{}, LayoutObservation{}, err
		}
		evaluations++
		if evaluations > request.Limits.MaxEvaluations {
			return Fixture{}, LayoutObservation{}, fmt.Errorf("%w: boundary evaluations", ErrLimit)
		}
		fixture, err := build(count)
		if err != nil {
			return Fixture{}, LayoutObservation{}, err
		}
		observation, err := request.Evaluate(ctx, fixture)
		return fixture, observation, err
	}
	_, baseline, err := evaluate(request.Minimum)
	if err != nil {
		return BoundaryResult{}, err
	}
	_, maximumObservation, err := evaluate(request.Maximum)
	if err != nil {
		return BoundaryResult{}, err
	}
	if baseline.Equal(maximumObservation) {
		return BoundaryResult{}, errors.New("paperscenario: boundary was not crossed")
	}
	low, high := request.Minimum, request.Maximum
	for low+1 < high {
		mid := low + (high-low)/2
		_, observation, err := evaluate(mid)
		if err != nil {
			return BoundaryResult{}, err
		}
		if observation.Equal(baseline) {
			low = mid
		} else {
			high = mid
		}
	}
	before, beforeObservation, err := evaluate(low)
	if err != nil {
		return BoundaryResult{}, err
	}
	after, afterObservation, err := evaluate(high)
	if err != nil {
		return BoundaryResult{}, err
	}
	return BoundaryResult{Path: request.Path, Boundary: high, Before: before, After: after, BeforeObservation: beforeObservation, AfterObservation: afterObservation, Evaluations: evaluations}, nil
}

type IssuePredicate func(context.Context, Fixture) (bool, error)

type MinimizeResult struct {
	Fixture     Fixture `json:"fixture"`
	Evaluations uint32  `json:"evaluations"`
}

// MinimizeFixture deterministically delta-debugs top-level fields, object
// fields, list items, and strings while preserving the caller's issue.
func MinimizeFixture(ctx context.Context, input Fixture, issue IssuePredicate, limits StressLimits) (MinimizeResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if limits == (StressLimits{}) {
		limits = DefaultStressLimits()
	}
	if !validStressLimits(limits) || issue == nil {
		return MinimizeResult{}, ErrLimit
	}
	current, err := normalizeStressFixture(input.Name, input.Locale, cloneFields(input.Values))
	if err != nil {
		return MinimizeResult{}, err
	}
	evaluations := uint32(0)
	keeps := func(candidate Fixture) (bool, error) {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		evaluations++
		if evaluations > limits.MaxEvaluations {
			return false, fmt.Errorf("%w: minimizer evaluations", ErrLimit)
		}
		return issue(ctx, candidate)
	}
	ok, err := keeps(current)
	if err != nil || !ok {
		if err == nil {
			err = errors.New("paperscenario: input does not reproduce issue")
		}
		return MinimizeResult{}, err
	}
	for changed := true; changed; {
		changed = false
		remaining := int(limits.MaxEvaluations - evaluations)
		for _, trialValues := range fixtureReductionCandidates(current.Values, remaining) {
			trial, normalizeErr := normalizeStressFixture(current.Name, current.Locale, trialValues)
			if normalizeErr != nil {
				continue
			}
			keep, issueErr := keeps(trial)
			if issueErr != nil {
				return MinimizeResult{}, issueErr
			}
			if keep {
				current, changed = trial, true
				break
			}
		}
	}
	return MinimizeResult{Fixture: current, Evaluations: evaluations}, nil
}

func fixtureReductionCandidates(fields []Field, limit int) [][]Field {
	if limit <= 0 {
		return nil
	}
	result := make([][]Field, 0, min(limit, len(fields)*2))
	for index := range fields {
		trial := cloneFields(fields)
		trial = append(trial[:index], trial[index+1:]...)
		result = append(result, trial)
		if len(result) == limit {
			return result
		}
	}
	for index := range fields {
		for _, reduced := range valueReductionCandidates(fields[index].Value, limit-len(result)) {
			trial := cloneFields(fields)
			trial[index].Value = reduced
			result = append(result, trial)
			if len(result) == limit {
				return result
			}
		}
	}
	return result
}

func valueReductionCandidates(value Value, limit int) []Value {
	if limit <= 0 {
		return nil
	}
	result := make([]Value, 0, min(limit, 8))
	switch value.Kind {
	case String:
		if len(value.String) > 1 {
			candidate := cloneValue(value)
			candidate.String = utf8Prefix(candidate.String, len(candidate.String)/2)
			if candidate.String != "" {
				result = append(result, candidate)
			}
		}
	case Object:
		for _, fields := range fixtureReductionCandidates(value.Object, limit) {
			candidate := cloneValue(value)
			candidate.Object = fields
			result = append(result, candidate)
			if len(result) == limit {
				break
			}
		}
	case List:
		for index := range value.List {
			candidate := cloneValue(value)
			candidate.List = append(candidate.List[:index], candidate.List[index+1:]...)
			result = append(result, candidate)
			if len(result) == limit {
				return result
			}
		}
		for index := range value.List {
			for _, reduced := range valueReductionCandidates(value.List[index].Value, limit-len(result)) {
				candidate := cloneValue(value)
				candidate.List[index].Value = reduced
				result = append(result, candidate)
				if len(result) == limit {
					return result
				}
			}
		}
	}
	return result
}

func utf8Prefix(value string, maximum int) string {
	end := 0
	for end < len(value) {
		_, size := utf8.DecodeRuneInString(value[end:])
		if size == 0 || end+size > maximum {
			break
		}
		end += size
	}
	return value[:end]
}

func normalizeStressFixture(name, locale string, values []Field) (Fixture, error) {
	resolved, err := Resolve([]Scenario{{Name: name, Locale: locale, Values: values}}, Limits{})
	if err != nil {
		return Fixture{}, err
	}
	return resolved[0], nil
}

func fixtureStringAtPath(fields []Field, path string) (string, bool) {
	value, ok := fixtureValueAtPath(fields, path)
	return value.String, ok && value.Kind == String
}

func setFixtureStringAtPath(fields []Field, path, replacement string) bool {
	return setFixtureValueAtPath(fields, path, Value{Kind: String, String: replacement})
}

func fixtureValueAtPath(fields []Field, path string) (Value, bool) {
	parts := strings.Split(path, ".")
	current := fields
	for index, part := range parts {
		position := fieldIndex(current, part)
		if position < 0 {
			return Value{}, false
		}
		if index == len(parts)-1 {
			return cloneValue(current[position].Value), true
		}
		if current[position].Value.Kind != Object {
			return Value{}, false
		}
		current = current[position].Value.Object
	}
	return Value{}, false
}

func setFixtureValueAtPath(fields []Field, path string, replacement Value) bool {
	parts := strings.Split(path, ".")
	current := fields
	for index, part := range parts {
		position := fieldIndex(current, part)
		if position < 0 {
			return false
		}
		if index == len(parts)-1 {
			current[position].Value = cloneValue(replacement)
			return true
		}
		if current[position].Value.Kind != Object {
			return false
		}
		current = current[position].Value.Object
	}
	return false
}
