// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperpkg

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestLockfileCanonicalRoundTripLookupAndProjectDigest(t *testing.T) {
	lockfile := validLockfile()
	encoded, err := Encode(lockfile)
	if err != nil {
		t.Fatal(err)
	}
	wantJSON := `{"schema_version":2,"entries":[{"import_path":"acme/charts","version":"v1.4.0","content_digest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","assets":[{"path":"fonts/body.ttf","digest":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},{"path":"images/chart.png","digest":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"}],"signature_policy":"required","offline_policy":"offline_only"},{"import_path":"acme/forms","version":"2026.07+approved","content_digest":"dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd","signature_policy":"allow_unsigned","offline_policy":"network_allowed"}]}`
	if string(encoded) != wantJSON {
		t.Fatalf("Encode() = %s\nwant %s", encoded, wantJSON)
	}
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, lockfile) {
		t.Fatalf("Decode() = %#v, want %#v", decoded, lockfile)
	}
	entry, found := decoded.Lookup("acme/charts")
	if !found || entry.ContentDigest != digest('a') || len(entry.Assets) != 2 {
		t.Fatalf("Lookup() = %#v, %v", entry, found)
	}
	entry.Assets[0].Path = "mutated"
	again, _ := decoded.Lookup("acme/charts")
	if again.Assets[0].Path != "fonts/body.ttf" {
		t.Fatal("Lookup exposed lockfile asset storage")
	}
	if _, found := decoded.Lookup("acme/missing"); found {
		t.Fatal("missing lookup unexpectedly succeeded")
	}
	project, err := decoded.ProjectDigest()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(project), "1d997996d5298572a7e283964052f4dd32a481276cd176ae4a7ed751bb4e3c47"; got != want {
		t.Fatalf("ProjectDigest() = %s, want %s", got, want)
	}
}

func TestDecodeRejectsUnknownTrailingAndNoncanonicalJSON(t *testing.T) {
	canonical, err := Encode(validLockfile())
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		data []byte
		want error
	}{
		{"unknown top-level", []byte(`{"schema_version":1,"unknown":true}`), ErrInvalidLockfile},
		{"unknown entry", []byte(`{"schema_version":1,"entries":[{"import_path":"a","content_digest":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","signature_policy":"required","offline_policy":"offline_only","unknown":true}]}`), ErrInvalidLockfile},
		{"trailing value", append(append([]byte(nil), canonical...), []byte(`{}`)...), ErrInvalidLockfile},
		{"trailing garbage", append(append([]byte(nil), canonical...), '!'), ErrInvalidLockfile},
		{"leading whitespace", append([]byte{' '}, canonical...), ErrNonCanonicalJSON},
		{"trailing whitespace", append(append([]byte(nil), canonical...), '\n'), ErrNonCanonicalJSON},
		{"field order", []byte(`{"entries":[],"schema_version":2}`), ErrNonCanonicalJSON},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Decode(test.data); !errors.Is(err, test.want) {
				t.Fatalf("Decode() = %v, want %v", err, test.want)
			}
		})
	}
}

func TestLockfileRejectsInvalidPathsOrderingDigestsAndPolicies(t *testing.T) {
	invalidPaths := []string{"", "/absolute", "../escape", "pkg/../escape", `pkg\asset`, "./pkg", "pkg//asset", "https://example.test/pkg", "file:pkg", "pkg?query", "pkg#fragment", "pkg%2fasset", "pkg name", "cafe\u0301/pkg"}
	for _, value := range invalidPaths {
		t.Run("import_"+strings.ReplaceAll(value, "/", "_"), func(t *testing.T) {
			lockfile := validLockfile()
			lockfile.Entries[0].ImportPath = value
			if _, err := Encode(lockfile); !errors.Is(err, ErrInvalidLockfile) {
				t.Fatalf("Encode(import %q) = %v", value, err)
			}
		})
		t.Run("asset_"+strings.ReplaceAll(value, "/", "_"), func(t *testing.T) {
			lockfile := validLockfile()
			lockfile.Entries[0].Assets[0].Path = value
			if _, err := Encode(lockfile); !errors.Is(err, ErrInvalidLockfile) {
				t.Fatalf("Encode(asset %q) = %v", value, err)
			}
		})
	}

	tests := []struct {
		name   string
		mutate func(*Lockfile)
	}{
		{"entry order", func(lockfile *Lockfile) {
			lockfile.Entries[0], lockfile.Entries[1] = lockfile.Entries[1], lockfile.Entries[0]
		}},
		{"duplicate entry", func(lockfile *Lockfile) { lockfile.Entries[1].ImportPath = lockfile.Entries[0].ImportPath }},
		{"asset order", func(lockfile *Lockfile) {
			lockfile.Entries[0].Assets[0], lockfile.Entries[0].Assets[1] = lockfile.Entries[0].Assets[1], lockfile.Entries[0].Assets[0]
		}},
		{"duplicate asset", func(lockfile *Lockfile) { lockfile.Entries[0].Assets[1].Path = lockfile.Entries[0].Assets[0].Path }},
		{"empty version", func(lockfile *Lockfile) { lockfile.Entries[0].Version = "" }},
		{"noncanonical version", func(lockfile *Lockfile) { lockfile.Entries[0].Version = "v1/next" }},
		{"short digest", func(lockfile *Lockfile) { lockfile.Entries[0].ContentDigest = "abc" }},
		{"uppercase digest", func(lockfile *Lockfile) { lockfile.Entries[0].ContentDigest = Digest(strings.Repeat("A", 64)) }},
		{"nonhex digest", func(lockfile *Lockfile) { lockfile.Entries[0].Assets[0].Digest = Digest(strings.Repeat("g", 64)) }},
		{"signature policy", func(lockfile *Lockfile) { lockfile.Entries[0].SignaturePolicy = "verify_someday" }},
		{"offline policy", func(lockfile *Lockfile) { lockfile.Entries[0].OfflinePolicy = "maybe" }},
		{"schema", func(lockfile *Lockfile) { lockfile.SchemaVersion++ }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lockfile := validLockfile()
			test.mutate(&lockfile)
			if _, err := Encode(lockfile); err == nil {
				t.Fatal("invalid lockfile unexpectedly encoded")
			}
		})
	}
}

func TestLockfileEnforcesEveryConfiguredAndHardLimit(t *testing.T) {
	lockfile := validLockfile()
	limits := DefaultLimits()
	limits.MaxEntries = 1
	if _, err := EncodeWithLimits(lockfile, limits); !errors.Is(err, ErrLockfileLimit) {
		t.Fatalf("entry-limited Encode() = %v", err)
	}
	limits = DefaultLimits()
	limits.MaxAssets = 1
	if _, err := EncodeWithLimits(lockfile, limits); !errors.Is(err, ErrLockfileLimit) {
		t.Fatalf("asset-limited Encode() = %v", err)
	}
	limits = DefaultLimits()
	limits.MaxPathBytes = 4
	if _, err := EncodeWithLimits(lockfile, limits); !errors.Is(err, ErrInvalidLockfile) || !errors.Is(err, ErrLockfileLimit) {
		t.Fatalf("path-limited Encode() = %v", err)
	}
	limits = DefaultLimits()
	limits.MaxStateBytes = 64
	if _, err := EncodeWithLimits(lockfile, limits); !errors.Is(err, ErrLockfileLimit) {
		t.Fatalf("state-limited Encode() = %v", err)
	}
	canonical, _ := Encode(lockfile)
	limits = DefaultLimits()
	limits.MaxLockfileBytes = uint64(len(canonical) - 1)
	if _, err := DecodeWithLimits(canonical, limits); !errors.Is(err, ErrLockfileLimit) {
		t.Fatalf("byte-limited Decode() = %v", err)
	}

	invalidLimits := []Limits{{}, func() Limits { value := DefaultLimits(); value.MaxEntries = HardMaxEntries + 1; return value }()}
	for _, limits := range invalidLimits {
		if _, err := EncodeWithLimits(lockfile, limits); !errors.Is(err, ErrLockfileLimit) {
			t.Fatalf("EncodeWithLimits(%+v) = %v", limits, err)
		}
	}
}

func TestProjectDigestChangesForEveryTransitiveIdentity(t *testing.T) {
	base := validLockfile()
	want, _ := base.ProjectDigest()
	mutations := []func(*Lockfile){
		func(lockfile *Lockfile) { lockfile.Entries[0].Version = "v1.4.1" },
		func(lockfile *Lockfile) { lockfile.Entries[0].ContentDigest = digest('e') },
		func(lockfile *Lockfile) { lockfile.Entries[0].Assets[0].Digest = digest('e') },
		func(lockfile *Lockfile) { lockfile.Entries[0].SignaturePolicy = SignatureAllowUnsigned },
		func(lockfile *Lockfile) { lockfile.Entries[0].OfflinePolicy = NetworkAllowed },
	}
	for index, mutate := range mutations {
		candidate := validLockfile()
		mutate(&candidate)
		got, err := candidate.ProjectDigest()
		if err != nil || got == want {
			t.Fatalf("mutation %d digest = %q, %v; base %q", index, got, err, want)
		}
	}
}

func TestParseDigest(t *testing.T) {
	if got, err := ParseDigest(string(digest('a'))); err != nil || got != digest('a') {
		t.Fatalf("ParseDigest() = %q, %v", got, err)
	}
	if _, err := ParseDigest(strings.Repeat("A", 64)); err == nil {
		t.Fatal("uppercase digest unexpectedly parsed")
	}
}

func validLockfile() Lockfile {
	return Lockfile{SchemaVersion: LockfileSchemaVersion, Entries: []Entry{
		{ImportPath: "acme/charts", Version: "v1.4.0", ContentDigest: digest('a'), Assets: []Asset{
			{Path: "fonts/body.ttf", Digest: digest('b')},
			{Path: "images/chart.png", Digest: digest('c')},
		}, SignaturePolicy: SignatureRequired, OfflinePolicy: OfflineOnly},
		{ImportPath: "acme/forms", Version: "2026.07+approved", ContentDigest: digest('d'), SignaturePolicy: SignatureAllowUnsigned, OfflinePolicy: NetworkAllowed},
	}}
}

func digest(character byte) Digest {
	return Digest(strings.Repeat(string(character), 64))
}

func TestDecodeRejectsUnsortedCanonicalShape(t *testing.T) {
	lockfile := validLockfile()
	lockfile.Entries[0], lockfile.Entries[1] = lockfile.Entries[1], lockfile.Entries[0]
	encoded, err := json.Marshal(lockfile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(encoded); !errors.Is(err, ErrInvalidLockfile) {
		t.Fatalf("Decode(unsorted) = %v", err)
	}
}
