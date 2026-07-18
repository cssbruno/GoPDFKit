// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperlang

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseResult contains a best-effort AST and all lexer/parser diagnostics.
type ParseResult struct {
	AST         AST
	Diagnostics []Diagnostic
}

// OK reports whether parsing completed without error diagnostics and produced
// one document root.
func (r ParseResult) OK() bool {
	if r.AST.Root == nil {
		return false
	}
	for _, diagnostic := range r.Diagnostics {
		if diagnostic.Severity == SeverityError {
			return false
		}
	}
	return true
}

// Parse lexes and parses one .paper source with recovery at line boundaries.
func Parse(file, source string) ParseResult {
	lexed := Lex(file, source)
	parser := paperParser{
		file: file, tokens: lexed.Tokens, diagnostics: append([]Diagnostic(nil), lexed.Diagnostics...),
		ids: make(map[string]Span),
	}
	parser.skipNewlines()
	root := parser.parseNode(0)
	if root != nil && root.Kind != NodeDocument {
		parser.add("PAPER_ROOT_DOCUMENT", "the root node must be document", "start the file with document @id:", root.HeaderSpan)
	}
	parser.skipNewlines()
	for !parser.at(TokenEOF) {
		token := parser.current()
		parser.add("PAPER_MULTIPLE_ROOTS", "only one document root is allowed", "indent this node under the document or move it to another file", token.Span)
		parser.skipLine()
		parser.skipNewlines()
	}
	if root == nil {
		span := Span{File: file, Start: Position{Line: 1, Column: 1}, End: Position{Line: 1, Column: 1}}
		if len(lexed.Tokens) != 0 {
			span = lexed.Tokens[len(lexed.Tokens)-1].Span
		}
		parser.add("PAPER_EMPTY_DOCUMENT", "the .paper source has no document node", "add document @id: as the root", span)
	}
	return ParseResult{AST: AST{File: file, Root: root}, Diagnostics: parser.diagnostics}
}

type paperParser struct {
	file        string
	tokens      []Token
	index       int
	diagnostics []Diagnostic
	ids         map[string]Span
}

func (p *paperParser) parseNode(depth int) *Node {
	if p.at(TokenEOF) || p.at(TokenDedent) {
		return nil
	}
	if !p.at(TokenIdentifier) {
		p.add("PAPER_EXPECTED_NODE", "expected a document component name", "use document, page, body, row, column, heading, paragraph, list, item, page-break, or text", p.current().Span)
		p.skipLine()
		return nil
	}
	name := p.advance()
	kind, isNode := parseNodeKind(name.Lexeme)
	if !isNode {
		p.add("PAPER_ROOT_PROPERTY", "a property cannot appear where a document node is required", "place this property inside a node", name.Span)
		p.skipLine()
		return nil
	}
	node := &Node{Kind: kind}
	headerStart := name.Span.Start
	if p.at(TokenReadableID) {
		id := p.advance()
		node.ID = id.Lexeme
		// Slot/fill/field and fixture names are scoped to their owning block
		// rather than being document-global durable node identities.
		if !scopedReadableID(kind) {
			if _, exists := p.ids[node.ID]; exists {
				p.add("PAPER_DUPLICATE_ID", fmt.Sprintf("readable ID %s is already used", node.ID), "choose a unique @id so tools can target this node", id.Span)
			} else {
				p.ids[node.ID] = id.Span
			}
		}
	} else if p.at(TokenInvalid) && strings.HasPrefix(p.current().Lexeme, "@") {
		p.advance()
	}
	if !p.consume(TokenColon) {
		p.add("PAPER_EXPECTED_COLON", fmt.Sprintf("%s declaration requires ':'", kind), "add ':' after the node name or @id", p.current().Span)
	}
	if isScalarToken(p.current().Kind) {
		value := p.parseScalar()
		node.Value = &value
	}
	lineEnd := p.current().Span.Start
	if p.at(TokenInvalid) {
		p.advance()
	}
	if !p.at(TokenNewline) && !p.at(TokenEOF) {
		p.add("PAPER_TRAILING_TOKENS", "unexpected tokens after declaration", "keep one scalar value per line", p.current().Span)
		p.skipLine()
	} else if p.at(TokenNewline) {
		lineEnd = p.advance().Span.Start
	}
	node.HeaderSpan = Span{File: p.file, Start: headerStart, End: lineEnd}
	node.Span = node.HeaderSpan

	// Blank and comment-only physical lines are trivia. The lexer represents
	// them as newlines without indentation tokens, so skip them before deciding
	// whether the declaration owns an indented block.
	p.skipNewlines()
	if p.at(TokenIndent) {
		p.advance()
		for !p.at(TokenDedent) && !p.at(TokenEOF) {
			if p.at(TokenNewline) {
				p.advance()
				continue
			}
			member := p.parseMember(depth + 1)
			if member.Node != nil || member.Property != nil {
				node.Members = append(node.Members, member)
				memberSpan := memberSpan(member)
				if memberSpan.End.Offset > node.Span.End.Offset {
					node.Span.End = memberSpan.End
				}
			}
		}
		p.consume(TokenDedent)
	}
	p.validateNode(node, depth)
	return node
}

func (p *paperParser) parseMember(depth int) Member {
	if !p.at(TokenIdentifier) {
		p.add("PAPER_EXPECTED_MEMBER", "expected a child node or property", "start the line with a component or property name", p.current().Span)
		p.skipLine()
		return Member{}
	}
	_, isNode := parseNodeKind(p.current().Lexeme)
	// `component: "@name"` is the readable reference property inside a use;
	// a component definition remains unambiguous as `component @name:` (or a
	// block-valued `component:`) because no scalar follows its colon.
	isComponentReference := p.current().Lexeme == string(NodeComponent) &&
		p.peek(1).Kind == TokenColon && isScalarToken(p.peek(2).Kind)
	isScenarioReference := p.current().Lexeme == string(NodeScenario) &&
		p.peek(1).Kind == TokenColon && isScalarToken(p.peek(2).Kind)
	// Fixture declarations always carry an @name. Keeping the words contextual
	// means existing properties such as `value: "..."` remain unambiguous.
	isContextualDeclarationProperty := isContextualNodeKind(NodeKind(p.current().Lexeme)) && p.peek(1).Kind != TokenReadableID
	if isNode && !isComponentReference && !isScenarioReference && !isContextualDeclarationProperty {
		return Member{Node: p.parseNode(depth)}
	}
	return Member{Property: p.parseProperty()}
}

func (p *paperParser) peek(offset int) Token {
	index := p.index + offset
	if index < 0 || index >= len(p.tokens) {
		return Token{Kind: TokenEOF, Span: Span{File: p.file}}
	}
	return p.tokens[index]
}

func (p *paperParser) parseProperty() *Property {
	name := p.advance()
	property := &Property{Name: name.Lexeme}
	if p.at(TokenReadableID) {
		id := p.advance()
		p.add("PAPER_PROPERTY_ID", "properties cannot have @ids", "remove the @id or use a document component", id.Span)
	}
	if !p.consume(TokenColon) {
		p.add("PAPER_EXPECTED_COLON", fmt.Sprintf("property %q requires ':'", name.Lexeme), "add ':' between the property name and value", p.current().Span)
	}
	if isScalarToken(p.current().Kind) {
		property.Value = p.parseScalar()
	} else {
		p.add("PAPER_EXPECTED_VALUE", fmt.Sprintf("property %q requires a scalar value", name.Lexeme), "use a quoted string, boolean, number, or unit such as 12pt", p.current().Span)
		property.Value = Scalar{Kind: ScalarString, Raw: "", Span: p.current().Span}
	}
	end := property.Value.Span.End
	if !p.at(TokenNewline) && !p.at(TokenEOF) {
		p.add("PAPER_TRAILING_TOKENS", "unexpected tokens after property value", "keep one scalar value per line", p.current().Span)
		p.skipLine()
	} else if p.at(TokenNewline) {
		p.advance()
	}
	property.Span = Span{File: p.file, Start: name.Span.Start, End: end}
	if p.at(TokenIndent) {
		p.add("PAPER_PROPERTY_BLOCK", "properties cannot own an indented block", "dedent the following lines or make this a component", p.current().Span)
		p.skipIndentedBlock()
	}
	return property
}

func (p *paperParser) parseScalar() Scalar {
	token := p.advance()
	value := Scalar{Raw: token.Lexeme, Span: token.Span}
	switch token.Kind {
	case TokenString:
		value.Kind = ScalarString
		decoded, err := strconv.Unquote(token.Lexeme)
		if err != nil {
			p.add("PAPER_INVALID_STRING", "string contains an invalid escape", `use escapes such as \n, \t, \" or \\`, token.Span)
			decoded = token.Lexeme
		}
		value.StringValue = &decoded
	case TokenBool:
		value.Kind = ScalarBool
		parsed := token.Lexeme == "true"
		value.BoolValue = &parsed
	case TokenNumber:
		value.Kind = ScalarNumber
		parsed, err := strconv.ParseFloat(token.Lexeme, 64)
		if err != nil {
			p.add("PAPER_INVALID_NUMBER", "number is outside the supported finite range", "use a finite decimal number", token.Span)
		}
		value.NumberValue = &parsed
	case TokenUnit:
		value.Kind = ScalarUnit
		numberText, unit := splitUnit(token.Lexeme)
		parsed, err := strconv.ParseFloat(numberText, 64)
		if err != nil || !validUnit(unit) {
			p.add("PAPER_INVALID_UNIT", fmt.Sprintf("%q is not a supported unit value", token.Lexeme), "use pt, mm, cm, in, px, pc, em, rem, vh, vw, or %", token.Span)
		}
		value.UnitValue = &UnitValue{Number: parsed, Unit: unit}
	case TokenNull:
		value.Kind = ScalarNull
	}
	return value
}

func (p *paperParser) validateNode(node *Node, depth int) {
	if node.Kind == NodeText || node.Kind == NodeValue || node.Kind == NodeArg {
		if node.Value == nil {
			code, subject, hint := "PAPER_TEXT_VALUE", "text", "write text: \"Your content\""
			switch node.Kind {
			case NodeValue:
				code, subject, hint = "PAPER_SCENARIO_VALUE", "scenario value", "write value @name: \"content\""
			case NodeArg:
				code, subject, hint = "PAPER_COMPONENT_ARG_VALUE", "component argument", "write arg @name: value"
			}
			p.add(code, subject+" requires an inline scalar value", hint, node.HeaderSpan)
		}
		if len(node.Members) != 0 {
			code, subject, hint := "PAPER_TEXT_BLOCK", "text", "move properties to paragraph or heading"
			switch node.Kind {
			case NodeValue:
				code, subject, hint = "PAPER_SCENARIO_VALUE_BLOCK", "scenario value", "use object or keyed-list for nested fixture data"
			case NodeArg:
				code, subject, hint = "PAPER_COMPONENT_ARG_BLOCK", "component argument", "arguments are scalar leaves"
			}
			p.add(code, subject+" cannot contain an indented block", hint, node.Span)
		}
	} else if node.Kind == NodePageBreak {
		if node.Value != nil {
			p.add("PAPER_NODE_VALUE", "page-break does not accept an inline scalar", "write page-break: on a line by itself", node.Value.Span)
		}
		if len(node.Members) != 0 {
			p.add("PAPER_PAGE_BREAK_BLOCK", "page-break cannot contain an indented block", "write page-break: on a line by itself", node.Span)
		}
	} else if node.Value != nil {
		p.add("PAPER_NODE_VALUE", fmt.Sprintf("%s does not accept an inline scalar", node.Kind), "move the value into a text child or named property", node.Value.Span)
	}
	if node.Kind != NodeText && node.Kind != NodeValue && node.Kind != NodeArg && node.Kind != NodePageBreak && node.Kind != NodeScenario && node.Kind != NodeObject && node.Kind != NodeKeyedList && node.Kind != NodeTheme && node.Kind != NodeStyle && node.Kind != NodeScope && len(node.Members) == 0 {
		p.add("PAPER_EMPTY_BLOCK", fmt.Sprintf("%s has no indented content", node.Kind), "add an indented child or property", node.HeaderSpan)
	}
	if (node.Kind == NodeScenario || isFixtureNodeKind(node.Kind)) && node.ID == "" {
		p.add("PAPER_SCENARIO_NAME", fmt.Sprintf("%s requires a readable @name", node.Kind), "add an @name after the declaration kind", node.HeaderSpan)
	}
	if (node.Kind == NodeTheme || node.Kind == NodeToken || node.Kind == NodeScope) && node.ID == "" {
		p.add("PAPER_THEME_NAME", fmt.Sprintf("%s requires a readable @name", node.Kind), "add an @name after the declaration kind", node.HeaderSpan)
	}
	if node.Kind == NodeStyle && node.ID == "" {
		p.add("PAPER_STYLE_NAME", "style requires a readable @name", "write style @body:", node.HeaderSpan)
	}
	if node.Kind == NodeRepeat && node.ID == "" {
		p.add("PAPER_REPEAT_NAME", "repeat requires a readable @name", "write repeat @name:", node.HeaderSpan)
	}
	if node.Kind == NodeLoop && node.ID == "" {
		p.add("PAPER_LOOP_NAME", "loop requires a readable @name", "write loop @name:", node.HeaderSpan)
	}
	if (node.Kind == NodeProp || node.Kind == NodeArg) && node.ID == "" {
		p.add("PAPER_COMPONENT_CONTRACT_NAME", fmt.Sprintf("%s requires a readable @name", node.Kind), "add an @name after the declaration kind", node.HeaderSpan)
	}
	for _, member := range node.Members {
		if member.Node == nil || allowedChild(node.Kind, member.Node.Kind) {
			continue
		}
		p.add("PAPER_INVALID_CHILD", fmt.Sprintf("%s cannot contain %s", node.Kind, member.Node.Kind), hierarchyHint(node.Kind), member.Node.HeaderSpan)
	}
	if depth > 0 && node.Kind == NodeDocument {
		p.add("PAPER_NESTED_DOCUMENT", "document may only appear at the root", "remove this wrapper or move it to another file", node.HeaderSpan)
	}
}

func allowedChild(parent, child NodeKind) bool {
	switch parent {
	case NodeDocument:
		return child == NodePage || child == NodeComponent || child == NodeSchema || child == NodeScenario || child == NodeTheme || child == NodeStyle
	case NodePage:
		return child == NodeBody || child == NodeHeader || child == NodeFooter
	case NodeHeader, NodeFooter:
		return isComponentBodyChild(child)
	case NodeBody:
		return child == NodeHeading || child == NodeParagraph || child == NodeList || child == NodePageBreak || child == NodeText || child == NodeRow || child == NodeColumn || child == NodeImage || child == NodeTable || child == NodeCanvas || child == NodeUse || child == NodeRepeat || child == NodeLoop
	case NodeCanvas:
		return child == NodeAnchor
	case NodeTable:
		return child == NodeTableTrack || child == NodeTableHeader || child == NodeTableRow
	case NodeTableHeader:
		return child == NodeTableRow
	case NodeTableRow:
		return child == NodeTableCell
	case NodeTableCell:
		return child == NodeText || child == NodeParagraph || child == NodeImage || child == NodeList
	case NodeRow, NodeColumn:
		return child == NodeHeading || child == NodeParagraph || child == NodeUse
	case NodeList:
		return child == NodeItem
	case NodeItem:
		return child == NodeParagraph || child == NodeText || child == NodeUse
	case NodeHeading, NodeParagraph:
		return child == NodeText
	case NodeComponent:
		return isComponentBodyChild(child) || child == NodeSlot || child == NodeProp
	case NodeProp:
		return false
	case NodeSlot, NodeFill:
		return isComponentBodyChild(child)
	case NodeUse:
		return child == NodeFill || child == NodeArg
	case NodeRepeat:
		return isComponentBodyChild(child) || child == NodeRepeat || child == NodeLoop
	case NodeLoop:
		return isComponentBodyChild(child) || child == NodeLoop
	case NodeSchema, NodeField:
		return child == NodeField
	case NodeScenario, NodeObject, NodeKeyedList:
		return isFixtureNodeKind(child)
	case NodeTheme, NodeScope:
		return child == NodeToken || child == NodeScope
	case NodeStyle:
		return false
	case NodeText, NodeArg, NodeImage, NodeTableTrack, NodeAnchor:
		return false
	default:
		return false
	}
}

func hierarchyHint(parent NodeKind) string {
	switch parent {
	case NodeDocument:
		return "place schema/component definitions and page nodes inside document"
	case NodePage:
		return "place one body and optional header/footer regions inside page"
	case NodeHeader, NodeFooter:
		return "header and footer regions accept ordinary body content"
	case NodeBody:
		return "body accepts canvas, row, column, table, image, heading, paragraph, list, page-break, and text"
	case NodeCanvas:
		return "canvas accepts explicit anchor children"
	case NodeTable:
		return "table accepts table-track, table-header, and table-row children"
	case NodeTableHeader:
		return "table-header accepts table-row children"
	case NodeTableRow:
		return "table-row accepts cell children"
	case NodeTableCell:
		return "cell accepts text, paragraph, list, and image children"
	case NodeRow, NodeColumn:
		return "row and column accept heading and paragraph children"
	case NodeList:
		return "list accepts item children"
	case NodeItem:
		return "item accepts paragraph, text, and use children"
	case NodeComponent:
		return "component accepts prop declarations, body nodes, nested uses, and slot placeholders"
	case NodeProp:
		return "prop accepts type, required, and default properties"
	case NodeSlot, NodeFill:
		return "slot defaults and fills accept body nodes and nested uses"
	case NodeUse:
		return "use accepts named arg and fill children"
	case NodeRepeat:
		return "repeat accepts exactly one existing block, component use, or nested repeat template"
	case NodeLoop:
		return "loop accepts exactly one existing block, component use, or nested loop template"
	case NodeSchema, NodeField:
		return "schema and object/list fields accept nested field declarations"
	case NodeScenario, NodeObject, NodeKeyedList:
		return "scenario data accepts value, object, and keyed-list declarations"
	case NodeTheme, NodeScope:
		return "themes and lexical scopes accept token and nested scope declarations"
	case NodeStyle:
		return "styles accept named design properties"
	case NodeHeading, NodeParagraph:
		return "heading and paragraph accept text children"
	default:
		return "text is a leaf node"
	}
}

func isComponentBodyChild(child NodeKind) bool {
	return child == NodeHeading || child == NodeParagraph || child == NodeList || child == NodePageBreak ||
		child == NodeText || child == NodeRow || child == NodeColumn || child == NodeImage || child == NodeTable || child == NodeUse || child == NodeLoop
}

func memberSpan(member Member) Span {
	if member.Node != nil {
		return member.Node.Span
	}
	if member.Property != nil {
		return member.Property.Span
	}
	return Span{}
}

func splitUnit(value string) (string, string) {
	index := 0
	if index < len(value) && (value[index] == '+' || value[index] == '-') {
		index++
	}
	for index < len(value) && (value[index] >= '0' && value[index] <= '9' || value[index] == '.') {
		index++
	}
	return value[:index], value[index:]
}

func validUnit(unit string) bool {
	switch unit {
	case "pt", "mm", "cm", "in", "px", "pc", "em", "rem", "vh", "vw", "%":
		return true
	default:
		return false
	}
}

func isScalarToken(kind TokenKind) bool {
	return kind == TokenString || kind == TokenBool || kind == TokenNumber || kind == TokenUnit || kind == TokenNull
}

func isFixtureNodeKind(kind NodeKind) bool {
	return kind == NodeValue || kind == NodeObject || kind == NodeKeyedList
}

func isContextualNodeKind(kind NodeKind) bool {
	return isFixtureNodeKind(kind) || kind == NodeTheme || kind == NodeStyle || kind == NodeToken || kind == NodeScope
}

func scopedReadableID(kind NodeKind) bool {
	return kind == NodeProp || kind == NodeArg || kind == NodeSlot || kind == NodeFill || kind == NodeField || kind == NodeToken || kind == NodeScope || isFixtureNodeKind(kind)
}

func (p *paperParser) current() Token {
	if p.index >= len(p.tokens) {
		return Token{Kind: TokenEOF, Span: Span{File: p.file}}
	}
	return p.tokens[p.index]
}

func (p *paperParser) at(kind TokenKind) bool { return p.current().Kind == kind }

func (p *paperParser) advance() Token {
	token := p.current()
	if p.index < len(p.tokens) {
		p.index++
	}
	return token
}

func (p *paperParser) consume(kind TokenKind) bool {
	if !p.at(kind) {
		return false
	}
	p.advance()
	return true
}

func (p *paperParser) skipLine() {
	for !p.at(TokenNewline) && !p.at(TokenEOF) {
		p.advance()
	}
	p.consume(TokenNewline)
}

func (p *paperParser) skipNewlines() {
	for p.at(TokenNewline) {
		p.advance()
	}
}

func (p *paperParser) skipIndentedBlock() {
	if !p.consume(TokenIndent) {
		return
	}
	depth := 1
	for depth > 0 && !p.at(TokenEOF) {
		switch p.advance().Kind {
		case TokenIndent:
			depth++
		case TokenDedent:
			depth--
		}
	}
}

func (p *paperParser) add(code, message, hint string, span Span) {
	p.diagnostics = append(p.diagnostics, errorDiagnostic(code, message, hint, span))
}
