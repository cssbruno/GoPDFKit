// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"errors"
	"testing"
)

func TestCompatibilityDecisionNeverSilentlyFallsBack(t *testing.T) {
	capability := UnsupportedCapability{Code: "HTML_FLEX_WRAP_UNSUPPORTED", Stage: StageLower,
		Message: "the complete fragment cannot be lowered", FallbackAvailable: true, FallbackName: "legacy-html-fragment"}
	strict, err := DecideCompatibility(CompatibilityStrict, capability)
	if err != nil || strict.UseWholeFragmentFallback || strict.Diagnostic.Severity != SeverityError {
		t.Fatalf("strict decision = %#v, %v", strict, err)
	}
	compatible, err := DecideCompatibility(CompatibilityFallbackOK, capability)
	if err != nil || !compatible.UseWholeFragmentFallback || compatible.Diagnostic.Severity != SeverityWarning ||
		len(compatible.Diagnostic.Evidence) != 2 {
		t.Fatalf("compatible decision = %#v, %v", compatible, err)
	}
	capability.FallbackAvailable, capability.FallbackName = false, ""
	missing, err := DecideCompatibility(CompatibilityFallbackOK, capability)
	if err != nil || missing.UseWholeFragmentFallback || missing.Diagnostic.Severity != SeverityError {
		t.Fatalf("missing fallback decision = %#v, %v", missing, err)
	}
}

func TestCompatibilityDecisionRejectsAmbiguousContracts(t *testing.T) {
	base := UnsupportedCapability{Code: "FEATURE_UNSUPPORTED", Stage: StageLower, Message: "unsupported"}
	if _, err := DecideCompatibility("automatic", base); !errors.Is(err, ErrCompatibilityContract) {
		t.Fatalf("mode error = %v", err)
	}
	base.FallbackName = "hidden"
	if _, err := DecideCompatibility(CompatibilityFallbackOK, base); !errors.Is(err, ErrCompatibilityContract) {
		t.Fatalf("ambiguous fallback error = %v", err)
	}
}
