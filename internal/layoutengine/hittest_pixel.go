// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"errors"
	"fmt"
	"math/big"
)

// HitTestMaxRasterDimension bounds screenshot-coordinate inputs independently
// from the retained plan. It prevents hostile metadata from turning a simple
// inspection request into unbounded integer work.
const HitTestMaxRasterDimension uint32 = 1 << 20

var ErrHitTestRasterInvalid = errors.New("layoutengine: hit-test raster geometry is invalid")

// RasterPixelQuery identifies one pixel in a rasterized page or crop. PixelX
// and PixelY are zero-based pixel indexes. CaptureBounds is the exact page-
// coordinate rectangle represented by the complete raster; it may include
// planned overflow outside PageBounds.
type RasterPixelQuery struct {
	Page          uint32 `json:"page"`
	PixelX        uint32 `json:"pixel_x"`
	PixelY        uint32 `json:"pixel_y"`
	PixelWidth    uint32 `json:"pixel_width"`
	PixelHeight   uint32 `json:"pixel_height"`
	CaptureBounds Rect   `json:"capture_bounds"`
}

// RasterPixelHitTest preserves the declared raster geometry and the exact
// pixel-center page coordinate used for the ordinary bounded hit test.
type RasterPixelHitTest struct {
	Raster    RasterPixelQuery `json:"raster"`
	PagePoint Point            `json:"page_point"`
	Hit       PageHitTest      `json:"hit"`
}

// HitTestRasterPixel maps a screenshot pixel center into exact fixed page
// coordinates, then uses the canonical page hit tester. Mapping is rational
// and rounds half a fixed unit away from zero; it does not pass through float,
// CSS pixels, DPI assumptions, or renderer-specific transforms.
func (p LayoutPlan) HitTestRasterPixel(query RasterPixelQuery) (RasterPixelHitTest, error) {
	if query.PixelWidth == 0 || query.PixelHeight == 0 ||
		query.PixelWidth > HitTestMaxRasterDimension || query.PixelHeight > HitTestMaxRasterDimension {
		return RasterPixelHitTest{}, fmt.Errorf("%w: dimensions must be between 1 and %d", ErrHitTestRasterInvalid, HitTestMaxRasterDimension)
	}
	if query.PixelX >= query.PixelWidth || query.PixelY >= query.PixelHeight {
		return RasterPixelHitTest{}, fmt.Errorf("%w: pixel (%d,%d) is outside %dx%d", ErrHitTestRasterInvalid,
			query.PixelX, query.PixelY, query.PixelWidth, query.PixelHeight)
	}
	if err := query.CaptureBounds.Validate(); err != nil {
		return RasterPixelHitTest{}, fmt.Errorf("%w: capture bounds: %v", ErrHitTestRasterInvalid, err)
	}
	if query.CaptureBounds.IsEmpty() {
		return RasterPixelHitTest{}, fmt.Errorf("%w: capture bounds must be non-empty", ErrHitTestRasterInvalid)
	}

	xOffset, err := rasterPixelCenterOffset(query.CaptureBounds.Width, query.PixelX, query.PixelWidth)
	if err != nil {
		return RasterPixelHitTest{}, fmt.Errorf("%w: x coordinate: %v", ErrHitTestRasterInvalid, err)
	}
	yOffset, err := rasterPixelCenterOffset(query.CaptureBounds.Height, query.PixelY, query.PixelHeight)
	if err != nil {
		return RasterPixelHitTest{}, fmt.Errorf("%w: y coordinate: %v", ErrHitTestRasterInvalid, err)
	}
	point, err := (Point{X: query.CaptureBounds.X, Y: query.CaptureBounds.Y}).Translate(xOffset, yOffset)
	if err != nil {
		return RasterPixelHitTest{}, fmt.Errorf("%w: mapped point: %v", ErrHitTestRasterInvalid, err)
	}
	hit, err := p.HitTestPage(query.Page, point)
	if err != nil {
		return RasterPixelHitTest{}, err
	}
	return RasterPixelHitTest{Raster: query, PagePoint: point, Hit: hit}, nil
}

func rasterPixelCenterOffset(extent Fixed, pixel, pixels uint32) (Fixed, error) {
	// extent * (2*pixel+1) / (2*pixels), rounded to the nearest fixed
	// unit. All operands are non-negative, so half-away-from-zero is the
	// familiar quotient after adding half the denominator.
	numerator := new(big.Int).Mul(big.NewInt(int64(extent)), new(big.Int).SetUint64(uint64(pixel)*2+1))
	denominator := new(big.Int).SetUint64(uint64(pixels) * 2)
	numerator.Add(numerator, new(big.Int).Rsh(new(big.Int).Set(denominator), 1))
	numerator.Quo(numerator, denominator)
	if !numerator.IsInt64() {
		return 0, ErrGeometryOverflow
	}
	return Fixed(numerator.Int64()), nil
}
