// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestCompactProvenanceInternsDeterministicallyAndSurvivesStoreAndQuery(t *testing.T) {
	plan, err := NewLayoutPlan(testPlanInput())
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.Projection()
	if len(projection.Provenance) != 1 || !reflect.DeepEqual(projection.FragmentProvenance, []ProvenanceID{1, 1}) || len(projection.LineProvenance) != 0 {
		t.Fatalf("compact provenance = %+v / %v / %v", projection.Provenance, projection.FragmentProvenance, projection.LineProvenance)
	}
	entry, ok := plan.ResolveProvenance(1)
	if !ok || entry.Node != 7 || entry.Key != "@lines" || entry.Instance != "@lines" {
		t.Fatalf("resolved provenance = %+v, %t", entry, ok)
	}
	if _, ok := plan.ResolveProvenance(0); ok {
		t.Fatal("zero provenance unexpectedly resolved")
	}
	paragraph, err := PlanParagraphFlow(testParagraphFlowInput(3, 2, 1, 1, ParagraphBreakPrefer))
	if err != nil {
		t.Fatal(err)
	}
	paragraphProjection := paragraph.Projection()
	if len(paragraphProjection.LineProvenance) != 3 {
		t.Fatalf("line provenance refs = %v", paragraphProjection.LineProvenance)
	}
	for index, id := range paragraphProjection.LineProvenance {
		if !id.Valid() || uint64(id) > uint64(len(paragraphProjection.Provenance)) {
			t.Fatalf("line %d provenance %d outside table", index, id)
		}
	}
	projection.Provenance[0].Key = "@mutated"
	projection.FragmentProvenance[0] = 99
	again := plan.Projection()
	if again.Provenance[0].Key != "@lines" || again.FragmentProvenance[0] != 1 {
		t.Fatal("projection aliases compact provenance storage")
	}
	query, err := plan.QueryStructure(StructuralQuery{Node: 7, MaxResults: 8})
	if err != nil || len(query.Fragments) != 2 || query.Fragments[0].Provenance != 1 || query.Fragments[1].Provenance != 1 {
		t.Fatalf("query provenance = %+v, %v", query.Fragments, err)
	}
	store, err := NewMemoryPlanStore(DefaultPlanStoreLimits())
	if err != nil {
		t.Fatal(err)
	}
	hash, err := store.Put(plan)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := store.Get(hash)
	if err != nil || !reflect.DeepEqual(restored.Projection().Provenance, again.Provenance) || !reflect.DeepEqual(restored.Projection().FragmentProvenance, again.FragmentProvenance) {
		t.Fatalf("restored provenance = %+v, %v", restored.Projection(), err)
	}
}

func TestDetailedBreaksAreExplicitBoundedCancelableAndConciseRecordsRemainCanonical(t *testing.T) {
	plan, err := NewLayoutPlan(testPlanInput())
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := plan.CanonicalJSON()
	if err != nil || bytes.Contains(canonical, []byte("break_details")) || !bytes.Contains(canonical, []byte(`"breaks"`)) {
		t.Fatalf("canonical concise projection = %s, %v", canonical, err)
	}
	details, err := plan.DetailedBreaksContext(context.Background(), BreakDetailLimits{})
	if err != nil || len(details) != 1 {
		t.Fatalf("details = %+v, %v", details, err)
	}
	want := detailForBreak(0, plan.Projection().Breaks[0])
	if !reflect.DeepEqual(details[0], want) {
		t.Fatalf("detail = %+v, want %+v", details[0], want)
	}
	detailed, err := plan.DetailedProjectionContext(context.Background(), BreakDetailLimits{})
	if err != nil || len(detailed.BreakDetails) != 1 || !reflect.DeepEqual(detailed.Plan.Breaks, plan.Projection().Breaks) {
		t.Fatalf("detailed projection = %+v, %v", detailed, err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := plan.DetailedBreaksContext(canceled, BreakDetailLimits{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled details = %v", err)
	}
	limits := DefaultBreakDetailLimits()
	limits.MaxWork = 3
	if _, err := plan.DetailedBreaksContext(context.Background(), limits); !errors.Is(err, ErrBreakDetailLimit) {
		t.Fatalf("work-limited details = %v", err)
	}
	limits = DefaultBreakDetailLimits()
	limits.MaxBreaks = 0
	if _, err := plan.DetailedBreaksContext(context.Background(), limits); !errors.Is(err, ErrBreakDetailLimit) {
		t.Fatalf("invalid partial limits = %v", err)
	}
}

func TestBreakDetailCodecAndStructuralQueryAreDeterministicAndBounded(t *testing.T) {
	plan, err := NewLayoutPlan(testPlanInput())
	if err != nil {
		t.Fatal(err)
	}
	details, _ := plan.DetailedBreaksContext(context.Background(), BreakDetailLimits{})
	encoded, err := EncodeBreakDetailSet(context.Background(), details, BreakDetailLimits{})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeBreakDetailSet(context.Background(), encoded, BreakDetailLimits{})
	if err != nil || !reflect.DeepEqual(decoded.Details, details) {
		t.Fatalf("decoded = %+v, %v", decoded, err)
	}
	again, _ := EncodeBreakDetailSet(context.Background(), decoded.Details, BreakDetailLimits{})
	if !bytes.Equal(encoded, again) {
		t.Fatalf("codec is nondeterministic:\n%s\n%s", encoded, again)
	}
	query := StructuralQuery{Node: 7, MaxResults: 8}
	queried, err := plan.QueryBreakDetailsContext(context.Background(), query, BreakDetailLimits{})
	if err != nil || queried.Count.Matches != 1 || len(queried.Details) != 1 || !reflect.DeepEqual(queried.Details[0], details[0]) {
		t.Fatalf("detail query = %+v, %v", queried, err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := plan.QueryBreakDetailsContext(canceled, query, BreakDetailLimits{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled query = %v", err)
	}
	queryLimits := DefaultBreakDetailLimits()
	queryLimits.MaxWork = 4
	if _, err := plan.QueryBreakDetailsContext(context.Background(), query, queryLimits); !errors.Is(err, ErrBreakDetailLimit) {
		t.Fatalf("work-limited query = %v", err)
	}
	if _, err := DecodeBreakDetailSet(context.Background(), append(append([]byte(nil), encoded...), []byte("{}")...), BreakDetailLimits{}); !errors.Is(err, ErrBreakDetailInvalid) {
		t.Fatalf("trailing codec value = %v", err)
	}
	invalid := cloneBreakDetail(details[0])
	invalid.Steps[0].Kind = "ambient"
	if _, err := EncodeBreakDetailSet(context.Background(), []BreakDetail{invalid}, BreakDetailLimits{}); !errors.Is(err, ErrBreakDetailInvalid) {
		t.Fatalf("invalid trace = %v", err)
	}
	duplicate := []BreakDetail{cloneBreakDetail(details[0]), cloneBreakDetail(details[0])}
	if _, err := EncodeBreakDetailSet(context.Background(), duplicate, BreakDetailLimits{}); !errors.Is(err, ErrBreakDetailInvalid) {
		t.Fatalf("duplicate break indexes = %v", err)
	}
}
