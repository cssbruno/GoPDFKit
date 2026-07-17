// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

// Package perfgate validates reproducible Go benchmark reports against an
// explicitly named host/toolchain calibration. It never runs benchmarks and
// therefore keeps wall-clock noise out of ordinary unit tests.
package perfgate

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

const SchemaVersion uint16 = 1

type Budget struct {
	MaxMedianNSPerOp float64 `json:"max_median_ns_per_op"`
	MaxBytesPerOp    float64 `json:"max_bytes_per_op"`
	MaxAllocsPerOp   float64 `json:"max_allocs_per_op"`
}

type Profile struct {
	SchemaVersion   uint16            `json:"schema_version"`
	Name            string            `json:"name"`
	GOOS            string            `json:"goos"`
	GOARCH          string            `json:"goarch"`
	GoVersionPrefix string            `json:"go_version_prefix"`
	CPU             string            `json:"cpu"`
	MinimumSamples  uint32            `json:"minimum_samples"`
	Benchmarks      map[string]Budget `json:"benchmarks"`
}

type Result struct {
	Name          string
	Samples       int
	MedianNSPerOp float64
	MaxBytesPerOp float64
	MaxAllocsOp   float64
}

type Summary struct {
	Profile string
	Results []Result
}

type sample struct{ ns, bytes, allocs float64 }

func Validate(profileJSON, report []byte) (Summary, error) {
	profile, err := decodeProfile(profileJSON)
	if err != nil {
		return Summary{}, err
	}
	if len(report) == 0 || len(report) > 16<<20 {
		return Summary{}, errors.New("perfgate: benchmark report is empty or exceeds 16 MiB")
	}
	headers := map[string]string{}
	rows := map[string][]sample{}
	cpus := map[string]bool{}
	scanner := bufio.NewScanner(bytes.NewReader(report))
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	lines := 0
	for scanner.Scan() {
		lines++
		if lines > 100000 {
			return Summary{}, errors.New("perfgate: benchmark report exceeds 100000 lines")
		}
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "# ") {
			if key, value, ok := strings.Cut(strings.TrimPrefix(line, "# "), ":"); ok {
				headers[strings.TrimSpace(key)] = strings.TrimSpace(value)
			}
			continue
		}
		if strings.HasPrefix(line, "cpu:") {
			cpus[strings.TrimSpace(strings.TrimPrefix(line, "cpu:"))] = true
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 || !strings.HasPrefix(fields[0], "BenchmarkPaperEngine") {
			continue
		}
		name := stripProcessorSuffix(fields[0])
		if _, required := profile.Benchmarks[name]; !required {
			continue
		}
		value, parseErr := parseSample(fields)
		if parseErr != nil {
			return Summary{}, fmt.Errorf("perfgate: %s: %w", name, parseErr)
		}
		rows[name] = append(rows[name], value)
	}
	if err := scanner.Err(); err != nil {
		return Summary{}, fmt.Errorf("perfgate: read benchmark report: %w", err)
	}
	for _, field := range []string{"command", "go-version", "goos", "goarch", "source-revision", "worktree"} {
		if headers[field] == "" {
			return Summary{}, fmt.Errorf("perfgate: benchmark report is missing fingerprint field %q", field)
		}
	}
	if headers["goos"] != profile.GOOS || headers["goarch"] != profile.GOARCH ||
		!strings.HasPrefix(headers["go-version"], profile.GoVersionPrefix) {
		return Summary{}, fmt.Errorf("perfgate: report toolchain %s/%s %q does not match profile %s/%s %q",
			headers["goos"], headers["goarch"], headers["go-version"], profile.GOOS, profile.GOARCH, profile.GoVersionPrefix)
	}
	if len(cpus) != 1 || !cpus[profile.CPU] {
		return Summary{}, fmt.Errorf("perfgate: report CPUs %v do not match calibrated CPU %q", sortedKeys(cpus), profile.CPU)
	}

	names := make([]string, 0, len(profile.Benchmarks))
	for name := range profile.Benchmarks {
		names = append(names, name)
	}
	sort.Strings(names)
	summary := Summary{Profile: profile.Name, Results: make([]Result, 0, len(names))}
	for _, name := range names {
		samples := rows[name]
		if len(samples) < int(profile.MinimumSamples) {
			return Summary{}, fmt.Errorf("perfgate: %s has %d samples, requires at least %d", name, len(samples), profile.MinimumSamples)
		}
		ns := make([]float64, len(samples))
		result := Result{Name: name, Samples: len(samples)}
		for index, current := range samples {
			ns[index] = current.ns
			result.MaxBytesPerOp = math.Max(result.MaxBytesPerOp, current.bytes)
			result.MaxAllocsOp = math.Max(result.MaxAllocsOp, current.allocs)
		}
		sort.Float64s(ns)
		result.MedianNSPerOp = ns[len(ns)/2]
		budget := profile.Benchmarks[name]
		if result.MedianNSPerOp > budget.MaxMedianNSPerOp || result.MaxBytesPerOp > budget.MaxBytesPerOp || result.MaxAllocsOp > budget.MaxAllocsPerOp {
			return Summary{}, fmt.Errorf("perfgate: %s exceeds profile %q: median ns/op %.0f/%.0f, max B/op %.0f/%.0f, max allocs/op %.0f/%.0f",
				name, profile.Name, result.MedianNSPerOp, budget.MaxMedianNSPerOp, result.MaxBytesPerOp, budget.MaxBytesPerOp, result.MaxAllocsOp, budget.MaxAllocsPerOp)
		}
		summary.Results = append(summary.Results, result)
	}
	return summary, nil
}

func decodeProfile(encoded []byte) (Profile, error) {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var profile Profile
	if err := decoder.Decode(&profile); err != nil {
		return Profile{}, fmt.Errorf("perfgate: decode profile: %w", err)
	}
	if profile.SchemaVersion != SchemaVersion || profile.Name == "" || profile.GOOS == "" || profile.GOARCH == "" ||
		profile.GoVersionPrefix == "" || profile.CPU == "" || profile.MinimumSamples < 2 || len(profile.Benchmarks) == 0 {
		return Profile{}, errors.New("perfgate: calibration profile is incomplete")
	}
	for name, budget := range profile.Benchmarks {
		if !strings.HasPrefix(name, "BenchmarkPaperEngine") || budget.MaxMedianNSPerOp <= 0 || budget.MaxBytesPerOp <= 0 || budget.MaxAllocsPerOp <= 0 {
			return Profile{}, fmt.Errorf("perfgate: invalid budget for %q", name)
		}
	}
	return profile, nil
}

func parseSample(fields []string) (sample, error) {
	result := sample{}
	found := map[string]bool{}
	for index := 1; index+1 < len(fields); index++ {
		unit := fields[index+1]
		if unit != "ns/op" && unit != "B/op" && unit != "allocs/op" {
			continue
		}
		value, err := strconv.ParseFloat(fields[index], 64)
		if err != nil || value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			return sample{}, fmt.Errorf("invalid %s value %q", unit, fields[index])
		}
		found[unit] = true
		switch unit {
		case "ns/op":
			result.ns = value
		case "B/op":
			result.bytes = value
		case "allocs/op":
			result.allocs = value
		}
	}
	for _, unit := range []string{"ns/op", "B/op", "allocs/op"} {
		if !found[unit] {
			return sample{}, fmt.Errorf("missing %s", unit)
		}
	}
	return result, nil
}

func stripProcessorSuffix(name string) string {
	if index := strings.LastIndexByte(name, '-'); index > 0 {
		if _, err := strconv.ParseUint(name[index+1:], 10, 32); err == nil {
			return name[:index]
		}
	}
	return name
}

func sortedKeys(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
