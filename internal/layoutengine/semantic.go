// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/language"
)

const (
	SemanticMaxNodes       uint32 = 100_000
	SemanticMaxOccurrences uint32 = 1_000_000
	SemanticMaxDepth       uint32 = 4_096
	SemanticMaxStateBytes  uint64 = 1 << 20
	SemanticMaxTextBytes   uint32 = 16 << 10
)

type SemanticNodeID uint32

func (id SemanticNodeID) Valid() bool { return id != 0 }

type SemanticRole string

const (
	SemanticRoleDocument  SemanticRole = "document"
	SemanticRoleSection   SemanticRole = "section"
	SemanticRoleHeading   SemanticRole = "heading"
	SemanticRoleParagraph SemanticRole = "paragraph"
	SemanticRoleList      SemanticRole = "list"
	SemanticRoleListItem  SemanticRole = "list_item"
	SemanticRoleTable     SemanticRole = "table"
	SemanticRoleRow       SemanticRole = "row"
	SemanticRoleCell      SemanticRole = "cell"
	SemanticRoleFigure    SemanticRole = "figure"
	SemanticRoleLink      SemanticRole = "link"
	SemanticRoleArtifact  SemanticRole = "artifact"
)

func (role SemanticRole) valid() bool {
	switch role {
	case SemanticRoleDocument, SemanticRoleSection, SemanticRoleHeading, SemanticRoleParagraph,
		SemanticRoleList, SemanticRoleListItem, SemanticRoleTable, SemanticRoleRow, SemanticRoleCell,
		SemanticRoleFigure, SemanticRoleLink, SemanticRoleArtifact:
		return true
	default:
		return false
	}
}

type SemanticNode struct {
	ID         SemanticNodeID     `json:"id"`
	Parent     SemanticNodeID     `json:"parent,omitempty"`
	Role       SemanticRole       `json:"role"`
	Key        NodeKey            `json:"key"`
	Instance   InstanceID         `json:"instance"`
	Source     SourceSpan         `json:"source"`
	Attributes SemanticAttributes `json:"attributes,omitzero"`
}

// SemanticAttributes is a fixed, map-free set of optional accessibility
// metadata. It is preserved as plan data; it does not imply PDF/UA tagging.
type SemanticAttributes struct {
	Language        string        `json:"language,omitempty"`
	AlternateText   string        `json:"alternate_text,omitempty"`
	ActualText      string        `json:"actual_text,omitempty"`
	HeadingLevel    uint8         `json:"heading_level,omitempty"`
	LinkDestination DestinationID `json:"link_destination,omitempty"`
	TableHeader     bool          `json:"table_header,omitempty"`
}

type ReadingOccurrence struct {
	Semantic     SemanticNodeID `json:"semantic"`
	Page         uint32         `json:"page"`
	Fragment     FragmentID     `json:"fragment"`
	ReadingIndex uint32         `json:"reading_index"`
}

// SemanticFragmentAssociation assigns the mandatory semantic owner of one
// visual fragment. Artifact ownership is represented here but deliberately
// omitted from ReadingOccurrence.
type SemanticFragmentAssociation struct {
	Semantic SemanticNodeID `json:"semantic"`
	Page     uint32         `json:"page"`
	Fragment FragmentID     `json:"fragment"`
}

// AttachSemantics adds a complete semantic tree and reading-order ownership
// without changing geometry or display commands.
func AttachSemantics(plan LayoutPlan, nodes []SemanticNode, associations []SemanticFragmentAssociation, occurrences []ReadingOccurrence) (LayoutPlan, error) {
	if err := plan.Validate(); err != nil {
		return LayoutPlan{}, err
	}
	projection := plan.Projection()
	if len(projection.SemanticNodes) != 0 || len(projection.SemanticFragments) != 0 || len(projection.ReadingOrder) != 0 {
		return LayoutPlan{}, errors.New("layoutengine: plan already has semantic associations")
	}
	return NewLayoutPlan(LayoutPlanInput{DeterministicInputs: projection.DeterministicInputs,
		Pages: projection.Pages, Fragments: projection.Fragments, Lines: projection.Lines,
		PageRegions: projection.PageRegions, GridTracks: projection.GridTracks,
		Fonts: projection.Fonts, GlyphRuns: projection.GlyphRuns, ImageResources: projection.ImageResources,
		Images: projection.Images, Destinations: projection.Destinations, Links: projection.Links,
		Paths: projection.Paths, Transforms: projection.Transforms, Clips: projection.Clips, Fills: projection.Fills, Strokes: projection.Strokes,
		Commands: projection.Commands, Breaks: projection.Breaks, Diagnostics: projection.Diagnostics,
		SemanticNodes: nodes, SemanticFragments: associations, ReadingOrder: occurrences})
}

// ReplaceSemantics returns the same immutable geometry/display plan with a
// complete replacement semantic projection. Frontend adapters use this after
// a generic planner has finished, when authored source identities can only be
// bound at the adapter boundary. The original plan is never mutated.
func ReplaceSemantics(plan LayoutPlan, nodes []SemanticNode, associations []SemanticFragmentAssociation, occurrences []ReadingOccurrence) (LayoutPlan, error) {
	if err := plan.Validate(); err != nil {
		return LayoutPlan{}, err
	}
	projection := plan.Projection()
	return NewLayoutPlan(LayoutPlanInput{DeterministicInputs: projection.DeterministicInputs,
		Pages: projection.Pages, Fragments: projection.Fragments, Lines: projection.Lines,
		PageRegions: projection.PageRegions, GridTracks: projection.GridTracks,
		Fonts: projection.Fonts, GlyphRuns: projection.GlyphRuns, ImageResources: projection.ImageResources,
		Images: projection.Images, Destinations: projection.Destinations, Links: projection.Links,
		Paths: projection.Paths, Transforms: projection.Transforms, Clips: projection.Clips, Fills: projection.Fills, Strokes: projection.Strokes,
		Commands: projection.Commands, Breaks: projection.Breaks, Diagnostics: projection.Diagnostics,
		SemanticNodes: nodes, SemanticFragments: associations, ReadingOrder: occurrences})
}

func validateSemantics(nodes []SemanticNode, associations []SemanticFragmentAssociation, occurrences []ReadingOccurrence, pages []PlannedPage, fragments map[FragmentID]Fragment, destinations []PlannedDestination, links []PlannedLink) error {
	if len(nodes) == 0 {
		if len(associations) != 0 || len(occurrences) != 0 {
			return planError("semantic_fragments", "associations exist without a semantic tree")
		}
		return nil
	}
	if uint64(len(nodes)) > uint64(SemanticMaxNodes) || uint64(len(associations)) > uint64(SemanticMaxOccurrences) || uint64(len(occurrences)) > uint64(SemanticMaxOccurrences) {
		return planError("semantic_nodes", "semantic state exceeds implementation limits")
	}
	stateBytes := uint64(len(associations))*32 + uint64(len(occurrences))*32
	for _, node := range nodes {
		cost := uint64(128 + len(node.Key) + len(node.Instance) + len(node.Source.File) +
			len(node.Attributes.Language) + len(node.Attributes.AlternateText) + len(node.Attributes.ActualText))
		if stateBytes > SemanticMaxStateBytes || cost > SemanticMaxStateBytes-stateBytes {
			return planError("semantic_nodes", "semantic state exceeds its byte limit")
		}
		stateBytes += cost
	}
	roots := 0
	identities := make(map[struct {
		key      NodeKey
		instance InstanceID
	}]SemanticNodeID, len(nodes))
	for index, node := range nodes {
		if node.ID != SemanticNodeID(index+1) {
			return planIndexedError("semantic_nodes", index, ".id", "semantic IDs are not consecutive and one-based")
		}
		if !node.Role.valid() {
			return planIndexedError("semantic_nodes", index, ".role", "is not a supported semantic role")
		}
		if err := validateTextIdentity("semantic node key", string(node.Key)); err != nil {
			return planIndexedError("semantic_nodes", index, ".key", err.Error())
		}
		if err := validateTextIdentity("semantic node instance", string(node.Instance)); err != nil {
			return planIndexedError("semantic_nodes", index, ".instance", err.Error())
		}
		if err := node.Source.Validate(); err != nil {
			return planIndexedError("semantic_nodes", index, ".source", err.Error())
		}
		if err := validateSemanticAttributes(node, len(destinations)); err != nil {
			return planIndexedError("semantic_nodes", index, ".attributes", err.Error())
		}
		identity := struct {
			key      NodeKey
			instance InstanceID
		}{node.Key, node.Instance}
		if _, duplicate := identities[identity]; duplicate {
			return planIndexedError("semantic_nodes", index, "", "duplicates a semantic key and instance")
		}
		identities[identity] = node.ID
		if !node.Parent.Valid() {
			roots++
		} else if uint64(node.Parent) > uint64(len(nodes)) || node.Parent == node.ID {
			return planIndexedError("semantic_nodes", index, ".parent", "references a missing node or itself")
		}
	}
	if roots != 1 {
		return planError("semantic_nodes", "must contain exactly one root")
	}
	for _, node := range nodes {
		if !node.Parent.Valid() && node.Role != SemanticRoleDocument {
			return planError(fmt.Sprintf("semantic_nodes[%d].role", node.ID-1), "root role must be document")
		}
	}
	colors := make([]uint8, len(nodes))
	var visit func(SemanticNodeID, uint32) error
	visit = func(id SemanticNodeID, depth uint32) error {
		if depth > SemanticMaxDepth {
			return planError("semantic_nodes", "tree exceeds maximum depth")
		}
		index := int(id - 1)
		if colors[index] == 1 {
			return planError(fmt.Sprintf("semantic_nodes[%d].parent", index), "forms a cycle")
		}
		if colors[index] == 2 {
			return nil
		}
		colors[index] = 1
		if parent := nodes[index].Parent; parent.Valid() {
			if err := visit(parent, depth+1); err != nil {
				return err
			}
		}
		colors[index] = 2
		return nil
	}
	for _, node := range nodes {
		if err := visit(node.ID, 1); err != nil {
			return err
		}
	}
	owned := make(map[FragmentID]SemanticNodeID, len(associations))
	for index, association := range associations {
		if !association.Semantic.Valid() || uint64(association.Semantic) > uint64(len(nodes)) {
			return planIndexedError("semantic_fragments", index, ".semantic", "references a missing semantic node")
		}
		fragment, exists := fragments[association.Fragment]
		if !exists {
			return planIndexedError("semantic_fragments", index, ".fragment", "references a missing fragment")
		}
		if association.Page == 0 || uint64(association.Page) > uint64(len(pages)) || fragment.Page != association.Page {
			return planIndexedError("semantic_fragments", index, ".page", "does not match the fragment page")
		}
		if _, duplicate := owned[association.Fragment]; duplicate {
			return planIndexedError("semantic_fragments", index, ".fragment", "has duplicate semantic ownership")
		}
		node := nodes[association.Semantic-1]
		if node.Key != fragment.Key || node.Instance != fragment.Instance || node.Source != fragment.Source {
			return planIndexedError("semantic_fragments", index, "", "semantic provenance does not match the owned fragment")
		}
		owned[association.Fragment] = association.Semantic
	}
	for fragment := range fragments {
		if !owned[fragment].Valid() {
			return planError("semantic_fragments", fmt.Sprintf("fragment %d has no semantic owner", fragment))
		}
	}
	linkDestinationsByFragment := make(map[FragmentID]map[DestinationID]struct{}, len(links))
	for _, link := range links {
		if link.Destination.Valid() {
			targets := linkDestinationsByFragment[link.Fragment]
			if targets == nil {
				targets = make(map[DestinationID]struct{})
				linkDestinationsByFragment[link.Fragment] = targets
			}
			targets[link.Destination] = struct{}{}
		}
	}
	for _, node := range nodes {
		if !node.Attributes.LinkDestination.Valid() {
			continue
		}
		matched := false
		for fragment, semantic := range owned {
			_, ownsTarget := linkDestinationsByFragment[fragment][node.Attributes.LinkDestination]
			if semantic == node.ID && ownsTarget {
				matched = true
				break
			}
		}
		if !matched {
			return planError(fmt.Sprintf("semantic_nodes[%d].attributes.link_destination", node.ID-1), "does not match an owned planned link")
		}
	}
	read := make(map[FragmentID]bool, len(occurrences))
	var previousPage, nextIndex uint32
	for index, occurrence := range occurrences {
		if !occurrence.Semantic.Valid() || uint64(occurrence.Semantic) > uint64(len(nodes)) {
			return planIndexedError("reading_order", index, ".semantic", "references a missing semantic node")
		}
		fragment, exists := fragments[occurrence.Fragment]
		if !exists {
			return planIndexedError("reading_order", index, ".fragment", "references a missing fragment")
		}
		if occurrence.Page == 0 || uint64(occurrence.Page) > uint64(len(pages)) || fragment.Page != occurrence.Page {
			return planIndexedError("reading_order", index, ".page", "does not match the fragment page")
		}
		owner := owned[occurrence.Fragment]
		if !owner.Valid() || owner != occurrence.Semantic {
			return planIndexedError("reading_order", index, "", "does not match the fragment semantic owner")
		}
		node := nodes[occurrence.Semantic-1]
		if node.Role == SemanticRoleArtifact {
			return planIndexedError("reading_order", index, "", "artifact fragments must not appear in reading order")
		}
		if read[occurrence.Fragment] {
			return planIndexedError("reading_order", index, ".fragment", "appears more than once in reading order")
		}
		if occurrence.Page < previousPage {
			return planIndexedError("reading_order", index, "", "is not in canonical page order")
		}
		if occurrence.Page != previousPage {
			previousPage, nextIndex = occurrence.Page, 0
		}
		if occurrence.ReadingIndex != nextIndex {
			return planIndexedError("reading_order", index, ".reading_index", "is not consecutive within the page")
		}
		nextIndex++
		read[occurrence.Fragment] = true
	}
	for fragment, semantic := range owned {
		if nodes[semantic-1].Role != SemanticRoleArtifact && !read[fragment] {
			return planError("reading_order", fmt.Sprintf("non-artifact fragment %d is missing", fragment))
		}
	}
	return nil
}

func validateSemanticAttributes(node SemanticNode, destinationCount int) error {
	attributes := node.Attributes
	if attributes.Language != "" {
		if err := validateSemanticLanguage(attributes.Language); err != nil {
			return err
		}
		if node.Role == SemanticRoleArtifact {
			return errors.New("artifact nodes cannot declare language")
		}
	}
	if attributes.AlternateText != "" {
		if node.Role != SemanticRoleFigure {
			return errors.New("alternate text is only valid on figure nodes")
		}
		if err := validateSemanticText("alternate text", attributes.AlternateText); err != nil {
			return err
		}
	}
	if attributes.ActualText != "" {
		switch node.Role {
		case SemanticRoleHeading, SemanticRoleParagraph, SemanticRoleListItem, SemanticRoleCell, SemanticRoleFigure, SemanticRoleLink:
		default:
			return errors.New("actual text is not valid on this semantic role")
		}
		if err := validateSemanticText("actual text", attributes.ActualText); err != nil {
			return err
		}
	}
	if attributes.HeadingLevel != 0 {
		if node.Role != SemanticRoleHeading || attributes.HeadingLevel > 6 {
			return errors.New("heading level must be between 1 and 6 on a heading node")
		}
	}
	if attributes.LinkDestination.Valid() {
		if node.Role != SemanticRoleLink || uint64(attributes.LinkDestination) > uint64(destinationCount) {
			return errors.New("link destination must reference an existing destination from a link node")
		}
	}
	if attributes.TableHeader && node.Role != SemanticRoleCell {
		return errors.New("table header is only valid on a cell node")
	}
	return nil
}

func validateSemanticLanguage(value string) error {
	tag, err := language.Parse(value)
	if err != nil || tag.String() != value || len(value) > 255 {
		return errors.New("language is not a canonical BCP 47 tag")
	}
	return nil
}

func validateSemanticText(name, value string) error {
	if len(value) > int(SemanticMaxTextBytes) || !utf8.ValidString(value) || strings.TrimSpace(value) != value {
		return fmt.Errorf("%s is not canonical UTF-8 or exceeds %d bytes", name, SemanticMaxTextBytes)
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return fmt.Errorf("%s contains a control character", name)
		}
	}
	return nil
}
