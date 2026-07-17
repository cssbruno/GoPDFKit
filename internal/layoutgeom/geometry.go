// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

// Package layoutgeom contains renderer-independent geometry used by both the
// public layout model and concrete renderers.
package layoutgeom

// TrackConstraint describes one row or column extent.
type TrackConstraint struct {
	Preferred float64
	Min       float64
	Max       float64
}

// ExceedsAvailableHeight reports whether content needs more vertical space
// than remains. Equality fits.
func ExceedsAvailableHeight(contentHeight, availableHeight float64) bool {
	return contentHeight > availableHeight
}

// ResolveTracks distributes total space across count tracks while respecting
// preferred, minimum, and maximum sizes. Impossible constraints are preserved
// instead of producing negative track sizes.
func ResolveTracks(total float64, count int, constraints []TrackConstraint) []float64 {
	if count <= 0 {
		return nil
	}
	if total < 0 {
		total = 0
	}
	widths := make([]float64, count)
	flexible := make([]bool, count)
	remaining := total
	flexibleCount := 0
	for i := range count {
		constraint := constraintAt(constraints, i)
		if constraint.Preferred > 0 {
			widths[i] = clamp(constraint.Preferred, constraint.Min, constraint.Max)
			remaining -= widths[i]
			continue
		}
		flexible[i] = true
		flexibleCount++
	}

	for flexibleCount > 0 {
		share := remaining / float64(flexibleCount)
		changed := false
		for i := range count {
			if !flexible[i] {
				continue
			}
			constraint := constraintAt(constraints, i)
			resolved := clamp(share, constraint.Min, constraint.Max)
			if resolved == share {
				continue
			}
			widths[i] = resolved
			remaining -= resolved
			flexible[i] = false
			flexibleCount--
			changed = true
		}
		if !changed {
			for i := range count {
				if flexible[i] {
					widths[i] = max(share, 0)
				}
			}
			break
		}
	}
	return widths
}

func constraintAt(constraints []TrackConstraint, index int) TrackConstraint {
	if index < 0 || index >= len(constraints) {
		return TrackConstraint{}
	}
	return constraints[index]
}

func clamp(value, minimum, maximum float64) float64 {
	if minimum > 0 && value < minimum {
		value = minimum
	}
	if maximum > 0 && value > maximum {
		value = maximum
	}
	return max(value, 0)
}

// TrackOffsets returns cumulative offsets for row or column sizes.
func TrackOffsets(sizes []float64) []float64 {
	offsets := make([]float64, len(sizes)+1)
	for i, size := range sizes {
		offsets[i+1] = offsets[i] + size
	}
	return offsets
}

// SpanSize returns the extent of span tracks starting at start.
func SpanSize(offsets []float64, start, span int) float64 {
	if span <= 0 || start < 0 || start >= len(offsets)-1 {
		return 0
	}
	end := min(start+span, len(offsets)-1)
	return offsets[end] - offsets[start]
}

// SumSpan returns the sum of span values starting at start.
func SumSpan(values []float64, start, span int) float64 {
	if span <= 0 || start < 0 || start >= len(values) {
		return 0
	}
	end := min(start+span, len(values))
	total := 0.0
	for _, value := range values[start:end] {
		total += value
	}
	return total
}
