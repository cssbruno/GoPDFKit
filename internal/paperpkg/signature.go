// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperpkg

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

const (
	SignatureEnvelopeSchemaVersion uint16 = 1
	HardMaxSignatureEnvelopeBytes  uint64 = 1 << 20
	HardMaxSignatures              uint32 = 4_096
	HardMaxTrustedKeys             uint32 = 4_096
	HardMaxKeyIDBytes              uint32 = 256
	signaturePayloadDomain                = "gopdfkit.paperpkg.entry-signature.v1"
)

var (
	ErrSignatureInvalid   = errors.New("paperpkg: invalid signature envelope or key")
	ErrSignatureLimit     = errors.New("paperpkg: signature limit exceeded")
	ErrSignatureCanonical = errors.New("paperpkg: signature envelope JSON is not canonical")
	ErrSignaturePolicy    = errors.New("paperpkg: signature policy violation")
	ErrSignatureKey       = errors.New("paperpkg: signature key is not trusted")
	ErrSignatureExpired   = errors.New("paperpkg: package signature is expired")
	ErrSignatureVerify    = errors.New("paperpkg: Ed25519 signature verification failed")
)

type SignatureLimits struct {
	MaxEnvelopeBytes uint64
	MaxSignatures    uint32
	MaxTrustedKeys   uint32
	MaxKeyIDBytes    uint32
}

func DefaultSignatureLimits() SignatureLimits {
	return SignatureLimits{MaxEnvelopeBytes: 256 << 10, MaxSignatures: 1_024, MaxTrustedKeys: 1_024, MaxKeyIDBytes: 128}
}

func (limits SignatureLimits) validate() error {
	if limits.MaxEnvelopeBytes == 0 || limits.MaxSignatures == 0 || limits.MaxTrustedKeys == 0 || limits.MaxKeyIDBytes == 0 {
		return fmt.Errorf("%w: every signature limit must be positive", ErrSignatureLimit)
	}
	if limits.MaxEnvelopeBytes > HardMaxSignatureEnvelopeBytes || limits.MaxSignatures > HardMaxSignatures ||
		limits.MaxTrustedKeys > HardMaxTrustedKeys || limits.MaxKeyIDBytes > HardMaxKeyIDBytes {
		return fmt.Errorf("%w: signature limit exceeds a hard cap", ErrSignatureLimit)
	}
	return nil
}

type TrustedKey struct {
	ID        string
	PublicKey ed25519.PublicKey
}

// EntrySignature is sorted by KeyID within an envelope. ExpiresUnix is zero
// for no expiry; otherwise the signature is valid only before that Unix time.
type EntrySignature struct {
	KeyID       string `json:"key_id"`
	ExpiresUnix int64  `json:"expires_unix,omitempty"`
	Signature   string `json:"signature"`
}

type SignatureEnvelope struct {
	SchemaVersion uint16           `json:"schema_version"`
	Signatures    []EntrySignature `json:"signatures"`
}

type SignatureVerifier struct {
	limits SignatureLimits
	keys   map[string]ed25519.PublicKey
}

func NewSignatureVerifier(keys []TrustedKey, limits SignatureLimits) (*SignatureVerifier, error) {
	if err := limits.validate(); err != nil {
		return nil, err
	}
	if uint64(len(keys)) > uint64(limits.MaxTrustedKeys) {
		return nil, fmt.Errorf("%w: too many trusted keys", ErrSignatureLimit)
	}
	verifier := &SignatureVerifier{limits: limits, keys: make(map[string]ed25519.PublicKey, len(keys))}
	for index, key := range keys {
		if err := validateKeyID(key.ID, limits.MaxKeyIDBytes); err != nil {
			return nil, fmt.Errorf("%w: trusted_keys[%d]: %v", ErrSignatureInvalid, index, err)
		}
		if len(key.PublicKey) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("%w: trusted key %q is not an Ed25519 public key", ErrSignatureInvalid, key.ID)
		}
		if _, duplicate := verifier.keys[key.ID]; duplicate {
			return nil, fmt.Errorf("%w: duplicate trusted key %q", ErrSignatureInvalid, key.ID)
		}
		verifier.keys[key.ID] = append(ed25519.PublicKey(nil), key.PublicKey...)
	}
	return verifier, nil
}

func EncodeSignatureEnvelope(envelope SignatureEnvelope) ([]byte, error) {
	return EncodeSignatureEnvelopeWithLimits(envelope, DefaultSignatureLimits())
}

func EncodeSignatureEnvelopeWithLimits(envelope SignatureEnvelope, limits SignatureLimits) ([]byte, error) {
	if err := validateSignatureEnvelope(envelope, limits); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSignatureInvalid, err)
	}
	if uint64(len(encoded)) > limits.MaxEnvelopeBytes {
		return nil, fmt.Errorf("%w: envelope exceeds its byte budget", ErrSignatureLimit)
	}
	return encoded, nil
}

func DecodeSignatureEnvelope(encoded []byte) (SignatureEnvelope, error) {
	return DecodeSignatureEnvelopeWithLimits(encoded, DefaultSignatureLimits())
}

func DecodeSignatureEnvelopeWithLimits(encoded []byte, limits SignatureLimits) (SignatureEnvelope, error) {
	if err := limits.validate(); err != nil {
		return SignatureEnvelope{}, err
	}
	if uint64(len(encoded)) > limits.MaxEnvelopeBytes {
		return SignatureEnvelope{}, fmt.Errorf("%w: envelope exceeds its byte budget", ErrSignatureLimit)
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var envelope SignatureEnvelope
	if err := decoder.Decode(&envelope); err != nil {
		return SignatureEnvelope{}, fmt.Errorf("%w: %v", ErrSignatureInvalid, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return SignatureEnvelope{}, fmt.Errorf("%w: trailing data", ErrSignatureInvalid)
	}
	canonical, err := EncodeSignatureEnvelopeWithLimits(envelope, limits)
	if err != nil {
		return SignatureEnvelope{}, err
	}
	if !bytes.Equal(encoded, canonical) {
		return SignatureEnvelope{}, ErrSignatureCanonical
	}
	return cloneSignatureEnvelope(envelope), nil
}

// SignEntry creates one deterministic envelope record. Ed25519 signing itself
// is deterministic. Callers combine records and sort them by KeyID.
func SignEntry(lockfile Lockfile, importPath, keyID string, privateKey ed25519.PrivateKey, expiresUnix int64) (EntrySignature, error) {
	if len(privateKey) != ed25519.PrivateKeySize {
		return EntrySignature{}, fmt.Errorf("%w: private key is not Ed25519", ErrSignatureInvalid)
	}
	if err := validateKeyID(keyID, HardMaxKeyIDBytes); err != nil {
		return EntrySignature{}, fmt.Errorf("%w: %v", ErrSignatureInvalid, err)
	}
	if expiresUnix < 0 {
		return EntrySignature{}, fmt.Errorf("%w: expiry must be non-negative", ErrSignatureInvalid)
	}
	payload, err := signaturePayload(lockfile, importPath, keyID, expiresUnix)
	if err != nil {
		return EntrySignature{}, err
	}
	signature := ed25519.Sign(privateKey, payload)
	return EntrySignature{KeyID: keyID, ExpiresUnix: expiresUnix, Signature: hex.EncodeToString(signature)}, nil
}

// VerifyEntry enforces the entry's signature policy using only the supplied
// trust keys and verification Unix time. It never consults the ambient clock.
func (verifier *SignatureVerifier) VerifyEntry(lockfile Lockfile, importPath string, envelope *SignatureEnvelope, verificationUnix int64) error {
	if verifier == nil || verificationUnix < 0 {
		return fmt.Errorf("%w: verifier and non-negative verification time are required", ErrSignatureInvalid)
	}
	entry, found := lockfile.Lookup(importPath)
	if !found {
		return fmt.Errorf("%w: lockfile entry %q was not found", ErrSignatureInvalid, importPath)
	}
	if err := lockfile.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrSignatureInvalid, err)
	}
	present := envelope != nil
	switch entry.SignaturePolicy {
	case SignatureForbidden:
		if present {
			return fmt.Errorf("%w: signatures are forbidden for %q", ErrSignaturePolicy, importPath)
		}
		return nil
	case SignatureRequired:
		if !present || len(envelope.Signatures) == 0 {
			return fmt.Errorf("%w: a signature is required for %q", ErrSignaturePolicy, importPath)
		}
	case SignatureOptional, SignatureAllowUnsigned:
		if !present {
			return nil
		}
	default:
		return fmt.Errorf("%w: unsupported policy %q", ErrSignaturePolicy, entry.SignaturePolicy)
	}
	if err := validateSignatureEnvelope(*envelope, verifier.limits); err != nil {
		return err
	}
	for _, signed := range envelope.Signatures {
		publicKey, trusted := verifier.keys[signed.KeyID]
		if !trusted {
			return fmt.Errorf("%w: %q", ErrSignatureKey, signed.KeyID)
		}
		if signed.ExpiresUnix != 0 && verificationUnix >= signed.ExpiresUnix {
			return fmt.Errorf("%w: key %q expired at %d", ErrSignatureExpired, signed.KeyID, signed.ExpiresUnix)
		}
		payload, err := signaturePayload(lockfile, importPath, signed.KeyID, signed.ExpiresUnix)
		if err != nil {
			return err
		}
		signature, _ := hex.DecodeString(signed.Signature)
		if !ed25519.Verify(publicKey, payload, signature) {
			return fmt.Errorf("%w: key %q", ErrSignatureVerify, signed.KeyID)
		}
	}
	return nil
}

type canonicalSignaturePayload struct {
	Domain                string          `json:"domain"`
	LockfileSchemaVersion uint16          `json:"lockfile_schema_version"`
	ProjectDigest         Digest          `json:"project_digest"`
	ImportPath            string          `json:"import_path"`
	ContentDigest         Digest          `json:"content_digest"`
	Assets                []Asset         `json:"assets,omitempty"`
	SignaturePolicy       SignaturePolicy `json:"signature_policy"`
	OfflinePolicy         OfflinePolicy   `json:"offline_policy"`
	KeyID                 string          `json:"key_id"`
	ExpiresUnix           int64           `json:"expires_unix,omitempty"`
}

func signaturePayload(lockfile Lockfile, importPath, keyID string, expiresUnix int64) ([]byte, error) {
	if err := lockfile.Validate(); err != nil {
		return nil, fmt.Errorf("%w: lockfile: %v", ErrSignatureInvalid, err)
	}
	entry, found := lockfile.Lookup(importPath)
	if !found {
		return nil, fmt.Errorf("%w: lockfile entry %q was not found", ErrSignatureInvalid, importPath)
	}
	projectDigest, err := lockfile.ProjectDigest()
	if err != nil {
		return nil, err
	}
	payload := canonicalSignaturePayload{Domain: signaturePayloadDomain, LockfileSchemaVersion: lockfile.SchemaVersion,
		ProjectDigest: projectDigest, ImportPath: entry.ImportPath, ContentDigest: entry.ContentDigest,
		Assets: append([]Asset(nil), entry.Assets...), SignaturePolicy: entry.SignaturePolicy,
		OfflinePolicy: entry.OfflinePolicy, KeyID: keyID, ExpiresUnix: expiresUnix}
	return json.Marshal(payload)
}

func validateSignatureEnvelope(envelope SignatureEnvelope, limits SignatureLimits) error {
	if err := limits.validate(); err != nil {
		return err
	}
	if envelope.SchemaVersion != SignatureEnvelopeSchemaVersion {
		return fmt.Errorf("%w: envelope schema is %d, want %d", ErrSignatureInvalid, envelope.SchemaVersion, SignatureEnvelopeSchemaVersion)
	}
	if envelope.Signatures == nil {
		return fmt.Errorf("%w: signatures must be a JSON array", ErrSignatureInvalid)
	}
	if uint64(len(envelope.Signatures)) > uint64(limits.MaxSignatures) {
		return fmt.Errorf("%w: too many signatures", ErrSignatureLimit)
	}
	previous := ""
	for index, signature := range envelope.Signatures {
		if err := validateKeyID(signature.KeyID, limits.MaxKeyIDBytes); err != nil {
			return fmt.Errorf("%w: signatures[%d]: %v", ErrSignatureInvalid, index, err)
		}
		if index > 0 && signature.KeyID <= previous {
			return fmt.Errorf("%w: signatures must be strictly sorted and unique", ErrSignatureInvalid)
		}
		previous = signature.KeyID
		if signature.ExpiresUnix < 0 {
			return fmt.Errorf("%w: signature expiry is negative", ErrSignatureInvalid)
		}
		if len(signature.Signature) != ed25519.SignatureSize*2 {
			return fmt.Errorf("%w: signature for %q has the wrong size", ErrSignatureInvalid, signature.KeyID)
		}
		decoded, err := hex.DecodeString(signature.Signature)
		if err != nil || hex.EncodeToString(decoded) != signature.Signature {
			return fmt.Errorf("%w: signature for %q is not lowercase hexadecimal", ErrSignatureInvalid, signature.KeyID)
		}
	}
	return nil
}

func validateKeyID(value string, maxBytes uint32) error {
	if value == "" || uint64(len(value)) > uint64(maxBytes) {
		return errors.New("key ID is empty or exceeds its byte limit")
	}
	for _, character := range value {
		if !((character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') ||
			character == '.' || character == '_' || character == '-') {
			return errors.New("key ID must use lowercase ASCII letters, digits, dot, underscore, or hyphen")
		}
	}
	return nil
}

func cloneSignatureEnvelope(envelope SignatureEnvelope) SignatureEnvelope {
	signatures := make([]EntrySignature, len(envelope.Signatures))
	copy(signatures, envelope.Signatures)
	envelope.Signatures = signatures
	return envelope
}

func SortEntrySignatures(signatures []EntrySignature) []EntrySignature {
	clone := append([]EntrySignature(nil), signatures...)
	sort.Slice(clone, func(i, j int) bool { return strings.Compare(clone[i].KeyID, clone[j].KeyID) < 0 })
	return clone
}
