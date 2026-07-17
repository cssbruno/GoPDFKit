// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

// Package papertheme resolves bounded, typed lexical design tokens without
// consulting parsers, layout engines, I/O, clocks, or process state.
package papertheme

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// Kind is the closed set of token/property value types.
type Kind string

const (
	Color  Kind = "color"
	String Kind = "string"
	Length Kind = "length"
	Number Kind = "number"
	Bool   Kind = "bool"
)

// Source identifies an authored declaration without coupling this package to
// a particular parser span type. Offsets are half-open UTF-8 byte offsets.
type Source struct {
	File        string `json:"file,omitempty"`
	StartOffset uint64 `json:"start_offset,omitempty"`
	EndOffset   uint64 `json:"end_offset,omitempty"`
	Line        uint32 `json:"line,omitempty"`
	Column      uint32 `json:"column,omitempty"`
}

type LengthValue struct {
	Number string `json:"number"`
	Unit   string `json:"unit"`
}

// Value is a closed tagged union. Number and Length.Number use canonical
// decimal spelling to avoid floating-point and locale ambiguity.
type Value struct {
	Kind   Kind        `json:"kind"`
	Color  string      `json:"color,omitempty"`
	String string      `json:"string,omitempty"`
	Length LengthValue `json:"length,omitempty"`
	Number string      `json:"number,omitempty"`
	Bool   bool        `json:"bool,omitempty"`
}

// Token is either a literal Value or an alias Reference. Kind is mandatory in
// both forms and makes alias type mismatches deterministic.
type Token struct {
	Name      string `json:"name"`
	Kind      Kind   `json:"kind"`
	Value     Value  `json:"value"`
	Reference string `json:"reference,omitempty"`
	Source    Source `json:"source"`
}

// Scope is an authored lexical scope. References first search the declaring
// scope, then its lexical ancestors, then the parent theme in the same manner.
type Scope struct {
	Name   string  `json:"name"`
	Tokens []Token `json:"tokens,omitempty"`
	Scopes []Scope `json:"scopes,omitempty"`
	Source Source  `json:"source"`
}

type Theme struct {
	Name   string  `json:"name"`
	Parent string  `json:"parent,omitempty"`
	Tokens []Token `json:"tokens,omitempty"`
	Scopes []Scope `json:"scopes,omitempty"`
	Source Source  `json:"source"`
}

// Property asks the resolver to compute one typed consumer property.
type Property struct {
	Name   string   `json:"name"`
	Theme  string   `json:"theme"`
	Scope  []string `json:"scope,omitempty"`
	Token  string   `json:"token"`
	Kind   Kind     `json:"kind"`
	Source Source   `json:"source"`
}

type Input struct {
	Themes     []Theme    `json:"themes,omitempty"`
	Properties []Property `json:"properties,omitempty"`
}

type TokenStep struct {
	Theme  string   `json:"theme"`
	Scope  []string `json:"scope,omitempty"`
	Token  string   `json:"token"`
	Source Source   `json:"source"`
}

// Provenance records the requesting property and every alias/literal token
// declaration followed to compute it.
type Provenance struct {
	Property Source      `json:"property_source,omitempty"`
	Chain    []TokenStep `json:"token_chain"`
}

type ResolvedToken struct {
	Name       string     `json:"name"`
	Scope      []string   `json:"scope,omitempty"`
	Value      Value      `json:"value"`
	Provenance Provenance `json:"provenance"`
}

type ResolvedTheme struct {
	Name   string          `json:"name"`
	Parent string          `json:"parent,omitempty"`
	Tokens []ResolvedToken `json:"tokens,omitempty"`
}

type ComputedProperty struct {
	Name       string     `json:"name"`
	Theme      string     `json:"theme"`
	Scope      []string   `json:"scope,omitempty"`
	Value      Value      `json:"value"`
	Provenance Provenance `json:"provenance"`
}

// Output is map-free and canonically ordered by Resolve.
type Output struct {
	Themes     []ResolvedTheme    `json:"themes,omitempty"`
	Properties []ComputedProperty `json:"properties,omitempty"`
}

func (o Output) CanonicalJSON() ([]byte, error) { return json.Marshal(o) }

type Severity string

const Error Severity = "error"

type Diagnostic struct {
	Code     string   `json:"code"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
	Hint     string   `json:"hint,omitempty"`
	Source   Source   `json:"source"`
}

type Result struct {
	Output      Output       `json:"output"`
	Canonical   []byte       `json:"canonical"`
	Digest      string       `json:"digest"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
}

func (r Result) OK() bool {
	for _, diagnostic := range r.Diagnostics {
		if diagnostic.Severity == Error {
			return false
		}
	}
	return true
}

func canonicalResult(output Output) ([]byte, string) {
	encoded, err := output.CanonicalJSON()
	if err != nil {
		// Output contains only JSON-safe closed values, so this is unreachable.
		return nil, ""
	}
	sum := sha256.Sum256(encoded)
	return encoded, hex.EncodeToString(sum[:])
}

type Limits struct {
	MaxThemes      uint32 `json:"max_themes"`
	MaxTokens      uint32 `json:"max_tokens"`
	MaxDepth       uint32 `json:"max_depth"`
	MaxWork        uint64 `json:"max_work"`
	MaxSourceBytes uint64 `json:"max_source_bytes"`
}

func DefaultLimits() Limits {
	return Limits{MaxThemes: 1024, MaxTokens: 100_000, MaxDepth: 64, MaxWork: 1_000_000, MaxSourceBytes: 8 << 20}
}
