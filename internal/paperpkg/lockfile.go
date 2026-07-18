// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

// Package paperpkg defines the bounded, deterministic storage contract for
// resolved Paper package imports. It deliberately performs no fetching and no
// signature verification.
package paperpkg

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

const (
	LockfileSchemaVersion uint16 = 1

	HardMaxLockfileBytes uint64 = 16 << 20
	HardMaxStateBytes    uint64 = 32 << 20
	HardMaxEntries       uint32 = 100_000
	HardMaxAssets        uint32 = 1_000_000
	HardMaxPathBytes     uint32 = 4 << 10
)

var (
	ErrInvalidLockfile  = errors.New("paperpkg: invalid lockfile")
	ErrLockfileLimit    = errors.New("paperpkg: lockfile limit exceeded")
	ErrLockfileSchema   = errors.New("paperpkg: lockfile schema mismatch")
	ErrNonCanonicalJSON = errors.New("paperpkg: lockfile JSON is not canonical")
)

// Digest is a lowercase hexadecimal SHA-256 digest.
type Digest string

func ParseDigest(value string) (Digest, error) {
	if err := validateDigest(Digest(value)); err != nil {
		return "", err
	}
	return Digest(value), nil
}

type SignaturePolicy string

const (
	SignatureRequired      SignaturePolicy = "required"
	SignatureOptional      SignaturePolicy = "optional"
	SignatureForbidden     SignaturePolicy = "forbidden"
	SignatureAllowUnsigned SignaturePolicy = "allow_unsigned"
)

func (policy SignaturePolicy) valid() bool {
	return policy == SignatureRequired || policy == SignatureOptional || policy == SignatureForbidden || policy == SignatureAllowUnsigned
}

type OfflinePolicy string

const (
	OfflineOnly    OfflinePolicy = "offline_only"
	NetworkAllowed OfflinePolicy = "network_allowed"
)

func (policy OfflinePolicy) valid() bool {
	return policy == OfflineOnly || policy == NetworkAllowed
}

// Asset records one project-root-relative asset and its resolved content digest.
type Asset struct {
	Path   string `json:"path"`
	Digest Digest `json:"digest"`
}

// Entry pins one normalized import path and its complete resolved identity.
// Assets must be sorted by Path. Policies state the resolver contract that
// produced the entry; this package does not execute either policy.
type Entry struct {
	ImportPath      string          `json:"import_path"`
	ContentDigest   Digest          `json:"content_digest"`
	Assets          []Asset         `json:"assets,omitempty"`
	SignaturePolicy SignaturePolicy `json:"signature_policy"`
	OfflinePolicy   OfflinePolicy   `json:"offline_policy"`
}

// Lockfile entries must be strictly sorted by ImportPath.
type Lockfile struct {
	SchemaVersion uint16  `json:"schema_version"`
	Entries       []Entry `json:"entries,omitempty"`
}

// Limits bound both trusted encoding and hostile decoding. Every field is
// required and may not exceed its implementation hard cap.
type Limits struct {
	MaxLockfileBytes uint64
	MaxStateBytes    uint64
	MaxEntries       uint32
	MaxAssets        uint32
	MaxPathBytes     uint32
}

func DefaultLimits() Limits {
	return Limits{
		MaxLockfileBytes: 4 << 20,
		MaxStateBytes:    8 << 20,
		MaxEntries:       10_000,
		MaxAssets:        100_000,
		MaxPathBytes:     1 << 10,
	}
}

func (limits Limits) validate() error {
	if limits.MaxLockfileBytes == 0 || limits.MaxStateBytes == 0 || limits.MaxEntries == 0 ||
		limits.MaxAssets == 0 || limits.MaxPathBytes == 0 {
		return fmt.Errorf("%w: every limit must be positive", ErrLockfileLimit)
	}
	if limits.MaxLockfileBytes > HardMaxLockfileBytes || limits.MaxStateBytes > HardMaxStateBytes ||
		limits.MaxEntries > HardMaxEntries || limits.MaxAssets > HardMaxAssets || limits.MaxPathBytes > HardMaxPathBytes {
		return fmt.Errorf("%w: configured limit exceeds an implementation hard cap", ErrLockfileLimit)
	}
	return nil
}

func (lockfile Lockfile) Validate() error {
	return lockfile.ValidateWithLimits(DefaultLimits())
}

func (lockfile Lockfile) ValidateWithLimits(limits Limits) error {
	if err := limits.validate(); err != nil {
		return err
	}
	if lockfile.SchemaVersion != LockfileSchemaVersion {
		return fmt.Errorf("%w: got %d, want %d", ErrLockfileSchema, lockfile.SchemaVersion, LockfileSchemaVersion)
	}
	if uint64(len(lockfile.Entries)) > uint64(limits.MaxEntries) {
		return fmt.Errorf("%w: too many entries", ErrLockfileLimit)
	}
	var stateBytes, assetCount uint64
	previousImport := ""
	for entryIndex, entry := range lockfile.Entries {
		entryPath := fmt.Sprintf("entries[%d]", entryIndex)
		if err := validateRelativePath(entry.ImportPath, limits.MaxPathBytes); err != nil {
			return invalidAt(entryPath+".import_path", err)
		}
		if entryIndex > 0 && entry.ImportPath <= previousImport {
			return invalidAt(entryPath+".import_path", errors.New("entries are not strictly sorted and unique"))
		}
		previousImport = entry.ImportPath
		if err := validateDigest(entry.ContentDigest); err != nil {
			return invalidAt(entryPath+".content_digest", err)
		}
		if !entry.SignaturePolicy.valid() {
			return invalidAt(entryPath+".signature_policy", errors.New("unsupported signature policy"))
		}
		if !entry.OfflinePolicy.valid() {
			return invalidAt(entryPath+".offline_policy", errors.New("unsupported offline policy"))
		}
		assetCount += uint64(len(entry.Assets))
		if assetCount > uint64(limits.MaxAssets) {
			return fmt.Errorf("%w: too many assets", ErrLockfileLimit)
		}
		stateBytes += uint64(len(entry.ImportPath) + len(entry.ContentDigest) + len(entry.SignaturePolicy) + len(entry.OfflinePolicy) + 96)
		previousAsset := ""
		for assetIndex, asset := range entry.Assets {
			assetPath := fmt.Sprintf("%s.assets[%d]", entryPath, assetIndex)
			if err := validateRelativePath(asset.Path, limits.MaxPathBytes); err != nil {
				return invalidAt(assetPath+".path", err)
			}
			if assetIndex > 0 && asset.Path <= previousAsset {
				return invalidAt(assetPath+".path", errors.New("assets are not strictly sorted and unique"))
			}
			previousAsset = asset.Path
			if err := validateDigest(asset.Digest); err != nil {
				return invalidAt(assetPath+".digest", err)
			}
			stateBytes += uint64(len(asset.Path) + len(asset.Digest) + 48)
			if stateBytes > limits.MaxStateBytes {
				return fmt.Errorf("%w: decoded state exceeds its byte budget", ErrLockfileLimit)
			}
		}
		if stateBytes > limits.MaxStateBytes {
			return fmt.Errorf("%w: decoded state exceeds its byte budget", ErrLockfileLimit)
		}
	}
	return nil
}

func Encode(lockfile Lockfile) ([]byte, error) {
	return EncodeWithLimits(lockfile, DefaultLimits())
}

func EncodeWithLimits(lockfile Lockfile, limits Limits) ([]byte, error) {
	if err := lockfile.ValidateWithLimits(limits); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(lockfile)
	if err != nil {
		return nil, fmt.Errorf("paperpkg: encode lockfile: %w", err)
	}
	if uint64(len(encoded)) > limits.MaxLockfileBytes {
		return nil, fmt.Errorf("%w: canonical JSON exceeds its byte budget", ErrLockfileLimit)
	}
	return encoded, nil
}

func Decode(encoded []byte) (Lockfile, error) {
	return DecodeWithLimits(encoded, DefaultLimits())
}

func DecodeWithLimits(encoded []byte, limits Limits) (Lockfile, error) {
	if err := limits.validate(); err != nil {
		return Lockfile{}, err
	}
	if uint64(len(encoded)) > limits.MaxLockfileBytes {
		return Lockfile{}, fmt.Errorf("%w: encoded lockfile exceeds its byte budget", ErrLockfileLimit)
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var lockfile Lockfile
	if err := decoder.Decode(&lockfile); err != nil {
		return Lockfile{}, fmt.Errorf("%w: %w", ErrInvalidLockfile, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("additional JSON value")
		}
		return Lockfile{}, fmt.Errorf("%w: trailing data: %w", ErrInvalidLockfile, err)
	}
	if err := lockfile.ValidateWithLimits(limits); err != nil {
		return Lockfile{}, err
	}
	canonical, err := EncodeWithLimits(lockfile, limits)
	if err != nil {
		return Lockfile{}, err
	}
	if !bytes.Equal(encoded, canonical) {
		return Lockfile{}, ErrNonCanonicalJSON
	}
	return cloneLockfile(lockfile), nil
}

// Lookup returns a detached entry using binary search over canonical order.
func (lockfile Lockfile) Lookup(importPath string) (Entry, bool) {
	index := sort.Search(len(lockfile.Entries), func(index int) bool {
		return lockfile.Entries[index].ImportPath >= importPath
	})
	if index == len(lockfile.Entries) || lockfile.Entries[index].ImportPath != importPath {
		return Entry{}, false
	}
	return cloneEntry(lockfile.Entries[index]), true
}

// ProjectDigest hashes the complete canonical lockfile, including policies and
// every transitive entry and asset digest.
func (lockfile Lockfile) ProjectDigest() (Digest, error) {
	return lockfile.ProjectDigestWithLimits(DefaultLimits())
}

func (lockfile Lockfile) ProjectDigestWithLimits(limits Limits) (Digest, error) {
	encoded, err := EncodeWithLimits(lockfile, limits)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return Digest(hex.EncodeToString(sum[:])), nil
}

func validateDigest(digest Digest) error {
	if len(digest) != sha256.Size*2 {
		return errors.New("digest must contain exactly 64 lowercase hexadecimal characters")
	}
	for _, character := range digest {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return errors.New("digest must contain exactly 64 lowercase hexadecimal characters")
		}
	}
	return nil
}

func validateRelativePath(value string, maxBytes uint32) error {
	if uint64(len(value)) > uint64(maxBytes) {
		return fmt.Errorf("%w: path exceeds its byte budget", ErrLockfileLimit)
	}
	if value == "" || !utf8.ValidString(value) || !norm.NFC.IsNormalString(value) {
		return errors.New("path is empty, invalid UTF-8, or not Unicode NFC")
	}
	if strings.Contains(value, `\`) || path.IsAbs(value) || path.Clean(value) != value || value == "." ||
		strings.ContainsAny(value, "?#%:") {
		return errors.New("path is not a normalized relative slash path")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "" || parsed.Host != "" {
		return errors.New("path must not be a URL")
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return errors.New("path contains an empty or traversal segment")
		}
	}
	for _, character := range value {
		if unicode.IsControl(character) || unicode.IsSpace(character) {
			return errors.New("path contains whitespace or a control character")
		}
	}
	return nil
}

func invalidAt(path string, err error) error {
	if errors.Is(err, ErrLockfileLimit) {
		return fmt.Errorf("%w: %s: %w", ErrInvalidLockfile, path, err)
	}
	return fmt.Errorf("%w: %s: %w", ErrInvalidLockfile, path, err)
}

func cloneLockfile(lockfile Lockfile) Lockfile {
	clone := Lockfile{SchemaVersion: lockfile.SchemaVersion, Entries: make([]Entry, len(lockfile.Entries))}
	for index, entry := range lockfile.Entries {
		clone.Entries[index] = cloneEntry(entry)
	}
	return clone
}

func cloneEntry(entry Entry) Entry {
	entry.Assets = append([]Asset(nil), entry.Assets...)
	return entry
}
