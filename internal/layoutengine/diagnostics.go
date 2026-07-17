// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

// DiagnosticCode is a stable, machine-readable failure or warning code.
type DiagnosticCode string

const (
	DiagnosticUnbreakableTooTall               DiagnosticCode = "UNBREAKABLE_TOO_TALL"
	DiagnosticParagraphConstraintRelaxed       DiagnosticCode = "PARAGRAPH_CONSTRAINT_RELAXED"
	DiagnosticParagraphConstraintUnsatisfiable DiagnosticCode = "PARAGRAPH_CONSTRAINT_UNSATISFIABLE"
	DiagnosticPageRegionNoBodySpace            DiagnosticCode = "PAGE_REGION_NO_BODY_SPACE"
	DiagnosticPageMasterRegionEmpty            DiagnosticCode = "PAGE_MASTER_REGION_EMPTY"
	DiagnosticPageMasterRegionInvalid          DiagnosticCode = "PAGE_MASTER_REGION_INVALID"
	DiagnosticPageMasterRegionOverlap          DiagnosticCode = "PAGE_MASTER_REGION_OVERLAP"
	DiagnosticKeepTooLarge                     DiagnosticCode = "KEEP_TOO_LARGE"
	DiagnosticRepeatedHeaderTooTall            DiagnosticCode = "REPEATED_HEADER_TOO_TALL"
	DiagnosticTableSpanInvalid                 DiagnosticCode = "TABLE_SPAN_INVALID"
	DiagnosticTableRowspanCrossesPage          DiagnosticCode = "TABLE_ROWSPAN_CROSSES_PAGE"
	DiagnosticTrackMinOverflow                 DiagnosticCode = "TRACK_MIN_OVERFLOW"
	DiagnosticConstraintCycle                  DiagnosticCode = "CONSTRAINT_CYCLE"
	DiagnosticConstraintOverdetermined         DiagnosticCode = "CONSTRAINT_OVERDETERMINED"
	DiagnosticReferenceLayoutUnstable          DiagnosticCode = "REFERENCE_LAYOUT_UNSTABLE"
	DiagnosticStackChildOverflow               DiagnosticCode = "STACK_CHILD_OVERFLOW"
	DiagnosticCanvasNodeOverflow               DiagnosticCode = "CANVAS_NODE_OVERFLOW"
	DiagnosticFontMissing                      DiagnosticCode = "FONT_MISSING"
	DiagnosticGlyphMissing                     DiagnosticCode = "GLYPH_MISSING"
	DiagnosticImageMissing                     DiagnosticCode = "IMAGE_MISSING"
	DiagnosticImageDimensionInvalid            DiagnosticCode = "IMAGE_DIMENSION_INVALID"
	DiagnosticImageFitInvalid                  DiagnosticCode = "IMAGE_FIT_INVALID"
	DiagnosticResourceLimit                    DiagnosticCode = "RESOURCE_LIMIT"
	DiagnosticWorkLimit                        DiagnosticCode = "WORK_LIMIT"
	DiagnosticCanceled                         DiagnosticCode = "CANCELED"
	DiagnosticPainterResourceMismatch          DiagnosticCode = "PAINTER_RESOURCE_MISMATCH"
)

func (c DiagnosticCode) validate() error {
	if c == "" {
		return errors.New("layoutengine: diagnostic code is empty")
	}
	for _, r := range c {
		if (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_' {
			return fmt.Errorf("layoutengine: diagnostic code %q is not canonical", c)
		}
	}
	return nil
}

// DiagnosticSeverity controls whether a diagnostic is informational, a
// recoverable warning, or an error that prevents publication.
type DiagnosticSeverity string

const (
	SeverityInfo    DiagnosticSeverity = "info"
	SeverityWarning DiagnosticSeverity = "warning"
	SeverityError   DiagnosticSeverity = "error"
)

func (s DiagnosticSeverity) valid() bool {
	return s == SeverityInfo || s == SeverityWarning || s == SeverityError
}

// PipelineStage identifies the subsystem that produced a diagnostic.
type PipelineStage string

const (
	StageParse     PipelineStage = "parse"
	StageTypecheck PipelineStage = "typecheck"
	StageLower     PipelineStage = "lower"
	StagePreflight PipelineStage = "preflight"
	StageLayout    PipelineStage = "layout"
	StagePaint     PipelineStage = "paint"
)

func (s PipelineStage) valid() bool {
	switch s {
	case StageParse, StageTypecheck, StageLower, StagePreflight, StageLayout, StagePaint:
		return true
	default:
		return false
	}
}

// DiagnosticLocation connects a diagnostic to semantic source and positioned
// output. Page is one-based; zero means not yet positioned.
type DiagnosticLocation struct {
	Node      NodeID     `json:"node,omitempty"`
	Key       NodeKey    `json:"key,omitempty"`
	Source    SourceSpan `json:"source"`
	Instance  InstanceID `json:"instance,omitempty"`
	Fragment  FragmentID `json:"fragment,omitempty"`
	Scenario  string     `json:"scenario,omitempty"`
	Page      uint32     `json:"page,omitempty"`
	Region    RegionID   `json:"region,omitempty"`
	Bounds    Rect       `json:"bounds"`
	HasBounds bool       `json:"has_bounds,omitempty"`
}

func (l DiagnosticLocation) validate() error {
	if l.Key != "" {
		if err := validateTextIdentity("diagnostic node key", string(l.Key)); err != nil {
			return err
		}
	}
	if l.Instance != "" {
		if err := validateTextIdentity("diagnostic instance ID", string(l.Instance)); err != nil {
			return err
		}
	}
	if l.Region != "" && !l.Region.Valid() {
		return errors.New("layoutengine: diagnostic region is invalid")
	}
	if err := l.Source.Validate(); err != nil {
		return err
	}
	if l.Scenario != "" {
		if strings.TrimSpace(l.Scenario) == "" {
			return errors.New("layoutengine: diagnostic scenario is only whitespace")
		}
		if !utf8.ValidString(l.Scenario) {
			return errors.New("layoutengine: diagnostic scenario is not valid UTF-8")
		}
	}
	if l.HasBounds {
		if err := l.Bounds.Validate(); err != nil {
			return fmt.Errorf("layoutengine: diagnostic bounds: %w", err)
		}
	} else if l.Bounds != (Rect{}) {
		return errors.New("layoutengine: diagnostic bounds are present without has_bounds")
	}
	return nil
}

// DiagnosticEvidence is one bounded, machine-readable fact supporting a
// diagnostic. Values are serialized strings with semantics determined by Key.
type DiagnosticEvidence struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// DiagnosticReference points to a related diagnostic without recursively
// embedding it.
type DiagnosticReference struct {
	Code     DiagnosticCode     `json:"code"`
	Location DiagnosticLocation `json:"location"`
}

// DiagnosticFixKind identifies an editor operation that may safely remedy a
// diagnostic. The operation remains separate from the human-readable message.
type DiagnosticFixKind string

const (
	FixSetProperty    DiagnosticFixKind = "set_property"
	FixRemoveNode     DiagnosticFixKind = "remove_node"
	FixDisableFeature DiagnosticFixKind = "disable_feature"
)

func (k DiagnosticFixKind) valid() bool {
	return k == FixSetProperty || k == FixRemoveNode || k == FixDisableFeature
}

// DiagnosticFix is a typed remedy proposal. Property and Value are used by
// property-oriented fixes and remain empty for node removal.
type DiagnosticFix struct {
	Kind     DiagnosticFixKind `json:"kind"`
	Target   NodeKey           `json:"target"`
	Property string            `json:"property,omitempty"`
	Value    string            `json:"value,omitempty"`
}

func (f DiagnosticFix) validate() error {
	if !f.Kind.valid() {
		return fmt.Errorf("layoutengine: invalid diagnostic fix kind %q", f.Kind)
	}
	if err := validateTextIdentity("diagnostic fix target", string(f.Target)); err != nil {
		return err
	}
	if (f.Kind == FixSetProperty || f.Kind == FixDisableFeature) && f.Property == "" {
		return errors.New("layoutengine: property diagnostic fix has no property")
	}
	if !utf8.ValidString(f.Property) || !utf8.ValidString(f.Value) {
		return errors.New("layoutengine: diagnostic fix contains invalid UTF-8")
	}
	if f.Kind == FixRemoveNode && (f.Property != "" || f.Value != "") {
		return errors.New("layoutengine: remove-node diagnostic fix has property data")
	}
	return nil
}

// Diagnostic is the structured diagnostic unit retained by a LayoutPlan.
type Diagnostic struct {
	Code     DiagnosticCode        `json:"code"`
	Severity DiagnosticSeverity    `json:"severity"`
	Stage    PipelineStage         `json:"stage"`
	Message  string                `json:"message"`
	Location DiagnosticLocation    `json:"location"`
	Evidence []DiagnosticEvidence  `json:"evidence,omitempty"`
	Related  []DiagnosticReference `json:"related,omitempty"`
	Fixes    []DiagnosticFix       `json:"fixes,omitempty"`
}

// Validate checks the stable diagnostic schema without interpreting layout
// policy.
func (d Diagnostic) Validate() error {
	if err := d.Code.validate(); err != nil {
		return err
	}
	if !d.Severity.valid() {
		return fmt.Errorf("layoutengine: invalid diagnostic severity %q", d.Severity)
	}
	if !d.Stage.valid() {
		return fmt.Errorf("layoutengine: invalid diagnostic stage %q", d.Stage)
	}
	if strings.TrimSpace(d.Message) == "" {
		return errors.New("layoutengine: diagnostic message is empty")
	}
	if !utf8.ValidString(d.Message) {
		return errors.New("layoutengine: diagnostic message is not valid UTF-8")
	}
	if err := d.Location.validate(); err != nil {
		return err
	}
	seenEvidence := make(map[string]struct{}, len(d.Evidence))
	for i, evidence := range d.Evidence {
		if evidence.Key == "" {
			return fmt.Errorf("layoutengine: diagnostic evidence %d has no key", i)
		}
		if !utf8.ValidString(evidence.Key) || !utf8.ValidString(evidence.Value) {
			return fmt.Errorf("layoutengine: diagnostic evidence %d contains invalid UTF-8", i)
		}
		if _, exists := seenEvidence[evidence.Key]; exists {
			return fmt.Errorf("layoutengine: duplicate diagnostic evidence key %q", evidence.Key)
		}
		seenEvidence[evidence.Key] = struct{}{}
	}
	for i, related := range d.Related {
		if err := related.Code.validate(); err != nil {
			return fmt.Errorf("layoutengine: related diagnostic %d: %w", i, err)
		}
		if err := related.Location.validate(); err != nil {
			return fmt.Errorf("layoutengine: related diagnostic %d: %w", i, err)
		}
	}
	for i, fix := range d.Fixes {
		if err := fix.validate(); err != nil {
			return fmt.Errorf("layoutengine: diagnostic fix %d: %w", i, err)
		}
	}
	return nil
}

func cloneDiagnostic(d Diagnostic) Diagnostic {
	d.Evidence = append([]DiagnosticEvidence(nil), d.Evidence...)
	d.Related = append([]DiagnosticReference(nil), d.Related...)
	d.Fixes = append([]DiagnosticFix(nil), d.Fixes...)
	// Evidence is a uniquely keyed fact set rather than an ordered narrative,
	// so canonical projections sort it by key. Related diagnostics and fixes
	// retain their supplied order because it can carry causal/editor intent.
	sort.Slice(d.Evidence, func(i, j int) bool {
		return d.Evidence[i].Key < d.Evidence[j].Key
	})
	return d
}
