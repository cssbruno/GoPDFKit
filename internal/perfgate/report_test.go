// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package perfgate

import (
	"os"
	"strings"
	"testing"
)

const testProfile = `{
  "schema_version":1,
  "name":"test-host-v1",
  "goos":"darwin",
  "goarch":"arm64",
  "go_version_prefix":"go version go1.26.",
  "cpu":"Test CPU",
  "minimum_samples":2,
  "benchmarks":{
    "BenchmarkPaperEnginePlannerTyped":{"max_median_ns_per_op":120,"max_bytes_per_op":210,"max_allocs_per_op":12},
    "BenchmarkPaperEngineWarmCompiledPaper":{"max_median_ns_per_op":320,"max_bytes_per_op":410,"max_allocs_per_op":22}
  }
}`

const testReport = `# paperrune-paper-engine-benchmark-v3
# command: go test ./document -run ^$ -bench BenchmarkPaperEngine -benchmem -count=2
# go-version: go version go1.26.5 darwin/arm64
# goos: darwin
# goarch: arm64
# source-revision: abc123
# worktree: clean
goos: darwin
goarch: arm64
pkg: example/document
cpu: Test CPU
BenchmarkPaperEngineWarmCompiledPaper-8 10 300 ns/op 400 B/op 20 allocs/op
BenchmarkPaperEnginePlannerTyped-8 10 100 ns/op 200 B/op 10 allocs/op
BenchmarkPaperEnginePlannerTyped-8 10 110 ns/op 201 B/op 11 allocs/op
BenchmarkPaperEngineWarmCompiledPaper-8 10 310 ns/op 401 B/op 21 allocs/op
`

func TestValidateIsDeterministicHostAwareAndUsesNamedBudgets(t *testing.T) {
	first, err := Validate([]byte(testProfile), []byte(testReport))
	if err != nil {
		t.Fatal(err)
	}
	second, err := Validate([]byte(testProfile), []byte(testReport))
	if err != nil || first.Profile != "test-host-v1" || len(first.Results) != 2 || len(second.Results) != 2 {
		t.Fatalf("summaries = %#v / %#v, %v", first, second, err)
	}
	if first.Results[0].Name != "BenchmarkPaperEnginePlannerTyped" || first.Results[0].MedianNSPerOp != 110 ||
		first.Results[1].Name != "BenchmarkPaperEngineWarmCompiledPaper" || first.Results[1].MaxAllocsOp != 21 {
		t.Fatalf("canonical results = %#v", first.Results)
	}
}

func TestValidateRejectsHostSampleMetricAndBudgetDrift(t *testing.T) {
	tests := []struct{ name, report, want string }{
		{"host", strings.Replace(testReport, "cpu: Test CPU", "cpu: Other CPU", 1), "calibrated CPU"},
		{"samples", strings.Replace(testReport, "BenchmarkPaperEnginePlannerTyped-8 10 110 ns/op 201 B/op 11 allocs/op\n", "", 1), "requires at least 2"},
		{"metric", strings.Replace(testReport, "400 B/op ", "", 1), "missing B/op"},
		{"timing budget", strings.Replace(testReport, "310 ns/op", "321 ns/op", 1), "exceeds profile"},
		{"allocation budget", strings.Replace(testReport, "401 B/op", "411 B/op", 1), "exceeds profile"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Validate([]byte(testProfile), []byte(test.report)); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestValidateRejectsUnknownOrIncompleteProfilesAndOversizedReports(t *testing.T) {
	unknown := strings.Replace(testProfile, `"schema_version":1,`, `"schema_version":1,"surprise":true,`, 1)
	if _, err := Validate([]byte(unknown), []byte(testReport)); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown profile error = %v", err)
	}
	incomplete := strings.Replace(testProfile, `"minimum_samples":2`, `"minimum_samples":1`, 1)
	if _, err := Validate([]byte(incomplete), []byte(testReport)); err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("incomplete profile error = %v", err)
	}
	if _, err := Validate([]byte(testProfile), make([]byte, 16<<20+1)); err == nil || !strings.Contains(err.Error(), "16 MiB") {
		t.Fatalf("oversized report error = %v", err)
	}
}

func TestCheckedAppleM2CalibrationReportPassesDeterministically(t *testing.T) {
	profile, err := os.ReadFile("../../docs/performance/calibrations/apple-m2-go1.26.json")
	if err != nil {
		t.Fatal(err)
	}
	report, err := os.ReadFile("../../docs/performance/baselines/paper-engine-stage0-apple-m2.txt")
	if err != nil {
		t.Fatal(err)
	}
	summary, err := Validate(profile, report)
	if err != nil || summary.Profile != "apple-m2-go1.26-paper-engine-v1" || len(summary.Results) != 11 {
		t.Fatalf("checked calibration = %#v, %v", summary, err)
	}
}
