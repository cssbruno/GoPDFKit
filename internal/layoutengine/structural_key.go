// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"hash"
)

const anonymousStructuralKeyDomain = "paperrune.layout.anonymous-structural-key.v1"

type AnonymousStructuralKeyInput struct {
	Revision    SourceRevisionID
	Parent      NodeKey
	Kind        string
	Ordinal     uint32
	Fingerprint string
}

// DeriveAnonymousStructuralKey creates a deterministic key scoped to one exact
// source revision. Fingerprint is the lowercase SHA-256 of the anonymous
// node's canonical semantic subtree. Ordinal disambiguates identical siblings.
// These keys are intentionally unsuitable for cross-revision durable targets.
func DeriveAnonymousStructuralKey(input AnonymousStructuralKeyInput) (NodeKey, error) {
	if !input.Revision.Valid() {
		return "", errors.New("layoutengine: anonymous structural key requires a source revision")
	}
	if input.Parent != "" {
		if err := validateTextIdentity("anonymous structural parent", string(input.Parent)); err != nil {
			return "", err
		}
	}
	if !validStructuralKind(input.Kind) {
		return "", errors.New("layoutengine: anonymous structural kind is not a canonical lowercase slug")
	}
	fingerprint, err := parseDigestID("anonymous structural fingerprint", input.Fingerprint)
	if err != nil {
		return "", err
	}
	digest := sha256.New()
	writeStructuralField(digest, anonymousStructuralKeyDomain)
	writeStructuralField(digest, input.Revision.String())
	writeStructuralField(digest, string(input.Parent))
	writeStructuralField(digest, input.Kind)
	var ordinal [4]byte
	binary.BigEndian.PutUint32(ordinal[:], input.Ordinal)
	digest.Write(ordinal[:])
	writeStructuralField(digest, fingerprint.String())
	return NodeKey("anon/" + hex.EncodeToString(digest.Sum(nil))), nil
}

func writeStructuralField(digest hash.Hash, value string) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(value)))
	digest.Write(size[:])
	digest.Write([]byte(value))
}

func validStructuralKind(kind string) bool {
	if kind == "" || len(kind) > 128 {
		return false
	}
	for index, character := range []byte(kind) {
		if character >= 'a' && character <= 'z' || index > 0 && character >= '0' && character <= '9' || index > 0 && (character == '-' || character == '_') {
			continue
		}
		return false
	}
	return true
}
