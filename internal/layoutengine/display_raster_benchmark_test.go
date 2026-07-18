// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"testing"
)

// BenchmarkDisplayRasterPNGEncode compares the production filter-none encoder
// with the previous standard-library BestSpeed path when
// GOPDFKIT_RASTER_BENCH_LEGACY=1. Keeping one benchmark name allows benchstat
// to compare exact before/after samples without editing benchmark output.
func BenchmarkDisplayRasterPNGEncode(b *testing.B) {
	canvas := image.NewRGBA(image.Rect(0, 0, 480, 320))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.RGBA{R: 255, G: 255, B: 255, A: 255}), image.Point{}, draw.Src)
	ink := image.NewUniform(color.RGBA{R: 28, G: 36, B: 48, A: 255})
	accent := image.NewUniform(color.RGBA{R: 42, G: 116, B: 178, A: 255})
	for line := 0; line < 14; line++ {
		y := 28 + line*18
		width := 250 + (line%4)*42
		draw.Draw(canvas, image.Rect(32, y, 32+width, y+3), ink, image.Point{}, draw.Src)
	}
	draw.Draw(canvas, image.Rect(32, 18, 448, 21), accent, image.Point{}, draw.Src)
	draw.Draw(canvas, image.Rect(32, 286, 448, 288), accent, image.Point{}, draw.Src)
	legacy := os.Getenv("GOPDFKIT_RASTER_BENCH_LEGACY") == "1"
	b.ReportAllocs()
	b.SetBytes(int64(len(canvas.Pix)))
	b.ResetTimer()
	for range b.N {
		var output bytes.Buffer
		var err error
		if legacy {
			err = (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&output, canvas)
		} else {
			err = encodeRasterPNG(&output, canvas)
		}
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDisplayRasterPrescriptionPayload(b *testing.B) {
	path := os.Getenv("GOPDFKIT_RASTER_PAYLOAD")
	if path == "" {
		b.Skip("set GOPDFKIT_RASTER_PAYLOAD to an exact web render payload")
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	var cache WebDisplayRenderCache
	useCache := os.Getenv("GOPDFKIT_RASTER_BENCH_CACHE") == "1"
	b.ResetTimer()
	for range b.N {
		var artifact DisplayRasterArtifact
		var renderErr error
		if useCache {
			artifact, renderErr = RenderWebDisplayPayloadCached(context.Background(), payload, &cache)
		} else {
			artifact, renderErr = RenderWebDisplayPayload(context.Background(), payload)
		}
		if renderErr != nil {
			b.Fatal(renderErr)
		}
		if len(artifact.PNG()) == 0 {
			b.Fatal("empty PNG")
		}
	}
}
