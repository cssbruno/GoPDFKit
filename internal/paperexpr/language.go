// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package paperexpr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// LanguageLimits bounds source parsing and the VM program produced by Compile.
// A zero value uses DefaultLanguageLimits. Nonzero limits must be complete.
type LanguageLimits struct {
	MaxSourceBytes uint32
	MaxTokens      uint32
	MaxDepth       uint32
	MaxNodes       uint32
	Program        Limits
}

func DefaultLanguageLimits() LanguageLimits {
	return LanguageLimits{
		MaxSourceBytes: 1 << 20,
		MaxTokens:      4096,
		MaxDepth:       128,
		MaxNodes:       4096,
		Program:        DefaultLimits(),
	}
}

// PathKind is an explicit static binding contract. Compile rejects duplicate,
// invalid, undeclared, or unsupported path kinds before producing bytecode.
type PathKind struct {
	Path string
	Kind Kind
}

// Expression is an immutable parsed expression. Its syntax tree is intentionally
// private so callers cannot construct an unbounded or invalid tree.
type Expression struct {
	root       *expressionNode
	source     string
	tokenCount uint32
	nodeCount  uint32
	maxDepth   uint32
}

func (e Expression) Source() string { return e.source }

// ExpressionError identifies the byte-offset range responsible for a parse or
// compile failure. Cause is one of ErrInvalid, ErrType, ErrBinding, or ErrLimit.
type ExpressionError struct {
	Offset  uint32
	End     uint32
	Problem string
	Cause   error
}

func (e *ExpressionError) Error() string {
	return fmt.Sprintf("%v at bytes %d:%d: %s", e.Cause, e.Offset, e.End, e.Problem)
}

func (e *ExpressionError) Unwrap() error { return e.Cause }

// Parse accepts this precedence, from tightest to loosest:
// primary/parentheses, !, typed +, ==/matches, &&, ||, and right-associative ?:.
func Parse(source string, limits LanguageLimits) (Expression, error) {
	normalized, err := normalizeLanguageLimits(limits)
	if err != nil {
		return Expression{}, expressionError(0, 0, "language limits are incomplete or exceed hard caps", ErrLimit)
	}
	if uint64(len(source)) > uint64(normalized.MaxSourceBytes) {
		return Expression{}, expressionError(normalized.MaxSourceBytes, boundedSourceOffset(len(source)), "source exceeds MaxSourceBytes", ErrLimit)
	}
	if !utf8.ValidString(source) {
		return Expression{}, expressionError(0, uint32(len(source)), "source is not valid UTF-8", ErrInvalid)
	}
	tokens, err := lexExpression(source, normalized)
	if err != nil {
		return Expression{}, err
	}
	parser := expressionParser{tokens: tokens, limits: normalized}
	root, err := parser.parseSelect(1)
	if err != nil {
		return Expression{}, err
	}
	if token := parser.peek(); token.kind != tokenEOF {
		return Expression{}, expressionError(token.start, token.end, fmt.Sprintf("unexpected token %q after expression", token.text), ErrInvalid)
	}
	return Expression{
		root:       root,
		source:     source,
		tokenCount: uint32(len(tokens) - 1),
		nodeCount:  parser.nodes,
		maxDepth:   parser.peakDepth,
	}, nil
}

// Compile parses, statically checks, and emits deterministic VM bytecode. The
// returned Kind is the expression result kind.
func Compile(source string, environment []PathKind, limits LanguageLimits) (Program, Kind, error) {
	expression, err := Parse(source, limits)
	if err != nil {
		return Program{}, Null, err
	}
	return CompileExpression(expression, environment, limits)
}

// CompileExpression statically checks a previously parsed expression and emits
// sorted/deduplicated constants and paths plus deterministic postfix bytecode.
func CompileExpression(expression Expression, environment []PathKind, limits LanguageLimits) (Program, Kind, error) {
	normalized, err := normalizeLanguageLimits(limits)
	if err != nil {
		return Program{}, Null, expressionError(0, 0, "language limits are incomplete or exceed hard caps", ErrLimit)
	}
	if expression.root == nil {
		return Program{}, Null, expressionError(0, 0, "expression has no parsed syntax tree", ErrInvalid)
	}
	if uint64(len(expression.source)) > uint64(normalized.MaxSourceBytes) || expression.tokenCount > normalized.MaxTokens ||
		expression.nodeCount > normalized.MaxNodes || expression.maxDepth > normalized.MaxDepth {
		return Program{}, Null, expressionError(expression.root.start, expression.root.end, "parsed expression exceeds compile limits", ErrLimit)
	}
	kinds, err := normalizePathKinds(environment, normalized.Program)
	if err != nil {
		return Program{}, Null, err
	}
	compiler := expressionCompiler{pathKinds: kinds, limits: normalized.Program}
	resultKind, err := compiler.check(expression.root)
	if err != nil {
		return Program{}, Null, err
	}
	compiler.indexInputs(expression.root)
	if uint32(len(compiler.constants)) > normalized.Program.MaxConstants {
		return Program{}, Null, expressionError(expression.root.start, expression.root.end, "constant count exceeds MaxConstants", ErrLimit)
	}
	if uint32(len(compiler.paths)) > normalized.Program.MaxPaths {
		return Program{}, Null, expressionError(expression.root.start, expression.root.end, "path count exceeds MaxPaths", ErrLimit)
	}
	program := Program{Constants: compiler.constants, Paths: compiler.paths}
	compiler.emit(expression.root, &program.Code)
	if uint32(len(program.Code)) > normalized.Program.MaxInstructions {
		return Program{}, Null, expressionError(expression.root.start, expression.root.end, "instruction count exceeds MaxInstructions", ErrLimit)
	}
	if _, err := validateProgram(program, normalized.Program); err != nil {
		return Program{}, Null, expressionError(expression.root.start, expression.root.end, "generated program exceeds VM limits", err)
	}
	return program, resultKind, nil
}

type tokenKind uint8

const (
	tokenEOF tokenKind = iota
	tokenNull
	tokenBool
	tokenInteger
	tokenString
	tokenPath
	tokenLeftParen
	tokenRightParen
	tokenNot
	tokenEqual
	tokenMatches
	tokenAnd
	tokenOr
	tokenPlus
	tokenQuestion
	tokenColon
)

type expressionToken struct {
	kind       tokenKind
	start, end uint32
	text       string
	value      Value
}

func lexExpression(source string, limits LanguageLimits) ([]expressionToken, error) {
	tokens := make([]expressionToken, 0)
	for offset := 0; offset < len(source); {
		character := source[offset]
		if character == ' ' || character == '\t' || character == '\r' || character == '\n' {
			offset++
			continue
		}
		start := offset
		var token expressionToken
		switch character {
		case '(':
			offset++
			token = simpleToken(tokenLeftParen, source, start, offset)
		case ')':
			offset++
			token = simpleToken(tokenRightParen, source, start, offset)
		case '+':
			offset++
			token = simpleToken(tokenPlus, source, start, offset)
		case '?':
			offset++
			token = simpleToken(tokenQuestion, source, start, offset)
		case ':':
			offset++
			token = simpleToken(tokenColon, source, start, offset)
		case '!':
			offset++
			token = simpleToken(tokenNot, source, start, offset)
		case '=':
			if offset+1 >= len(source) || source[offset+1] != '=' {
				return nil, expressionError(uint32(start), uint32(start+1), "expected ==", ErrInvalid)
			}
			offset += 2
			token = simpleToken(tokenEqual, source, start, offset)
		case '&':
			if offset+1 >= len(source) || source[offset+1] != '&' {
				return nil, expressionError(uint32(start), uint32(start+1), "expected &&", ErrInvalid)
			}
			offset += 2
			token = simpleToken(tokenAnd, source, start, offset)
		case '|':
			if offset+1 >= len(source) || source[offset+1] != '|' {
				return nil, expressionError(uint32(start), uint32(start+1), "expected ||", ErrInvalid)
			}
			offset += 2
			token = simpleToken(tokenOr, source, start, offset)
		case '"':
			var err error
			token, offset, err = lexExpressionString(source, start)
			if err != nil {
				return nil, err
			}
		default:
			if character == '-' || character >= '0' && character <= '9' {
				var err error
				token, offset, err = lexExpressionInteger(source, start)
				if err != nil {
					return nil, err
				}
			} else if isPathStart(character) {
				for offset < len(source) && isPathByte(source[offset]) {
					offset++
				}
				text := source[start:offset]
				switch text {
				case "null":
					token = expressionToken{kind: tokenNull, start: uint32(start), end: uint32(offset), text: text, value: Value{Kind: Null}}
				case "true":
					token = expressionToken{kind: tokenBool, start: uint32(start), end: uint32(offset), text: text, value: Value{Kind: Bool, Bool: true}}
				case "false":
					token = expressionToken{kind: tokenBool, start: uint32(start), end: uint32(offset), text: text, value: Value{Kind: Bool}}
				case "matches":
					token = expressionToken{kind: tokenMatches, start: uint32(start), end: uint32(offset), text: text}
				default:
					if !validPath(text) {
						return nil, expressionError(uint32(start), uint32(offset), "invalid dotted binding path", ErrInvalid)
					}
					token = expressionToken{kind: tokenPath, start: uint32(start), end: uint32(offset), text: text}
				}
			} else {
				_, size := utf8.DecodeRuneInString(source[offset:])
				return nil, expressionError(uint32(start), uint32(start+size), fmt.Sprintf("unexpected character %q", source[start:start+size]), ErrInvalid)
			}
		}
		tokens = append(tokens, token)
		if uint32(len(tokens)) > limits.MaxTokens {
			return nil, expressionError(token.start, token.end, "token count exceeds MaxTokens", ErrLimit)
		}
	}
	tokens = append(tokens, expressionToken{kind: tokenEOF, start: uint32(len(source)), end: uint32(len(source))})
	return tokens, nil
}

func simpleToken(kind tokenKind, source string, start, end int) expressionToken {
	return expressionToken{kind: kind, start: uint32(start), end: uint32(end), text: source[start:end]}
}

func lexExpressionInteger(source string, start int) (expressionToken, int, error) {
	offset := start
	if source[offset] == '-' {
		offset++
		if offset >= len(source) || source[offset] < '0' || source[offset] > '9' {
			return expressionToken{}, offset, expressionError(uint32(start), uint32(offset), "minus must begin an integer literal", ErrInvalid)
		}
	}
	for offset < len(source) && source[offset] >= '0' && source[offset] <= '9' {
		offset++
	}
	raw := source[start:offset]
	digits := strings.TrimPrefix(raw, "-")
	if len(digits) > 1 && digits[0] == '0' || raw == "-0" {
		return expressionToken{}, offset, expressionError(uint32(start), uint32(offset), "integer must use canonical base-10 notation", ErrInvalid)
	}
	integer, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return expressionToken{}, offset, expressionError(uint32(start), uint32(offset), "integer is outside int64 range", ErrInvalid)
	}
	return expressionToken{kind: tokenInteger, start: uint32(start), end: uint32(offset), text: raw, value: Value{Kind: Integer, Integer: integer}}, offset, nil
}

func lexExpressionString(source string, start int) (expressionToken, int, error) {
	offset := start + 1
	for offset < len(source) {
		character := source[offset]
		if character == '"' {
			offset++
			raw := source[start:offset]
			if err := validateJSONStringEscapes(raw); err != nil {
				return expressionToken{}, offset, expressionError(uint32(start), uint32(offset), err.Error(), ErrInvalid)
			}
			var decoded string
			if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
				return expressionToken{}, offset, expressionError(uint32(start), uint32(offset), "invalid quoted UTF-8 string", ErrInvalid)
			}
			return expressionToken{kind: tokenString, start: uint32(start), end: uint32(offset), text: raw, value: Value{Kind: String, String: decoded}}, offset, nil
		}
		if character < 0x20 {
			return expressionToken{}, offset, expressionError(uint32(offset), uint32(offset+1), "unescaped control character in string", ErrInvalid)
		}
		if character == '\\' {
			offset += 2
			continue
		}
		_, size := utf8.DecodeRuneInString(source[offset:])
		offset += size
	}
	return expressionToken{}, offset, expressionError(uint32(start), uint32(len(source)), "unterminated string literal", ErrInvalid)
}

func validateJSONStringEscapes(raw string) error {
	for index := 1; index < len(raw)-1; index++ {
		if raw[index] != '\\' {
			continue
		}
		index++
		if index >= len(raw)-1 {
			return errors.New("incomplete string escape")
		}
		switch raw[index] {
		case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
		case 'u':
			if index+4 >= len(raw) {
				return errors.New("incomplete Unicode escape")
			}
			first, ok := parseHex16(raw[index+1 : index+5])
			if !ok {
				return errors.New("invalid Unicode escape")
			}
			index += 4
			if first >= 0xd800 && first <= 0xdbff {
				if index+6 >= len(raw) || raw[index+1:index+3] != `\u` {
					return errors.New("high surrogate requires a low surrogate")
				}
				second, ok := parseHex16(raw[index+3 : index+7])
				if !ok || second < 0xdc00 || second > 0xdfff {
					return errors.New("high surrogate requires a low surrogate")
				}
				index += 6
			} else if first >= 0xdc00 && first <= 0xdfff {
				return errors.New("low surrogate has no high surrogate")
			}
		default:
			return fmt.Errorf("unsupported string escape \\%c", raw[index])
		}
	}
	return nil
}

func parseHex16(value string) (uint16, bool) {
	parsed, err := strconv.ParseUint(value, 16, 16)
	return uint16(parsed), err == nil
}

func isPathStart(character byte) bool {
	return character == '_' || character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z'
}

func isPathByte(character byte) bool {
	return isPathStart(character) || character >= '0' && character <= '9' || character == '-' || character == '.'
}

type expressionNodeKind uint8

const (
	nodeLiteral expressionNodeKind = iota
	nodePath
	nodeNot
	nodeEqual
	nodeMatches
	nodeAnd
	nodeOr
	nodePlus
	nodeSelect
)

type expressionNode struct {
	kind             expressionNodeKind
	start, end       uint32
	opOffset         uint32
	value            Value
	path             string
	left, right, alt *expressionNode
	inferred         Kind
	height           uint32
}

type expressionParser struct {
	tokens    []expressionToken
	index     int
	nodes     uint32
	peakDepth uint32
	limits    LanguageLimits
}

func (p *expressionParser) parseSelect(depth uint32) (*expressionNode, error) {
	if err := p.checkDepth(depth); err != nil {
		return nil, err
	}
	condition, err := p.parseOr(depth)
	if err != nil || p.peek().kind != tokenQuestion {
		return condition, err
	}
	operator := p.take()
	whenTrue, err := p.parseSelect(depth + 1)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(tokenColon, "conditional expression requires :"); err != nil {
		return nil, err
	}
	whenFalse, err := p.parseSelect(depth + 1)
	if err != nil {
		return nil, err
	}
	return p.node(nodeSelect, condition.start, whenFalse.end, operator.start, condition, whenTrue, whenFalse)
}

func (p *expressionParser) parseOr(depth uint32) (*expressionNode, error) {
	return p.parseBinary(depth, p.parseAnd, tokenOr, nodeOr)
}

func (p *expressionParser) parseAnd(depth uint32) (*expressionNode, error) {
	return p.parseBinary(depth, p.parseEqual, tokenAnd, nodeAnd)
}

func (p *expressionParser) parseEqual(depth uint32) (*expressionNode, error) {
	left, err := p.parsePlus(depth)
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tokenEqual || p.peek().kind == tokenMatches {
		operator := p.take()
		right, err := p.parsePlus(depth)
		if err != nil {
			return nil, err
		}
		kind := nodeEqual
		if operator.kind == tokenMatches {
			kind = nodeMatches
		}
		left, err = p.node(kind, left.start, right.end, operator.start, left, right, nil)
		if err != nil {
			return nil, err
		}
	}
	return left, nil
}

func (p *expressionParser) parsePlus(depth uint32) (*expressionNode, error) {
	return p.parseBinary(depth, p.parseUnary, tokenPlus, nodePlus)
}

type parseLevel func(uint32) (*expressionNode, error)

func (p *expressionParser) parseBinary(depth uint32, next parseLevel, tokenKind tokenKind, nodeKind expressionNodeKind) (*expressionNode, error) {
	left, err := next(depth)
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tokenKind {
		operator := p.take()
		right, err := next(depth)
		if err != nil {
			return nil, err
		}
		left, err = p.node(nodeKind, left.start, right.end, operator.start, left, right, nil)
		if err != nil {
			return nil, err
		}
	}
	return left, nil
}

func (p *expressionParser) parseUnary(depth uint32) (*expressionNode, error) {
	if token := p.peek(); token.kind == tokenNot {
		p.take()
		child, err := p.parseUnary(depth + 1)
		if err != nil {
			return nil, err
		}
		return p.node(nodeNot, token.start, child.end, token.start, child, nil, nil)
	}
	return p.parsePrimary(depth)
}

func (p *expressionParser) parsePrimary(depth uint32) (*expressionNode, error) {
	if err := p.checkDepth(depth); err != nil {
		return nil, err
	}
	token := p.take()
	switch token.kind {
	case tokenNull, tokenBool, tokenInteger, tokenString:
		return p.leaf(nodeLiteral, token)
	case tokenPath:
		node, err := p.leaf(nodePath, token)
		if err == nil {
			node.path = token.text
		}
		return node, err
	case tokenLeftParen:
		node, err := p.parseSelect(depth + 1)
		if err != nil {
			return nil, err
		}
		closing, err := p.expect(tokenRightParen, "opening parenthesis requires )")
		if err != nil {
			return nil, err
		}
		node.start, node.end = token.start, closing.end
		return node, nil
	case tokenEOF:
		return nil, expressionError(token.start, token.end, "expected expression", ErrInvalid)
	default:
		return nil, expressionError(token.start, token.end, fmt.Sprintf("unexpected token %q", token.text), ErrInvalid)
	}
}

func (p *expressionParser) leaf(kind expressionNodeKind, token expressionToken) (*expressionNode, error) {
	p.nodes++
	if p.nodes > p.limits.MaxNodes {
		return nil, expressionError(token.start, token.end, "node count exceeds MaxNodes", ErrLimit)
	}
	return &expressionNode{kind: kind, start: token.start, end: token.end, value: token.value, height: 1}, nil
}

func (p *expressionParser) node(kind expressionNodeKind, start, end, op uint32, left, right, alt *expressionNode) (*expressionNode, error) {
	p.nodes++
	if p.nodes > p.limits.MaxNodes {
		return nil, expressionError(start, end, "node count exceeds MaxNodes", ErrLimit)
	}
	height := uint32(1)
	for _, child := range []*expressionNode{left, right, alt} {
		if child != nil && child.height+1 > height {
			height = child.height + 1
		}
	}
	if height > p.limits.MaxDepth {
		return nil, expressionError(start, end, "expression depth exceeds MaxDepth", ErrLimit)
	}
	if height > p.peakDepth {
		p.peakDepth = height
	}
	return &expressionNode{kind: kind, start: start, end: end, opOffset: op, left: left, right: right, alt: alt, height: height}, nil
}

func (p *expressionParser) checkDepth(depth uint32) error {
	if depth > p.limits.MaxDepth {
		token := p.peek()
		return expressionError(token.start, token.end, "parse nesting exceeds MaxDepth", ErrLimit)
	}
	if depth > p.peakDepth {
		p.peakDepth = depth
	}
	return nil
}

func (p *expressionParser) expect(kind tokenKind, problem string) (expressionToken, error) {
	token := p.peek()
	if token.kind != kind {
		return expressionToken{}, expressionError(token.start, token.end, problem, ErrInvalid)
	}
	p.index++
	return token, nil
}

func (p *expressionParser) peek() expressionToken { return p.tokens[p.index] }

func (p *expressionParser) take() expressionToken {
	token := p.tokens[p.index]
	if token.kind != tokenEOF {
		p.index++
	}
	return token
}

type expressionCompiler struct {
	pathKinds     map[string]Kind
	limits        Limits
	constants     []Value
	paths         []string
	constantIndex map[Value]uint32
	pathIndex     map[string]uint32
}

func (c *expressionCompiler) check(node *expressionNode) (Kind, error) {
	switch node.kind {
	case nodeLiteral:
		if node.value.Kind == String && uint32(len(node.value.String)) > c.limits.MaxStringBytes {
			return Null, expressionError(node.start, node.end, "string literal exceeds MaxStringBytes", ErrLimit)
		}
		node.inferred = node.value.Kind
	case nodePath:
		kind, exists := c.pathKinds[node.path]
		if !exists {
			return Null, expressionError(node.start, node.end, fmt.Sprintf("binding path %q is not declared", node.path), ErrBinding)
		}
		node.inferred = kind
	case nodeNot:
		kind, err := c.check(node.left)
		if err != nil {
			return Null, err
		}
		if kind != Bool {
			return Null, expressionError(node.opOffset, node.opOffset+1, "! requires bool", ErrType)
		}
		node.inferred = Bool
	case nodeEqual:
		left, right, err := c.checkPair(node)
		if err != nil {
			return Null, err
		}
		if left != right {
			return Null, expressionError(node.opOffset, node.opOffset+2, "== operands must have the same static kind", ErrType)
		}
		node.inferred = Bool
	case nodeMatches:
		left, right, err := c.checkPair(node)
		if err != nil {
			return Null, err
		}
		if left != String || right != String {
			return Null, expressionError(node.opOffset, node.opOffset+7, "matches requires two strings", ErrType)
		}
		if node.right.kind == nodeLiteral {
			if uint32(len(node.right.value.String)) > c.limits.MaxPatternBytes {
				return Null, expressionError(node.right.start, node.right.end, "match pattern exceeds MaxPatternBytes", ErrLimit)
			}
			work := uint64(0)
			if _, err := wildcardTokens(context.Background(), node.right.value.String, c.limits, &work); err != nil {
				return Null, expressionError(node.right.start, node.right.end, err.Error(), err)
			}
		}
		node.inferred = Bool
	case nodeAnd, nodeOr:
		left, right, err := c.checkPair(node)
		if err != nil {
			return Null, err
		}
		if left != Bool || right != Bool {
			return Null, expressionError(node.opOffset, node.opOffset+2, "boolean operator requires bool operands", ErrType)
		}
		node.inferred = Bool
	case nodePlus:
		left, right, err := c.checkPair(node)
		if err != nil {
			return Null, err
		}
		if left != right || left != Integer && left != String {
			return Null, expressionError(node.opOffset, node.opOffset+1, "+ requires two integers or two strings", ErrType)
		}
		node.inferred = left
	case nodeSelect:
		condition, err := c.check(node.left)
		if err != nil {
			return Null, err
		}
		whenTrue, err := c.check(node.right)
		if err != nil {
			return Null, err
		}
		whenFalse, err := c.check(node.alt)
		if err != nil {
			return Null, err
		}
		if condition != Bool {
			return Null, expressionError(node.opOffset, node.opOffset+1, "conditional condition must be bool", ErrType)
		}
		if whenTrue != whenFalse {
			return Null, expressionError(node.opOffset, node.opOffset+1, "conditional branches must have the same static kind", ErrType)
		}
		node.inferred = whenTrue
	default:
		return Null, expressionError(node.start, node.end, "unknown expression node", ErrInvalid)
	}
	return node.inferred, nil
}

func (c *expressionCompiler) checkPair(node *expressionNode) (Kind, Kind, error) {
	left, err := c.check(node.left)
	if err != nil {
		return Null, Null, err
	}
	right, err := c.check(node.right)
	return left, right, err
}

func (c *expressionCompiler) indexInputs(root *expressionNode) {
	constantSet := make(map[Value]bool)
	pathSet := make(map[string]bool)
	var walk func(*expressionNode)
	walk = func(node *expressionNode) {
		if node == nil {
			return
		}
		if node.kind == nodeLiteral {
			constantSet[node.value] = true
		} else if node.kind == nodePath {
			pathSet[node.path] = true
		}
		walk(node.left)
		walk(node.right)
		walk(node.alt)
	}
	walk(root)
	for value := range constantSet {
		c.constants = append(c.constants, value)
	}
	sort.Slice(c.constants, func(i, j int) bool { return lessExpressionValue(c.constants[i], c.constants[j]) })
	c.constantIndex = make(map[Value]uint32, len(c.constants))
	for index, value := range c.constants {
		c.constantIndex[value] = uint32(index)
	}
	for path := range pathSet {
		c.paths = append(c.paths, path)
	}
	sort.Strings(c.paths)
	c.pathIndex = make(map[string]uint32, len(c.paths))
	for index, path := range c.paths {
		c.pathIndex[path] = uint32(index)
	}
}

func (c *expressionCompiler) emit(node *expressionNode, code *[]Instruction) {
	switch node.kind {
	case nodeLiteral:
		*code = append(*code, Instruction{Op: OpConstant, Arg: c.constantIndex[node.value]})
	case nodePath:
		*code = append(*code, Instruction{Op: OpLoad, Arg: c.pathIndex[node.path]})
	case nodeNot:
		c.emit(node.left, code)
		*code = append(*code, Instruction{Op: OpNot})
	case nodeEqual, nodeMatches, nodeAnd, nodeOr, nodePlus:
		c.emit(node.left, code)
		c.emit(node.right, code)
		op := OpEqual
		switch node.kind {
		case nodeAnd:
			op = OpAnd
		case nodeMatches:
			op = OpMatches
		case nodeOr:
			op = OpOr
		case nodePlus:
			if node.inferred == Integer {
				op = OpAddInteger
			} else {
				op = OpConcat
			}
		}
		*code = append(*code, Instruction{Op: op})
	case nodeSelect:
		c.emit(node.left, code)
		c.emit(node.right, code)
		c.emit(node.alt, code)
		*code = append(*code, Instruction{Op: OpSelect})
	}
}

func lessExpressionValue(left, right Value) bool {
	if left.Kind != right.Kind {
		return left.Kind < right.Kind
	}
	switch left.Kind {
	case Bool:
		return !left.Bool && right.Bool
	case Integer:
		return left.Integer < right.Integer
	case String:
		return left.String < right.String
	default:
		return false
	}
}

func normalizePathKinds(environment []PathKind, limits Limits) (map[string]Kind, error) {
	result := make(map[string]Kind, len(environment))
	for _, entry := range environment {
		if !validPath(entry.Path) || uint32(len(entry.Path)) > limits.MaxStringBytes {
			return nil, expressionError(0, 0, fmt.Sprintf("invalid environment path %q", entry.Path), ErrBinding)
		}
		if entry.Kind > String {
			return nil, expressionError(0, 0, fmt.Sprintf("invalid kind for %q", entry.Path), ErrType)
		}
		if _, duplicate := result[entry.Path]; duplicate {
			return nil, expressionError(0, 0, fmt.Sprintf("duplicate environment path %q", entry.Path), ErrBinding)
		}
		result[entry.Path] = entry.Kind
	}
	return result, nil
}

func normalizeLanguageLimits(limits LanguageLimits) (LanguageLimits, error) {
	if limits == (LanguageLimits{}) {
		return DefaultLanguageLimits(), nil
	}
	hard := DefaultLanguageLimits()
	if limits.MaxSourceBytes == 0 || limits.MaxSourceBytes > hard.MaxSourceBytes ||
		limits.MaxTokens == 0 || limits.MaxTokens > hard.MaxTokens ||
		limits.MaxDepth == 0 || limits.MaxDepth > hard.MaxDepth ||
		limits.MaxNodes == 0 || limits.MaxNodes > hard.MaxNodes {
		return LanguageLimits{}, ErrLimit
	}
	program, err := normalizeLimits(limits.Program)
	if err != nil {
		return LanguageLimits{}, err
	}
	limits.Program = program
	return limits, nil
}

func expressionError(start, end uint32, problem string, cause error) error {
	return &ExpressionError{Offset: start, End: end, Problem: problem, Cause: cause}
}

func boundedSourceOffset(offset int) uint32 {
	if uint64(offset) > uint64(^uint32(0)) {
		return ^uint32(0)
	}
	return uint32(offset)
}
