// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestMemorySegmentedPlanStoreReconstructsCanonicalPlanAndReadsMetadataAlone(t *testing.T) {
	plan := segmentedStoreTestPlan(t)
	limits := DefaultSegmentedPlanStoreLimits()
	limits.MaxPagesPerSegment = 2
	store, err := NewMemorySegmentedPlanStore(limits)
	if err != nil {
		t.Fatal(err)
	}
	wantHash, _ := plan.Hash()
	hash, err := store.Put(context.Background(), plan)
	if err != nil || hash != wantHash {
		t.Fatalf("Put() = %s, %v; want %s", hash, err, wantHash)
	}
	manifest, err := decodeSegmentedManifest(store.manifests[hash], hash)
	if err != nil || len(manifest.Segments) != 3 {
		t.Fatalf("manifest segments = %d, %v", len(manifest.Segments), err)
	}
	for _, reference := range manifest.Segments {
		if reference.PageCount == 0 || reference.PageCount > 2 {
			t.Fatalf("bounded reference = %+v", reference)
		}
	}
	loaded, err := store.Get(context.Background(), hash)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := loaded.Hash(); got != hash {
		t.Fatalf("reconstructed hash = %s, want %s", got, hash)
	}
	projection := loaded.Projection()
	projection.Fragments[0].Key = "@mutated"
	again, _ := store.Get(context.Background(), hash)
	if again.Projection().Fragments[0].Key == "@mutated" {
		t.Fatal("Get returned mutable aliases")
	}
	metadata, err := store.PageMetadata(context.Background(), hash, 4)
	if err != nil || metadata.Page.Number != 4 || metadata.SegmentIndex != 1 {
		t.Fatalf("PageMetadata() = %+v, %v", metadata, err)
	}
	// Metadata is manifest-only: removing display segments does not affect it.
	store.mu.Lock()
	delete(store.segments, manifest.Segments[0].Hash)
	store.mu.Unlock()
	if _, err := store.PageMetadata(context.Background(), hash, 4); err != nil {
		t.Fatalf("metadata unexpectedly loaded a segment: %v", err)
	}
	if _, err := store.Get(context.Background(), hash); !errors.Is(err, ErrSegmentedPlanStoreMissing) {
		t.Fatalf("missing segment Get() = %v", err)
	}
	if _, err := store.Put(context.Background(), plan); !errors.Is(err, ErrPlanStoreCorrupt) {
		t.Fatalf("idempotent Put did not detect missing retained segment: %v", err)
	}
}

func TestMemorySegmentedPlanStoreRoundTripsRetainedGridTracks(t *testing.T) {
	result, err := PlanGrid(GridPlanInput{
		PageSize: Size{Width: 100, Height: 100}, Region: Rect{Width: 100, Height: 20},
		Columns: []GridTrack{{Kind: GridTrackFixed, Size: 40}, {Kind: GridTrackFixed, Size: 55}},
		Rows:    []GridTrack{{Kind: GridTrackFixed, Size: 20}}, ColumnGap: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewMemorySegmentedPlanStore(DefaultSegmentedPlanStoreLimits())
	if err != nil {
		t.Fatal(err)
	}
	hash, err := store.Put(context.Background(), result.Plan)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := decodeSegmentedManifest(store.manifests[hash], hash)
	if err != nil || manifest.Counts.GridTracks != 3 || manifest.Counts.PageRegions != 1 {
		t.Fatalf("grid-track manifest = %+v, %v", manifest.Counts, err)
	}
	loaded, err := store.Get(context.Background(), hash)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := loaded.Projection().GridTracks, result.Plan.Projection().GridTracks; !reflect.DeepEqual(got, want) {
		t.Fatalf("segmented grid tracks = %+v, want %+v", got, want)
	}
	if got, want := loaded.Projection().PageRegions, result.Plan.Projection().PageRegions; !reflect.DeepEqual(got, want) {
		t.Fatalf("segmented page regions = %+v, want %+v", got, want)
	}
}

func TestMemorySegmentedPlanStoreDetectsCorruptionCancellationAndEveryLimit(t *testing.T) {
	plan := segmentedStoreTestPlan(t)
	limits := DefaultSegmentedPlanStoreLimits()
	limits.MaxPagesPerSegment = 1
	store, _ := NewMemorySegmentedPlanStore(limits)
	hash, _ := store.Put(context.Background(), plan)
	manifest, _ := decodeSegmentedManifest(store.manifests[hash], hash)
	segmentHash := manifest.Segments[0].Hash
	store.segments[segmentHash][0] ^= 1
	if _, err := store.Get(context.Background(), hash); !errors.Is(err, ErrPlanStoreCorrupt) || !errors.Is(err, ErrPlanStoreHashMismatch) {
		t.Fatalf("corrupt segment Get() = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	fresh, _ := NewMemorySegmentedPlanStore(DefaultSegmentedPlanStoreLimits())
	if _, err := fresh.Put(ctx, plan); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Put() = %v", err)
	}
	workLimits := DefaultSegmentedPlanStoreLimits()
	workLimits.MaxWork = 1
	workStore, _ := NewMemorySegmentedPlanStore(workLimits)
	if _, err := workStore.Put(context.Background(), plan); !errors.Is(err, ErrSegmentedPlanStoreWork) {
		t.Fatalf("work-limited Put() = %v", err)
	}
	byteLimits := DefaultSegmentedPlanStoreLimits()
	byteLimits.MaxSegmentBytes = 64
	byteStore, _ := NewMemorySegmentedPlanStore(byteLimits)
	if _, err := byteStore.Put(context.Background(), plan); !errors.Is(err, ErrPlanStoreLimit) {
		t.Fatalf("segment-byte-limited Put() = %v", err)
	}
	invalid := DefaultSegmentedPlanStoreLimits()
	invalid.MaxPlans = hardSegmentedMaxPlans + 1
	if _, err := NewMemorySegmentedPlanStore(invalid); !errors.Is(err, ErrSegmentedPlanStoreLimits) {
		t.Fatalf("hard-cap constructor = %v", err)
	}
}

func TestMemorySegmentedPlanStoreRejectsSemanticManifestCountCorruption(t *testing.T) {
	plan := segmentedStoreTestPlan(t)
	store, _ := NewMemorySegmentedPlanStore(DefaultSegmentedPlanStoreLimits())
	hash, err := store.Put(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := decodeSegmentedManifest(store.manifests[hash], hash)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Counts.SemanticNodes++
	store.manifests[hash], err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(context.Background(), hash); !errors.Is(err, ErrPlanStoreCorrupt) {
		t.Fatalf("semantic count-corrupt Get() = %v", err)
	}
}

func TestFileSegmentedPlanStorePublishesManifestLastIgnoresOrphansAndDetectsMissingSegments(t *testing.T) {
	directory := t.TempDir()
	limits := DefaultSegmentedPlanStoreLimits()
	limits.MaxPagesPerSegment = 2
	store, err := NewFileSegmentedPlanStore(directory, limits)
	if err != nil {
		t.Fatal(err)
	}
	// A content-addressed orphan models a crash after segment publication but
	// before the manifest commit. It is invisible and consumes no logical stats.
	orphan := []byte(`{"orphan":true}`)
	orphanHash := SegmentHash(sha256.Sum256(orphan))
	if err := os.WriteFile(store.segmentPath(orphanHash), orphan, 0o600); err != nil {
		t.Fatal(err)
	}
	if stats, err := store.Stats(); err != nil || stats != (PlanStoreStats{}) {
		t.Fatalf("orphan Stats() = %+v, %v", stats, err)
	}

	plan := segmentedStoreTestPlan(t)
	hash, err := store.Put(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(store.manifestPath(hash)); err != nil {
		t.Fatalf("manifest was not committed: %v", err)
	}
	entries, _ := os.ReadDir(directory)
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			t.Fatalf("temporary publication leaked: %s", entry.Name())
		}
	}
	metadata, err := store.PageMetadata(context.Background(), hash, 5)
	if err != nil || metadata.Page.Number != 5 || metadata.SegmentIndex != 2 {
		t.Fatalf("file PageMetadata() = %+v, %v", metadata, err)
	}
	manifestBytes, _ := readSegmentedFile(store.manifestPath(hash), limits.MaxPlanBytes)
	manifest, _ := decodeSegmentedManifest(manifestBytes, hash)
	if err := os.Remove(store.segmentPath(manifest.Segments[0].Hash)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(context.Background(), hash); !errors.Is(err, ErrSegmentedPlanStoreMissing) {
		t.Fatalf("missing file segment Get() = %v", err)
	}
	if _, err := store.Put(context.Background(), plan); !errors.Is(err, ErrSegmentedPlanStoreMissing) {
		t.Fatalf("existing-manifest Put() did not detect missing segment: %v", err)
	}
	if _, err := NewFileSegmentedPlanStore(directory, limits); !errors.Is(err, ErrSegmentedPlanStoreMissing) {
		t.Fatalf("reopen missing segment = %v", err)
	}
}

func TestFileSegmentedPlanStoreConcurrentRoundTripAndLegacyRejection(t *testing.T) {
	directory := t.TempDir()
	store, err := NewFileSegmentedPlanStore(directory, DefaultSegmentedPlanStoreLimits())
	if err != nil {
		t.Fatal(err)
	}
	plan := segmentedStoreTestPlan(t)
	want, _ := plan.Hash()
	const workers = 12
	start := make(chan struct{})
	errorsSeen := make(chan error, workers)
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			hash, err := store.Put(context.Background(), plan)
			if err == nil && hash != want {
				err = errors.New("unexpected segmented plan hash")
			}
			if err == nil {
				_, err = store.Get(context.Background(), hash)
			}
			errorsSeen <- err
		}()
	}
	close(start)
	wait.Wait()
	close(errorsSeen)
	for err := range errorsSeen {
		if err != nil {
			t.Errorf("concurrent segmented operation: %v", err)
		}
	}

	legacy := t.TempDir()
	if err := os.WriteFile(filepath.Join(legacy, want.String()+".json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFileSegmentedPlanStore(legacy, DefaultSegmentedPlanStoreLimits()); !errors.Is(err, ErrPlanStoreSchema) {
		t.Fatalf("legacy directory rejection = %v", err)
	}
}

func segmentedStoreTestPlan(t *testing.T) LayoutPlan {
	t.Helper()
	blocks := make([]VerticalFlowBlock, 5)
	for index := range blocks {
		blocks[index] = VerticalFlowBlock{Node: NodeID(index + 1), Key: NodeKey("@segment-" + string(rune('a'+index))),
			Instance: InstanceID("@segment-" + string(rune('a'+index))), Height: 15}
	}
	geometry, err := PlanVerticalFlow(VerticalFlowInput{PageSize: Size{Width: 100, Height: 20},
		Body: Rect{Width: 100, Height: 20}, Blocks: blocks})
	if err != nil {
		t.Fatal(err)
	}
	projection := geometry.Projection()
	links := make([]PlannedLink, len(projection.Fragments))
	for index, fragment := range projection.Fragments {
		links[index] = PlannedLink{Fragment: fragment.ID, Bounds: fragment.BorderBox,
			URI: "https://example.test/page", Source: fragment.Source}
	}
	plan, err := AttachLinks(geometry, nil, links)
	if err != nil {
		t.Fatal(err)
	}
	projection = plan.Projection()
	nodes := []SemanticNode{{ID: 1, Role: SemanticRoleDocument, Key: "@document", Instance: "@document"}}
	associations := make([]SemanticFragmentAssociation, len(projection.Fragments))
	reading := make([]ReadingOccurrence, len(projection.Fragments))
	pageIndexes := make(map[uint32]uint32)
	for index, fragment := range projection.Fragments {
		semantic := SemanticNodeID(index + 2)
		nodes = append(nodes, SemanticNode{ID: semantic, Parent: 1, Role: SemanticRoleLink,
			Key: fragment.Key, Instance: fragment.Instance, Source: fragment.Source})
		associations[index] = SemanticFragmentAssociation{Semantic: semantic, Page: fragment.Page, Fragment: fragment.ID}
		reading[index] = ReadingOccurrence{Semantic: semantic, Page: fragment.Page, Fragment: fragment.ID, ReadingIndex: pageIndexes[fragment.Page]}
		pageIndexes[fragment.Page]++
	}
	plan, err = AttachSemantics(plan, nodes, associations, reading)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}
