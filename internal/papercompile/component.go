// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package papercompile

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cssbruno/paperrune/internal/paperexpr"
	"github.com/cssbruno/paperrune/internal/paperlang"
	"github.com/cssbruno/paperrune/internal/paperscenario"
)

// ExpansionLimits bounds deterministic component instantiation. Zero selects
// defaults; partially specified or over-hard-cap limits produce diagnostics.
type ExpansionLimits struct {
	MaxDepth      uint32
	MaxNodes      uint32
	MaxComponents uint32
}

func DefaultExpansionLimits() ExpansionLimits {
	return ExpansionLimits{MaxDepth: 32, MaxNodes: 100_000, MaxComponents: 4096}
}

type expansionProvenance struct {
	definition      paperlang.Span
	invocation      paperlang.Span
	instancePath    string
	bindingBase     string
	bindingSpan     paperlang.Span
	bindingRequired bool
	repeatItem      bool
	repeatSource    string
	repeatKey       string
	repeatItemBase  string
	repeatValue     paperscenario.Value
	repeatFields    []FieldDescriptor
	loopItem        bool
	loopIndex       int64
	loopFirst       bool
	loopLast        bool
	loopRoot        paperscenario.Value
	loopEnvironment []paperexpr.PathKind
}

type componentExpansionResult struct {
	ast         paperlang.AST
	provenance  map[*paperlang.Node]expansionProvenance
	diagnostics []paperlang.Diagnostic
}

type componentExpander struct {
	limits      ExpansionLimits
	definitions map[string]*paperlang.Node
	provenance  map[*paperlang.Node]expansionProvenance
	diagnostics []paperlang.Diagnostic
	stack       map[string]bool
	nodes       uint32
	serial      uint32
	limitHit    bool
	scenario    string
	contracts   map[*paperlang.Node]componentContract
}

type expansionOrigin struct {
	definition      paperlang.Span
	invocation      paperlang.Span
	instancePath    string
	prefix          string
	preserveIDs     bool
	bindingBase     string
	bindingSpan     paperlang.Span
	bindingRequired bool
	props           map[string]paperlang.Scalar
}

type componentProp struct {
	name         string
	typeName     string
	required     bool
	defaultValue *paperlang.Scalar
}

type componentContract struct {
	props map[string]componentProp
	slots map[string]componentSlotContract
}

func expandComponents(ast paperlang.AST, limits ExpansionLimits, scenario string) componentExpansionResult {
	expander := componentExpander{
		limits: limits, definitions: make(map[string]*paperlang.Node),
		provenance: make(map[*paperlang.Node]expansionProvenance), stack: make(map[string]bool),
		scenario:  strings.TrimPrefix(strings.TrimSpace(scenario), "@"),
		contracts: make(map[*paperlang.Node]componentContract),
	}
	if limits == (ExpansionLimits{}) {
		expander.limits = DefaultExpansionLimits()
	} else if limits.MaxDepth == 0 || limits.MaxNodes == 0 || limits.MaxComponents == 0 ||
		limits.MaxDepth > 128 || limits.MaxNodes > 1_000_000 || limits.MaxComponents > 16_384 {
		expander.add("PAPER_COMPONENT_LIMITS", "component expansion limits are incomplete or exceed hard caps", "use positive limits within the documented hard caps", rootSpan(ast))
		expander.limits = DefaultExpansionLimits()
	}
	result := componentExpansionResult{ast: paperlang.AST{File: ast.File}, provenance: expander.provenance}
	if ast.Root == nil {
		result.diagnostics = expander.diagnostics
		return result
	}
	if ast.Root.Kind != paperlang.NodeDocument {
		result.ast.Root = expander.cloneOrdinary(ast.Root, expansionOrigin{}, 0)
		result.diagnostics = expander.diagnostics
		return result
	}
	for _, member := range ast.Root.Members {
		if member.Node == nil || member.Node.Kind != paperlang.NodeComponent {
			continue
		}
		definition := member.Node
		if definition.ID == "" {
			expander.add("PAPER_COMPONENT_NAME", "component definition requires a readable @name", "write component @name:", definition.HeaderSpan)
			continue
		}
		if _, duplicate := expander.definitions[definition.ID]; duplicate {
			expander.add("PAPER_COMPONENT_DUPLICATE", fmt.Sprintf("component %s is defined more than once", definition.ID), "keep one definition per component name", definition.HeaderSpan)
			continue
		}
		if uint32(len(expander.definitions)) >= expander.limits.MaxComponents {
			expander.add("PAPER_COMPONENT_LIMIT", "component definition count exceeds the configured limit", "split the document or raise the bounded component limit", definition.HeaderSpan)
			continue
		}
		expander.definitions[definition.ID] = definition
		expander.validateDefinition(definition)
	}

	root := expander.cloneHeader(ast.Root, expansionOrigin{})
	if root != nil {
		for _, member := range ast.Root.Members {
			if member.Property != nil {
				root.Members = append(root.Members, cloneExpansionProperty(member.Property))
				continue
			}
			if member.Node == nil || member.Node.Kind == paperlang.NodeComponent || member.Node.Kind == paperlang.NodeSchema || member.Node.Kind == paperlang.NodeObjectType || member.Node.Kind == paperlang.NodeScenario || member.Node.Kind == paperlang.NodeTheme || member.Node.Kind == paperlang.NodeStyle {
				continue
			}
			for _, node := range expander.expandNode(member.Node, expansionOrigin{}, 0) {
				root.Members = append(root.Members, paperlang.Member{Node: node})
			}
		}
	}
	result.ast.Root = root
	result.diagnostics = expander.diagnostics
	return result
}

func (e *componentExpander) validateDefinition(definition *paperlang.Node) {
	slots := make(map[string]bool)
	slotContracts := make(map[string]componentSlotContract)
	props := make(map[string]componentProp)
	for _, member := range definition.Members {
		if member.Property != nil {
			e.add("PAPER_COMPONENT_BINDING_UNSUPPORTED", fmt.Sprintf("component property %q is not supported", member.Property.Name), "declare typed values with prop @name:", member.Property.Span)
			continue
		}
		if member.Node != nil && member.Node.Kind == paperlang.NodeProp {
			declaration := e.componentProp(member.Node)
			if declaration.name != "" {
				if _, duplicate := props[declaration.name]; duplicate {
					e.add("PAPER_COMPONENT_PROP_DUPLICATE", fmt.Sprintf("prop @%s is declared more than once", declaration.name), "keep one declaration per prop name", member.Node.HeaderSpan)
				} else {
					props[declaration.name] = declaration
				}
			}
			continue
		}
		if member.Node == nil || member.Node.Kind != paperlang.NodeSlot {
			continue
		}
		slot := member.Node
		if slot.ID == "" {
			e.add("PAPER_SLOT_NAME", "slot requires a readable @name", "write slot @name:", slot.HeaderSpan)
			continue
		}
		if slots[slot.ID] {
			e.add("PAPER_SLOT_DUPLICATE", fmt.Sprintf("slot %s is declared more than once", slot.ID), "keep one slot declaration per component", slot.HeaderSpan)
			continue
		}
		slots[slot.ID] = true
		slotContracts[slot.ID] = e.slotContract(slot)
	}
	e.contracts[definition] = componentContract{props: props, slots: slotContracts}
}

func (e *componentExpander) componentProp(node *paperlang.Node) componentProp {
	declaration := componentProp{name: strings.TrimPrefix(node.ID, "@")}
	if declaration.name == "" {
		e.add("PAPER_COMPONENT_PROP_NAME", "prop requires a readable @name", "write prop @name:", node.HeaderSpan)
	}
	seen := make(map[string]bool)
	for _, member := range node.Members {
		if member.Node != nil {
			e.add("PAPER_COMPONENT_PROP_CHILD", "prop declarations accept scalar contract properties only", "use type, required, and default properties", member.Node.HeaderSpan)
			continue
		}
		if member.Property == nil {
			continue
		}
		property := member.Property
		if seen[property.Name] {
			e.add("PAPER_COMPONENT_PROP_CONTRACT_DUPLICATE", fmt.Sprintf("prop property %q is repeated", property.Name), "keep one contract value", property.Span)
			continue
		}
		seen[property.Name] = true
		switch property.Name {
		case "type":
			if property.Value.Kind != paperlang.ScalarString || property.Value.StringValue == nil {
				e.add("PAPER_COMPONENT_PROP_TYPE", "prop type requires a quoted scalar type", "use string, bool, number, unit, or any", property.Value.Span)
				continue
			}
			declaration.typeName = strings.ToLower(strings.TrimSpace(*property.Value.StringValue))
			if !validComponentPropType(declaration.typeName) {
				e.add("PAPER_COMPONENT_PROP_TYPE", fmt.Sprintf("prop type %q is unsupported", declaration.typeName), "use string, bool, number, unit, or any", property.Value.Span)
			}
		case "required":
			if property.Value.Kind != paperlang.ScalarBool || property.Value.BoolValue == nil {
				e.add("PAPER_COMPONENT_PROP_REQUIRED", "prop required must be boolean", "use true or false", property.Value.Span)
				continue
			}
			declaration.required = *property.Value.BoolValue
		case "default":
			value := cloneExpansionScalar(property.Value)
			declaration.defaultValue = &value
		default:
			e.add("PAPER_COMPONENT_PROP_CONTRACT", fmt.Sprintf("prop property %q is unsupported", property.Name), "use type, required, and default", property.Span)
		}
	}
	if declaration.typeName == "" {
		e.add("PAPER_COMPONENT_PROP_TYPE", fmt.Sprintf("prop @%s has no type", declaration.name), "add type: \"string\", \"bool\", \"number\", \"unit\", or \"any\"", node.HeaderSpan)
	}
	if declaration.required && declaration.defaultValue != nil {
		e.add("PAPER_COMPONENT_PROP_AMBIGUOUS", fmt.Sprintf("required prop @%s cannot also have a default", declaration.name), "remove required: true or remove default", node.HeaderSpan)
	}
	if declaration.defaultValue != nil && validComponentPropType(declaration.typeName) && !componentScalarMatches(declaration.typeName, *declaration.defaultValue) {
		e.add("PAPER_COMPONENT_PROP_DEFAULT_TYPE", fmt.Sprintf("default for prop @%s does not match %s", declaration.name, declaration.typeName), "use a default of the declared scalar type", declaration.defaultValue.Span)
	}
	return declaration
}

func (e *componentExpander) expandNode(source *paperlang.Node, origin expansionOrigin, depth uint32) []*paperlang.Node {
	if source == nil || e.limitHit {
		return nil
	}
	if source.Kind == paperlang.NodeUse {
		return e.expandUse(source, origin, depth)
	}
	if source.Kind == paperlang.NodeComponent || source.Kind == paperlang.NodeProp || source.Kind == paperlang.NodeArg || source.Kind == paperlang.NodeSlot || source.Kind == paperlang.NodeFill || source.Kind == paperlang.NodeSchema || source.Kind == paperlang.NodeObjectType || source.Kind == paperlang.NodeField || source.Kind == paperlang.NodeTheme || source.Kind == paperlang.NodeStyle || source.Kind == paperlang.NodeToken || source.Kind == paperlang.NodeScope {
		e.add("PAPER_COMPONENT_HIERARCHY", fmt.Sprintf("%s cannot appear in expanded output", source.Kind), "keep definitions at document scope and slots/fills in their owning component/use", source.HeaderSpan)
		return nil
	}
	clone := e.cloneHeader(source, origin)
	if clone == nil {
		return nil
	}
	for _, member := range source.Members {
		if member.Property != nil {
			clone.Members = append(clone.Members, cloneExpansionPropertyWithProps(member.Property, origin.props))
			continue
		}
		for _, child := range e.expandNode(member.Node, origin, depth) {
			clone.Members = append(clone.Members, paperlang.Member{Node: child})
		}
	}
	return []*paperlang.Node{clone}
}

func (e *componentExpander) expandUse(use *paperlang.Node, parent expansionOrigin, depth uint32) []*paperlang.Node {
	if depth >= e.limits.MaxDepth {
		e.add("PAPER_COMPONENT_DEPTH", "component expansion exceeds the configured recursion depth", "reduce nested uses or raise the bounded depth limit", use.HeaderSpan)
		return nil
	}
	instanceID := use.ID
	if parent.instancePath != "" && !parent.preserveIDs {
		name := strings.TrimPrefix(instanceID, "@")
		if name != "" {
			instanceID = parent.prefix + "--" + name
			if !strings.HasPrefix(instanceID, "@") {
				instanceID = "@" + instanceID
			}
		}
	}
	if instanceID == "" {
		e.add("PAPER_COMPONENT_INSTANCE", "use requires a unique readable @instance ID", "write use @instance:", use.HeaderSpan)
		return nil
	}
	componentName := ""
	bindingBase, bindingSpan, bindingRequired := parent.bindingBase, parent.bindingSpan, parent.bindingRequired
	if parent.bindingBase == "" {
		bindingRequired = true
	}
	fills := make(map[string][]*paperlang.Node)
	args := make(map[string]paperlang.Scalar)
	for _, member := range use.Members {
		if member.Property != nil {
			property := member.Property
			value := substituteComponentScalar(property.Value, parent.props)
			if property.Name == "bind" {
				if value.Kind != paperlang.ScalarString || value.StringValue == nil {
					e.add("PAPER_BIND_PATH", "bind requires a quoted schema path", "use field.path with one schema, schema.field with several schemas, or a component-relative path", value.Span)
					continue
				}
				if strings.HasPrefix(strings.TrimSpace(*value.StringValue), "@") {
					e.add("PAPER_BIND_PATH", "bind paths no longer use @schema prefixes", "remove @ and use a root-relative path", value.Span)
					continue
				}
				bindingBase = combineBindingPath(parent.bindingBase, *value.StringValue)
				bindingSpan = value.Span
				continue
			}
			if property.Name == "bind-required" {
				if value.Kind != paperlang.ScalarBool || value.BoolValue == nil {
					e.add("PAPER_BIND_REQUIRED", "bind-required must be boolean", "use true or false", property.Value.Span)
					continue
				}
				bindingRequired = *value.BoolValue
				continue
			}
			if property.Name != "component" {
				e.add("PAPER_COMPONENT_BINDING_UNSUPPORTED", fmt.Sprintf("use property %q is not supported", property.Name), "data bindings and scenarios are not part of the initial component contract", property.Span)
				continue
			}
			if value.Kind != paperlang.ScalarString || value.StringValue == nil {
				e.add("PAPER_COMPONENT_REFERENCE", "component property requires a quoted @component name", "write component: \"@name\"", property.Value.Span)
				continue
			}
			componentName = *value.StringValue
			continue
		}
		if member.Node != nil && member.Node.Kind == paperlang.NodeArg {
			arg := member.Node
			name := strings.TrimPrefix(arg.ID, "@")
			if name == "" || arg.Value == nil {
				e.add("PAPER_COMPONENT_ARG", "arg requires a readable @name and scalar value", "write arg @name: value", arg.HeaderSpan)
				continue
			}
			if _, duplicate := args[name]; duplicate {
				e.add("PAPER_COMPONENT_ARG_DUPLICATE", fmt.Sprintf("arg @%s is passed more than once", name), "keep one argument per prop", arg.HeaderSpan)
				continue
			}
			args[name] = substituteComponentScalar(*arg.Value, parent.props)
			continue
		}
		if member.Node == nil || member.Node.Kind != paperlang.NodeFill {
			e.add("PAPER_COMPONENT_HIERARCHY", "use accepts only named arg and fill children", "use arg @prop: value or fill @slot:", memberSpanForExpansion(member))
			continue
		}
		fill := member.Node
		if fill.ID == "" {
			e.add("PAPER_FILL_NAME", "fill requires a readable @slot name", "write fill @slot:", fill.HeaderSpan)
			continue
		}
		for _, fillMember := range fill.Members {
			if fillMember.Property != nil && fillMember.Property.Name != "scenario" {
				e.add("PAPER_FILL_PROPERTY", fmt.Sprintf("fill property %q is not supported", fillMember.Property.Name), "fill accepts only scenario plus document nodes", fillMember.Property.Span)
			}
		}
		fills[fill.ID] = append(fills[fill.ID], fill)
	}
	if componentName == "" {
		e.add("PAPER_COMPONENT_REFERENCE", "use has no component reference", "add component: \"@name\"", use.HeaderSpan)
		return nil
	}
	definition := e.definitions[componentName]
	if definition == nil {
		e.add("PAPER_COMPONENT_UNKNOWN", fmt.Sprintf("component %s is not defined", componentName), "define it under document before using it", use.HeaderSpan)
		return nil
	}
	resolvedProps := make(map[string]paperlang.Scalar)
	contract := e.contracts[definition]
	argNames := make([]string, 0, len(args))
	for name := range args {
		argNames = append(argNames, name)
	}
	sort.Strings(argNames)
	for _, name := range argNames {
		declaration, exists := contract.props[name]
		if !exists {
			e.add("PAPER_COMPONENT_ARG_UNKNOWN", fmt.Sprintf("component %s has no prop @%s", componentName, name), "remove the argument or declare the prop", use.HeaderSpan)
			continue
		}
		value := args[name]
		if !componentScalarMatches(declaration.typeName, value) {
			e.add("PAPER_COMPONENT_ARG_TYPE", fmt.Sprintf("arg @%s does not match declared type %s", name, declaration.typeName), "pass a scalar of the declared type", value.Span)
			continue
		}
		resolvedProps[name] = value
	}
	propNames := make([]string, 0, len(contract.props))
	for name := range contract.props {
		propNames = append(propNames, name)
	}
	sort.Strings(propNames)
	for _, name := range propNames {
		if _, present := resolvedProps[name]; present {
			continue
		}
		declaration := contract.props[name]
		if declaration.defaultValue != nil {
			resolvedProps[name] = cloneExpansionScalar(*declaration.defaultValue)
		} else if declaration.required {
			e.add("PAPER_COMPONENT_ARG_MISSING", fmt.Sprintf("required prop @%s is not provided", name), "add a matching arg to the use instance", use.HeaderSpan)
		}
	}
	if e.stack[componentName] {
		e.add("PAPER_COMPONENT_CYCLE", fmt.Sprintf("component cycle reaches %s", componentName), "remove the recursive use chain", use.HeaderSpan)
		return nil
	}
	e.stack[componentName] = true
	defer delete(e.stack, componentName)
	instancePath := instanceID
	if parent.instancePath != "" {
		instancePath = parent.instancePath + "/" + instanceID
	}
	invocationSpan := use.Span
	if parent.invocation.File != "" {
		invocationSpan = parent.invocation
	}
	slots := make(map[string]*paperlang.Node)
	for _, member := range definition.Members {
		if member.Node != nil && member.Node.Kind == paperlang.NodeSlot && member.Node.ID != "" {
			if _, exists := slots[member.Node.ID]; !exists {
				slots[member.Node.ID] = member.Node
			}
		}
	}
	fillNames := make([]string, 0, len(fills))
	for name := range fills {
		fillNames = append(fillNames, name)
	}
	sort.Strings(fillNames)
	for _, name := range fillNames {
		variants := fills[name]
		if slots[name] == nil {
			for _, fill := range variants {
				e.add("PAPER_FILL_UNKNOWN", fmt.Sprintf("component %s has no slot %s", componentName, name), "remove the fill or use a declared slot name", fill.HeaderSpan)
			}
		}
	}

	output := make([]*paperlang.Node, 0)
	for _, member := range definition.Members {
		if member.Node == nil {
			continue
		}
		template := member.Node
		if template.Kind == paperlang.NodeProp {
			continue
		}
		if template.Kind != paperlang.NodeSlot {
			origin := expansionOrigin{definition: template.Span, invocation: invocationSpan, instancePath: instancePath, prefix: instanceID,
				bindingBase: bindingBase, bindingSpan: bindingSpan, bindingRequired: bindingRequired, props: resolvedProps}
			output = append(output, e.expandNode(template, origin, depth+1)...)
			continue
		}
		if template.ID == "" || slots[template.ID] != template {
			continue
		}
		contract := e.contracts[definition].slots[template.ID]
		fill := e.selectSlotFill(template, contract, fills[template.ID])
		selected := componentChildNodes(template)
		origin := expansionOrigin{definition: template.Span, invocation: invocationSpan, instancePath: instancePath + "/" + template.ID, prefix: instanceID + "--" + strings.TrimPrefix(template.ID, "@"),
			bindingBase: bindingBase, bindingSpan: bindingSpan, bindingRequired: bindingRequired, props: resolvedProps}
		if fill != nil {
			selected = componentChildNodes(fill)
			origin.invocation = fill.Span
			origin.preserveIDs = true
		} else if contract.required && len(selected) == 0 {
			e.add("PAPER_SLOT_MISSING", fmt.Sprintf("required slot %s is not filled", template.ID), "add a matching fill block to the use instance", use.HeaderSpan)
			continue
		}
		if contract.cardinality == "one" && len(selected) != 1 {
			e.add("PAPER_SLOT_CARDINALITY", fmt.Sprintf("slot %s requires exactly one child, got %d", template.ID, len(selected)), "provide exactly one compatible child", template.HeaderSpan)
			continue
		}
		for _, selectedNode := range selected {
			if !componentSlotAccepts(contract.slotType, selectedNode.Kind) {
				e.add("PAPER_SLOT_TYPE", fmt.Sprintf("slot %s of type %q cannot accept %s", template.ID, contract.slotType, selectedNode.Kind), "change the fill content or slot type", selectedNode.HeaderSpan)
				continue
			}
			childOrigin := origin
			if fill == nil {
				childOrigin.definition = selectedNode.Span
			} else {
				childOrigin.invocation = selectedNode.Span
			}
			output = append(output, e.expandNode(selectedNode, childOrigin, depth+1)...)
		}
	}
	return output
}

type componentSlotContract struct {
	slotType        string
	required        bool
	cardinality     string
	layoutAffecting bool
	scenarios       map[string]bool
}

func (e *componentExpander) slotContract(slot *paperlang.Node) componentSlotContract {
	contract := componentSlotContract{slotType: "blocks", cardinality: "many", scenarios: make(map[string]bool)}
	for _, member := range slot.Members {
		if member.Property == nil {
			continue
		}
		property := member.Property
		switch property.Name {
		case "type":
			if property.Value.Kind != paperlang.ScalarString || property.Value.StringValue == nil {
				e.add("PAPER_SLOT_TYPE", "slot type requires a quoted name", "use blocks, text, list, or row-column", property.Value.Span)
				continue
			}
			contract.slotType = strings.ToLower(strings.TrimSpace(*property.Value.StringValue))
			if contract.slotType != "blocks" && contract.slotType != "text" && contract.slotType != "list" && contract.slotType != "row-column" {
				e.add("PAPER_SLOT_TYPE", fmt.Sprintf("slot type %q is unsupported", contract.slotType), "use blocks, text, list, or row-column", property.Value.Span)
			}
		case "required":
			if property.Value.Kind != paperlang.ScalarBool || property.Value.BoolValue == nil {
				e.add("PAPER_SLOT_REQUIRED", "required must be boolean", "use required: true or required: false", property.Value.Span)
				continue
			}
			contract.required = *property.Value.BoolValue
		case "cardinality":
			if property.Value.Kind != paperlang.ScalarString || property.Value.StringValue == nil {
				e.add("PAPER_SLOT_CARDINALITY", "slot cardinality requires a quoted name", "use one or many", property.Value.Span)
				continue
			}
			contract.cardinality = strings.ToLower(strings.TrimSpace(*property.Value.StringValue))
			if contract.cardinality != "one" && contract.cardinality != "many" {
				e.add("PAPER_SLOT_CARDINALITY", fmt.Sprintf("slot cardinality %q is unsupported", contract.cardinality), "use one or many", property.Value.Span)
			}
		case "layout-affecting":
			if property.Value.Kind != paperlang.ScalarBool || property.Value.BoolValue == nil {
				e.add("PAPER_SLOT_LAYOUT", "layout-affecting must be boolean", "use true or false", property.Value.Span)
				continue
			}
			contract.layoutAffecting = *property.Value.BoolValue
		case "scenarios":
			if property.Value.Kind != paperlang.ScalarString || property.Value.StringValue == nil {
				e.add("PAPER_SLOT_SCENARIOS", "scenarios requires a quoted comma-separated list", "use scenarios: \"@compact, @expanded\"", property.Value.Span)
				continue
			}
			for _, raw := range strings.Split(*property.Value.StringValue, ",") {
				name := strings.TrimPrefix(strings.TrimSpace(raw), "@")
				if name == "" || !validScenarioContractName(name) {
					e.add("PAPER_SLOT_SCENARIOS", fmt.Sprintf("scenario name %q is invalid", strings.TrimSpace(raw)), "use bounded readable scenario names", property.Value.Span)
					continue
				}
				if contract.scenarios[name] {
					e.add("PAPER_SLOT_SCENARIOS", fmt.Sprintf("scenario @%s is listed more than once", name), "keep each scenario once", property.Value.Span)
				}
				contract.scenarios[name] = true
			}
		default:
			e.add("PAPER_SLOT_CONTRACT", fmt.Sprintf("slot property %q is not supported", property.Name), "use type, required, cardinality, layout-affecting, and scenarios", property.Span)
		}
	}
	if contract.required && len(componentChildNodes(slot)) != 0 {
		e.add("PAPER_SLOT_DEFAULT_REQUIRED", fmt.Sprintf("required slot %s cannot also define default children", slot.ID), "remove required: true or remove the defaults", slot.HeaderSpan)
	}
	if contract.layoutAffecting && len(contract.scenarios) == 0 {
		e.add("PAPER_SLOT_SCENARIOS_REQUIRED", fmt.Sprintf("layout-affecting slot %s requires scenario names", slot.ID), "add scenarios: \"@compact, @expanded\"", slot.HeaderSpan)
	}
	if !contract.layoutAffecting && len(contract.scenarios) != 0 {
		e.add("PAPER_SLOT_SCENARIOS_LAYOUT", fmt.Sprintf("slot %s declares scenarios but is not layout-affecting", slot.ID), "add layout-affecting: true or remove scenarios", slot.HeaderSpan)
	}
	return contract
}

func (e *componentExpander) selectSlotFill(slot *paperlang.Node, contract componentSlotContract, fills []*paperlang.Node) *paperlang.Node {
	if !contract.layoutAffecting {
		if len(fills) > 1 {
			e.add("PAPER_FILL_DUPLICATE", fmt.Sprintf("slot %s is filled more than once", slot.ID), "merge content into one fill block", fills[1].HeaderSpan)
		}
		for _, fill := range fills {
			if scenario, present := fillScenario(fill); present {
				e.add("PAPER_FILL_SCENARIO_UNEXPECTED", fmt.Sprintf("fill %s names scenario @%s for a non-layout slot", fill.ID, scenario), "remove scenario or mark the slot layout-affecting", fill.HeaderSpan)
			}
		}
		if len(fills) != 0 {
			return fills[0]
		}
		return nil
	}
	if e.scenario == "" {
		e.add("PAPER_SLOT_SCENARIO_REQUIRED", fmt.Sprintf("layout-affecting slot %s requires an explicitly selected scenario", slot.ID), "compile with one declared scenario", slot.HeaderSpan)
		return nil
	}
	if !contract.scenarios[e.scenario] {
		e.add("PAPER_SLOT_SCENARIO_UNKNOWN", fmt.Sprintf("scenario @%s is not allowed by slot %s", e.scenario, slot.ID), "select one scenario named by the slot contract", slot.HeaderSpan)
		return nil
	}
	var selected *paperlang.Node
	for _, fill := range fills {
		scenario, present := fillScenario(fill)
		if !present || scenario == "" {
			e.add("PAPER_FILL_SCENARIO_REQUIRED", fmt.Sprintf("fill %s for a layout-affecting slot requires scenario", fill.ID), "add scenario: \"@name\"", fill.HeaderSpan)
			continue
		}
		if !contract.scenarios[scenario] {
			e.add("PAPER_FILL_SCENARIO_UNKNOWN", fmt.Sprintf("fill %s uses undeclared scenario @%s", fill.ID, scenario), "use one scenario named by the slot contract", fill.HeaderSpan)
			continue
		}
		if scenario != e.scenario {
			continue
		}
		if selected != nil {
			e.add("PAPER_FILL_DUPLICATE", fmt.Sprintf("slot %s has multiple fills for scenario @%s", slot.ID, scenario), "keep one fill per slot and scenario", fill.HeaderSpan)
			continue
		}
		selected = fill
	}
	return selected
}

func (e *componentExpander) cloneOrdinary(source *paperlang.Node, origin expansionOrigin, depth uint32) *paperlang.Node {
	nodes := e.expandNode(source, origin, depth)
	if len(nodes) == 0 {
		return nil
	}
	return nodes[0]
}

func (e *componentExpander) cloneHeader(source *paperlang.Node, origin expansionOrigin) *paperlang.Node {
	if e.nodes >= e.limits.MaxNodes {
		if !e.limitHit {
			e.add("PAPER_COMPONENT_NODE_LIMIT", "component expansion exceeds the configured node limit", "reduce instances or raise the bounded node limit", source.HeaderSpan)
		}
		e.limitHit = true
		return nil
	}
	e.nodes++
	clone := &paperlang.Node{Kind: source.Kind, ID: source.ID, FieldType: source.FieldType, TypeRef: source.TypeRef, ItemType: source.ItemType, ItemTypeRef: source.ItemTypeRef, Optional: source.Optional, HeaderSpan: source.HeaderSpan, Span: source.Span}
	if source.Value != nil {
		value := substituteComponentScalar(*source.Value, origin.props)
		clone.Value = &value
	}
	if origin.instancePath != "" {
		if !origin.preserveIDs || clone.ID == "" {
			e.serial++
			name := strings.TrimPrefix(clone.ID, "@")
			if name == "" {
				name = fmt.Sprintf("node-%d", e.serial)
			}
			clone.ID = origin.prefix + "--" + name
			if !strings.HasPrefix(clone.ID, "@") {
				clone.ID = "@" + clone.ID
			}
		}
		e.provenance[clone] = expansionProvenance{definition: origin.definition, invocation: origin.invocation, instancePath: origin.instancePath,
			bindingBase: origin.bindingBase, bindingSpan: origin.bindingSpan, bindingRequired: origin.bindingRequired}
	}
	return clone
}

func (e *componentExpander) add(code, message, hint string, span paperlang.Span) {
	e.diagnostics = append(e.diagnostics, paperlang.Diagnostic{Code: code, Severity: paperlang.SeverityError, Message: message, Hint: hint, Span: span})
}

func componentChildNodes(node *paperlang.Node) []*paperlang.Node {
	children := make([]*paperlang.Node, 0, len(node.Members))
	for _, member := range node.Members {
		if member.Node != nil {
			children = append(children, member.Node)
		}
	}
	return children
}

func componentSlotAccepts(slotType string, kind paperlang.NodeKind) bool {
	switch slotType {
	case "blocks":
		return componentBodyKind(kind)
	case "text":
		return kind == paperlang.NodeText || kind == paperlang.NodeParagraph || kind == paperlang.NodeHeading
	case "list":
		return kind == paperlang.NodeList
	case "row-column":
		return kind == paperlang.NodeRow || kind == paperlang.NodeColumn
	default:
		return false
	}
}

func componentBodyKind(kind paperlang.NodeKind) bool {
	return kind == paperlang.NodeText || kind == paperlang.NodeParagraph || kind == paperlang.NodeHeading ||
		kind == paperlang.NodeList || kind == paperlang.NodePageBreak || kind == paperlang.NodeRow ||
		kind == paperlang.NodeColumn || kind == paperlang.NodeImage || kind == paperlang.NodeTable || kind == paperlang.NodeUse || kind == paperlang.NodeRepeat || kind == paperlang.NodeLoop
}

func cloneExpansionProperty(source *paperlang.Property) paperlang.Member {
	if source == nil {
		return paperlang.Member{}
	}
	clone := *source
	clone.Value = cloneExpansionScalar(source.Value)
	return paperlang.Member{Property: &clone}
}

func cloneExpansionPropertyWithProps(source *paperlang.Property, props map[string]paperlang.Scalar) paperlang.Member {
	if source == nil {
		return paperlang.Member{}
	}
	clone := *source
	clone.Value = substituteComponentScalar(source.Value, props)
	return paperlang.Member{Property: &clone}
}

func substituteComponentScalar(source paperlang.Scalar, props map[string]paperlang.Scalar) paperlang.Scalar {
	clone := cloneExpansionScalar(source)
	if source.Kind != paperlang.ScalarString || source.StringValue == nil || len(props) == 0 {
		return clone
	}
	value := strings.TrimSpace(*source.StringValue)
	if len(value) < 4 || !strings.HasPrefix(value, "${") || !strings.HasSuffix(value, "}") {
		return clone
	}
	name := strings.TrimSpace(value[2 : len(value)-1])
	if replacement, exists := props[name]; exists {
		return cloneExpansionScalar(replacement)
	}
	return clone
}

func validComponentPropType(name string) bool {
	return name == "string" || name == "bool" || name == "number" || name == "unit" || name == "length" || name == "any"
}

func componentScalarMatches(typeName string, value paperlang.Scalar) bool {
	switch typeName {
	case "string":
		return value.Kind == paperlang.ScalarString
	case "bool":
		return value.Kind == paperlang.ScalarBool
	case "number":
		return value.Kind == paperlang.ScalarNumber
	case "unit", "length":
		return value.Kind == paperlang.ScalarUnit
	case "any":
		return true
	default:
		return false
	}
}

func fillScenario(fill *paperlang.Node) (string, bool) {
	for _, member := range fill.Members {
		if member.Property == nil || member.Property.Name != "scenario" {
			continue
		}
		value := member.Property.Value
		if value.Kind != paperlang.ScalarString || value.StringValue == nil {
			return "", true
		}
		return strings.TrimPrefix(strings.TrimSpace(*value.StringValue), "@"), true
	}
	return "", false
}

func validScenarioContractName(name string) bool {
	if name == "" || len(name) > 128 {
		return false
	}
	for index, character := range name {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character == '_' || character == '-' || index > 0 && character >= '0' && character <= '9' {
			continue
		}
		return false
	}
	return true
}

func cloneExpansionScalar(source paperlang.Scalar) paperlang.Scalar {
	clone := source
	if source.StringValue != nil {
		value := *source.StringValue
		clone.StringValue = &value
	}
	if source.BoolValue != nil {
		value := *source.BoolValue
		clone.BoolValue = &value
	}
	if source.NumberValue != nil {
		value := *source.NumberValue
		clone.NumberValue = &value
	}
	if source.UnitValue != nil {
		value := *source.UnitValue
		clone.UnitValue = &value
	}
	return clone
}

func rootSpan(ast paperlang.AST) paperlang.Span {
	if ast.Root != nil {
		return ast.Root.HeaderSpan
	}
	return paperlang.Span{File: ast.File, Start: paperlang.Position{Line: 1, Column: 1}, End: paperlang.Position{Line: 1, Column: 1}}
}

func memberSpanForExpansion(member paperlang.Member) paperlang.Span {
	if member.Node != nil {
		return member.Node.Span
	}
	if member.Property != nil {
		return member.Property.Span
	}
	return paperlang.Span{}
}

func combineBindingPath(base, path string) string {
	path = strings.TrimSpace(path)
	if path == "" || strings.HasPrefix(path, "@") || base == "" {
		return path
	}
	return strings.TrimSuffix(base, ".") + "." + strings.TrimPrefix(path, ".")
}
