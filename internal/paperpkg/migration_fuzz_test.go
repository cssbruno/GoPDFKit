// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperpkg

import (
	"encoding/json"
	"strings"
	"testing"
)

func FuzzMigrateLockfile(f *testing.F) {
	legacy := struct {
		SchemaVersion uint16    `json:"schema_version"`
		Entries       []entryV1 `json:"entries,omitempty"`
	}{SchemaVersion: 1}
	legacy.Entries = append(legacy.Entries, entryV1{ImportPath: "example", ContentDigest: Digest(strings.Repeat("a", 64)),
		SignaturePolicy: SignatureRequired, OfflinePolicy: OfflineOnly})
	seed, err := json.Marshal(legacy)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(seed)
	f.Add([]byte(`{"schema_version":2,"packages":[]}`))
	f.Add([]byte(`{"schema_version":1,"packages":null}`))
	f.Add([]byte("not json"))

	f.Fuzz(func(t *testing.T, input []byte) {
		migrated, _, err := MigrateLockfile(input)
		if err != nil {
			return
		}
		lock, err := Decode(migrated)
		if err != nil {
			t.Fatalf("migration emitted invalid lockfile: %v", err)
		}
		canonical, err := Encode(lock)
		if err != nil {
			t.Fatalf("migration output could not be re-encoded: %v", err)
		}
		if string(canonical) != string(migrated) {
			t.Fatalf("migration output was not canonical\nwant %s\ngot  %s", canonical, migrated)
		}
	})
}
