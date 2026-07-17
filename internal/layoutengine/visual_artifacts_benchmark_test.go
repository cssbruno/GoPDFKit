// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import "testing"

var paperVisualBenchmarkSink AgentVisualBundle

// BenchmarkPaperVisualContactSheet measures deterministic retained-plan page
// composition, exact transforms, manifest hashing, and detached SVG output.
func BenchmarkPaperVisualContactSheet(b *testing.B) {
	plan := paperVisualBenchmarkPlan(b)
	request := AgentVisualRequest{
		Mode: AICaptureGeometry, IncludeContactSheet: true,
		ContactSheetColumns: 2, Limits: DefaultAgentVisualLimits(),
	}
	benchmarkAgentVisualRequest(b, plan, request)
}

// BenchmarkPaperVisualExactCrops measures exact node-fragment selection and
// crop generation separately from contact-sheet presentation.
func BenchmarkPaperVisualExactCrops(b *testing.B) {
	plan := paperVisualBenchmarkPlan(b)
	projection := plan.Projection()
	nodes := make([]NodeID, len(projection.Fragments))
	for index, fragment := range projection.Fragments {
		nodes[index] = fragment.Node
	}
	request := AgentVisualRequest{
		Mode: AICaptureGeometry, Nodes: nodes, Limits: DefaultAgentVisualLimits(),
	}
	benchmarkAgentVisualRequest(b, plan, request)
}

func paperVisualBenchmarkPlan(b *testing.B) LayoutPlan {
	b.Helper()
	plan, err := NewLayoutPlan(testPlanInput())
	if err != nil {
		b.Fatal(err)
	}
	return plan
}

func benchmarkAgentVisualRequest(b *testing.B, plan LayoutPlan, request AgentVisualRequest) {
	b.Helper()
	bundle, err := CaptureAgentVisualArtifacts(plan, request)
	if err != nil || bundle.Manifest().ArtifactCount == 0 {
		b.Fatalf("validate visual request: artifacts=%d err=%v", bundle.Manifest().ArtifactCount, err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		bundle, err := CaptureAgentVisualArtifacts(plan, request)
		if err != nil {
			b.Fatal(err)
		}
		paperVisualBenchmarkSink = bundle
	}
}
