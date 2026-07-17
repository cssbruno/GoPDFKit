// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutgeom

import "testing"

func TestResolveTracksRespectsPreferredMinimumAndMaximum(t *testing.T) {
	got := ResolveTracks(100, 3, []TrackConstraint{
		{Preferred: 20},
		{Min: 40},
		{Max: 25},
	})
	want := []float64{20, 55, 25}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ResolveTracks()[%d] = %.2f, want %.2f (%v)", i, got[i], want[i], got)
		}
	}
}

func TestResolveTracksPreservesImpossibleMinimums(t *testing.T) {
	got := ResolveTracks(20, 2, []TrackConstraint{{Min: 15}, {Min: 15}})
	if got[0] != 15 || got[1] != 15 {
		t.Fatalf("ResolveTracks() = %v, want [15 15]", got)
	}
}
