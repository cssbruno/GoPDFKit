// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperpkg

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// LockfileMigration reports the exact deterministic schema transition applied
// by MigrateLockfile. A zero transition means the input was already current.
type LockfileMigration struct {
	FromSchema uint16 `json:"from_schema"`
	ToSchema   uint16 `json:"to_schema"`
	Changed    bool   `json:"changed"`
}

type lockfileV1 struct {
	SchemaVersion uint16    `json:"schema_version"`
	Entries       []entryV1 `json:"entries,omitempty"`
}

type entryV1 struct {
	ImportPath      string          `json:"import_path"`
	ContentDigest   Digest          `json:"content_digest"`
	Assets          []Asset         `json:"assets,omitempty"`
	SignaturePolicy SignaturePolicy `json:"signature_policy"`
	OfflinePolicy   OfflinePolicy   `json:"offline_policy"`
}

// MigrateLockfile validates canonical input and returns a detached canonical
// lockfile using the current schema. Schema 1 did not record a publisher
// version, so migration assigns the content-addressed version
// "sha256-<content digest>". This preserves reproducibility without guessing a
// semantic version that was absent from the old lockfile.
func MigrateLockfile(encoded []byte) ([]byte, LockfileMigration, error) {
	return MigrateLockfileWithLimits(encoded, DefaultLimits())
}

func MigrateLockfileWithLimits(encoded []byte, limits Limits) ([]byte, LockfileMigration, error) {
	if err := limits.validate(); err != nil {
		return nil, LockfileMigration{}, err
	}
	if uint64(len(encoded)) > limits.MaxLockfileBytes {
		return nil, LockfileMigration{}, fmt.Errorf("%w: encoded lockfile exceeds its byte budget", ErrLockfileLimit)
	}
	var header struct {
		SchemaVersion uint16 `json:"schema_version"`
	}
	if err := json.Unmarshal(encoded, &header); err != nil {
		return nil, LockfileMigration{}, fmt.Errorf("%w: %w", ErrInvalidLockfile, err)
	}
	if header.SchemaVersion == LockfileSchemaVersion {
		lockfile, err := DecodeWithLimits(encoded, limits)
		if err != nil {
			return nil, LockfileMigration{}, err
		}
		current, err := EncodeWithLimits(lockfile, limits)
		return current, LockfileMigration{FromSchema: LockfileSchemaVersion, ToSchema: LockfileSchemaVersion}, err
	}
	if header.SchemaVersion != 1 {
		return nil, LockfileMigration{}, fmt.Errorf("%w: got %d, can migrate 1 or %d", ErrLockfileSchema, header.SchemaVersion, LockfileSchemaVersion)
	}

	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var legacy lockfileV1
	if err := decoder.Decode(&legacy); err != nil {
		return nil, LockfileMigration{}, fmt.Errorf("%w: %w", ErrInvalidLockfile, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("additional JSON value")
		}
		return nil, LockfileMigration{}, fmt.Errorf("%w: trailing data: %w", ErrInvalidLockfile, err)
	}
	canonicalLegacy, err := json.Marshal(legacy)
	if err != nil {
		return nil, LockfileMigration{}, fmt.Errorf("paperpkg: encode legacy lockfile: %w", err)
	}
	if !bytes.Equal(encoded, canonicalLegacy) {
		return nil, LockfileMigration{}, ErrNonCanonicalJSON
	}

	current := Lockfile{SchemaVersion: LockfileSchemaVersion, Entries: make([]Entry, len(legacy.Entries))}
	for index, entry := range legacy.Entries {
		current.Entries[index] = Entry{ImportPath: entry.ImportPath, Version: "sha256-" + string(entry.ContentDigest),
			ContentDigest: entry.ContentDigest, Assets: append([]Asset(nil), entry.Assets...),
			SignaturePolicy: entry.SignaturePolicy, OfflinePolicy: entry.OfflinePolicy}
	}
	migrated, err := EncodeWithLimits(current, limits)
	if err != nil {
		return nil, LockfileMigration{}, err
	}
	return migrated, LockfileMigration{FromSchema: 1, ToSchema: LockfileSchemaVersion, Changed: true}, nil
}
