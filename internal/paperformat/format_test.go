// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperformat

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestFormatNumberGolden(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		call func() (string, error)
		want string
	}{
		{
			name: "en integer",
			call: func() (string, error) { return FormatInteger("1234567", "en-US", Limits{}) },
			want: "\u20661,234,567\u2069",
		},
		{
			name: "pt integer",
			call: func() (string, error) { return FormatInteger("-1234567", "pt-BR", Limits{}) },
			want: "\u2066-1.234.567\u2069",
		},
		{
			name: "ar integer",
			call: func() (string, error) { return FormatInteger("1234567", "ar", Limits{}) },
			want: "\u2067١٬٢٣٤٬٥٦٧\u2069",
		},
		{
			name: "en padded decimal",
			call: func() (string, error) {
				return FormatDecimal("1234.5", "en-US", Precision{MinFractionDigits: 2, MaxFractionDigits: 4}, Limits{})
			},
			want: "\u20661,234.50\u2069",
		},
		{
			name: "pt preserved decimal",
			call: func() (string, error) {
				return FormatDecimal("1234.500", "pt-BR", Precision{MaxFractionDigits: 3}, Limits{})
			},
			want: "\u20661.234,500\u2069",
		},
		{
			name: "ar decimal",
			call: func() (string, error) {
				return FormatDecimal("-1234.5", "ar", Precision{MinFractionDigits: 2, MaxFractionDigits: 2}, Limits{})
			},
			want: "\u2067-١٬٢٣٤٫٥٠\u2069",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := test.call()
			if err != nil || got != test.want {
				t.Fatalf("got %q, %v; want %q", got, err, test.want)
			}
		})
	}
}

func TestFormatCurrencyGoldenAndBidiIsolation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		locale   string
		currency string
		want     string
	}{
		{locale: "en-US", currency: "USD", want: "\u2066$1,234.50\u2069"},
		{locale: "pt-BR", currency: "BRL", want: "\u2066R$\u00a01.234,50\u2069"},
		{locale: "ar", currency: "SAR", want: "\u2067١٬٢٣٤٫٥٠\u00a0ر.س\u2069"},
		{locale: "ar", currency: "USD", want: "\u2067١٬٢٣٤٫٥٠\u00a0\u2066US$\u2069\u2069"},
		{locale: "en-US", currency: "SAR", want: "\u2066\u2067ر.س\u2069\u00a01,234.50\u2069"},
	}
	for _, test := range tests {
		t.Run(test.locale+"/"+test.currency, func(t *testing.T) {
			t.Parallel()
			got, err := FormatCurrency("1234.5", test.currency, test.locale, Limits{})
			if err != nil || got != test.want {
				t.Fatalf("got %q, %v; want %q", got, err, test.want)
			}
		})
	}
}

func TestFormatDateAndTimeGoldenWithExplicitZones(t *testing.T) {
	t.Parallel()

	instant := time.Date(2026, time.July, 16, 23, 30, 5, 987, time.UTC)
	tests := []struct {
		name string
		call func() (string, error)
		want string
	}{
		{
			name: "en date fixed rollover",
			call: func() (string, error) { return FormatDate(instant, "en-US", FixedOffset(120), Limits{}) },
			want: "\u206607/17/2026\u2069",
		},
		{
			name: "en time fixed rollover",
			call: func() (string, error) { return FormatTime(instant, "en-US", FixedOffset(120), Limits{}) },
			want: "\u206601:30:05 AM\u2069",
		},
		{
			name: "pt date pinned location",
			call: func() (string, error) {
				return FormatDate(instant, "pt-BR", PinnedLocation("America/Fortaleza"), Limits{})
			},
			want: "\u206616/07/2026\u2069",
		},
		{
			name: "pt time pinned location",
			call: func() (string, error) {
				return FormatTime(instant, "pt-BR", PinnedLocation("America/Fortaleza"), Limits{})
			},
			want: "\u206620:30:05\u2069",
		},
		{
			name: "ar date pinned location",
			call: func() (string, error) { return FormatDate(instant, "ar", PinnedLocation("Asia/Riyadh"), Limits{}) },
			want: "\u2067١٧/٠٧/٢٠٢٦\u2069",
		},
		{
			name: "ar time pinned location",
			call: func() (string, error) { return FormatTime(instant, "ar", PinnedLocation("Asia/Riyadh"), Limits{}) },
			want: "\u2067٠٢:٣٠:٠٥\u2069",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := test.call()
			if err != nil || got != test.want {
				t.Fatalf("got %q, %v; want %q", got, err, test.want)
			}
		})
	}
}

func TestFormattingIsIndependentOfInputLocationAndDeterministic(t *testing.T) {
	t.Parallel()

	inputZone := time.FixedZone("caller-zone", 9*60*60)
	value := time.Date(2026, time.July, 17, 8, 0, 0, 0, inputZone) // 2026-07-16 23:00 UTC.
	first, err := FormatDate(value, "en-US", PinnedLocation("UTC"), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := FormatDate(value, "en-US", PinnedLocation("UTC"), Limits{})
	if err != nil || first != second || first != "\u206607/16/2026\u2069" {
		t.Fatalf("results = %q, %q, %v", first, second, err)
	}
	if FormatDataVersion != "paperformat/2026-01" {
		t.Fatalf("unexpected data version %q", FormatDataVersion)
	}
}

func TestRejectsInvalidCanonicalNumbers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		problem string
	}{
		{name: "empty", input: "", problem: "empty"},
		{name: "positive sign", input: "+1", problem: "integer digits"},
		{name: "leading zero", input: "01", problem: "leading"},
		{name: "negative zero", input: "-0", problem: "negative zero"},
		{name: "negative decimal zero", input: "-0.00", problem: "negative zero"},
		{name: "empty fraction", input: "1.", problem: "fractional digits"},
		{name: "exponent", input: "1e2", problem: "decimal"},
		{name: "localized input", input: "١٢", problem: "integer digits"},
		{name: "multiple points", input: "1.2.3", problem: "characters"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := FormatDecimal(test.input, "en-US", Precision{MaxFractionDigits: 2}, Limits{})
			assertFormatError(t, err, ErrInvalid, test.problem)
		})
	}

	_, err := FormatInteger("1.0", "en-US", Limits{})
	assertFormatError(t, err, ErrInvalid, "cannot contain")
	_, err = FormatDecimal("1.234", "en-US", Precision{MaxFractionDigits: 2}, Limits{})
	assertFormatError(t, err, ErrPrecision, "rounding")
	_, err = FormatCurrency("1.234", "USD", "en-US", Limits{})
	assertFormatError(t, err, ErrPrecision, "rounding")
}

func TestRejectsUnknownLocaleCurrencyAndTimezone(t *testing.T) {
	t.Parallel()

	_, err := FormatInteger("1", "en", Limits{})
	assertFormatError(t, err, ErrLocale, "not in")
	_, err = FormatCurrency("1", "JPY", "en-US", Limits{})
	assertFormatError(t, err, ErrCurrency, "JPY")
	_, err = FormatCurrency("1", "usd", "en-US", Limits{})
	assertFormatError(t, err, ErrCurrency, "usd")

	instant := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err = FormatDate(instant, "en-US", TimeZonePolicy{}, Limits{})
	assertFormatError(t, err, ErrTimezone, "kind")
	_, err = FormatDate(instant, "en-US", FixedOffset(14*60+1), Limits{})
	assertFormatError(t, err, ErrTimezone, "within")
	_, err = FormatDate(instant, "en-US", TimeZonePolicy{Kind: FixedOffsetZone, Location: "UTC"}, Limits{})
	assertFormatError(t, err, ErrTimezone, "location-free")
	_, err = FormatDate(instant, "en-US", PinnedLocation("Europe/London"), Limits{})
	assertFormatError(t, err, ErrTimezone, "not in")
	_, err = FormatDate(instant, "en-US", TimeZonePolicy{Kind: PinnedLocationZone, Location: "UTC", OffsetMinutes: 1}, Limits{})
	assertFormatError(t, err, ErrTimezone, "cannot also")
}

func TestFormattingLimitsAndPrecision(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		limits  func() Limits
		call    func(Limits) (string, error)
		problem string
	}{
		{
			name: "input", limits: func() Limits { value := DefaultLimits(); value.MaxInputBytes = 3; return value },
			call: func(limits Limits) (string, error) { return FormatInteger("1234", "en-US", limits) }, problem: "MaxInputBytes",
		},
		{
			name: "output", limits: func() Limits { value := DefaultLimits(); value.MaxOutputBytes = 4; return value },
			call: func(limits Limits) (string, error) { return FormatInteger("1", "en-US", limits) }, problem: "MaxOutputBytes",
		},
		{
			name: "digits", limits: func() Limits { value := DefaultLimits(); value.MaxDigits = 3; return value },
			call: func(limits Limits) (string, error) { return FormatInteger("1234", "en-US", limits) }, problem: "MaxDigits",
		},
		{
			name: "fraction", limits: func() Limits { value := DefaultLimits(); value.MaxFractionDigits = 1; return value },
			call: func(limits Limits) (string, error) {
				return FormatDecimal("1.23", "en-US", Precision{MaxFractionDigits: 1}, limits)
			}, problem: "formatter MaxFractionDigits",
		},
		{
			name: "work", limits: func() Limits { value := DefaultLimits(); value.MaxWork = 2; return value },
			call: func(limits Limits) (string, error) { return FormatInteger("1", "en-US", limits) }, problem: "MaxWork",
		},
		{
			name: "currency precision limit", limits: func() Limits { value := DefaultLimits(); value.MaxFractionDigits = 1; return value },
			call: func(limits Limits) (string, error) { return FormatCurrency("1", "USD", "en-US", limits) }, problem: "currency precision",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := test.call(test.limits())
			assertFormatError(t, err, ErrLimit, test.problem)
		})
	}

	_, err := FormatInteger("1", "en-US", Limits{MaxDigits: 1})
	assertFormatError(t, err, ErrLimit, "limits")
	_, err = FormatDecimal("1", "en-US", Precision{MinFractionDigits: 3, MaxFractionDigits: 2}, Limits{})
	assertFormatError(t, err, ErrPrecision, "minimum")
	_, err = FormatDecimal("1", "en-US", Precision{MaxFractionDigits: DefaultLimits().MaxFractionDigits + 1}, Limits{})
	assertFormatError(t, err, ErrLimit, "precision")

	tooLate := time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err = FormatDate(tooLate, "en-US", PinnedLocation("UTC"), Limits{})
	assertFormatError(t, err, ErrInvalid, "year")
}

func TestRejectsInvalidUTF8(t *testing.T) {
	t.Parallel()

	_, err := FormatInteger(string([]byte{0xff}), "en-US", Limits{})
	assertFormatError(t, err, ErrInvalid, "UTF-8")
}

func assertFormatError(t *testing.T, err, cause error, problem string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected %v containing %q", cause, problem)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("error %v does not wrap %v", err, cause)
	}
	var typed *Error
	if !errors.As(err, &typed) || typed.Operation == "" || typed.Field == "" {
		t.Fatalf("error is not fully typed: %#v", err)
	}
	if !strings.Contains(err.Error(), problem) {
		t.Fatalf("error %q does not contain %q", err, problem)
	}
}
