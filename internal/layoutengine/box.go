// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"context"
	"errors"
	"fmt"
)

var (
	ErrBoxNegativeEdge       = errors.New("layoutengine: box edge is negative")
	ErrBoxNegativeContent    = errors.New("layoutengine: box content height is negative")
	ErrBoxHorizontalOverflow = errors.New("layoutengine: box horizontal edges exceed the body width")
)

// BoxFlowStyle is the initial fixed-point box model. Every edge is explicit;
// percentages, auto values, margin collapsing, and negative margins belong to
// later syntax-specific lowering rather than this canonical planner boundary.
type BoxFlowStyle struct {
	Margin  Insets
	Border  Insets
	Padding Insets
}

// BoxFlowBlock is one already-measured, indivisible content box. Pagination
// uses its complete outer height, while the resulting Fragment records exact
// border and content boxes and leaves margin represented by placement gaps.
type BoxFlowBlock struct {
	Node          NodeID
	Key           NodeKey
	Instance      InstanceID
	Source        SourceSpan
	ContentHeight Fixed
	Style         BoxFlowStyle
}

type BoxFlowInput struct {
	PageSize Size
	Body     Rect
	Blocks   []BoxFlowBlock
}

// PlanBoxFlow resolves fixed box geometry and delegates only indivisible
// pagination to the vertical-flow kernel. It performs no text measurement or
// painting. Margins participate in break decisions but never inflate the
// fragment's BorderBox.
func PlanBoxFlow(input BoxFlowInput) (LayoutPlan, error) {
	return PlanBoxFlowContext(context.Background(), input, VerticalFlowLimits{})
}

// PlanBoxFlowContext bounds box count and delegated pagination work while
// honoring cancellation during the linear box-resolution and projection
// passes. The zero limits value selects DefaultVerticalFlowLimits.
func PlanBoxFlowContext(ctx context.Context, input BoxFlowInput, limits VerticalFlowLimits) (LayoutPlan, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	limits, err := normalizeVerticalFlowLimits(limits)
	if err != nil {
		return LayoutPlan{}, err
	}
	if uint64(len(input.Blocks)) > uint64(limits.MaxBlocks) {
		return LayoutPlan{}, newPlanningError(ErrFlowStateLimit, Diagnostic{Code: DiagnosticResourceLimit,
			Severity: SeverityError, Stage: StageLayout, Message: "box flow block count exceeds its state limit"})
	}
	if err := validateVerticalFlowInput(VerticalFlowInput{PageSize: input.PageSize, Body: input.Body}); err != nil {
		return LayoutPlan{}, err
	}
	resolved := make([]resolvedBoxFlowBlock, len(input.Blocks))
	flow := VerticalFlowInput{PageSize: input.PageSize, Body: input.Body, Blocks: make([]VerticalFlowBlock, len(input.Blocks))}
	for index, block := range input.Blocks {
		if index&63 == 0 {
			if err := ctx.Err(); err != nil {
				return LayoutPlan{}, newPlanningError(err, Diagnostic{Code: DiagnosticCanceled, Severity: SeverityError,
					Stage: StageLayout, Message: "box flow planning was canceled"})
			}
		}
		item, err := resolveBoxFlowBlock(input.Body.Width, block)
		if err != nil {
			return LayoutPlan{}, fmt.Errorf("layoutengine: box flow block %d: %w", index, err)
		}
		resolved[index] = item
		flow.Blocks[index] = VerticalFlowBlock{
			Node: block.Node, Key: block.Key, Instance: block.Instance, Source: block.Source, Height: item.outerHeight,
		}
	}
	geometry, err := PlanVerticalFlowContext(ctx, flow, limits)
	if err != nil {
		return LayoutPlan{}, err
	}
	projection := geometry.Projection()
	for index := range projection.Fragments {
		if index&63 == 0 {
			if err := ctx.Err(); err != nil {
				return LayoutPlan{}, newPlanningError(err, Diagnostic{Code: DiagnosticCanceled, Severity: SeverityError,
					Stage: StageLayout, Message: "box flow projection was canceled"})
			}
		}
		outer := projection.Fragments[index].BorderBox
		item := resolved[index]
		borderX, err := outer.X.Add(item.block.Style.Margin.Left)
		if err != nil {
			return LayoutPlan{}, fmt.Errorf("layoutengine: box flow block %d border x: %w", index, err)
		}
		borderY, err := outer.Y.Add(item.block.Style.Margin.Top)
		if err != nil {
			return LayoutPlan{}, fmt.Errorf("layoutengine: box flow block %d border y: %w", index, err)
		}
		borderBox, err := NewRect(borderX, borderY, item.borderWidth, item.borderHeight)
		if err != nil {
			return LayoutPlan{}, fmt.Errorf("layoutengine: box flow block %d border box: %w", index, err)
		}
		contentX, err := addFixed(borderX, item.block.Style.Border.Left, item.block.Style.Padding.Left)
		if err != nil {
			return LayoutPlan{}, fmt.Errorf("layoutengine: box flow block %d content x: %w", index, err)
		}
		contentY, err := addFixed(borderY, item.block.Style.Border.Top, item.block.Style.Padding.Top)
		if err != nil {
			return LayoutPlan{}, fmt.Errorf("layoutengine: box flow block %d content y: %w", index, err)
		}
		contentBox, err := NewRect(contentX, contentY, item.contentWidth, item.block.ContentHeight)
		if err != nil {
			return LayoutPlan{}, fmt.Errorf("layoutengine: box flow block %d content box: %w", index, err)
		}
		paddingBox, err := borderBox.Inset(item.block.Style.Border)
		if err != nil {
			return LayoutPlan{}, fmt.Errorf("layoutengine: box flow block %d padding box: %w", index, err)
		}
		projection.Fragments[index].MarginBox = outer
		projection.Fragments[index].BorderBox = borderBox
		projection.Fragments[index].PaddingBox = paddingBox
		projection.Fragments[index].ContentBox = contentBox
	}
	fragments := make(map[FragmentID]Fragment, len(projection.Fragments))
	for _, fragment := range projection.Fragments {
		fragments[fragment.ID] = fragment
	}
	for index := range projection.Diagnostics {
		diagnostic := &projection.Diagnostics[index]
		if diagnostic.Location.HasBounds && diagnostic.Location.Fragment.Valid() {
			diagnostic.Location.Bounds = fragments[diagnostic.Location.Fragment].BorderBox
		}
	}
	return NewLayoutPlan(LayoutPlanInput{
		DeterministicInputs: projection.DeterministicInputs,
		Pages:               projection.Pages, Fragments: projection.Fragments, Lines: projection.Lines,
		PageRegions: projection.PageRegions, GridTracks: projection.GridTracks,
		Fonts: projection.Fonts, GlyphRuns: projection.GlyphRuns,
		ImageResources: projection.ImageResources, Images: projection.Images,
		Commands: projection.Commands, Breaks: projection.Breaks, Diagnostics: projection.Diagnostics,
		SemanticNodes: projection.SemanticNodes, SemanticFragments: projection.SemanticFragments, ReadingOrder: projection.ReadingOrder,
	})
}

type resolvedBoxFlowBlock struct {
	block        BoxFlowBlock
	outerHeight  Fixed
	borderWidth  Fixed
	borderHeight Fixed
	contentWidth Fixed
}

func resolveBoxFlowBlock(bodyWidth Fixed, block BoxFlowBlock) (resolvedBoxFlowBlock, error) {
	if err := validateBoxEdges(block.Style); err != nil {
		return resolvedBoxFlowBlock{}, err
	}
	if block.ContentHeight < 0 {
		return resolvedBoxFlowBlock{}, ErrBoxNegativeContent
	}
	outerHorizontal, err := addFixed(
		block.Style.Margin.Left, block.Style.Margin.Right,
		block.Style.Border.Left, block.Style.Border.Right,
		block.Style.Padding.Left, block.Style.Padding.Right,
	)
	if err != nil {
		return resolvedBoxFlowBlock{}, err
	}
	contentWidth, err := bodyWidth.Sub(outerHorizontal)
	if err != nil {
		return resolvedBoxFlowBlock{}, err
	}
	if contentWidth < 0 {
		return resolvedBoxFlowBlock{}, ErrBoxHorizontalOverflow
	}
	borderWidth, err := bodyWidth.Sub(block.Style.Margin.Left)
	if err != nil {
		return resolvedBoxFlowBlock{}, err
	}
	borderWidth, err = borderWidth.Sub(block.Style.Margin.Right)
	if err != nil {
		return resolvedBoxFlowBlock{}, err
	}
	borderVertical, err := addFixed(
		block.Style.Border.Top, block.Style.Border.Bottom,
		block.Style.Padding.Top, block.Style.Padding.Bottom,
		block.ContentHeight,
	)
	if err != nil {
		return resolvedBoxFlowBlock{}, err
	}
	outerHeight, err := addFixed(block.Style.Margin.Top, borderVertical, block.Style.Margin.Bottom)
	if err != nil {
		return resolvedBoxFlowBlock{}, err
	}
	return resolvedBoxFlowBlock{
		block: block, outerHeight: outerHeight, borderWidth: borderWidth,
		borderHeight: borderVertical, contentWidth: contentWidth,
	}, nil
}

func validateBoxEdges(style BoxFlowStyle) error {
	for _, value := range []Fixed{
		style.Margin.Top, style.Margin.Right, style.Margin.Bottom, style.Margin.Left,
		style.Border.Top, style.Border.Right, style.Border.Bottom, style.Border.Left,
		style.Padding.Top, style.Padding.Right, style.Padding.Bottom, style.Padding.Left,
	} {
		if value < 0 {
			return ErrBoxNegativeEdge
		}
	}
	return nil
}

func addFixed(values ...Fixed) (Fixed, error) {
	var total Fixed
	for _, value := range values {
		var err error
		total, err = total.Add(value)
		if err != nil {
			return 0, err
		}
	}
	return total, nil
}
