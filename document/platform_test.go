// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"runtime"
	"testing"
)

// requireDarwinRasterBaseline keeps Apple M2 pixel goldens from being
// compared against a different OS/font rasterization stack. Linux release
// validation still exercises the underlying planning and PDF contracts; the
// named Apple M2 raster baseline is validated on its calibrated platform.
func requireDarwinRasterBaseline(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "darwin" {
		t.Skipf("Apple M2 raster baseline is not applicable on %s", runtime.GOOS)
	}
}
