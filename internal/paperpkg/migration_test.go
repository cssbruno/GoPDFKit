// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperpkg

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

const canonicalV1Lockfile = `{"schema_version":1,"entries":[{"import_path":"acme/charts","content_digest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","assets":[{"path":"fonts/body.ttf","digest":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}],"signature_policy":"required","offline_policy":"offline_only"}]}`

func TestMigrateLockfileV1AddsContentAddressedVersionDeterministically(t *testing.T) {
	first, report, err := MigrateLockfile([]byte(canonicalV1Lockfile))
	if err != nil {
		t.Fatal(err)
	}
	if report != (LockfileMigration{FromSchema: 1, ToSchema: LockfileSchemaVersion, Changed: true}) {
		t.Fatalf("migration report = %+v", report)
	}
	second, secondReport, err := MigrateLockfile([]byte(canonicalV1Lockfile))
	if err != nil || !bytes.Equal(first, second) || secondReport != report {
		t.Fatalf("repeated migration = %s, %+v, %v", second, secondReport, err)
	}
	lockfile, err := Decode(first)
	if err != nil {
		t.Fatal(err)
	}
	entry, found := lockfile.Lookup("acme/charts")
	wantVersion := "sha256-" + strings.Repeat("a", 64)
	if !found || entry.Version != wantVersion || entry.ContentDigest != digest('a') || lockfile.SchemaVersion != LockfileSchemaVersion {
		t.Fatalf("migrated lockfile = %+v", lockfile)
	}

	current, currentReport, err := MigrateLockfile(first)
	if err != nil || !bytes.Equal(current, first) || currentReport.Changed || currentReport.FromSchema != LockfileSchemaVersion || currentReport.ToSchema != LockfileSchemaVersion {
		t.Fatalf("current migration = %s, %+v, %v", current, currentReport, err)
	}
}

func TestMigrateLockfileRejectsNoncanonicalUnknownUnsupportedAndBoundedInput(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want error
	}{
		{"whitespace", append([]byte{' '}, canonicalV1Lockfile...), ErrNonCanonicalJSON},
		{"unknown", []byte(`{"schema_version":1,"entries":[],"unknown":true}`), ErrInvalidLockfile},
		{"trailing", append([]byte(canonicalV1Lockfile), []byte(`{}`)...), ErrInvalidLockfile},
		{"unsupported", []byte(`{"schema_version":0}`), ErrLockfileSchema},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, _, err := MigrateLockfile(test.data); !errors.Is(err, test.want) {
				t.Fatalf("MigrateLockfile() = %v, want %v", err, test.want)
			}
		})
	}
	limits := DefaultLimits()
	limits.MaxLockfileBytes = uint64(len(canonicalV1Lockfile) - 1)
	if _, _, err := MigrateLockfileWithLimits([]byte(canonicalV1Lockfile), limits); !errors.Is(err, ErrLockfileLimit) {
		t.Fatalf("bounded migration = %v", err)
	}
}
