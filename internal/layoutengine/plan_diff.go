// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

type PlanDiffLimits struct {
	MaxPageChanges     uint64
	MaxFragmentChanges uint64
}

func DefaultPlanDiffLimits() PlanDiffLimits {
	return PlanDiffLimits{MaxPageChanges: 4096, MaxFragmentChanges: 1 << 20}
}

type PlanDiffChangeKind string

const (
	PlanDiffAdded    PlanDiffChangeKind = "added"
	PlanDiffRemoved  PlanDiffChangeKind = "removed"
	PlanDiffModified PlanDiffChangeKind = "modified"
)

type PageDiff struct {
	Page   uint32             `json:"page"`
	Kind   PlanDiffChangeKind `json:"kind"`
	Before *PlannedPage       `json:"before,omitempty"`
	After  *PlannedPage       `json:"after,omitempty"`
}

// FragmentDiffKey remains stable when unrelated fragments are inserted. The
// occurrence disambiguates continuation fragments of one semantic instance.
type FragmentDiffKey struct {
	Node       NodeID     `json:"node"`
	Key        NodeKey    `json:"key"`
	Instance   InstanceID `json:"instance"`
	Occurrence uint32     `json:"occurrence"`
}

type FragmentDiff struct {
	Identity FragmentDiffKey    `json:"identity"`
	Kind     PlanDiffChangeKind `json:"kind"`
	Before   *Fragment          `json:"before,omitempty"`
	After    *Fragment          `json:"after,omitempty"`
}

type PlanDiffCount struct {
	Before uint64 `json:"before"`
	After  uint64 `json:"after"`
}

// LayoutPlanDiff is a deterministic structural comparison, not a PDF byte or
// raster comparison. Totals are exact even when returned changes are bounded.
type LayoutPlanDiff struct {
	BeforeHash PlanHash `json:"before_hash"`
	AfterHash  PlanHash `json:"after_hash"`
	Equal      bool     `json:"equal"`

	Pages       PlanDiffCount `json:"pages"`
	Lines       PlanDiffCount `json:"lines"`
	Commands    PlanDiffCount `json:"commands"`
	Fonts       PlanDiffCount `json:"fonts"`
	Images      PlanDiffCount `json:"images"`
	Breaks      PlanDiffCount `json:"breaks"`
	Diagnostics PlanDiffCount `json:"diagnostics"`

	PageChanges              []PageDiff     `json:"page_changes,omitempty"`
	PageChangeTotal          uint64         `json:"page_change_total"`
	PageChangesTruncated     bool           `json:"page_changes_truncated"`
	FragmentChanges          []FragmentDiff `json:"fragment_changes,omitempty"`
	FragmentChangeTotal      uint64         `json:"fragment_change_total"`
	FragmentChangesTruncated bool           `json:"fragment_changes_truncated"`

	FontCatalogChanged  bool `json:"font_catalog_changed"`
	ImageCatalogChanged bool `json:"image_catalog_changed"`
	DisplayListChanged  bool `json:"display_list_changed"`
	BreakLedgerChanged  bool `json:"break_ledger_changed"`
	DiagnosticsChanged  bool `json:"diagnostics_changed"`
}

func DiffLayoutPlans(before, after LayoutPlan) (LayoutPlanDiff, error) {
	return DiffLayoutPlansWithLimits(before, after, DefaultPlanDiffLimits())
}

func DiffLayoutPlansWithLimits(before, after LayoutPlan, limits PlanDiffLimits) (LayoutPlanDiff, error) {
	if limits.MaxPageChanges == 0 || limits.MaxFragmentChanges == 0 {
		return LayoutPlanDiff{}, errors.New("layoutengine: plan diff limits must be positive")
	}
	if err := before.Validate(); err != nil {
		return LayoutPlanDiff{}, fmt.Errorf("layoutengine: invalid before plan: %w", err)
	}
	if err := after.Validate(); err != nil {
		return LayoutPlanDiff{}, fmt.Errorf("layoutengine: invalid after plan: %w", err)
	}
	beforeHash, err := before.Hash()
	if err != nil {
		return LayoutPlanDiff{}, err
	}
	afterHash, err := after.Hash()
	if err != nil {
		return LayoutPlanDiff{}, err
	}
	b := before.Projection()
	a := after.Projection()
	diff := LayoutPlanDiff{
		BeforeHash: beforeHash, AfterHash: afterHash, Equal: beforeHash == afterHash,
		Pages: countPair(len(b.Pages), len(a.Pages)), Lines: countPair(len(b.Lines), len(a.Lines)),
		Commands: countPair(len(b.Commands), len(a.Commands)), Fonts: countPair(len(b.Fonts), len(a.Fonts)),
		Images: countPair(len(b.Images), len(a.Images)), Breaks: countPair(len(b.Breaks), len(a.Breaks)),
		Diagnostics:        countPair(len(b.Diagnostics), len(a.Diagnostics)),
		FontCatalogChanged: !jsonEqual(b.Fonts, a.Fonts), ImageCatalogChanged: !jsonEqual(b.ImageResources, a.ImageResources),
		DisplayListChanged: !jsonEqual(struct {
			Lines     []PlannedLine
			GlyphRuns []CoreGlyphRun
			Images    []PlannedImage
			Commands  []DisplayCommand
		}{b.Lines, b.GlyphRuns, b.Images, b.Commands}, struct {
			Lines     []PlannedLine
			GlyphRuns []CoreGlyphRun
			Images    []PlannedImage
			Commands  []DisplayCommand
		}{a.Lines, a.GlyphRuns, a.Images, a.Commands}),
		BreakLedgerChanged: !jsonEqual(b.Breaks, a.Breaks), DiagnosticsChanged: !jsonEqual(b.Diagnostics, a.Diagnostics),
	}
	diffPages(&diff, b.Pages, a.Pages, limits.MaxPageChanges)
	diffFragments(&diff, b.Fragments, a.Fragments, limits.MaxFragmentChanges)
	return diff, nil
}

func (diff LayoutPlanDiff) CanonicalJSON() ([]byte, error) {
	return json.Marshal(diff)
}

func countPair(before, after int) PlanDiffCount {
	return PlanDiffCount{Before: uint64(before), After: uint64(after)}
}

func diffPages(diff *LayoutPlanDiff, before, after []PlannedPage, limit uint64) {
	count := len(before)
	if len(after) > count {
		count = len(after)
	}
	for index := 0; index < count; index++ {
		var change PageDiff
		switch {
		case index >= len(before):
			value := after[index]
			change = PageDiff{Page: uint32(index + 1), Kind: PlanDiffAdded, After: &value}
		case index >= len(after):
			value := before[index]
			change = PageDiff{Page: uint32(index + 1), Kind: PlanDiffRemoved, Before: &value}
		case before[index] != after[index]:
			beforeValue, afterValue := before[index], after[index]
			change = PageDiff{Page: uint32(index + 1), Kind: PlanDiffModified, Before: &beforeValue, After: &afterValue}
		default:
			continue
		}
		diff.PageChangeTotal++
		if uint64(len(diff.PageChanges)) < limit {
			diff.PageChanges = append(diff.PageChanges, change)
		}
	}
	diff.PageChangesTruncated = diff.PageChangeTotal > uint64(len(diff.PageChanges))
}

func diffFragments(diff *LayoutPlanDiff, before, after []Fragment, limit uint64) {
	beforeMap := indexFragments(before)
	afterMap := indexFragments(after)
	keys := make([]FragmentDiffKey, 0, len(beforeMap)+len(afterMap))
	seen := make(map[FragmentDiffKey]bool, len(beforeMap)+len(afterMap))
	for key := range beforeMap {
		seen[key] = true
		keys = append(keys, key)
	}
	for key := range afterMap {
		if !seen[key] {
			keys = append(keys, key)
		}
	}
	sort.Slice(keys, func(i, j int) bool { return fragmentDiffKeyLess(keys[i], keys[j]) })
	for _, key := range keys {
		beforeValue, beforeOK := beforeMap[key]
		afterValue, afterOK := afterMap[key]
		var change FragmentDiff
		switch {
		case !beforeOK:
			value := afterValue
			change = FragmentDiff{Identity: key, Kind: PlanDiffAdded, After: &value}
		case !afterOK:
			value := beforeValue
			change = FragmentDiff{Identity: key, Kind: PlanDiffRemoved, Before: &value}
		case beforeValue != afterValue:
			beforeCopy, afterCopy := beforeValue, afterValue
			change = FragmentDiff{Identity: key, Kind: PlanDiffModified, Before: &beforeCopy, After: &afterCopy}
		default:
			continue
		}
		diff.FragmentChangeTotal++
		if uint64(len(diff.FragmentChanges)) < limit {
			diff.FragmentChanges = append(diff.FragmentChanges, change)
		}
	}
	diff.FragmentChangesTruncated = diff.FragmentChangeTotal > uint64(len(diff.FragmentChanges))
}

func indexFragments(fragments []Fragment) map[FragmentDiffKey]Fragment {
	counts := make(map[struct {
		node     NodeID
		key      NodeKey
		instance InstanceID
	}]uint32)
	indexed := make(map[FragmentDiffKey]Fragment, len(fragments))
	for _, fragment := range fragments {
		identity := struct {
			node     NodeID
			key      NodeKey
			instance InstanceID
		}{fragment.Node, fragment.Key, fragment.Instance}
		occurrence := counts[identity]
		counts[identity]++
		key := FragmentDiffKey{Node: fragment.Node, Key: fragment.Key, Instance: fragment.Instance, Occurrence: occurrence}
		indexed[key] = fragment
	}
	return indexed
}

func fragmentDiffKeyLess(left, right FragmentDiffKey) bool {
	if left.Key != right.Key {
		return left.Key < right.Key
	}
	if left.Instance != right.Instance {
		return left.Instance < right.Instance
	}
	if left.Node != right.Node {
		return left.Node < right.Node
	}
	return left.Occurrence < right.Occurrence
}

func jsonEqual(left, right any) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && string(leftJSON) == string(rightJSON)
}
