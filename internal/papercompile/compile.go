// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

// Package papercompile lowers the syntax-only paperlang AST into the shared
// renderer-independent layout model. It performs no measurement or painting.
package papercompile

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
	"github.com/cssbruno/gopdfkit/internal/paperlang"
	"github.com/cssbruno/gopdfkit/internal/paperscenario"
	"github.com/cssbruno/gopdfkit/internal/papertheme"
	"github.com/cssbruno/gopdfkit/layout"
)

// PageSpec is the point-based physical page selected during compilation.
// LayoutDocument currently owns page content and margins; the explicit page
// spec remains beside it until page size becomes part of that shared model.
type PageSpec struct {
	Width  float64 `json:"width_pt"`
	Height float64 `json:"height_pt"`
}

// NodeMapping preserves readable source IDs without leaking them into visual
// text. BodyIndex and SegmentIndex are -1 when the mapped node has no such
// target in LayoutDocument.
type NodeMapping struct {
	ID           string             `json:"id"`
	Kind         paperlang.NodeKind `json:"kind"`
	BodyIndex    int                `json:"body_index"`
	SegmentIndex int                `json:"segment_index"`
	// NestedBlockIndex distinguishes multiple visual text blocks inside one
	// list item. It is -1 for ordinary body mappings and the item itself.
	NestedBlockIndex int `json:"nested_block_index"`
	// Region distinguishes page-shell content from ordinary body content while
	// keeping BodyIndex local to the region's own block list.
	Region layoutengine.RegionID `json:"region,omitempty"`
	Span   paperlang.Span        `json:"span"`
	// DefinitionSpan and InvocationSpan are both populated for component
	// expansion so tools can navigate template and instance source separately.
	DefinitionSpan    paperlang.Span `json:"definition_span"`
	InvocationSpan    paperlang.Span `json:"invocation_span"`
	InstancePath      string         `json:"instance_path,omitempty"`
	BindingPath       string         `json:"binding_path,omitempty"`
	BindingSpan       paperlang.Span `json:"binding_span"`
	BindingNullable   bool           `json:"binding_nullable,omitempty"`
	BindingCollection bool           `json:"binding_collection,omitempty"`
	ResourceKind      string         `json:"resource_kind,omitempty"`
	ResourceDigest    string         `json:"resource_digest,omitempty"`
}

type CompileMapping struct {
	// SourceRevision is populated by the document adapter before planning. The
	// syntax compiler deliberately does not hash ambient source bytes itself.
	SourceRevision  string                 `json:"source_revision,omitempty"`
	Nodes           []NodeMapping          `json:"nodes,omitempty"`
	AnonymousNodes  []NodeMapping          `json:"anonymous_nodes,omitempty"`
	ThemeProperties []ThemePropertyMapping `json:"theme_properties,omitempty"`
	ComputedStyles  []ComputedStyleMapping `json:"computed_styles,omitempty"`
}

// ComputedStyleMapping retains the exact resolved style values attached to a
// readable source block. It is a detached compiler projection for inspectors;
// it does not expose scenario data or renderer-owned geometry.
type ComputedStyleMapping struct {
	NodeID    string             `json:"node_id,omitempty"`
	NodeKind  paperlang.NodeKind `json:"node_kind"`
	Source    paperlang.Span     `json:"source"`
	TextStyle *layout.TextStyle  `json:"text_style,omitempty"`
	BoxStyle  *layout.BoxStyle   `json:"box_style,omitempty"`
}

// ThemePropertyMapping preserves the exact resolved token chain behind one
// computed layout property, including consumers without readable node IDs.
type ThemePropertyMapping struct {
	NodeID       string                `json:"node_id,omitempty"`
	NodeKind     paperlang.NodeKind    `json:"node_kind"`
	Property     string                `json:"property"`
	ConsumerSpan paperlang.Span        `json:"consumer_span"`
	Theme        string                `json:"theme"`
	Token        string                `json:"token"`
	Value        papertheme.Value      `json:"value"`
	Provenance   papertheme.Provenance `json:"provenance"`
}

// Result is a deterministic compile projection. Document is nil when no
// document root could be lowered. Diagnostics use stable traversal order.
type Result struct {
	Document *layout.LayoutDocument
	Tree     layoutengine.CanonicalTree
	Page     PageSpec
	// Locale is the explicit resolved scenario locale. It is empty when no
	// scenario locale was declared; adapters must then select an explicit
	// locale rather than consulting the host environment.
	Locale string
	// ScenarioDigest identifies the exact normalized fixture selected for this
	// compile. It is empty for template-only compilation.
	ScenarioDigest string
	Mapping        CompileMapping
	Schemas        []SchemaDescriptor
	Diagnostics    []paperlang.Diagnostic
}

func (r Result) OK() bool {
	if r.Document == nil {
		return false
	}
	for _, diagnostic := range r.Diagnostics {
		if diagnostic.Severity == paperlang.SeverityError {
			return false
		}
	}
	return true
}

// Compile lowers a parsed AST without mutating the AST or consulting a PDF
// Document. All physical lengths in the result are expressed in PDF points.
func Compile(ast paperlang.AST) Result {
	return compileWithLimits(ast, ExpansionLimits{}, SchemaLimits{}, AssetCatalog{})
}

// CompileWithAssets resolves only explicit content-addressed `asset:name`
// references from the supplied immutable catalog. It never reads ambient
// files, URLs, environment variables, or process-global resource state.
func CompileWithAssets(ast paperlang.AST, assets AssetCatalog) Result {
	return compileWithLimits(ast, ExpansionLimits{}, SchemaLimits{}, assets)
}

// CompileWithResolver compiles a document whose root may declare one or more
// source-relative design imports. The resolver is the only authority allowed
// to provide imported source bytes.
func CompileWithResolver(ast paperlang.AST, resolver ImportResolver) Result {
	return compileWithLimitsAndResolver(ast, ExpansionLimits{}, SchemaLimits{}, AssetCatalog{}, resolver)
}

// CompileWithAssetsAndResolver combines explicit asset and source import
// boundaries.
func CompileWithAssetsAndResolver(ast paperlang.AST, assets AssetCatalog, resolver ImportResolver) Result {
	return compileWithLimitsAndResolver(ast, ExpansionLimits{}, SchemaLimits{}, assets, resolver)
}

// CompileWithExpansionLimits compiles with explicit bounded component
// expansion. A zero limits value selects conservative defaults.
func CompileWithExpansionLimits(ast paperlang.AST, limits ExpansionLimits) Result {
	return compileWithLimits(ast, limits, SchemaLimits{}, AssetCatalog{})
}

// CompileWithSchemaLimits compiles with explicit schema/path bounds.
func CompileWithSchemaLimits(ast paperlang.AST, limits SchemaLimits) Result {
	return compileWithLimits(ast, ExpansionLimits{}, limits, AssetCatalog{})
}

func compileWithLimits(ast paperlang.AST, expansionLimits ExpansionLimits, schemaLimits SchemaLimits, assets AssetCatalog) Result {
	return compileWithLimitsAndResolver(ast, expansionLimits, schemaLimits, assets, nil)
}

func compileWithLimitsAndResolver(ast paperlang.AST, expansionLimits ExpansionLimits, schemaLimits SchemaLimits, assets AssetCatalog, resolver ImportResolver) Result {
	return compilePipeline(ast, expansionLimits, schemaLimits, nil, assets, resolver)
}

func compilePipeline(ast paperlang.AST, expansionLimits ExpansionLimits, schemaLimits SchemaLimits, scenario *scenarioCompileRequest, assets AssetCatalog, resolver ImportResolver) Result {
	imports := resolveImports(ast, resolver, ImportLimits{})
	ast = imports.ast
	if scenario != nil && scenario.dataSet && strings.TrimSpace(scenario.dataOptions.Locale) == "" {
		scenario.dataOptions.Locale = paperDocumentLanguage(ast)
	}
	schemas := analyzeSchemas(ast, schemaLimits)
	themes := ExtractThemes(ast)
	selectedScenario := ""
	if scenario != nil {
		selectedScenario = scenario.name
	}
	expanded := expandComponents(ast, expansionLimits, selectedScenario)
	if scenario != nil {
		expanded = expandSelectedScenario(ast, schemas, expanded, scenario)
	} else {
		expanded = deferDynamicFlow(expanded)
	}
	bindings := validateBindings(expanded.ast, expanded.provenance, schemas, schemaLimits)
	c := compiler{
		result: Result{Document: layout.NewLayoutDocument(), Page: PageSpec{Width: 595.275590551, Height: 841.88976378}, Schemas: schemas.descriptors},
		ids:    make(map[string]paperlang.Span), provenance: expanded.provenance, bindings: bindings.metadata,
		themeInput: themes.Input, themeOutput: themes.Output, themeDiagnostics: themes.Diagnostics,
		assets: assets,
	}
	if scenario != nil {
		c.fixture = scenario.fixture
		if scenario.fixture != nil {
			c.result.Locale = scenario.fixture.Locale
			c.result.ScenarioDigest = scenario.fixture.Digest
		}
	}
	c.result.Diagnostics = append(c.result.Diagnostics, schemas.diagnostics...)
	c.result.Diagnostics = append(c.result.Diagnostics, themes.Diagnostics...)
	c.result.Diagnostics = append(c.result.Diagnostics, imports.diagnostics...)
	c.result.Diagnostics = append(c.result.Diagnostics, expanded.diagnostics...)
	c.result.Diagnostics = append(c.result.Diagnostics, bindings.diagnostics...)
	root := expanded.ast.Root
	if root == nil || root.Kind != paperlang.NodeDocument {
		span := paperlang.Span{File: expanded.ast.File, Start: paperlang.Position{Line: 1, Column: 1}, End: paperlang.Position{Line: 1, Column: 1}}
		if root != nil {
			span = root.HeaderSpan
		}
		c.add("PAPER_COMPILE_DOCUMENT", "compile input must have one document root", "parse a valid document node before compiling", span)
		c.result.Document = nil
		return c.result
	}
	c.collectStyleRules(ast.Root)
	c.mapNode(root, -1, -1)
	properties, children := c.members(root, documentProperties)
	c.compileDocumentProperties(properties)
	var page *paperlang.Node
	for _, child := range children {
		if child.Kind != paperlang.NodePage {
			c.unsupportedNode(child, "document supports only one page node")
			continue
		}
		if page != nil {
			c.add("PAPER_COMPILE_MULTIPLE_PAGES", "multiple page nodes cannot be represented by one layout document", "keep one page template in the initial compiler", child.HeaderSpan)
			continue
		}
		page = child
	}
	if page == nil {
		c.add("PAPER_COMPILE_PAGE", "document has no page node", "add one page containing one body", root.HeaderSpan)
		return c.result
	}
	c.compilePage(page)
	if tree, err := lowerCompileMappingTree(c.result.Mapping); err != nil {
		c.add("PAPER_COMPILE_TREE", "canonical private-tree lowering failed: "+err.Error(), "reduce the document or correct colliding readable IDs", root.HeaderSpan)
	} else {
		c.result.Tree = tree
	}
	return c.result
}

func paperDocumentLanguage(ast paperlang.AST) string {
	if ast.Root == nil {
		return ""
	}
	for _, member := range ast.Root.Members {
		property := member.Property
		if property != nil && property.Name == "language" && property.Value.Kind == paperlang.ScalarString && property.Value.StringValue != nil {
			return strings.TrimSpace(*property.Value.StringValue)
		}
	}
	return ""
}

// deferDynamicFlow gives the scenario-neutral compiler a stable empty-state
// projection. Repeat and loop nodes are validated and expanded by
// CompileScenario; without a selected fixture they contribute no flow, but do
// not make the authored source revision invalid or uneditable.
func deferDynamicFlow(input componentExpansionResult) componentExpansionResult {
	var prune func(*paperlang.Node)
	prune = func(node *paperlang.Node) {
		if node == nil {
			return
		}
		members := node.Members[:0]
		for _, member := range node.Members {
			if member.Node != nil && (member.Node.Kind == paperlang.NodeRepeat || member.Node.Kind == paperlang.NodeLoop) {
				input.diagnostics = append(input.diagnostics, paperlang.Diagnostic{Code: "PAPER_COMPILE_DYNAMIC_DEFERRED", Severity: paperlang.SeverityWarning, Message: fmt.Sprintf("%s is deferred until a scenario is selected", member.Node.Kind), Hint: "compile with one declared scenario to render dynamic content", Span: member.Node.HeaderSpan})
				continue
			}
			prune(member.Node)
			members = append(members, member)
		}
		node.Members = members
	}
	prune(input.ast.Root)
	return input
}

type compiler struct {
	result           Result
	ids              map[string]paperlang.Span
	provenance       map[*paperlang.Node]expansionProvenance
	bindings         map[*paperlang.Node]bindingMetadata
	fixture          *paperscenario.Fixture
	themeInput       papertheme.Input
	themeOutput      papertheme.Output
	themeDiagnostics []paperlang.Diagnostic
	selectedTheme    string
	themeSelected    bool
	assets           AssetCatalog
	styleRules       map[string]styleRule
}

var documentProperties = map[string]bool{"title": true, "language": true, "theme": true, "import": true}
var pageProperties = map[string]bool{
	"size": true, "width": true, "height": true, "margin": true,
	"margin-top": true, "margin-right": true, "margin-bottom": true, "margin-left": true,
}
var pageRegionProperties = func() map[string]bool {
	properties := map[string]bool{}
	for _, name := range boxPropertyNames {
		properties[name] = true
	}
	return properties
}()
var textStyleProperties = map[string]bool{
	"font": true, "size": true, "line-height": true, "color": true, "align": true, "bold": true, "italic": true,
	"font-token": true, "size-token": true, "line-height-token": true, "color-token": true,
}
var bindingTextProperties = func() map[string]bool {
	properties := copyPropertySet(textStyleProperties)
	for _, name := range []string{"bind", "bind-required", "format", "format-locale", "format-currency", "format-min-fraction", "format-max-fraction", "when"} {
		properties[name] = true
	}
	return properties
}()
var boxPropertyNames = []string{
	"margin", "margin-top", "margin-right", "margin-bottom", "margin-left",
	"padding", "padding-top", "padding-right", "padding-bottom", "padding-left",
	"border-width", "border-top-width", "border-right-width", "border-bottom-width", "border-left-width",
	"border-color", "border-radius", "background",
}
var boxedTextProperties = func() map[string]bool {
	properties := copyPropertySet(bindingTextProperties)
	properties["style"] = true
	for _, name := range boxPropertyNames {
		properties[name] = true
	}
	return properties
}()
var listProperties = func() map[string]bool {
	properties := copyPropertySet(textStyleProperties)
	properties["style"] = true
	properties["ordered"] = true
	properties["marker"] = true
	properties["when"] = true
	for _, name := range boxPropertyNames {
		properties[name] = true
	}
	return properties
}()
var rowColumnProperties = map[string]bool{"gap": true, "cross-align": true, "when": true}
var canvasProperties = map[string]bool{"width": true, "height": true, "default-horizontal": true, "default-vertical": true}
var canvasAnchorProperties = func() map[string]bool {
	properties := map[string]bool{"width": true, "height": true, "alt": true,
		"left": true, "right": true, "center-x": true, "top": true, "bottom": true, "center-y": true}
	for _, name := range boxPropertyNames {
		properties[name] = true
	}
	return properties
}()
var imageProperties = func() map[string]bool {
	properties := map[string]bool{
		"source": true, "width": true, "height": true, "max-width": true, "max-height": true,
		"fit": true, "focus-x": true, "focus-y": true, "align": true,
		"alt": true, "decorative": true, "caption": true, "when": true,
	}
	properties["style"] = true
	for _, name := range boxPropertyNames {
		properties[name] = true
	}
	return properties
}()
var tableProperties = map[string]bool{"caption": true, "repeat-header": true, "split": true, "when": true}
var tableTrackProperties = map[string]bool{"width": true, "min-width": true, "max-width": true}
var tableRowProperties = map[string]bool{"keep-together": true, "keep-with-next": true, "orphans": true, "widows": true}
var tableCellProperties = func() map[string]bool {
	properties := copyPropertySet(boxedTextProperties)
	for _, name := range []string{"header", "colspan", "rowspan", "vertical-align"} {
		properties[name] = true
	}
	return properties
}()
var rowColumnChildProperties = func() map[string]bool {
	properties := copyPropertySet(boxedTextProperties)
	for _, name := range []string{"level", "track", "track-size", "track-min", "track-max", "track-weight", "cross-align"} {
		properties[name] = true
	}
	return properties
}()

func (c *compiler) compileDocumentProperties(properties map[string]paperlang.Property) {
	if property, ok := properties["theme"]; ok {
		if value, valid := c.stringProperty(property); valid {
			c.selectedTheme = strings.TrimPrefix(strings.TrimSpace(value), "@")
			if c.theme(c.selectedTheme) == nil {
				c.add("PAPER_COMPILE_THEME_UNKNOWN", fmt.Sprintf("document selects unknown theme %q", value), "select a declared theme @name", property.Value.Span)
			} else {
				c.themeSelected = true
			}
		}
	}
	if property, ok := properties["title"]; ok {
		if value, valid := c.stringProperty(property); valid {
			c.result.Document.Title = value
		}
	}
	if property, ok := properties["language"]; ok {
		if value, valid := c.stringProperty(property); valid {
			c.result.Document.Language = value
		}
	}
}

func (c *compiler) compilePage(node *paperlang.Node) {
	c.mapNode(node, -1, -1)
	properties, children := c.members(node, pageProperties)
	if property, ok := properties["size"]; ok {
		if name, valid := c.stringProperty(property); valid {
			switch strings.ToLower(strings.TrimSpace(name)) {
			case "a4":
				c.result.Page = PageSpec{Width: 595.275590551, Height: 841.88976378}
			case "a3":
				c.result.Page = PageSpec{Width: 841.88976378, Height: 1190.551181102}
			case "letter":
				c.result.Page = PageSpec{Width: 612, Height: 792}
			case "legal":
				c.result.Page = PageSpec{Width: 612, Height: 1008}
			default:
				c.add("PAPER_COMPILE_PAGE_SIZE", fmt.Sprintf("unsupported page size %q", name), "use A3, A4, Letter, Legal, or explicit width and height", property.Value.Span)
			}
		}
	}
	if property, ok := properties["width"]; ok {
		if value, valid := c.lengthProperty(property, true); valid {
			c.result.Page.Width = value
		}
	}
	if property, ok := properties["height"]; ok {
		if value, valid := c.lengthProperty(property, true); valid {
			c.result.Page.Height = value
		}
	}
	if c.result.Page.Width <= 0 || c.result.Page.Height <= 0 {
		c.add("PAPER_COMPILE_PAGE_EXTENT", "page width and height must be positive", "use positive physical lengths", node.HeaderSpan)
	}

	var margins layout.Spacing
	if property, ok := properties["margin"]; ok {
		if value, valid := c.lengthProperty(property, false); valid {
			margins = layout.Spacing{Top: value, Right: value, Bottom: value, Left: value}
		}
	}
	for _, side := range []struct {
		name string
		set  func(float64)
	}{
		{"margin-top", func(value float64) { margins.Top = value }},
		{"margin-right", func(value float64) { margins.Right = value }},
		{"margin-bottom", func(value float64) { margins.Bottom = value }},
		{"margin-left", func(value float64) { margins.Left = value }},
	} {
		if property, ok := properties[side.name]; ok {
			if value, valid := c.lengthProperty(property, false); valid {
				side.set(value)
			}
		}
	}
	c.result.Document.PageTemplate.Margins = margins

	var body, header, footer *paperlang.Node
	for _, child := range children {
		switch child.Kind {
		case paperlang.NodeBody:
			if body != nil {
				c.add("PAPER_COMPILE_MULTIPLE_BODIES", "multiple body nodes cannot be represented by one layout document", "merge content into one body", child.HeaderSpan)
			} else {
				body = child
			}
		case paperlang.NodeHeader:
			if header != nil {
				c.add("PAPER_COMPILE_MULTIPLE_HEADERS", "page declares more than one default header region", "merge header content", child.HeaderSpan)
			} else {
				header = child
			}
		case paperlang.NodeFooter:
			if footer != nil {
				c.add("PAPER_COMPILE_MULTIPLE_FOOTERS", "page declares more than one default footer region", "merge footer content", child.HeaderSpan)
			} else {
				footer = child
			}
		default:
			c.unsupportedNode(child, "page supports one body and optional header/footer regions")
		}
	}
	if body == nil {
		c.add("PAPER_COMPILE_BODY", "page has no body node", "add one body containing text blocks", node.HeaderSpan)
		return
	}
	if header != nil {
		blocks, _, box := c.compilePageRegion(header)
		c.result.Document.PageTemplate.Header = &layout.HeaderBlock{Blocks: blocks, Box: box}
	}
	if footer != nil {
		blocks, reserve, box := c.compilePageRegion(footer)
		c.result.Document.PageTemplate.Footer = &layout.FooterBlock{Blocks: blocks, ReservePageArea: reserve, Box: box}
	}
	c.compileBody(body)
}

func (c *compiler) compilePageRegion(node *paperlang.Node) ([]layout.Block, bool, layout.BoxStyle) {
	properties, children := c.members(node, pageRegionProperties)
	reserve := true
	c.mapNode(node, -1, -1)
	original := c.result.Document.Body
	mappingStart := len(c.result.Mapping.Nodes)
	c.result.Document.Body = nil
	c.compileFlowChildren(children, "page region supports ordinary body content")
	blocks := append([]layout.Block(nil), c.result.Document.Body...)
	c.result.Document.Body = original
	region := layoutengine.RegionHeader
	if node.Kind == paperlang.NodeFooter {
		region = layoutengine.RegionFooter
	}
	for index := mappingStart; index < len(c.result.Mapping.Nodes); index++ {
		c.result.Mapping.Nodes[index].Region = region
	}
	if len(blocks) == 0 {
		c.add("PAPER_COMPILE_EMPTY_REGION", "page region has no supported content", "add a paragraph, heading, list, image, row, or table", node.HeaderSpan)
	}
	return blocks, reserve, c.compileBoxStyle(properties)
}

func (c *compiler) compileBody(node *paperlang.Node) {
	c.mapNode(node, -1, -1)
	_, children := c.members(node, map[string]bool{})
	c.compileFlowChildren(children, "body supports row, column, image, heading, paragraph, list, page-break, and text")
}

func (c *compiler) compileFlowChildren(children []*paperlang.Node, hint string) {
	for _, child := range children {
		switch child.Kind {
		case paperlang.NodeHeading:
			c.compileHeading(child)
		case paperlang.NodeParagraph:
			c.compileParagraph(child)
		case paperlang.NodeList:
			c.compileList(child)
		case paperlang.NodeText:
			c.compileBodyText(child)
		case paperlang.NodePageBreak:
			c.compilePageBreak(child)
		case paperlang.NodeRow, paperlang.NodeColumn:
			c.compileRowColumn(child)
		case paperlang.NodeImage:
			c.compileImage(child)
		case paperlang.NodeTable:
			c.compileTable(child)
		case paperlang.NodeCanvas:
			c.compileCanvas(child)
		case paperlang.NodeRepeat, paperlang.NodeLoop:
			// Dynamic authoring remains a valid base document even when no
			// scenario is selected. Scenario compilation expands these nodes
			// before flow lowering; the neutral base projection intentionally
			// omits them instead of making the source revision uneditable.
			c.mapNode(child, -1, -1)
			c.warn("PAPER_COMPILE_DYNAMIC_DEFERRED", fmt.Sprintf("%s is deferred until a scenario is selected", child.Kind), "compile with one declared scenario to render dynamic content", child.HeaderSpan)
		default:
			c.unsupportedNode(child, hint)
		}
	}
}

func (c *compiler) compileCanvas(node *paperlang.Node) {
	properties, children := c.members(node, canvasProperties)
	block := layout.CanvasBlock{DefaultHorizontal: "left", DefaultVertical: "top"}
	for _, dimension := range []struct {
		name string
		set  func(float64)
	}{{"width", func(value float64) { block.Width = value }}, {"height", func(value float64) { block.Height = value }}} {
		if property, ok := properties[dimension.name]; ok {
			if value, valid := c.lengthProperty(property, true); valid {
				dimension.set(value)
			}
		} else {
			c.add("PAPER_COMPILE_CANVAS_SIZE", "canvas requires explicit width and height", "add positive width and height properties", node.HeaderSpan)
		}
	}
	for _, defaultProperty := range []struct {
		name  string
		set   func(string)
		valid map[string]bool
	}{{"default-horizontal", func(value string) { block.DefaultHorizontal = value }, map[string]bool{"left": true, "right": true, "center-x": true}},
		{"default-vertical", func(value string) { block.DefaultVertical = value }, map[string]bool{"top": true, "bottom": true, "center-y": true}}} {
		if property, ok := properties[defaultProperty.name]; ok {
			if value, valid := c.stringProperty(property); valid {
				value = strings.ToLower(strings.TrimSpace(value))
				if !defaultProperty.valid[value] {
					c.add("PAPER_COMPILE_CANVAS_DEFAULT", fmt.Sprintf("unsupported canvas default %q", value), "use a same-axis edge or center anchor", property.Value.Span)
				} else {
					defaultProperty.set(value)
				}
			}
		}
	}
	bodyIndex := len(c.result.Document.Body)
	c.mapNode(node, bodyIndex, -1)
	seen := map[string]bool{}
	for _, child := range children {
		if child.Kind != paperlang.NodeAnchor {
			c.unsupportedNode(child, "canvas accepts explicit anchor children")
			continue
		}
		item := c.compileCanvasAnchor(child)
		if item.ID == "" || seen[item.ID] {
			c.add("PAPER_COMPILE_CANVAS_ID", "canvas anchors require unique explicit IDs", "name each anchor once, for example anchor @label", child.HeaderSpan)
		} else {
			seen[item.ID] = true
		}
		itemIndex := len(block.Items)
		block.Items = append(block.Items, item)
		c.mapNestedNode(child, bodyIndex, itemIndex, -1)
	}
	if len(block.Items) == 0 {
		c.add("PAPER_COMPILE_CANVAS_EMPTY", "canvas requires at least one anchor", "add an explicitly sized anchor child", node.HeaderSpan)
	}
	c.result.Document.Body = append(c.result.Document.Body, block)
	c.recordComputedStyle(node, bodyIndex)
}

func (c *compiler) compileCanvasAnchor(node *paperlang.Node) layout.CanvasItem {
	properties, children := c.members(node, canvasAnchorProperties)
	for _, child := range children {
		c.unsupportedNode(child, "anchor is a property-only canvas node")
	}
	item := layout.CanvasItem{ID: node.ID, Box: c.compileBoxStyle(properties)}
	for _, dimension := range []struct {
		name string
		set  func(float64)
	}{{"width", func(value float64) { item.Width = value }}, {"height", func(value float64) { item.Height = value }}} {
		if property, ok := properties[dimension.name]; ok {
			if value, valid := c.lengthProperty(property, true); valid {
				dimension.set(value)
			}
		} else {
			c.add("PAPER_COMPILE_CANVAS_ITEM_SIZE", "canvas anchor requires explicit width and height", "add positive width and height properties", node.HeaderSpan)
		}
	}
	if property, ok := properties["alt"]; ok {
		item.Alt, _ = c.stringProperty(property)
	}
	for _, anchor := range []string{"left", "right", "center-x", "top", "bottom", "center-y"} {
		if property, ok := properties[anchor]; ok {
			if expression, valid := c.stringProperty(property); valid {
				constraint, problem := parseCanvasConstraint(anchor, expression)
				if problem != "" {
					c.add("PAPER_COMPILE_CANVAS_CONSTRAINT", problem, "use \"canvas.left\", \"@peer.right + 8pt\", or another same-axis anchor", property.Value.Span)
				} else {
					item.Constraints = append(item.Constraints, constraint)
				}
			}
		}
	}
	return item
}

func parseCanvasConstraint(anchor, expression string) (layout.CanvasConstraint, string) {
	fields := strings.Fields(strings.TrimSpace(expression))
	if len(fields) != 1 && len(fields) != 3 {
		return layout.CanvasConstraint{}, "canvas constraint must be a target anchor with an optional signed physical offset"
	}
	target := strings.Split(fields[0], ".")
	if len(target) != 2 || target[0] == "" || target[1] == "" || target[0] != "canvas" && !strings.HasPrefix(target[0], "@") {
		return layout.CanvasConstraint{}, "canvas constraint target must be canvas.anchor or @sibling.anchor"
	}
	constraint := layout.CanvasConstraint{Anchor: anchor, Target: target[0], TargetAnchor: target[1]}
	if len(fields) == 3 {
		if fields[1] != "+" && fields[1] != "-" {
			return layout.CanvasConstraint{}, "canvas constraint offset operator must be + or -"
		}
		value, ok := parseCanvasOffset(fields[2])
		if !ok {
			return layout.CanvasConstraint{}, "canvas constraint offset must use pt, mm, cm, in, px, or pc"
		}
		if fields[1] == "-" {
			value = -value
		}
		constraint.Offset = value
	}
	return constraint, ""
}

func parseCanvasOffset(source string) (float64, bool) {
	for _, unit := range []string{"pt", "mm", "cm", "in", "px", "pc"} {
		if !strings.HasSuffix(source, unit) {
			continue
		}
		value, err := strconv.ParseFloat(strings.TrimSuffix(source, unit), 64)
		if err != nil || !isFinite(value) || value < 0 {
			return 0, false
		}
		switch unit {
		case "mm":
			value *= 72 / 25.4
		case "cm":
			value *= 72 / 2.54
		case "in":
			value *= 72
		case "px":
			value *= 72.0 / 96.0
		case "pc":
			value *= 12
		}
		return value, true
	}
	return 0, false
}

func (c *compiler) compileTable(node *paperlang.Node) {
	properties, children := c.members(node, tableProperties)
	table := layout.TableBlock{}
	if property, ok := properties["caption"]; ok {
		if value, valid := c.stringProperty(property); valid {
			table.Caption = value
		}
	}
	if property, ok := properties["repeat-header"]; ok {
		if value, valid := c.boolProperty(property); valid {
			table.Style.RepeatHeader = value
		}
	}
	if property, ok := properties["split"]; ok {
		if value, valid := c.stringProperty(property); valid {
			switch strings.ToLower(strings.TrimSpace(value)) {
			case "rows":
				table.Style.KeepRows = false
			case "avoid":
				table.Style.KeepRows = true
			default:
				c.add("PAPER_COMPILE_TABLE_SPLIT", "table split must be rows or avoid", "use split: \"rows\"", property.Value.Span)
			}
		}
	}
	rowCount, cellCount := 0, 0
	compileRow := func(rowNode *paperlang.Node, header bool) layout.TableRow {
		rowProperties, cells := c.members(rowNode, tableRowProperties)
		row := layout.TableRow{}
		if property, ok := rowProperties["keep-together"]; ok {
			row.KeepTogether, _ = c.boolProperty(property)
		}
		if property, ok := rowProperties["keep-with-next"]; ok {
			row.KeepWithNext, _ = c.boolProperty(property)
		}
		for _, cellNode := range cells {
			if cellNode.Kind != paperlang.NodeTableCell {
				c.unsupportedNode(cellNode, "table-row accepts only cell children")
				continue
			}
			cellCount++
			cellProperties, content := c.members(cellNode, tableCellProperties)
			cell := layout.TableCell{Header: header, ColSpan: 1, RowSpan: 1, Style: c.compileTextStyle(cellNode, cellProperties), Box: c.compileBoxStyle(cellProperties)}
			if property, ok := cellProperties["header"]; ok {
				cell.Header, _ = c.boolProperty(property)
			}
			for _, span := range []struct {
				name string
				set  func(int)
			}{{"colspan", func(v int) { cell.ColSpan = v }}, {"rowspan", func(v int) { cell.RowSpan = v }}} {
				if property, ok := cellProperties[span.name]; ok {
					if value, valid := c.numberProperty(property); valid && value >= 1 && value <= 1024 && math.Trunc(value) == value {
						span.set(int(value))
					} else if valid {
						c.add("PAPER_COMPILE_TABLE_SPAN", span.name+" must be an integer from 1 through 1024", "use a bounded positive integer", property.Value.Span)
					}
				}
			}
			for _, child := range content {
				switch child.Kind {
				case paperlang.NodeText:
					text := ""
					if child.Value != nil && child.Value.StringValue != nil {
						text = *child.Value.StringValue
					}
					cell.Blocks = append(cell.Blocks, layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: text}}})
				case paperlang.NodeParagraph:
					p, textNodes := c.members(child, boxedTextProperties)
					segments, _ := c.compileTextChildren(child, textNodes)
					cell.Blocks = append(cell.Blocks, layout.ParagraphBlock{Segments: segments, Style: c.compileTextStyle(child, p), Box: c.compileBoxStyle(p)})
				case paperlang.NodeList:
					listProperties, items := c.members(child, listProperties)
					list := layout.ListBlock{Style: c.compileTextStyle(child, listProperties), Box: c.compileBoxStyle(listProperties)}
					if property, ok := listProperties["ordered"]; ok {
						list.Ordered, _ = c.boolProperty(property)
					}
					for _, itemNode := range items {
						_, itemChildren := c.members(itemNode, map[string]bool{})
						item := layout.ListItem{}
						for _, itemChild := range itemChildren {
							if itemChild.Kind == paperlang.NodeText && itemChild.Value != nil && itemChild.Value.StringValue != nil {
								item.Blocks = append(item.Blocks, layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: *itemChild.Value.StringValue}}})
							}
						}
						if len(item.Blocks) > 0 {
							list.Items = append(list.Items, item)
						}
					}
					if len(list.Items) > 0 {
						cell.Blocks = append(cell.Blocks, list)
					}
				case paperlang.NodeImage:
					p, _ := c.members(child, imageProperties)
					image := layout.ImageBlock{}
					if property, ok := p["source"]; ok {
						if source, valid := c.stringProperty(property); valid {
							data, format, problem := c.decodeImageSource(source)
							if problem != "" {
								c.add("PAPER_COMPILE_IMAGE_SOURCE", problem, "use asset:name from an explicit catalog or a bounded PNG/JPEG data resource", property.Value.Span)
							} else {
								image.Data, image.Format = data, format
							}
						}
					}
					if property, ok := p["alt"]; ok {
						image.Alt, _ = c.stringProperty(property)
					}
					if property, ok := p["decorative"]; ok {
						image.Decorative, _ = c.boolProperty(property)
					}
					c.compileImageDimensions(p, &image)
					if property, ok := p["fit"]; ok {
						if value, valid := c.stringProperty(property); valid {
							switch strings.ToLower(strings.TrimSpace(value)) {
							case "auto":
								image.Fit = layout.ImageFitAuto
							case "contain":
								image.Fit = layout.ImageFitContain
							case "cover":
								image.Fit = layout.ImageFitCover
							default:
								c.add("PAPER_COMPILE_IMAGE_FIT", "nested image fit is unsupported", "use auto, contain, or cover", property.Value.Span)
							}
						}
					}
					if len(image.Data) > 0 && (image.Decorative || strings.TrimSpace(image.Alt) != "") {
						cell.Blocks = append(cell.Blocks, image)
					}
				default:
					c.unsupportedNode(child, "table cell supports text, paragraph, list, and image content")
				}
			}
			if len(cell.Blocks) == 0 {
				c.add("PAPER_COMPILE_TABLE_CELL", "table cell has no supported content", "add text or paragraph content", cellNode.HeaderSpan)
			}
			row.Cells = append(row.Cells, cell)
		}
		rowCount++
		return row
	}
	for _, child := range children {
		switch child.Kind {
		case paperlang.NodeTableTrack:
			p, _ := c.members(child, tableTrackProperties)
			column := layout.TableColumn{}
			for _, name := range []string{"width", "min-width", "max-width"} {
				property, ok := p[name]
				if !ok {
					continue
				}
				value, valid := c.contextualLengthProperty(property, false, true)
				if !valid || value.auto {
					continue
				}
				switch name {
				case "width":
					column.Width, column.WidthPercent = value.points, value.percent
				case "min-width":
					column.MinWidth, column.MinWidthPercent = value.points, value.percent
				case "max-width":
					column.MaxWidth, column.MaxWidthPercent = value.points, value.percent
				}
			}
			table.Columns = append(table.Columns, column)
		case paperlang.NodeTableRow:
			table.Body = append(table.Body, compileRow(child, false))
		case paperlang.NodeTableHeader:
			_, rows := c.members(child, map[string]bool{})
			for _, row := range rows {
				table.Header = append(table.Header, compileRow(row, true))
			}
		}
	}
	if rowCount == 0 || rowCount > 10000 || cellCount > 100000 {
		c.add("PAPER_COMPILE_TABLE_LIMIT", "table rows or cells are empty or exceed bounded limits", "use 1..10000 rows and at most 100000 cells", node.HeaderSpan)
	}
	index := len(c.result.Document.Body)
	c.result.Document.Body = append(c.result.Document.Body, table)
	c.mapNode(node, index, -1)
}

const maxPaperInlineImageBytes = 512 << 10

func (c *compiler) compileImage(node *paperlang.Node) {
	properties, children := c.members(node, imageProperties)
	for _, child := range children {
		c.unsupportedNode(child, "image is a property-only figure node")
	}
	block := layout.ImageBlock{Box: c.compileBoxStyle(properties)}
	if property, ok := properties["source"]; ok {
		if value, valid := c.stringProperty(property); valid {
			data, format, problem := c.decodeImageSource(value)
			if problem != "" {
				c.add("PAPER_COMPILE_IMAGE_SOURCE", problem, "use asset:name from an explicit catalog or a bounded PNG/JPEG data resource", property.Value.Span)
			} else {
				block.Data, block.Format = data, format
			}
		}
	} else {
		c.add("PAPER_COMPILE_IMAGE_SOURCE", "image requires an explicit source resource", "add source: \"asset:name\" or a bounded PNG/JPEG data resource", node.HeaderSpan)
	}
	c.compileImageDimensions(properties, &block)
	if property, ok := properties["fit"]; ok {
		if value, valid := c.stringProperty(property); valid {
			switch strings.ToLower(strings.TrimSpace(value)) {
			case "auto":
				block.Fit = layout.ImageFitAuto
			case "contain":
				block.Fit = layout.ImageFitContain
			case "cover":
				block.Fit = layout.ImageFitCover
			default:
				c.add("PAPER_COMPILE_IMAGE_FIT", fmt.Sprintf("image fit %q is unsupported", value), "use auto, contain, or cover", property.Value.Span)
			}
		}
	}
	block.FocusX, block.FocusY = 0.5, 0.5
	for _, focus := range []struct {
		name string
		set  func(float64)
	}{{"focus-x", func(value float64) { block.FocusX = value }}, {"focus-y", func(value float64) { block.FocusY = value }}} {
		if property, ok := properties[focus.name]; ok {
			block.FocusSet = true
			if value, valid := c.numberProperty(property); valid {
				if value < 0 || value > 1 {
					c.add("PAPER_COMPILE_IMAGE_FOCUS", focus.name+" must be between 0 and 1", "use a decimal such as 0.5", property.Value.Span)
				} else {
					focus.set(value)
				}
			}
		}
	}
	if property, ok := properties["align"]; ok {
		if value, valid := c.stringProperty(property); valid {
			switch strings.ToLower(strings.TrimSpace(value)) {
			case "left", "center", "right":
				block.Align = strings.ToLower(strings.TrimSpace(value))
			default:
				c.add("PAPER_COMPILE_IMAGE_ALIGN", fmt.Sprintf("image alignment %q is unsupported", value), "use left, center, or right", property.Value.Span)
			}
		}
	}
	if property, ok := properties["alt"]; ok {
		if value, valid := c.stringProperty(property); valid {
			block.Alt = value
		}
	}
	if property, ok := properties["decorative"]; ok {
		if value, valid := c.boolProperty(property); valid {
			block.Decorative = value
		}
	}
	if block.Decorative && strings.TrimSpace(block.Alt) != "" {
		c.add("PAPER_COMPILE_IMAGE_ACCESSIBILITY", "decorative image cannot also expose alternative text", "remove alt or set decorative: false", node.HeaderSpan)
	} else if !block.Decorative && strings.TrimSpace(block.Alt) == "" {
		c.add("PAPER_COMPILE_IMAGE_ACCESSIBILITY", "meaningful image requires non-empty alternative text", "add alt text or set decorative: true", node.HeaderSpan)
	}
	if property, ok := properties["caption"]; ok {
		if value, valid := c.stringProperty(property); valid && strings.TrimSpace(value) != "" {
			block.Caption = []layout.TextSegment{{Text: value}}
		}
	}
	blockIndex := len(c.result.Document.Body)
	c.result.Document.Body = append(c.result.Document.Body, block)
	c.mapNode(node, blockIndex, -1)
	if len(block.Data) != 0 && len(c.result.Mapping.Nodes) != 0 {
		digest := sha256.Sum256(block.Data)
		mapped := &c.result.Mapping.Nodes[len(c.result.Mapping.Nodes)-1]
		mapped.ResourceKind = "image/" + block.Format
		mapped.ResourceDigest = hex.EncodeToString(digest[:])
	}
}

func (c *compiler) compileImageDimensions(properties map[string]paperlang.Property, block *layout.ImageBlock) {
	for _, name := range []string{"width", "height", "max-width", "max-height"} {
		property, ok := properties[name]
		if !ok {
			continue
		}
		value, valid := c.contextualLengthProperty(property, false, true)
		if !valid || value.auto {
			continue
		}
		if value.percentSet {
			switch name {
			case "width":
				block.WidthPercent = value.percent
			case "max-width":
				block.MaxWidthPercent = value.percent
			default:
				c.add("PAPER_COMPILE_PERCENT_AXIS", fmt.Sprintf("image %s cannot use a percentage because flow height is automatic", name), "use \"auto\" or a physical height; width and max-width accept percentages", property.Value.Span)
			}
			continue
		}
		switch name {
		case "width":
			block.Width = value.points
		case "height":
			block.Height = value.points
		case "max-width":
			block.MaxWidth = value.points
		case "max-height":
			block.MaxHeight = value.points
		}
	}
}

func decodePaperImageSource(source string) ([]byte, string, string) {
	var format, encoded string
	switch {
	case strings.HasPrefix(source, "data:image/png;base64,"):
		format, encoded = "png", strings.TrimPrefix(source, "data:image/png;base64,")
	case strings.HasPrefix(source, "data:image/jpeg;base64,"):
		format, encoded = "jpg", strings.TrimPrefix(source, "data:image/jpeg;base64,")
	default:
		return nil, "", "image source must be an exact PNG or JPEG base64 data resource"
	}
	if encoded == "" || base64.StdEncoding.DecodedLen(len(encoded)) > maxPaperInlineImageBytes {
		return nil, "", fmt.Sprintf("decoded image resource exceeds the %d-byte limit", maxPaperInlineImageBytes)
	}
	data, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil || len(data) == 0 {
		return nil, "", "image source contains invalid canonical base64"
	}
	if format == "png" && (len(data) < 8 || string(data[:8]) != "\x89PNG\r\n\x1a\n") {
		return nil, "", "PNG image source has an invalid signature"
	}
	if format == "jpg" && (len(data) < 3 || data[0] != 0xff || data[1] != 0xd8 || data[2] != 0xff) {
		return nil, "", "JPEG image source has an invalid signature"
	}
	return data, format, ""
}

func (c *compiler) decodeImageSource(source string) ([]byte, string, string) {
	if !strings.HasPrefix(source, "asset:") {
		return decodePaperImageSource(source)
	}
	name := strings.TrimPrefix(source, "asset:")
	if !validAssetName(name) || source != "asset:"+name {
		return nil, "", "image asset reference must be exact asset:name syntax"
	}
	resource, ok := c.assets.Resolve(name)
	if !ok {
		return nil, "", fmt.Sprintf("image asset %q is not present in the explicit content-addressed catalog", name)
	}
	format := "png"
	if resource.MediaType == "image/jpeg" {
		format = "jpeg"
	}
	return resource.Data, format, ""
}

func (c *compiler) compileRowColumn(node *paperlang.Node) {
	properties, children := c.members(node, rowColumnProperties)
	direction := layout.RowDirection
	if node.Kind == paperlang.NodeColumn {
		direction = layout.ColumnDirection
	}
	block := layout.RowColumnBlock{Direction: direction, CrossAlign: "stretch"}
	if property, ok := properties["gap"]; ok {
		if value, valid := c.lengthProperty(property, false); valid {
			block.Gap = value
		}
	}
	if property, ok := properties["cross-align"]; ok {
		if value, valid := c.stringProperty(property); valid {
			if align, supported := canonicalCrossAlign(value); supported {
				block.CrossAlign = align
			} else {
				c.add("PAPER_COMPILE_CROSS_ALIGN", fmt.Sprintf("cross alignment %q is unsupported", value), "use start, center, end, or stretch", property.Value.Span)
			}
		}
	}
	bodyIndex := len(c.result.Document.Body)
	c.mapNode(node, bodyIndex, -1)
	for _, child := range children {
		if child.Kind != paperlang.NodeParagraph && child.Kind != paperlang.NodeHeading {
			c.unsupportedNode(child, "row and column initially support paragraph and heading children")
			continue
		}
		itemIndex := len(block.Items)
		item, textNodes := c.compileRowColumnItem(child)
		block.Items = append(block.Items, item)
		c.mapNestedNode(child, bodyIndex, itemIndex, -1)
		for textIndex, textNode := range textNodes {
			c.mapNestedNode(textNode, bodyIndex, itemIndex, textIndex)
		}
	}
	if len(block.Items) == 0 {
		c.add("PAPER_COMPILE_ROW_COLUMN_ITEMS", fmt.Sprintf("%s has no compilable children", node.Kind), "add one or more paragraph or heading children", node.HeaderSpan)
	}
	c.result.Document.Body = append(c.result.Document.Body, block)
	c.recordComputedStyle(node, bodyIndex)
}

func (c *compiler) compileRowColumnItem(node *paperlang.Node) (layout.RowColumnItem, []*paperlang.Node) {
	properties, children := c.members(node, rowColumnChildProperties)
	style := c.compileTextStyle(node, properties)
	segments, textNodes := c.compileTextChildren(node, children)
	var child layout.Block
	if node.Kind == paperlang.NodeHeading {
		level := 1
		if property, ok := properties["level"]; ok {
			if value, valid := c.numberProperty(property); valid && value >= 1 && value <= 6 && math.Trunc(value) == value {
				level = int(value)
			} else if valid {
				c.add("PAPER_COMPILE_HEADING_LEVEL", "heading level must be an integer from 1 through 6", "use level: 1 through level: 6", property.Value.Span)
			}
		}
		child = layout.HeadingBlock{Level: level, Segments: segments, Style: style, Box: c.compileBoxStyle(properties)}
	} else {
		child = layout.ParagraphBlock{Segments: segments, Style: style, Box: c.compileBoxStyle(properties)}
	}
	item := layout.RowColumnItem{Block: child, Track: layout.RowColumnTrack{Kind: layout.RowColumnTrackAuto}}
	trackExplicit := false
	if property, ok := properties["track"]; ok {
		if value, valid := c.stringProperty(property); valid {
			trackExplicit = true
			switch strings.ToLower(strings.TrimSpace(value)) {
			case "fixed":
				item.Track.Kind = layout.RowColumnTrackFixed
			case "auto":
				item.Track.Kind = layout.RowColumnTrackAuto
			case "fraction", "fr":
				item.Track.Kind = layout.RowColumnTrackFraction
			case "flex":
				item.Track.Kind = layout.RowColumnTrackFlex
				item.Track.BasisKind = layout.RowColumnFlexBasisContent
				item.Track.Shrink = 1
			default:
				c.add("PAPER_COMPILE_TRACK", fmt.Sprintf("track kind %q is unsupported", value), "use fixed, auto, fraction, or flex", property.Value.Span)
			}
		}
	}
	if property, ok := properties["track-size"]; ok {
		if value, valid := c.contextualLengthProperty(property, true, true); valid {
			switch {
			case value.auto:
				if trackExplicit && item.Track.Kind != layout.RowColumnTrackAuto && item.Track.Kind != layout.RowColumnTrackFlex {
					c.warn("PAPER_COMPILE_TRACK_CONTEXT", "automatic track-size promotes the selected track to flexible sizing", "use track: \"flex\" or remove the track property for an explicit contract", property.Value.Span)
				}
				item.Track = layout.RowColumnTrack{Kind: layout.RowColumnTrackFlex, BasisKind: layout.RowColumnFlexBasisContent, Shrink: 1}
			case value.percentSet:
				if trackExplicit && item.Track.Kind != layout.RowColumnTrackAuto && item.Track.Kind != layout.RowColumnTrackFlex {
					c.warn("PAPER_COMPILE_TRACK_CONTEXT", "percentage track-size promotes the selected track to flexible sizing", "use track: \"flex\" or remove the track property for an explicit contract", property.Value.Span)
				}
				item.Track = layout.RowColumnTrack{Kind: layout.RowColumnTrackFlex, BasisKind: layout.RowColumnFlexBasisPercent, BasisPercent: value.percent, Shrink: 1}
			case item.Track.Kind == layout.RowColumnTrackFlex:
				item.Track.BasisKind, item.Track.Basis = layout.RowColumnFlexBasisFixed, value.points
			case !trackExplicit:
				item.Track.Kind, item.Track.Size = layout.RowColumnTrackFixed, value.points
			default:
				item.Track.Size = value.points
			}
		}
	}
	if property, ok := properties["track-min"]; ok {
		if value, valid := c.contextualLengthProperty(property, false, true); valid && !value.auto {
			if value.percentSet {
				if item.Track.Kind != layout.RowColumnTrackAuto && item.Track.Kind != layout.RowColumnTrackFlex {
					c.add("PAPER_COMPILE_TRACK_CONTEXT", "percentage track-min requires a flexible track", "use track: \"flex\" or remove the fixed/fraction track property", property.Value.Span)
				}
				if item.Track.Kind != layout.RowColumnTrackFlex {
					item.Track = layout.RowColumnTrack{Kind: layout.RowColumnTrackFlex, BasisKind: layout.RowColumnFlexBasisContent, Shrink: 1}
				}
				item.Track.MinPercent = value.percent
			} else {
				item.Track.Min = value.points
			}
		}
	}
	if property, ok := properties["track-max"]; ok {
		if value, valid := c.contextualLengthProperty(property, false, true); valid && !value.auto {
			if item.Track.Kind != layout.RowColumnTrackFlex {
				if item.Track.Kind != layout.RowColumnTrackAuto {
					c.add("PAPER_COMPILE_TRACK_CONTEXT", "track-max requires a flexible track", "use track: \"flex\" or remove the fixed/fraction track property", property.Value.Span)
				}
				item.Track = layout.RowColumnTrack{Kind: layout.RowColumnTrackFlex, BasisKind: layout.RowColumnFlexBasisContent, Shrink: 1}
			}
			if value.percentSet {
				item.Track.MaxPercent = value.percent
			} else {
				item.Track.Max = value.points
			}
		}
	}
	if property, ok := properties["track-weight"]; ok {
		if value, valid := c.numberProperty(property); valid {
			if value <= 0 || value > math.MaxUint32 || math.Trunc(value) != value {
				c.add("PAPER_COMPILE_TRACK_WEIGHT", "track-weight must be a positive 32-bit integer", "use an integer such as 1 or 2", property.Value.Span)
			} else {
				item.Track.Weight = uint32(value)
			}
		}
	}
	if property, ok := properties["cross-align"]; ok {
		if value, valid := c.stringProperty(property); valid {
			if align, supported := canonicalCrossAlign(value); supported {
				item.CrossAlign = align
			} else {
				c.add("PAPER_COMPILE_CROSS_ALIGN", fmt.Sprintf("cross alignment %q is unsupported", value), "use start, center, end, or stretch", property.Value.Span)
			}
		}
	}
	switch item.Track.Kind {
	case layout.RowColumnTrackFixed:
		if item.Track.Size <= 0 {
			c.add("PAPER_COMPILE_TRACK_SIZE", "fixed track requires a positive track-size", "add track-size: 120pt", node.HeaderSpan)
		}
		if item.Track.Weight != 0 {
			c.add("PAPER_COMPILE_TRACK_WEIGHT", "fixed track cannot have track-weight", "remove track-weight", node.HeaderSpan)
		}
	case layout.RowColumnTrackAuto:
		if item.Track.Size != 0 || item.Track.Weight != 0 {
			c.add("PAPER_COMPILE_TRACK_AUTO", "auto track cannot have track-size or track-weight", "remove the fixed or fractional property", node.HeaderSpan)
		}
	case layout.RowColumnTrackFraction:
		if item.Track.Weight == 0 {
			c.add("PAPER_COMPILE_TRACK_WEIGHT", "fraction track requires track-weight", "add track-weight: 1", node.HeaderSpan)
		}
		if item.Track.Size != 0 {
			c.add("PAPER_COMPILE_TRACK_SIZE", "fraction track cannot have track-size", "remove track-size", node.HeaderSpan)
		}
	case layout.RowColumnTrackFlex:
		if item.Track.BasisKind == "" {
			item.Track.BasisKind = layout.RowColumnFlexBasisContent
			item.Track.Shrink = 1
		}
		if item.Track.Weight != 0 {
			c.warn("PAPER_COMPILE_TRACK_WEIGHT", "track-weight is ignored after contextual sizing promotes the track to flex", "remove track-weight or use a fraction track without track-size", node.HeaderSpan)
			item.Track.Weight = 0
		}
	}
	return item, textNodes
}

func (c *compiler) compilePageBreak(node *paperlang.Node) {
	c.members(node, map[string]bool{})
	blockIndex := len(c.result.Document.Body)
	c.result.Document.Body = append(c.result.Document.Body, layout.PageBreakBlock{After: true})
	c.mapNode(node, blockIndex, -1)
}

func (c *compiler) compileList(node *paperlang.Node) {
	properties, children := c.members(node, listProperties)
	style := c.compileTextStyle(node, properties)
	ordered, orderedExplicit := false, false
	if property, ok := properties["ordered"]; ok {
		if value, valid := c.boolProperty(property); valid {
			ordered, orderedExplicit = value, true
		}
	}
	marker := ""
	if property, ok := properties["marker"]; ok {
		if value, valid := c.stringProperty(property); valid {
			marker, _ = canonicalListMarker(value)
			if marker == "" {
				c.add("PAPER_COMPILE_LIST_MARKER", fmt.Sprintf("list marker %q is unsupported", value), "use decimal, dash, or asterisk", property.Value.Span)
			} else {
				markerOrdered := marker == "decimal"
				if orderedExplicit && markerOrdered != ordered {
					c.add("PAPER_COMPILE_LIST_MARKER_ORDER", fmt.Sprintf("marker %q conflicts with ordered: %t", marker, ordered), "use decimal for ordered lists, or dash/asterisk for unordered lists", property.Value.Span)
				} else if !orderedExplicit {
					ordered = markerOrdered
				}
			}
		}
	}
	if marker == "" {
		if ordered {
			marker = "decimal"
		} else {
			marker = "dash"
		}
	}

	bodyIndex := len(c.result.Document.Body)
	block := layout.ListBlock{Ordered: ordered, MarkerStyle: marker, Style: style, Box: c.compileBoxStyle(properties)}
	c.mapNode(node, bodyIndex, -1)
	for _, child := range children {
		if child.Kind != paperlang.NodeItem {
			c.unsupportedNode(child, "list supports item children")
			continue
		}
		itemIndex := len(block.Items)
		item, ok := c.compileListItem(child, style, bodyIndex, itemIndex)
		if ok {
			block.Items = append(block.Items, item)
		}
	}
	if len(block.Items) == 0 {
		c.add("PAPER_COMPILE_LIST_ITEMS", "list has no compilable items", "add one or more item nodes containing text", node.HeaderSpan)
	}
	c.result.Document.Body = append(c.result.Document.Body, block)
	c.recordComputedStyle(node, bodyIndex)
}

func (c *compiler) compileListItem(node *paperlang.Node, listStyle layout.TextStyle, bodyIndex, itemIndex int) (layout.ListItem, bool) {
	_, children := c.members(node, map[string]bool{"when": true})
	c.mapNode(node, bodyIndex, itemIndex)
	item := layout.ListItem{}
	for _, child := range children {
		switch child.Kind {
		case paperlang.NodeText:
			nestedBlockIndex := len(item.Blocks)
			text, ok := c.textNodeValue(child)
			if !ok {
				continue
			}
			item.Blocks = append(item.Blocks, layout.ParagraphBlock{
				Segments: []layout.TextSegment{{Text: text}}, Style: listStyle,
			})
			c.mapNestedNode(child, bodyIndex, itemIndex, nestedBlockIndex)
		case paperlang.NodeParagraph:
			nestedBlockIndex := len(item.Blocks)
			properties, paragraphChildren := c.members(child, boxedTextProperties)
			style := layout.MergedTextStyle(listStyle, c.compileTextStyle(child, properties))
			segments, textNodes := c.compileTextChildren(child, paragraphChildren)
			item.Blocks = append(item.Blocks, layout.ParagraphBlock{Segments: segments, Style: style, Box: c.compileBoxStyle(properties)})
			c.mapNestedNode(child, bodyIndex, itemIndex, nestedBlockIndex)
			for _, textNode := range textNodes {
				c.mapNestedNode(textNode, bodyIndex, itemIndex, nestedBlockIndex)
			}
		default:
			c.unsupportedNode(child, "item supports paragraph and text children")
		}
	}
	if len(item.Blocks) == 0 {
		c.add("PAPER_COMPILE_LIST_ITEM_TEXT", "list item has no compilable text", "add a text child or paragraph", node.HeaderSpan)
		return layout.ListItem{}, false
	}
	return item, true
}

func (c *compiler) compileHeading(node *paperlang.Node) {
	allowed := copyPropertySet(boxedTextProperties)
	allowed["level"] = true
	properties, children := c.members(node, allowed)
	style := c.compileTextStyle(node, properties)
	level := 1
	if property, ok := properties["level"]; ok {
		if value, valid := c.numberProperty(property); valid && value >= 1 && value <= 6 && math.Trunc(value) == value {
			level = int(value)
		} else if valid {
			c.add("PAPER_COMPILE_HEADING_LEVEL", "heading level must be an integer from 1 through 6", "use level: 1 through level: 6", property.Value.Span)
		}
	}
	segments, textNodes := c.compileTextChildren(node, children)
	blockIndex := len(c.result.Document.Body)
	c.result.Document.Body = append(c.result.Document.Body, layout.HeadingBlock{Level: level, Segments: segments, Style: style, Box: c.compileBoxStyle(properties)})
	c.mapNode(node, blockIndex, -1)
	for index, textNode := range textNodes {
		c.mapNode(textNode, blockIndex, index)
	}
}

func (c *compiler) compileParagraph(node *paperlang.Node) {
	properties, children := c.members(node, boxedTextProperties)
	style := c.compileTextStyle(node, properties)
	segments, textNodes := c.compileTextChildren(node, children)
	blockIndex := len(c.result.Document.Body)
	c.result.Document.Body = append(c.result.Document.Body, layout.ParagraphBlock{Segments: segments, Style: style, Box: c.compileBoxStyle(properties)})
	c.mapNode(node, blockIndex, -1)
	for index, textNode := range textNodes {
		c.mapNode(textNode, blockIndex, index)
	}
}

func (c *compiler) compileBodyText(node *paperlang.Node) {
	text, ok := c.textNodeValue(node)
	if !ok {
		return
	}
	blockIndex := len(c.result.Document.Body)
	c.result.Document.Body = append(c.result.Document.Body, layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: text}}})
	c.mapNode(node, blockIndex, 0)
}

func (c *compiler) compileTextChildren(parent *paperlang.Node, children []*paperlang.Node) ([]layout.TextSegment, []*paperlang.Node) {
	segments := make([]layout.TextSegment, 0, len(children))
	textNodes := make([]*paperlang.Node, 0, len(children))
	for _, child := range children {
		if child.Kind != paperlang.NodeText {
			c.unsupportedNode(child, fmt.Sprintf("%s supports text children", parent.Kind))
			continue
		}
		if text, ok := c.textNodeValue(child); ok {
			segments = append(segments, layout.TextSegment{Text: text})
			textNodes = append(textNodes, child)
		}
	}
	if len(segments) == 0 {
		c.add("PAPER_COMPILE_TEXT_REQUIRED", fmt.Sprintf("%s has no compilable text", parent.Kind), "add a text child with a quoted string", parent.HeaderSpan)
		return segments, textNodes
	}
	if binding, bound := c.bindings[parent]; bound && c.fixture != nil {
		text, ok := c.bindingText(parent, binding)
		if !ok {
			text = ""
		}
		segments[0].Text = text
		for index := 1; index < len(segments); index++ {
			segments[index].Text = ""
		}
	}
	return segments, textNodes
}

func (c *compiler) textNodeValue(node *paperlang.Node) (string, bool) {
	if node.Value == nil || node.Value.Kind != paperlang.ScalarString || node.Value.StringValue == nil {
		span := node.HeaderSpan
		if node.Value != nil {
			span = node.Value.Span
		}
		c.add("PAPER_COMPILE_TEXT_VALUE", "text must contain a quoted string", "write text: \"content\"", span)
		return "", false
	}
	return *node.Value.StringValue, true
}

func (c *compiler) compileTextStyle(node *paperlang.Node, properties map[string]paperlang.Property) layout.TextStyle {
	var style layout.TextStyle
	c.compileThemeFont(node, properties, &style)
	c.compileThemeLength(node, properties, "size", "size-token", &style.FontSize)
	c.compileThemeLength(node, properties, "line-height", "line-height-token", &style.LineHeight)
	c.compileThemeColor(node, properties, &style)
	if property, ok := properties["font"]; ok {
		if value, valid := c.stringProperty(property); valid {
			canonical, supported := canonicalCoreFont(value)
			if !supported {
				resource, custom := c.assets.ResolveFont(value)
				if !custom {
					c.add("PAPER_COMPILE_FONT", fmt.Sprintf("font %q is not an admitted core or project font", value), "use a declared core font or an explicit manifest font family/name", property.Value.Span)
				} else {
					// The portable manifest name is the PDF resource key. Human
					// family metadata remains lookup-only so spaces cannot create
					// unstable PDF name fragments.
					style.FontFamily = resource.Name
				}
			} else {
				style.FontFamily = canonical
			}
		}
	}
	if property, ok := properties["size"]; ok {
		if value, valid := c.lengthProperty(property, true); valid {
			style.FontSize = value
		}
	}
	if property, ok := properties["line-height"]; ok {
		if value, valid := c.lengthProperty(property, true); valid {
			style.LineHeight = value
		}
	}
	if property, ok := properties["color"]; ok {
		if value, valid := c.stringProperty(property); valid {
			if color, supported := canonicalTextColor(value); supported {
				style.Color = color
			} else {
				c.add("PAPER_COMPILE_COLOR", fmt.Sprintf("color %q is not canonical #RRGGBB", value), "use a six-digit RGB color such as #112233", property.Value.Span)
			}
		}
	}
	if property, ok := properties["align"]; ok {
		if value, valid := c.stringProperty(property); valid {
			if align, supported := canonicalAlign(value); supported {
				style.Align = align
			} else {
				c.add("PAPER_COMPILE_ALIGN", fmt.Sprintf("alignment %q is unsupported", value), "use left, center, right, or justify", property.Value.Span)
			}
		}
	}
	if property, ok := properties["bold"]; ok {
		if value, valid := c.boolProperty(property); valid {
			style.Bold = value
		}
	}
	if property, ok := properties["italic"]; ok {
		if value, valid := c.boolProperty(property); valid {
			style.Italic = value
		}
	}
	return style
}

func (c *compiler) compileBoxStyle(properties map[string]paperlang.Property) layout.BoxStyle {
	var box layout.BoxStyle
	if property, ok := properties["margin"]; ok {
		if value, valid := c.lengthProperty(property, false); valid {
			box.Margin = layout.Spacing{Top: value, Right: value, Bottom: value, Left: value}
		}
	}
	for _, side := range []struct {
		name string
		set  func(float64)
	}{
		{"margin-top", func(value float64) { box.Margin.Top = value }},
		{"margin-right", func(value float64) { box.Margin.Right = value }},
		{"margin-bottom", func(value float64) { box.Margin.Bottom = value }},
		{"margin-left", func(value float64) { box.Margin.Left = value }},
	} {
		if property, ok := properties[side.name]; ok {
			if value, valid := c.lengthProperty(property, false); valid {
				side.set(value)
			}
		}
	}
	if property, ok := properties["padding"]; ok {
		if value, valid := c.lengthProperty(property, false); valid {
			box.Padding = layout.Spacing{Top: value, Right: value, Bottom: value, Left: value}
		}
	}
	for _, side := range []struct {
		name string
		set  func(float64)
	}{
		{"padding-top", func(value float64) { box.Padding.Top = value }},
		{"padding-right", func(value float64) { box.Padding.Right = value }},
		{"padding-bottom", func(value float64) { box.Padding.Bottom = value }},
		{"padding-left", func(value float64) { box.Padding.Left = value }},
	} {
		if property, ok := properties[side.name]; ok {
			if value, valid := c.lengthProperty(property, false); valid {
				side.set(value)
			}
		}
	}
	var borderColor = layout.DocumentColor{R: 0, G: 0, B: 0, Set: true}
	if property, ok := properties["border-color"]; ok {
		if value, valid := c.stringProperty(property); valid {
			if color, supported := canonicalTextColor(value); supported {
				borderColor = color
			} else {
				c.add("PAPER_COMPILE_BORDER_COLOR", fmt.Sprintf("border color %q is not canonical #RRGGBB", value), "use a six-digit RGB color such as #112233", property.Value.Span)
			}
		}
	}
	setBorder := func(side *layout.BorderSide, width float64) {
		side.Width, side.Color = width, borderColor
		if width > 0 {
			side.Style = "solid"
		}
	}
	if property, ok := properties["border-width"]; ok {
		if value, valid := c.lengthProperty(property, false); valid {
			setBorder(&box.Border.Top, value)
			setBorder(&box.Border.Right, value)
			setBorder(&box.Border.Bottom, value)
			setBorder(&box.Border.Left, value)
		}
	}
	for _, side := range []struct {
		name   string
		border *layout.BorderSide
	}{
		{"border-top-width", &box.Border.Top}, {"border-right-width", &box.Border.Right},
		{"border-bottom-width", &box.Border.Bottom}, {"border-left-width", &box.Border.Left},
	} {
		if property, ok := properties[side.name]; ok {
			if value, valid := c.lengthProperty(property, false); valid {
				setBorder(side.border, value)
			}
		}
	}
	if property, ok := properties["border-radius"]; ok {
		if value, valid := c.lengthProperty(property, false); valid {
			box.BorderRadius = value
		}
	}
	if property, ok := properties["background"]; ok {
		if value, valid := c.stringProperty(property); valid {
			if color, supported := canonicalTextColor(value); supported {
				box.BackgroundColor = color
			} else {
				c.add("PAPER_COMPILE_BACKGROUND", fmt.Sprintf("background %q is not canonical #RRGGBB", value), "use a six-digit RGB color such as #f2f4f8", property.Value.Span)
			}
		}
	}
	return box
}

func (c *compiler) compileThemeFont(node *paperlang.Node, properties map[string]paperlang.Property, style *layout.TextStyle) {
	token, hasToken := properties["font-token"]
	if _, literal := properties["font"]; literal {
		if hasToken {
			c.themeOverride("font", token)
		}
		return
	}
	if !hasToken {
		return
	}
	resolved, name, ok := c.resolveThemeToken(token, papertheme.String)
	if !ok {
		return
	}
	canonical, supported := canonicalCoreFont(resolved.Value.String)
	if !supported {
		c.add("PAPER_COMPILE_THEME_TOKEN_VALUE", fmt.Sprintf("font token %q resolves to unsupported core font %q", name, resolved.Value.String), "use Courier, Helvetica, Times, Symbol, or ZapfDingbats", token.Value.Span)
		return
	}
	style.FontFamily = canonical
	c.recordThemeProperty(node, token, name, resolved)
}

func (c *compiler) compileThemeLength(node *paperlang.Node, properties map[string]paperlang.Property, literalName, tokenName string, destination *float64) {
	token, hasToken := properties[tokenName]
	if _, literal := properties[literalName]; literal {
		if hasToken {
			c.themeOverride(literalName, token)
		}
		return
	}
	if !hasToken {
		return
	}
	resolved, name, ok := c.resolveThemeToken(token, papertheme.Length)
	if !ok {
		return
	}
	if resolved.Value.Length.Unit != "pt" {
		c.add("PAPER_COMPILE_THEME_TOKEN_VALUE", fmt.Sprintf("%s %q uses unsupported unit %q", tokenName, name, resolved.Value.Length.Unit), "use a positive pt length in the initial theme compiler", token.Value.Span)
		return
	}
	value, err := strconv.ParseFloat(resolved.Value.Length.Number, 64)
	if err != nil || !isFinite(value) || value <= 0 {
		c.add("PAPER_COMPILE_THEME_TOKEN_VALUE", fmt.Sprintf("%s %q does not resolve to a positive finite length", tokenName, name), "use a positive pt length", token.Value.Span)
		return
	}
	*destination = value
	c.recordThemeProperty(node, token, name, resolved)
}

func (c *compiler) compileThemeColor(node *paperlang.Node, properties map[string]paperlang.Property, style *layout.TextStyle) {
	token, hasToken := properties["color-token"]
	if _, literal := properties["color"]; literal {
		if hasToken {
			c.themeOverride("color", token)
		}
		return
	}
	if !hasToken {
		return
	}
	resolved, name, ok := c.resolveThemeToken(token, papertheme.Color)
	if !ok {
		return
	}
	color, supported := canonicalTextColor(resolved.Value.Color)
	if !supported {
		c.add("PAPER_COMPILE_THEME_TOKEN_VALUE", fmt.Sprintf("color token %q is not canonical #RRGGBB", name), "use a six-digit RGB color", token.Value.Span)
		return
	}
	style.Color = color
	c.recordThemeProperty(node, token, name, resolved)
}

func (c *compiler) resolveThemeToken(property paperlang.Property, expected papertheme.Kind) (papertheme.ResolvedToken, string, bool) {
	if property.Value.Kind != paperlang.ScalarString || property.Value.StringValue == nil {
		c.add("PAPER_COMPILE_THEME_TOKEN_TYPE", fmt.Sprintf("property %q requires a quoted token reference", property.Name), "use a quoted root-scope token name", property.Value.Span)
		return papertheme.ResolvedToken{}, "", false
	}
	value := *property.Value.StringValue
	name := strings.TrimPrefix(strings.TrimSpace(value), "@")
	if !c.themeSelected {
		c.add("PAPER_COMPILE_THEME_REQUIRED", fmt.Sprintf("property %q requires a selected document theme", property.Name), "add theme: \"@name\" to document", property.Value.Span)
		return papertheme.ResolvedToken{}, name, false
	}
	resolved, status := c.visibleThemeToken(c.selectedTheme, name)
	switch status {
	case themeTokenUnknown:
		c.add("PAPER_COMPILE_THEME_TOKEN_UNKNOWN", fmt.Sprintf("token %q is not visible in theme %q root scope", name, c.selectedTheme), "declare the token in the selected theme or an inherited root scope", property.Value.Span)
		return papertheme.ResolvedToken{}, name, false
	case themeTokenType:
		c.add("PAPER_COMPILE_THEME_TOKEN_TYPE", fmt.Sprintf("token %q has an invalid or mismatched type", name), fmt.Sprintf("use a %s token", expected), property.Value.Span)
		return papertheme.ResolvedToken{}, name, false
	case themeTokenInvalid:
		c.add("PAPER_COMPILE_THEME_TOKEN_VALUE", fmt.Sprintf("token %q cannot be resolved to a valid value", name), "fix the token literal or reference chain", property.Value.Span)
		return papertheme.ResolvedToken{}, name, false
	}
	if resolved.Value.Kind != expected {
		c.add("PAPER_COMPILE_THEME_TOKEN_TYPE", fmt.Sprintf("token %q resolves to %s, but %s requires %s", name, resolved.Value.Kind, property.Name, expected), "select a token with the required kind", property.Value.Span)
		return papertheme.ResolvedToken{}, name, false
	}
	return resolved, name, true
}

type themeTokenStatus uint8

const (
	themeTokenOK themeTokenStatus = iota
	themeTokenUnknown
	themeTokenType
	themeTokenInvalid
)

func (c *compiler) visibleThemeToken(themeName, tokenName string) (papertheme.ResolvedToken, themeTokenStatus) {
	visited := make(map[string]bool)
	for themeName != "" && !visited[themeName] {
		visited[themeName] = true
		theme := c.theme(themeName)
		if theme == nil {
			return papertheme.ResolvedToken{}, themeTokenUnknown
		}
		for _, definition := range theme.Tokens {
			if definition.Name != tokenName {
				continue
			}
			for _, resolvedTheme := range c.themeOutput.Themes {
				if resolvedTheme.Name != themeName {
					continue
				}
				for _, token := range resolvedTheme.Tokens {
					if token.Name == tokenName && len(token.Scope) == 0 {
						return token, themeTokenOK
					}
				}
			}
			return papertheme.ResolvedToken{}, c.invalidThemeTokenStatus(definition)
		}
		themeName = theme.Parent
	}
	return papertheme.ResolvedToken{}, themeTokenUnknown
}

func (c *compiler) invalidThemeTokenStatus(token papertheme.Token) themeTokenStatus {
	for _, diagnostic := range c.themeDiagnostics {
		if diagnostic.Span.Start.Offset < token.Source.StartOffset || diagnostic.Span.Start.Offset > token.Source.EndOffset || diagnostic.Span.File != token.Source.File {
			continue
		}
		switch diagnostic.Code {
		case "PAPER_THEME_TOKEN_UNKNOWN":
			return themeTokenUnknown
		case "PAPER_THEME_TOKEN_TYPE":
			return themeTokenType
		}
	}
	return themeTokenInvalid
}

func (c *compiler) theme(name string) *papertheme.Theme {
	for index := range c.themeInput.Themes {
		if c.themeInput.Themes[index].Name == name {
			return &c.themeInput.Themes[index]
		}
	}
	return nil
}

func (c *compiler) recordThemeProperty(node *paperlang.Node, property paperlang.Property, tokenName string, resolved papertheme.ResolvedToken) {
	provenance := papertheme.Provenance{Property: papertheme.Source{
		File: property.Value.Span.File, StartOffset: property.Value.Span.Start.Offset, EndOffset: property.Value.Span.End.Offset,
		Line: property.Value.Span.Start.Line, Column: property.Value.Span.Start.Column,
	}, Chain: cloneThemeTokenSteps(resolved.Provenance.Chain)}
	c.result.Mapping.ThemeProperties = append(c.result.Mapping.ThemeProperties, ThemePropertyMapping{
		NodeID: node.ID, NodeKind: node.Kind, Property: property.Name, ConsumerSpan: property.Value.Span,
		Theme: c.selectedTheme, Token: tokenName, Value: resolved.Value, Provenance: provenance,
	})
}

func cloneThemeTokenSteps(input []papertheme.TokenStep) []papertheme.TokenStep {
	if len(input) == 0 {
		return nil
	}
	output := make([]papertheme.TokenStep, len(input))
	copy(output, input)
	for index := range output {
		output[index].Scope = append([]string(nil), input[index].Scope...)
	}
	return output
}

func (c *compiler) themeOverride(literal string, token paperlang.Property) {
	c.warn("PAPER_COMPILE_THEME_STYLE_OVERRIDE", fmt.Sprintf("literal property %q overrides %q", literal, token.Name), "remove one property to make the style source unambiguous", token.Span)
}

func canonicalTextColor(value string) (layout.DocumentColor, bool) {
	if len(value) != 7 || value[0] != '#' {
		return layout.DocumentColor{}, false
	}
	red, redErr := strconv.ParseUint(value[1:3], 16, 8)
	green, greenErr := strconv.ParseUint(value[3:5], 16, 8)
	blue, blueErr := strconv.ParseUint(value[5:7], 16, 8)
	if redErr != nil || greenErr != nil || blueErr != nil {
		return layout.DocumentColor{}, false
	}
	return layout.DocumentColor{R: int(red), G: int(green), B: int(blue), Set: true}, true
}

func (c *compiler) members(node *paperlang.Node, supported map[string]bool) (map[string]paperlang.Property, []*paperlang.Node) {
	properties := make(map[string]paperlang.Property)
	children := make([]*paperlang.Node, 0)
	for _, member := range node.Members {
		if member.Node != nil {
			children = append(children, member.Node)
			continue
		}
		if member.Property == nil {
			continue
		}
		property := *member.Property
		if _, duplicate := properties[property.Name]; duplicate {
			if node.Kind == paperlang.NodeDocument && property.Name == "import" {
				continue
			}
			c.add("PAPER_COMPILE_DUPLICATE_PROPERTY", fmt.Sprintf("property %q is repeated on %s", property.Name, node.Kind), "remove the duplicate; the first value is retained", property.Span)
			continue
		}
		properties[property.Name] = property
		if !supported[property.Name] {
			c.add("PAPER_COMPILE_UNSUPPORTED_PROPERTY", fmt.Sprintf("property %q is unsupported on %s", property.Name, node.Kind), "remove it or use a supported property", property.Span)
		}
	}
	return c.applyStyle(properties, supported), children
}

func (c *compiler) stringProperty(property paperlang.Property) (string, bool) {
	if property.Value.Kind != paperlang.ScalarString || property.Value.StringValue == nil {
		c.typeError(property, "quoted string")
		return "", false
	}
	return *property.Value.StringValue, true
}

func (c *compiler) boolProperty(property paperlang.Property) (bool, bool) {
	if property.Value.Kind != paperlang.ScalarBool || property.Value.BoolValue == nil {
		c.typeError(property, "boolean")
		return false, false
	}
	return *property.Value.BoolValue, true
}

func (c *compiler) numberProperty(property paperlang.Property) (float64, bool) {
	if property.Value.Kind != paperlang.ScalarNumber || property.Value.NumberValue == nil {
		c.typeError(property, "number")
		return 0, false
	}
	return *property.Value.NumberValue, true
}

type contextualLength struct {
	points     float64
	percent    uint32
	percentSet bool
	auto       bool
}

// contextualLengthProperty retains percentages until a layout container has
// a definite size. Percent uses millionths of one percent so compilation and
// fixed-point planning remain deterministic across renderers and DPI values.
func (c *compiler) contextualLengthProperty(property paperlang.Property, positive, allowAuto bool) (contextualLength, bool) {
	if allowAuto && property.Value.Kind == paperlang.ScalarString && property.Value.StringValue != nil {
		if strings.EqualFold(strings.TrimSpace(*property.Value.StringValue), "auto") {
			return contextualLength{auto: true}, true
		}
		c.typeError(property, "physical length, percentage, or \"auto\"")
		return contextualLength{}, false
	}
	if property.Value.Kind == paperlang.ScalarUnit && property.Value.UnitValue != nil && property.Value.UnitValue.Unit == "%" {
		value := property.Value.UnitValue.Number
		if !isFinite(value) || value < 0 || value > 100 || positive && value == 0 {
			qualifier := "from 0% through 100%"
			if positive {
				qualifier = "greater than 0% and at most 100%"
			}
			c.add("PAPER_COMPILE_PERCENT", fmt.Sprintf("property %q must be %s", property.Name, qualifier), "use a bounded percentage such as 50% or 100%", property.Value.Span)
			return contextualLength{}, false
		}
		scaled := math.Round(value * 1_000_000)
		if scaled < 0 || scaled > 100_000_000 {
			c.add("PAPER_COMPILE_PERCENT", fmt.Sprintf("property %q percentage is outside the representable range", property.Name), "use a percentage from 0% through 100%", property.Value.Span)
			return contextualLength{}, false
		}
		return contextualLength{percent: uint32(scaled), percentSet: true}, true
	}
	points, valid := c.lengthProperty(property, positive)
	return contextualLength{points: points}, valid
}

func (c *compiler) lengthProperty(property paperlang.Property, positive bool) (float64, bool) {
	var points float64
	switch property.Value.Kind {
	case paperlang.ScalarNumber:
		if property.Value.NumberValue == nil {
			c.typeError(property, "physical length")
			return 0, false
		}
		points = *property.Value.NumberValue
	case paperlang.ScalarUnit:
		if property.Value.UnitValue == nil {
			c.typeError(property, "physical length")
			return 0, false
		}
		value := property.Value.UnitValue.Number
		switch property.Value.UnitValue.Unit {
		case "pt":
			points = value
		case "mm":
			points = value * 72 / 25.4
		case "cm":
			points = value * 72 / 2.54
		case "in":
			points = value * 72
		case "px":
			points = value * 72 / 96
		case "pc":
			points = value * 12
		default:
			c.add("PAPER_COMPILE_RELATIVE_UNIT", fmt.Sprintf("unit %q needs layout context", property.Value.UnitValue.Unit), "use pt, mm, cm, in, px, or pc in the initial compiler", property.Value.Span)
			return 0, false
		}
	default:
		c.typeError(property, "physical length")
		return 0, false
	}
	if !isFinite(points) || points < 0 || positive && points == 0 {
		qualifier := "non-negative"
		if positive {
			qualifier = "positive"
		}
		c.add("PAPER_COMPILE_LENGTH", fmt.Sprintf("property %q must be a finite %s length", property.Name, qualifier), "use a valid physical length", property.Value.Span)
		return 0, false
	}
	return points, true
}

func (c *compiler) typeError(property paperlang.Property, expected string) {
	c.add("PAPER_COMPILE_PROPERTY_TYPE", fmt.Sprintf("property %q requires a %s", property.Name, expected), "change the property value type", property.Value.Span)
}

func (c *compiler) mapNode(node *paperlang.Node, bodyIndex, segmentIndex int) {
	c.mapNestedNode(node, bodyIndex, segmentIndex, -1)
	c.recordComputedStyle(node, bodyIndex)
}

func (c *compiler) recordComputedStyle(node *paperlang.Node, bodyIndex int) {
	if node == nil || node.ID == "" || bodyIndex < 0 || bodyIndex >= len(c.result.Document.Body) {
		return
	}
	textStyle, boxStyle, ok := computedBlockStyle(c.result.Document.Body[bodyIndex])
	if !ok {
		return
	}
	c.result.Mapping.ComputedStyles = append(c.result.Mapping.ComputedStyles, ComputedStyleMapping{
		NodeID: node.ID, NodeKind: node.Kind, Source: node.Span, TextStyle: textStyle, BoxStyle: boxStyle,
	})
}

func computedBlockStyle(block layout.Block) (*layout.TextStyle, *layout.BoxStyle, bool) {
	copyText := func(style layout.TextStyle) *layout.TextStyle { return &style }
	copyBox := func(style layout.BoxStyle) *layout.BoxStyle { return &style }
	switch value := block.(type) {
	case layout.ParagraphBlock:
		return copyText(value.EffectiveStyle()), copyBox(value.EffectiveBox()), true
	case layout.HeadingBlock:
		return copyText(value.EffectiveStyle()), copyBox(value.EffectiveBox()), true
	case layout.ListBlock:
		return copyText(value.EffectiveStyle()), copyBox(value.EffectiveBox()), true
	case layout.TableBlock:
		return nil, copyBox(value.EffectiveBox()), true
	case layout.ImageBlock:
		return nil, copyBox(value.EffectiveBox()), true
	case layout.SignatureRowBlock:
		return nil, copyBox(value.EffectiveBox()), true
	case layout.MetadataGridBlock:
		return copyText(value.EffectiveStyle()), copyBox(value.EffectiveBox()), true
	case layout.QRVerificationBlock:
		return copyText(value.EffectiveStyle()), copyBox(value.EffectiveBox()), true
	case layout.NoteBoxBlock:
		return copyText(value.EffectiveStyle()), copyBox(value.EffectiveBox()), true
	case layout.SectionBlock:
		return nil, copyBox(value.EffectiveBox()), true
	case layout.ClauseBlock:
		return nil, copyBox(value.EffectiveBox()), true
	default:
		return nil, nil, false
	}
}

func (c *compiler) mapNestedNode(node *paperlang.Node, bodyIndex, segmentIndex, nestedBlockIndex int) {
	if node == nil {
		return
	}
	mapping := NodeMapping{
		ID: node.ID, Kind: node.Kind, BodyIndex: bodyIndex, SegmentIndex: segmentIndex,
		NestedBlockIndex: nestedBlockIndex, Span: node.Span,
		DefinitionSpan: c.provenance[node].definition, InvocationSpan: c.provenance[node].invocation,
		InstancePath: c.provenance[node].instancePath,
		BindingPath:  c.bindings[node].path, BindingSpan: c.bindings[node].span,
		BindingNullable: c.bindings[node].nullable, BindingCollection: c.bindings[node].collection,
	}
	if node.ID == "" {
		for _, existing := range c.result.Mapping.AnonymousNodes {
			if existing.Kind == mapping.Kind && existing.BodyIndex == bodyIndex && existing.SegmentIndex == segmentIndex &&
				existing.NestedBlockIndex == nestedBlockIndex && existing.Span == mapping.Span {
				return
			}
		}
		c.result.Mapping.AnonymousNodes = append(c.result.Mapping.AnonymousNodes, mapping)
		return
	}
	if _, duplicate := c.ids[node.ID]; duplicate {
		c.add("PAPER_COMPILE_DUPLICATE_ID", fmt.Sprintf("readable ID %s is repeated", node.ID), "use a unique readable ID", node.HeaderSpan)
		return
	}
	c.ids[node.ID] = node.HeaderSpan
	c.result.Mapping.Nodes = append(c.result.Mapping.Nodes, mapping)
}

func (c *compiler) unsupportedNode(node *paperlang.Node, hint string) {
	c.add("PAPER_COMPILE_UNSUPPORTED_NODE", fmt.Sprintf("%s cannot be lowered here", node.Kind), hint, node.HeaderSpan)
}

func (c *compiler) add(code, message, hint string, span paperlang.Span) {
	c.result.Diagnostics = append(c.result.Diagnostics, paperlang.Diagnostic{
		Code: code, Severity: paperlang.SeverityError, Message: message, Hint: hint, Span: span,
	})
}

func (c *compiler) warn(code, message, hint string, span paperlang.Span) {
	c.result.Diagnostics = append(c.result.Diagnostics, paperlang.Diagnostic{
		Code: code, Severity: paperlang.SeverityWarning, Message: message, Hint: hint, Span: span,
	})
}

func copyPropertySet(input map[string]bool) map[string]bool {
	result := make(map[string]bool, len(input)+1)
	for key, value := range input {
		result[key] = value
	}
	return result
}

func canonicalCoreFont(value string) (string, bool) {
	switch strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), " ", "")) {
	case "courier":
		return "Courier", true
	case "helvetica", "arial":
		return "Helvetica", true
	case "times", "timesroman":
		return "Times", true
	case "symbol":
		return "Symbol", true
	case "zapfdingbats":
		return "ZapfDingbats", true
	default:
		return "", false
	}
}

func canonicalAlign(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "l", "left":
		return "L", true
	case "c", "center", "centre":
		return "C", true
	case "r", "right":
		return "R", true
	case "j", "justify", "justified":
		return "J", true
	default:
		return "", false
	}
}

func canonicalCrossAlign(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "start":
		return "start", true
	case "center", "centre":
		return "center", true
	case "end":
		return "end", true
	case "stretch":
		return "stretch", true
	default:
		return "", false
	}
}

func canonicalListMarker(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "decimal", "number", "numbers":
		return "decimal", true
	case "dash", "hyphen":
		return "dash", true
	case "asterisk", "star":
		return "asterisk", true
	default:
		return "", false
	}
}

func isFinite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }
