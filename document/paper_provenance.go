// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"encoding/json"
	"fmt"

	"github.com/cssbruno/gopdfkit/internal/papercompile"
	"github.com/cssbruno/gopdfkit/internal/paperlang"
	"github.com/cssbruno/gopdfkit/internal/papertheme"
	"github.com/cssbruno/gopdfkit/layout"
)

// PaperPlanSourceSpan identifies the authored source range that caused one
// retained binding or style decision. It is detached from the compiler's
// internal span type so the plan API does not expose parser contracts.
type PaperPlanSourceSpan struct {
	File        string `json:"file,omitempty"`
	StartOffset uint64 `json:"start_offset,omitempty"`
	EndOffset   uint64 `json:"end_offset,omitempty"`
	StartLine   uint32 `json:"start_line,omitempty"`
	StartColumn uint32 `json:"start_column,omitempty"`
	EndLine     uint32 `json:"end_line,omitempty"`
	EndColumn   uint32 `json:"end_column,omitempty"`
}

// PaperPlanBindingProvenance is the exact data path used by a compiled text
// node. The path is source metadata, not a value read from the scenario.
type PaperPlanBindingProvenance struct {
	Node       string              `json:"node,omitempty"`
	Kind       string              `json:"kind"`
	Path       string              `json:"path"`
	Source     PaperPlanSourceSpan `json:"source"`
	Nullable   bool                `json:"nullable,omitempty"`
	Collection bool                `json:"collection,omitempty"`
}

// PaperPlanTokenStep is one authored token or alias declaration in a
// resolved style-token chain.
type PaperPlanTokenStep struct {
	Theme  string              `json:"theme"`
	Scope  []string            `json:"scope,omitempty"`
	Token  string              `json:"token"`
	Source PaperPlanSourceSpan `json:"source"`
}

// PaperPlanStyleTokenProvenance describes one computed property and the
// exact theme/token chain that supplied its value.
type PaperPlanStyleTokenProvenance struct {
	Node       string               `json:"node,omitempty"`
	Kind       string               `json:"kind"`
	Property   string               `json:"property"`
	Theme      string               `json:"theme"`
	Token      string               `json:"token"`
	Value      string               `json:"value"`
	Consumer   PaperPlanSourceSpan  `json:"consumer"`
	TokenChain []PaperPlanTokenStep `json:"token_chain"`
}

// PaperPlanComputedStyleProvenance is the resolved, renderer-independent
// style attached to one readable source block. It is exact compiler output,
// not browser-computed CSS.
type PaperPlanComputedStyleProvenance struct {
	Node      string              `json:"node,omitempty"`
	Kind      string              `json:"kind"`
	Source    PaperPlanSourceSpan `json:"source"`
	TextStyle *layout.TextStyle   `json:"text_style,omitempty"`
	BoxStyle  *layout.BoxStyle    `json:"box_style,omitempty"`
}

// PaperPlanProvenance is a bounded, deterministic source projection attached
// to Explain responses. It contains paths and declarations, never scenario
// values or raw resource bytes.
type PaperPlanProvenance struct {
	Bindings       []PaperPlanBindingProvenance       `json:"bindings,omitempty"`
	StyleTokens    []PaperPlanStyleTokenProvenance    `json:"style_tokens,omitempty"`
	ComputedStyles []PaperPlanComputedStyleProvenance `json:"computed_styles,omitempty"`
}

// Provenance returns detached binding and style-token evidence for this exact
// immutable plan. A zero plan returns an error rather than an ambiguous empty
// projection.
func (p PaperPlan) Provenance() (PaperPlanProvenance, error) {
	if p.hash == "" {
		return PaperPlanProvenance{}, fmt.Errorf("document: empty paper plan")
	}
	result := PaperPlanProvenance{
		Bindings:       make([]PaperPlanBindingProvenance, 0),
		StyleTokens:    make([]PaperPlanStyleTokenProvenance, 0),
		ComputedStyles: make([]PaperPlanComputedStyleProvenance, 0),
	}
	for _, node := range p.mapping.Nodes {
		if node.BindingPath == "" {
			continue
		}
		result.Bindings = append(result.Bindings, PaperPlanBindingProvenance{
			Node: node.ID, Kind: string(node.Kind), Path: node.BindingPath,
			Source: paperPlanSourceSpan(node.BindingSpan), Nullable: node.BindingNullable,
			Collection: node.BindingCollection,
		})
	}
	for _, property := range p.mapping.ThemeProperties {
		chain := make([]PaperPlanTokenStep, len(property.Provenance.Chain))
		for index, step := range property.Provenance.Chain {
			chain[index] = PaperPlanTokenStep{Theme: step.Theme, Scope: append([]string(nil), step.Scope...), Token: step.Token, Source: paperThemeSourceSpan(step.Source)}
		}
		result.StyleTokens = append(result.StyleTokens, PaperPlanStyleTokenProvenance{
			Node: property.NodeID, Kind: string(property.Value.Kind), Property: property.Property,
			Theme: property.Theme, Token: property.Token, Value: paperThemeValueText(property.Value),
			Consumer: paperLangSourceSpan(property.ConsumerSpan), TokenChain: chain,
		})
	}
	for _, style := range p.mapping.ComputedStyles {
		entry := PaperPlanComputedStyleProvenance{Node: style.NodeID, Kind: string(style.NodeKind), Source: paperLangSourceSpan(style.Source)}
		if style.TextStyle != nil {
			copy := *style.TextStyle
			entry.TextStyle = &copy
		}
		if style.BoxStyle != nil {
			copy := *style.BoxStyle
			entry.BoxStyle = &copy
		}
		result.ComputedStyles = append(result.ComputedStyles, entry)
	}
	return result, nil
}

func clonePaperCompileMapping(input papercompile.CompileMapping) papercompile.CompileMapping {
	result := papercompile.CompileMapping{
		Nodes:           append([]papercompile.NodeMapping(nil), input.Nodes...),
		ThemeProperties: make([]papercompile.ThemePropertyMapping, len(input.ThemeProperties)),
		ComputedStyles:  make([]papercompile.ComputedStyleMapping, len(input.ComputedStyles)),
	}
	for index, property := range input.ThemeProperties {
		result.ThemeProperties[index] = property
		result.ThemeProperties[index].Provenance.Chain = make([]papertheme.TokenStep, len(property.Provenance.Chain))
		for stepIndex, step := range property.Provenance.Chain {
			result.ThemeProperties[index].Provenance.Chain[stepIndex] = papertheme.TokenStep{
				Theme: step.Theme, Scope: append([]string(nil), step.Scope...), Token: step.Token, Source: step.Source,
			}
		}
	}
	for index, style := range input.ComputedStyles {
		result.ComputedStyles[index] = style
		if style.TextStyle != nil {
			copy := *style.TextStyle
			result.ComputedStyles[index].TextStyle = &copy
		}
		if style.BoxStyle != nil {
			copy := *style.BoxStyle
			result.ComputedStyles[index].BoxStyle = &copy
		}
	}
	return result
}

func paperPlanSourceSpan(span paperlang.Span) PaperPlanSourceSpan {
	return paperLangSourceSpan(span)
}

func paperLangSourceSpan(span paperlang.Span) PaperPlanSourceSpan {
	return PaperPlanSourceSpan{File: span.File, StartOffset: span.Start.Offset, EndOffset: span.End.Offset,
		StartLine: span.Start.Line, StartColumn: span.Start.Column, EndLine: span.End.Line, EndColumn: span.End.Column}
}

func paperThemeSourceSpan(source papertheme.Source) PaperPlanSourceSpan {
	return PaperPlanSourceSpan{File: source.File, StartOffset: source.StartOffset, EndOffset: source.EndOffset,
		StartLine: source.Line, StartColumn: source.Column}
}

func paperThemeValueText(value papertheme.Value) string {
	switch value.Kind {
	case papertheme.Color:
		return value.Color
	case papertheme.String:
		return value.String
	case papertheme.Length:
		return value.Length.Number + value.Length.Unit
	case papertheme.Number:
		return value.Number
	case papertheme.Bool:
		if value.Bool {
			return "true"
		}
		return "false"
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			return ""
		}
		return string(encoded)
	}
}
