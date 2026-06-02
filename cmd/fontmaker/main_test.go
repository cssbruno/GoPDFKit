/****************************************************************************
 * Software: GoPDFKit                                                         *
 * License:  MIT License                                                    *
 *                                                                          *
 * Copyright (c) 2026 cssBruno                                              *
 ****************************************************************************/

package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestFontmakerBuildsCalligraFont(t *testing.T) {
	tempDir := t.TempDir()
	binPath := filepath.Join(tempDir, "fontmaker")

	build := exec.Command("go", "build", "-o", binPath, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, output)
	}

	run := exec.Command(binPath,
		"--dst="+tempDir,
		"--embed",
		"--enc=../../assets/static/font/cp1252.map",
		"../../assets/static/font/calligra.ttf",
	)
	output, err := run.CombinedOutput()
	if err != nil {
		t.Fatalf("fontmaker: %v\n%s", err, output)
	}
	for _, want := range []string{
		"Font definition file successfully generated",
		"fontmaker: generated 1 font definition(s)",
	} {
		if !strings.Contains(string(output), want) {
			t.Fatalf("fontmaker output missing %q:\n%s", want, output)
		}
	}
	for _, name := range []string{"calligra.json", "calligra.z"} {
		if _, err = os.Stat(filepath.Join(tempDir, name)); err != nil {
			t.Fatalf("generated %s: %v", name, err)
		}
	}
}

func TestRunRejectsMissingFontArgument(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := newCommand("fontmaker", &stdout, &stderr, neverBuild(t)).run(nil)

	if code != exitUsage {
		t.Fatalf("exit code = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(stderr.String(), "at least one") {
		t.Fatalf("stderr missing validation error:\n%s", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
}

func TestRunContinuesAfterOneFontFails(t *testing.T) {
	dir := t.TempDir()
	encoding := filepath.Join(dir, "test.map")
	goodFont := filepath.Join(dir, "good.ttf")
	badFont := filepath.Join(dir, "bad.ttf")
	writeFile(t, encoding, "!20 U+0020 space\n")
	writeFile(t, goodFont, "not a real font; fake builder handles it")
	writeFile(t, badFont, "not a real font; fake builder handles it")

	var built []string
	build := func(fontPath, encodingPath, outputDir string, log io.Writer, embed bool) error {
		built = append(built, filepath.Base(fontPath))
		if filepath.Base(fontPath) == "bad.ttf" {
			return errors.New("synthetic failure")
		}
		return nil
	}
	var stdout, stderr bytes.Buffer
	code := newCommand("fontmaker", &stdout, &stderr, build).run([]string{
		"--dst", filepath.Join(dir, "out"),
		"--enc", encoding,
		goodFont,
		badFont,
	})

	if code != exitBuild {
		t.Fatalf("exit code = %d, want %d", code, exitBuild)
	}
	if got := strings.Join(built, ","); got != "good.ttf,bad.ttf" {
		t.Fatalf("built fonts = %s", got)
	}
	if !strings.Contains(stderr.String(), "bad.ttf: synthetic failure") {
		t.Fatalf("stderr missing joined failure:\n%s", stderr.String())
	}
}

func TestResolveEncodingFindsRepositoryDefault(t *testing.T) {
	got, err := resolveEncoding(defaultEncoding)
	if err != nil {
		t.Fatalf("resolveEncoding: %v", err)
	}
	if filepath.Base(got) != defaultEncoding {
		t.Fatalf("resolved encoding = %s, want %s", got, defaultEncoding)
	}
}

func neverBuild(t *testing.T) fontBuildFunc {
	t.Helper()
	return func(fontPath, encodingPath, outputDir string, log io.Writer, embed bool) error {
		t.Fatalf("build called for %s", fontPath)
		return nil
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
