// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package paperlang

// DiagnosticSeverity is a stable diagnostic category.
type DiagnosticSeverity string

const (
	SeverityError   DiagnosticSeverity = "error"
	SeverityWarning DiagnosticSeverity = "warning"
)

// Diagnostic is a source-anchored parser or lexer problem suitable for an
// editor. Hint is optional and should describe a concrete correction.
type Diagnostic struct {
	Code     string             `json:"code"`
	Severity DiagnosticSeverity `json:"severity"`
	Message  string             `json:"message"`
	Hint     string             `json:"hint,omitempty"`
	Span     Span               `json:"span"`
}

func errorDiagnostic(code, message, hint string, span Span) Diagnostic {
	return Diagnostic{Code: code, Severity: SeverityError, Message: message, Hint: hint, Span: span}
}
