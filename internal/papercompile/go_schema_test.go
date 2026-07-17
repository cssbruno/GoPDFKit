// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package papercompile

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

type adapterAddress struct {
	City     string
	Nickname *string `paper:"nickname,optional"`
}

type adapterItem struct {
	Price float64
}

type adapterInvoice struct {
	CustomerID string
	Customer   *adapterAddress `paper:"customer,required"`
	Items      []adapterItem   `paper:"items,required"`
	Flags      [2]bool
	Ignored    map[string]string `paper:"-"`
	hidden     string
}

type adapterPointerItems struct {
	Items []*adapterItem
}

type adapterOrder struct {
	Lines []string
}

type adapterOrders struct {
	Orders []adapterOrder
}

type adapterCycle struct {
	Next *adapterCycle
}

type adapterBadMap struct {
	Values map[string]string
}

type adapterBadInterface struct {
	Value any
}

type adapterDuplicate struct {
	First  string `paper:"same"`
	Second string `paper:"same"`
}

type adapterBadTag struct {
	Name string `paper:",omitempty"`
}

type adapterNestedSlice struct {
	Values [][]string
}

type adapterEmpty struct {
	hidden string
}

func TestSchemaDescriptorFromGoType(t *testing.T) {
	t.Parallel()

	policy := GoSchemaPolicy{ListBounds: map[string]uint32{"items": 50}}
	descriptor, err := SchemaDescriptorFromGoType("@invoice", reflect.TypeOf(adapterInvoice{}), policy)
	if err != nil {
		t.Fatalf("convert type: %v", err)
	}
	if descriptor.Name != "@invoice" || descriptor.Kind != SchemaObject || len(descriptor.Fields) != 4 {
		t.Fatalf("unexpected root descriptor: %#v", descriptor)
	}
	if got := descriptor.Fields[0]; got.Name != "customerID" || got.Kind != SchemaString || !got.Required {
		t.Fatalf("unexpected default field mapping: %#v", got)
	}
	customer := descriptor.Fields[1]
	if customer.Name != "customer" || customer.Kind != SchemaObject || !customer.Required || len(customer.Fields) != 2 {
		t.Fatalf("unexpected customer mapping: %#v", customer)
	}
	if nickname := customer.Fields[1]; nickname.Name != "nickname" || nickname.Kind != SchemaString || nickname.Required {
		t.Fatalf("unexpected pointer nullability: %#v", nickname)
	}
	items := descriptor.Fields[2]
	if items.Kind != SchemaList || !items.Required || items.ItemKind != SchemaObject || !items.ItemRequired || items.MaxItems != 50 || len(items.Fields) != 1 {
		t.Fatalf("unexpected slice mapping: %#v", items)
	}
	flags := descriptor.Fields[3]
	if flags.Kind != SchemaList || !flags.Required || flags.ItemKind != SchemaBool || !flags.ItemRequired || flags.MaxItems != 2 {
		t.Fatalf("unexpected array mapping: %#v", flags)
	}

	again, err := SchemaDescriptorFromGoValue("@invoice", adapterInvoice{}, policy)
	if err != nil {
		t.Fatalf("convert value type: %v", err)
	}
	if !reflect.DeepEqual(descriptor, again) {
		t.Fatalf("value and type adapters differ:\n%#v\n%#v", descriptor, again)
	}
}

func TestSchemaDescriptorFromGoTypeRootAndNullableItems(t *testing.T) {
	t.Parallel()

	primitive, err := SchemaDescriptorFromGoType("@label", reflect.TypeOf(""), GoSchemaPolicy{})
	if err != nil || primitive.Kind != SchemaString {
		t.Fatalf("root primitive: %#v, %v", primitive, err)
	}
	list, err := SchemaDescriptorFromGoType("@labels", reflect.TypeOf([]string{}), GoSchemaPolicy{ListBounds: map[string]uint32{"$": 10}})
	if err != nil || list.Kind != SchemaList || list.ItemKind != SchemaString || !list.ItemRequired || list.MaxItems != 10 {
		t.Fatalf("root list: %#v, %v", list, err)
	}
	pointers, err := SchemaDescriptorFromGoType("@items", reflect.TypeOf(adapterPointerItems{}), GoSchemaPolicy{ListBounds: map[string]uint32{"items": 4}})
	if err != nil {
		t.Fatalf("pointer items: %v", err)
	}
	if pointers.Fields[0].ItemRequired {
		t.Fatalf("pointer list items must be nullable: %#v", pointers.Fields[0])
	}

	nested, err := SchemaDescriptorFromGoType("@orders", reflect.TypeOf(adapterOrders{}), GoSchemaPolicy{ListBounds: map[string]uint32{
		"orders":         5,
		"orders[].lines": 20,
	}})
	if err != nil {
		t.Fatalf("nested list path: %v", err)
	}
	lines := nested.Fields[0].Fields[0]
	if lines.Name != "lines" || lines.Kind != SchemaList || lines.ItemKind != SchemaString || lines.MaxItems != 20 {
		t.Fatalf("unexpected nested bound mapping: %#v", lines)
	}
}

func TestSchemaDescriptorFromGoTypeRejectsUnsupportedContracts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		typ     reflect.Type
		policy  GoSchemaPolicy
		problem string
	}{
		{name: "map", typ: reflect.TypeOf(adapterBadMap{}), problem: "map"},
		{name: "interface", typ: reflect.TypeOf(adapterBadInterface{}), problem: "interface"},
		{name: "function", typ: reflect.TypeOf(func() {}), problem: "func"},
		{name: "channel", typ: reflect.TypeOf(make(chan int)), problem: "chan"},
		{name: "cycle", typ: reflect.TypeOf(adapterCycle{}), problem: "cycle"},
		{name: "bad tag", typ: reflect.TypeOf(adapterBadTag{}), problem: "omitempty"},
		{name: "duplicate", typ: reflect.TypeOf(adapterDuplicate{}), problem: "duplicated"},
		{name: "empty", typ: reflect.TypeOf(adapterEmpty{}), problem: "no exported"},
		{name: "missing bound", typ: reflect.TypeOf([]string{}), problem: "ListBounds"},
		{name: "zero array", typ: reflect.TypeOf([0]string{}), problem: "zero-length"},
		{
			name: "nested slice", typ: reflect.TypeOf(adapterNestedSlice{}),
			policy: GoSchemaPolicy{ListBounds: map[string]uint32{"values": 3, "values[]": 2}}, problem: "nested lists",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := SchemaDescriptorFromGoType("@test", test.typ, test.policy)
			assertGoSchemaError(t, err, test.problem)
		})
	}
}

func TestSchemaDescriptorFromGoTypeEnforcesPolicyAndLimits(t *testing.T) {
	t.Parallel()

	_, err := SchemaDescriptorFromGoType("@labels", reflect.TypeOf([]string{}), GoSchemaPolicy{ListBounds: map[string]uint32{"$": 2, "typo": 2}})
	assertGoSchemaError(t, err, "does not match")

	_, err = SchemaDescriptorFromGoType("@flags", reflect.TypeOf([2]bool{}), GoSchemaPolicy{ListBounds: map[string]uint32{"$": 2}})
	assertGoSchemaError(t, err, "does not match")

	limits := DefaultSchemaLimits()
	limits.MaxFields = 1
	_, err = SchemaDescriptorFromGoType("@invoice", reflect.TypeOf(adapterInvoice{}), GoSchemaPolicy{ListBounds: map[string]uint32{"items": 2}, Limits: limits})
	assertGoSchemaError(t, err, "field count")

	limits = DefaultSchemaLimits()
	limits.MaxDepth = 1
	_, err = SchemaDescriptorFromGoType("@invoice", reflect.TypeOf(adapterInvoice{}), GoSchemaPolicy{ListBounds: map[string]uint32{"items": 2}, Limits: limits})
	assertGoSchemaError(t, err, "nesting")

	limits = DefaultSchemaLimits()
	limits.MaxPathSegments = 1
	_, err = SchemaDescriptorFromGoType("@invoice", reflect.TypeOf(adapterInvoice{}), GoSchemaPolicy{ListBounds: map[string]uint32{"items": 2}, Limits: limits})
	assertGoSchemaError(t, err, "segment limit")

	limits = DefaultSchemaLimits()
	limits.MaxPathBytes = 5
	_, err = SchemaDescriptorFromGoType("@invoice", reflect.TypeOf(adapterInvoice{}), GoSchemaPolicy{ListBounds: map[string]uint32{"items": 2}, Limits: limits})
	assertGoSchemaError(t, err, "byte limit")

	_, err = SchemaDescriptorFromGoType("@bad", reflect.TypeOf(""), GoSchemaPolicy{Limits: SchemaLimits{MaxFields: 1}})
	assertGoSchemaError(t, err, "limits")

	_, err = SchemaDescriptorFromGoType("@labels", reflect.TypeOf([]string{}), GoSchemaPolicy{ListBounds: map[string]uint32{"$": DefaultSchemaLimits().MaxListItems + 1}})
	assertGoSchemaError(t, err, "MaxListItems")
}

func TestSchemaDescriptorFromGoTypeValidatesBoundaryInputs(t *testing.T) {
	t.Parallel()

	_, err := SchemaDescriptorFromGoType("invoice", reflect.TypeOf(""), GoSchemaPolicy{})
	assertGoSchemaError(t, err, "@name")
	_, err = SchemaDescriptorFromGoType("@invoice", nil, GoSchemaPolicy{})
	assertGoSchemaError(t, err, "nil reflect.Type")
	_, err = SchemaDescriptorFromGoValue("@invoice", nil, GoSchemaPolicy{})
	assertGoSchemaError(t, err, "nil has no static")
}

func assertGoSchemaError(t *testing.T, err error, contains string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q", contains)
	}
	if !errors.Is(err, ErrGoSchemaAdapter) {
		t.Fatalf("error does not wrap ErrGoSchemaAdapter: %v", err)
	}
	var typed *GoSchemaError
	if !errors.As(err, &typed) || typed.Path == "" {
		t.Fatalf("error does not expose a path: %#v", err)
	}
	if !strings.Contains(err.Error(), contains) {
		t.Fatalf("error %q does not contain %q", err, contains)
	}
}
