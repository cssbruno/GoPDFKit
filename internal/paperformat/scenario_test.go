// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperformat

import (
	"errors"
	"strings"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/paperscenario"
)

func TestFormatValueCoversClosedScalarFormats(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value paperscenario.Value
		spec  FormatSpec
		want  string
	}{
		{
			name:  "string",
			value: paperscenario.Value{Kind: paperscenario.String, String: "Ada"},
			spec:  FormatSpec{Kind: ValueFormatString},
			want:  "\u2066Ada\u2069",
		},
		{
			name:  "bool true",
			value: paperscenario.Value{Kind: paperscenario.Bool, Bool: true},
			spec:  FormatSpec{Kind: ValueFormatBool},
			want:  "\u2066true\u2069",
		},
		{
			name:  "bool false",
			value: paperscenario.Value{Kind: paperscenario.Bool},
			spec:  FormatSpec{Kind: ValueFormatBool},
			want:  "\u2066false\u2069",
		},
		{
			name:  "integer",
			value: paperscenario.Value{Kind: paperscenario.Number, Number: "1234567"},
			spec:  FormatSpec{Kind: ValueFormatInteger, Locale: "pt-BR"},
			want:  "\u20661.234.567\u2069",
		},
		{
			name:  "decimal",
			value: paperscenario.Value{Kind: paperscenario.Number, Number: "1234.5"},
			spec:  FormatSpec{Kind: ValueFormatDecimal, Locale: "ar", Precision: Precision{MinFractionDigits: 2, MaxFractionDigits: 2}},
			want:  "\u2067١٬٢٣٤٫٥٠\u2069",
		},
		{
			name:  "currency",
			value: paperscenario.Value{Kind: paperscenario.Number, Number: "1234.5"},
			spec:  FormatSpec{Kind: ValueFormatCurrency, Locale: "en-US", Currency: "USD"},
			want:  "\u2066$1,234.50\u2069",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			first, err := FormatValue(test.value, test.spec)
			if err != nil || first != test.want {
				t.Fatalf("FormatValue() = %q, %v; want %q", first, err, test.want)
			}
			second, err := FormatValue(test.value, test.spec)
			if err != nil || second != first {
				t.Fatalf("second FormatValue() = %q, %v; first %q", second, err, first)
			}
		})
	}
}

func TestFormatValueBareOutputOmitsOnlyTheOuterDirectionalIsolate(t *testing.T) {
	t.Parallel()
	got, err := FormatValue(
		paperscenario.Value{Kind: paperscenario.Number, Number: "1234.5"},
		FormatSpec{Kind: ValueFormatCurrency, Locale: "en-US", Currency: "USD", Output: ValueOutputBare},
	)
	if err != nil || got != "$1,234.50" {
		t.Fatalf("FormatValue(bare) = %q, %v", got, err)
	}
	_, err = FormatValue(paperscenario.Value{Kind: paperscenario.String, String: "x"}, FormatSpec{Kind: ValueFormatString, Output: "raw-ish"})
	assertFormatError(t, err, ErrInvalid, "output mode")
}

func TestFormatValueRequiresExactKindsAndValidScenarioScalars(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value paperscenario.Value
		spec  FormatSpec
	}{
		{
			name:  "no string coercion to number",
			value: paperscenario.Value{Kind: paperscenario.String, String: "12"},
			spec:  FormatSpec{Kind: ValueFormatInteger, Locale: "en-US"},
		},
		{
			name:  "no number coercion to string",
			value: paperscenario.Value{Kind: paperscenario.Number, Number: "12"},
			spec:  FormatSpec{Kind: ValueFormatString},
		},
		{
			name:  "null unsupported",
			value: paperscenario.Value{Kind: paperscenario.Null},
			spec:  FormatSpec{Kind: ValueFormatString},
		},
		{
			name:  "object unsupported",
			value: paperscenario.Value{Kind: paperscenario.Object},
			spec:  FormatSpec{Kind: ValueFormatString},
		},
		{
			name:  "malformed number",
			value: paperscenario.Value{Kind: paperscenario.Number, Number: "12", String: "extra"},
			spec:  FormatSpec{Kind: ValueFormatDecimal, Locale: "en-US", Precision: Precision{MaxFractionDigits: 2}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := FormatValue(test.value, test.spec)
			assertFormatError(t, err, ErrInvalid, "requires")
		})
	}

	_, err := FormatValue(
		paperscenario.Value{Kind: paperscenario.Number, Number: "1.0"},
		FormatSpec{Kind: ValueFormatInteger, Locale: "en-US"},
	)
	assertFormatError(t, err, ErrInvalid, "cannot contain")
}

func TestFormatValueRequiresExplicitRelevantSpecFields(t *testing.T) {
	t.Parallel()

	number := paperscenario.Value{Kind: paperscenario.Number, Number: "1"}
	tests := []struct {
		name    string
		spec    FormatSpec
		cause   error
		problem string
	}{
		{name: "kind", spec: FormatSpec{}, cause: ErrInvalid, problem: "kind is required"},
		{name: "unknown kind", spec: FormatSpec{Kind: "scientific"}, cause: ErrInvalid, problem: "unsupported"},
		{name: "integer locale", spec: FormatSpec{Kind: ValueFormatInteger}, cause: ErrLocale, problem: "explicit"},
		{name: "decimal locale", spec: FormatSpec{Kind: ValueFormatDecimal, Precision: Precision{MaxFractionDigits: 2}}, cause: ErrLocale, problem: "explicit"},
		{name: "currency locale", spec: FormatSpec{Kind: ValueFormatCurrency, Currency: "USD"}, cause: ErrLocale, problem: "explicit"},
		{name: "currency code", spec: FormatSpec{Kind: ValueFormatCurrency, Locale: "en-US"}, cause: ErrCurrency, problem: "explicit"},
		{name: "decimal currency", spec: FormatSpec{Kind: ValueFormatDecimal, Locale: "en-US", Currency: "USD"}, cause: ErrInvalid, problem: "does not accept"},
		{name: "currency precision", spec: FormatSpec{Kind: ValueFormatCurrency, Locale: "en-US", Currency: "USD", Precision: Precision{MaxFractionDigits: 2}}, cause: ErrInvalid, problem: "pinned"},
		{name: "string locale", spec: FormatSpec{Kind: ValueFormatString, Locale: "en-US"}, cause: ErrInvalid, problem: "do not accept"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := FormatValue(number, test.spec)
			assertFormatError(t, err, test.cause, test.problem)
		})
	}

	_, err := FormatValue(number, FormatSpec{Kind: ValueFormatDecimal, Locale: "en", Precision: Precision{MaxFractionDigits: 2}})
	assertFormatError(t, err, ErrLocale, "not in")
	_, err = FormatValue(number, FormatSpec{Kind: ValueFormatCurrency, Locale: "en-US", Currency: "usd"})
	assertFormatError(t, err, ErrCurrency, "usd")
}

func TestFormatValueAppliesBoundsToPlainAndNumericValues(t *testing.T) {
	t.Parallel()

	limits := DefaultLimits()
	limits.MaxInputBytes = 3
	_, err := FormatValue(
		paperscenario.Value{Kind: paperscenario.String, String: "four"},
		FormatSpec{Kind: ValueFormatString, Limits: limits},
	)
	assertFormatError(t, err, ErrLimit, "MaxInputBytes")

	limits = DefaultLimits()
	limits.MaxOutputBytes = 8
	_, err = FormatValue(
		paperscenario.Value{Kind: paperscenario.Bool, Bool: true},
		FormatSpec{Kind: ValueFormatBool, Limits: limits},
	)
	assertFormatError(t, err, ErrLimit, "MaxOutputBytes")

	limits = DefaultLimits()
	limits.MaxDigits = 2
	_, err = FormatValue(
		paperscenario.Value{Kind: paperscenario.Number, Number: "123"},
		FormatSpec{Kind: ValueFormatInteger, Locale: "en-US", Limits: limits},
	)
	assertFormatError(t, err, ErrLimit, "MaxDigits")

	_, err = FormatValue(
		paperscenario.Value{Kind: paperscenario.String, String: "ok"},
		FormatSpec{Kind: ValueFormatString, Limits: Limits{MaxInputBytes: 2}},
	)
	assertFormatError(t, err, ErrLimit, "limits")
}

func TestFormatValueRejectsInvalidUTF8String(t *testing.T) {
	t.Parallel()

	_, err := FormatValue(
		paperscenario.Value{Kind: paperscenario.String, String: string([]byte{0xff})},
		FormatSpec{Kind: ValueFormatString},
	)
	assertFormatError(t, err, ErrInvalid, "UTF-8")
}

func TestFormatValueErrorsRemainTyped(t *testing.T) {
	t.Parallel()

	_, err := FormatValue(paperscenario.Value{Kind: paperscenario.List}, FormatSpec{Kind: ValueFormatBool})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("error %v does not wrap ErrInvalid", err)
	}
	var typed *Error
	if !errors.As(err, &typed) || typed.Operation != "scenario" || !strings.HasPrefix(typed.Field, "value") {
		t.Fatalf("error is not stable and typed: %#v", err)
	}
}
