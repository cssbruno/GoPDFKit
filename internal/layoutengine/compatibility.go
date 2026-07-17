// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"errors"
	"fmt"
	"strings"
)

type CompatibilityMode string

const (
	CompatibilityStrict     CompatibilityMode = "strict"
	CompatibilityFallbackOK CompatibilityMode = "fallback-allowed"
)

var ErrCompatibilityContract = errors.New("layoutengine: compatibility contract is invalid")

// UnsupportedCapability describes one frontend capability that the unified
// planner cannot currently lower. FallbackName must identify a whole-fragment
// compatibility implementation; nested legacy islands are never authorized.
type UnsupportedCapability struct {
	Code              DiagnosticCode
	Stage             PipelineStage
	Message           string
	Location          DiagnosticLocation
	FallbackAvailable bool
	FallbackName      string
}

type CompatibilityDecision struct {
	UseWholeFragmentFallback bool
	Diagnostic               Diagnostic
}

// DecideCompatibility makes strict-versus-fallback behavior explicit and
// inspectable. Strict mode always fails. Fallback-allowed mode only authorizes
// a named whole-fragment fallback; absence of one remains an error.
func DecideCompatibility(mode CompatibilityMode, capability UnsupportedCapability) (CompatibilityDecision, error) {
	if mode != CompatibilityStrict && mode != CompatibilityFallbackOK {
		return CompatibilityDecision{}, fmt.Errorf("%w: unknown mode %q", ErrCompatibilityContract, mode)
	}
	if err := capability.Code.validate(); err != nil || !capability.Stage.valid() || strings.TrimSpace(capability.Message) == "" {
		return CompatibilityDecision{}, fmt.Errorf("%w: capability identity", ErrCompatibilityContract)
	}
	if err := capability.Location.validate(); err != nil {
		return CompatibilityDecision{}, fmt.Errorf("%w: location: %v", ErrCompatibilityContract, err)
	}
	if capability.FallbackAvailable {
		if err := validateTextIdentity("compatibility fallback name", capability.FallbackName); err != nil {
			return CompatibilityDecision{}, fmt.Errorf("%w: %v", ErrCompatibilityContract, err)
		}
	} else if capability.FallbackName != "" {
		return CompatibilityDecision{}, fmt.Errorf("%w: unavailable fallback has a name", ErrCompatibilityContract)
	}
	decision := CompatibilityDecision{Diagnostic: Diagnostic{
		Code: capability.Code, Severity: SeverityError, Stage: capability.Stage,
		Message: capability.Message, Location: capability.Location,
		Evidence: []DiagnosticEvidence{{Key: "compatibility_mode", Value: string(mode)}},
	}}
	if mode == CompatibilityFallbackOK && capability.FallbackAvailable {
		decision.UseWholeFragmentFallback = true
		decision.Diagnostic.Severity = SeverityWarning
		decision.Diagnostic.Evidence = append(decision.Diagnostic.Evidence,
			DiagnosticEvidence{Key: "whole_fragment_fallback", Value: capability.FallbackName})
	}
	if err := decision.Diagnostic.Validate(); err != nil {
		return CompatibilityDecision{}, fmt.Errorf("%w: diagnostic: %v", ErrCompatibilityContract, err)
	}
	return decision, nil
}
