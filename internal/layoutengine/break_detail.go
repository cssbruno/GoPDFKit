// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const BreakDetailSchemaVersion uint16 = 1

var (
	ErrBreakDetailLimit   = errors.New("layoutengine: detailed break limit exceeded")
	ErrBreakDetailInvalid = errors.New("layoutengine: detailed break record is invalid")
)

type BreakTraceKind string

const (
	BreakTraceAvailable  BreakTraceKind = "available_capacity"
	BreakTraceRequired   BreakTraceKind = "triggering_requirement"
	BreakTraceTransition BreakTraceKind = "selected_transition"
)

func (kind BreakTraceKind) valid() bool {
	return kind == BreakTraceAvailable || kind == BreakTraceRequired || kind == BreakTraceTransition
}

// BreakTraceStep is one compact causal fact. Value is a fixed height for the
// capacity/requirement steps and zero for the selected transition.
type BreakTraceStep struct {
	Kind     BreakTraceKind `json:"kind"`
	Fragment FragmentID     `json:"fragment"`
	Page     uint32         `json:"page"`
	Region   RegionID       `json:"region"`
	Value    Fixed          `json:"value"`
}

// BreakDetail is an optional bounded expansion of one always-retained concise
// BreakDecision. BreakIndex is zero-based in canonical plan order.
type BreakDetail struct {
	BreakIndex uint32           `json:"break_index"`
	Reason     BreakReason      `json:"reason"`
	Steps      []BreakTraceStep `json:"steps"`
}

type BreakDetailLimits struct {
	MaxBreaks uint32
	MaxSteps  uint32
	MaxWork   uint64
	MaxBytes  uint64
}

func DefaultBreakDetailLimits() BreakDetailLimits {
	return BreakDetailLimits{MaxBreaks: 1 << 16, MaxSteps: 8, MaxWork: 1 << 20, MaxBytes: 16 << 20}
}

func normalizeBreakDetailLimits(limits BreakDetailLimits) (BreakDetailLimits, error) {
	if limits == (BreakDetailLimits{}) {
		return DefaultBreakDetailLimits(), nil
	}
	defaults := DefaultBreakDetailLimits()
	if limits.MaxBreaks == 0 || limits.MaxSteps < 3 || limits.MaxWork == 0 || limits.MaxBytes == 0 ||
		limits.MaxBreaks > defaults.MaxBreaks || limits.MaxSteps > defaults.MaxSteps ||
		limits.MaxWork > defaults.MaxWork || limits.MaxBytes > defaults.MaxBytes {
		return BreakDetailLimits{}, ErrBreakDetailLimit
	}
	return limits, nil
}

// DetailedBreaksContext explicitly materializes bounded causal traces. Normal
// plan projection, hashing, persistence, and concise structural queries never
// pay this cost or retain these optional records.
func (p LayoutPlan) DetailedBreaksContext(ctx context.Context, limits BreakDetailLimits) ([]BreakDetail, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	limits, err := normalizeBreakDetailLimits(limits)
	if err != nil {
		return nil, err
	}
	if uint64(len(p.breaks)) > uint64(limits.MaxBreaks) {
		return nil, ErrBreakDetailLimit
	}
	result := make([]BreakDetail, 0, len(p.breaks))
	work := uint64(0)
	for index, decision := range p.breaks {
		if index&63 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, newPlanningError(err, Diagnostic{Code: DiagnosticCanceled, Severity: SeverityError,
					Stage: StageLayout, Message: "detailed break trace generation was canceled"})
			}
		}
		work += 4
		if work > limits.MaxWork {
			return nil, ErrBreakDetailLimit
		}
		result = append(result, detailForBreak(uint32(index), decision))
	}
	return result, nil
}

func detailForBreak(index uint32, decision BreakDecision) BreakDetail {
	return BreakDetail{BreakIndex: index, Reason: decision.Reason, Steps: []BreakTraceStep{
		{Kind: BreakTraceAvailable, Fragment: decision.Preceding, Page: decision.FromPage, Region: decision.Region, Value: decision.Available},
		{Kind: BreakTraceRequired, Fragment: decision.Triggering, Page: decision.FromPage, Region: decision.Region, Value: decision.Required},
		{Kind: BreakTraceTransition, Fragment: decision.Triggering, Page: decision.ToPage, Region: decision.Region},
	}}
}

type DetailedLayoutPlanProjection struct {
	Plan         LayoutPlanProjection `json:"plan"`
	BreakDetails []BreakDetail        `json:"break_details,omitempty"`
}

func (p LayoutPlan) DetailedProjectionContext(ctx context.Context, limits BreakDetailLimits) (DetailedLayoutPlanProjection, error) {
	details, err := p.DetailedBreaksContext(ctx, limits)
	if err != nil {
		return DetailedLayoutPlanProjection{}, err
	}
	return DetailedLayoutPlanProjection{Plan: p.Projection(), BreakDetails: details}, nil
}

type BreakDetailSet struct {
	SchemaVersion uint16        `json:"schema_version"`
	Details       []BreakDetail `json:"details"`
}

func EncodeBreakDetailSet(ctx context.Context, details []BreakDetail, limits BreakDetailLimits) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	limits, err := normalizeBreakDetailLimits(limits)
	if err != nil {
		return nil, err
	}
	if uint64(len(details)) > uint64(limits.MaxBreaks) {
		return nil, ErrBreakDetailLimit
	}
	set := BreakDetailSet{SchemaVersion: BreakDetailSchemaVersion, Details: make([]BreakDetail, len(details))}
	work := uint64(0)
	for index, detail := range details {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		work += uint64(len(detail.Steps) + 1)
		if work > limits.MaxWork || uint32(len(detail.Steps)) > limits.MaxSteps {
			return nil, ErrBreakDetailLimit
		}
		if err := validateBreakDetail(detail); err != nil {
			return nil, fmt.Errorf("details[%d]: %w", index, err)
		}
		if index > 0 && details[index-1].BreakIndex >= detail.BreakIndex {
			return nil, ErrBreakDetailInvalid
		}
		set.Details[index] = cloneBreakDetail(detail)
	}
	encoded, err := json.Marshal(set)
	if err != nil {
		return nil, err
	}
	if uint64(len(encoded)) > limits.MaxBytes {
		return nil, ErrBreakDetailLimit
	}
	return encoded, nil
}

func DecodeBreakDetailSet(ctx context.Context, encoded []byte, limits BreakDetailLimits) (BreakDetailSet, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	limits, err := normalizeBreakDetailLimits(limits)
	if err != nil {
		return BreakDetailSet{}, err
	}
	if uint64(len(encoded)) > limits.MaxBytes {
		return BreakDetailSet{}, ErrBreakDetailLimit
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var set BreakDetailSet
	if err := decoder.Decode(&set); err != nil {
		return BreakDetailSet{}, err
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		return BreakDetailSet{}, ErrBreakDetailInvalid
	}
	if set.SchemaVersion != BreakDetailSchemaVersion || uint64(len(set.Details)) > uint64(limits.MaxBreaks) {
		return BreakDetailSet{}, ErrBreakDetailInvalid
	}
	work := uint64(0)
	for index, detail := range set.Details {
		if err := ctx.Err(); err != nil {
			return BreakDetailSet{}, err
		}
		work += uint64(len(detail.Steps) + 1)
		if work > limits.MaxWork || uint32(len(detail.Steps)) > limits.MaxSteps {
			return BreakDetailSet{}, ErrBreakDetailLimit
		}
		if err := validateBreakDetail(detail); err != nil {
			return BreakDetailSet{}, fmt.Errorf("details[%d]: %w", index, err)
		}
		if index > 0 && set.Details[index-1].BreakIndex >= detail.BreakIndex {
			return BreakDetailSet{}, ErrBreakDetailInvalid
		}
		set.Details[index] = cloneBreakDetail(detail)
	}
	return set, nil
}

func validateBreakDetail(detail BreakDetail) error {
	if !detail.Reason.valid() || len(detail.Steps) != 3 {
		return ErrBreakDetailInvalid
	}
	want := []BreakTraceKind{BreakTraceAvailable, BreakTraceRequired, BreakTraceTransition}
	for index, step := range detail.Steps {
		if !step.Kind.valid() || step.Kind != want[index] || !step.Fragment.Valid() || step.Page == 0 || !step.Region.Valid() || step.Value < 0 {
			return ErrBreakDetailInvalid
		}
	}
	if detail.Steps[2].Value != 0 {
		return ErrBreakDetailInvalid
	}
	return nil
}

func cloneBreakDetail(detail BreakDetail) BreakDetail {
	detail.Steps = cloneSlice(detail.Steps)
	return detail
}
