// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package pdfverify

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type PopplerRasterizer struct {
	Binary   string
	Version  string
	TempRoot string
}

func (r PopplerRasterizer) Rasterize(ctx context.Context, pdf []byte, dpi uint32, dimensions []image.Point, limits Limits) (RasterOutput, error) {
	if ctx == nil || r.Binary == "" || !validLabel(r.Version) || len(dimensions) == 0 || uint32(len(dimensions)) > limits.MaxPages {
		return RasterOutput{}, ErrInvalid
	}
	var versionOutput boundedBuffer
	versionOutput.remaining = 64 << 10
	versionCommand := exec.CommandContext(ctx, r.Binary, "-v") // #nosec G204 -- executable path is an explicit trusted verifier configuration; no shell is used.
	versionCommand.Stdout, versionCommand.Stderr = &versionOutput, &versionOutput
	if err := versionCommand.Run(); err != nil || !strings.Contains(versionOutput.String(), "pdftoppm version "+r.Version) {
		return RasterOutput{}, fmt.Errorf("pdftoppm version does not match pinned %q", r.Version)
	}
	root, err := os.MkdirTemp(r.TempRoot, "gopdfkit-pdfverify-")
	if err != nil {
		return RasterOutput{}, err
	}
	defer os.RemoveAll(root)
	input := filepath.Join(root, "final.pdf")
	if err := os.WriteFile(input, pdf, 0o600); err != nil {
		return RasterOutput{}, err
	}
	output := RasterOutput{Renderer: "poppler/pdftoppm", Version: r.Version, Pages: make([][]byte, len(dimensions))}
	var total uint64
	for index, dimension := range dimensions {
		if err := ctx.Err(); err != nil {
			return RasterOutput{}, err
		}
		if dimension.X <= 0 || dimension.Y <= 0 || uint64(dimension.X)*uint64(dimension.Y) > limits.MaxPixelsPerPage {
			return RasterOutput{}, ErrLimit
		}
		prefix := filepath.Join(root, "page-"+strconv.Itoa(index+1))
		args := []string{"-png", "-singlefile", "-f", strconv.Itoa(index + 1), "-l", strconv.Itoa(index + 1), "-r", strconv.FormatUint(uint64(dpi), 10), "-scale-to-x", strconv.Itoa(dimension.X), "-scale-to-y", strconv.Itoa(dimension.Y), input, prefix}
		command := exec.CommandContext(ctx, r.Binary, args...) // #nosec G204 -- executable path is an explicit trusted verifier configuration; no shell is used.
		var diagnostics boundedBuffer
		diagnostics.remaining = 64 << 10
		command.Stdout, command.Stderr = &diagnostics, &diagnostics
		if err := command.Run(); err != nil {
			return RasterOutput{}, fmt.Errorf("pdftoppm page %d: %w: %s", index+1, err, diagnostics.String())
		}
		path := prefix + ".png"
		info, err := os.Stat(path)
		if err != nil || info.Size() <= 0 || uint64(info.Size()) > limits.MaxRasterBytesPage || uint64(info.Size()) > limits.MaxTotalRasterBytes-total {
			return RasterOutput{}, ErrLimit
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return RasterOutput{}, err
		}
		total += uint64(len(data))
		output.Pages[index] = data
	}
	return output, nil
}

type boundedBuffer struct {
	bytes.Buffer
	remaining int
}

func (b *boundedBuffer) Write(data []byte) (int, error) {
	original := len(data)
	if len(data) > b.remaining {
		data = data[:b.remaining]
	}
	b.remaining -= len(data)
	_, _ = b.Buffer.Write(data)
	return original, nil
}
