// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/cssbruno/gopdfkit/internal/paperedit"
	"github.com/cssbruno/gopdfkit/internal/paperlang"
)

// PaperBoxProperty is the closed authored vocabulary exposed to box handles.
// Each request changes one readable property and therefore produces one
// minimal CST patch even when the property did not previously exist.
type PaperBoxProperty string

const (
	PaperBoxPadding           PaperBoxProperty = "padding"
	PaperBoxPaddingTop        PaperBoxProperty = "padding-top"
	PaperBoxPaddingRight      PaperBoxProperty = "padding-right"
	PaperBoxPaddingBottom     PaperBoxProperty = "padding-bottom"
	PaperBoxPaddingLeft       PaperBoxProperty = "padding-left"
	PaperBoxBorderWidth       PaperBoxProperty = "border-width"
	PaperBoxBorderTopWidth    PaperBoxProperty = "border-top-width"
	PaperBoxBorderRightWidth  PaperBoxProperty = "border-right-width"
	PaperBoxBorderBottomWidth PaperBoxProperty = "border-bottom-width"
	PaperBoxBorderLeftWidth   PaperBoxProperty = "border-left-width"
	PaperBoxBorderColor       PaperBoxProperty = "border-color"
	PaperBoxRadius            PaperBoxProperty = "border-radius"
	PaperBoxBackground        PaperBoxProperty = "background"
)

type PaperPageMarginProperty string

const (
	PaperPageMarginAll    PaperPageMarginProperty = "margin"
	PaperPageMarginTop    PaperPageMarginProperty = "margin-top"
	PaperPageMarginRight  PaperPageMarginProperty = "margin-right"
	PaperPageMarginBottom PaperPageMarginProperty = "margin-bottom"
	PaperPageMarginLeft   PaperPageMarginProperty = "margin-left"
)

type PaperSetPageMarginRequest struct {
	Guard    PaperMutationGuard      `json:"guard"`
	Property PaperPageMarginProperty `json:"property"`
	Points   float64                 `json:"points"`
}

type PaperSetPageSizeRequest struct {
	Guard        PaperMutationGuard `json:"guard"`
	WidthPoints  float64            `json:"width_points"`
	HeightPoints float64            `json:"height_points"`
}

type PaperCanvasAnchorProperty string

const (
	PaperCanvasLeft    PaperCanvasAnchorProperty = "left"
	PaperCanvasRight   PaperCanvasAnchorProperty = "right"
	PaperCanvasCenterX PaperCanvasAnchorProperty = "center-x"
	PaperCanvasTop     PaperCanvasAnchorProperty = "top"
	PaperCanvasBottom  PaperCanvasAnchorProperty = "bottom"
	PaperCanvasCenterY PaperCanvasAnchorProperty = "center-y"
)

type PaperSetCanvasAnchorRequest struct {
	Guard        PaperMutationGuard        `json:"guard"`
	Property     PaperCanvasAnchorProperty `json:"property"`
	Reference    string                    `json:"reference"`
	TargetAnchor PaperCanvasAnchorProperty `json:"target_anchor"`
	Offset       float64                   `json:"offset_points,omitempty"`
}

func canvasAnchorAxis(property PaperCanvasAnchorProperty) byte {
	switch property {
	case PaperCanvasLeft, PaperCanvasRight, PaperCanvasCenterX:
		return 'x'
	case PaperCanvasTop, PaperCanvasBottom, PaperCanvasCenterY:
		return 'y'
	default:
		return 0
	}
}

// PaperSetCanvasAnchor edits one governing source constraint. The local
// canvas is an explicit transitive effect because moving one child changes
// the dependency graph reviewed for every dependent sibling.
func (w *Workspace) PaperSetCanvasAnchor(request PaperSetCanvasAnchorRequest) (PaperMutationResult, error) {
	opened, revision, err := w.mutationRevision(request.Guard)
	if err != nil {
		return PaperMutationResult{}, err
	}
	node, parent := sourceNodeAndParent(revision.parsed.AST.Root, request.Guard.Target)
	if node == nil || parent == nil || node.Kind != paperlang.NodeAnchor || parent.Kind != paperlang.NodeCanvas || parent.ID == "" {
		return PaperMutationResult{}, workspaceError("INVALID_CANVAS_ANCHOR_TARGET", "canvas handles require an exact anchor directly inside an addressed canvas", paperedit.ErrInvalidOperation)
	}
	if err := requireAdditionalTargetGuard(revision, request.Guard, parent.ID); err != nil {
		return PaperMutationResult{}, err
	}
	axis := canvasAnchorAxis(request.Property)
	if axis == 0 || canvasAnchorAxis(request.TargetAnchor) != axis {
		return PaperMutationResult{}, workspaceError("INVALID_CANVAS_ANCHOR_PROPERTY", "canvas anchors must use supported same-axis source and target anchors", paperedit.ErrInvalidOperation)
	}
	if request.Reference != "canvas" {
		reference := findNodeByID(revision.parsed.AST.Root, request.Reference)
		_, referenceParent := sourceNodeAndParent(revision.parsed.AST.Root, request.Reference)
		if reference == nil || reference.Kind != paperlang.NodeAnchor || referenceParent != parent || reference.ID == node.ID {
			return PaperMutationResult{}, workspaceError("INVALID_CANVAS_ANCHOR_REFERENCE", "canvas anchor reference must be a different addressed sibling in the same canvas", paperedit.ErrInvalidOperation)
		}
	}
	if !finiteLayoutHandle(request.Offset) || request.Offset < -1_000_000 || request.Offset > 1_000_000 {
		return PaperMutationResult{}, workspaceError("INVALID_CANVAS_ANCHOR_OFFSET", "canvas anchor offset must be a finite bounded point value", paperedit.ErrInvalidOperation)
	}
	expression := request.Reference + "." + string(request.TargetAnchor)
	if request.Offset > 0 {
		expression += " + " + strconv.FormatFloat(request.Offset, 'f', -1, 64) + "pt"
	} else if request.Offset < 0 {
		expression += " - " + strconv.FormatFloat(-request.Offset, 'f', -1, 64) + "pt"
	}
	operation := paperedit.SetProperty{Target: request.Guard.Target, Name: string(request.Property), Value: paperedit.StringValue(expression)}
	return w.applyPaperMutation("set_canvas_anchor", request.Guard, opened, revision, []string{request.Guard.Target, parent.ID}, []paperedit.Operation{operation}, "INVALID_CANVAS_CONSTRAINT")
}

// PaperSetPageMargin edits the authored page-master body region instead of a
// computed page box. The page node is the governing source node and the full
// candidate is recompiled before publication.
func (w *Workspace) PaperSetPageMargin(request PaperSetPageMarginRequest) (PaperMutationResult, error) {
	opened, revision, err := w.mutationRevision(request.Guard)
	if err != nil {
		return PaperMutationResult{}, err
	}
	node := findNodeByID(revision.parsed.AST.Root, request.Guard.Target)
	if node == nil || node.Kind != paperlang.NodePage {
		return PaperMutationResult{}, workspaceError("INVALID_PAGE_MASTER_TARGET", "page-master margin handles require an exact page source node", paperedit.ErrInvalidOperation)
	}
	switch request.Property {
	case PaperPageMarginAll, PaperPageMarginTop, PaperPageMarginRight, PaperPageMarginBottom, PaperPageMarginLeft:
	default:
		return PaperMutationResult{}, workspaceError("INVALID_PAGE_MARGIN_PROPERTY", "page margin is outside the closed handle vocabulary", paperedit.ErrInvalidOperation)
	}
	if !finiteLayoutHandle(request.Points) || request.Points < 0 || request.Points > 1_000_000 {
		return PaperMutationResult{}, workspaceError("INVALID_PAGE_MARGIN_VALUE", "page margin must be a finite non-negative point value", paperedit.ErrInvalidOperation)
	}
	operation := paperedit.SetProperty{Target: request.Guard.Target, Name: string(request.Property), Value: paperedit.UnitValue(request.Points, "pt")}
	return w.applyPaperMutation("set_page_margin", request.Guard, opened, revision, []string{request.Guard.Target}, []paperedit.Operation{operation}, "INVALID_PAGE_MASTER")
}

// PaperSetPageSize writes explicit physical page dimensions. Presets are
// resolved by the caller before this boundary so the retained source remains
// unambiguous and custom sizes use the identical mutation path.
func (w *Workspace) PaperSetPageSize(request PaperSetPageSizeRequest) (PaperMutationResult, error) {
	opened, revision, err := w.mutationRevision(request.Guard)
	if err != nil {
		return PaperMutationResult{}, err
	}
	node := findNodeByID(revision.parsed.AST.Root, request.Guard.Target)
	if node == nil || node.Kind != paperlang.NodePage {
		return PaperMutationResult{}, workspaceError("INVALID_PAGE_SIZE_TARGET", "page-size handles require an exact page source node", paperedit.ErrInvalidOperation)
	}
	if !finiteLayoutHandle(request.WidthPoints) || !finiteLayoutHandle(request.HeightPoints) ||
		request.WidthPoints <= 0 || request.HeightPoints <= 0 || request.WidthPoints > 1_000_000 || request.HeightPoints > 1_000_000 {
		return PaperMutationResult{}, workspaceError("INVALID_PAGE_SIZE_VALUE", "page dimensions must be finite positive point values", paperedit.ErrInvalidOperation)
	}
	operations := []paperedit.Operation{
		paperedit.SetProperty{Target: request.Guard.Target, Name: "width", Value: paperedit.UnitValue(request.WidthPoints, "pt")},
		paperedit.SetProperty{Target: request.Guard.Target, Name: "height", Value: paperedit.UnitValue(request.HeightPoints, "pt")},
	}
	return w.applyPaperMutation("set_page_size", request.Guard, opened, revision, []string{request.Guard.Target}, operations, "INVALID_PAGE_SIZE")
}

type PaperSetPageRegionRequest struct {
	Guard    PaperMutationGuard `json:"guard"`
	Property string             `json:"property"`
	Points   float64            `json:"points,omitempty"`
	Color    string             `json:"color,omitempty"`
	Bool     bool               `json:"bool,omitempty"`
}

// PaperSetPageRegion writes the authored header/footer node and treats the
// governing page master as a transitive effect because region measurement
// changes the body's available rectangle.
func (w *Workspace) PaperSetPageRegion(request PaperSetPageRegionRequest) (PaperMutationResult, error) {
	opened, revision, err := w.mutationRevision(request.Guard)
	if err != nil {
		return PaperMutationResult{}, err
	}
	node, parent := sourceNodeAndParent(revision.parsed.AST.Root, request.Guard.Target)
	if node == nil || parent == nil || (node.Kind != paperlang.NodeHeader && node.Kind != paperlang.NodeFooter) || parent.Kind != paperlang.NodePage || parent.ID == "" {
		return PaperMutationResult{}, workspaceError("INVALID_PAGE_REGION_TARGET", "page-region handles require an exact header or footer directly inside an addressed page", paperedit.ErrInvalidOperation)
	}
	if err := requireAdditionalTargetGuard(revision, request.Guard, parent.ID); err != nil {
		return PaperMutationResult{}, err
	}
	var value paperedit.Value
	property := PaperBoxProperty(request.Property)
	switch {
	case property.length():
		if request.Color != "" || request.Bool || !finiteLayoutHandle(request.Points) || request.Points < 0 || request.Points > 1_000_000 {
			return PaperMutationResult{}, workspaceError("INVALID_PAGE_REGION_VALUE", "region length must be a finite non-negative point value", paperedit.ErrInvalidOperation)
		}
		value = paperedit.UnitValue(request.Points, "pt")
	case property.color():
		color, ok := canonicalLayoutHandleColor(request.Color)
		if !ok || request.Points != 0 || request.Bool {
			return PaperMutationResult{}, workspaceError("INVALID_PAGE_REGION_VALUE", "region color must be canonical #RRGGBB", paperedit.ErrInvalidOperation)
		}
		value = paperedit.StringValue(color)
	default:
		return PaperMutationResult{}, workspaceError("INVALID_PAGE_REGION_PROPERTY", "page region property is outside the closed handle vocabulary", paperedit.ErrInvalidOperation)
	}
	operation := paperedit.SetProperty{Target: request.Guard.Target, Name: request.Property, Value: value}
	return w.applyPaperMutation("set_page_region", request.Guard, opened, revision, []string{request.Guard.Target, parent.ID}, []paperedit.Operation{operation}, "INVALID_PAGE_REGION")
}

func (property PaperBoxProperty) length() bool {
	switch property {
	case PaperBoxPadding, PaperBoxPaddingTop, PaperBoxPaddingRight, PaperBoxPaddingBottom, PaperBoxPaddingLeft,
		PaperBoxBorderWidth, PaperBoxBorderTopWidth, PaperBoxBorderRightWidth, PaperBoxBorderBottomWidth,
		PaperBoxBorderLeftWidth, PaperBoxRadius:
		return true
	default:
		return false
	}
}

func (property PaperBoxProperty) color() bool {
	return property == PaperBoxBorderColor || property == PaperBoxBackground
}

type PaperSetBoxPropertyRequest struct {
	Guard    PaperMutationGuard `json:"guard"`
	Property PaperBoxProperty   `json:"property"`
	Points   float64            `json:"points,omitempty"`
	Color    string             `json:"color,omitempty"`
}

// PaperTextProperty is the closed authored typography vocabulary exposed by
// text-style handles. Font replacement is explicit and never automatic.
type PaperTextProperty string

const PaperTextFont PaperTextProperty = "font"

type PaperSetTextPropertyRequest struct {
	Guard    PaperMutationGuard `json:"guard"`
	Property PaperTextProperty  `json:"property"`
	Text     string             `json:"text"`
}

func canonicalPaperCoreFont(value string) (string, bool) {
	switch value {
	case "Courier", "Helvetica", "Times", "Symbol", "ZapfDingbats":
		return value, true
	default:
		return "", false
	}
}

// PaperSetTextProperty replaces one authored text-style property. The source
// may fail compilation because of the old font; the replacement is published
// only when the complete candidate compiles.
func (w *Workspace) PaperSetTextProperty(request PaperSetTextPropertyRequest) (PaperMutationResult, error) {
	opened, revision, err := w.mutationRevision(request.Guard)
	if err != nil {
		return PaperMutationResult{}, err
	}
	node := findNodeByID(revision.parsed.AST.Root, request.Guard.Target)
	if node == nil || node.Kind != paperlang.NodeParagraph && node.Kind != paperlang.NodeHeading && node.Kind != paperlang.NodeList {
		return PaperMutationResult{}, workspaceError("INVALID_TEXT_STYLE_TARGET", "text-style handles require a paragraph, heading, or list source node", paperedit.ErrInvalidOperation)
	}
	if request.Property != PaperTextFont {
		return PaperMutationResult{}, workspaceError("INVALID_TEXT_STYLE_PROPERTY", "text-style property is outside the closed handle vocabulary", paperedit.ErrInvalidOperation)
	}
	font, ok := canonicalPaperCoreFont(request.Text)
	if !ok {
		return PaperMutationResult{}, workspaceError("INVALID_TEXT_STYLE_VALUE", "font must name an existing supported core font", paperedit.ErrInvalidOperation)
	}
	operation := paperedit.SetProperty{Target: request.Guard.Target, Name: string(request.Property), Value: paperedit.StringValue(font)}
	return w.applyPaperMutation("set_text_property", request.Guard, opened, revision, []string{request.Guard.Target}, []paperedit.Operation{operation}, "INVALID_TEXT_STYLE")
}

// PaperSetBoxProperty updates one box handle on a paragraph, heading, or list.
// The typed compiler validates the complete candidate before publication.
func (w *Workspace) PaperSetBoxProperty(request PaperSetBoxPropertyRequest) (PaperMutationResult, error) {
	opened, revision, err := w.mutationRevision(request.Guard)
	if err != nil {
		return PaperMutationResult{}, err
	}
	node := findNodeByID(revision.parsed.AST.Root, request.Guard.Target)
	if node == nil || node.Kind != paperlang.NodeParagraph && node.Kind != paperlang.NodeHeading && node.Kind != paperlang.NodeList {
		return PaperMutationResult{}, workspaceError("INVALID_BOX_TARGET", "box handles require a paragraph, heading, or list source node", paperedit.ErrInvalidOperation)
	}
	var value paperedit.Value
	switch {
	case request.Property.length():
		if request.Color != "" || !finiteLayoutHandle(request.Points) || request.Points < 0 || request.Points > 1_000_000 {
			return PaperMutationResult{}, workspaceError("INVALID_BOX_VALUE", "box length must be a finite non-negative point value", paperedit.ErrInvalidOperation)
		}
		value = paperedit.UnitValue(request.Points, "pt")
	case request.Property.color():
		color, ok := canonicalLayoutHandleColor(request.Color)
		if !ok || request.Points != 0 {
			return PaperMutationResult{}, workspaceError("INVALID_BOX_VALUE", "box color must be canonical #RRGGBB", paperedit.ErrInvalidOperation)
		}
		value = paperedit.StringValue(color)
	default:
		return PaperMutationResult{}, workspaceError("INVALID_BOX_PROPERTY", "box property is outside the closed handle vocabulary", paperedit.ErrInvalidOperation)
	}
	operation := paperedit.SetProperty{Target: request.Guard.Target, Name: string(request.Property), Value: value}
	return w.applyPaperMutation("set_box_property", request.Guard, opened, revision, []string{request.Guard.Target}, []paperedit.Operation{operation}, "INVALID_BOX_STYLE")
}

type PaperGridTrackProperty string

const (
	PaperGridTrackKind   PaperGridTrackProperty = "track"
	PaperGridTrackSize   PaperGridTrackProperty = "track-size"
	PaperGridTrackMin    PaperGridTrackProperty = "track-min"
	PaperGridTrackWeight PaperGridTrackProperty = "track-weight"
)

type PaperSetGridTrackRequest struct {
	Guard    PaperMutationGuard     `json:"guard"`
	Property PaperGridTrackProperty `json:"property"`
	Kind     string                 `json:"kind,omitempty"`
	Points   float64                `json:"points,omitempty"`
	Weight   uint32                 `json:"weight,omitempty"`
}

type PaperImageProperty string

const (
	PaperImageFit        PaperImageProperty = "fit"
	PaperImageFocusX     PaperImageProperty = "focus-x"
	PaperImageFocusY     PaperImageProperty = "focus-y"
	PaperImageWidth      PaperImageProperty = "width"
	PaperImageHeight     PaperImageProperty = "height"
	PaperImageMaxWidth   PaperImageProperty = "max-width"
	PaperImageMaxHeight  PaperImageProperty = "max-height"
	PaperImageAlt        PaperImageProperty = "alt"
	PaperImageDecorative PaperImageProperty = "decorative"
	PaperImageSource     PaperImageProperty = "source"
)

type PaperSetImagePropertyRequest struct {
	Guard    PaperMutationGuard `json:"guard"`
	Property PaperImageProperty `json:"property"`
	Fit      string             `json:"fit,omitempty"`
	Number   float64            `json:"number,omitempty"`
	Points   float64            `json:"points,omitempty"`
	Text     string             `json:"text,omitempty"`
	Bool     bool               `json:"bool,omitempty"`
}

// PaperSetImageProperty changes one authored image concern. Accessibility
// transitions may update alt and decorative together because publishing an
// invalid intermediate accessibility state is forbidden.
func (w *Workspace) PaperSetImageProperty(request PaperSetImagePropertyRequest) (PaperMutationResult, error) {
	opened, revision, err := w.mutationRevision(request.Guard)
	if err != nil {
		return PaperMutationResult{}, err
	}
	node := findNodeByID(revision.parsed.AST.Root, request.Guard.Target)
	if node == nil || node.Kind != paperlang.NodeImage {
		return PaperMutationResult{}, workspaceError("INVALID_IMAGE_TARGET", "image handles require an exact image source node", paperedit.ErrInvalidOperation)
	}
	operations := make([]paperedit.Operation, 0, 2)
	add := func(name string, value paperedit.Value) {
		operations = append(operations, paperedit.SetProperty{Target: request.Guard.Target, Name: name, Value: value})
	}
	switch request.Property {
	case PaperImageSource:
		if request.Fit != "" || request.Number != 0 || request.Points != 0 || request.Bool || !strings.HasPrefix(request.Text, "asset:") || len(request.Text) <= len("asset:") {
			return PaperMutationResult{}, workspaceError("INVALID_IMAGE_VALUE", "image source replacement must be one exact asset:name reference", paperedit.ErrInvalidOperation)
		}
		add("source", paperedit.StringValue(request.Text))
	case PaperImageFit:
		fit := strings.ToLower(strings.TrimSpace(request.Fit))
		if (fit != "auto" && fit != "contain" && fit != "cover") || request.Number != 0 || request.Points != 0 || request.Text != "" || request.Bool {
			return PaperMutationResult{}, workspaceError("INVALID_IMAGE_VALUE", "image fit must be auto, contain, or cover without unrelated values", paperedit.ErrInvalidOperation)
		}
		add("fit", paperedit.StringValue(fit))
	case PaperImageFocusX, PaperImageFocusY:
		if request.Fit != "" || request.Points != 0 || request.Text != "" || request.Bool || !finiteLayoutHandle(request.Number) || request.Number < 0 || request.Number > 1 {
			return PaperMutationResult{}, workspaceError("INVALID_IMAGE_VALUE", "image focus must be a finite number between 0 and 1", paperedit.ErrInvalidOperation)
		}
		add(string(request.Property), paperedit.NumberValue(request.Number))
	case PaperImageWidth, PaperImageHeight, PaperImageMaxWidth, PaperImageMaxHeight:
		if request.Fit != "" || request.Number != 0 || request.Text != "" || request.Bool || !finiteLayoutHandle(request.Points) || request.Points <= 0 || request.Points > 1_000_000 {
			return PaperMutationResult{}, workspaceError("INVALID_IMAGE_VALUE", "image dimension must be a finite positive point value", paperedit.ErrInvalidOperation)
		}
		add(string(request.Property), paperedit.UnitValue(request.Points, "pt"))
	case PaperImageAlt:
		if request.Fit != "" || request.Number != 0 || request.Points != 0 || request.Bool || !utf8.ValidString(request.Text) || len(request.Text) > w.maxMutationPayloadBytes() {
			return PaperMutationResult{}, workspaceError("INVALID_IMAGE_VALUE", "image alt text must be valid bounded UTF-8 without unrelated values", paperedit.ErrInvalidOperation)
		}
		add("alt", paperedit.StringValue(request.Text))
		if request.Text != "" && imageBoolProperty(node, "decorative") {
			add("decorative", paperedit.BoolValue(false))
		}
	case PaperImageDecorative:
		if request.Fit != "" || request.Number != 0 || request.Points != 0 || request.Text != "" {
			return PaperMutationResult{}, workspaceError("INVALID_IMAGE_VALUE", "decorative accepts only its boolean value", paperedit.ErrInvalidOperation)
		}
		add("decorative", paperedit.BoolValue(request.Bool))
		if request.Bool && imageStringProperty(node, "alt") != "" {
			add("alt", paperedit.StringValue(""))
		}
	default:
		return PaperMutationResult{}, workspaceError("INVALID_IMAGE_PROPERTY", "image property is outside the closed handle vocabulary", paperedit.ErrInvalidOperation)
	}
	return w.applyPaperMutation("set_image_property", request.Guard, opened, revision, []string{request.Guard.Target}, operations, "INVALID_IMAGE_PROPERTY_STATE")
}

func imageStringProperty(node *paperlang.Node, name string) string {
	for _, member := range node.Members {
		if member.Property != nil && member.Property.Name == name && member.Property.Value.StringValue != nil {
			return *member.Property.Value.StringValue
		}
	}
	return ""
}

func imageBoolProperty(node *paperlang.Node, name string) bool {
	for _, member := range node.Members {
		if member.Property != nil && member.Property.Name == name && member.Property.Value.BoolValue != nil {
			return *member.Property.Value.BoolValue
		}
	}
	return false
}

// PaperSetGridTrack changes one readable track property on a direct
// paragraph/heading child of a row or column. The parent is an explicit direct
// authorization effect and requires its own exact target precondition because
// changing one track can reposition every sibling.
func (w *Workspace) PaperSetGridTrack(request PaperSetGridTrackRequest) (PaperMutationResult, error) {
	opened, revision, err := w.mutationRevision(request.Guard)
	if err != nil {
		return PaperMutationResult{}, err
	}
	node, parent := sourceNodeAndParent(revision.parsed.AST.Root, request.Guard.Target)
	if node == nil || parent == nil || (node.Kind != paperlang.NodeParagraph && node.Kind != paperlang.NodeHeading) ||
		(parent.Kind != paperlang.NodeRow && parent.Kind != paperlang.NodeColumn) || parent.ID == "" {
		return PaperMutationResult{}, workspaceError("INVALID_GRID_TRACK_TARGET", "grid track handles require a readable paragraph or heading directly inside a readable row or column", paperedit.ErrInvalidOperation)
	}
	if err := requireAdditionalTargetGuard(revision, request.Guard, parent.ID); err != nil {
		return PaperMutationResult{}, err
	}
	var value paperedit.Value
	switch request.Property {
	case PaperGridTrackKind:
		kind := strings.ToLower(strings.TrimSpace(request.Kind))
		if kind != "fixed" && kind != "auto" && kind != "fraction" || request.Points != 0 || request.Weight != 0 {
			return PaperMutationResult{}, workspaceError("INVALID_GRID_TRACK_VALUE", "track kind must be fixed, auto, or fraction without unrelated values", paperedit.ErrInvalidOperation)
		}
		value = paperedit.StringValue(kind)
	case PaperGridTrackSize, PaperGridTrackMin:
		if request.Kind != "" || request.Weight != 0 || !finiteLayoutHandle(request.Points) || request.Points < 0 || request.Points > 1_000_000 {
			return PaperMutationResult{}, workspaceError("INVALID_GRID_TRACK_VALUE", "track length must be a finite non-negative point value", paperedit.ErrInvalidOperation)
		}
		value = paperedit.UnitValue(request.Points, "pt")
	case PaperGridTrackWeight:
		if request.Kind != "" || request.Points != 0 || request.Weight == 0 {
			return PaperMutationResult{}, workspaceError("INVALID_GRID_TRACK_VALUE", "track weight must be a positive 32-bit integer", paperedit.ErrInvalidOperation)
		}
		value = paperedit.NumberValue(float64(request.Weight))
	default:
		return PaperMutationResult{}, workspaceError("INVALID_GRID_TRACK_PROPERTY", "grid track property is outside the closed handle vocabulary", paperedit.ErrInvalidOperation)
	}
	operation := paperedit.SetProperty{Target: request.Guard.Target, Name: string(request.Property), Value: value}
	return w.applyPaperMutation("set_grid_track", request.Guard, opened, revision, []string{request.Guard.Target, parent.ID}, []paperedit.Operation{operation}, "INVALID_GRID_TRACK")
}

func finiteLayoutHandle(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func canonicalLayoutHandleColor(value string) (string, bool) {
	if len(value) != 7 || value[0] != '#' {
		return "", false
	}
	if _, err := strconv.ParseUint(value[1:], 16, 24); err != nil {
		return "", false
	}
	return strings.ToLower(value), true
}

func sourceNodeAndParent(root *paperlang.Node, target string) (*paperlang.Node, *paperlang.Node) {
	var found, parent *paperlang.Node
	var walk func(*paperlang.Node, *paperlang.Node)
	walk = func(node, owner *paperlang.Node) {
		if node == nil || found != nil {
			return
		}
		if node.ID == target {
			found, parent = node, owner
			return
		}
		for _, member := range node.Members {
			walk(member.Node, node)
		}
	}
	walk(root, nil)
	return found, parent
}

func requireAdditionalTargetGuard(revision *revisionRecord, guard PaperMutationGuard, target string) error {
	var matched *paperedit.TargetPrecondition
	for _, precondition := range guard.TargetPreconditions {
		if precondition.Target != target {
			continue
		}
		if matched != nil {
			return workspaceError("TRANSITIVE_PRECONDITION_INVALID", fmt.Sprintf("layout mutation declares transitive target %s more than once", target), paperedit.ErrInvalidOperation)
		}
		copy := precondition
		matched = &copy
	}
	if matched == nil || matched.ExpectedFingerprint == "" || matched.ExpectedInstance == "" {
		return workspaceError("TRANSITIVE_PRECONDITION_REQUIRED", fmt.Sprintf("layout mutation requires an exact precondition for transitive target %s", target), paperedit.ErrInvalidOperation)
	}
	actualFingerprint, err := paperedit.FingerprintNode(revision.file, revision.source, target)
	if err != nil {
		return workspaceError("TRANSITIVE_PRECONDITION_INVALID", fmt.Sprintf("layout mutation transitive target %s cannot be fingerprinted", target), paperedit.ErrInvalidOperation)
	}
	actualInstance, err := paperedit.SourceInstance(revision.file, revision.source, target)
	if err != nil {
		return workspaceError("TRANSITIVE_PRECONDITION_INVALID", fmt.Sprintf("layout mutation transitive target %s is not an exact source instance", target), paperedit.ErrInvalidOperation)
	}
	if matched.ExpectedFingerprint != actualFingerprint || matched.ExpectedInstance != actualInstance {
		return workspaceError("TRANSITIVE_PRECONDITION_CONFLICT", fmt.Sprintf("layout mutation transitive target %s changed after review", target), ErrRevisionConflict)
	}
	return nil
}
