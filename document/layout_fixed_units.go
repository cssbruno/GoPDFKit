// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import "github.com/cssbruno/gopdfkit/internal/layoutengine"

func fixedFromDocumentUnits(f *Document, value float64) (layoutengine.Fixed, error) {
	unit := layoutengine.DocumentUnit(f.unitStr)
	switch f.unitStr {
	case "point":
		unit = layoutengine.DocumentUnitPoint
	case "in":
		unit = layoutengine.DocumentUnitInch
	}
	return layoutengine.FixedFromDocumentUnits(value, unit)
}
