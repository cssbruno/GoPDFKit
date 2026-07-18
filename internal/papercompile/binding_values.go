// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package papercompile

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/cssbruno/gopdfkit/internal/paperformat"
	"github.com/cssbruno/gopdfkit/internal/paperlang"
	"github.com/cssbruno/gopdfkit/internal/paperscenario"
)

// bindingText evaluates a validated primitive binding against the explicitly
// selected fixture. Evaluation is deliberately part of scenario compilation:
// ordinary Compile keeps authored placeholders and never selects ambient data.
func (c *compiler) bindingText(node *paperlang.Node, binding bindingMetadata) (string, bool) {
	value, found, problem := c.lookupBindingValue(node, binding.path)
	if problem != "" {
		c.add("PAPER_BIND_VALUE", problem, "make the selected fixture match the declared schema and binding path", binding.span)
		return "", false
	}
	if !found {
		if binding.required {
			c.add("PAPER_BIND_VALUE_MISSING", fmt.Sprintf("selected scenario @%s has no value for required binding %s", c.fixture.Name, binding.path), "add the required fixture value or set bind-required: false", binding.span)
			return "", false
		}
		return "", true
	}
	if value.Kind == paperscenario.Null {
		if binding.required {
			c.add("PAPER_BIND_VALUE_NULL", fmt.Sprintf("selected scenario @%s supplies null for required binding %s", c.fixture.Name, binding.path), "supply a primitive value or set bind-required: false", binding.span)
			return "", false
		}
		return "", true
	}
	if !bindingKindMatches(binding.kind, value.Kind) {
		c.add("PAPER_BIND_VALUE_TYPE", fmt.Sprintf("selected scenario @%s supplies %s for %s binding %s", c.fixture.Name, value.Kind, binding.kind, binding.path), "make the fixture value match the declared schema type", binding.span)
		return "", false
	}
	if formatted, selected, ok := c.formatBindingValue(node, value, binding.span); selected {
		return formatted, ok
	}
	switch value.Kind {
	case paperscenario.String:
		return value.String, true
	case paperscenario.Number:
		return value.Number, true
	case paperscenario.Bool:
		return strconv.FormatBool(value.Bool), true
	default:
		c.add("PAPER_BIND_VALUE_TYPE", fmt.Sprintf("binding %s resolved to non-primitive %s", binding.path, value.Kind), "bind paragraph and heading text to string, number, or bool values", binding.span)
		return "", false
	}
}

func (c *compiler) formatBindingValue(node *paperlang.Node, value paperscenario.Value, bindingSpan paperlang.Span) (string, bool, bool) {
	properties := make(map[string]*paperlang.Property)
	var firstFormatSpan = bindingSpan
	for index := range node.Members {
		property := node.Members[index].Property
		if property == nil || !strings.HasPrefix(property.Name, "format") {
			continue
		}
		properties[property.Name] = property
		if firstFormatSpan == bindingSpan {
			firstFormatSpan = property.Span
		}
	}
	format := properties["format"]
	if format == nil {
		if len(properties) != 0 {
			c.add("PAPER_BIND_FORMAT", "format-* properties require a format kind", "add format or remove its format-* properties", firstFormatSpan)
			return "", true, false
		}
		return "", false, true
	}
	if format.Value.Kind != paperlang.ScalarString || format.Value.StringValue == nil {
		c.add("PAPER_BIND_FORMAT", "format must be a quoted string", "use string, bool, integer, decimal, or currency", format.Value.Span)
		return "", true, false
	}
	spec := paperformat.FormatSpec{
		Kind:   paperformat.ValueFormatKind(strings.TrimSpace(*format.Value.StringValue)),
		Locale: c.fixture.Locale,
		Output: paperformat.ValueOutputBare,
	}
	if property := properties["format-locale"]; property != nil {
		if property.Value.Kind != paperlang.ScalarString || property.Value.StringValue == nil {
			c.add("PAPER_BIND_FORMAT", "format-locale must be a quoted locale", "use an explicitly supported locale such as en-US or pt-BR", property.Value.Span)
			return "", true, false
		}
		spec.Locale = strings.TrimSpace(*property.Value.StringValue)
	}
	if property := properties["format-currency"]; property != nil {
		if property.Value.Kind != paperlang.ScalarString || property.Value.StringValue == nil {
			c.add("PAPER_BIND_FORMAT", "format-currency must be a quoted ISO currency", "use an explicitly supported uppercase currency such as USD or BRL", property.Value.Span)
			return "", true, false
		}
		spec.Currency = strings.TrimSpace(*property.Value.StringValue)
	}
	var precision paperformat.Precision
	for _, input := range []struct {
		name   string
		target *uint32
	}{
		{name: "format-min-fraction", target: &precision.MinFractionDigits},
		{name: "format-max-fraction", target: &precision.MaxFractionDigits},
	} {
		property := properties[input.name]
		if property == nil {
			continue
		}
		if property.Value.Kind != paperlang.ScalarNumber || property.Value.NumberValue == nil ||
			*property.Value.NumberValue < 0 || *property.Value.NumberValue > 18 || math.Trunc(*property.Value.NumberValue) != *property.Value.NumberValue {
			c.add("PAPER_BIND_FORMAT", input.name+" must be an integer from 0 through 18", "use an explicit bounded fraction-digit count", property.Value.Span)
			return "", true, false
		}
		*input.target = uint32(*property.Value.NumberValue)
	}
	spec.Precision = precision
	formatted, err := paperformat.FormatValue(value, spec)
	if err != nil {
		c.add("PAPER_BIND_FORMAT", err.Error(), "make the format kind, value type, locale, currency, and precision agree", format.Value.Span)
		return "", true, false
	}
	return formatted, true, true
}

func (c *compiler) lookupBindingValue(node *paperlang.Node, path string) (paperscenario.Value, bool, string) {
	provenance := c.provenance[node]
	if provenance.repeatItem {
		if provenance.repeatItemBase != "" && provenance.repeatValue.Kind == paperscenario.Object {
			return lookupRepeatItemBinding(provenance.repeatValue, path, provenance.repeatItemBase)
		}
		return lookupRepeatBinding(c.fixture.Values, path, provenance.repeatSource, provenance.repeatKey)
	}
	parts, problem := fixturePathParts(path)
	if problem != "" {
		return paperscenario.Value{}, false, problem
	}
	value, found, problem := lookupFixtureFields(c.fixture.Values, parts)
	return value, found, problem
}

func lookupRepeatItemBinding(item paperscenario.Value, path, itemBase string) (paperscenario.Value, bool, string) {
	if item.Kind != paperscenario.Object {
		return paperscenario.Value{}, false, fmt.Sprintf("repeat item %s resolves to %s instead of object", itemBase, item.Kind)
	}
	prefix := strings.TrimSuffix(itemBase, ".") + "."
	if !strings.HasPrefix(path, prefix) {
		return paperscenario.Value{}, false, fmt.Sprintf("repeat binding %s is outside item context %s", path, itemBase)
	}
	relative := strings.TrimPrefix(path, prefix)
	if relative == "" || strings.Contains(relative, "[]") {
		return paperscenario.Value{}, false, fmt.Sprintf("repeat binding %s is not a primitive item path", path)
	}
	return lookupFixtureFields(item.Object, strings.Split(relative, "."))
}

func lookupRepeatBinding(fields []paperscenario.Field, path, source, key string) (paperscenario.Value, bool, string) {
	sourceParts, problem := fixturePathParts(source)
	if problem != "" {
		return paperscenario.Value{}, false, problem
	}
	collection, found, problem := lookupFixtureFields(fields, sourceParts)
	if problem != "" || !found {
		return paperscenario.Value{}, found, problem
	}
	if collection.Kind != paperscenario.List {
		return paperscenario.Value{}, false, fmt.Sprintf("repeat source %s resolves to %s instead of a keyed list", source, collection.Kind)
	}
	var item *paperscenario.Value
	for index := range collection.List {
		if collection.List[index].Key == key {
			item = &collection.List[index].Value
			break
		}
	}
	if item == nil {
		return paperscenario.Value{}, false, fmt.Sprintf("repeat source %s has no stable item key %q", source, key)
	}
	if item.Kind != paperscenario.Object {
		return paperscenario.Value{}, false, fmt.Sprintf("repeat item %s[%s] resolves to %s instead of object", source, key, item.Kind)
	}
	prefix := source + "[]."
	if !strings.HasPrefix(path, prefix) {
		return paperscenario.Value{}, false, fmt.Sprintf("repeat binding %s is outside source %s", path, source)
	}
	itemPath := strings.TrimPrefix(path, prefix)
	if itemPath == "" || strings.Contains(itemPath, "[]") {
		return paperscenario.Value{}, false, fmt.Sprintf("repeat binding %s is not a primitive item path", path)
	}
	return lookupFixtureFields(item.Object, strings.Split(itemPath, "."))
}

func fixturePathParts(path string) ([]string, string) {
	parts := strings.Split(strings.TrimSpace(path), ".")
	if len(parts) < 2 || !strings.HasPrefix(parts[0], "@") {
		return nil, fmt.Sprintf("binding %s is not an absolute fixture path", path)
	}
	for _, part := range parts[1:] {
		if part == "" || strings.Contains(part, "[]") {
			return nil, fmt.Sprintf("binding %s cannot be resolved as one primitive fixture path", path)
		}
	}
	return parts[1:], ""
}

func lookupFixtureFields(fields []paperscenario.Field, parts []string) (paperscenario.Value, bool, string) {
	if len(parts) == 0 {
		return paperscenario.Value{}, false, "fixture binding path is empty"
	}
	for index, name := range parts {
		var value paperscenario.Value
		found := false
		for _, field := range fields {
			if field.Name == name {
				value, found = field.Value, true
				break
			}
		}
		if !found {
			return paperscenario.Value{}, false, ""
		}
		if index == len(parts)-1 {
			return value, true, ""
		}
		if value.Kind != paperscenario.Object {
			return paperscenario.Value{}, false, fmt.Sprintf("fixture path traverses %s field %q as an object", value.Kind, name)
		}
		fields = value.Object
	}
	return paperscenario.Value{}, false, "fixture binding path is empty"
}

func bindingKindMatches(schema SchemaKind, fixture paperscenario.Kind) bool {
	return schema == SchemaString && fixture == paperscenario.String ||
		schema == SchemaNumber && fixture == paperscenario.Number ||
		schema == SchemaBool && fixture == paperscenario.Bool
}
