// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import "fmt"

type rowColumnLogicalLine struct {
	start      int
	count      int
	crossSize  Fixed
	mainSizes  []Fixed
	usedMain   Fixed
	crossStart Fixed
}

func planWrappedRowColumn(input RowColumnPlanInput, wrap RowColumnWrap, alignContent ContentAlignment, containerAlign CrossAlignment, justify MainAlignment, budget *rowColumnBudget) (RowColumnPlanResult, error) {
	mainAvailable, crossAvailable := rowColumnAxes(input.Direction, input.Region.Width, input.Region.Height)
	lines, err := formRowColumnLines(input, mainAvailable, budget)
	if err != nil {
		return RowColumnPlanResult{}, err
	}
	mainSizes := make([]Fixed, len(input.Children))
	var maxUsedMain Fixed
	for index := range lines {
		line := &lines[index]
		lineInput := input
		lineInput.Children = input.Children[line.start : line.start+line.count]
		line.mainSizes, line.usedMain, err = resolveRowColumnTracks(lineInput, mainAvailable, budget)
		if err != nil {
			return RowColumnPlanResult{}, err
		}
		copy(mainSizes[line.start:line.start+line.count], line.mainSizes)
		if line.usedMain > maxUsedMain {
			maxUsedMain = line.usedMain
		}
	}
	requiredCross, err := rowColumnLinesRequiredCross(lines, input.CrossGap)
	if err != nil {
		return RowColumnPlanResult{}, err
	}
	if requiredCross > crossAvailable {
		return RowColumnPlanResult{}, rowColumnOverflow(input, -1, "cross", requiredCross, crossAvailable)
	}
	if err := placeRowColumnLines(lines, input.CrossGap, crossAvailable, rowColumnCrossOrigin(input.Direction, input.Region), wrap, alignContent, requiredCross); err != nil {
		return RowColumnPlanResult{}, err
	}

	planInput := LayoutPlanInput{Pages: []PlannedPage{{
		Number: 1, Size: input.PageSize, Fragments: IndexRange{Count: uint32(len(input.Children))},
	}}}
	planInput.Fragments = make([]Fragment, 0, len(input.Children))
	if err := budget.charge(uint64(len(input.Children)) + uint64(len(lines)) + 1); err != nil {
		return RowColumnPlanResult{}, err
	}
	lineProjection := make([]RowColumnLine, len(lines))
	for lineIndex := range lines {
		line := &lines[lineIndex]
		mainOffset, distributedGaps, err := resolveMainAxisPlacement(justify, line.count, mainAvailable, line.usedMain)
		if err != nil {
			return RowColumnPlanResult{}, err
		}
		mainCursor, err := rowColumnMainOrigin(input.Direction, input.Region).Add(mainOffset)
		if err != nil {
			return RowColumnPlanResult{}, err
		}
		lineProjection[lineIndex] = RowColumnLine{
			Children:   IndexRange{Start: uint32(line.start), Count: uint32(line.count)},
			CrossStart: line.crossStart, CrossSize: line.crossSize, UsedMain: line.usedMain,
		}
		for localIndex := 0; localIndex < line.count; localIndex++ {
			if err := budget.charge(1); err != nil {
				return RowColumnPlanResult{}, err
			}
			childIndex := line.start + localIndex
			child := input.Children[childIndex]
			align := child.Align
			if align == "" {
				align = containerAlign
			}
			if !align.valid() {
				return RowColumnPlanResult{}, fmt.Errorf("%w: child %d", ErrRowColumnAlignment, childIndex)
			}
			crossPosition, crossExtent, err := resolveCrossAxis(child, align, line.crossStart, line.crossSize, input.Region, childIndex)
			if err != nil {
				return RowColumnPlanResult{}, err
			}
			mainPosition, err := rowColumnPhysicalMainPosition(input.Direction, input.Region, mainCursor, line.mainSizes[localIndex], input.ReverseMain)
			if err != nil {
				return RowColumnPlanResult{}, err
			}
			bounds, err := rowColumnRect(input.Direction, mainPosition, crossPosition, line.mainSizes[localIndex], crossExtent)
			if err != nil {
				return RowColumnPlanResult{}, fmt.Errorf("layoutengine: wrapped row or column child %d bounds: %w", childIndex, err)
			}
			planInput.Fragments = append(planInput.Fragments, Fragment{
				ID: FragmentID(childIndex + 1), Node: child.Node, Key: child.Key, Instance: child.Instance,
				Page: 1, Region: RegionBody, BorderBox: bounds, ContentBox: bounds,
				Source: child.Source, Continuation: ContinuationWhole,
			})
			mainCursor, err = mainCursor.Add(line.mainSizes[localIndex])
			if err != nil {
				return RowColumnPlanResult{}, err
			}
			if localIndex+1 < line.count {
				mainCursor, err = mainCursor.Add(input.Gap)
				if err == nil {
					mainCursor, err = mainCursor.Add(distributedGaps[localIndex])
				}
				if err != nil {
					return RowColumnPlanResult{}, err
				}
			}
		}
	}
	plan, err := NewLayoutPlan(planInput)
	if err != nil {
		return RowColumnPlanResult{}, err
	}
	return RowColumnPlanResult{Plan: plan, mainSizes: mainSizes, lines: lineProjection, UsedMain: maxUsedMain}, nil
}

// formRowColumnLines uses each track's fixed size or measured minimum as its
// wrapping basis. A flexible track is then resolved again with the existing
// per-line solver, so wrapping introduces no second sizing policy.
func formRowColumnLines(input RowColumnPlanInput, available Fixed, budget *rowColumnBudget) ([]rowColumnLogicalLine, error) {
	if len(input.Children) == 0 {
		return nil, nil
	}
	lines := make([]rowColumnLogicalLine, 0, len(input.Children))
	_, crossAvailable := rowColumnAxes(input.Direction, input.Region.Width, input.Region.Height)
	start := 0
	var used Fixed
	var cross Fixed
	for index, child := range input.Children {
		if err := budget.charge(1); err != nil {
			return nil, err
		}
		if err := validateRowColumnChild(child, index); err != nil {
			return nil, err
		}
		basis, err := rowColumnWrapBasis(child, index, available)
		if err != nil {
			return nil, err
		}
		next := basis
		if index > start {
			next, err = used.Add(input.Gap)
			if err == nil {
				next, err = next.Add(basis)
			}
			if err != nil {
				return nil, err
			}
		}
		crossExtent, crossErr := resolveRowColumnCrossExtent(child, crossAvailable, false)
		if crossErr != nil {
			return nil, crossErr
		}
		if index > start && next > available {
			lines = append(lines, rowColumnLogicalLine{start: start, count: index - start, crossSize: cross})
			start, used, cross = index, basis, crossExtent
			continue
		}
		used = next
		if crossExtent > cross {
			cross = crossExtent
		}
	}
	return append(lines, rowColumnLogicalLine{start: start, count: len(input.Children) - start, crossSize: cross}), nil
}

func rowColumnWrapBasis(child RowColumnChild, index int, available Fixed) (Fixed, error) {
	minimum, constraintErr := resolveRowColumnMinimum(child.Track, available)
	if constraintErr != nil {
		return 0, constraintErr
	}
	if child.MinMain > minimum {
		minimum = child.MinMain
	}
	switch child.Track.Kind {
	case RowColumnTrackFixed:
		if child.Track.Weight != 0 {
			return 0, fmt.Errorf("%w: child %d fixed track has a weight", ErrRowColumnTrack, index)
		}
		return child.Track.Size, nil
	case RowColumnTrackAuto:
		if child.Track.Weight != 0 || child.Track.Size != 0 {
			return 0, fmt.Errorf("%w: child %d auto track has fixed or weighted data", ErrRowColumnTrack, index)
		}
		return minimum, nil
	case RowColumnTrackFraction:
		if child.Track.Weight == 0 || child.Track.Size != 0 {
			return 0, fmt.Errorf("%w: child %d fractional track requires only a positive weight", ErrRowColumnTrack, index)
		}
		return minimum, nil
	case RowColumnTrackFlex:
		basis, err := resolveRowColumnFlexBasis(child.Track, available, child.ContentMain)
		if err != nil {
			return 0, fmt.Errorf("%w: child %d: %v", ErrRowColumnTrack, index, err)
		}
		maximum, maxErr := resolveRowColumnMaximum(child.Track, available)
		if maxErr != nil {
			return 0, maxErr
		}
		return clampRowColumnFlexSize(basis, minimum, maximum), nil
	default:
		return 0, fmt.Errorf("%w: child %d", ErrRowColumnTrack, index)
	}
}

func placeRowColumnLines(lines []rowColumnLogicalLine, gap, available, origin Fixed, wrap RowColumnWrap, alignment ContentAlignment, required Fixed) error {
	if len(lines) == 0 {
		return nil
	}
	free := available - required
	leading := Fixed(0)
	distributed := make([]Fixed, max(0, len(lines)-1))
	if alignment == ContentStretch && len(lines) != 0 {
		growth := distributeMainSpace(free, len(lines))
		for index := range lines {
			lines[index].crossSize += growth[index]
		}
		free = 0
	} else {
		mainAlignment, err := contentMainAlignment(alignment)
		if err != nil {
			return err
		}
		leading, distributed, err = resolveMainAxisPlacement(mainAlignment, len(lines), available, required)
		if err != nil {
			return err
		}
	}
	cursor := leading
	for index := range lines {
		logicalOffset := cursor
		physicalOffset := logicalOffset
		if wrap == RowColumnWrapReverse {
			physicalOffset = available - logicalOffset - lines[index].crossSize
		}
		position, err := origin.Add(physicalOffset)
		if err != nil {
			return err
		}
		lines[index].crossStart = position
		cursor += lines[index].crossSize
		if index+1 < len(lines) {
			cursor += gap + distributed[index]
		}
	}
	return nil
}

func rowColumnLinesRequiredCross(lines []rowColumnLogicalLine, gap Fixed) (Fixed, error) {
	var total Fixed
	for index := range lines {
		next, err := total.Add(lines[index].crossSize)
		if err != nil {
			return 0, err
		}
		total = next
	}
	if len(lines) > 1 {
		gapTotal, err := gap.MulInt(int64(len(lines) - 1))
		if err != nil {
			return 0, err
		}
		total, err = total.Add(gapTotal)
		if err != nil {
			return 0, err
		}
	}
	return total, nil
}

func contentMainAlignment(alignment ContentAlignment) (MainAlignment, error) {
	switch alignment {
	case ContentStart:
		return MainStart, nil
	case ContentCenter:
		return MainCenter, nil
	case ContentEnd:
		return MainEnd, nil
	case ContentSpaceBetween:
		return MainSpaceBetween, nil
	case ContentSpaceAround:
		return MainSpaceAround, nil
	case ContentSpaceEvenly:
		return MainSpaceEvenly, nil
	default:
		return "", ErrRowColumnContentAlign
	}
}

func (wrap RowColumnWrap) valid() bool {
	return wrap == RowColumnNoWrap || wrap == RowColumnWrapForward || wrap == RowColumnWrapReverse
}

func (alignment ContentAlignment) valid() bool {
	return alignment == ContentStart || alignment == ContentCenter || alignment == ContentEnd ||
		alignment == ContentSpaceBetween || alignment == ContentSpaceAround ||
		alignment == ContentSpaceEvenly || alignment == ContentStretch
}
