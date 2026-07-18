// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import "math"

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) <= 1e-9
}
