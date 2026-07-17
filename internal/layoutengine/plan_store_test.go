// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestMemoryPlanStoreRoundTripIsCanonicalDetachedAndIdempotent(t *testing.T) {
	plan := planStoreTestPlan(t, "first")
	store, err := NewMemoryPlanStore(DefaultPlanStoreLimits())
	if err != nil {
		t.Fatalf("NewMemoryPlanStore() = %v", err)
	}
	wantHash, _ := plan.Hash()
	hash, err := store.Put(plan)
	if err != nil || hash != wantHash {
		t.Fatalf("Put() = %s, %v; want %s", hash, err, wantHash)
	}
	encoded, _ := plan.CanonicalJSON()
	stats, _ := store.Stats()
	if stats != (PlanStoreStats{Items: 1, Bytes: uint64(len(encoded))}) {
		t.Fatalf("Stats() = %+v", stats)
	}
	if secondHash, err := store.Put(plan); err != nil || secondHash != hash {
		t.Fatalf("idempotent Put() = %s, %v", secondHash, err)
	}
	if after, _ := store.Stats(); after != stats {
		t.Fatalf("idempotent Put changed stats: %+v != %+v", after, stats)
	}

	loaded, err := store.Get(hash)
	if err != nil {
		t.Fatalf("Get() = %v", err)
	}
	projection := loaded.Projection()
	projection.Pages[0].Number = 99
	projection.Diagnostics[0].Evidence[0].Value = "mutated"
	reloaded, err := store.Get(hash)
	if err != nil {
		t.Fatalf("second Get() = %v", err)
	}
	if got, _ := reloaded.Hash(); got != hash {
		t.Fatalf("detached mutation changed stored plan: %s != %s", got, hash)
	}
	if reloaded.Projection().Pages[0].Number != 1 || reloaded.Projection().Diagnostics[0].Evidence[0].Value == "mutated" {
		t.Fatal("Get exposed stored plan aliases")
	}

	var absent PlanHash
	absent[0] = 1
	if _, err := store.Get(absent); !errors.Is(err, ErrPlanStoreNotFound) {
		t.Fatalf("missing Get() = %v", err)
	}
	if string(store.items[hash]) != string(encoded) {
		t.Fatal("store did not retain the exact canonical serialization")
	}
}

func TestMemoryPlanStoreRoundTripsCoreGlyphColor(t *testing.T) {
	input := coreGlyphPlanInput()
	input.GlyphRuns[0].Color = CoreRGBColor{R: 17, G: 34, B: 51, Set: true}
	plan, err := NewLayoutPlan(input)
	if err != nil {
		t.Fatalf("NewLayoutPlan() = %v", err)
	}
	store, err := NewMemoryPlanStore(DefaultPlanStoreLimits())
	if err != nil {
		t.Fatalf("NewMemoryPlanStore() = %v", err)
	}
	hash, err := store.Put(plan)
	if err != nil {
		t.Fatalf("Put() = %v", err)
	}
	loaded, err := store.Get(hash)
	if err != nil {
		t.Fatalf("Get() = %v", err)
	}
	if got := loaded.Projection().GlyphRuns[0].Color; got != input.GlyphRuns[0].Color {
		t.Fatalf("stored color = %#v, want %#v", got, input.GlyphRuns[0].Color)
	}
}

func TestMemoryPlanStoreEnforcesEveryLimit(t *testing.T) {
	first := planStoreTestPlan(t, "first")
	second := planStoreTestPlan(t, "second")
	firstJSON, _ := first.CanonicalJSON()
	secondJSON, _ := second.CanonicalJSON()
	maxPlan := uint64(len(firstJSON))
	if uint64(len(secondJSON)) > maxPlan {
		maxPlan = uint64(len(secondJSON))
	}

	itemStore, err := NewMemoryPlanStore(PlanStoreLimits{MaxItems: 1, MaxPlanBytes: maxPlan, MaxTotalBytes: maxPlan * 2})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := itemStore.Put(first); err != nil {
		t.Fatal(err)
	}
	if _, err := itemStore.Put(second); !errors.Is(err, ErrPlanStoreLimit) {
		t.Fatalf("item-limited Put() = %v", err)
	}

	planStore, err := NewMemoryPlanStore(PlanStoreLimits{
		MaxItems: 1, MaxPlanBytes: uint64(len(firstJSON)) - 1, MaxTotalBytes: uint64(len(firstJSON)) - 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := planStore.Put(first); !errors.Is(err, ErrPlanStoreLimit) {
		t.Fatalf("plan-byte-limited Put() = %v", err)
	}

	totalStore, err := NewMemoryPlanStore(PlanStoreLimits{
		MaxItems: 2, MaxPlanBytes: maxPlan, MaxTotalBytes: uint64(len(firstJSON) + len(secondJSON) - 1),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := totalStore.Put(first); err != nil {
		t.Fatal(err)
	}
	if _, err := totalStore.Put(second); !errors.Is(err, ErrPlanStoreLimit) {
		t.Fatalf("total-byte-limited Put() = %v", err)
	}

	badLimits := []PlanStoreLimits{
		{},
		{MaxItems: 1, MaxPlanBytes: 2, MaxTotalBytes: 1},
		{MaxItems: 1, MaxPlanBytes: ^uint64(0), MaxTotalBytes: ^uint64(0)},
	}
	for _, limits := range badLimits {
		if _, err := NewMemoryPlanStore(limits); !errors.Is(err, ErrPlanStoreInvalidLimits) {
			t.Fatalf("NewMemoryPlanStore(%+v) = %v", limits, err)
		}
	}
}

func TestMemoryPlanStoreRejectsHashSchemaAndCanonicalCorruption(t *testing.T) {
	plan := planStoreTestPlan(t, "corruption")
	store, _ := NewMemoryPlanStore(DefaultPlanStoreLimits())
	hash, err := store.Put(plan)
	if err != nil {
		t.Fatal(err)
	}
	store.items[hash][0] ^= 1
	if _, err := store.Get(hash); !errors.Is(err, ErrPlanStoreCorrupt) || !errors.Is(err, ErrPlanStoreHashMismatch) {
		t.Fatalf("hash-corrupt Get() = %v", err)
	}

	projection := plan.Projection()
	projection.SchemaVersion--
	wrongSchema, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	schemaHash := PlanHash(sha256.Sum256(wrongSchema))
	store.items[schemaHash] = wrongSchema
	if _, err := store.Get(schemaHash); !errors.Is(err, ErrPlanStoreCorrupt) || !errors.Is(err, ErrPlanStoreSchema) {
		t.Fatalf("schema-corrupt Get() = %v", err)
	}

	for name, mutate := range map[string]func(*LayoutPlanProjection){
		"planner": func(value *LayoutPlanProjection) { value.PlannerVersion = "layoutengine/unknown" },
		"painter": func(value *LayoutPlanProjection) { value.PainterContractVersion = "display-list/unknown" },
	} {
		projection := plan.Projection()
		mutate(&projection)
		wrongVersion, marshalErr := json.Marshal(projection)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		versionHash := PlanHash(sha256.Sum256(wrongVersion))
		store.items[versionHash] = wrongVersion
		if _, getErr := store.Get(versionHash); !errors.Is(getErr, ErrPlanStoreCorrupt) || !errors.Is(getErr, ErrPlanStoreSchema) {
			t.Fatalf("%s-version-corrupt Get() = %v", name, getErr)
		}
	}

	canonical, _ := plan.CanonicalJSON()
	noncanonical := append(append([]byte(nil), canonical...), '\n')
	noncanonicalHash := PlanHash(sha256.Sum256(noncanonical))
	store.items[noncanonicalHash] = noncanonical
	if _, err := store.Get(noncanonicalHash); !errors.Is(err, ErrPlanStoreCorrupt) {
		t.Fatalf("noncanonical Get() = %v", err)
	}
}

func TestFilePlanStorePublishesAtomicallyReopensAndDetectsCorruption(t *testing.T) {
	directory := t.TempDir()
	limits := DefaultPlanStoreLimits()
	store, err := NewFilePlanStore(directory, limits)
	if err != nil {
		t.Fatalf("NewFilePlanStore() = %v", err)
	}
	plan := planStoreTestPlan(t, "filesystem")
	hash, err := store.Put(plan)
	if err != nil {
		t.Fatalf("Put() = %v", err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != hash.String()+".json" {
		t.Fatalf("published entries = %v", entryNames(entries))
	}
	reopened, err := NewFilePlanStore(directory, limits)
	if err != nil {
		t.Fatalf("reopen = %v", err)
	}
	loaded, err := reopened.Get(hash)
	if err != nil {
		t.Fatalf("reopened Get() = %v", err)
	}
	if got, _ := loaded.Hash(); got != hash {
		t.Fatalf("loaded hash = %s, want %s", got, hash)
	}

	path := filepath.Join(directory, hash.String()+".json")
	if err := os.WriteFile(path, []byte(`{"schema_version":5}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.Get(hash); !errors.Is(err, ErrPlanStoreCorrupt) || !errors.Is(err, ErrPlanStoreHashMismatch) {
		t.Fatalf("corrupt filesystem Get() = %v", err)
	}
	if _, err := NewFilePlanStore(directory, limits); !errors.Is(err, ErrPlanStoreCorrupt) {
		t.Fatalf("constructor with corrupt item = %v", err)
	}
}

func TestFilePlanStoreConcurrentPutAndGet(t *testing.T) {
	store, err := NewFilePlanStore(t.TempDir(), DefaultPlanStoreLimits())
	if err != nil {
		t.Fatal(err)
	}
	plan := planStoreTestPlan(t, "concurrent")
	wantHash, _ := plan.Hash()

	const workers = 24
	start := make(chan struct{})
	errorsSeen := make(chan error, workers)
	var wait sync.WaitGroup
	for i := 0; i < workers; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			hash, err := store.Put(plan)
			if err != nil {
				errorsSeen <- err
				return
			}
			if hash != wantHash {
				errorsSeen <- errors.New("Put returned a different hash")
				return
			}
			loaded, err := store.Get(hash)
			if err != nil {
				errorsSeen <- err
				return
			}
			if got, _ := loaded.Hash(); got != hash {
				errorsSeen <- errors.New("Get returned a different plan")
			}
		}()
	}
	close(start)
	wait.Wait()
	close(errorsSeen)
	for err := range errorsSeen {
		t.Errorf("concurrent operation: %v", err)
	}
	stats, err := store.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.Items != 1 {
		t.Fatalf("Stats() = %+v", stats)
	}
}

func TestFilePlanStoreLimitAndSchemaValidationSurviveRestart(t *testing.T) {
	plan := planStoreTestPlan(t, "limit")
	encoded, _ := plan.CanonicalJSON()
	limits := PlanStoreLimits{MaxItems: 1, MaxPlanBytes: uint64(len(encoded)), MaxTotalBytes: uint64(len(encoded))}
	directory := t.TempDir()
	store, err := NewFilePlanStore(directory, limits)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Put(plan); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFilePlanStore(directory, PlanStoreLimits{
		MaxItems: 1, MaxPlanBytes: uint64(len(encoded)) - 1, MaxTotalBytes: uint64(len(encoded)) - 1,
	}); !errors.Is(err, ErrPlanStoreLimit) {
		t.Fatalf("reopen under smaller byte budget = %v", err)
	}

	wrong := plan.Projection()
	wrong.SchemaVersion--
	wrongJSON, _ := json.Marshal(wrong)
	wrongHash := PlanHash(sha256.Sum256(wrongJSON))
	wrongDirectory := t.TempDir()
	if err := os.WriteFile(filepath.Join(wrongDirectory, wrongHash.String()+".json"), wrongJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFilePlanStore(wrongDirectory, DefaultPlanStoreLimits()); !errors.Is(err, ErrPlanStoreSchema) || !errors.Is(err, ErrPlanStoreCorrupt) {
		t.Fatalf("reopen wrong-schema store = %v", err)
	}
}

func planStoreTestPlan(t *testing.T, message string) LayoutPlan {
	t.Helper()
	input := testPlanInput()
	input.Diagnostics[0].Message = message
	plan, err := NewLayoutPlan(input)
	if err != nil {
		t.Fatalf("NewLayoutPlan() = %v", err)
	}
	return plan
}

func entryNames(entries []os.DirEntry) []string {
	names := make([]string, len(entries))
	for i, entry := range entries {
		names[i] = entry.Name()
	}
	return names
}

var (
	_ PlanStore = (*MemoryPlanStore)(nil)
	_ PlanStore = (*FilePlanStore)(nil)
)
