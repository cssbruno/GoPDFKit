// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

// Command paper-characterize emits a deterministic JSON compatibility report
// for one or more generated PDF fixtures.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cssbruno/gopdfkit/document"
	"github.com/cssbruno/gopdfkit/internal/characterize"
)

func main() {
	command := flag.String("command", "paper-characterize", "reproduction command recorded in the report")
	builtin := flag.String("builtin", "", "emit the built-in typed or html characterization corpus")
	flag.Parse()
	if *builtin != "" {
		encoded, err := builtinCharacterization(context.Background(), *builtin)
		if err == nil {
			_, err = os.Stdout.Write(append(encoded, '\n'))
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if err := run(context.Background(), flag.Args(), *command); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func builtinCharacterization(ctx context.Context, kind string) ([]byte, error) {
	normalized := strings.ToLower(strings.TrimSpace(kind))
	var projection []byte
	var err error
	switch normalized {
	case "typed":
		result, runErr := document.RunTypedCharacterization(ctx, document.TypedCharacterizationLimits{})
		err = runErr
		if err != nil {
			return nil, err
		}
		projection, err = result.CanonicalJSON()
	case "html":
		result, runErr := document.RunHTMLCharacterization(ctx)
		err = runErr
		if err != nil {
			return nil, err
		}
		projection, err = result.CanonicalJSON()
	default:
		return nil, fmt.Errorf("paper-characterize: builtin must be typed or html")
	}
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		SchemaVersion uint16                   `json:"schema_version"`
		Kind          string                   `json:"kind"`
		Command       string                   `json:"command"`
		Fingerprint   characterize.Fingerprint `json:"fingerprint"`
		Projection    json.RawMessage          `json:"projection"`
	}{SchemaVersion: 1, Kind: normalized, Command: "paper-characterize -builtin " + normalized,
		Fingerprint: characterize.RuntimeFingerprint(), Projection: projection})
}

func run(ctx context.Context, names []string, command string) error {
	if len(names) == 0 {
		return fmt.Errorf("paper-characterize: provide at least one PDF fixture")
	}
	ordered := append([]string(nil), names...)
	sort.Strings(ordered)
	limits := characterize.DefaultLimits()
	artifacts := make([]characterize.Artifact, 0, len(ordered))
	var total uint64
	for _, name := range ordered {
		if err := ctx.Err(); err != nil {
			return err
		}
		info, err := os.Stat(name)
		if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || uint64(info.Size()) > limits.MaxPDFBytes || uint64(info.Size()) > limits.MaxTotalBytes-total { // #nosec G115 -- fixed-width conversion is bounded by the surrounding parser, planner, or resource invariant
			return fmt.Errorf("paper-characterize: invalid or over-budget fixture %q", name)
		}
		content, err := os.ReadFile(name) // #nosec G304 -- name is an explicit characterization fixture path validated above.
		if err != nil {
			return fmt.Errorf("paper-characterize: read %q: %w", name, err)
		}
		total += uint64(len(content))
		artifacts = append(artifacts, characterize.Artifact{Name: filepath.ToSlash(filepath.Clean(name)), PDF: content})
	}
	report, err := characterize.Build(ctx, artifacts, strings.TrimSpace(command), characterize.RuntimeFingerprint(), limits)
	if err != nil {
		return err
	}
	encoded, err := report.CanonicalJSON(limits.MaxJSONBytes)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(encoded)
	return err
}
