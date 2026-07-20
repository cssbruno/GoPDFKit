// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

// Package paperformat provides a small deterministic presentation formatter.
// It supports exactly en-US, pt-BR, and ar using pinned local data. Every output
// is wrapped in a Unicode directional isolate so it can be embedded safely in
// surrounding left-to-right or right-to-left text.
package paperformat

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// FormatDataVersion changes whenever locale, currency, timezone, digit, or
// presentation patterns change.
const FormatDataVersion = "paperformat/2026-01"

const (
	leftToRightIsolate = "\u2066"
	rightToLeftIsolate = "\u2067"
	popIsolate         = "\u2069"
	nonBreakingSpace   = "\u00a0"
)

var (
	ErrInvalid   = errors.New("paperformat: invalid input")
	ErrLocale    = errors.New("paperformat: unsupported locale")
	ErrCurrency  = errors.New("paperformat: unsupported currency")
	ErrTimezone  = errors.New("paperformat: unsupported timezone")
	ErrPrecision = errors.New("paperformat: precision error")
	ErrLimit     = errors.New("paperformat: limit exceeded")
)

type Error struct {
	Operation string
	Field     string
	Problem   string
	Cause     error
}

func (e *Error) Error() string {
	location := e.Operation
	if e.Field != "" {
		location += "." + e.Field
	}
	return fmt.Sprintf("%s: %v: %s", location, e.Cause, e.Problem)
}

func (e *Error) Unwrap() error { return e.Cause }

type Limits struct {
	MaxInputBytes     uint32
	MaxOutputBytes    uint32
	MaxDigits         uint32
	MaxFractionDigits uint32
	MaxWork           uint64
}

func DefaultLimits() Limits {
	return Limits{
		MaxInputBytes:     4096,
		MaxOutputBytes:    8192,
		MaxDigits:         1024,
		MaxFractionDigits: 18,
		MaxWork:           16_384,
	}
}

// Precision preserves provided fractional digits, pads to MinFractionDigits,
// and rejects input beyond MaxFractionDigits. It never rounds silently.
type Precision struct {
	MinFractionDigits uint32
	MaxFractionDigits uint32
}

type TimeZoneKind uint8

const (
	FixedOffsetZone TimeZoneKind = iota + 1
	PinnedLocationZone
)

// TimeZonePolicy is always explicit. Fixed offsets accept -14:00 through
// +14:00. Pinned locations are a deliberately small, fixed-offset table; they
// do not consult host tzdata and do not model historical daylight transitions.
type TimeZonePolicy struct {
	Kind          TimeZoneKind
	OffsetMinutes int16
	Location      string
}

func FixedOffset(minutes int16) TimeZonePolicy {
	return TimeZonePolicy{Kind: FixedOffsetZone, OffsetMinutes: minutes}
}

func PinnedLocation(name string) TimeZonePolicy {
	return TimeZonePolicy{Kind: PinnedLocationZone, Location: name}
}

func FormatInteger(input, locale string, limits Limits) (string, error) {
	const operation = "integer"
	data, normalized, err := prepare(operation, input, locale, limits)
	if err != nil {
		return "", err
	}
	number, err := parseDecimal(operation, input, normalized)
	if err != nil {
		return "", err
	}
	if number.fraction != "" {
		return "", formatError(operation, "input", "integer input cannot contain a decimal fraction", ErrInvalid)
	}
	formatted := localizeNumber(number.negative, number.integer, "", data)
	return finish(operation, input, formatted, data, normalized)
}

func FormatDecimal(input, locale string, precision Precision, limits Limits) (string, error) {
	const operation = "decimal"
	data, normalized, err := prepare(operation, input, locale, limits)
	if err != nil {
		return "", err
	}
	if err := validatePrecision(operation, precision, normalized); err != nil {
		return "", err
	}
	number, err := parseDecimal(operation, input, normalized)
	if err != nil {
		return "", err
	}
	if uint32(len(number.fraction)) > precision.MaxFractionDigits { // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
		return "", formatError(operation, "precision", "input has more fractional digits than MaxFractionDigits; rounding is not implicit", ErrPrecision)
	}
	fraction := number.fraction
	for uint32(len(fraction)) < precision.MinFractionDigits { // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
		fraction += "0"
	}
	formatted := localizeNumber(number.negative, number.integer, fraction, data)
	return finish(operation, input, formatted, data, normalized)
}

func FormatCurrency(input, currency, locale string, limits Limits) (string, error) {
	const operation = "currency"
	data, normalized, err := prepare(operation, input, locale, limits)
	if err != nil {
		return "", err
	}
	currencyPattern, exists := currencies[currency]
	if !exists {
		return "", formatError(operation, "currency", fmt.Sprintf("ISO currency %q is not in %s", currency, FormatDataVersion), ErrCurrency)
	}
	if currency == "" || currency != strings.ToUpper(currency) {
		return "", formatError(operation, "currency", "currency must be an explicit uppercase ISO code", ErrCurrency)
	}
	if currencyPattern.fractionDigits > normalized.MaxFractionDigits {
		return "", formatError(operation, "currency", "currency precision exceeds MaxFractionDigits", ErrLimit)
	}
	number, err := parseDecimal(operation, input, normalized)
	if err != nil {
		return "", err
	}
	if uint32(len(number.fraction)) > currencyPattern.fractionDigits { // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
		return "", formatError(operation, "input", fmt.Sprintf("%s accepts at most %d fractional digits; rounding is not implicit", currency, currencyPattern.fractionDigits), ErrPrecision)
	}
	fraction := number.fraction
	for uint32(len(fraction)) < currencyPattern.fractionDigits { // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
		fraction += "0"
	}
	numberText := localizeNumber(number.negative, number.integer, fraction, data)
	symbol := currencyPattern.symbol(locale)
	formatted := formatCurrencyPattern(numberText, symbol.text, symbol.rtl, data)
	return finish(operation, input+currency, formatted, data, normalized)
}

func FormatDate(value time.Time, locale string, zone TimeZonePolicy, limits Limits) (string, error) {
	const operation = "date"
	data, normalized, err := prepare(operation, "", locale, limits)
	if err != nil {
		return "", err
	}
	localized, err := applyTimeZone(operation, value, zone)
	if err != nil {
		return "", err
	}
	if localized.Year() < 1 || localized.Year() > 9999 {
		return "", formatError(operation, "value", "date year must be between 0001 and 9999", ErrInvalid)
	}
	year := fourDigits(localized.Year())
	month := twoDigits(int(localized.Month()))
	day := twoDigits(localized.Day())
	var formatted string
	if data.dateMonthFirst {
		formatted = month + "/" + day + "/" + year
	} else {
		formatted = day + "/" + month + "/" + year
	}
	formatted = translateDigits(formatted, data.digits)
	return finish(operation, "", formatted, data, normalized)
}

func FormatTime(value time.Time, locale string, zone TimeZonePolicy, limits Limits) (string, error) {
	const operation = "time"
	data, normalized, err := prepare(operation, "", locale, limits)
	if err != nil {
		return "", err
	}
	localized, err := applyTimeZone(operation, value, zone)
	if err != nil {
		return "", err
	}
	hour := localized.Hour()
	suffix := ""
	if data.twelveHour {
		suffix = " AM"
		if hour >= 12 {
			suffix = " PM"
		}
		hour %= 12
		if hour == 0 {
			hour = 12
		}
	}
	formatted := twoDigits(hour) + ":" + twoDigits(localized.Minute()) + ":" + twoDigits(localized.Second()) + suffix
	formatted = translateDigits(formatted, data.digits)
	return finish(operation, "", formatted, data, normalized)
}

type decimalValue struct {
	negative bool
	integer  string
	fraction string
}

func parseDecimal(operation, input string, limits Limits) (decimalValue, error) {
	if input == "" {
		return decimalValue{}, formatError(operation, "input", "canonical decimal input is empty", ErrInvalid)
	}
	if !utf8.ValidString(input) {
		return decimalValue{}, formatError(operation, "input", "input is not valid UTF-8", ErrInvalid)
	}
	if uint32(len(input)) > limits.MaxInputBytes { // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
		return decimalValue{}, formatError(operation, "input", "input exceeds MaxInputBytes", ErrLimit)
	}
	value := decimalValue{}
	offset := 0
	if input[0] == '-' {
		value.negative = true
		offset++
	}
	integerStart := offset
	for offset < len(input) && input[offset] >= '0' && input[offset] <= '9' {
		offset++
	}
	if offset == integerStart {
		return decimalValue{}, formatError(operation, "input", "canonical decimal requires integer digits", ErrInvalid)
	}
	value.integer = input[integerStart:offset]
	if len(value.integer) > 1 && value.integer[0] == '0' {
		return decimalValue{}, formatError(operation, "input", "canonical decimal forbids leading integer zeroes", ErrInvalid)
	}
	if offset < len(input) {
		if input[offset] != '.' {
			return decimalValue{}, formatError(operation, "input", "canonical decimal permits only digits and one ASCII decimal point", ErrInvalid)
		}
		offset++
		fractionStart := offset
		for offset < len(input) && input[offset] >= '0' && input[offset] <= '9' {
			offset++
		}
		if offset == fractionStart {
			return decimalValue{}, formatError(operation, "input", "decimal point requires fractional digits", ErrInvalid)
		}
		value.fraction = input[fractionStart:offset]
	}
	if offset != len(input) {
		return decimalValue{}, formatError(operation, "input", "canonical decimal contains unsupported characters", ErrInvalid)
	}
	if uint64(len(value.integer))+uint64(len(value.fraction)) > uint64(limits.MaxDigits) {
		return decimalValue{}, formatError(operation, "input", "decimal digit count exceeds MaxDigits", ErrLimit)
	}
	if uint32(len(value.fraction)) > limits.MaxFractionDigits { // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
		return decimalValue{}, formatError(operation, "input", "fraction exceeds the formatter MaxFractionDigits", ErrLimit)
	}
	if value.negative && allZero(value.integer) && allZero(value.fraction) {
		return decimalValue{}, formatError(operation, "input", "canonical decimal forbids negative zero", ErrInvalid)
	}
	return value, nil
}

func validatePrecision(operation string, precision Precision, limits Limits) error {
	if precision.MinFractionDigits > precision.MaxFractionDigits {
		return formatError(operation, "precision", "minimum fraction digits exceed maximum", ErrPrecision)
	}
	if precision.MaxFractionDigits > limits.MaxFractionDigits {
		return formatError(operation, "precision", "fraction precision exceeds MaxFractionDigits", ErrLimit)
	}
	return nil
}

func localizeNumber(negative bool, integer, fraction string, data localeData) string {
	grouped := groupDigits(integer, data.group)
	if fraction != "" {
		grouped += data.decimal + fraction
	}
	if negative {
		grouped = "-" + grouped
	}
	return translateDigits(grouped, data.digits)
}

func groupDigits(integer, separator string) string {
	if len(integer) <= 3 {
		return integer
	}
	first := len(integer) % 3
	if first == 0 {
		first = 3
	}
	var result strings.Builder
	result.Grow(len(integer) + len(integer)/3*len(separator))
	result.WriteString(integer[:first])
	for offset := first; offset < len(integer); offset += 3 {
		result.WriteString(separator)
		result.WriteString(integer[offset : offset+3])
	}
	return result.String()
}

func translateDigits(value string, digits [10]string) string {
	if digits[0] == "0" {
		return value
	}
	var result strings.Builder
	result.Grow(len(value) * 2)
	for _, character := range value {
		if character >= '0' && character <= '9' {
			result.WriteString(digits[character-'0'])
		} else {
			result.WriteRune(character)
		}
	}
	return result.String()
}

func formatCurrencyPattern(number, symbol string, symbolRTL bool, data localeData) string {
	if data.rtl {
		if !symbolRTL {
			symbol = leftToRightIsolate + symbol + popIsolate
		}
		return number + nonBreakingSpace + symbol
	}
	if symbolRTL {
		symbol = rightToLeftIsolate + symbol + popIsolate
		return symbol + nonBreakingSpace + number
	}
	if data.currencyAfter {
		return number + nonBreakingSpace + symbol
	}
	if data.currencySpace {
		return symbol + nonBreakingSpace + number
	}
	return symbol + number
}

func finish(operation, input, formatted string, data localeData, limits Limits) (string, error) {
	prefix := leftToRightIsolate
	if data.rtl {
		prefix = rightToLeftIsolate
	}
	output := prefix + formatted + popIsolate
	if uint64(len(output)) > uint64(limits.MaxOutputBytes) {
		return "", formatError(operation, "output", "formatted output exceeds MaxOutputBytes", ErrLimit)
	}
	work := uint64(len(input)) + uint64(len(formatted)) + uint64(utf8.RuneCountInString(formatted)) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
	if work > limits.MaxWork {
		return "", formatError(operation, "work", "formatting exceeds MaxWork", ErrLimit)
	}
	return output, nil
}

func prepare(operation, input, locale string, limits Limits) (localeData, Limits, error) {
	normalized, err := normalizeLimits(limits)
	if err != nil {
		return localeData{}, Limits{}, formatError(operation, "limits", "limits are incomplete or exceed hard caps", ErrLimit)
	}
	if uint64(len(input)) > uint64(normalized.MaxInputBytes) {
		return localeData{}, Limits{}, formatError(operation, "input", "input exceeds MaxInputBytes", ErrLimit)
	}
	data, exists := locales[locale]
	if !exists {
		return localeData{}, Limits{}, formatError(operation, "locale", fmt.Sprintf("locale %q is not in %s", locale, FormatDataVersion), ErrLocale)
	}
	return data, normalized, nil
}

func normalizeLimits(limits Limits) (Limits, error) {
	if limits == (Limits{}) {
		return DefaultLimits(), nil
	}
	hard := DefaultLimits()
	if limits.MaxInputBytes == 0 || limits.MaxInputBytes > hard.MaxInputBytes ||
		limits.MaxOutputBytes == 0 || limits.MaxOutputBytes > hard.MaxOutputBytes ||
		limits.MaxDigits == 0 || limits.MaxDigits > hard.MaxDigits ||
		limits.MaxFractionDigits == 0 || limits.MaxFractionDigits > hard.MaxFractionDigits ||
		limits.MaxWork == 0 || limits.MaxWork > hard.MaxWork {
		return Limits{}, ErrLimit
	}
	return limits, nil
}

func applyTimeZone(operation string, value time.Time, policy TimeZonePolicy) (time.Time, error) {
	var offset int16
	switch policy.Kind {
	case FixedOffsetZone:
		if policy.Location != "" || policy.OffsetMinutes < -14*60 || policy.OffsetMinutes > 14*60 {
			return time.Time{}, formatError(operation, "timezone", "fixed offset must be explicit, location-free, and within -14:00 through +14:00", ErrTimezone)
		}
		offset = policy.OffsetMinutes
	case PinnedLocationZone:
		if policy.OffsetMinutes != 0 {
			return time.Time{}, formatError(operation, "timezone", "pinned location cannot also specify an offset", ErrTimezone)
		}
		var exists bool
		offset, exists = pinnedLocations[policy.Location]
		if !exists {
			return time.Time{}, formatError(operation, "timezone", fmt.Sprintf("location %q is not in %s", policy.Location, FormatDataVersion), ErrTimezone)
		}
	default:
		return time.Time{}, formatError(operation, "timezone", "timezone policy kind is required", ErrTimezone)
	}
	return value.UTC().Add(time.Duration(offset) * time.Minute), nil
}

func allZero(value string) bool {
	for _, character := range value {
		if character != '0' {
			return false
		}
	}
	return true
}

func twoDigits(value int) string {
	return string([]byte{'0' + byte(value/10), '0' + byte(value%10)}) // #nosec G115 -- low-width representation is explicitly normalized before packing
}

func fourDigits(value int) string {
	return string([]byte{
		'0' + byte(value/1000%10), // #nosec G115 -- low-width representation is explicitly normalized before packing
		'0' + byte(value/100%10),  // #nosec G115 -- low-width representation is explicitly normalized before packing
		'0' + byte(value/10%10),   // #nosec G115 -- low-width representation is explicitly normalized before packing
		'0' + byte(value%10),      // #nosec G115 -- low-width representation is explicitly normalized before packing
	})
}

func formatError(operation, field, problem string, cause error) error {
	return &Error{Operation: operation, Field: field, Problem: problem, Cause: cause}
}

type localeData struct {
	digits         [10]string
	group          string
	decimal        string
	rtl            bool
	dateMonthFirst bool
	twelveHour     bool
	currencyAfter  bool
	currencySpace  bool
}

var locales = map[string]localeData{
	"en-US": {
		digits:         latinDigits,
		group:          ",",
		decimal:        ".",
		dateMonthFirst: true,
		twelveHour:     true,
	},
	"pt-BR": {
		digits:        latinDigits,
		group:         ".",
		decimal:       ",",
		currencySpace: true,
	},
	"ar": {
		digits:        arabicIndicDigits,
		group:         "\u066c",
		decimal:       "\u066b",
		rtl:           true,
		currencyAfter: true,
	},
}

var latinDigits = [10]string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9"}
var arabicIndicDigits = [10]string{"٠", "١", "٢", "٣", "٤", "٥", "٦", "٧", "٨", "٩"}

type currencySymbol struct {
	text string
	rtl  bool
}

type currencyData struct {
	fractionDigits uint32
	enUS           currencySymbol
	ptBR           currencySymbol
	ar             currencySymbol
}

func (c currencyData) symbol(locale string) currencySymbol {
	switch locale {
	case "pt-BR":
		return c.ptBR
	case "ar":
		return c.ar
	default:
		return c.enUS
	}
}

var currencies = map[string]currencyData{
	"USD": {fractionDigits: 2, enUS: currencySymbol{text: "$"}, ptBR: currencySymbol{text: "US$"}, ar: currencySymbol{text: "US$"}},
	"BRL": {fractionDigits: 2, enUS: currencySymbol{text: "R$"}, ptBR: currencySymbol{text: "R$"}, ar: currencySymbol{text: "R$"}},
	"EUR": {fractionDigits: 2, enUS: currencySymbol{text: "€"}, ptBR: currencySymbol{text: "€"}, ar: currencySymbol{text: "€"}},
	"SAR": {fractionDigits: 2, enUS: currencySymbol{text: "ر.س", rtl: true}, ptBR: currencySymbol{text: "ر.س", rtl: true}, ar: currencySymbol{text: "ر.س", rtl: true}},
}

var pinnedLocations = map[string]int16{
	"UTC":               0,
	"America/Fortaleza": -3 * 60,
	"Asia/Riyadh":       3 * 60,
}
