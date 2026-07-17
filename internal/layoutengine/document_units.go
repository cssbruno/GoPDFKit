// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"errors"
	"fmt"
)

// DocumentUnit names the four geometry units accepted by the existing public
// Document constructor. Planner coordinates remain PDF points internally.
type DocumentUnit string

const (
	DocumentUnitPoint      DocumentUnit = "pt"
	DocumentUnitMillimeter DocumentUnit = "mm"
	DocumentUnitCentimeter DocumentUnit = "cm"
	DocumentUnitInch       DocumentUnit = "inch"
)

var ErrDocumentUnitInvalid = errors.New("layoutengine: document unit is invalid")

// FixedFromDocumentUnits converts existing Document geometry into canonical
// 1/1024-point coordinates. Metric conversions use the exact definitions
// 1 inch = 25.4 mm and 1 point = 1/72 inch:
//
//	pt: 1 point
//	mm: 360/127 points
//	cm: 3600/127 points
//	in: 72 points
//
// The final fixed conversion retains the engine-wide nearest-unit,
// half-away-from-zero rounding rule.
func FixedFromDocumentUnits(value float64, unit DocumentUnit) (Fixed, error) {
	var numerator, denominator float64
	switch unit {
	case DocumentUnitPoint:
		numerator, denominator = 1, 1
	case DocumentUnitMillimeter:
		numerator, denominator = 360, 127
	case DocumentUnitCentimeter:
		numerator, denominator = 3600, 127
	case DocumentUnitInch:
		numerator, denominator = 72, 1
	default:
		return 0, fmt.Errorf("%w: %q", ErrDocumentUnitInvalid, unit)
	}
	return FixedFromPoints(value * numerator / denominator)
}
