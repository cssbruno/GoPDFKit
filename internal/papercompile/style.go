// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package papercompile

import (
	"fmt"
	"strings"

	"github.com/cssbruno/gopdfkit/internal/paperlang"
)

// styleRule is deliberately a small named property bundle. It is resolved
// before layout lowering, so ordinary property precedence remains easy to
// read: a local property replaces the imported/named rule property.
type styleRule struct {
	properties map[string]paperlang.Property
	span       paperlang.Span
}

var styleRuleProperties = func() map[string]bool {
	properties := copyPropertySet(textStyleProperties)
	properties["style"] = true
	for _, name := range boxPropertyNames {
		properties[name] = true
	}
	return properties
}()

func (c *compiler) collectStyleRules(root *paperlang.Node) {
	c.styleRules = make(map[string]styleRule)
	if root == nil {
		return
	}
	order := make([]string, 0)
	raw := make(map[string]styleRule)
	for _, member := range root.Members {
		style := member.Node
		if style == nil || style.Kind != paperlang.NodeStyle {
			continue
		}
		name := strings.TrimPrefix(strings.TrimSpace(style.ID), "@")
		if name == "" {
			c.add("PAPER_STYLE_NAME", "style requires a readable @name", "write style @body:", style.HeaderSpan)
			continue
		}
		if _, duplicate := raw[name]; duplicate {
			c.add("PAPER_STYLE_DUPLICATE", fmt.Sprintf("style @%s is defined more than once", name), "keep one definition per style name", style.HeaderSpan)
			continue
		}
		properties := make(map[string]paperlang.Property)
		for _, child := range style.Members {
			if child.Node != nil {
				c.add("PAPER_STYLE_CHILD", fmt.Sprintf("style cannot contain %s", child.Node.Kind), "use named properties inside the style", child.Node.HeaderSpan)
				continue
			}
			if child.Property == nil {
				continue
			}
			property := *child.Property
			if _, duplicate := properties[property.Name]; duplicate {
				c.add("PAPER_COMPILE_DUPLICATE_PROPERTY", fmt.Sprintf("property %q is repeated on style", property.Name), "remove the duplicate; the first value is retained", property.Span)
				continue
			}
			properties[property.Name] = property
			if !styleRuleProperties[property.Name] {
				c.add("PAPER_STYLE_PROPERTY", fmt.Sprintf("property %q is unsupported in style @%s", property.Name, name), "use text and box design properties", property.Span)
			}
		}
		raw[name] = styleRule{properties: properties, span: style.HeaderSpan}
		order = append(order, name)
	}

	state := make(map[string]uint8)
	for _, name := range order {
		c.resolveStyleRule(name, raw, state, nil)
	}
}

func (c *compiler) resolveStyleRule(name string, raw map[string]styleRule, state map[string]uint8, chain []string) map[string]paperlang.Property {
	if state[name] == 2 {
		return c.styleRules[name].properties
	}
	if state[name] == 1 {
		c.add("PAPER_STYLE_CYCLE", fmt.Sprintf("style inheritance cycle includes @%s", name), "remove the cycle from style properties", raw[name].span)
		return nil
	}
	declaration, exists := raw[name]
	if !exists {
		c.add("PAPER_STYLE_UNKNOWN", fmt.Sprintf("style @%s is not declared", name), "define the style or import the file that contains it", paperlang.Span{})
		return nil
	}
	state[name] = 1
	resolved := make(map[string]paperlang.Property)
	if property, ok := declaration.properties["style"]; ok {
		if property.Value.Kind != paperlang.ScalarString || property.Value.StringValue == nil {
			c.typeError(property, "quoted style name")
		} else {
			parent := strings.TrimPrefix(strings.TrimSpace(*property.Value.StringValue), "@")
			if parent == "" {
				c.add("PAPER_STYLE_UNKNOWN", "style reference cannot be empty", "use style: \"@base\"", property.Value.Span)
			} else if containsString(chain, parent) || parent == name {
				c.add("PAPER_STYLE_CYCLE", fmt.Sprintf("style inheritance cycle includes @%s", parent), "remove the cycle from style properties", property.Value.Span)
			} else if inherited := c.resolveStyleRule(parent, raw, state, append(chain, name)); inherited != nil {
				for key, value := range inherited {
					resolved[key] = value
				}
			}
		}
	}
	for key, value := range declaration.properties {
		if key != "style" {
			resolved[key] = value
		}
	}
	c.styleRules[name] = styleRule{properties: resolved, span: declaration.span}
	state[name] = 2
	return resolved
}

func (c *compiler) applyStyle(properties map[string]paperlang.Property, supported map[string]bool) map[string]paperlang.Property {
	property, ok := properties["style"]
	if !ok {
		return properties
	}
	if property.Value.Kind != paperlang.ScalarString || property.Value.StringValue == nil {
		c.typeError(property, "quoted style name")
		return properties
	}
	name := strings.TrimPrefix(strings.TrimSpace(*property.Value.StringValue), "@")
	rule, exists := c.styleRules[name]
	if !exists {
		c.add("PAPER_STYLE_UNKNOWN", fmt.Sprintf("style @%s is not declared", name), "define the style or import the file that contains it", property.Value.Span)
		return properties
	}
	merged := make(map[string]paperlang.Property, len(properties)+len(rule.properties))
	for key, value := range rule.properties {
		if !supported[key] {
			c.add("PAPER_STYLE_PROPERTY", fmt.Sprintf("style property %q is unsupported on this %s", key, "node"), "use a style with properties supported by the target node", value.Span)
			continue
		}
		merged[key] = value
	}
	for key, value := range properties {
		merged[key] = value
	}
	return merged
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
