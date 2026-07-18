// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"math/big"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
)

// typedContainerPercent resolves a millionth-of-one-percent ratio in fixed
// point. DPI is deliberately absent: raster density is a paint concern, not a
// document-layout input.
func typedContainerPercent(reference layoutengine.Fixed, percent uint32) (layoutengine.Fixed, error) {
	if reference < 0 || percent > 100_000_000 {
		return 0, layoutengine.ErrGeometryOverflow
	}
	product := new(big.Int).Mul(big.NewInt(int64(reference)), new(big.Int).SetUint64(uint64(percent)))
	product.Quo(product, big.NewInt(100_000_000))
	if !product.IsInt64() || product.Sign() < 0 {
		return 0, layoutengine.ErrGeometryOverflow
	}
	return layoutengine.Fixed(product.Int64()), nil
}

func (f *Document) typedContainerPercentUnits(reference float64, percent uint32) (float64, error) {
	fixed, err := layoutengine.FixedFromPoints(f.UnitToPointConvert(reference))
	if err != nil {
		return 0, err
	}
	resolved, err := typedContainerPercent(fixed, percent)
	if err != nil {
		return 0, err
	}
	return f.PointConvert(resolved.Points()), nil
}
