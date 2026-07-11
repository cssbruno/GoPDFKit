// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultEncoding = "cp1252.map"
	exitOK          = 0
	exitUsage       = 2
	exitBuild       = 1
)

type command struct {
	name   string
	stdout io.Writer
	stderr io.Writer
	build  fontBuildFunc
}

type fontmakerOptions struct {
	outputDir string
	encoding  string
	embed     bool
	help      bool
	fonts     []string
}

func newCommand(name string, stdout, stderr io.Writer, build fontBuildFunc) command {
	base := filepath.Base(name)
	if base == "." || base == string(filepath.Separator) || base == "" {
		base = "fontmaker"
	}
	return command{
		name:   base,
		stdout: stdout,
		stderr: stderr,
		build:  build,
	}
}

func (cmd command) run(args []string) int {
	options, err := cmd.parse(args)
	if err != nil {
		_, _ = fmt.Fprintln(cmd.stderr, err)
		cmd.printUsage()
		return exitUsage
	}
	if options.help {
		cmd.printUsage()
		return exitOK
	}
	if err = options.prepare(); err != nil {
		_, _ = fmt.Fprintln(cmd.stderr, err)
		return exitUsage
	}
	if err = cmd.makeFonts(options); err != nil {
		_, _ = fmt.Fprintln(cmd.stderr, err)
		return exitBuild
	}
	return exitOK
}

func (cmd command) parse(args []string) (fontmakerOptions, error) {
	options := fontmakerOptions{
		outputDir: ".",
		encoding:  defaultEncoding,
	}
	flags := cmd.newFlagSet(&options, io.Discard)
	if err := flags.Parse(args); err != nil {
		return fontmakerOptions{}, err
	}
	options.fonts = flags.Args()
	return options, nil
}

func (cmd command) newFlagSet(options *fontmakerOptions, output io.Writer) *flag.FlagSet {
	flags := flag.NewFlagSet(cmd.name, flag.ContinueOnError)
	flags.SetOutput(output)
	flags.StringVar(&options.outputDir, "dst", options.outputDir, "directory for generated .json and .z files")
	flags.StringVar(&options.encoding, "enc", options.encoding, "code page map file")
	flags.BoolVar(&options.embed, "embed", false, "compress and embed the font program")
	flags.BoolVar(&options.help, "help", false, "show command usage")
	return flags
}

func (options *fontmakerOptions) prepare() error {
	if len(options.fonts) == 0 {
		return errors.New("at least one TrueType, OpenType, or Type1 font must be specified")
	}
	outputDir := strings.TrimSpace(options.outputDir)
	if outputDir == "" {
		return errors.New("output directory cannot be empty")
	}
	if err := os.MkdirAll(outputDir, 0o750); err != nil { // #nosec G703 -- --dst explicitly selects the CLI output directory.
		return fmt.Errorf("prepare output directory %s: %w", outputDir, err)
	}
	encoding, err := resolveEncoding(strings.TrimSpace(options.encoding))
	if err != nil {
		return err
	}
	options.outputDir = outputDir
	options.encoding = encoding
	return nil
}

func (cmd command) makeFonts(options fontmakerOptions) error {
	var failures []error
	for _, fontPath := range options.fonts {
		fontPath = strings.TrimSpace(fontPath)
		if fontPath == "" {
			failures = append(failures, errors.New("empty font path"))
			continue
		}
		if _, err := os.Stat(fontPath); err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", fontPath, err))
			continue
		}
		if err := cmd.build(fontPath, options.encoding, options.outputDir, cmd.stdout, options.embed); err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", fontPath, err))
		}
	}
	if len(failures) > 0 {
		return errors.Join(failures...)
	}
	_, _ = fmt.Fprintf(cmd.stdout, "fontmaker: generated %d font definition(s) in %s\n", len(options.fonts), options.outputDir)
	return nil
}

func resolveEncoding(encoding string) (string, error) {
	if encoding == "" {
		return "", errors.New("encoding file cannot be empty")
	}
	if fileExists(encoding) {
		return encoding, nil
	}
	if filepath.IsAbs(encoding) {
		return "", fmt.Errorf("encoding file not found: %s", encoding)
	}
	for _, candidate := range encodingCandidates(encoding) {
		if fileExists(candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("encoding file not found: %s", encoding)
}

func encodingCandidates(encoding string) []string {
	return []string{
		filepath.Join("assets", "static", "font", encoding),
		filepath.Join("..", "..", "assets", "static", "font", encoding),
		filepath.Join("..", "assets", "static", "font", encoding),
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path) // #nosec G703 -- fontmaker accepts operator-selected font and encoding paths.
	return err == nil && info != nil && !info.IsDir()
}

func (cmd command) printUsage() {
	_, _ = fmt.Fprintf(cmd.stderr, "Usage: %s [options] font_file [font_file...]\n\n", cmd.name)
	_, _ = fmt.Fprintln(cmd.stderr, "Generate GoPDFKit JSON font definitions from TrueType, OpenType, or binary Type1 fonts.")
	_, _ = fmt.Fprintln(cmd.stderr, "Type1 fonts require an AFM metrics file with the same base path.")
	_, _ = fmt.Fprintln(cmd.stderr)
	_, _ = fmt.Fprintln(cmd.stderr, "Options:")
	cmd.printFlagDefaults()
	_, _ = fmt.Fprintln(cmd.stderr)
	_, _ = fmt.Fprintf(cmd.stderr, "Example:\n  %s --embed --enc=../../assets/static/font/cp1252.map --dst=../../assets/static/font ../../assets/static/font/calligra.ttf\n", cmd.name)
}

func (cmd command) printFlagDefaults() {
	options := fontmakerOptions{
		outputDir: ".",
		encoding:  defaultEncoding,
	}
	flags := cmd.newFlagSet(&options, cmd.stderr)
	flags.PrintDefaults()
}
