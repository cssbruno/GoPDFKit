// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"hash"
)

var (
	ErrFlowBreakToken  = errors.New("layoutengine: vertical flow break token is invalid")
	ErrFlowResumeLimit = errors.New("layoutengine: vertical flow resume block limit is invalid")
)

// VerticalFlowBreakToken is an opaque immutable continuation owned by one
// exact VerticalFlowInput and normalized limit policy. It contains no caller-
// mutable planner state and cannot be serialized as a public interchange
// format. Tokens are deterministic and may be copied safely.
type VerticalFlowBreakToken struct {
	owner    [sha256.Size]byte
	valid    bool
	complete bool
	next     uint32
	workUsed uint64
	state    verticalFlowResumeState
}

type verticalFlowResumeState struct {
	input                     LayoutPlanInput
	pageNumber                uint32
	pageFragmentStart         int
	pageCommandStart          int
	cursor                    Fixed
	hasConsumedPositiveHeight bool
	lastPositiveFragment      FragmentID
	lastPositiveOverflow      bool
}

// VerticalFlowResumeResult is one bounded advance. Plan is a valid cumulative
// snapshot including the currently open page. Done means Next is a completed
// token and the plan is final. Consumed is the number of source blocks advanced
// by this call.
type VerticalFlowResumeResult struct {
	Plan     LayoutPlan
	Next     VerticalFlowBreakToken
	Done     bool
	Consumed uint32
}

// StartVerticalFlow validates input and returns its first opaque break token.
func StartVerticalFlow(input VerticalFlowInput) (VerticalFlowBreakToken, error) {
	return StartVerticalFlowContext(context.Background(), input, VerticalFlowLimits{})
}

// StartVerticalFlowContext is the bounded, cancellation-aware token creation
// entry point. Validation and ownership hashing are completed before a token
// is returned.
func StartVerticalFlowContext(ctx context.Context, input VerticalFlowInput, limits VerticalFlowLimits) (VerticalFlowBreakToken, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	limits, err := normalizeVerticalFlowLimits(limits)
	if err != nil {
		return VerticalFlowBreakToken{}, err
	}
	budget := verticalFlowBudget{ctx: ctx, limit: limits.MaxWork}
	if err := budget.charge(1); err != nil {
		return VerticalFlowBreakToken{}, err
	}
	if err := validateVerticalFlowInputBounded(input, limits, &budget); err != nil {
		return VerticalFlowBreakToken{}, err
	}
	owner, err := verticalFlowOwnership(ctx, input, limits)
	if err != nil {
		return VerticalFlowBreakToken{}, err
	}
	paginator := verticalFlowPaginator{
		pageSize: input.PageSize, body: input.Body,
		input:    LayoutPlanInput{Fragments: make([]Fragment, 0, len(input.Blocks))},
		maxPages: limits.MaxPages, budget: &budget,
	}
	if err := paginator.startPage(); err != nil {
		return VerticalFlowBreakToken{}, err
	}
	return tokenFromPaginator(owner, 0, false, &paginator), nil
}

// ResumeVerticalFlow advances at most maxBlocks source blocks using default
// limits. The supplied token must come from StartVerticalFlow or a prior result
// for the exact same input.
func ResumeVerticalFlow(input VerticalFlowInput, token VerticalFlowBreakToken, maxBlocks uint32) (VerticalFlowResumeResult, error) {
	return ResumeVerticalFlowContext(context.Background(), input, token, maxBlocks, VerticalFlowLimits{})
}

// ResumeVerticalFlowContext advances an opaque token under the exact limit
// policy that created it. Cancellation or failure leaves the supplied token
// reusable because all continuation state is cloned before mutation.
func ResumeVerticalFlowContext(ctx context.Context, input VerticalFlowInput, token VerticalFlowBreakToken, maxBlocks uint32, limits VerticalFlowLimits) (VerticalFlowResumeResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	limits, err := normalizeVerticalFlowLimits(limits)
	if err != nil {
		return VerticalFlowResumeResult{}, err
	}
	if maxBlocks == 0 || maxBlocks > limits.MaxBlocks {
		return VerticalFlowResumeResult{}, ErrFlowResumeLimit
	}
	validation := verticalFlowBudget{ctx: ctx, limit: limits.MaxWork}
	if err := validation.charge(1); err != nil {
		return VerticalFlowResumeResult{}, err
	}
	if err := validateVerticalFlowInputBounded(input, limits, &validation); err != nil {
		return VerticalFlowResumeResult{}, err
	}
	owner, err := verticalFlowOwnership(ctx, input, limits)
	if err != nil {
		return VerticalFlowResumeResult{}, err
	}
	if err := validateVerticalFlowToken(token, owner, input, limits); err != nil {
		return VerticalFlowResumeResult{}, err
	}
	budget := verticalFlowBudget{ctx: ctx, limit: limits.MaxWork, used: token.workUsed}
	paginator := paginatorFromToken(input, limits, &budget, token)
	end := uint64(token.next) + uint64(maxBlocks)
	if end > uint64(len(input.Blocks)) {
		end = uint64(len(input.Blocks))
	}
	for index := token.next; uint64(index) < end; index++ {
		if err := budget.charge(1); err != nil {
			return VerticalFlowResumeResult{}, err
		}
		block := input.Blocks[index]
		remaining, err := paginator.remainingHeight()
		if err != nil {
			return VerticalFlowResumeResult{}, err
		}
		var decision *BreakDecision
		if block.Height > 0 && block.Height > remaining && paginator.hasConsumedPositiveHeight {
			decision, err = paginator.advanceForBlock(FragmentID(index+1), block.Height, remaining)
			if err != nil {
				return VerticalFlowResumeResult{}, err
			}
		}
		if err := paginator.place(block, FragmentID(index+1)); err != nil {
			return VerticalFlowResumeResult{}, err
		}
		if decision != nil {
			paginator.input.Breaks = append(paginator.input.Breaks, *decision)
		}
	}
	nextBlock := uint32(end)
	done := nextBlock == uint32(len(input.Blocks))
	next := tokenFromPaginator(owner, nextBlock, done, &paginator)
	snapshot := cloneVerticalFlowPlanInput(paginator.input)
	snapshot.Pages = append(snapshot.Pages, PlannedPage{
		Number: paginator.pageNumber, Size: paginator.pageSize,
		Fragments: IndexRange{Start: uint32(paginator.pageFragmentStart), Count: uint32(len(snapshot.Fragments) - paginator.pageFragmentStart)},
		Commands:  IndexRange{Start: uint32(paginator.pageCommandStart), Count: uint32(len(snapshot.Commands) - paginator.pageCommandStart)},
	})
	plan, err := NewLayoutPlan(snapshot)
	if err != nil {
		return VerticalFlowResumeResult{}, err
	}
	return VerticalFlowResumeResult{Plan: plan, Next: next, Done: done, Consumed: nextBlock - token.next}, nil
}

func tokenFromPaginator(owner [sha256.Size]byte, next uint32, complete bool, paginator *verticalFlowPaginator) VerticalFlowBreakToken {
	return VerticalFlowBreakToken{
		owner: owner, valid: true, complete: complete, next: next, workUsed: paginator.budget.used,
		state: verticalFlowResumeState{
			input: cloneVerticalFlowPlanInput(paginator.input), pageNumber: paginator.pageNumber,
			pageFragmentStart: paginator.pageFragmentStart, pageCommandStart: paginator.pageCommandStart,
			cursor: paginator.cursor, hasConsumedPositiveHeight: paginator.hasConsumedPositiveHeight,
			lastPositiveFragment: paginator.lastPositiveFragment, lastPositiveOverflow: paginator.lastPositiveOverflow,
		},
	}
}

func paginatorFromToken(input VerticalFlowInput, limits VerticalFlowLimits, budget *verticalFlowBudget, token VerticalFlowBreakToken) verticalFlowPaginator {
	return verticalFlowPaginator{
		pageSize: input.PageSize, body: input.Body, input: cloneVerticalFlowPlanInput(token.state.input),
		maxPages: limits.MaxPages, budget: budget, pageNumber: token.state.pageNumber,
		pageFragmentStart: token.state.pageFragmentStart, pageCommandStart: token.state.pageCommandStart,
		cursor: token.state.cursor, hasConsumedPositiveHeight: token.state.hasConsumedPositiveHeight,
		lastPositiveFragment: token.state.lastPositiveFragment, lastPositiveOverflow: token.state.lastPositiveOverflow,
	}
}

func validateVerticalFlowToken(token VerticalFlowBreakToken, owner [sha256.Size]byte, input VerticalFlowInput, limits VerticalFlowLimits) error {
	if !token.valid || token.complete || token.owner != owner || token.next > uint32(len(input.Blocks)) ||
		token.workUsed > limits.MaxWork || token.state.pageNumber == 0 || token.state.pageNumber > limits.MaxPages ||
		token.state.pageFragmentStart < 0 || token.state.pageFragmentStart > len(token.state.input.Fragments) ||
		token.state.pageCommandStart < 0 || token.state.pageCommandStart > len(token.state.input.Commands) ||
		len(token.state.input.Fragments) != int(token.next) {
		return ErrFlowBreakToken
	}
	if token.state.cursor < input.Body.Y || !token.state.hasConsumedPositiveHeight && token.state.lastPositiveFragment.Valid() ||
		token.state.hasConsumedPositiveHeight && !token.state.lastPositiveFragment.Valid() {
		return ErrFlowBreakToken
	}
	return nil
}

func cloneVerticalFlowPlanInput(input LayoutPlanInput) LayoutPlanInput {
	result := LayoutPlanInput{
		Pages: cloneSlice(input.Pages), Fragments: cloneSlice(input.Fragments), Commands: cloneSlice(input.Commands),
		Breaks: cloneSlice(input.Breaks),
	}
	if len(input.Diagnostics) != 0 {
		result.Diagnostics = make([]Diagnostic, len(input.Diagnostics))
		for index, diagnostic := range input.Diagnostics {
			result.Diagnostics[index] = cloneDiagnostic(diagnostic)
		}
	}
	return result
}

func verticalFlowOwnership(ctx context.Context, input VerticalFlowInput, limits VerticalFlowLimits) ([sha256.Size]byte, error) {
	digest := sha256.New()
	writeFlowInt64(digest, int64(input.PageSize.Width))
	writeFlowInt64(digest, int64(input.PageSize.Height))
	for _, value := range []Fixed{input.Body.X, input.Body.Y, input.Body.Width, input.Body.Height} {
		writeFlowInt64(digest, int64(value))
	}
	writeFlowUint32(digest, limits.MaxBlocks)
	writeFlowUint32(digest, limits.MaxPages)
	writeFlowUint64(digest, limits.MaxWork)
	writeFlowUint64(digest, uint64(len(input.Blocks)))
	for index, block := range input.Blocks {
		if index&63 == 0 {
			if err := ctx.Err(); err != nil {
				return [sha256.Size]byte{}, newPlanningError(err, Diagnostic{Code: DiagnosticCanceled, Severity: SeverityError,
					Stage: StageLayout, Message: "vertical flow token ownership validation was canceled"})
			}
		}
		writeFlowUint32(digest, uint32(block.Node))
		writeFlowString(digest, string(block.Key))
		writeFlowString(digest, string(block.Instance))
		writeFlowString(digest, block.Source.File)
		writeFlowPosition(digest, block.Source.Start)
		writeFlowPosition(digest, block.Source.End)
		writeFlowInt64(digest, int64(block.Height))
		if block.Repeated {
			writeFlowUint32(digest, 1)
		} else {
			writeFlowUint32(digest, 0)
		}
	}
	var owner [sha256.Size]byte
	copy(owner[:], digest.Sum(nil))
	return owner, nil
}

func writeFlowPosition(digest hash.Hash, value SourcePosition) {
	writeFlowUint64(digest, value.Offset)
	writeFlowUint32(digest, value.Line)
	writeFlowUint32(digest, value.Column)
}

func writeFlowString(digest hash.Hash, value string) {
	writeFlowUint64(digest, uint64(len(value)))
	_, _ = digest.Write([]byte(value))
}

func writeFlowInt64(digest hash.Hash, value int64) {
	_ = binary.Write(digest, binary.LittleEndian, value)
}
func writeFlowUint64(digest hash.Hash, value uint64) {
	_ = binary.Write(digest, binary.LittleEndian, value)
}
func writeFlowUint32(digest hash.Hash, value uint32) {
	_ = binary.Write(digest, binary.LittleEndian, value)
}
