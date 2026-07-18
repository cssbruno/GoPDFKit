// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/cssbruno/gopdfkit/internal/paperscenario"
)

var ErrScenarioRevisionNotFound = workspaceError("SCENARIO_REVISION_NOT_FOUND", "scenario revision handle is not retained", ErrRevisionNotFound)

type ScenarioFixtureSnapshot struct {
	Name   string `json:"name"`
	Locale string `json:"locale,omitempty"`
	Digest string `json:"digest"`
}

type ScenarioRevisionSnapshot struct {
	Handle           ScenarioRevisionHandle    `json:"-"`
	Digest           string                    `json:"digest"`
	Fixtures         []ScenarioFixtureSnapshot `json:"fixtures"`
	Capability       CapabilityMode            `json:"capability"`
	DisclosureDomain DisclosureDomain          `json:"disclosure_domain"`
	ExpiresAt        time.Time                 `json:"expires_at"`
}

// CreateScenarioRevision resolves and retains a separate immutable fixture
// revision. Full fixture values remain private to the workspace; snapshots
// expose only names, locale pins, and non-secret deterministic digests.
func (w *Workspace) CreateScenarioRevision(input []paperscenario.Scenario, limits paperscenario.Limits) (ScenarioRevisionSnapshot, error) {
	if w == nil {
		return ScenarioRevisionSnapshot{}, workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	fixtures, err := paperscenario.Resolve(input, limits)
	if err != nil {
		return ScenarioRevisionSnapshot{}, workspaceError("INVALID_SCENARIO", "scenario fixtures are invalid", err)
	}
	record := &scenarioRevisionRecord{fixtures: fixtures, digest: scenarioDigest(fixtures)}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pruneExpiredHandlesLocked(w.now())
	if len(w.scenarioRevisions) >= w.limits.MaxScenarioRevisions {
		return ScenarioRevisionSnapshot{}, workspaceError("SCENARIO_REVISION_LIMIT", "workspace scenario revision capacity is exhausted", ErrLimit)
	}
	w.nextScenarioRevision++
	record.handle = ScenarioRevisionHandle{value: w.newHandle(handleScenarioRevision, capabilityRead, w.nextScenarioRevision)}
	record.expires = w.expiresAt(w.handleTTL)
	record.disclosure = w.disclosureDomain
	record.partition = w.partition
	w.scenarioRevisions[w.nextScenarioRevision] = record
	return scenarioSnapshot(record), nil
}

func (w *Workspace) OpenScenarioRevision(handle ScenarioRevisionHandle) (ScenarioRevisionSnapshot, error) {
	if w == nil {
		return ScenarioRevisionSnapshot{}, workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	record, err := w.scenarioRevisionLocked(handle)
	if err != nil {
		return ScenarioRevisionSnapshot{}, err
	}
	return scenarioSnapshot(record), nil
}

func (w *Workspace) scenarioRevisionLocked(handle ScenarioRevisionHandle) (*scenarioRevisionRecord, error) {
	if err := w.validateHandle(handle.value, handleScenarioRevision, capabilityRead, false); err != nil {
		return nil, err
	}
	record := w.scenarioRevisions[handle.value.serial]
	if record == nil || record.handle != handle || !w.ownsPartition(record.partition) {
		return nil, w.unavailableHandle(handle.value, ErrScenarioRevisionNotFound)
	}
	if err := w.ensureLive(handle.value, record.expires); err != nil {
		return nil, err
	}
	return record, nil
}

func scenarioSnapshot(record *scenarioRevisionRecord) ScenarioRevisionSnapshot {
	snapshot := ScenarioRevisionSnapshot{Handle: record.handle, Digest: record.digest, Fixtures: make([]ScenarioFixtureSnapshot, len(record.fixtures)),
		Capability: CapabilityRead, DisclosureDomain: record.disclosure, ExpiresAt: record.expires}
	for i, fixture := range record.fixtures {
		snapshot.Fixtures[i] = ScenarioFixtureSnapshot{Name: fixture.Name, Locale: fixture.Locale, Digest: fixture.Digest}
	}
	return snapshot
}

func cloneScenarioSnapshot(snapshot ScenarioRevisionSnapshot) ScenarioRevisionSnapshot {
	snapshot.Fixtures = append([]ScenarioFixtureSnapshot(nil), snapshot.Fixtures...)
	return snapshot
}

func scenarioDigest(fixtures []paperscenario.Fixture) string {
	encoded, err := json.Marshal(fixtures)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}
