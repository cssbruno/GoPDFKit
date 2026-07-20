// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

// Package paperexpr evaluates a closed, deterministic expression bytecode.
// Programs receive only explicit immutable bindings and expose no I/O, time,
// randomness, reflection, process, or network capability.
package paperexpr

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

type Kind uint8

const (
	Null Kind = iota
	Bool
	Integer
	String
)

type Value struct {
	Kind    Kind
	Bool    bool
	Integer int64
	String  string
}

type Op uint8

const (
	OpConstant Op = iota + 1
	OpLoad
	OpEqual
	OpNot
	OpAnd
	OpOr
	OpAddInteger
	OpConcat
	OpMatches
	OpSelect
)

type Instruction struct {
	Op  Op
	Arg uint32
}

type Program struct {
	Constants []Value
	Paths     []string
	Code      []Instruction
}

type Binding struct {
	Path  string
	Value Value
}

type Limits struct {
	MaxInstructions uint32
	MaxConstants    uint32
	MaxPaths        uint32
	MaxStack        uint32
	MaxStringBytes  uint32
	MaxPatternBytes uint32
	MaxWork         uint64
}

func DefaultLimits() Limits {
	return Limits{MaxInstructions: 4096, MaxConstants: 1024, MaxPaths: 1024, MaxStack: 256, MaxStringBytes: 1 << 20, MaxPatternBytes: 64 << 10, MaxWork: 1 << 20}
}

var (
	ErrInvalid = errors.New("paperexpr: invalid program")
	ErrType    = errors.New("paperexpr: type error")
	ErrBinding = errors.New("paperexpr: binding error")
	ErrLimit   = errors.New("paperexpr: limit exceeded")
)

// Evaluate validates and executes program. Bindings and program slices are
// read-only; the returned value owns its string.
func Evaluate(ctx context.Context, program Program, bindings []Binding, limits Limits) (Value, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	limits, err := normalizeLimits(limits)
	if err != nil {
		return Value{}, err
	}
	maxStack, err := validateProgram(program, limits)
	if err != nil {
		return Value{}, err
	}
	values, err := validateBindings(bindings, limits)
	if err != nil {
		return Value{}, err
	}
	stack := make([]Value, 0, maxStack)
	work := uint64(0)
	for pc, instruction := range program.Code {
		if pc&63 == 0 {
			if err := ctx.Err(); err != nil {
				return Value{}, err
			}
		}
		work++
		if work > limits.MaxWork {
			return Value{}, ErrLimit
		}
		switch instruction.Op {
		case OpConstant:
			stack = append(stack, program.Constants[instruction.Arg])
		case OpLoad:
			path := program.Paths[instruction.Arg]
			index := sort.Search(len(values), func(i int) bool { return values[i].Path >= path })
			if index == len(values) || values[index].Path != path {
				return Value{}, fmt.Errorf("%w: missing %q", ErrBinding, path)
			}
			stack = append(stack, values[index].Value)
		case OpNot:
			value := pop1(&stack)
			if value.Kind != Bool {
				return Value{}, fmt.Errorf("%w: not requires bool", ErrType)
			}
			stack = append(stack, Value{Kind: Bool, Bool: !value.Bool})
		case OpEqual:
			left, right := pop2(&stack)
			stack = append(stack, Value{Kind: Bool, Bool: equal(left, right)})
		case OpAnd, OpOr:
			left, right := pop2(&stack)
			if left.Kind != Bool || right.Kind != Bool {
				return Value{}, fmt.Errorf("%w: boolean operator requires bool", ErrType)
			}
			stack = append(stack, Value{Kind: Bool, Bool: instruction.Op == OpAnd && left.Bool && right.Bool || instruction.Op == OpOr && (left.Bool || right.Bool)})
		case OpAddInteger:
			left, right := pop2(&stack)
			if left.Kind != Integer || right.Kind != Integer {
				return Value{}, fmt.Errorf("%w: addition requires integers", ErrType)
			}
			result := left.Integer + right.Integer
			if right.Integer > 0 && result < left.Integer || right.Integer < 0 && result > left.Integer {
				return Value{}, fmt.Errorf("%w: integer overflow", ErrType)
			}
			stack = append(stack, Value{Kind: Integer, Integer: result})
		case OpConcat:
			left, right := pop2(&stack)
			if left.Kind != String || right.Kind != String {
				return Value{}, fmt.Errorf("%w: concat requires strings", ErrType)
			}
			if uint64(len(left.String))+uint64(len(right.String)) > uint64(limits.MaxStringBytes) {
				return Value{}, ErrLimit
			}
			work += uint64(len(left.String) + len(right.String))
			if work > limits.MaxWork {
				return Value{}, ErrLimit
			}
			stack = append(stack, Value{Kind: String, String: left.String + right.String})
		case OpMatches:
			text, pattern := pop2(&stack)
			if text.Kind != String || pattern.Kind != String {
				return Value{}, fmt.Errorf("%w: matches requires strings", ErrType)
			}
			matched, err := wildcardMatch(ctx, text.String, pattern.String, limits, &work)
			if err != nil {
				return Value{}, err
			}
			stack = append(stack, Value{Kind: Bool, Bool: matched})
		case OpSelect:
			condition, whenTrue, whenFalse := pop3(&stack)
			if condition.Kind != Bool {
				return Value{}, fmt.Errorf("%w: select condition requires bool", ErrType)
			}
			if condition.Bool {
				stack = append(stack, whenTrue)
			} else {
				stack = append(stack, whenFalse)
			}
		default:
			return Value{}, ErrInvalid
		}
	}
	if len(stack) != 1 {
		return Value{}, ErrInvalid
	}
	return stack[0], nil
}

func validateProgram(program Program, limits Limits) (uint32, error) {
	if len(program.Code) == 0 || uint32(len(program.Code)) > limits.MaxInstructions || uint32(len(program.Constants)) > limits.MaxConstants || uint32(len(program.Paths)) > limits.MaxPaths { // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
		return 0, ErrLimit
	}
	for i, value := range program.Constants {
		if err := validateValue(value, limits); err != nil {
			return 0, fmt.Errorf("constant %d: %w", i, err)
		}
	}
	for i, path := range program.Paths {
		if !validPath(path) || uint32(len(path)) > limits.MaxStringBytes { // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
			return 0, fmt.Errorf("%w: invalid path %d", ErrInvalid, i)
		}
		if i > 0 && program.Paths[i-1] >= path {
			return 0, fmt.Errorf("%w: paths must be unique and sorted", ErrInvalid)
		}
	}
	depth, peak := int64(0), int64(0)
	for pc, instruction := range program.Code {
		pop, push := int64(0), int64(1)
		switch instruction.Op {
		case OpConstant:
			if uint64(instruction.Arg) >= uint64(len(program.Constants)) {
				return 0, fmt.Errorf("%w: instruction %d constant", ErrInvalid, pc)
			}
		case OpLoad:
			if uint64(instruction.Arg) >= uint64(len(program.Paths)) {
				return 0, fmt.Errorf("%w: instruction %d path", ErrInvalid, pc)
			}
		case OpNot:
			pop = 1
		case OpEqual, OpAnd, OpOr, OpAddInteger, OpConcat, OpMatches:
			pop = 2
		case OpSelect:
			pop = 3
		default:
			return 0, fmt.Errorf("%w: instruction %d opcode", ErrInvalid, pc)
		}
		if instruction.Op != OpConstant && instruction.Op != OpLoad && instruction.Arg != 0 {
			return 0, fmt.Errorf("%w: instruction %d has unused argument", ErrInvalid, pc)
		}
		if depth < pop {
			return 0, fmt.Errorf("%w: instruction %d stack underflow", ErrInvalid, pc)
		}
		depth = depth - pop + push
		if depth > peak {
			peak = depth
		}
		if peak > int64(limits.MaxStack) {
			return 0, ErrLimit
		}
	}
	if depth != 1 {
		return 0, fmt.Errorf("%w: final stack depth", ErrInvalid)
	}
	return uint32(peak), nil
}

func validateBindings(bindings []Binding, limits Limits) ([]Binding, error) {
	result := append([]Binding(nil), bindings...)
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	for i := range result {
		if !validPath(result[i].Path) || uint32(len(result[i].Path)) > limits.MaxStringBytes { // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
			return nil, ErrBinding
		}
		if i > 0 && result[i-1].Path == result[i].Path {
			return nil, fmt.Errorf("%w: duplicate %q", ErrBinding, result[i].Path)
		}
		if err := validateValue(result[i].Value, limits); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func validateValue(value Value, limits Limits) error {
	if value.Kind > String {
		return ErrType
	}
	if value.Kind != Bool && value.Bool || value.Kind != Integer && value.Integer != 0 || value.Kind != String && value.String != "" {
		return ErrType
	}
	if value.Kind == String && (!utf8.ValidString(value.String) || uint32(len(value.String)) > limits.MaxStringBytes) { // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
		return ErrLimit
	}
	return nil
}

func validPath(path string) bool {
	if path == "" || strings.HasPrefix(path, ".") || strings.HasSuffix(path, ".") {
		return false
	}
	for _, part := range strings.Split(path, ".") {
		if part == "" {
			return false
		}
		for i, r := range part {
			if r != '_' && r != '-' && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (i == 0 || r < '0' || r > '9') {
				return false
			}
		}
	}
	return true
}

func normalizeLimits(limits Limits) (Limits, error) {
	if limits == (Limits{}) {
		return DefaultLimits(), nil
	}
	hard := DefaultLimits()
	if limits.MaxInstructions == 0 || limits.MaxInstructions > hard.MaxInstructions || limits.MaxConstants == 0 || limits.MaxConstants > hard.MaxConstants ||
		limits.MaxPaths == 0 || limits.MaxPaths > hard.MaxPaths || limits.MaxStack == 0 || limits.MaxStack > hard.MaxStack ||
		limits.MaxStringBytes == 0 || limits.MaxStringBytes > hard.MaxStringBytes || limits.MaxPatternBytes == 0 || limits.MaxPatternBytes > hard.MaxPatternBytes ||
		limits.MaxWork == 0 || limits.MaxWork > hard.MaxWork {
		return Limits{}, ErrLimit
	}
	return limits, nil
}

type wildcardToken struct {
	kind  byte
	value rune
}

const (
	wildcardLiteral byte = iota
	wildcardAny
	wildcardStar
)

// wildcardMatch implements a closed full-string glob contract. `*` matches
// zero or more Unicode scalar values, `?` matches exactly one, and `\` escapes
// only `*`, `?`, or `\`. The explicit work counter bounds retry behavior.
func wildcardMatch(ctx context.Context, text, pattern string, limits Limits, work *uint64) (bool, error) {
	if uint32(len(pattern)) > limits.MaxPatternBytes { // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
		return false, ErrLimit
	}
	tokens, err := wildcardTokens(ctx, pattern, limits, work)
	if err != nil {
		return false, err
	}
	characters := []rune(text)
	textIndex, patternIndex := 0, 0
	lastStar, starTextIndex := -1, 0
	for textIndex < len(characters) {
		if err := matchWork(ctx, work, limits.MaxWork); err != nil {
			return false, err
		}
		if patternIndex < len(tokens) && (tokens[patternIndex].kind == wildcardAny ||
			tokens[patternIndex].kind == wildcardLiteral && tokens[patternIndex].value == characters[textIndex]) {
			textIndex++
			patternIndex++
			continue
		}
		if patternIndex < len(tokens) && tokens[patternIndex].kind == wildcardStar {
			lastStar = patternIndex
			starTextIndex = textIndex
			patternIndex++
			continue
		}
		if lastStar >= 0 && starTextIndex < len(characters) {
			starTextIndex++
			textIndex = starTextIndex
			patternIndex = lastStar + 1
			continue
		}
		return false, nil
	}
	for patternIndex < len(tokens) && tokens[patternIndex].kind == wildcardStar {
		if err := matchWork(ctx, work, limits.MaxWork); err != nil {
			return false, err
		}
		patternIndex++
	}
	return patternIndex == len(tokens), nil
}

func wildcardTokens(ctx context.Context, pattern string, limits Limits, work *uint64) ([]wildcardToken, error) {
	runes := []rune(pattern)
	tokens := make([]wildcardToken, 0, len(runes))
	for index := 0; index < len(runes); index++ {
		if err := matchWork(ctx, work, limits.MaxWork); err != nil {
			return nil, err
		}
		switch runes[index] {
		case '*':
			if len(tokens) == 0 || tokens[len(tokens)-1].kind != wildcardStar {
				tokens = append(tokens, wildcardToken{kind: wildcardStar})
			}
		case '?':
			tokens = append(tokens, wildcardToken{kind: wildcardAny})
		case '\\':
			index++
			if index >= len(runes) || runes[index] != '*' && runes[index] != '?' && runes[index] != '\\' {
				return nil, fmt.Errorf("%w: wildcard escape must precede *, ?, or backslash", ErrInvalid)
			}
			tokens = append(tokens, wildcardToken{kind: wildcardLiteral, value: runes[index]})
		default:
			tokens = append(tokens, wildcardToken{kind: wildcardLiteral, value: runes[index]})
		}
	}
	return tokens, nil
}

func matchWork(ctx context.Context, work *uint64, limit uint64) error {
	if *work == ^uint64(0) {
		return ErrLimit
	}
	(*work)++
	if *work > limit {
		return ErrLimit
	}
	if *work&63 == 0 {
		return ctx.Err()
	}
	return nil
}

func equal(left, right Value) bool {
	return left.Kind == right.Kind && left.Bool == right.Bool && left.Integer == right.Integer && left.String == right.String
}

func pop1(stack *[]Value) Value {
	index := len(*stack) - 1
	value := (*stack)[index]
	*stack = (*stack)[:index]
	return value
}

func pop2(stack *[]Value) (Value, Value) {
	right := pop1(stack)
	left := pop1(stack)
	return left, right
}

func pop3(stack *[]Value) (Value, Value, Value) {
	whenFalse := pop1(stack)
	whenTrue := pop1(stack)
	condition := pop1(stack)
	return condition, whenTrue, whenFalse
}
