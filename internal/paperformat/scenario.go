// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperformat

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/cssbruno/gopdfkit/internal/paperscenario"
)

// ValueFormatKind is the closed set of scenario-value presentations supported
// by FormatValue. Formatting never coerces one scenario kind into another.
type ValueFormatKind string

// ValueOutputMode controls whether FormatValue returns a standalone visual
// token or a token protected for embedding in surrounding bidirectional text.
// The zero value is the safer isolated form.
type ValueOutputMode string

const (
	ValueFormatString   ValueFormatKind = "string"
	ValueFormatBool     ValueFormatKind = "bool"
	ValueFormatInteger  ValueFormatKind = "integer"
	ValueFormatDecimal  ValueFormatKind = "decimal"
	ValueFormatCurrency ValueFormatKind = "currency"

	ValueOutputIsolated ValueOutputMode = ""
	ValueOutputBare     ValueOutputMode = "bare"
)

// FormatSpec describes one deterministic, bounded scenario-value formatting
// operation. Locale is required for numeric presentation. Currency is required
// only for ValueFormatCurrency. Decimal precision must be explicit; the zero
// precision accepts numbers without a fraction. A zero Limits value selects
// DefaultLimits, which remains bounded by the package's pinned hard caps.
type FormatSpec struct {
	Kind      ValueFormatKind `json:"kind"`
	Locale    string          `json:"locale,omitempty"`
	Currency  string          `json:"currency,omitempty"`
	Precision Precision       `json:"precision,omitempty"`
	Limits    Limits          `json:"limits,omitempty"`
	Output    ValueOutputMode `json:"output,omitempty"`
}

// FormatValue formats one resolved scenario scalar without consulting ambient
// locale, timezone, clock, environment, or process state. Object, list, and
// null values are intentionally unsupported at this presentation boundary.
func FormatValue(value paperscenario.Value, spec FormatSpec) (string, error) {
	limits, err := validateFormatSpec(spec)
	if err != nil {
		return "", err
	}
	var formatted string
	switch spec.Kind {
	case ValueFormatString:
		if value.Kind != paperscenario.String || value.Number != "" || value.Bool || len(value.Object) != 0 || len(value.List) != 0 {
			return "", scenarioFormatError("value", value.Kind, paperscenario.String)
		}
		formatted, err = formatScenarioText(value.String, limits)
	case ValueFormatBool:
		if value.Kind != paperscenario.Bool || value.String != "" || value.Number != "" || len(value.Object) != 0 || len(value.List) != 0 {
			return "", scenarioFormatError("value", value.Kind, paperscenario.Bool)
		}
		if value.Bool {
			formatted, err = formatScenarioText("true", limits)
		} else {
			formatted, err = formatScenarioText("false", limits)
		}
	case ValueFormatInteger:
		var number string
		number, err = scenarioNumber(value)
		if err != nil {
			return "", err
		}
		formatted, err = FormatInteger(number, spec.Locale, limits)
	case ValueFormatDecimal:
		var number string
		number, err = scenarioNumber(value)
		if err != nil {
			return "", err
		}
		formatted, err = FormatDecimal(number, spec.Locale, spec.Precision, limits)
	case ValueFormatCurrency:
		var number string
		number, err = scenarioNumber(value)
		if err != nil {
			return "", err
		}
		formatted, err = FormatCurrency(number, spec.Currency, spec.Locale, limits)
	default:
		return "", formatError("scenario", "spec.kind", fmt.Sprintf("format kind %q is unsupported", spec.Kind), ErrInvalid)
	}
	if err != nil {
		return "", err
	}
	if spec.Output == ValueOutputBare {
		return stripOuterDirectionalIsolate(formatted), nil
	}
	return formatted, nil
}

func validateFormatSpec(spec FormatSpec) (Limits, error) {
	limits, err := normalizeLimits(spec.Limits)
	if err != nil {
		return Limits{}, formatError("scenario", "spec.limits", "limits are incomplete or exceed hard caps", ErrLimit)
	}
	if spec.Output != ValueOutputIsolated && spec.Output != ValueOutputBare {
		return Limits{}, formatError("scenario", "spec.output", fmt.Sprintf("output mode %q is unsupported", spec.Output), ErrInvalid)
	}
	switch spec.Kind {
	case ValueFormatString, ValueFormatBool:
		if spec.Locale != "" || spec.Currency != "" || spec.Precision != (Precision{}) {
			return Limits{}, formatError("scenario", "spec", "string and bool formats do not accept locale, currency, or precision", ErrInvalid)
		}
	case ValueFormatInteger:
		if spec.Locale == "" {
			return Limits{}, formatError("scenario", "spec.locale", "integer format requires an explicit supported locale", ErrLocale)
		}
		if spec.Currency != "" || spec.Precision != (Precision{}) {
			return Limits{}, formatError("scenario", "spec", "integer format does not accept currency or precision", ErrInvalid)
		}
	case ValueFormatDecimal:
		if spec.Locale == "" {
			return Limits{}, formatError("scenario", "spec.locale", "decimal format requires an explicit supported locale", ErrLocale)
		}
		if spec.Currency != "" {
			return Limits{}, formatError("scenario", "spec.currency", "decimal format does not accept currency", ErrInvalid)
		}
	case ValueFormatCurrency:
		if spec.Locale == "" {
			return Limits{}, formatError("scenario", "spec.locale", "currency format requires an explicit supported locale", ErrLocale)
		}
		if spec.Currency == "" {
			return Limits{}, formatError("scenario", "spec.currency", "currency format requires an explicit supported currency", ErrCurrency)
		}
		if spec.Precision != (Precision{}) {
			return Limits{}, formatError("scenario", "spec.precision", "currency precision is pinned by currency data", ErrInvalid)
		}
	case "":
		return Limits{}, formatError("scenario", "spec.kind", "format kind is required", ErrInvalid)
	default:
		return Limits{}, formatError("scenario", "spec.kind", fmt.Sprintf("format kind %q is unsupported", spec.Kind), ErrInvalid)
	}
	return limits, nil
}

func stripOuterDirectionalIsolate(value string) string {
	if strings.HasSuffix(value, popIsolate) && (strings.HasPrefix(value, leftToRightIsolate) || strings.HasPrefix(value, rightToLeftIsolate)) {
		return strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(value, leftToRightIsolate), rightToLeftIsolate), popIsolate)
	}
	return value
}

func scenarioNumber(value paperscenario.Value) (string, error) {
	if value.Kind != paperscenario.Number || value.String != "" || value.Bool || len(value.Object) != 0 || len(value.List) != 0 {
		return "", scenarioFormatError("value", value.Kind, paperscenario.Number)
	}
	return value.Number, nil
}

func scenarioFormatError(field string, got, want paperscenario.Kind) error {
	return formatError("scenario", field, fmt.Sprintf("format requires a valid %s scenario value; got %q", want, got), ErrInvalid)
}

func formatScenarioText(value string, limits Limits) (string, error) {
	if !utf8.ValidString(value) {
		return "", formatError("scenario", "value", "text is not valid UTF-8", ErrInvalid)
	}
	if uint64(len(value)) > uint64(limits.MaxInputBytes) {
		return "", formatError("scenario", "value", "text exceeds MaxInputBytes", ErrLimit)
	}
	return finish("scenario", value, value, locales["en-US"], limits)
}
