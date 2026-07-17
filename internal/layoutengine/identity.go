// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxIdentityBytes = 4 << 10

const maxSourceNodeIDBytes = 128

const maxRegionIDBytes = 128

// RegionID identifies a canonical page-master region.
type RegionID string

const (
	// RegionHeader is the page-master band above normal body flow.
	RegionHeader RegionID = "header"
	// RegionBody is the primary normal-flow region on a page.
	RegionBody RegionID = "body"
	// RegionFooter is the page-master band below normal body flow.
	RegionFooter RegionID = "footer"
)

// NewRegionID validates and constructs a lowercase region slug.
func NewRegionID(value string) (RegionID, error) {
	if len(value) == 0 || len(value) > maxRegionIDBytes {
		return "", errors.New("layoutengine: region ID must be a non-empty slug")
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		if (c >= 'a' && c <= 'z') ||
			(i > 0 && c >= '0' && c <= '9') ||
			(i > 0 && (c == '-' || c == '_')) {
			continue
		}
		return "", errors.New("layoutengine: region ID is not a canonical lowercase slug")
	}
	return RegionID(value), nil
}

// Valid reports whether id is a canonical region identity.
func (id RegionID) Valid() bool {
	_, err := NewRegionID(string(id))
	return err == nil
}

// SourceNodeID is a durable, human-authored source handle such as
// "@invoice-lines". Its value is opaque so invalid slugs cannot be constructed
// outside this package.
type SourceNodeID struct {
	value string
}

// NewSourceNodeID validates and constructs a readable, case-sensitive @slug.
func NewSourceNodeID(value string) (SourceNodeID, error) {
	if len(value) < 2 || len(value) > maxSourceNodeIDBytes || value[0] != '@' {
		return SourceNodeID{}, errors.New("layoutengine: source node ID must be an @slug")
	}
	for i := 1; i < len(value); i++ {
		c := value[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '_' {
			continue
		}
		return SourceNodeID{}, errors.New("layoutengine: source node ID contains a non-slug character")
	}
	first := value[1]
	if (first < 'a' || first > 'z') && (first < 'A' || first > 'Z') {
		return SourceNodeID{}, errors.New("layoutengine: source node ID slug must start with a letter")
	}
	return SourceNodeID{value: value}, nil
}

// Valid reports whether id contains a validated source handle.
func (id SourceNodeID) Valid() bool { return id.value != "" }

func (id SourceNodeID) String() string { return id.value }

// digestID is the shared storage implementation for distinct revision and
// derived-artifact identities. It is never exposed as an identity itself.
type digestID struct {
	sum [sha256.Size]byte
}

func parseDigestID(name, value string) (digestID, error) {
	if len(value) != hex.EncodedLen(sha256.Size) {
		return digestID{}, fmt.Errorf("layoutengine: %s must be 64 lowercase hexadecimal characters", name)
	}
	for _, c := range []byte(value) {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return digestID{}, fmt.Errorf("layoutengine: %s is not canonical lowercase hexadecimal", name)
		}
	}
	var sum [sha256.Size]byte
	if _, err := hex.Decode(sum[:], []byte(value)); err != nil {
		return digestID{}, fmt.Errorf("layoutengine: decode %s: %w", name, err)
	}
	if sum == ([sha256.Size]byte{}) {
		return digestID{}, fmt.Errorf("layoutengine: %s is the absent digest", name)
	}
	return digestID{sum: sum}, nil
}

func (id digestID) Valid() bool { return id.sum != ([sha256.Size]byte{}) }

func (id digestID) String() string { return hex.EncodeToString(id.sum[:]) }

// The following identities deliberately remain different Go types even though
// each stores a canonical SHA-256 digest. This prevents revisions, plans, and
// renders from being substituted accidentally.
type SourceRevisionID struct{ digestID }
type SemanticTemplateID struct{ digestID }
type ScenarioRevisionID struct{ digestID }
type PolicyRevisionID struct{ digestID }
type ResourceCatalogID struct{ digestID }
type PlanID struct{ digestID }
type RenderID struct{ digestID }

func ParseSourceRevisionID(value string) (SourceRevisionID, error) {
	id, err := parseDigestID("source revision ID", value)
	return SourceRevisionID{id}, err
}

func ParseSemanticTemplateID(value string) (SemanticTemplateID, error) {
	id, err := parseDigestID("semantic template ID", value)
	return SemanticTemplateID{id}, err
}

func ParseScenarioRevisionID(value string) (ScenarioRevisionID, error) {
	id, err := parseDigestID("scenario revision ID", value)
	return ScenarioRevisionID{id}, err
}

func ParsePolicyRevisionID(value string) (PolicyRevisionID, error) {
	id, err := parseDigestID("policy revision ID", value)
	return PolicyRevisionID{id}, err
}

func ParseResourceCatalogID(value string) (ResourceCatalogID, error) {
	id, err := parseDigestID("resource catalog ID", value)
	return ResourceCatalogID{id}, err
}

func ParsePlanID(value string) (PlanID, error) {
	id, err := parseDigestID("plan ID", value)
	return PlanID{id}, err
}

func ParseRenderID(value string) (RenderID, error) {
	id, err := parseDigestID("render ID", value)
	return RenderID{id}, err
}

// NodeKey is the source-stable, human- or tool-facing identity of a node.
// Unlike NodeID, it survives dense-tree reindexing.
type NodeKey string

// NodeID identifies a node in one dense compiled tree. Zero means absent.
type NodeID uint32

// InstanceID identifies one expanded component or repeated-data instance. Its
// canonical path is stable when the source key and repeated-data keys are
// stable.
type InstanceID string

// FragmentID identifies one positioned fragment inside a single LayoutPlan.
// It is intentionally unsuitable as a persistent editing target. Zero means
// absent.
type FragmentID uint32

// NewNodeKey validates and constructs a source-stable key.
func NewNodeKey(value string) (NodeKey, error) {
	if err := validateTextIdentity("node key", value); err != nil {
		return "", err
	}
	return NodeKey(value), nil
}

// Valid reports whether id denotes a node.
func (id NodeID) Valid() bool { return id != 0 }

// NewInstanceID validates and constructs a canonical expanded-instance path.
func NewInstanceID(value string) (InstanceID, error) {
	if err := validateTextIdentity("instance ID", value); err != nil {
		return "", err
	}
	return InstanceID(value), nil
}

// Valid reports whether id denotes an expanded instance.
func (id InstanceID) Valid() bool { return id != "" }

// Valid reports whether id denotes a plan-local fragment.
func (id FragmentID) Valid() bool { return id != 0 }

func validateTextIdentity(name, value string) error {
	if value == "" {
		return fmt.Errorf("layoutengine: %s is empty", name)
	}
	if len(value) > maxIdentityBytes {
		return fmt.Errorf("layoutengine: %s exceeds %d bytes", name, maxIdentityBytes)
	}
	if !utf8.ValidString(value) {
		return fmt.Errorf("layoutengine: %s is not valid UTF-8", name)
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("layoutengine: %s has surrounding whitespace", name)
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return fmt.Errorf("layoutengine: %s contains a control character", name)
		}
	}
	return nil
}

// SourcePosition is a UTF-8 source position. Offset is a zero-based byte
// offset; Line and Column are one-based display coordinates.
type SourcePosition struct {
	Offset uint64 `json:"offset"`
	Line   uint32 `json:"line"`
	Column uint32 `json:"column"`
}

func (p SourcePosition) validate(name string) error {
	if p.Line == 0 {
		return fmt.Errorf("layoutengine: %s line is zero", name)
	}
	if p.Column == 0 {
		return fmt.Errorf("layoutengine: %s column is zero", name)
	}
	return nil
}

// SourceSpan is a half-open source range [Start, End). The zero value denotes
// generated content with no direct source location.
type SourceSpan struct {
	File  string         `json:"file"`
	Start SourcePosition `json:"start"`
	End   SourcePosition `json:"end"`
}

// IsZero reports whether the span represents generated content.
func (s SourceSpan) IsZero() bool {
	return s.File == "" && s.Start == (SourcePosition{}) && s.End == (SourcePosition{})
}

// Validate checks that the span is either absent or a well-ordered source
// range. Start and End may be equal for a point diagnostic.
func (s SourceSpan) Validate() error {
	if s.IsZero() {
		return nil
	}
	if s.File == "" {
		return errors.New("layoutengine: source span file is empty")
	}
	if !utf8.ValidString(s.File) || strings.TrimSpace(s.File) != s.File {
		return errors.New("layoutengine: source span file is not canonical UTF-8")
	}
	for _, r := range s.File {
		if unicode.IsControl(r) {
			return errors.New("layoutengine: source span file contains a control character")
		}
	}
	if err := s.Start.validate("source span start"); err != nil {
		return err
	}
	if err := s.End.validate("source span end"); err != nil {
		return err
	}
	if s.End.Offset < s.Start.Offset {
		return errors.New("layoutengine: source span end precedes start")
	}
	if s.End.Line < s.Start.Line ||
		(s.End.Line == s.Start.Line && s.End.Column < s.Start.Column) {
		return errors.New("layoutengine: source span end position precedes start")
	}
	return nil
}
