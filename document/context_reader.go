// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"context"
	"io"
)

// contextReader makes cancellation consistent for bounded reads performed by
// image, attachment, SVG, and import helpers.
type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (reader contextReader) Read(buffer []byte) (int, error) {
	if err := outputCanceledError(reader.ctx); err != nil {
		return 0, err
	}
	count, err := reader.r.Read(buffer)
	if err != nil {
		return count, err
	}
	if count == 0 {
		return 0, outputCanceledError(reader.ctx)
	}
	return count, nil
}
