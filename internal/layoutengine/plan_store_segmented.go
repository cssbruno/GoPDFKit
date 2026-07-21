// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

const (
	SegmentedPlanManifestFormatVersion uint16 = 1
	SegmentedPlanSegmentFormatVersion  uint16 = 1
	segmentedManifestSuffix                   = ".manifest.json"
	segmentedSegmentSuffix                    = ".segment.json"
	hardSegmentedMaxPlans              uint64 = 65_536
	hardSegmentedMaxPlanBytes          uint64 = 1 << 30
	hardSegmentedMaxTotalBytes         uint64 = 16 << 30
	hardSegmentedMaxSegmentBytes       uint64 = 128 << 20
	hardSegmentedMaxSegments           uint32 = 1_000_000
	hardSegmentedMaxPagesPerSegment    uint32 = 1_024
	hardSegmentedMaxWork               uint64 = 100_000_000
)

var (
	ErrSegmentedPlanStoreLimits  = errors.New("layoutengine: invalid segmented plan store limits")
	ErrSegmentedPlanStoreWork    = errors.New("layoutengine: segmented plan store work limit exceeded")
	ErrSegmentedPlanStoreMissing = errors.New("layoutengine: segmented plan store segment missing")
)

type SegmentedPlanStoreLimits struct {
	MaxPlans           uint64
	MaxPlanBytes       uint64
	MaxTotalBytes      uint64
	MaxSegmentBytes    uint64
	MaxSegmentsPerPlan uint32
	MaxPagesPerSegment uint32
	MaxWork            uint64
}

func DefaultSegmentedPlanStoreLimits() SegmentedPlanStoreLimits {
	return SegmentedPlanStoreLimits{MaxPlans: 1_024, MaxPlanBytes: 256 << 20, MaxTotalBytes: 1 << 30,
		MaxSegmentBytes: 32 << 20, MaxSegmentsPerPlan: 65_536, MaxPagesPerSegment: 16, MaxWork: 20_000_000}
}

func (limits SegmentedPlanStoreLimits) validate() error {
	if limits.MaxPlans == 0 || limits.MaxPlanBytes == 0 || limits.MaxTotalBytes == 0 || limits.MaxSegmentBytes == 0 ||
		limits.MaxSegmentsPerPlan == 0 || limits.MaxPagesPerSegment == 0 || limits.MaxWork == 0 {
		return fmt.Errorf("%w: every bound must be positive", ErrSegmentedPlanStoreLimits)
	}
	if limits.MaxPlanBytes > limits.MaxTotalBytes || limits.MaxSegmentBytes > limits.MaxPlanBytes ||
		limits.MaxPlanBytes >= uint64(^uint64(0)>>1) || limits.MaxSegmentBytes >= uint64(^uint64(0)>>1) {
		return fmt.Errorf("%w: byte bounds are inconsistent or too large", ErrSegmentedPlanStoreLimits)
	}
	if limits.MaxPlans > hardSegmentedMaxPlans || limits.MaxPlanBytes > hardSegmentedMaxPlanBytes ||
		limits.MaxTotalBytes > hardSegmentedMaxTotalBytes || limits.MaxSegmentBytes > hardSegmentedMaxSegmentBytes ||
		limits.MaxSegmentsPerPlan > hardSegmentedMaxSegments || limits.MaxPagesPerSegment > hardSegmentedMaxPagesPerSegment ||
		limits.MaxWork > hardSegmentedMaxWork {
		return fmt.Errorf("%w: caller bounds exceed implementation hard caps", ErrSegmentedPlanStoreLimits)
	}
	return nil
}

type segmentedBudget struct {
	ctx   context.Context
	limit uint64
	used  uint64
}

func (budget *segmentedBudget) charge(amount uint64) error {
	if err := budget.ctx.Err(); err != nil {
		return newPlanningError(err, Diagnostic{Code: DiagnosticCanceled, Severity: SeverityError, Stage: StageLayout,
			Message: "segmented plan store operation was canceled"})
	}
	if amount > budget.limit-budget.used {
		return fmt.Errorf("%w: used=%d requested=%d limit=%d", ErrSegmentedPlanStoreWork, budget.used, amount, budget.limit)
	}
	budget.used += amount
	return nil
}

type SegmentHash [sha256.Size]byte

func (hash SegmentHash) String() string { return hex.EncodeToString(hash[:]) }

type SegmentedPlanPageMetadata struct {
	PlanHash     PlanHash    `json:"plan_hash"`
	Page         PlannedPage `json:"page"`
	SegmentIndex uint32      `json:"segment_index"`
	SegmentHash  SegmentHash `json:"segment_hash"`
}

type segmentedPlanCounts struct {
	Pages             uint32 `json:"pages"`
	Fragments         uint32 `json:"fragments"`
	Lines             uint32 `json:"lines"`
	PageRegions       uint32 `json:"page_regions"`
	GridTracks        uint32 `json:"grid_tracks"`
	GlyphRuns         uint32 `json:"glyph_runs"`
	Images            uint32 `json:"images"`
	Links             uint32 `json:"links"`
	Commands          uint32 `json:"commands"`
	Breaks            uint32 `json:"breaks"`
	Diagnostics       uint32 `json:"diagnostics"`
	SemanticNodes     uint32 `json:"semantic_nodes"`
	SemanticFragments uint32 `json:"semantic_fragments"`
	ReadingOrder      uint32 `json:"reading_order"`
}

type segmentedPlanReference struct {
	Index      uint32      `json:"index"`
	PageStart  uint32      `json:"page_start"`
	PageCount  uint32      `json:"page_count"`
	ByteLength uint64      `json:"byte_length"`
	Hash       SegmentHash `json:"hash"`
}

type segmentedPlanManifest struct {
	FormatVersion          uint16                      `json:"format_version"`
	PlanSchemaVersion      uint16                      `json:"plan_schema_version"`
	PlannerVersion         string                      `json:"planner_version"`
	PainterContractVersion string                      `json:"painter_contract_version"`
	PlanHash               PlanHash                    `json:"plan_hash"`
	DeterministicInputs    *DeterministicInputManifest `json:"deterministic_inputs,omitempty"`
	Counts                 segmentedPlanCounts         `json:"counts"`
	Pages                  []PlannedPage               `json:"pages,omitempty"`
	Fonts                  []CoreFontResource          `json:"fonts,omitempty"`
	ImageResources         []ImageResource             `json:"image_resources,omitempty"`
	Destinations           []PlannedDestination        `json:"destinations,omitempty"`
	Paths                  []PlannedPath               `json:"paths,omitempty"`
	Transforms             []Transform                 `json:"transforms,omitempty"`
	Clips                  []PlannedClip               `json:"clips,omitempty"`
	Fills                  []PlannedFill               `json:"fills,omitempty"`
	Strokes                []PlannedStroke             `json:"strokes,omitempty"`
	SemanticNodes          []SemanticNode              `json:"semantic_nodes,omitempty"`
	Segments               []segmentedPlanReference    `json:"segments,omitempty"`
}

type indexedFragment struct {
	Index uint32   `json:"index"`
	Value Fragment `json:"value"`
}
type indexedLine struct {
	Index uint32      `json:"index"`
	Value PlannedLine `json:"value"`
}
type indexedGridTrack struct {
	Index uint32           `json:"index"`
	Value PlannedGridTrack `json:"value"`
}
type indexedPageRegion struct {
	Index uint32            `json:"index"`
	Value PlannedPageRegion `json:"value"`
}
type indexedGlyphRun struct {
	Index uint32       `json:"index"`
	Value CoreGlyphRun `json:"value"`
}
type indexedImage struct {
	Index uint32       `json:"index"`
	Value PlannedImage `json:"value"`
}
type indexedLink struct {
	Index uint32      `json:"index"`
	Value PlannedLink `json:"value"`
}
type indexedCommand struct {
	Index uint32         `json:"index"`
	Value DisplayCommand `json:"value"`
}
type indexedBreak struct {
	Index uint32        `json:"index"`
	Value BreakDecision `json:"value"`
}
type indexedDiagnostic struct {
	Index uint32     `json:"index"`
	Value Diagnostic `json:"value"`
}
type indexedReadingOccurrence struct {
	Index uint32            `json:"index"`
	Value ReadingOccurrence `json:"value"`
}
type indexedSemanticFragment struct {
	Index uint32                      `json:"index"`
	Value SemanticFragmentAssociation `json:"value"`
}

type segmentedPlanPayload struct {
	FormatVersion     uint16                     `json:"format_version"`
	PlanHash          PlanHash                   `json:"plan_hash"`
	Index             uint32                     `json:"index"`
	PageStart         uint32                     `json:"page_start"`
	Pages             []PlannedPage              `json:"pages"`
	Fragments         []indexedFragment          `json:"fragments,omitempty"`
	Lines             []indexedLine              `json:"lines,omitempty"`
	PageRegions       []indexedPageRegion        `json:"page_regions,omitempty"`
	GridTracks        []indexedGridTrack         `json:"grid_tracks,omitempty"`
	GlyphRuns         []indexedGlyphRun          `json:"glyph_runs,omitempty"`
	Images            []indexedImage             `json:"images,omitempty"`
	Links             []indexedLink              `json:"links,omitempty"`
	Commands          []indexedCommand           `json:"commands,omitempty"`
	Breaks            []indexedBreak             `json:"breaks,omitempty"`
	Diagnostics       []indexedDiagnostic        `json:"diagnostics,omitempty"`
	ReadingOrder      []indexedReadingOccurrence `json:"reading_order,omitempty"`
	SemanticFragments []indexedSemanticFragment  `json:"semantic_fragments,omitempty"`
}

type segmentedEncodedPlan struct {
	hash          PlanHash
	manifest      []byte
	manifestValue segmentedPlanManifest
	segments      map[SegmentHash][]byte
	totalBytes    uint64
}

type SegmentedPlanStore interface {
	Put(context.Context, LayoutPlan) (PlanHash, error)
	Get(context.Context, PlanHash) (LayoutPlan, error)
	PageMetadata(context.Context, PlanHash, uint32) (SegmentedPlanPageMetadata, error)
	Stats() (PlanStoreStats, error)
}

type MemorySegmentedPlanStore struct {
	mu        sync.RWMutex
	limits    SegmentedPlanStoreLimits
	manifests map[PlanHash][]byte
	segments  map[SegmentHash][]byte
	bytes     uint64
}

func NewMemorySegmentedPlanStore(limits SegmentedPlanStoreLimits) (*MemorySegmentedPlanStore, error) {
	if err := limits.validate(); err != nil {
		return nil, err
	}
	return &MemorySegmentedPlanStore{limits: limits, manifests: make(map[PlanHash][]byte), segments: make(map[SegmentHash][]byte)}, nil
}

func (store *MemorySegmentedPlanStore) Put(ctx context.Context, plan LayoutPlan) (PlanHash, error) {
	encoded, err := encodeSegmentedPlan(ctx, plan, store.limits)
	if err != nil {
		return PlanHash{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if existing, ok := store.manifests[encoded.hash]; ok {
		if !bytes.Equal(existing, encoded.manifest) {
			return PlanHash{}, corruptionError(ErrPlanStoreHashMismatch, "segmented manifest differs for one plan hash")
		}
		for segmentHash, want := range encoded.segments {
			got, exists := store.segments[segmentHash]
			if !exists || !bytes.Equal(got, want) {
				return PlanHash{}, corruptionError(ErrPlanStoreHashMismatch, "retained segmented payload is missing or corrupt")
			}
		}
		return encoded.hash, nil
	}
	if uint64(len(store.manifests)) >= store.limits.MaxPlans {
		return PlanHash{}, ErrPlanStoreLimit
	}
	addition := uint64(len(encoded.manifest))
	for hash, payload := range encoded.segments {
		if _, ok := store.segments[hash]; !ok {
			addition += uint64(len(payload))
		}
	}
	if store.bytes > store.limits.MaxTotalBytes || addition > store.limits.MaxTotalBytes-store.bytes {
		return PlanHash{}, ErrPlanStoreLimit
	}
	for hash, payload := range encoded.segments {
		if _, ok := store.segments[hash]; !ok {
			store.segments[hash] = append([]byte(nil), payload...)
		}
	}
	store.manifests[encoded.hash] = append([]byte(nil), encoded.manifest...)
	store.bytes += addition
	return encoded.hash, nil
}

func (store *MemorySegmentedPlanStore) Get(ctx context.Context, hash PlanHash) (LayoutPlan, error) {
	store.mu.RLock()
	manifest, ok := store.manifests[hash]
	manifest = append([]byte(nil), manifest...)
	segments := make(map[SegmentHash][]byte)
	if ok {
		value, err := decodeSegmentedManifest(manifest, hash)
		if err == nil {
			for _, reference := range value.Segments {
				segments[reference.Hash] = append([]byte(nil), store.segments[reference.Hash]...)
			}
		}
	}
	store.mu.RUnlock()
	if !ok {
		return LayoutPlan{}, fmt.Errorf("%w: %s", ErrPlanStoreNotFound, hash)
	}
	return reconstructSegmentedPlan(ctx, hash, manifest, func(reference segmentedPlanReference) ([]byte, error) {
		payload, exists := segments[reference.Hash]
		if !exists || len(payload) == 0 {
			return nil, fmt.Errorf("%w: %s", ErrSegmentedPlanStoreMissing, reference.Hash)
		}
		return payload, nil
	}, store.limits)
}

func (store *MemorySegmentedPlanStore) PageMetadata(ctx context.Context, hash PlanHash, page uint32) (SegmentedPlanPageMetadata, error) {
	store.mu.RLock()
	encoded, ok := store.manifests[hash]
	encoded = append([]byte(nil), encoded...)
	store.mu.RUnlock()
	if !ok {
		return SegmentedPlanPageMetadata{}, ErrPlanStoreNotFound
	}
	return segmentedPageMetadata(ctx, encoded, hash, page, store.limits)
}

func (store *MemorySegmentedPlanStore) Stats() (PlanStoreStats, error) {
	store.mu.RLock()
	defer store.mu.RUnlock()
	return PlanStoreStats{Items: uint64(len(store.manifests)), Bytes: store.bytes}, nil
}

type FileSegmentedPlanStore struct {
	mu               sync.Mutex
	directory        string
	segmentDirectory string
	limits           SegmentedPlanStoreLimits
}

func NewFileSegmentedPlanStore(directory string, limits SegmentedPlanStoreLimits) (*FileSegmentedPlanStore, error) {
	if err := limits.validate(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(directory) == "" {
		return nil, errors.New("layoutengine: segmented plan store directory is empty")
	}
	clean := filepath.Clean(directory)
	if err := os.MkdirAll(clean, 0o755); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(clean)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if _, legacy := planHashFromFilename(entry.Name()); legacy {
			return nil, fmt.Errorf("%w: legacy monolithic item %s requires a separate store directory", ErrPlanStoreSchema, entry.Name())
		}
	}
	segmentDirectory := filepath.Join(clean, "segments")
	if err := os.MkdirAll(segmentDirectory, 0o755); err != nil {
		return nil, err
	}
	store := &FileSegmentedPlanStore{directory: clean, segmentDirectory: segmentDirectory, limits: limits}
	if _, err := store.scanLocked(context.Background()); err != nil {
		return nil, err
	}
	return store, nil
}

func (store *FileSegmentedPlanStore) Put(ctx context.Context, plan LayoutPlan) (PlanHash, error) {
	encoded, err := encodeSegmentedPlan(ctx, plan, store.limits)
	if err != nil {
		return PlanHash{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	manifestPath := store.manifestPath(encoded.hash)
	if existing, readErr := readSegmentedFile(manifestPath, store.limits.MaxPlanBytes); readErr == nil {
		if !bytes.Equal(existing, encoded.manifest) {
			return PlanHash{}, corruptionError(ErrPlanStoreHashMismatch, "segmented manifest differs")
		}
		if _, scanErr := store.scanLocked(ctx); scanErr != nil {
			return PlanHash{}, scanErr
		}
		return encoded.hash, nil
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return PlanHash{}, readErr
	}
	stats, err := store.scanLocked(ctx)
	if err != nil {
		return PlanHash{}, err
	}
	if stats.Items >= store.limits.MaxPlans {
		return PlanHash{}, ErrPlanStoreLimit
	}
	addition := uint64(len(encoded.manifest))
	for hash, payload := range encoded.segments {
		if _, err := os.Stat(store.segmentPath(hash)); errors.Is(err, os.ErrNotExist) {
			addition += uint64(len(payload))
		} else if err != nil {
			return PlanHash{}, err
		}
	}
	if stats.Bytes > store.limits.MaxTotalBytes || addition > store.limits.MaxTotalBytes-stats.Bytes {
		return PlanHash{}, ErrPlanStoreLimit
	}
	// Segment blobs are published first under their content hashes. The
	// manifest rename below is the commit point: readers cannot discover a
	// partially published plan. A failed manifest publication may leave safe
	// orphan blobs; scans and Stats count only committed-manifest references, so
	// those orphans never consume the logical capacity budget.
	publisher := &FilePlanStore{directory: store.segmentDirectory}
	hashes := make([]SegmentHash, 0, len(encoded.segments))
	for hash := range encoded.segments {
		hashes = append(hashes, hash)
	}
	sort.Slice(hashes, func(i, j int) bool { return hashes[i].String() < hashes[j].String() })
	for _, hash := range hashes {
		path := store.segmentPath(hash)
		if existing, err := readSegmentedFile(path, store.limits.MaxSegmentBytes); err == nil {
			if !bytes.Equal(existing, encoded.segments[hash]) {
				return PlanHash{}, corruptionError(ErrPlanStoreHashMismatch, "existing segment bytes differ")
			}
		} else if errors.Is(err, os.ErrNotExist) {
			if err := publisher.publishLocked(path, encoded.segments[hash]); err != nil {
				return PlanHash{}, err
			}
		} else {
			return PlanHash{}, err
		}
	}
	manifestPublisher := &FilePlanStore{directory: store.directory}
	if err := manifestPublisher.publishLocked(manifestPath, encoded.manifest); err != nil {
		return PlanHash{}, err
	}
	return encoded.hash, nil
}

func (store *FileSegmentedPlanStore) Get(ctx context.Context, hash PlanHash) (LayoutPlan, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	manifest, err := readSegmentedFile(store.manifestPath(hash), store.limits.MaxPlanBytes)
	if errors.Is(err, os.ErrNotExist) {
		return LayoutPlan{}, ErrPlanStoreNotFound
	}
	if err != nil {
		return LayoutPlan{}, err
	}
	return reconstructSegmentedPlan(ctx, hash, manifest, func(reference segmentedPlanReference) ([]byte, error) {
		payload, err := readSegmentedFile(store.segmentPath(reference.Hash), store.limits.MaxSegmentBytes)
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrSegmentedPlanStoreMissing, reference.Hash)
		}
		return payload, err
	}, store.limits)
}

func (store *FileSegmentedPlanStore) PageMetadata(ctx context.Context, hash PlanHash, page uint32) (SegmentedPlanPageMetadata, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	manifest, err := readSegmentedFile(store.manifestPath(hash), store.limits.MaxPlanBytes)
	if errors.Is(err, os.ErrNotExist) {
		return SegmentedPlanPageMetadata{}, ErrPlanStoreNotFound
	}
	if err != nil {
		return SegmentedPlanPageMetadata{}, err
	}
	return segmentedPageMetadata(ctx, manifest, hash, page, store.limits)
}

func (store *FileSegmentedPlanStore) Stats() (PlanStoreStats, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.scanLocked(context.Background())
}
func (store *FileSegmentedPlanStore) manifestPath(hash PlanHash) string {
	return filepath.Join(store.directory, hash.String()+segmentedManifestSuffix)
}
func (store *FileSegmentedPlanStore) segmentPath(hash SegmentHash) string {
	return filepath.Join(store.segmentDirectory, hash.String()+segmentedSegmentSuffix)
}

func (store *FileSegmentedPlanStore) scanLocked(ctx context.Context) (PlanStoreStats, error) {
	entries, err := os.ReadDir(store.directory)
	if err != nil {
		return PlanStoreStats{}, err
	}
	stats := PlanStoreStats{}
	referenced := make(map[SegmentHash]uint64)
	for _, entry := range entries {
		hash, ok := segmentedPlanHashFromFilename(entry.Name())
		if !ok {
			continue
		}
		manifest, err := readSegmentedFile(filepath.Join(store.directory, entry.Name()), store.limits.MaxPlanBytes)
		if err != nil {
			return PlanStoreStats{}, err
		}
		value, err := decodeSegmentedManifest(manifest, hash)
		if err != nil {
			return PlanStoreStats{}, err
		}
		if err := validateSegmentedManifestLimits(value, store.limits); err != nil {
			return PlanStoreStats{}, err
		}
		stats.Items++
		if uint64(len(manifest)) > store.limits.MaxTotalBytes-stats.Bytes {
			return PlanStoreStats{}, ErrPlanStoreLimit
		}
		stats.Bytes += uint64(len(manifest))
		if stats.Items > store.limits.MaxPlans {
			return PlanStoreStats{}, ErrPlanStoreLimit
		}
		for _, reference := range value.Segments {
			referenced[reference.Hash] = reference.ByteLength
		}
	}
	for hash, length := range referenced {
		if err := ctx.Err(); err != nil {
			return PlanStoreStats{}, err
		}
		payload, err := readSegmentedFile(store.segmentPath(hash), store.limits.MaxSegmentBytes)
		if errors.Is(err, os.ErrNotExist) {
			return PlanStoreStats{}, fmt.Errorf("%w: %s", ErrSegmentedPlanStoreMissing, hash)
		}
		if err != nil {
			return PlanStoreStats{}, err
		}
		if uint64(len(payload)) != length || SegmentHash(sha256.Sum256(payload)) != hash {
			return PlanStoreStats{}, corruptionError(ErrPlanStoreHashMismatch, "segment digest or length mismatch")
		}
		if uint64(len(payload)) > store.limits.MaxTotalBytes-stats.Bytes {
			return PlanStoreStats{}, ErrPlanStoreLimit
		}
		stats.Bytes += uint64(len(payload))
	}
	if stats.Bytes > store.limits.MaxTotalBytes {
		return PlanStoreStats{}, ErrPlanStoreLimit
	}
	return stats, nil
}

func encodeSegmentedPlan(ctx context.Context, plan LayoutPlan, limits SegmentedPlanStoreLimits) (segmentedEncodedPlan, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := plan.Validate(); err != nil {
		return segmentedEncodedPlan{}, err
	}
	canonical, err := plan.CanonicalJSON()
	if err != nil {
		return segmentedEncodedPlan{}, err
	}
	if uint64(len(canonical)) > limits.MaxPlanBytes {
		return segmentedEncodedPlan{}, ErrPlanStoreLimit
	}
	hash := PlanHash(sha256.Sum256(canonical))
	projection := plan.Projection()
	budget := segmentedBudget{ctx: ctx, limit: limits.MaxWork}
	pageCount := len(projection.Pages)
	segmentCount := 0
	if pageCount > 0 {
		segmentCount = (pageCount + int(limits.MaxPagesPerSegment) - 1) / int(limits.MaxPagesPerSegment)
	} else {
		segmentCount = 1
	}
	if uint64(segmentCount) > uint64(limits.MaxSegmentsPerPlan) {
		return segmentedEncodedPlan{}, ErrPlanStoreLimit
	}
	segments := make([]segmentedPlanPayload, segmentCount)
	for index := range segments {
		start := index * int(limits.MaxPagesPerSegment)
		end := start + int(limits.MaxPagesPerSegment)
		if end > pageCount {
			end = pageCount
		}
		pageStart := uint32(start + 1)
		if pageCount == 0 {
			pageStart = 0
		}
		segments[index] = segmentedPlanPayload{FormatVersion: SegmentedPlanSegmentFormatVersion, PlanHash: hash, Index: uint32(index), PageStart: pageStart, Pages: cloneSlice(projection.Pages[start:end])}
	}
	segmentForPage := func(page uint32) int {
		if page == 0 || segmentCount == 0 {
			return 0
		}
		return int((page - 1) / limits.MaxPagesPerSegment)
	}
	fragmentPages := make(map[FragmentID]uint32, len(projection.Fragments))
	work := uint64(len(projection.Pages)) + uint64(len(projection.Fragments)) + uint64(len(projection.Lines)) + uint64(len(projection.PageRegions)) + uint64(len(projection.GridTracks)) +
		uint64(len(projection.GlyphRuns)) + uint64(len(projection.Images)) + uint64(len(projection.Links)) +
		displayGraphicsProjectionWork(projection) +
		uint64(len(projection.Commands)) + uint64(len(projection.Breaks)) + uint64(len(projection.Diagnostics)) +
		uint64(len(projection.SemanticNodes)) + uint64(len(projection.SemanticFragments)) + uint64(len(projection.ReadingOrder)) + uint64(segmentCount)
	if err := budget.charge(work); err != nil {
		return segmentedEncodedPlan{}, err
	}
	for index, value := range projection.Fragments {
		fragmentPages[value.ID] = value.Page
		s := segmentForPage(value.Page)
		segments[s].Fragments = append(segments[s].Fragments, indexedFragment{uint32(index), value})
	}
	for index, value := range projection.Lines {
		page := fragmentPages[value.Fragment]
		s := segmentForPage(page)
		segments[s].Lines = append(segments[s].Lines, indexedLine{uint32(index), value})
	}
	for index, value := range projection.GridTracks {
		s := segmentForPage(value.Page)
		segments[s].GridTracks = append(segments[s].GridTracks, indexedGridTrack{uint32(index), value})
	}
	for index, value := range projection.PageRegions {
		s := segmentForPage(value.Page)
		segments[s].PageRegions = append(segments[s].PageRegions, indexedPageRegion{uint32(index), value})
	}
	for index, value := range projection.GlyphRuns {
		page := fragmentPages[projection.Lines[value.Line].Fragment]
		s := segmentForPage(page)
		segments[s].GlyphRuns = append(segments[s].GlyphRuns, indexedGlyphRun{uint32(index), cloneCoreGlyphRun(value)})
	}
	for index, value := range projection.Images {
		page := fragmentPages[value.Fragment]
		s := segmentForPage(page)
		segments[s].Images = append(segments[s].Images, indexedImage{uint32(index), clonePlannedImage(value)})
	}
	for index, value := range projection.Links {
		page := fragmentPages[value.Fragment]
		s := segmentForPage(page)
		segments[s].Links = append(segments[s].Links, indexedLink{uint32(index), value})
	}
	for pageIndex, page := range projection.Pages {
		end, _ := page.Commands.end(len(projection.Commands))
		for index := int(page.Commands.Start); index < end; index++ {
			segments[segmentForPage(uint32(pageIndex+1))].Commands = append(segments[segmentForPage(uint32(pageIndex+1))].Commands, indexedCommand{uint32(index), projection.Commands[index]})
		}
	}
	for index, value := range projection.Breaks {
		s := segmentForPage(value.ToPage)
		segments[s].Breaks = append(segments[s].Breaks, indexedBreak{uint32(index), value})
	}
	for index, value := range projection.Diagnostics {
		page := value.Location.Page
		if page == 0 && value.Location.Fragment.Valid() {
			page = fragmentPages[value.Location.Fragment]
		}
		s := segmentForPage(page)
		segments[s].Diagnostics = append(segments[s].Diagnostics, indexedDiagnostic{uint32(index), cloneDiagnostic(value)})
	}
	for index, value := range projection.ReadingOrder {
		s := segmentForPage(value.Page)
		segments[s].ReadingOrder = append(segments[s].ReadingOrder, indexedReadingOccurrence{uint32(index), value})
	}
	for index, value := range projection.SemanticFragments {
		s := segmentForPage(value.Page)
		segments[s].SemanticFragments = append(segments[s].SemanticFragments, indexedSemanticFragment{uint32(index), value})
	}
	manifest := segmentedPlanManifest{FormatVersion: SegmentedPlanManifestFormatVersion, PlanSchemaVersion: LayoutPlanSchemaVersion,
		PlannerVersion: PlannerVersion, PainterContractVersion: PainterContractVersion, PlanHash: hash,
		DeterministicInputs: projection.DeterministicInputs,
		Counts: segmentedPlanCounts{Pages: uint32(len(projection.Pages)), Fragments: uint32(len(projection.Fragments)), Lines: uint32(len(projection.Lines)),
			PageRegions: uint32(len(projection.PageRegions)), GridTracks: uint32(len(projection.GridTracks)),
			GlyphRuns: uint32(len(projection.GlyphRuns)), Images: uint32(len(projection.Images)), Links: uint32(len(projection.Links)),
			Commands: uint32(len(projection.Commands)), Breaks: uint32(len(projection.Breaks)), Diagnostics: uint32(len(projection.Diagnostics)),
			SemanticNodes: uint32(len(projection.SemanticNodes)), SemanticFragments: uint32(len(projection.SemanticFragments)), ReadingOrder: uint32(len(projection.ReadingOrder))},
		Pages: cloneSlice(projection.Pages), Fonts: cloneSlice(projection.Fonts), ImageResources: cloneSlice(projection.ImageResources),
		Destinations: cloneSlice(projection.Destinations), Paths: clonePlannedPaths(projection.Paths), Transforms: cloneSlice(projection.Transforms),
		Clips: cloneSlice(projection.Clips), Fills: cloneSlice(projection.Fills), Strokes: clonePlannedStrokes(projection.Strokes), SemanticNodes: cloneSlice(projection.SemanticNodes)}
	payloads := make(map[SegmentHash][]byte, len(segments))
	total := uint64(0)
	for index, segment := range segments {
		encoded, err := json.Marshal(segment)
		if err != nil {
			return segmentedEncodedPlan{}, err
		}
		if uint64(len(encoded)) > limits.MaxSegmentBytes {
			return segmentedEncodedPlan{}, ErrPlanStoreLimit
		}
		sh := SegmentHash(sha256.Sum256(encoded))
		payloads[sh] = encoded
		manifest.Segments = append(manifest.Segments, segmentedPlanReference{uint32(index), segment.PageStart, uint32(len(segment.Pages)), uint64(len(encoded)), sh})
		total += uint64(len(encoded))
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return segmentedEncodedPlan{}, err
	}
	total += uint64(len(manifestJSON))
	if total > limits.MaxPlanBytes {
		return segmentedEncodedPlan{}, ErrPlanStoreLimit
	}
	return segmentedEncodedPlan{hash: hash, manifest: manifestJSON, manifestValue: manifest, segments: payloads, totalBytes: total}, nil
}

func reconstructSegmentedPlan(ctx context.Context, hash PlanHash, encoded []byte, load func(segmentedPlanReference) ([]byte, error), limits SegmentedPlanStoreLimits) (LayoutPlan, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	manifest, err := decodeSegmentedManifest(encoded, hash)
	if err != nil {
		return LayoutPlan{}, err
	}
	if err := validateSegmentedManifestLimits(manifest, limits); err != nil {
		return LayoutPlan{}, err
	}
	budget := segmentedBudget{ctx: ctx, limit: limits.MaxWork}
	work := uint64(manifest.Counts.Pages) + uint64(manifest.Counts.Fragments) + uint64(manifest.Counts.Lines) + uint64(manifest.Counts.PageRegions) + uint64(manifest.Counts.GridTracks) +
		uint64(manifest.Counts.GlyphRuns) + uint64(manifest.Counts.Images) + uint64(manifest.Counts.Links) +
		displayGraphicsManifestWork(manifest) +
		uint64(manifest.Counts.Commands) + uint64(manifest.Counts.Breaks) + uint64(manifest.Counts.Diagnostics) +
		uint64(manifest.Counts.SemanticNodes) + uint64(manifest.Counts.SemanticFragments) + uint64(manifest.Counts.ReadingOrder) + uint64(len(manifest.Segments))
	if err := budget.charge(work); err != nil {
		return LayoutPlan{}, err
	}
	input := LayoutPlanInput{DeterministicInputs: manifest.DeterministicInputs, Pages: make([]PlannedPage, manifest.Counts.Pages), Fragments: make([]Fragment, manifest.Counts.Fragments), Lines: make([]PlannedLine, manifest.Counts.Lines), PageRegions: make([]PlannedPageRegion, manifest.Counts.PageRegions), GridTracks: make([]PlannedGridTrack, manifest.Counts.GridTracks), Fonts: cloneSlice(manifest.Fonts), GlyphRuns: make([]CoreGlyphRun, manifest.Counts.GlyphRuns), ImageResources: cloneSlice(manifest.ImageResources), Images: make([]PlannedImage, manifest.Counts.Images), Destinations: cloneSlice(manifest.Destinations), Links: make([]PlannedLink, manifest.Counts.Links), Paths: clonePlannedPaths(manifest.Paths), Transforms: cloneSlice(manifest.Transforms), Clips: cloneSlice(manifest.Clips), Fills: cloneSlice(manifest.Fills), Strokes: clonePlannedStrokes(manifest.Strokes), Commands: make([]DisplayCommand, manifest.Counts.Commands), Breaks: make([]BreakDecision, manifest.Counts.Breaks), Diagnostics: make([]Diagnostic, manifest.Counts.Diagnostics), SemanticNodes: cloneSlice(manifest.SemanticNodes), SemanticFragments: make([]SemanticFragmentAssociation, manifest.Counts.SemanticFragments), ReadingOrder: make([]ReadingOccurrence, manifest.Counts.ReadingOrder)}
	pageSeen := make([]bool, len(input.Pages))
	fragSeen := make([]bool, len(input.Fragments))
	lineSeen := make([]bool, len(input.Lines))
	pageRegionSeen := make([]bool, len(input.PageRegions))
	gridTrackSeen := make([]bool, len(input.GridTracks))
	glyphSeen := make([]bool, len(input.GlyphRuns))
	imageSeen := make([]bool, len(input.Images))
	linkSeen := make([]bool, len(input.Links))
	commandSeen := make([]bool, len(input.Commands))
	breakSeen := make([]bool, len(input.Breaks))
	diagnosticSeen := make([]bool, len(input.Diagnostics))
	readingSeen := make([]bool, len(input.ReadingOrder))
	semanticFragmentSeen := make([]bool, len(input.SemanticFragments))
	for _, reference := range manifest.Segments {
		payload, err := load(reference)
		if err != nil {
			return LayoutPlan{}, err
		}
		segment, err := decodeSegmentedPayload(payload, reference, hash)
		if err != nil {
			return LayoutPlan{}, err
		}
		for offset, page := range segment.Pages {
			index := int(segment.PageStart) - 1 + offset
			if err := putSegmented(index, page, input.Pages, pageSeen); err != nil {
				return LayoutPlan{}, err
			}
		}
		for _, v := range segment.Fragments {
			if err := putSegmented(int(v.Index), v.Value, input.Fragments, fragSeen); err != nil {
				return LayoutPlan{}, err
			}
		}
		for _, v := range segment.Lines {
			if err := putSegmented(int(v.Index), v.Value, input.Lines, lineSeen); err != nil {
				return LayoutPlan{}, err
			}
		}
		for _, v := range segment.GridTracks {
			if err := putSegmented(int(v.Index), v.Value, input.GridTracks, gridTrackSeen); err != nil {
				return LayoutPlan{}, err
			}
		}
		for _, v := range segment.PageRegions {
			if err := putSegmented(int(v.Index), v.Value, input.PageRegions, pageRegionSeen); err != nil {
				return LayoutPlan{}, err
			}
		}
		for _, v := range segment.GlyphRuns {
			if err := putSegmented(int(v.Index), cloneCoreGlyphRun(v.Value), input.GlyphRuns, glyphSeen); err != nil {
				return LayoutPlan{}, err
			}
		}
		for _, v := range segment.Images {
			if err := putSegmented(int(v.Index), clonePlannedImage(v.Value), input.Images, imageSeen); err != nil {
				return LayoutPlan{}, err
			}
		}
		for _, v := range segment.Links {
			if err := putSegmented(int(v.Index), v.Value, input.Links, linkSeen); err != nil {
				return LayoutPlan{}, err
			}
		}
		for _, v := range segment.Commands {
			if err := putSegmented(int(v.Index), v.Value, input.Commands, commandSeen); err != nil {
				return LayoutPlan{}, err
			}
		}
		for _, v := range segment.Breaks {
			if err := putSegmented(int(v.Index), v.Value, input.Breaks, breakSeen); err != nil {
				return LayoutPlan{}, err
			}
		}
		for _, v := range segment.Diagnostics {
			if err := putSegmented(int(v.Index), cloneDiagnostic(v.Value), input.Diagnostics, diagnosticSeen); err != nil {
				return LayoutPlan{}, err
			}
		}
		for _, v := range segment.ReadingOrder {
			if err := putSegmented(int(v.Index), v.Value, input.ReadingOrder, readingSeen); err != nil {
				return LayoutPlan{}, err
			}
		}
		for _, v := range segment.SemanticFragments {
			if err := putSegmented(int(v.Index), v.Value, input.SemanticFragments, semanticFragmentSeen); err != nil {
				return LayoutPlan{}, err
			}
		}
	}
	for _, seen := range [][]bool{pageSeen, fragSeen, lineSeen, pageRegionSeen, gridTrackSeen, glyphSeen, imageSeen, linkSeen, commandSeen, breakSeen, diagnosticSeen, semanticFragmentSeen, readingSeen} {
		for _, value := range seen {
			if !value {
				return LayoutPlan{}, corruptionError(nil, "segmented projection has a missing indexed value")
			}
		}
	}
	plan, err := NewLayoutPlan(input)
	if err != nil {
		return LayoutPlan{}, corruptionError(nil, err.Error())
	}
	actual, _ := plan.Hash()
	if actual != hash {
		return LayoutPlan{}, corruptionError(ErrPlanStoreHashMismatch, "reconstructed plan hash mismatch")
	}
	return plan, nil
}

func displayGraphicsProjectionWork(projection LayoutPlanProjection) uint64 {
	work := uint64(len(projection.Paths) + len(projection.Transforms) + len(projection.Clips) + len(projection.Fills) + len(projection.Strokes))
	for _, path := range projection.Paths {
		work += uint64(len(path.Segments))
	}
	return work
}

func displayGraphicsManifestWork(manifest segmentedPlanManifest) uint64 {
	work := uint64(len(manifest.Paths) + len(manifest.Transforms) + len(manifest.Clips) + len(manifest.Fills) + len(manifest.Strokes))
	for _, path := range manifest.Paths {
		work += uint64(len(path.Segments))
	}
	return work
}

func putSegmented[T any](index int, value T, values []T, seen []bool) error {
	if index < 0 || index >= len(values) || seen[index] {
		return corruptionError(nil, "segmented projection index is invalid or duplicated")
	}
	values[index] = value
	seen[index] = true
	return nil
}

func decodeSegmentedManifest(encoded []byte, expected PlanHash) (segmentedPlanManifest, error) {
	var value segmentedPlanManifest
	if err := decodeCanonicalSegmented(encoded, &value); err != nil {
		return value, err
	}
	if value.FormatVersion != SegmentedPlanManifestFormatVersion || value.PlanSchemaVersion != LayoutPlanSchemaVersion ||
		value.PlannerVersion != PlannerVersion || value.PainterContractVersion != PainterContractVersion {
		return value, corruptionError(ErrPlanStoreSchema, "segmented manifest version mismatch")
	}
	if value.PlanHash != expected {
		return value, corruptionError(ErrPlanStoreHashMismatch, "manifest plan hash mismatch")
	}
	if len(value.Pages) != int(value.Counts.Pages) {
		return value, corruptionError(nil, "manifest page count mismatch")
	}
	if len(value.SemanticNodes) != int(value.Counts.SemanticNodes) {
		return value, corruptionError(nil, "manifest semantic-node count mismatch")
	}
	expectedPage := uint32(1)
	for index, reference := range value.Segments {
		zeroPagePlan := value.Counts.Pages == 0 && len(value.Segments) == 1
		if reference.Index != uint32(index) || reference.ByteLength == 0 ||
			(!zeroPagePlan && (reference.PageCount == 0 || reference.PageStart != expectedPage)) ||
			(zeroPagePlan && (reference.PageStart != 0 || reference.PageCount != 0)) {
			return value, corruptionError(nil, "manifest segment reference is invalid")
		}
		expectedPage += reference.PageCount
	}
	if value.Counts.Pages > 0 && expectedPage != value.Counts.Pages+1 {
		return value, corruptionError(nil, "manifest segment page ranges do not cover the plan")
	}
	return value, nil
}

func validateSegmentedManifestLimits(manifest segmentedPlanManifest, limits SegmentedPlanStoreLimits) error {
	if uint64(len(manifest.Segments)) > uint64(limits.MaxSegmentsPerPlan) {
		return fmt.Errorf("%w: manifest segment count", ErrPlanStoreLimit)
	}
	var bytes uint64
	for _, reference := range manifest.Segments {
		if reference.PageCount > limits.MaxPagesPerSegment || reference.ByteLength > limits.MaxSegmentBytes ||
			bytes > limits.MaxPlanBytes || reference.ByteLength > limits.MaxPlanBytes-bytes {
			return fmt.Errorf("%w: manifest segment bounds", ErrPlanStoreLimit)
		}
		bytes += reference.ByteLength
	}
	work := uint64(manifest.Counts.Pages) + uint64(manifest.Counts.Fragments) + uint64(manifest.Counts.Lines) + uint64(manifest.Counts.PageRegions) + uint64(manifest.Counts.GridTracks) +
		uint64(manifest.Counts.GlyphRuns) + uint64(manifest.Counts.Images) + uint64(manifest.Counts.Links) +
		uint64(manifest.Counts.Commands) + uint64(manifest.Counts.Breaks) + uint64(manifest.Counts.Diagnostics) +
		uint64(manifest.Counts.SemanticNodes) + uint64(manifest.Counts.SemanticFragments) + uint64(manifest.Counts.ReadingOrder) + uint64(len(manifest.Segments))
	if work > limits.MaxWork {
		return ErrSegmentedPlanStoreWork
	}
	return nil
}
func decodeSegmentedPayload(encoded []byte, reference segmentedPlanReference, planHash PlanHash) (segmentedPlanPayload, error) {
	if uint64(len(encoded)) != reference.ByteLength || SegmentHash(sha256.Sum256(encoded)) != reference.Hash {
		return segmentedPlanPayload{}, corruptionError(ErrPlanStoreHashMismatch, "segment bytes mismatch")
	}
	var value segmentedPlanPayload
	if err := decodeCanonicalSegmented(encoded, &value); err != nil {
		return value, err
	}
	if value.FormatVersion != SegmentedPlanSegmentFormatVersion || value.PlanHash != planHash || value.Index != reference.Index || value.PageStart != reference.PageStart || uint32(len(value.Pages)) != reference.PageCount {
		return value, corruptionError(ErrPlanStoreSchema, "segment envelope mismatch")
	}
	return value, nil
}
func decodeCanonicalSegmented[T any](encoded []byte, value *T) error {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return corruptionError(nil, err.Error())
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return corruptionError(nil, err.Error())
	}
	canonical, err := json.Marshal(value)
	if err != nil || !bytes.Equal(canonical, encoded) {
		return corruptionError(nil, "segmented JSON is not canonical")
	}
	return nil
}

func segmentedPageMetadata(ctx context.Context, encoded []byte, hash PlanHash, page uint32, limits SegmentedPlanStoreLimits) (SegmentedPlanPageMetadata, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return SegmentedPlanPageMetadata{}, err
	}
	manifest, err := decodeSegmentedManifest(encoded, hash)
	if err != nil {
		return SegmentedPlanPageMetadata{}, err
	}
	if err := validateSegmentedManifestLimits(manifest, limits); err != nil {
		return SegmentedPlanPageMetadata{}, err
	}
	if page == 0 || uint64(page) > uint64(len(manifest.Pages)) {
		return SegmentedPlanPageMetadata{}, ErrPlanStoreNotFound
	}
	for _, reference := range manifest.Segments {
		if page >= reference.PageStart && page < reference.PageStart+reference.PageCount {
			return SegmentedPlanPageMetadata{PlanHash: hash, Page: manifest.Pages[page-1], SegmentIndex: reference.Index, SegmentHash: reference.Hash}, nil
		}
	}
	return SegmentedPlanPageMetadata{}, corruptionError(nil, "page has no segment reference")
}

func readSegmentedFile(path string, limit uint64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader := io.LimitReader(file, int64(limit)+1)
	encoded, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	if uint64(len(encoded)) > limit {
		return nil, ErrPlanStoreLimit
	}
	return encoded, nil
}
func segmentedPlanHashFromFilename(name string) (PlanHash, bool) {
	if !strings.HasSuffix(name, segmentedManifestSuffix) {
		return PlanHash{}, false
	}
	raw := strings.TrimSuffix(name, segmentedManifestSuffix)
	decoded, err := hex.DecodeString(raw)
	if err != nil || len(decoded) != sha256.Size || raw != strings.ToLower(raw) {
		return PlanHash{}, false
	}
	var hash PlanHash
	copy(hash[:], decoded)
	return hash, true
}

var _ SegmentedPlanStore = (*MemorySegmentedPlanStore)(nil)
var _ SegmentedPlanStore = (*FileSegmentedPlanStore)(nil)
