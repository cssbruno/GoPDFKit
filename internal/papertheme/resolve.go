// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package papertheme

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

type indexedTheme struct {
	input  *Theme
	scopes map[string]*indexedScope
}

type indexedScope struct {
	path   []string
	tokens map[string]*Token
}

type tokenIdentity struct {
	theme string
	scope string
	name  string
}

type resolver struct {
	input       Input
	limits      Limits
	result      Result
	themes      map[string]*indexedTheme
	themeOrder  []string
	work        uint64
	tokens      uint32
	bytes       uint64
	reported    map[string]bool
	parentState map[string]uint8
}

// Resolve validates and resolves all authored token definitions and requested
// properties. Results never alias caller-owned slices.
func Resolve(input Input, limits Limits) Result {
	r := resolver{
		input: input, limits: limits, themes: make(map[string]*indexedTheme),
		reported: make(map[string]bool), parentState: make(map[string]uint8),
	}
	if limits == (Limits{}) {
		r.limits = DefaultLimits()
	} else if !validLimits(limits) {
		r.add("PAPER_THEME_LIMITS", "theme limits are incomplete or exceed hard caps", "use positive limits no greater than DefaultLimits", Source{})
		r.limits = DefaultLimits()
	}
	if !r.preflightBytes() {
		return r.finish()
	}
	r.indexThemes()
	r.validateParents()
	r.resolveThemes()
	r.resolveProperties()
	return r.finish()
}

func validLimits(l Limits) bool {
	h := DefaultLimits()
	return l.MaxThemes > 0 && l.MaxThemes <= h.MaxThemes && l.MaxTokens > 0 && l.MaxTokens <= h.MaxTokens &&
		l.MaxDepth > 0 && l.MaxDepth <= h.MaxDepth && l.MaxWork > 0 && l.MaxWork <= h.MaxWork &&
		l.MaxSourceBytes > 0 && l.MaxSourceBytes <= h.MaxSourceBytes
}

func (r *resolver) preflightBytes() bool {
	for themeIndex := range r.input.Themes {
		theme := &r.input.Themes[themeIndex]
		if uint32(themeIndex) >= r.limits.MaxThemes {
			r.addOnce("themes", "PAPER_THEME_COUNT_LIMIT", "theme count exceeds the configured limit", "reduce themes or raise the bounded limit", theme.Source)
			return false
		}
		if !r.spend(1, theme.Source) {
			return false
		}
		if !r.addBytes(theme.Name, theme.Parent, theme.Source.File) || !r.scopeBytes(theme.Tokens, theme.Scopes, 1) {
			return false
		}
	}
	for _, property := range r.input.Properties {
		if !r.spend(1, property.Source) {
			return false
		}
		if !r.addBytes(property.Name, property.Theme, property.Token, property.Source.File) {
			return false
		}
		for _, scope := range property.Scope {
			if !r.addBytes(scope) {
				return false
			}
		}
	}
	return true
}

func (r *resolver) scopeBytes(tokens []Token, scopes []Scope, depth uint32) bool {
	if depth > r.limits.MaxDepth {
		r.addOnce("depth", "PAPER_THEME_DEPTH_LIMIT", "theme scope nesting exceeds the configured depth", "flatten nested scopes or raise the bounded limit", Source{})
		return false
	}
	for _, token := range tokens {
		if r.tokens >= r.limits.MaxTokens {
			r.addOnce("tokens", "PAPER_THEME_TOKEN_LIMIT", "token count exceeds the configured limit", "reduce tokens or raise the bounded limit", token.Source)
			return false
		}
		r.tokens++
		if !r.spend(1, token.Source) {
			return false
		}
		if !r.addBytes(token.Name, string(token.Kind), token.Reference, token.Source.File, token.Value.Color, token.Value.String, token.Value.Length.Number, token.Value.Length.Unit, token.Value.Number) {
			return false
		}
	}
	for _, scope := range scopes {
		if !r.spend(1, scope.Source) {
			return false
		}
		if !r.addBytes(scope.Name, scope.Source.File) || !r.scopeBytes(scope.Tokens, scope.Scopes, depth+1) {
			return false
		}
	}
	return true
}

func (r *resolver) addBytes(values ...string) bool {
	for _, value := range values {
		if uint64(len(value)) > r.limits.MaxSourceBytes-r.bytes {
			r.addOnce("bytes", "PAPER_THEME_BYTE_LIMIT", "theme input exceeds the configured byte limit", "reduce token data or raise the bounded limit", Source{})
			return false
		}
		r.bytes += uint64(len(value))
	}
	return true
}

func (r *resolver) indexThemes() {
	for index := range r.input.Themes {
		theme := &r.input.Themes[index]
		if uint32(len(r.themeOrder)) >= r.limits.MaxThemes {
			r.add("PAPER_THEME_COUNT_LIMIT", "theme count exceeds the configured limit", "reduce themes or raise the bounded limit", theme.Source)
			continue
		}
		if !validName(theme.Name) {
			r.add("PAPER_THEME_NAME", fmt.Sprintf("theme name %q is invalid", theme.Name), "use letters, digits, '_' or '-' with a non-digit first character", theme.Source)
			continue
		}
		if _, duplicate := r.themes[theme.Name]; duplicate {
			r.add("PAPER_THEME_DUPLICATE", fmt.Sprintf("theme %q is declared more than once", theme.Name), "keep one declaration per theme", theme.Source)
			continue
		}
		indexed := &indexedTheme{input: theme, scopes: make(map[string]*indexedScope)}
		r.themes[theme.Name] = indexed
		r.themeOrder = append(r.themeOrder, theme.Name)
		r.indexScope(indexed, nil, theme.Tokens, theme.Scopes, theme.Source, 1)
	}
}

func (r *resolver) indexScope(theme *indexedTheme, path []string, tokens []Token, scopes []Scope, source Source, depth uint32) {
	if depth > r.limits.MaxDepth {
		r.add("PAPER_THEME_DEPTH_LIMIT", "theme scope nesting exceeds the configured depth", "flatten nested scopes or raise the bounded limit", source)
		return
	}
	key := scopeKey(path)
	indexed := &indexedScope{path: cloneStrings(path), tokens: make(map[string]*Token)}
	theme.scopes[key] = indexed
	for tokenIndex := range tokens {
		token := &tokens[tokenIndex]
		if !validName(token.Name) {
			r.add("PAPER_THEME_TOKEN_NAME", fmt.Sprintf("token name %q is invalid", token.Name), "use letters, digits, '_' or '-' with a non-digit first character", token.Source)
			continue
		}
		if _, duplicate := indexed.tokens[token.Name]; duplicate {
			r.add("PAPER_THEME_TOKEN_DUPLICATE", fmt.Sprintf("token %q is repeated in one lexical scope", token.Name), "keep one declaration per scope", token.Source)
			continue
		}
		indexed.tokens[token.Name] = token
	}
	seenScopes := make(map[string]bool)
	for scopeIndex := range scopes {
		scope := &scopes[scopeIndex]
		if !validName(scope.Name) {
			r.add("PAPER_THEME_SCOPE_NAME", fmt.Sprintf("scope name %q is invalid", scope.Name), "use letters, digits, '_' or '-'", scope.Source)
			continue
		}
		if seenScopes[scope.Name] {
			r.add("PAPER_THEME_SCOPE_DUPLICATE", fmt.Sprintf("scope %q is repeated under one parent", scope.Name), "keep one child scope with this name", scope.Source)
			continue
		}
		seenScopes[scope.Name] = true
		childPath := append(cloneStrings(path), scope.Name)
		r.indexScope(theme, childPath, scope.Tokens, scope.Scopes, scope.Source, depth+1)
	}
}

func (r *resolver) validateParents() {
	for _, name := range r.themeOrder {
		parent := r.themes[name].input.Parent
		if parent != "" {
			if !validName(parent) {
				r.add("PAPER_THEME_PARENT", fmt.Sprintf("theme %q has invalid parent %q", name, parent), "use the name of another theme", r.themes[name].input.Source)
			} else if _, exists := r.themes[parent]; !exists {
				r.add("PAPER_THEME_PARENT_UNKNOWN", fmt.Sprintf("theme %q has unknown parent %q", name, parent), "declare the parent theme", r.themes[name].input.Source)
			}
		}
	}
	var visit func(string, uint32)
	visit = func(name string, depth uint32) {
		if depth > r.limits.MaxDepth {
			r.addOnce("parent-depth", "PAPER_THEME_DEPTH_LIMIT", "theme inheritance exceeds the configured depth", "shorten the parent chain", r.themes[name].input.Source)
			return
		}
		r.parentState[name] = 1
		parent := r.themes[name].input.Parent
		if _, exists := r.themes[parent]; exists && parent != "" {
			switch r.parentState[parent] {
			case 0:
				visit(parent, depth+1)
			case 1:
				r.addOnce("parent-cycle:"+name, "PAPER_THEME_PARENT_CYCLE", fmt.Sprintf("theme inheritance cycle reaches %q", parent), "remove one parent edge from the cycle", r.themes[name].input.Source)
			}
		}
		r.parentState[name] = 2
	}
	for _, name := range r.themeOrder {
		if r.parentState[name] == 0 {
			visit(name, 1)
		}
	}
}

func (r *resolver) resolveThemes() {
	resolved := make([]ResolvedTheme, 0, len(r.themeOrder))
	for _, themeName := range r.themeOrder {
		theme := r.themes[themeName]
		output := ResolvedTheme{Name: themeName, Parent: theme.input.Parent}
		keys := make([]string, 0, len(theme.scopes))
		for key := range theme.scopes {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			scope := theme.scopes[key]
			names := make([]string, 0, len(scope.tokens))
			for name := range scope.tokens {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				value, chain, ok := r.resolveAt(themeName, scope.path, name, scope.tokens[name].Source)
				if ok {
					output.Tokens = append(output.Tokens, ResolvedToken{Name: name, Scope: cloneStrings(scope.path), Value: value, Provenance: Provenance{Chain: cloneSteps(chain)}})
				}
			}
		}
		resolved = append(resolved, output)
	}
	sort.Slice(resolved, func(i, j int) bool { return resolved[i].Name < resolved[j].Name })
	r.result.Output.Themes = resolved
}

func (r *resolver) resolveProperties() {
	seen := make(map[string]bool)
	properties := make([]ComputedProperty, 0, len(r.input.Properties))
	for _, property := range r.input.Properties {
		if !validName(property.Name) {
			r.add("PAPER_THEME_PROPERTY_NAME", fmt.Sprintf("property name %q is invalid", property.Name), "use letters, digits, '_' or '-'", property.Source)
			continue
		}
		if seen[property.Name] {
			r.add("PAPER_THEME_PROPERTY_DUPLICATE", fmt.Sprintf("computed property %q is requested more than once", property.Name), "use one request per property name", property.Source)
			continue
		}
		seen[property.Name] = true
		theme := r.themes[property.Theme]
		if theme == nil {
			r.add("PAPER_THEME_UNKNOWN", fmt.Sprintf("property %q uses unknown theme %q", property.Name, property.Theme), "select a declared theme", property.Source)
			continue
		}
		if theme.scopes[scopeKey(property.Scope)] == nil {
			r.add("PAPER_THEME_SCOPE_UNKNOWN", fmt.Sprintf("property %q uses an unknown lexical scope", property.Name), "select an existing scope path", property.Source)
			continue
		}
		if !validKind(property.Kind) {
			r.add("PAPER_THEME_PROPERTY_TYPE", fmt.Sprintf("property %q has unsupported type %q", property.Name, property.Kind), "use color, string, length, number, or bool", property.Source)
			continue
		}
		value, chain, ok := r.resolveAt(property.Theme, property.Scope, property.Token, property.Source)
		if !ok {
			continue
		}
		if value.Kind != property.Kind {
			r.add("PAPER_THEME_PROPERTY_TYPE", fmt.Sprintf("property %q requires %s but token %q resolves to %s", property.Name, property.Kind, property.Token, value.Kind), "select a token with the required type", property.Source)
			continue
		}
		properties = append(properties, ComputedProperty{
			Name: property.Name, Theme: property.Theme, Scope: cloneStrings(property.Scope), Value: value,
			Provenance: Provenance{Property: property.Source, Chain: cloneSteps(chain)},
		})
	}
	sort.Slice(properties, func(i, j int) bool {
		if properties[i].Name != properties[j].Name {
			return properties[i].Name < properties[j].Name
		}
		return properties[i].Theme < properties[j].Theme
	})
	r.result.Output.Properties = properties
}

func (r *resolver) resolveAt(theme string, scope []string, name string, source Source) (Value, []TokenStep, bool) {
	definitionTheme, definitionScope, token := r.lookup(theme, scope, name)
	if token == nil {
		r.addOnce("unknown:"+theme+":"+scopeKey(scope)+":"+name, "PAPER_THEME_TOKEN_UNKNOWN", fmt.Sprintf("token %q is not visible from theme %q", name, theme), "declare the token in this scope, an ancestor scope, or a parent theme", source)
		return Value{}, nil, false
	}
	return r.resolveToken(definitionTheme, definitionScope, token, nil, make(map[tokenIdentity]bool), 1)
}

func (r *resolver) resolveToken(theme string, scope []string, token *Token, chain []TokenStep, active map[tokenIdentity]bool, depth uint32) (Value, []TokenStep, bool) {
	if depth > r.limits.MaxDepth {
		r.addOnce("token-depth:"+theme+":"+scopeKey(scope)+":"+token.Name, "PAPER_THEME_DEPTH_LIMIT", "token alias chain exceeds the configured depth", "shorten the alias chain", token.Source)
		return Value{}, nil, false
	}
	if !r.spend(1, token.Source) {
		return Value{}, nil, false
	}
	identity := tokenIdentity{theme: theme, scope: scopeKey(scope), name: token.Name}
	if active[identity] {
		r.addOnce("token-cycle:"+identity.theme+":"+identity.scope+":"+identity.name, "PAPER_THEME_TOKEN_CYCLE", fmt.Sprintf("token alias cycle reaches %q", token.Name), "replace one alias with a literal or another token", token.Source)
		return Value{}, nil, false
	}
	active[identity] = true
	defer delete(active, identity)
	chain = append(chain, TokenStep{Theme: theme, Scope: cloneStrings(scope), Token: token.Name, Source: token.Source})
	if !validKind(token.Kind) {
		r.addOnce("kind:"+identity.theme+":"+identity.scope+":"+identity.name, "PAPER_THEME_TOKEN_TYPE", fmt.Sprintf("token %q has unsupported type %q", token.Name, token.Kind), "use color, string, length, number, or bool", token.Source)
		return Value{}, nil, false
	}
	if token.Reference == "" {
		value, ok := canonicalValue(token.Value)
		if !ok || value.Kind != token.Kind {
			r.addOnce("literal:"+identity.theme+":"+identity.scope+":"+identity.name, "PAPER_THEME_TOKEN_TYPE", fmt.Sprintf("token %q literal does not match declared %s type", token.Name, token.Kind), "provide one valid literal matching the declared kind", token.Source)
			return Value{}, nil, false
		}
		return value, chain, true
	}
	if token.Value != (Value{}) {
		r.addOnce("union:"+identity.theme+":"+identity.scope+":"+identity.name, "PAPER_THEME_TOKEN_VALUE", fmt.Sprintf("token %q cannot contain both a literal and a reference", token.Name), "keep either Value or Reference", token.Source)
		return Value{}, nil, false
	}
	if !validName(token.Reference) {
		r.addOnce("reference:"+identity.theme+":"+identity.scope+":"+identity.name, "PAPER_THEME_TOKEN_UNKNOWN", fmt.Sprintf("token %q has invalid reference %q", token.Name, token.Reference), "reference a token by name", token.Source)
		return Value{}, nil, false
	}
	nextTheme, nextScope, next := r.lookup(theme, scope, token.Reference)
	if next == nil {
		r.addOnce("reference-unknown:"+identity.theme+":"+identity.scope+":"+identity.name, "PAPER_THEME_TOKEN_UNKNOWN", fmt.Sprintf("token %q references unknown token %q", token.Name, token.Reference), "declare the referenced token in a visible scope", token.Source)
		return Value{}, nil, false
	}
	value, resolvedChain, ok := r.resolveToken(nextTheme, nextScope, next, chain, active, depth+1)
	if ok && value.Kind != token.Kind {
		r.addOnce("alias-type:"+identity.theme+":"+identity.scope+":"+identity.name, "PAPER_THEME_TOKEN_TYPE", fmt.Sprintf("token %q declares %s but reference %q resolves to %s", token.Name, token.Kind, token.Reference, value.Kind), "make the alias and referenced token types match", token.Source)
		return Value{}, nil, false
	}
	return value, resolvedChain, ok
}

func (r *resolver) lookup(themeName string, path []string, name string) (string, []string, *Token) {
	visited := make(map[string]bool)
	for themeName != "" && !visited[themeName] {
		visited[themeName] = true
		theme := r.themes[themeName]
		if theme == nil {
			return "", nil, nil
		}
		for depth := len(path); depth >= 0; depth-- {
			if !r.spend(1, theme.input.Source) {
				return "", nil, nil
			}
			candidatePath := path[:depth]
			if scope := theme.scopes[scopeKey(candidatePath)]; scope != nil {
				if token := scope.tokens[name]; token != nil {
					return themeName, cloneStrings(candidatePath), token
				}
			}
		}
		themeName = theme.input.Parent
	}
	return "", nil, nil
}

func canonicalValue(value Value) (Value, bool) {
	switch value.Kind {
	case Color:
		if value.String != "" || value.Length != (LengthValue{}) || value.Number != "" || value.Bool || !canonicalColor(value.Color) {
			return Value{}, false
		}
		value.Color = strings.ToLower(value.Color)
	case String:
		if value.Color != "" || value.Length != (LengthValue{}) || value.Number != "" || value.Bool || !utf8.ValidString(value.String) {
			return Value{}, false
		}
	case Length:
		if value.Color != "" || value.String != "" || value.Number != "" || value.Bool || !canonicalNumber(value.Length.Number) || !validUnit(value.Length.Unit) {
			return Value{}, false
		}
	case Number:
		if value.Color != "" || value.String != "" || value.Length != (LengthValue{}) || value.Bool || !canonicalNumber(value.Number) {
			return Value{}, false
		}
	case Bool:
		if value.Color != "" || value.String != "" || value.Length != (LengthValue{}) || value.Number != "" {
			return Value{}, false
		}
	default:
		return Value{}, false
	}
	return value, true
}

func validKind(kind Kind) bool {
	return kind == Color || kind == String || kind == Length || kind == Number || kind == Bool
}

func canonicalColor(value string) bool {
	if len(value) != 7 && len(value) != 9 || len(value) == 0 || value[0] != '#' {
		return false
	}
	for _, character := range value[1:] {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') && (character < 'A' || character > 'F') {
			return false
		}
	}
	return true
}

func canonicalNumber(value string) bool {
	if value == "0" {
		return true
	}
	if value == "" || value[0] == '+' || strings.HasSuffix(value, ".") {
		return false
	}
	digits := value
	negative := false
	if digits[0] == '-' {
		negative = true
		digits = digits[1:]
	}
	parts := strings.Split(digits, ".")
	if len(parts) > 2 || parts[0] == "" || len(parts[0]) > 1 && parts[0][0] == '0' {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		for _, character := range part {
			if character < '0' || character > '9' {
				return false
			}
		}
	}
	if len(parts) == 2 && parts[1][len(parts[1])-1] == '0' {
		return false
	}
	return !negative || parts[0] != "0" || len(parts) != 1
}

func validUnit(unit string) bool {
	switch unit {
	case "pt", "mm", "cm", "in", "px", "pc", "em", "rem", "%":
		return true
	default:
		return false
	}
}

func validName(value string) bool {
	if value == "" || len(value) > 256 || !utf8.ValidString(value) {
		return false
	}
	for index, character := range value {
		if character != '_' && character != '-' && (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') && (index == 0 || character < '0' || character > '9') {
			return false
		}
	}
	return true
}

func scopeKey(path []string) string { return strings.Join(path, "\x00") }

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return append([]string(nil), values...)
}

func cloneSteps(values []TokenStep) []TokenStep {
	if len(values) == 0 {
		return nil
	}
	result := make([]TokenStep, len(values))
	for index, value := range values {
		result[index] = value
		result[index].Scope = cloneStrings(value.Scope)
	}
	return result
}

func (r *resolver) spend(amount uint64, source Source) bool {
	if amount > r.limits.MaxWork-r.work {
		r.addOnce("work", "PAPER_THEME_WORK_LIMIT", "theme resolution exceeds the configured work limit", "reduce aliases/properties or raise the bounded limit", source)
		return false
	}
	r.work += amount
	return true
}

func (r *resolver) add(code, message, hint string, source Source) {
	r.result.Diagnostics = append(r.result.Diagnostics, Diagnostic{Code: code, Severity: Error, Message: message, Hint: hint, Source: source})
}

func (r *resolver) addOnce(key, code, message, hint string, source Source) {
	if r.reported[key] {
		return
	}
	r.reported[key] = true
	r.add(code, message, hint, source)
}

func (r *resolver) finish() Result {
	r.result.Canonical, r.result.Digest = canonicalResult(r.result.Output)
	r.result.Canonical = append([]byte(nil), r.result.Canonical...)
	return r.result
}
