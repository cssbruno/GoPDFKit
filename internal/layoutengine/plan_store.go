// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	defaultPlanStoreMaxItems      uint64 = 1_024
	defaultPlanStoreMaxPlanBytes  uint64 = 64 << 20
	defaultPlanStoreMaxTotalBytes uint64 = 512 << 20
)

var (
	ErrPlanStoreNotFound      = errors.New("layoutengine: plan store item not found")
	ErrPlanStoreLimit         = errors.New("layoutengine: plan store limit exceeded")
	ErrPlanStoreCorrupt       = errors.New("layoutengine: corrupt plan store item")
	ErrPlanStoreSchema        = errors.New("layoutengine: plan store schema mismatch")
	ErrPlanStoreHashMismatch  = errors.New("layoutengine: plan store hash mismatch")
	ErrPlanStoreInvalidLimits = errors.New("layoutengine: invalid plan store limits")
)

// PlanStoreLimits are hard publication and loading budgets. All fields are
// required so a store can never silently become unbounded.
type PlanStoreLimits struct {
	MaxItems      uint64
	MaxPlanBytes  uint64
	MaxTotalBytes uint64
}

// DefaultPlanStoreLimits returns conservative limits by value so one caller
// cannot mutate another store's defaults.
func DefaultPlanStoreLimits() PlanStoreLimits {
	return PlanStoreLimits{
		MaxItems:      defaultPlanStoreMaxItems,
		MaxPlanBytes:  defaultPlanStoreMaxPlanBytes,
		MaxTotalBytes: defaultPlanStoreMaxTotalBytes,
	}
}

func (limits PlanStoreLimits) validate() error {
	if limits.MaxItems == 0 || limits.MaxPlanBytes == 0 || limits.MaxTotalBytes == 0 {
		return fmt.Errorf("%w: every limit must be positive", ErrPlanStoreInvalidLimits)
	}
	if limits.MaxPlanBytes > limits.MaxTotalBytes {
		return fmt.Errorf("%w: max plan bytes exceeds max total bytes", ErrPlanStoreInvalidLimits)
	}
	// io.LimitReader accepts int64, and loading needs one extra byte to detect
	// a file that grows beyond the configured maximum during a read.
	if limits.MaxPlanBytes >= uint64(^uint64(0)>>1) {
		return fmt.Errorf("%w: max plan bytes is too large", ErrPlanStoreInvalidLimits)
	}
	return nil
}

// PlanStoreStats reports canonical stored bytes, not implementation overhead.
type PlanStoreStats struct {
	Items uint64
	Bytes uint64
}

// PlanStore is an immutable content-addressed LayoutPlan store. Put is
// idempotent for an already-present plan. Implementations are safe for
// concurrent use and Get always reconstructs detached plan storage.
type PlanStore interface {
	Put(LayoutPlan) (PlanHash, error)
	Get(PlanHash) (LayoutPlan, error)
	Stats() (PlanStoreStats, error)
}

// MemoryPlanStore is a bounded in-memory PlanStore.
type MemoryPlanStore struct {
	mu     sync.RWMutex
	limits PlanStoreLimits
	items  map[PlanHash][]byte
	bytes  uint64
}

// NewMemoryPlanStore constructs an empty in-memory store with explicit hard
// limits.
func NewMemoryPlanStore(limits PlanStoreLimits) (*MemoryPlanStore, error) {
	if err := limits.validate(); err != nil {
		return nil, err
	}
	return &MemoryPlanStore{limits: limits, items: make(map[PlanHash][]byte)}, nil
}

// Put validates and canonically serializes a plan before publishing it under
// the SHA-256 digest of those exact bytes.
func (store *MemoryPlanStore) Put(plan LayoutPlan) (PlanHash, error) {
	hash, encoded, err := encodePlanForStore(plan, store.limits)
	if err != nil {
		return PlanHash{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if existing, ok := store.items[hash]; ok {
		if _, err := decodeStoredPlan(existing, hash); err != nil {
			return PlanHash{}, err
		}
		if !bytes.Equal(existing, encoded) {
			return PlanHash{}, corruptionError(ErrPlanStoreHashMismatch, "same digest has different canonical bytes")
		}
		return hash, nil
	}
	if err := checkPlanStoreAddition(store.limits, uint64(len(store.items)), store.bytes, uint64(len(encoded))); err != nil {
		return PlanHash{}, err
	}
	store.items[hash] = append([]byte(nil), encoded...)
	store.bytes += uint64(len(encoded))
	return hash, nil
}

// Get verifies the stored bytes and returns a newly reconstructed LayoutPlan.
func (store *MemoryPlanStore) Get(hash PlanHash) (LayoutPlan, error) {
	store.mu.RLock()
	encoded, ok := store.items[hash]
	if ok {
		encoded = append([]byte(nil), encoded...)
	}
	store.mu.RUnlock()
	if !ok {
		return LayoutPlan{}, fmt.Errorf("%w: %s", ErrPlanStoreNotFound, hash)
	}
	return decodeStoredPlan(encoded, hash)
}

func (store *MemoryPlanStore) Stats() (PlanStoreStats, error) {
	store.mu.RLock()
	defer store.mu.RUnlock()
	return PlanStoreStats{Items: uint64(len(store.items)), Bytes: store.bytes}, nil
}

// FilePlanStore persists one canonical JSON file per plan. Publication writes
// and fsyncs a private temporary file before atomically renaming it into the
// content-addressed namespace. Concurrency safety is provided per store
// instance; readers never observe a partially written plan.
type FilePlanStore struct {
	mu        sync.Mutex
	directory string
	limits    PlanStoreLimits
}

// NewFilePlanStore creates the directory when necessary and verifies every
// existing content-addressed item before returning it to callers.
func NewFilePlanStore(directory string, limits PlanStoreLimits) (*FilePlanStore, error) {
	if err := limits.validate(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(directory) == "" {
		return nil, errors.New("layoutengine: plan store directory is empty")
	}
	clean := filepath.Clean(directory)
	if err := os.MkdirAll(clean, 0o700); err != nil {
		return nil, fmt.Errorf("layoutengine: create plan store directory: %w", err)
	}
	info, err := os.Stat(clean)
	if err != nil {
		return nil, fmt.Errorf("layoutengine: stat plan store directory: %w", err)
	}
	if !info.IsDir() {
		return nil, errors.New("layoutengine: plan store path is not a directory")
	}
	store := &FilePlanStore{directory: clean, limits: limits}
	if _, err := store.scanLocked(); err != nil {
		return nil, err
	}
	return store, nil
}

func (store *FilePlanStore) Put(plan LayoutPlan) (PlanHash, error) {
	hash, encoded, err := encodePlanForStore(plan, store.limits)
	if err != nil {
		return PlanHash{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()

	path := store.path(hash)
	if _, err := os.Stat(path); err == nil {
		existing, readErr := store.readLocked(path)
		if readErr != nil {
			return PlanHash{}, readErr
		}
		if _, decodeErr := decodeStoredPlan(existing, hash); decodeErr != nil {
			return PlanHash{}, decodeErr
		}
		if !bytes.Equal(existing, encoded) {
			return PlanHash{}, corruptionError(ErrPlanStoreHashMismatch, "same digest has different canonical bytes")
		}
		return hash, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return PlanHash{}, fmt.Errorf("layoutengine: stat plan store item: %w", err)
	}

	stats, err := store.scanLocked()
	if err != nil {
		return PlanHash{}, err
	}
	if err := checkPlanStoreAddition(store.limits, stats.Items, stats.Bytes, uint64(len(encoded))); err != nil {
		return PlanHash{}, err
	}
	if err := store.publishLocked(path, encoded); err != nil {
		return PlanHash{}, err
	}
	return hash, nil
}

func (store *FilePlanStore) Get(hash PlanHash) (LayoutPlan, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	encoded, err := store.readLocked(store.path(hash))
	if err != nil {
		return LayoutPlan{}, err
	}
	return decodeStoredPlan(encoded, hash)
}

func (store *FilePlanStore) Stats() (PlanStoreStats, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.scanLocked()
}

func (store *FilePlanStore) path(hash PlanHash) string {
	return filepath.Join(store.directory, hash.String()+".json")
}

func (store *FilePlanStore) readLocked(path string) ([]byte, error) {
	file, err := os.Open(path) // #nosec G304 -- path is a content-addressed child of the validated store directory.
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", ErrPlanStoreNotFound, filepath.Base(path))
	}
	if err != nil {
		return nil, fmt.Errorf("layoutengine: open plan store item: %w", err)
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("layoutengine: stat plan store item: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, corruptionError(nil, "stored item is not a regular file")
	}
	if info.Size() < 0 || uint64(info.Size()) > store.limits.MaxPlanBytes { // #nosec G115 -- fixed-width conversion is bounded by the surrounding parser, planner, or resource invariant
		return nil, fmt.Errorf("%w: stored plan exceeds %d bytes", ErrPlanStoreLimit, store.limits.MaxPlanBytes)
	}
	reader := io.LimitReader(file, int64(store.limits.MaxPlanBytes)+1) // #nosec G115 -- fixed-width conversion is bounded by the surrounding parser, planner, or resource invariant
	encoded, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("layoutengine: read plan store item: %w", err)
	}
	if uint64(len(encoded)) > store.limits.MaxPlanBytes {
		return nil, fmt.Errorf("%w: stored plan exceeds %d bytes", ErrPlanStoreLimit, store.limits.MaxPlanBytes)
	}
	return encoded, nil
}

func (store *FilePlanStore) scanLocked() (PlanStoreStats, error) {
	entries, err := os.ReadDir(store.directory)
	if err != nil {
		return PlanStoreStats{}, fmt.Errorf("layoutengine: scan plan store: %w", err)
	}
	var stats PlanStoreStats
	for _, entry := range entries {
		hash, ok := planHashFromFilename(entry.Name())
		if !ok {
			continue
		}
		encoded, err := store.readLocked(filepath.Join(store.directory, entry.Name()))
		if err != nil {
			return PlanStoreStats{}, err
		}
		if _, err := decodeStoredPlan(encoded, hash); err != nil {
			return PlanStoreStats{}, fmt.Errorf("layoutengine: verify %s: %w", entry.Name(), err)
		}
		if stats.Items >= store.limits.MaxItems {
			return PlanStoreStats{}, fmt.Errorf("%w: store contains more than %d items", ErrPlanStoreLimit, store.limits.MaxItems)
		}
		length := uint64(len(encoded))
		if length > store.limits.MaxTotalBytes-stats.Bytes {
			return PlanStoreStats{}, fmt.Errorf("%w: store exceeds %d bytes", ErrPlanStoreLimit, store.limits.MaxTotalBytes)
		}
		stats.Items++
		stats.Bytes += length
	}
	return stats, nil
}

func (store *FilePlanStore) publishLocked(path string, encoded []byte) (err error) {
	temporary, err := os.CreateTemp(store.directory, ".plan-*.tmp")
	if err != nil {
		return fmt.Errorf("layoutengine: create plan store temporary file: %w", err)
	}
	temporaryName := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryName)
	}()
	if _, err := temporary.Write(encoded); err != nil {
		return fmt.Errorf("layoutengine: write plan store temporary file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("layoutengine: sync plan store temporary file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("layoutengine: close plan store temporary file: %w", err)
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return fmt.Errorf("layoutengine: publish plan store item: %w", err)
	}
	return nil
}

func encodePlanForStore(plan LayoutPlan, limits PlanStoreLimits) (PlanHash, []byte, error) {
	if err := plan.Validate(); err != nil {
		return PlanHash{}, nil, fmt.Errorf("layoutengine: store invalid plan: %w", err)
	}
	encoded, err := plan.CanonicalJSON()
	if err != nil {
		return PlanHash{}, nil, fmt.Errorf("layoutengine: serialize plan for store: %w", err)
	}
	if uint64(len(encoded)) > limits.MaxPlanBytes {
		return PlanHash{}, nil, fmt.Errorf("%w: plan has %d bytes, maximum is %d", ErrPlanStoreLimit, len(encoded), limits.MaxPlanBytes)
	}
	hash := sha256.Sum256(encoded)
	return hash, encoded, nil
}

func decodeStoredPlan(encoded []byte, expected PlanHash) (LayoutPlan, error) {
	actual := sha256.Sum256(encoded)
	if actual != expected {
		return LayoutPlan{}, corruptionError(ErrPlanStoreHashMismatch, fmt.Sprintf("content hash is %s, want %s", PlanHash(actual), expected))
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var projection LayoutPlanProjection
	if err := decoder.Decode(&projection); err != nil {
		return LayoutPlan{}, corruptionError(nil, "decode canonical projection: "+err.Error())
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return LayoutPlan{}, corruptionError(nil, err.Error())
	}
	if projection.SchemaVersion != LayoutPlanSchemaVersion {
		return LayoutPlan{}, corruptionError(ErrPlanStoreSchema, fmt.Sprintf("schema is %d, want %d", projection.SchemaVersion, LayoutPlanSchemaVersion))
	}
	if projection.PlannerVersion != PlannerVersion || projection.PainterContractVersion != PainterContractVersion {
		return LayoutPlan{}, corruptionError(ErrPlanStoreSchema, "planner or painter contract version mismatch")
	}
	plan, err := NewLayoutPlan(layoutPlanInputFromStoredProjection(projection))
	if err != nil {
		return LayoutPlan{}, corruptionError(nil, "invalid projection: "+err.Error())
	}
	canonical, err := plan.CanonicalJSON()
	if err != nil {
		return LayoutPlan{}, corruptionError(nil, "re-serialize projection: "+err.Error())
	}
	if !bytes.Equal(canonical, encoded) {
		return LayoutPlan{}, corruptionError(nil, "stored bytes are not the canonical projection")
	}
	return plan, nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var trailing json.RawMessage
	err := decoder.Decode(&trailing)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("decode trailing data: %w", err)
	}
	return errors.New("stored projection has trailing JSON data")
}

func layoutPlanInputFromStoredProjection(projection LayoutPlanProjection) LayoutPlanInput {
	return LayoutPlanInput{
		DeterministicInputs: projection.DeterministicInputs,
		Pages:               projection.Pages,
		Fragments:           projection.Fragments,
		Lines:               projection.Lines,
		PageRegions:         projection.PageRegions,
		GridTracks:          projection.GridTracks,
		Fonts:               projection.Fonts,
		GlyphRuns:           projection.GlyphRuns,
		ImageResources:      projection.ImageResources,
		Images:              projection.Images,
		Destinations:        projection.Destinations,
		Links:               projection.Links,
		Paths:               projection.Paths,
		Transforms:          projection.Transforms,
		Clips:               projection.Clips,
		Fills:               projection.Fills,
		Strokes:             projection.Strokes,
		Commands:            projection.Commands,
		Breaks:              projection.Breaks,
		Diagnostics:         projection.Diagnostics,
		SemanticNodes:       projection.SemanticNodes,
		SemanticFragments:   projection.SemanticFragments,
		ReadingOrder:        projection.ReadingOrder,
	}
}

func checkPlanStoreAddition(limits PlanStoreLimits, items, totalBytes, planBytes uint64) error {
	if planBytes > limits.MaxPlanBytes {
		return fmt.Errorf("%w: plan has %d bytes, maximum is %d", ErrPlanStoreLimit, planBytes, limits.MaxPlanBytes)
	}
	if items >= limits.MaxItems {
		return fmt.Errorf("%w: store contains %d items, maximum is %d", ErrPlanStoreLimit, items, limits.MaxItems)
	}
	if totalBytes > limits.MaxTotalBytes || planBytes > limits.MaxTotalBytes-totalBytes {
		return fmt.Errorf("%w: store would exceed %d bytes", ErrPlanStoreLimit, limits.MaxTotalBytes)
	}
	return nil
}

func corruptionError(kind error, detail string) error {
	if kind == nil {
		return fmt.Errorf("%w: %s", ErrPlanStoreCorrupt, detail)
	}
	return fmt.Errorf("%w: %w: %s", ErrPlanStoreCorrupt, kind, detail)
}

func planHashFromFilename(name string) (PlanHash, bool) {
	if len(name) != sha256.Size*2+len(".json") || !strings.HasSuffix(name, ".json") {
		return PlanHash{}, false
	}
	encoded := strings.TrimSuffix(name, ".json")
	if encoded != strings.ToLower(encoded) {
		return PlanHash{}, false
	}
	decoded, err := hex.DecodeString(encoded)
	if err != nil || len(decoded) != sha256.Size {
		return PlanHash{}, false
	}
	var hash PlanHash
	copy(hash[:], decoded)
	return hash, true
}
