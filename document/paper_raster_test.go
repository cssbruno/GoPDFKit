// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"context"
	"testing"

	"golang.org/x/image/font/gofont/goregular"
)

func TestPaperPlanCaptureRasterPagesIsDetachedDeterministicAndPlanBound(t *testing.T) {
	plan, result, err := PlanPaper("raster.paper", paperPipelineFixture)
	if err != nil || !result.OK() {
		t.Fatalf("PlanPaper = %#v, %v", result, err)
	}
	request := DefaultPaperPlanRasterRequest()
	request.CoreFontProgram = goregular.TTF
	first, err := plan.CaptureRasterPages(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := plan.CaptureRasterPages(context.Background(), request)
	if err != nil || first.PlanHash != plan.Hash() || first.Renderer == "" || first.DPI != request.DPI || len(first.Pages) != plan.PageCount() || len(second.Pages) != len(first.Pages) {
		t.Fatalf("capture = %#v / %#v, %v", first, second, err)
	}
	for index := range first.Pages {
		left, right := first.Pages[index], second.Pages[index]
		if left.Page != uint32(index+1) || left.PNGSHA256 == "" || left.ManifestSHA256 == "" || len(left.PNG) == 0 || len(left.ManifestJSON) == 0 || !bytes.Equal(left.PNG, right.PNG) || !bytes.Equal(left.ManifestJSON, right.ManifestJSON) {
			t.Fatalf("page[%d] = %#v / %#v", index, left, right)
		}
	}
	first.Pages[0].PNG[0] ^= 0xff
	if bytes.Equal(first.Pages[0].PNG, second.Pages[0].PNG) {
		t.Fatal("captured page aliases another result")
	}
}
