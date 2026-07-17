// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestPlanPaperIdentityExcludesAmbientProcessEnvironment(t *testing.T) {
	if os.Getenv("GOPDFKIT_PAPER_IDENTITY_HELPER") == "1" {
		plan, result, err := PlanPaper("deterministic.paper", paperPipelineFixture)
		if err != nil {
			t.Fatal(err)
		}
		manifest, err := plan.DeterministicInputManifest()
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(struct {
			PlanHash string          `json:"plan_hash"`
			Inputs   json.RawMessage `json:"inputs"`
		}{PlanHash: result.Hash, Inputs: manifest.JSON()})
		if err != nil {
			t.Fatal(err)
		}
		_, _ = os.Stdout.Write(encoded)
		os.Exit(0)
	}

	run := func(directory string, variables map[string]string) []byte {
		t.Helper()
		command := exec.Command(os.Args[0], "-test.run=^TestPlanPaperIdentityExcludesAmbientProcessEnvironment$")
		command.Dir = directory
		command.Env = deterministicFixtureEnvironment(os.Environ(), variables)
		output, err := command.Output()
		if err != nil {
			t.Fatalf("deterministic identity helper: %v", err)
		}
		return output
	}
	firstRoot, secondRoot := t.TempDir(), t.TempDir()
	first := run(firstRoot, map[string]string{
		"GOPDFKIT_PAPER_IDENTITY_HELPER": "1", "TZ": "Pacific/Kiritimati", "LANG": "tr_TR.UTF-8", "LC_ALL": "tr_TR.UTF-8",
		"SOURCE_DATE_EPOCH": "1", "HOME": filepath.Join(firstRoot, "home"), "FONTCONFIG_FILE": filepath.Join(firstRoot, "fonts.conf"), "FONTCONFIG_PATH": firstRoot,
	})
	second := run(secondRoot, map[string]string{
		"GOPDFKIT_PAPER_IDENTITY_HELPER": "1", "TZ": "America/Los_Angeles", "LANG": "ja_JP.UTF-8", "LC_ALL": "ja_JP.UTF-8",
		"SOURCE_DATE_EPOCH": "4102444800", "HOME": filepath.Join(secondRoot, "home"), "FONTCONFIG_FILE": filepath.Join(secondRoot, "missing-fonts.conf"), "FONTCONFIG_PATH": secondRoot,
	})
	if !bytes.Equal(first, second) || !json.Valid(first) {
		t.Fatalf("ambient process state changed identical-input identity:\n%s\n%s", first, second)
	}
	for _, ambient := range []string{firstRoot, secondRoot, "Pacific/Kiritimati", "America/Los_Angeles", "tr_TR", "ja_JP", "SOURCE_DATE_EPOCH", "FONTCONFIG"} {
		if bytes.Contains(first, []byte(ambient)) {
			t.Fatalf("identity fixture leaked ambient value %q: %s", ambient, first)
		}
	}
	sum := sha256.Sum256(first)
	const fixtureSHA256 = "d40fdb40f29612f11b876eaca6221c039e51bef4544305a26f9b9cf1f8eca4b9"
	if got := hex.EncodeToString(sum[:]); got != fixtureSHA256 {
		t.Fatalf("deterministic paper identity fixture hash = %s", got)
	}
}

func deterministicFixtureEnvironment(base []string, replacements map[string]string) []string {
	result := make([]string, 0, len(base)+len(replacements))
	for _, entry := range base {
		key := entry
		if index := strings.IndexByte(entry, '='); index >= 0 {
			key = entry[:index]
		}
		if _, replaced := replacements[key]; !replaced {
			result = append(result, entry)
		}
	}
	keys := make([]string, 0, len(replacements))
	for key := range replacements {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		result = append(result, key+"="+replacements[key])
	}
	return result
}
