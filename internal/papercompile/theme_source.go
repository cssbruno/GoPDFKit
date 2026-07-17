// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package papercompile

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/cssbruno/gopdfkit/internal/paperlang"
	"github.com/cssbruno/gopdfkit/internal/papertheme"
)

// ThemeSourceResult is the source-located .paper-to-theme projection. Input is
// suitable for later property requests; Output and Digest contain the resolved
// declaration catalog. Styles are intentionally not applied to layout here.
type ThemeSourceResult struct {
	Input       papertheme.Input
	Output      papertheme.Output
	Canonical   []byte
	Digest      string
	Diagnostics []paperlang.Diagnostic
}

func (r ThemeSourceResult) OK() bool {
	for _, diagnostic := range r.Diagnostics {
		if diagnostic.Severity == paperlang.SeverityError {
			return false
		}
	}
	return true
}

func CompileThemeSource(file, source string) ThemeSourceResult {
	return CompileThemeSourceWithLimits(file, source, papertheme.Limits{})
}

func CompileThemeSourceWithLimits(file, source string, limits papertheme.Limits) ThemeSourceResult {
	parsed := paperlang.Parse(file, source)
	extracted := ExtractThemesWithLimits(parsed.AST, limits)
	extracted.Diagnostics = append(append([]paperlang.Diagnostic(nil), parsed.Diagnostics...), extracted.Diagnostics...)
	return extracted
}

func ExtractThemes(ast paperlang.AST) ThemeSourceResult {
	return ExtractThemesWithLimits(ast, papertheme.Limits{})
}

func ExtractThemesWithLimits(ast paperlang.AST, limits papertheme.Limits) ThemeSourceResult {
	effective := themeSourceLimits(limits)
	a := themeSourceAnalysis{limits: effective, spans: make(map[themeSourceKey]paperlang.Span), root: rootSpan(ast)}
	if ast.Root == nil || ast.Root.Kind != paperlang.NodeDocument {
		a.add("PAPER_THEME_DOCUMENT", "theme extraction requires one document root", "parse a valid document before extracting themes", a.root)
		return a.result
	}
	for _, member := range ast.Root.Members {
		if member.Node == nil || member.Node.Kind != paperlang.NodeTheme {
			continue
		}
		if !a.spend(member.Node.HeaderSpan) {
			break
		}
		if uint32(len(a.result.Input.Themes)) >= a.limits.MaxThemes {
			a.addOnce("themes", "PAPER_THEME_COUNT_LIMIT", "theme count exceeds the configured limit", "reduce themes or raise the bounded limit", member.Node.HeaderSpan)
			break
		}
		a.result.Input.Themes = append(a.result.Input.Themes, a.theme(member.Node))
	}

	resolved := papertheme.Resolve(a.result.Input, limits)
	a.result.Output = resolved.Output
	a.result.Canonical = append([]byte(nil), resolved.Canonical...)
	a.result.Digest = resolved.Digest
	for _, diagnostic := range resolved.Diagnostics {
		a.result.Diagnostics = append(a.result.Diagnostics, paperlang.Diagnostic{
			Code: diagnostic.Code, Severity: paperlang.SeverityError,
			Message: diagnostic.Message, Hint: diagnostic.Hint, Span: a.spanFor(diagnostic.Source),
		})
	}
	return a.result
}

type themeSourceAnalysis struct {
	result   ThemeSourceResult
	limits   papertheme.Limits
	spans    map[themeSourceKey]paperlang.Span
	root     paperlang.Span
	tokens   uint32
	work     uint64
	reported map[string]bool
}

type themeSourceKey struct {
	file       string
	start, end uint64
}

func (a *themeSourceAnalysis) theme(node *paperlang.Node) papertheme.Theme {
	theme := papertheme.Theme{Name: sourceDeclarationName(node.ID), Source: a.source(node.Span)}
	properties, children := a.declarationMembers(node, map[string]bool{"parent": true})
	if property := properties["parent"]; property != nil {
		if property.Value.Kind != paperlang.ScalarString || property.Value.StringValue == nil {
			a.add("PAPER_THEME_PARENT", "theme parent must be a quoted theme name", "use parent: \"base\"", property.Value.Span)
		} else {
			theme.Parent = strings.TrimPrefix(*property.Value.StringValue, "@")
		}
	}
	for _, child := range children {
		switch child.Kind {
		case paperlang.NodeToken:
			if a.reserveToken(child.HeaderSpan) {
				theme.Tokens = append(theme.Tokens, a.token(child))
			}
		case paperlang.NodeScope:
			theme.Scopes = append(theme.Scopes, a.scope(child, 2))
		default:
			a.add("PAPER_THEME_CHILD", fmt.Sprintf("theme cannot contain %s", child.Kind), "use token or scope declarations", child.HeaderSpan)
		}
	}
	return theme
}

func (a *themeSourceAnalysis) scope(node *paperlang.Node, depth uint32) papertheme.Scope {
	scope := papertheme.Scope{Name: sourceDeclarationName(node.ID), Source: a.source(node.Span)}
	if depth > a.limits.MaxDepth {
		a.addOnce("depth", "PAPER_THEME_DEPTH_LIMIT", "theme scope nesting exceeds the configured depth", "flatten nested scopes or raise the bounded limit", node.HeaderSpan)
		return scope
	}
	_, children := a.declarationMembers(node, map[string]bool{})
	for _, child := range children {
		switch child.Kind {
		case paperlang.NodeToken:
			if a.reserveToken(child.HeaderSpan) {
				scope.Tokens = append(scope.Tokens, a.token(child))
			}
		case paperlang.NodeScope:
			scope.Scopes = append(scope.Scopes, a.scope(child, depth+1))
		default:
			a.add("PAPER_THEME_CHILD", fmt.Sprintf("scope cannot contain %s", child.Kind), "use token or nested scope declarations", child.HeaderSpan)
		}
	}
	return scope
}

func (a *themeSourceAnalysis) token(node *paperlang.Node) papertheme.Token {
	token := papertheme.Token{Name: sourceDeclarationName(node.ID), Source: a.source(node.Span)}
	properties, children := a.declarationMembers(node, map[string]bool{"type": true, "value": true, "reference": true})
	for _, child := range children {
		a.add("PAPER_THEME_TOKEN_CHILD", fmt.Sprintf("token cannot contain %s", child.Kind), "use type and value/reference properties only", child.HeaderSpan)
	}
	if property := properties["type"]; property == nil || property.Value.Kind != paperlang.ScalarString || property.Value.StringValue == nil {
		span := node.HeaderSpan
		if property != nil {
			span = property.Value.Span
		}
		a.add("PAPER_THEME_TOKEN_TYPE", "token requires a quoted type", "use color, string, length, number, or bool", span)
	} else {
		token.Kind = papertheme.Kind(strings.ToLower(strings.TrimSpace(*property.Value.StringValue)))
	}
	if property := properties["reference"]; property != nil {
		if property.Value.Kind != paperlang.ScalarString || property.Value.StringValue == nil {
			a.add("PAPER_THEME_TOKEN_UNKNOWN", "token reference must be a quoted token name", "use reference: \"other-token\"", property.Value.Span)
		} else {
			token.Reference = strings.TrimPrefix(*property.Value.StringValue, "@")
		}
	}
	if property := properties["value"]; property != nil {
		if value, ok := a.themeValue(property.Value, token.Kind); ok {
			token.Value = value
		}
	}
	return token
}

func (a *themeSourceAnalysis) themeValue(scalar paperlang.Scalar, declared papertheme.Kind) (papertheme.Value, bool) {
	switch scalar.Kind {
	case paperlang.ScalarString:
		if scalar.StringValue == nil {
			return papertheme.Value{}, false
		}
		if declared == papertheme.Color {
			return papertheme.Value{Kind: papertheme.Color, Color: *scalar.StringValue}, true
		}
		return papertheme.Value{Kind: papertheme.String, String: *scalar.StringValue}, true
	case paperlang.ScalarBool:
		if scalar.BoolValue != nil {
			return papertheme.Value{Kind: papertheme.Bool, Bool: *scalar.BoolValue}, true
		}
	case paperlang.ScalarNumber:
		if scalar.NumberValue != nil && !math.IsNaN(*scalar.NumberValue) && !math.IsInf(*scalar.NumberValue, 0) {
			number := strconv.FormatFloat(*scalar.NumberValue, 'f', -1, 64)
			if *scalar.NumberValue == 0 {
				number = "0"
			}
			return papertheme.Value{Kind: papertheme.Number, Number: number}, true
		}
	case paperlang.ScalarUnit:
		if scalar.UnitValue != nil && !math.IsNaN(scalar.UnitValue.Number) && !math.IsInf(scalar.UnitValue.Number, 0) {
			number := strconv.FormatFloat(scalar.UnitValue.Number, 'f', -1, 64)
			if scalar.UnitValue.Number == 0 {
				number = "0"
			}
			return papertheme.Value{Kind: papertheme.Length, Length: papertheme.LengthValue{Number: number, Unit: scalar.UnitValue.Unit}}, true
		}
	case paperlang.ScalarNull:
		a.add("PAPER_THEME_TOKEN_VALUE", "theme tokens do not support null literals", "use color, string, length, number, or bool", scalar.Span)
		return papertheme.Value{}, false
	}
	a.add("PAPER_THEME_TOKEN_VALUE", "token literal is invalid", "use a supported typed scalar", scalar.Span)
	return papertheme.Value{}, false
}

func (a *themeSourceAnalysis) declarationMembers(node *paperlang.Node, allowed map[string]bool) (map[string]*paperlang.Property, []*paperlang.Node) {
	properties := make(map[string]*paperlang.Property)
	children := make([]*paperlang.Node, 0)
	for _, member := range node.Members {
		if member.Node != nil {
			if !a.spend(member.Node.HeaderSpan) {
				break
			}
			children = append(children, member.Node)
			continue
		}
		if member.Property == nil {
			continue
		}
		if !a.spend(member.Property.Span) {
			break
		}
		property := member.Property
		if !allowed[property.Name] {
			a.add("PAPER_THEME_PROPERTY", fmt.Sprintf("property %q is not supported on %s", property.Name, node.Kind), "use only the documented theme declaration properties", property.Span)
			continue
		}
		if properties[property.Name] != nil {
			a.add("PAPER_THEME_PROPERTY_DUPLICATE", fmt.Sprintf("property %q is repeated", property.Name), "remove the duplicate property", property.Span)
			continue
		}
		properties[property.Name] = property
	}
	return properties, children
}

func (a *themeSourceAnalysis) reserveToken(span paperlang.Span) bool {
	if a.tokens >= a.limits.MaxTokens {
		a.addOnce("tokens", "PAPER_THEME_TOKEN_LIMIT", "token count exceeds the configured limit", "reduce tokens or raise the bounded limit", span)
		return false
	}
	a.tokens++
	return true
}

func (a *themeSourceAnalysis) spend(span paperlang.Span) bool {
	if a.work >= a.limits.MaxWork {
		a.addOnce("work", "PAPER_THEME_WORK_LIMIT", "theme source extraction exceeds the configured work limit", "reduce declarations or raise the bounded limit", span)
		return false
	}
	a.work++
	return true
}

func (a *themeSourceAnalysis) source(span paperlang.Span) papertheme.Source {
	source := papertheme.Source{
		File: span.File, StartOffset: span.Start.Offset, EndOffset: span.End.Offset,
		Line: span.Start.Line, Column: span.Start.Column,
	}
	a.spans[themeSourceKey{file: source.File, start: source.StartOffset, end: source.EndOffset}] = span
	return source
}

func (a *themeSourceAnalysis) spanFor(source papertheme.Source) paperlang.Span {
	if span, ok := a.spans[themeSourceKey{file: source.File, start: source.StartOffset, end: source.EndOffset}]; ok {
		return span
	}
	return a.root
}

func (a *themeSourceAnalysis) add(code, message, hint string, span paperlang.Span) {
	a.result.Diagnostics = append(a.result.Diagnostics, paperlang.Diagnostic{Code: code, Severity: paperlang.SeverityError, Message: message, Hint: hint, Span: span})
}

func (a *themeSourceAnalysis) addOnce(key, code, message, hint string, span paperlang.Span) {
	if a.reported == nil {
		a.reported = make(map[string]bool)
	}
	if a.reported[key] {
		return
	}
	a.reported[key] = true
	a.add(code, message, hint, span)
}

func sourceDeclarationName(id string) string { return strings.TrimPrefix(id, "@") }

func themeSourceLimits(limits papertheme.Limits) papertheme.Limits {
	if limits == (papertheme.Limits{}) {
		return papertheme.DefaultLimits()
	}
	hard := papertheme.DefaultLimits()
	if limits.MaxThemes == 0 || limits.MaxThemes > hard.MaxThemes || limits.MaxTokens == 0 || limits.MaxTokens > hard.MaxTokens ||
		limits.MaxDepth == 0 || limits.MaxDepth > hard.MaxDepth || limits.MaxWork == 0 || limits.MaxWork > hard.MaxWork ||
		limits.MaxSourceBytes == 0 || limits.MaxSourceBytes > hard.MaxSourceBytes {
		return hard
	}
	return limits
}
