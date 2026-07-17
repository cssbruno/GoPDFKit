// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperpkg

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"strings"
	"testing"
)

func TestEd25519SignatureEnvelopeCanonicalRoundTripAndVerification(t *testing.T) {
	lockfile := validLockfile()
	publicKey, privateKey := deterministicKey('a')
	signed, err := SignEntry(lockfile, "acme/charts", "release-key", privateKey, 2_000)
	if err != nil {
		t.Fatal(err)
	}
	envelope := SignatureEnvelope{SchemaVersion: SignatureEnvelopeSchemaVersion, Signatures: []EntrySignature{signed}}
	encoded, err := EncodeSignatureEnvelope(envelope)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeSignatureEnvelope(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded.Signatures) != 1 || decoded.Signatures[0] != signed || !strings.Contains(string(encoded), `"key_id":"release-key"`) {
		t.Fatalf("signature envelope = %s / %#v", encoded, decoded)
	}
	verifier := newTestSignatureVerifier(t, []TrustedKey{{ID: "release-key", PublicKey: publicKey}})
	if err := verifier.VerifyEntry(lockfile, "acme/charts", &decoded, 1_999); err != nil {
		t.Fatalf("VerifyEntry() = %v", err)
	}
	decoded.Signatures[0].Signature = strings.Repeat("0", ed25519.SignatureSize*2)
	if envelope.Signatures[0].Signature != signed.Signature {
		t.Fatal("decoded envelope aliases input signature storage")
	}
	second, err := SignEntry(lockfile, "acme/charts", "release-key", privateKey, 2_000)
	if err != nil || second != signed {
		t.Fatalf("deterministic SignEntry() = %#v, %v; want %#v", second, err, signed)
	}
}

func TestSignatureBindsProjectEntryAssetsAndPolicies(t *testing.T) {
	base := validLockfile()
	publicKey, privateKey := deterministicKey('a')
	signed, _ := SignEntry(base, "acme/charts", "release-key", privateKey, 0)
	envelope := &SignatureEnvelope{SchemaVersion: SignatureEnvelopeSchemaVersion, Signatures: []EntrySignature{signed}}
	verifier := newTestSignatureVerifier(t, []TrustedKey{{ID: "release-key", PublicKey: publicKey}})
	mutations := []struct {
		name   string
		mutate func(*Lockfile)
	}{
		{"content", func(lockfile *Lockfile) { lockfile.Entries[0].ContentDigest = digest('e') }},
		{"asset", func(lockfile *Lockfile) { lockfile.Entries[0].Assets[0].Digest = digest('e') }},
		{"offline policy", func(lockfile *Lockfile) { lockfile.Entries[0].OfflinePolicy = NetworkAllowed }},
		{"signature policy", func(lockfile *Lockfile) { lockfile.Entries[0].SignaturePolicy = SignatureOptional }},
		{"other transitive entry", func(lockfile *Lockfile) { lockfile.Entries[1].ContentDigest = digest('e') }},
	}
	for _, test := range mutations {
		t.Run(test.name, func(t *testing.T) {
			candidate := cloneLockfile(base)
			test.mutate(&candidate)
			if err := verifier.VerifyEntry(candidate, "acme/charts", envelope, 1); !errors.Is(err, ErrSignatureVerify) {
				t.Fatalf("tampered VerifyEntry() = %v", err)
			}
		})
	}
}

func TestSignatureVerifierRejectsWrongUnknownAndDuplicateKeys(t *testing.T) {
	lockfile := validLockfile()
	publicKey, privateKey := deterministicKey('a')
	_, wrongPrivate := deterministicKey('b')
	wrongPublic := wrongPrivate.Public().(ed25519.PublicKey)
	signed, _ := SignEntry(lockfile, "acme/charts", "release-key", privateKey, 0)
	envelope := &SignatureEnvelope{SchemaVersion: SignatureEnvelopeSchemaVersion, Signatures: []EntrySignature{signed}}
	wrong := newTestSignatureVerifier(t, []TrustedKey{{ID: "release-key", PublicKey: wrongPublic}})
	if err := wrong.VerifyEntry(lockfile, "acme/charts", envelope, 1); !errors.Is(err, ErrSignatureVerify) {
		t.Fatalf("wrong-key VerifyEntry() = %v", err)
	}
	unknown := newTestSignatureVerifier(t, []TrustedKey{{ID: "other-key", PublicKey: publicKey}})
	if err := unknown.VerifyEntry(lockfile, "acme/charts", envelope, 1); !errors.Is(err, ErrSignatureKey) {
		t.Fatalf("unknown-key VerifyEntry() = %v", err)
	}
	limits := DefaultSignatureLimits()
	if verifier, err := NewSignatureVerifier([]TrustedKey{{ID: "release-key", PublicKey: publicKey}, {ID: "release-key", PublicKey: publicKey}}, limits); err == nil || verifier != nil {
		t.Fatalf("duplicate NewSignatureVerifier() = %#v, %v", verifier, err)
	}
	if verifier, err := NewSignatureVerifier([]TrustedKey{{ID: "BAD KEY", PublicKey: publicKey}}, limits); err == nil || verifier != nil {
		t.Fatalf("invalid-key NewSignatureVerifier() = %#v, %v", verifier, err)
	}
}

func TestSignaturePolicyAndExplicitExpiryEnforcement(t *testing.T) {
	publicKey, privateKey := deterministicKey('a')
	verifier := newTestSignatureVerifier(t, []TrustedKey{{ID: "release-key", PublicKey: publicKey}})
	required := validLockfile()
	if err := verifier.VerifyEntry(required, "acme/charts", nil, 1); !errors.Is(err, ErrSignaturePolicy) {
		t.Fatalf("required unsigned VerifyEntry() = %v", err)
	}
	signed, _ := SignEntry(required, "acme/charts", "release-key", privateKey, 100)
	envelope := &SignatureEnvelope{SchemaVersion: SignatureEnvelopeSchemaVersion, Signatures: []EntrySignature{signed}}
	if err := verifier.VerifyEntry(required, "acme/charts", envelope, 99); err != nil {
		t.Fatalf("pre-expiry VerifyEntry() = %v", err)
	}
	if err := verifier.VerifyEntry(required, "acme/charts", envelope, 100); !errors.Is(err, ErrSignatureExpired) {
		t.Fatalf("expiry-boundary VerifyEntry() = %v", err)
	}
	if err := verifier.VerifyEntry(required, "acme/charts", envelope, -1); !errors.Is(err, ErrSignatureInvalid) {
		t.Fatalf("negative-time VerifyEntry() = %v", err)
	}

	optional := cloneLockfile(required)
	optional.Entries[0].SignaturePolicy = SignatureOptional
	if err := verifier.VerifyEntry(optional, "acme/charts", nil, 1); err != nil {
		t.Fatalf("optional unsigned VerifyEntry() = %v", err)
	}
	forbidden := cloneLockfile(required)
	forbidden.Entries[0].SignaturePolicy = SignatureForbidden
	if err := verifier.VerifyEntry(forbidden, "acme/charts", nil, 1); err != nil {
		t.Fatalf("forbidden absent VerifyEntry() = %v", err)
	}
	forbiddenSigned, _ := SignEntry(forbidden, "acme/charts", "release-key", privateKey, 0)
	forbiddenEnvelope := &SignatureEnvelope{SchemaVersion: SignatureEnvelopeSchemaVersion, Signatures: []EntrySignature{forbiddenSigned}}
	if err := verifier.VerifyEntry(forbidden, "acme/charts", forbiddenEnvelope, 1); !errors.Is(err, ErrSignaturePolicy) {
		t.Fatalf("forbidden signed VerifyEntry() = %v", err)
	}
}

func TestSignatureEnvelopeRejectsNoncanonicalUnknownTrailingAndInvalidRecords(t *testing.T) {
	lockfile := validLockfile()
	_, privateA := deterministicKey('a')
	_, privateB := deterministicKey('b')
	a, _ := SignEntry(lockfile, "acme/charts", "a-key", privateA, 0)
	b, _ := SignEntry(lockfile, "acme/charts", "b-key", privateB, 0)
	canonical, _ := EncodeSignatureEnvelope(SignatureEnvelope{SchemaVersion: SignatureEnvelopeSchemaVersion, Signatures: []EntrySignature{a}})
	tests := []struct {
		name string
		data []byte
		want error
	}{
		{"unknown", []byte(`{"schema_version":1,"signatures":[],"unknown":true}`), ErrSignatureInvalid},
		{"trailing value", append(append([]byte(nil), canonical...), []byte(`{}`)...), ErrSignatureInvalid},
		{"whitespace", append([]byte{' '}, canonical...), ErrSignatureCanonical},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := DecodeSignatureEnvelope(test.data); !errors.Is(err, test.want) {
				t.Fatalf("DecodeSignatureEnvelope() = %v, want %v", err, test.want)
			}
		})
	}
	invalid := []SignatureEnvelope{
		{SchemaVersion: 1},
		{SchemaVersion: 2, Signatures: []EntrySignature{a}},
		{SchemaVersion: 1, Signatures: []EntrySignature{b, a}},
		{SchemaVersion: 1, Signatures: []EntrySignature{a, a}},
		{SchemaVersion: 1, Signatures: []EntrySignature{{KeyID: "a-key", Signature: "bad"}}},
		{SchemaVersion: 1, Signatures: []EntrySignature{{KeyID: "BAD", Signature: strings.Repeat("0", 128)}}},
		{SchemaVersion: 1, Signatures: []EntrySignature{{KeyID: "a-key", ExpiresUnix: -1, Signature: strings.Repeat("0", 128)}}},
	}
	for index, envelope := range invalid {
		if _, err := EncodeSignatureEnvelope(envelope); err == nil {
			t.Fatalf("invalid envelope %d unexpectedly encoded", index)
		}
	}
}

func TestSignatureLimitsAndTamperedSignature(t *testing.T) {
	lockfile := validLockfile()
	publicKey, privateKey := deterministicKey('a')
	signed, _ := SignEntry(lockfile, "acme/charts", "release-key", privateKey, 0)
	envelope := SignatureEnvelope{SchemaVersion: 1, Signatures: []EntrySignature{signed}}
	limits := DefaultSignatureLimits()
	limits.MaxSignatures = 1
	tooMany := envelope
	second := signed
	second.KeyID = "z-key"
	tooMany.Signatures = []EntrySignature{signed, second}
	if _, err := EncodeSignatureEnvelopeWithLimits(tooMany, limits); !errors.Is(err, ErrSignatureLimit) {
		t.Fatalf("count-limited envelope = %v", err)
	}
	encoded, _ := EncodeSignatureEnvelope(envelope)
	limits = DefaultSignatureLimits()
	limits.MaxEnvelopeBytes = uint64(len(encoded) - 1)
	if _, err := DecodeSignatureEnvelopeWithLimits(encoded, limits); !errors.Is(err, ErrSignatureLimit) {
		t.Fatalf("byte-limited envelope = %v", err)
	}
	limits = DefaultSignatureLimits()
	limits.MaxTrustedKeys = 1
	if verifier, err := NewSignatureVerifier([]TrustedKey{{ID: "a", PublicKey: publicKey}, {ID: "b", PublicKey: publicKey}}, limits); !errors.Is(err, ErrSignatureLimit) || verifier != nil {
		t.Fatalf("key-count-limited verifier = %#v, %v", verifier, err)
	}

	tampered := envelope
	tampered.Signatures = append([]EntrySignature(nil), envelope.Signatures...)
	tampered.Signatures[0].Signature = "0" + tampered.Signatures[0].Signature[1:]
	verifier := newTestSignatureVerifier(t, []TrustedKey{{ID: "release-key", PublicKey: publicKey}})
	if err := verifier.VerifyEntry(lockfile, "acme/charts", &tampered, 1); !errors.Is(err, ErrSignatureVerify) {
		t.Fatalf("tampered signature VerifyEntry() = %v", err)
	}
}

func deterministicKey(character byte) (ed25519.PublicKey, ed25519.PrivateKey) {
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{character}, ed25519.SeedSize))
	return append(ed25519.PublicKey(nil), privateKey[ed25519.SeedSize:]...), privateKey
}

func newTestSignatureVerifier(t *testing.T, keys []TrustedKey) *SignatureVerifier {
	t.Helper()
	verifier, err := NewSignatureVerifier(keys, DefaultSignatureLimits())
	if err != nil {
		t.Fatal(err)
	}
	return verifier
}
