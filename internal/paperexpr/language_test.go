// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperexpr

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestCompileAndEvaluateHumanExpression(t *testing.T) {
	t.Parallel()

	source := `!disabled && customer.active || fallback ? "Hello, " + customer.name : "bye"`
	environment := []PathKind{
		{Path: "fallback", Kind: Bool},
		{Path: "customer.name", Kind: String},
		{Path: "disabled", Kind: Bool},
		{Path: "customer.active", Kind: Bool},
	}
	program, kind, err := Compile(source, environment, LanguageLimits{})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if kind != String {
		t.Fatalf("result kind = %v, want string", kind)
	}
	if want := []string{"customer.active", "customer.name", "disabled", "fallback"}; !reflect.DeepEqual(program.Paths, want) {
		t.Fatalf("paths = %#v, want %#v", program.Paths, want)
	}
	if want := []Value{{Kind: String, String: "Hello, "}, {Kind: String, String: "bye"}}; !reflect.DeepEqual(program.Constants, want) {
		t.Fatalf("constants = %#v, want %#v", program.Constants, want)
	}
	value, err := Evaluate(context.Background(), program, []Binding{
		{Path: "customer.name", Value: Value{Kind: String, String: "Ada"}},
		{Path: "customer.active", Value: Value{Kind: Bool, Bool: true}},
		{Path: "fallback", Value: Value{Kind: Bool}},
		{Path: "disabled", Value: Value{Kind: Bool}},
	}, Limits{})
	if err != nil || value.Kind != String || value.String != "Hello, Ada" {
		t.Fatalf("value = %#v, error = %v", value, err)
	}
}

func TestCompilePrecedenceKindsAndDeterminism(t *testing.T) {
	t.Parallel()

	environment := []PathKind{{Path: "b", Kind: Integer}, {Path: "a", Kind: Integer}}
	program, kind, err := Compile(`a + 2 + 3 == 10`, environment, LanguageLimits{})
	if err != nil {
		t.Fatalf("compile integer expression: %v", err)
	}
	if kind != Bool || !reflect.DeepEqual(program.Paths, []string{"a"}) || !reflect.DeepEqual(program.Constants, []Value{
		{Kind: Integer, Integer: 2},
		{Kind: Integer, Integer: 3},
		{Kind: Integer, Integer: 10},
	}) {
		t.Fatalf("unexpected program: %#v, kind %v", program, kind)
	}
	wantCode := []Instruction{
		{Op: OpLoad},
		{Op: OpConstant},
		{Op: OpAddInteger},
		{Op: OpConstant, Arg: 1},
		{Op: OpAddInteger},
		{Op: OpConstant, Arg: 2},
		{Op: OpEqual},
	}
	if !reflect.DeepEqual(program.Code, wantCode) {
		t.Fatalf("bytecode = %#v, want %#v", program.Code, wantCode)
	}
	value, err := Evaluate(context.Background(), program, []Binding{{Path: "a", Value: Value{Kind: Integer, Integer: 5}}}, Limits{})
	if err != nil || value.Kind != Bool || !value.Bool {
		t.Fatalf("integer result = %#v, %v", value, err)
	}

	second, secondKind, err := Compile(`a + 2 + 3 == 10`, []PathKind{{Path: "a", Kind: Integer}, {Path: "b", Kind: Integer}}, LanguageLimits{})
	if err != nil || secondKind != kind || !reflect.DeepEqual(program, second) {
		t.Fatalf("nondeterministic compile: %#v %#v, %v", program, second, err)
	}

	conditional, conditionalKind, err := Compile(`first ? 1 : second ? 2 : 3`, []PathKind{{Path: "second", Kind: Bool}, {Path: "first", Kind: Bool}}, LanguageLimits{})
	if err != nil || conditionalKind != Integer {
		t.Fatalf("right-associative conditional: %#v, %v, %v", conditional, conditionalKind, err)
	}
	value, err = Evaluate(context.Background(), conditional, []Binding{
		{Path: "first", Value: Value{Kind: Bool}},
		{Path: "second", Value: Value{Kind: Bool, Bool: true}},
	}, Limits{})
	if err != nil || value.Integer != 2 {
		t.Fatalf("conditional result = %#v, %v", value, err)
	}
}

func TestCompileMatchesSyntaxTypingPrecedenceAndLiteralDiagnostics(t *testing.T) {
	t.Parallel()

	environment := []PathKind{{Path: "name", Kind: String}, {Path: "active", Kind: Bool}, {Path: "pattern", Kind: String}}
	program, kind, err := Compile(`active && name matches "Ada*"`, environment, LanguageLimits{})
	if err != nil || kind != Bool {
		t.Fatalf("compile matches = %#v, %v, %v", program, kind, err)
	}
	if want := []Instruction{{Op: OpLoad}, {Op: OpLoad, Arg: 1}, {Op: OpConstant}, {Op: OpMatches}, {Op: OpAnd}}; !reflect.DeepEqual(program.Code, want) {
		t.Fatalf("matches bytecode = %#v, want %#v", program.Code, want)
	}
	value, err := Evaluate(context.Background(), program, []Binding{
		{Path: "active", Value: Value{Kind: Bool, Bool: true}},
		{Path: "name", Value: Value{Kind: String, String: "Ada Lovelace"}},
		{Path: "pattern", Value: Value{Kind: String, String: "unused"}},
	}, Limits{})
	if err != nil || value != (Value{Kind: Bool, Bool: true}) {
		t.Fatalf("matches value = %#v, %v", value, err)
	}

	dynamic, dynamicKind, err := Compile(`name matches pattern`, environment, LanguageLimits{})
	if err != nil || dynamicKind != Bool || len(dynamic.Code) != 3 || dynamic.Code[2].Op != OpMatches {
		t.Fatalf("dynamic match = %#v, %v, %v", dynamic, dynamicKind, err)
	}
	_, _, err = Compile(`active matches "*"`, environment, LanguageLimits{})
	assertExpressionError(t, err, ErrType, 7, "two strings")
	_, _, err = Compile(`name matches "bad\\"`, environment, LanguageLimits{})
	assertExpressionError(t, err, ErrInvalid, 13, "escape")

	limits := DefaultLanguageLimits()
	limits.Program.MaxPatternBytes = 3
	_, _, err = Compile(`name matches "four"`, environment, limits)
	assertExpressionError(t, err, ErrLimit, 13, "MaxPatternBytes")
}

func TestCompileLiteralsAndDeduplicatesInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		source string
		kind   Kind
		value  Value
	}{
		{source: `null == null`, kind: Bool, value: Value{Kind: Bool, Bool: true}},
		{source: `-42`, kind: Integer, value: Value{Kind: Integer, Integer: -42}},
		{source: `"Olá \ud83d\ude80"`, kind: String, value: Value{Kind: String, String: "Olá 🚀"}},
		{source: `true || false`, kind: Bool, value: Value{Kind: Bool, Bool: true}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.source, func(t *testing.T) {
			t.Parallel()
			program, kind, err := Compile(test.source, nil, LanguageLimits{})
			if err != nil || kind != test.kind {
				t.Fatalf("compile = %#v, %v, %v", program, kind, err)
			}
			value, err := Evaluate(context.Background(), program, nil, Limits{})
			if err != nil || value != test.value {
				t.Fatalf("evaluate = %#v, %v; want %#v", value, err, test.value)
			}
		})
	}

	program, _, err := Compile(`name + name + "x" + "x"`, []PathKind{{Path: "name", Kind: String}}, LanguageLimits{})
	if err != nil {
		t.Fatalf("compile repeated inputs: %v", err)
	}
	if !reflect.DeepEqual(program.Paths, []string{"name"}) || !reflect.DeepEqual(program.Constants, []Value{{Kind: String, String: "x"}}) {
		t.Fatalf("inputs were not deduplicated: %#v", program)
	}
}

func TestParseReturnsBoundedImmutableExpression(t *testing.T) {
	t.Parallel()

	expression, err := Parse(`(true || false) && !false`, LanguageLimits{})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if expression.Source() != `(true || false) && !false` || expression.tokenCount != 8 || expression.nodeCount != 6 {
		t.Fatalf("unexpected parsed metadata: %#v", expression)
	}
	program, kind, err := CompileExpression(expression, nil, LanguageLimits{})
	if err != nil || kind != Bool {
		t.Fatalf("compile parsed expression: %#v, %v, %v", program, kind, err)
	}
}

func TestCompileRejectsStaticTypeAndEnvironmentErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		source      string
		environment []PathKind
		cause       error
		offset      uint32
		problem     string
	}{
		{name: "mixed plus", source: `count + "x"`, environment: []PathKind{{Path: "count", Kind: Integer}}, cause: ErrType, offset: 6, problem: "two integers or two strings"},
		{name: "boolean", source: `name && true`, environment: []PathKind{{Path: "name", Kind: String}}, cause: ErrType, offset: 5, problem: "bool"},
		{name: "equality", source: `count == name`, environment: []PathKind{{Path: "count", Kind: Integer}, {Path: "name", Kind: String}}, cause: ErrType, offset: 6, problem: "same static kind"},
		{name: "condition", source: `count ? 1 : 2`, environment: []PathKind{{Path: "count", Kind: Integer}}, cause: ErrType, offset: 6, problem: "condition"},
		{name: "branches", source: `flag ? 1 : "x"`, environment: []PathKind{{Path: "flag", Kind: Bool}}, cause: ErrType, offset: 5, problem: "branches"},
		{name: "not", source: `!count`, environment: []PathKind{{Path: "count", Kind: Integer}}, cause: ErrType, offset: 0, problem: "requires bool"},
		{name: "missing", source: `missing`, cause: ErrBinding, offset: 0, problem: "not declared"},
		{name: "duplicate environment", source: `name`, environment: []PathKind{{Path: "name", Kind: String}, {Path: "name", Kind: String}}, cause: ErrBinding, offset: 0, problem: "duplicate"},
		{name: "invalid environment", source: `name`, environment: []PathKind{{Path: "bad path", Kind: String}}, cause: ErrBinding, offset: 0, problem: "invalid environment"},
		{name: "invalid environment kind", source: `name`, environment: []PathKind{{Path: "name", Kind: 255}}, cause: ErrType, offset: 0, problem: "invalid kind"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := Compile(test.source, test.environment, LanguageLimits{})
			assertExpressionError(t, err, test.cause, test.offset, test.problem)
		})
	}
}

func TestParseRejectsInvalidSyntaxWithOffsets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		source  string
		offset  uint32
		problem string
	}{
		{name: "empty", source: ``, offset: 0, problem: "expected expression"},
		{name: "leading zero", source: `01`, offset: 0, problem: "canonical"},
		{name: "negative zero", source: `-0`, offset: 0, problem: "canonical"},
		{name: "overflow", source: `9223372036854775808`, offset: 0, problem: "int64"},
		{name: "single equal", source: `a = b`, offset: 2, problem: "expected =="},
		{name: "single ampersand", source: `true & false`, offset: 5, problem: "expected &&"},
		{name: "invalid path", source: `a..b`, offset: 0, problem: "dotted"},
		{name: "unexpected rune", source: `true × false`, offset: 5, problem: "unexpected character"},
		{name: "unterminated string", source: `"hello`, offset: 0, problem: "unterminated"},
		{name: "invalid escape", source: `"\x41"`, offset: 0, problem: "unsupported string escape"},
		{name: "lone surrogate", source: `"\ud800"`, offset: 0, problem: "surrogate"},
		{name: "missing parenthesis", source: `(true`, offset: 5, problem: "requires )"},
		{name: "missing colon", source: `true ? 1`, offset: 8, problem: "requires :"},
		{name: "trailing", source: `true false`, offset: 5, problem: "after expression"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := Parse(test.source, LanguageLimits{})
			assertExpressionError(t, err, ErrInvalid, test.offset, test.problem)
		})
	}
}

func TestCompileEnforcesLanguageAndProgramLimits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		source  string
		limits  func() LanguageLimits
		problem string
	}{
		{name: "source bytes", source: `true`, limits: func() LanguageLimits { value := DefaultLanguageLimits(); value.MaxSourceBytes = 3; return value }, problem: "MaxSourceBytes"},
		{name: "tokens", source: `1 + 2`, limits: func() LanguageLimits { value := DefaultLanguageLimits(); value.MaxTokens = 2; return value }, problem: "MaxTokens"},
		{name: "nodes", source: `1 + 2`, limits: func() LanguageLimits { value := DefaultLanguageLimits(); value.MaxNodes = 2; return value }, problem: "MaxNodes"},
		{name: "depth", source: `!!!true`, limits: func() LanguageLimits { value := DefaultLanguageLimits(); value.MaxDepth = 2; return value }, problem: "MaxDepth"},
		{name: "constants", source: `1 + 2`, limits: func() LanguageLimits { value := DefaultLanguageLimits(); value.Program.MaxConstants = 1; return value }, problem: "MaxConstants"},
		{name: "paths", source: `a + b`, limits: func() LanguageLimits { value := DefaultLanguageLimits(); value.Program.MaxPaths = 1; return value }, problem: "MaxPaths"},
		{name: "instructions", source: `1 + 2`, limits: func() LanguageLimits {
			value := DefaultLanguageLimits()
			value.Program.MaxInstructions = 2
			return value
		}, problem: "MaxInstructions"},
		{name: "stack", source: `1 + 2`, limits: func() LanguageLimits { value := DefaultLanguageLimits(); value.Program.MaxStack = 1; return value }, problem: "VM limits"},
		{name: "string", source: `"abc"`, limits: func() LanguageLimits {
			value := DefaultLanguageLimits()
			value.Program.MaxStringBytes = 2
			return value
		}, problem: "MaxStringBytes"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			environment := []PathKind{{Path: "a", Kind: Integer}, {Path: "b", Kind: Integer}}
			_, _, err := Compile(test.source, environment, test.limits())
			assertExpressionError(t, err, ErrLimit, expressionErrorOffset(err), test.problem)
		})
	}

	_, _, err := Compile(`true`, nil, LanguageLimits{MaxTokens: 1})
	assertExpressionError(t, err, ErrLimit, 0, "limits")

	expression, err := Parse(`true || false`, LanguageLimits{})
	if err != nil {
		t.Fatalf("parse for re-limit: %v", err)
	}
	tighter := DefaultLanguageLimits()
	tighter.MaxTokens = 2
	_, _, err = CompileExpression(expression, nil, tighter)
	assertExpressionError(t, err, ErrLimit, expression.root.start, "compile limits")
}

func assertExpressionError(t *testing.T, err, cause error, offset uint32, problem string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected %v containing %q", cause, problem)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("error %v does not wrap %v", err, cause)
	}
	var typed *ExpressionError
	if !errors.As(err, &typed) {
		t.Fatalf("error is not ExpressionError: %T", err)
	}
	if typed.Offset != offset {
		t.Fatalf("offset = %d, want %d (%v)", typed.Offset, offset, err)
	}
	if !strings.Contains(err.Error(), problem) {
		t.Fatalf("error %q does not contain %q", err, problem)
	}
}

func expressionErrorOffset(err error) uint32 {
	var typed *ExpressionError
	if errors.As(err, &typed) {
		return typed.Offset
	}
	return 0
}
