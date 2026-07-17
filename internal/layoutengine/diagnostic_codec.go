// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const DiagnosticSchemaVersion uint16 = 1

var ErrDiagnosticCodecLimit = errors.New("layoutengine: diagnostic codec limit exceeded")

type DiagnosticCodecLimits struct {
	MaxDiagnostics uint32
	MaxBytes       uint64
}

func DefaultDiagnosticCodecLimits() DiagnosticCodecLimits {
	return DiagnosticCodecLimits{MaxDiagnostics: 1 << 16, MaxBytes: 16 << 20}
}

// DiagnosticSet is the stable versioned transport envelope. Diagnostic order
// is causal and preserved; evidence within each diagnostic is canonicalized by
// key through the same cloning rule used by LayoutPlan.
type DiagnosticSet struct {
	SchemaVersion uint16       `json:"schema_version"`
	Diagnostics   []Diagnostic `json:"diagnostics"`
}

func EncodeDiagnosticSet(diagnostics []Diagnostic, limits DiagnosticCodecLimits) ([]byte, error) {
	limits, err := normalizeDiagnosticCodecLimits(limits)
	if err != nil {
		return nil, err
	}
	if uint64(len(diagnostics)) > uint64(limits.MaxDiagnostics) {
		return nil, fmt.Errorf("%w: diagnostic count", ErrDiagnosticCodecLimit)
	}
	set := DiagnosticSet{SchemaVersion: DiagnosticSchemaVersion, Diagnostics: make([]Diagnostic, len(diagnostics))}
	for index, diagnostic := range diagnostics {
		if err := diagnostic.Validate(); err != nil {
			return nil, fmt.Errorf("layoutengine: diagnostics[%d]: %w", index, err)
		}
		set.Diagnostics[index] = cloneDiagnostic(diagnostic)
	}
	encoded, err := json.Marshal(set)
	if err != nil {
		return nil, fmt.Errorf("layoutengine: encode diagnostics: %w", err)
	}
	if uint64(len(encoded)) > limits.MaxBytes {
		return nil, fmt.Errorf("%w: encoded bytes", ErrDiagnosticCodecLimit)
	}
	return encoded, nil
}

func DecodeDiagnosticSet(encoded []byte, limits DiagnosticCodecLimits) (DiagnosticSet, error) {
	limits, err := normalizeDiagnosticCodecLimits(limits)
	if err != nil {
		return DiagnosticSet{}, err
	}
	if uint64(len(encoded)) > limits.MaxBytes {
		return DiagnosticSet{}, fmt.Errorf("%w: encoded bytes", ErrDiagnosticCodecLimit)
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var set DiagnosticSet
	if err := decoder.Decode(&set); err != nil {
		return DiagnosticSet{}, fmt.Errorf("layoutengine: decode diagnostics: %w", err)
	}
	if err := requireDiagnosticJSONEOF(decoder); err != nil {
		return DiagnosticSet{}, err
	}
	if set.SchemaVersion != DiagnosticSchemaVersion {
		return DiagnosticSet{}, fmt.Errorf("layoutengine: unsupported diagnostic schema version %d", set.SchemaVersion)
	}
	if uint64(len(set.Diagnostics)) > uint64(limits.MaxDiagnostics) {
		return DiagnosticSet{}, fmt.Errorf("%w: diagnostic count", ErrDiagnosticCodecLimit)
	}
	for index, diagnostic := range set.Diagnostics {
		if err := diagnostic.Validate(); err != nil {
			return DiagnosticSet{}, fmt.Errorf("layoutengine: diagnostics[%d]: %w", index, err)
		}
		set.Diagnostics[index] = cloneDiagnostic(diagnostic)
	}
	return set, nil
}

func normalizeDiagnosticCodecLimits(limits DiagnosticCodecLimits) (DiagnosticCodecLimits, error) {
	if limits == (DiagnosticCodecLimits{}) {
		return DefaultDiagnosticCodecLimits(), nil
	}
	defaults := DefaultDiagnosticCodecLimits()
	if limits.MaxDiagnostics == 0 || limits.MaxBytes == 0 ||
		limits.MaxDiagnostics > defaults.MaxDiagnostics || limits.MaxBytes > defaults.MaxBytes {
		return DiagnosticCodecLimits{}, errors.New("layoutengine: invalid diagnostic codec limits")
	}
	return limits, nil
}

func requireDiagnosticJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return errors.New("layoutengine: diagnostic JSON has trailing value")
		}
		return fmt.Errorf("layoutengine: diagnostic JSON trailing data: %w", err)
	}
	return nil
}
