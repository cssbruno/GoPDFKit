// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"errors"
	"math"
	"testing"
)

func TestFixedFromDocumentUnitsUsesExactPublishedScales(t *testing.T) {
	tests := []struct {
		value float64
		unit  DocumentUnit
		want  Fixed
	}{
		{72, DocumentUnitPoint, 72 * Fixed(FixedScale)},
		{25.4, DocumentUnitMillimeter, 72 * Fixed(FixedScale)},
		{2.54, DocumentUnitCentimeter, 72 * Fixed(FixedScale)},
		{1, DocumentUnitInch, 72 * Fixed(FixedScale)},
		{-1, DocumentUnitPoint, -Fixed(FixedScale)},
	}
	for _, test := range tests {
		got, err := FixedFromDocumentUnits(test.value, test.unit)
		if err != nil || got != test.want {
			t.Fatalf("FixedFromDocumentUnits(%g, %q) = %d, %v; want %d", test.value, test.unit, got, err, test.want)
		}
	}
}

func TestFixedFromDocumentUnitsRejectsInvalidAndNonFiniteInputs(t *testing.T) {
	if _, err := FixedFromDocumentUnits(1, "px"); !errors.Is(err, ErrDocumentUnitInvalid) {
		t.Fatalf("invalid unit error = %v", err)
	}
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if _, err := FixedFromDocumentUnits(value, DocumentUnitMillimeter); !errors.Is(err, ErrNonFiniteCoordinate) {
			t.Fatalf("non-finite %v error = %v", value, err)
		}
	}
}
