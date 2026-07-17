// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/cssbruno/gopdfkit/internal/perfgate"
)

func main() {
	profilePath := flag.String("profile", "docs/performance/calibrations/apple-m2-go1.26.json", "named host calibration profile")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: paper-benchmark-check [-profile profile.json] report.txt")
		os.Exit(2)
	}
	profile, err := os.ReadFile(*profilePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read calibration profile: %v\n", err)
		os.Exit(1)
	}
	report, err := os.ReadFile(flag.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "read benchmark report: %v\n", err)
		os.Exit(1)
	}
	summary, err := perfgate.Validate(profile, report)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("Paper Engine performance gate passed: profile=%s cohorts=%d\n", summary.Profile, len(summary.Results))
	for _, result := range summary.Results {
		fmt.Printf("%s samples=%d median_ns/op=%.0f max_B/op=%.0f max_allocs/op=%.0f\n",
			result.Name, result.Samples, result.MedianNSPerOp, result.MaxBytesPerOp, result.MaxAllocsOp)
	}
}
