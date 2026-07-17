// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package main

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

func TestRunRejectsMissingAndCanceledInput(t *testing.T) {
	if err := run(context.Background(), nil, "test"); err == nil {
		t.Fatal("missing fixtures succeeded")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := run(ctx, []string{"missing.pdf"}, "test"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled run error = %v", err)
	}
}

func TestBuiltinCharacterizationIsDeterministicBoundedAndCancelable(t *testing.T) {
	for _, kind := range []string{"typed", "html"} {
		first, err := builtinCharacterization(t.Context(), kind)
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		second, err := builtinCharacterization(t.Context(), kind)
		if err != nil || !bytes.Equal(first, second) || len(first) == 0 || len(first) > 128<<20 ||
			!bytes.Contains(first, []byte(`"fixtures"`)) || !bytes.Contains(first, []byte(`"raster_status"`)) ||
			!bytes.Contains(first, []byte(`"png_sha256"`)) || !bytes.Contains(first, []byte(`"manifest_sha256"`)) {
			t.Fatalf("%s report is invalid/nondeterministic: bytes=%d err=%v", kind, len(first), err)
		}
	}
	if _, err := builtinCharacterization(t.Context(), "unknown"); err == nil {
		t.Fatal("unknown builtin succeeded")
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := builtinCharacterization(canceled, "typed"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled builtin = %v", err)
	}
}
