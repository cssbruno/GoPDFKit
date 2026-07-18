// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
)

type characterizationRasterPin struct {
	Name   string                          `json:"name"`
	Status string                          `json:"status"`
	Raster *CharacterizationRasterEvidence `json:"raster,omitempty"`
}

func TestCharacterizationRasterPagesArePinnedBoundedAndFailureAtomic(t *testing.T) {
	requireDarwinRasterBaseline(t)
	typed, err := RunTypedCharacterization(t.Context(), TypedCharacterizationLimits{})
	if err != nil {
		t.Fatal(err)
	}
	typedPins := make([]characterizationRasterPin, len(typed.Fixtures))
	var typedRasterPages uint32
	var typedRasterBytes uint64
	for index, fixture := range typed.Fixtures {
		typedPins[index] = characterizationRasterPin{Name: fixture.Name, Status: fixture.RasterStatus, Raster: fixture.Raster}
		planned := fixture.PlanHash != ""
		validateCharacterizationRasterFixture(t, fixture.Name, fixture.Pages, planned, fixture.RasterStatus, fixture.Raster)
		if !planned && fixture.Raster != nil {
			t.Fatalf("failed typed fixture %q fabricated raster evidence", fixture.Name)
		}
		if fixture.Raster != nil {
			typedRasterPages += uint32(len(fixture.Raster.Pages))
			for _, page := range fixture.Raster.Pages {
				typedRasterBytes += page.PNGBytes
			}
		}
	}
	if typedRasterPages != 41 || typedRasterPages > characterizationRasterMaxPages || typedRasterBytes == 0 || typedRasterBytes > characterizationRasterMaxPNGBytes {
		t.Fatalf("typed raster totals pages=%d bytes=%d", typedRasterPages, typedRasterBytes)
	}
	// Includes the documented local-canvas characterization fixture so the
	// public typed inventory and its visual evidence stay in lockstep.
	if got := characterizationRasterPinsHash(t, typedPins); got != "6a88abd716d64899669013b2c1d08725b6c0e3e2c533c8a01a7028bd1763e366" {
		t.Fatalf("typed raster baseline drift: got %s", got)
	}

	html, err := RunHTMLCharacterization(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	htmlPins := make([]characterizationRasterPin, len(html.Fixtures))
	var htmlRasterPages uint32
	var htmlRasterBytes uint64
	for index, fixture := range html.Fixtures {
		htmlPins[index] = characterizationRasterPin{Name: fixture.Name, Status: fixture.RasterStatus, Raster: fixture.Raster}
		planned := fixture.Outcome == "planned"
		validateCharacterizationRasterFixture(t, fixture.Name, fixture.Pages, planned, fixture.RasterStatus, fixture.Raster)
		if !planned && fixture.Raster != nil {
			t.Fatalf("non-planned HTML fixture %q fabricated direct-plan raster evidence", fixture.Name)
		}
		if fixture.Raster != nil {
			htmlRasterPages += uint32(len(fixture.Raster.Pages))
			for _, page := range fixture.Raster.Pages {
				htmlRasterBytes += page.PNGBytes
			}
		}
	}
	if htmlRasterPages != 1 || htmlRasterBytes == 0 || htmlRasterBytes > characterizationRasterMaxPNGBytes {
		t.Fatalf("HTML raster totals pages=%d bytes=%d", htmlRasterPages, htmlRasterBytes)
	}
	if got := characterizationRasterPinsHash(t, htmlPins); got != "9e6707c1c60c07836b37bfd98abfe6e2af238685498f30d484f4ac9ef13327a1" {
		t.Fatalf("HTML raster baseline drift: got %s", got)
	}

	fixture := typedCharacterizationFixtures()[0]
	planner := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 200, Ht: fixture.pageHeight}), WithNoCompression())
	plan, err := planner.PlanLayoutDocument(fixture.doc)
	if err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	artifact, status, err := captureCharacterizationRaster(canceled, fixture.inventory.Name, plan, &characterizationRasterBudget{})
	if !errors.Is(err, context.Canceled) || artifact != nil || status != "" {
		t.Fatalf("canceled raster artifact=%+v status=%q err=%v", artifact, status, err)
	}
	artifact, status, err = captureCharacterizationRaster(t.Context(), fixture.inventory.Name, plan, &characterizationRasterBudget{pages: characterizationRasterMaxPages})
	if err == nil || artifact != nil || status != "" {
		t.Fatalf("over-budget raster artifact=%+v status=%q err=%v", artifact, status, err)
	}
}

func validateCharacterizationRasterFixture(t *testing.T, name string, pages int, required bool, status string, evidence *CharacterizationRasterEvidence) {
	t.Helper()
	if !required {
		if status != "not-applicable" || evidence != nil {
			t.Fatalf("fixture %q raster status=%q evidence=%+v", name, status, evidence)
		}
		return
	}
	if status != "captured" || evidence == nil || len(evidence.Pages) != pages || evidence.AuthoritativePDF ||
		evidence.Renderer != layoutengine.DisplayRasterRendererVersion || evidence.Profile != layoutengine.DefaultDisplayRasterProfile() {
		t.Fatalf("fixture %q incomplete raster evidence: status=%q evidence=%+v", name, status, evidence)
	}
	for index, page := range evidence.Pages {
		if page.Page != uint32(index+1) || page.PNGSHA256 == "" || page.PNGBytes == 0 || page.ManifestSHA256 == "" ||
			page.Manifest.Page != page.Page || page.Manifest.PNGSHA256 != page.PNGSHA256 || page.Manifest.PNGByteLength != page.PNGBytes ||
			page.Manifest.AuthoritativePDF || page.Manifest.Profile != evidence.Profile || page.Manifest.Identity.RendererVersion != evidence.Renderer {
			t.Fatalf("fixture %q page %d raster evidence=%+v", name, index+1, page)
		}
		manifestJSON, err := page.Manifest.CanonicalJSON()
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(manifestJSON)
		if page.ManifestSHA256 != hex.EncodeToString(digest[:]) {
			t.Fatalf("fixture %q page %d manifest hash mismatch", name, index+1)
		}
	}
}

func characterizationRasterPinsHash(t *testing.T, pins []characterizationRasterPin) string {
	t.Helper()
	encoded, err := json.Marshal(pins)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}
