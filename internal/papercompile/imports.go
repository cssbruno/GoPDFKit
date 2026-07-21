// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package papercompile

import (
	"fmt"
	"path"
	"strings"

	"github.com/cssbruno/paperrune/internal/paperlang"
)

// ImportResolver is the explicit source boundary used by .paper imports.
// The compiler never reads files by itself: callers decide how an import is
// loaded and return its stable source filename for diagnostics and provenance.
type ImportResolver func(importerFile, importPath string) (file, source string, err error)

// ImportLimits bounds recursive design-file imports. Zero fields select the
// conservative defaults below.
type ImportLimits struct {
	MaxDepth uint32
	MaxFiles uint32
	MaxBytes uint64
}

func defaultImportLimits() ImportLimits {
	return ImportLimits{MaxDepth: 16, MaxFiles: 32, MaxBytes: 8 << 20}
}

func normalizeImportLimits(limits ImportLimits) ImportLimits {
	defaults := defaultImportLimits()
	if limits.MaxDepth == 0 {
		limits.MaxDepth = defaults.MaxDepth
	}
	if limits.MaxFiles == 0 {
		limits.MaxFiles = defaults.MaxFiles
	}
	if limits.MaxBytes == 0 {
		limits.MaxBytes = defaults.MaxBytes
	}
	return limits
}

type importResolution struct {
	ast         paperlang.AST
	diagnostics []paperlang.Diagnostic
}

// resolveImports expands only the design declarations from imported
// documents. Pages, components, scenarios, and schemas stay owned by the
// importing document, which keeps imports composable and prevents a second
// document from silently becoming part of the layout.
func resolveImports(ast paperlang.AST, resolver ImportResolver, limits ImportLimits) importResolution {
	limits = normalizeImportLimits(limits)
	result := importResolution{ast: paperlang.AST{File: ast.File, Root: cloneImportNode(ast.Root)}}
	if result.ast.Root == nil || result.ast.Root.Kind != paperlang.NodeDocument {
		return result
	}
	imports := importProperties(result.ast.Root)
	if len(imports) == 0 {
		return result
	}
	if resolver == nil {
		for _, property := range imports {
			result.diagnostics = append(result.diagnostics, importDiagnostic(
				"PAPER_IMPORT_RESOLVER",
				"document declares an import but no import resolver was provided",
				"compile with an explicit source-relative import resolver",
				property.Span,
			))
		}
		return result
	}

	walker := importWalker{
		resolver: resolver,
		limits:   limits,
		seen:     make(map[string]bool),
		stack:    make(map[string]bool),
	}
	mainKey := ast.File
	if mainKey == "" {
		mainKey = "<document>"
	}
	walker.stack[mainKey] = true
	walker.visit(mainKey, result.ast.Root, 0)
	walker.stack[mainKey] = false

	if len(walker.members) != 0 {
		members := make([]paperlang.Member, 0, len(walker.members)+len(result.ast.Root.Members))
		members = append(members, walker.members...)
		members = append(members, result.ast.Root.Members...)
		result.ast.Root.Members = members
	}
	result.diagnostics = walker.diagnostics
	return result
}

type importWalker struct {
	resolver    ImportResolver
	limits      ImportLimits
	seen        map[string]bool
	stack       map[string]bool
	files       uint32
	bytes       uint64
	members     []paperlang.Member
	diagnostics []paperlang.Diagnostic
}

func (w *importWalker) visit(importer string, root *paperlang.Node, depth uint32) {
	for _, property := range importProperties(root) {
		if depth >= w.limits.MaxDepth {
			w.add("PAPER_IMPORT_DEPTH", "import nesting exceeds the configured limit", "flatten the design imports or raise the bounded depth limit", property.Span)
			continue
		}
		if property.Value.Kind != paperlang.ScalarString || property.Value.StringValue == nil {
			w.add("PAPER_IMPORT_PATH", "import must be a quoted relative path", "use import: \"styles.paper\"", property.Value.Span)
			continue
		}
		importPath := strings.TrimSpace(*property.Value.StringValue)
		if !safeImportPath(importPath) {
			w.add("PAPER_IMPORT_PATH", fmt.Sprintf("import path %q is not a safe relative path", importPath), "use a relative .paper path without an absolute prefix or URL", property.Value.Span)
			continue
		}
		if w.files >= w.limits.MaxFiles {
			w.add("PAPER_IMPORT_COUNT_LIMIT", "import file count exceeds the configured limit", "reduce imported files or raise the bounded file limit", property.Span)
			continue
		}
		file, source, err := w.resolver(importer, importPath)
		if err != nil {
			w.add("PAPER_IMPORT_RESOLVE", fmt.Sprintf("cannot resolve import %q: %v", importPath, err), "make the source-relative design file available", property.Value.Span)
			continue
		}
		if file == "" {
			file = importPath
		}
		if w.stack[file] {
			w.add("PAPER_IMPORT_CYCLE", fmt.Sprintf("import cycle reaches %q", file), "remove the cycle from the design-file imports", property.Value.Span)
			continue
		}
		if w.seen[file] {
			continue
		}
		if uint64(len(source)) > w.limits.MaxBytes-w.bytes {
			w.add("PAPER_IMPORT_BYTES_LIMIT", "imported source bytes exceed the configured limit", "reduce imported source size or raise the bounded byte limit", property.Value.Span)
			continue
		}
		w.files++
		w.bytes += uint64(len(source))
		w.seen[file] = true
		parsed := paperlang.Parse(file, source)
		w.diagnostics = append(w.diagnostics, parsed.Diagnostics...)
		if parsed.AST.Root == nil || parsed.AST.Root.Kind != paperlang.NodeDocument {
			continue
		}
		w.stack[file] = true
		w.visit(file, parsed.AST.Root, depth+1)
		w.stack[file] = false
		for _, member := range parsed.AST.Root.Members {
			if member.Node == nil {
				continue
			}
			switch member.Node.Kind {
			case paperlang.NodeTheme, paperlang.NodeStyle:
				w.members = append(w.members, paperlang.Member{Node: cloneImportNode(member.Node)})
			case paperlang.NodePage, paperlang.NodeComponent, paperlang.NodeSchema, paperlang.NodeObjectType, paperlang.NodeScenario:
				w.add("PAPER_IMPORT_MEMBER", fmt.Sprintf("imported file cannot provide %s", member.Node.Kind), "keep pages and document behavior in the importing file; export themes and styles only", member.Node.HeaderSpan)
			}
		}
	}
}

func importProperties(root *paperlang.Node) []*paperlang.Property {
	if root == nil {
		return nil
	}
	properties := make([]*paperlang.Property, 0, 1)
	for _, member := range root.Members {
		if member.Property != nil && member.Property.Name == "import" {
			properties = append(properties, member.Property)
		}
	}
	return properties
}

func safeImportPath(value string) bool {
	if value == "" || strings.ContainsRune(value, '\x00') || strings.Contains(value, "://") || strings.HasPrefix(value, "~") {
		return false
	}
	if strings.HasPrefix(value, "/") || strings.HasPrefix(value, "\\") || (len(value) > 1 && value[1] == ':') {
		return false
	}
	clean := path.Clean(strings.ReplaceAll(value, "\\", "/"))
	if clean == "." {
		return false
	}
	return true
}

func importDiagnostic(code, message, hint string, span paperlang.Span) paperlang.Diagnostic {
	return paperlang.Diagnostic{Code: code, Severity: paperlang.SeverityError, Message: message, Hint: hint, Span: span}
}

func (w *importWalker) add(code, message, hint string, span paperlang.Span) {
	w.diagnostics = append(w.diagnostics, importDiagnostic(code, message, hint, span))
}

func cloneImportNode(node *paperlang.Node) *paperlang.Node {
	if node == nil {
		return nil
	}
	clone := *node
	clone.Value = cloneImportScalar(node.Value)
	if len(node.Members) == 0 {
		clone.Members = nil
		return &clone
	}
	clone.Members = make([]paperlang.Member, len(node.Members))
	for index, member := range node.Members {
		clone.Members[index].Node = cloneImportNode(member.Node)
		if member.Property != nil {
			property := *member.Property
			property.Value = *cloneImportScalar(&member.Property.Value)
			clone.Members[index].Property = &property
		}
	}
	return &clone
}

func cloneImportScalar(value *paperlang.Scalar) *paperlang.Scalar {
	if value == nil {
		return nil
	}
	clone := *value
	if value.StringValue != nil {
		copyValue := *value.StringValue
		clone.StringValue = &copyValue
	}
	if value.BoolValue != nil {
		copyValue := *value.BoolValue
		clone.BoolValue = &copyValue
	}
	if value.NumberValue != nil {
		copyValue := *value.NumberValue
		clone.NumberValue = &copyValue
	}
	if value.UnitValue != nil {
		copyValue := *value.UnitValue
		clone.UnitValue = &copyValue
	}
	return &clone
}
