// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package paperexpr

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
)

func TestEvaluateTypedDeterministicProgram(t *testing.T) {
	program := Program{
		Constants: []Value{{Kind: String, String: "Hello "}, {Kind: String, String: "Hello Ada"}, {Kind: String, String: "unexpected"}},
		Paths:     []string{"customer.name"},
		Code: []Instruction{
			{Op: OpConstant, Arg: 0}, {Op: OpLoad, Arg: 0}, {Op: OpConcat},
			{Op: OpConstant, Arg: 1}, {Op: OpEqual},
			{Op: OpConstant, Arg: 1}, {Op: OpConstant, Arg: 2}, {Op: OpSelect},
		},
	}
	bindings := []Binding{{Path: "customer.name", Value: Value{Kind: String, String: "Ada"}}}
	first, err := Evaluate(context.Background(), program, bindings, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Evaluate(context.Background(), program, bindings, Limits{})
	if err != nil || first != second || first.String != "Hello Ada" {
		t.Fatalf("results = %+v %+v, %v", first, second, err)
	}
}

func TestEvaluateBooleanIntegerAndMissingBinding(t *testing.T) {
	program := Program{Paths: []string{"a", "b"}, Code: []Instruction{{Op: OpLoad}, {Op: OpLoad, Arg: 1}, {Op: OpAddInteger}}}
	value, err := Evaluate(context.Background(), program, []Binding{{Path: "b", Value: Value{Kind: Integer, Integer: 2}}, {Path: "a", Value: Value{Kind: Integer, Integer: 3}}}, Limits{})
	if err != nil || value.Integer != 5 {
		t.Fatalf("value = %+v, error = %v", value, err)
	}
	_, err = Evaluate(context.Background(), program, []Binding{{Path: "a", Value: Value{Kind: Integer}}}, Limits{})
	if !errors.Is(err, ErrBinding) {
		t.Fatalf("missing binding error = %v", err)
	}
	overflow := Program{Constants: []Value{{Kind: Integer, Integer: math.MaxInt64}, {Kind: Integer, Integer: 1}}, Code: []Instruction{{Op: OpConstant}, {Op: OpConstant, Arg: 1}, {Op: OpAddInteger}}}
	if _, err := Evaluate(context.Background(), overflow, nil, Limits{}); !errors.Is(err, ErrType) {
		t.Fatalf("overflow error = %v", err)
	}
}

func TestEvaluateRejectsInvalidProgramsTypesAndLimits(t *testing.T) {
	tests := []Program{
		{},
		{Code: []Instruction{{Op: OpAnd}}},
		{Code: []Instruction{{Op: OpConstant}}},
		{Constants: []Value{{Kind: Bool, String: "hidden"}}, Code: []Instruction{{Op: OpConstant}}},
		{Paths: []string{"b", "a"}, Code: []Instruction{{Op: OpLoad}}},
		{Code: []Instruction{{Op: 255}}},
	}
	for i, program := range tests {
		if _, err := Evaluate(context.Background(), program, nil, Limits{}); err == nil {
			t.Fatalf("program %d unexpectedly accepted", i)
		}
	}
	limits := DefaultLimits()
	limits.MaxStack = 1
	program := Program{Constants: []Value{{Kind: Null}}, Code: []Instruction{{Op: OpConstant}, {Op: OpConstant}, {Op: OpEqual}}}
	if _, err := Evaluate(context.Background(), program, nil, limits); !errors.Is(err, ErrLimit) {
		t.Fatalf("stack limit error = %v", err)
	}
}

func TestEvaluateCancellationAndStringWorkLimit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	program := Program{Constants: []Value{{Kind: Null}}, Code: []Instruction{{Op: OpConstant}}}
	if _, err := Evaluate(ctx, program, nil, Limits{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error = %v", err)
	}
	limits := DefaultLimits()
	limits.MaxWork = 2
	concat := Program{Constants: []Value{{Kind: String, String: "aa"}}, Code: []Instruction{{Op: OpConstant}, {Op: OpConstant}, {Op: OpConcat}}}
	if _, err := Evaluate(context.Background(), concat, nil, limits); !errors.Is(err, ErrLimit) {
		t.Fatalf("work limit error = %v", err)
	}
}

func TestEvaluateBoundedUnicodeWildcardMatches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		text, pattern string
		matched       bool
	}{
		{text: "invoice-2026", pattern: "invoice-*", matched: true},
		{text: "Olá 🚀", pattern: "Olá ?", matched: true},
		{text: "file?.pdf", pattern: `file\?.pdf`, matched: true},
		{text: "file*.pdf", pattern: `file\*.pdf`, matched: true},
		{text: `a\b`, pattern: `a\\b`, matched: true},
		{text: "prefix-suffix", pattern: "prefix", matched: false},
		{text: "", pattern: "*", matched: true},
	}
	for _, test := range tests {
		program := Program{Constants: []Value{{Kind: String, String: test.text}, {Kind: String, String: test.pattern}}, Code: []Instruction{{Op: OpConstant}, {Op: OpConstant, Arg: 1}, {Op: OpMatches}}}
		value, err := Evaluate(context.Background(), program, nil, Limits{})
		if err != nil || value.Kind != Bool || value.Bool != test.matched {
			t.Fatalf("%q matches %q = %#v, %v; want %t", test.text, test.pattern, value, err, test.matched)
		}
	}
}

func TestEvaluateMatchesRejectsTypesPatternsAndBoundedAdversarialWork(t *testing.T) {
	t.Parallel()

	nonStrings := Program{Constants: []Value{{Kind: Integer, Integer: 1}, {Kind: String, String: "*"}}, Code: []Instruction{{Op: OpConstant}, {Op: OpConstant, Arg: 1}, {Op: OpMatches}}}
	if _, err := Evaluate(context.Background(), nonStrings, nil, Limits{}); !errors.Is(err, ErrType) {
		t.Fatalf("matches type error = %v", err)
	}
	invalid := Program{Constants: []Value{{Kind: String, String: "x"}, {Kind: String, String: `bad\`}}, Code: []Instruction{{Op: OpConstant}, {Op: OpConstant, Arg: 1}, {Op: OpMatches}}}
	if _, err := Evaluate(context.Background(), invalid, nil, Limits{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid pattern error = %v", err)
	}
	patternLimited := DefaultLimits()
	patternLimited.MaxPatternBytes = 3
	tooLong := Program{Constants: []Value{{Kind: String, String: "text"}, {Kind: String, String: "text"}}, Code: []Instruction{{Op: OpConstant}, {Op: OpConstant, Arg: 1}, {Op: OpMatches}}}
	if _, err := Evaluate(context.Background(), tooLong, nil, patternLimited); !errors.Is(err, ErrLimit) {
		t.Fatalf("pattern byte limit error = %v", err)
	}

	workLimited := DefaultLimits()
	workLimited.MaxWork = 50
	adversarial := Program{Constants: []Value{
		{Kind: String, String: strings.Repeat("a", 128)},
		{Kind: String, String: "*aaaaaaaaab"},
	}, Code: []Instruction{{Op: OpConstant}, {Op: OpConstant, Arg: 1}, {Op: OpMatches}}}
	if _, err := Evaluate(context.Background(), adversarial, nil, workLimited); !errors.Is(err, ErrLimit) {
		t.Fatalf("adversarial match work error = %v", err)
	}
}
