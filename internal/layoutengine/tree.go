// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
)

const CanonicalTreeSchemaVersion uint16 = 1

var (
	ErrCanonicalTreeInvalid   = errors.New("layoutengine: canonical tree is invalid")
	ErrCanonicalTreeLimit     = errors.New("layoutengine: canonical tree limit exceeded")
	ErrCanonicalTreeCollision = errors.New("layoutengine: canonical tree identity collision")
)

type TreeNodeIndex uint32 // zero-based arena position; never a persistent identity
type TreeStringID uint32  // one-based intern-table reference
type TreeStyleID uint32
type TreeTrackID uint32
type TreeResourceID uint32
type TreeSemanticID uint32

type TreeLengthKind string

const (
	TreeLengthAuto     TreeLengthKind = "auto"
	TreeLengthFixed    TreeLengthKind = "fixed"
	TreeLengthPercent  TreeLengthKind = "percent"
	TreeLengthFraction TreeLengthKind = "fraction"
)

// TreeLength preserves unresolved constraint-dependent units. Value is fixed
// point for fixed lengths and a 1/1024 ratio for percent/fraction values.
type TreeLength struct {
	Kind  TreeLengthKind `json:"kind"`
	Value Fixed          `json:"value,omitempty"`
}

type TreeStyle struct {
	FontFamily TreeStringID  `json:"font_family,omitempty"`
	Align      TreeStringID  `json:"align,omitempty"`
	FontSize   Fixed         `json:"font_size,omitempty"`
	LineHeight Fixed         `json:"line_height,omitempty"`
	Margin     [4]TreeLength `json:"margin"`
	Flags      uint16        `json:"flags,omitempty"`
}

type TreeTrack struct {
	Name TreeStringID `json:"name,omitempty"`
	Min  TreeLength   `json:"min"`
	Max  TreeLength   `json:"max"`
}

type TreeResource struct {
	Kind   TreeStringID `json:"kind"`
	Key    TreeStringID `json:"key"`
	Digest TreeStringID `json:"digest"`
}

type TreeSemantic struct {
	Role  SemanticRole `json:"role"`
	Label TreeStringID `json:"label,omitempty"`
}

// TreeNode is map-free hot-path state. Child indexes live in one shared dense
// edge table so direct children remain compact even when subtrees vary.
type TreeNode struct {
	ID         NodeID         `json:"id"`
	Key        NodeKey        `json:"key"`
	Kind       TreeStringID   `json:"kind"`
	Style      TreeStyleID    `json:"style,omitempty"`
	Track      TreeTrackID    `json:"track,omitempty"`
	Resource   TreeResourceID `json:"resource,omitempty"`
	Semantic   TreeSemanticID `json:"semantic,omitempty"`
	Text       TreeStringID   `json:"text,omitempty"`
	ChildStart uint32         `json:"child_start"`
	ChildCount uint32         `json:"child_count"`
	Source     SourceSpan     `json:"source"`
	Flags      uint32         `json:"flags,omitempty"`
}

type TreeStyleInput struct {
	FontFamily string
	Align      string
	FontSize   Fixed
	LineHeight Fixed
	Margin     [4]TreeLength
	Flags      uint16
}

type TreeTrackInput struct {
	Name     string
	Min, Max TreeLength
}
type TreeResourceInput struct{ Kind, Key, Digest string }
type TreeSemanticInput struct {
	Role  SemanticRole
	Label string
}

type TreeNodeInput struct {
	ID       NodeID
	Key      NodeKey
	Kind     string
	Parent   int64 // -1 is a root; otherwise a prior zero-based dense index
	Text     string
	Style    *TreeStyleInput
	Track    *TreeTrackInput
	Resource *TreeResourceInput
	Semantic *TreeSemanticInput
	Source   SourceSpan
	Flags    uint32
}

type CanonicalTreeInput struct{ Nodes []TreeNodeInput }

type CanonicalTreeLimits struct {
	MaxNodes, MaxEdges, MaxStrings, MaxStyles, MaxTracks, MaxResources, MaxSemantics uint32
	MaxStringBytes, MaxWork                                                          uint64
}

func DefaultCanonicalTreeLimits() CanonicalTreeLimits {
	return CanonicalTreeLimits{MaxNodes: 1 << 20, MaxEdges: 1 << 20, MaxStrings: 1 << 20,
		MaxStyles: 1 << 18, MaxTracks: 1 << 18, MaxResources: 1 << 18, MaxSemantics: 1 << 18,
		MaxStringBytes: 32 << 20, MaxWork: 16 << 20}
}

func normalizeCanonicalTreeLimits(l CanonicalTreeLimits) (CanonicalTreeLimits, error) {
	if l == (CanonicalTreeLimits{}) {
		return DefaultCanonicalTreeLimits(), nil
	}
	h := DefaultCanonicalTreeLimits()
	if l.MaxNodes == 0 || l.MaxEdges == 0 || l.MaxStrings == 0 || l.MaxStyles == 0 || l.MaxTracks == 0 ||
		l.MaxResources == 0 || l.MaxSemantics == 0 || l.MaxStringBytes == 0 || l.MaxWork == 0 ||
		l.MaxNodes > h.MaxNodes || l.MaxEdges > h.MaxEdges || l.MaxStrings > h.MaxStrings || l.MaxStyles > h.MaxStyles ||
		l.MaxTracks > h.MaxTracks || l.MaxResources > h.MaxResources || l.MaxSemantics > h.MaxSemantics ||
		l.MaxStringBytes > h.MaxStringBytes || l.MaxWork > h.MaxWork {
		return CanonicalTreeLimits{}, ErrCanonicalTreeLimit
	}
	return l, nil
}

type CanonicalTree struct {
	nodes     []TreeNode
	children  []TreeNodeIndex
	strings   []string
	styles    []TreeStyle
	tracks    []TreeTrack
	resources []TreeResource
	semantics []TreeSemantic
}

type CanonicalTreeProjection struct {
	SchemaVersion uint16          `json:"schema_version"`
	Nodes         []TreeNode      `json:"nodes"`
	Children      []TreeNodeIndex `json:"children,omitempty"`
	Strings       []string        `json:"strings,omitempty"`
	Styles        []TreeStyle     `json:"styles,omitempty"`
	Tracks        []TreeTrack     `json:"tracks,omitempty"`
	Resources     []TreeResource  `json:"resources,omitempty"`
	Semantics     []TreeSemantic  `json:"semantics,omitempty"`
}

type treeBuilder struct {
	ctx               context.Context
	limits            CanonicalTreeLimits
	work, stringBytes uint64
	tree              CanonicalTree
	strings           map[string]TreeStringID
	styles            map[TreeStyle]TreeStyleID
	tracks            map[TreeTrack]TreeTrackID
	resources         map[TreeResource]TreeResourceID
	resourceKeys      map[string]TreeResource
	semantics         map[TreeSemantic]TreeSemanticID
	nodeIDs           map[NodeID]struct{}
	nodeKeys          map[NodeKey]struct{}
	childLists        [][]TreeNodeIndex
}

func NewCanonicalTree(ctx context.Context, input CanonicalTreeInput, limits CanonicalTreeLimits) (CanonicalTree, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	limits, err := normalizeCanonicalTreeLimits(limits)
	if err != nil {
		return CanonicalTree{}, err
	}
	if uint64(len(input.Nodes)) > uint64(limits.MaxNodes) || uint64(len(input.Nodes)) > math.MaxUint32 {
		return CanonicalTree{}, ErrCanonicalTreeLimit
	}
	b := treeBuilder{ctx: ctx, limits: limits, strings: map[string]TreeStringID{}, styles: map[TreeStyle]TreeStyleID{},
		tracks: map[TreeTrack]TreeTrackID{}, resources: map[TreeResource]TreeResourceID{}, resourceKeys: map[string]TreeResource{},
		semantics: map[TreeSemantic]TreeSemanticID{}, nodeIDs: map[NodeID]struct{}{}, nodeKeys: map[NodeKey]struct{}{},
		childLists: make([][]TreeNodeIndex, len(input.Nodes))}
	for index, spec := range input.Nodes {
		if err := b.charge(1); err != nil {
			return CanonicalTree{}, err
		}
		node, err := b.node(spec, index)
		if err != nil {
			return CanonicalTree{}, fmt.Errorf("node[%d]: %w", index, err)
		}
		b.tree.nodes = append(b.tree.nodes, node)
		if spec.Parent >= 0 {
			p := int(spec.Parent)
			if p >= index {
				return CanonicalTree{}, fmt.Errorf("node[%d]: %w: parent must precede child", index, ErrCanonicalTreeInvalid)
			}
			b.childLists[p] = append(b.childLists[p], TreeNodeIndex(index))
		}
	}
	for index := range b.tree.nodes {
		list := b.childLists[index]
		if uint64(len(b.tree.children))+uint64(len(list)) > uint64(limits.MaxEdges) {
			return CanonicalTree{}, ErrCanonicalTreeLimit
		}
		b.tree.nodes[index].ChildStart = uint32(len(b.tree.children))
		b.tree.nodes[index].ChildCount = uint32(len(list))
		b.tree.children = append(b.tree.children, list...)
	}
	if err := validateCanonicalTree(b.tree); err != nil {
		return CanonicalTree{}, err
	}
	return b.tree, nil
}

func (b *treeBuilder) charge(n uint64) error {
	if err := ChargePlanningWork(b.ctx, "canonical tree lowering", n); err != nil {
		return err
	}
	if n > b.limits.MaxWork-b.work {
		return ErrCanonicalTreeLimit
	}
	b.work += n
	return nil
}

func (b *treeBuilder) intern(value string) (TreeStringID, error) {
	if value == "" {
		return 0, nil
	}
	if id := b.strings[value]; id != 0 {
		return id, nil
	}
	if err := validateTextIdentity("canonical tree string", value); err != nil {
		return 0, err
	}
	if uint64(len(b.tree.strings)) >= uint64(b.limits.MaxStrings) || uint64(len(value)) > b.limits.MaxStringBytes-b.stringBytes {
		return 0, ErrCanonicalTreeLimit
	}
	b.stringBytes += uint64(len(value))
	b.tree.strings = append(b.tree.strings, value)
	id := TreeStringID(len(b.tree.strings))
	b.strings[value] = id
	return id, b.charge(1)
}

func (b *treeBuilder) node(s TreeNodeInput, index int) (TreeNode, error) {
	if !s.ID.Valid() || s.Key == "" {
		return TreeNode{}, ErrCanonicalTreeInvalid
	}
	if _, ok := b.nodeIDs[s.ID]; ok {
		return TreeNode{}, ErrCanonicalTreeCollision
	}
	b.nodeIDs[s.ID] = struct{}{}
	if _, ok := b.nodeKeys[s.Key]; ok {
		return TreeNode{}, ErrCanonicalTreeCollision
	}
	b.nodeKeys[s.Key] = struct{}{}
	if err := validateTextIdentity("canonical tree node key", string(s.Key)); err != nil {
		return TreeNode{}, err
	}
	if s.Parent < -1 || s.Parent >= int64(index) {
		return TreeNode{}, ErrCanonicalTreeInvalid
	}
	if err := s.Source.Validate(); err != nil {
		return TreeNode{}, err
	}
	kind, err := b.intern(s.Kind)
	if err != nil {
		return TreeNode{}, err
	}
	if kind == 0 {
		return TreeNode{}, ErrCanonicalTreeInvalid
	}
	text, err := b.intern(s.Text)
	if err != nil {
		return TreeNode{}, err
	}
	n := TreeNode{ID: s.ID, Key: s.Key, Kind: kind, Text: text, Source: s.Source, Flags: s.Flags}
	if s.Style != nil {
		v, err := b.style(*s.Style)
		if err != nil {
			return TreeNode{}, err
		}
		n.Style = v
	}
	if s.Track != nil {
		v, err := b.track(*s.Track)
		if err != nil {
			return TreeNode{}, err
		}
		n.Track = v
	}
	if s.Resource != nil {
		v, err := b.resource(*s.Resource)
		if err != nil {
			return TreeNode{}, err
		}
		n.Resource = v
	}
	if s.Semantic != nil {
		v, err := b.semantic(*s.Semantic)
		if err != nil {
			return TreeNode{}, err
		}
		n.Semantic = v
	}
	return n, nil
}

func validTreeLength(v TreeLength) bool {
	return v.Kind == TreeLengthAuto && v.Value == 0 || (v.Kind == TreeLengthFixed || v.Kind == TreeLengthPercent || v.Kind == TreeLengthFraction) && v.Value >= 0
}
func (b *treeBuilder) style(in TreeStyleInput) (TreeStyleID, error) {
	f, e := b.intern(in.FontFamily)
	if e != nil {
		return 0, e
	}
	a, e := b.intern(in.Align)
	if e != nil {
		return 0, e
	}
	for _, v := range in.Margin {
		if !validTreeLength(v) {
			return 0, ErrCanonicalTreeInvalid
		}
	}
	s := TreeStyle{f, a, in.FontSize, in.LineHeight, in.Margin, in.Flags}
	if id := b.styles[s]; id != 0 {
		return id, nil
	}
	if uint32(len(b.tree.styles)) >= b.limits.MaxStyles {
		return 0, ErrCanonicalTreeLimit
	}
	b.tree.styles = append(b.tree.styles, s)
	id := TreeStyleID(len(b.tree.styles))
	b.styles[s] = id
	return id, b.charge(1)
}
func (b *treeBuilder) track(in TreeTrackInput) (TreeTrackID, error) {
	n, e := b.intern(in.Name)
	if e != nil {
		return 0, e
	}
	if !validTreeLength(in.Min) || !validTreeLength(in.Max) {
		return 0, ErrCanonicalTreeInvalid
	}
	v := TreeTrack{Name: n, Min: in.Min, Max: in.Max}
	if id := b.tracks[v]; id != 0 {
		return id, nil
	}
	if uint32(len(b.tree.tracks)) >= b.limits.MaxTracks {
		return 0, ErrCanonicalTreeLimit
	}
	b.tree.tracks = append(b.tree.tracks, v)
	id := TreeTrackID(len(b.tree.tracks))
	b.tracks[v] = id
	return id, b.charge(1)
}
func (b *treeBuilder) resource(in TreeResourceInput) (TreeResourceID, error) {
	k, e := b.intern(in.Kind)
	if e != nil {
		return 0, e
	}
	key, e := b.intern(in.Key)
	if e != nil {
		return 0, e
	}
	d, e := b.intern(in.Digest)
	if e != nil {
		return 0, e
	}
	if k == 0 || key == 0 || d == 0 {
		return 0, ErrCanonicalTreeInvalid
	}
	v := TreeResource{Kind: k, Key: key, Digest: d}
	if prior, ok := b.resourceKeys[in.Key]; ok && prior != v {
		return 0, ErrCanonicalTreeCollision
	}
	b.resourceKeys[in.Key] = v
	if id := b.resources[v]; id != 0 {
		return id, nil
	}
	if uint32(len(b.tree.resources)) >= b.limits.MaxResources {
		return 0, ErrCanonicalTreeLimit
	}
	b.tree.resources = append(b.tree.resources, v)
	id := TreeResourceID(len(b.tree.resources))
	b.resources[v] = id
	return id, b.charge(1)
}
func (b *treeBuilder) semantic(in TreeSemanticInput) (TreeSemanticID, error) {
	l, e := b.intern(in.Label)
	if e != nil {
		return 0, e
	}
	if !in.Role.valid() {
		return 0, ErrCanonicalTreeInvalid
	}
	v := TreeSemantic{Role: in.Role, Label: l}
	if id := b.semantics[v]; id != 0 {
		return id, nil
	}
	if uint32(len(b.tree.semantics)) >= b.limits.MaxSemantics {
		return 0, ErrCanonicalTreeLimit
	}
	b.tree.semantics = append(b.tree.semantics, v)
	id := TreeSemanticID(len(b.tree.semantics))
	b.semantics[v] = id
	return id, b.charge(1)
}

func (t CanonicalTree) Projection() CanonicalTreeProjection {
	return CanonicalTreeProjection{CanonicalTreeSchemaVersion, cloneSlice(t.nodes), cloneSlice(t.children), cloneSlice(t.strings), cloneSlice(t.styles), cloneSlice(t.tracks), cloneSlice(t.resources), cloneSlice(t.semantics)}
}
func (t CanonicalTree) Node(index TreeNodeIndex) (TreeNode, bool) {
	if uint64(index) >= uint64(len(t.nodes)) {
		return TreeNode{}, false
	}
	return t.nodes[index], true
}
func (t CanonicalTree) String(id TreeStringID) (string, bool) {
	if id == 0 || uint64(id) > uint64(len(t.strings)) {
		return "", false
	}
	return t.strings[id-1], true
}
func (t CanonicalTree) CanonicalJSON() ([]byte, error) {
	if err := validateCanonicalTree(t); err != nil {
		return nil, err
	}
	return json.Marshal(t.Projection())
}
func (t CanonicalTree) Hash() (string, error) {
	b, e := t.CanonicalJSON()
	if e != nil {
		return "", e
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// SemanticHash identifies compiled meaning while deliberately excluding
// source locations. Formatting and comments may move spans without changing
// the semantic template identity used by plan caches.
func (t CanonicalTree) SemanticHash() (string, error) {
	if err := validateCanonicalTree(t); err != nil {
		return "", err
	}
	projection := t.Projection()
	for index := range projection.Nodes {
		projection.Nodes[index].Source = SourceSpan{}
	}
	encoded, err := json.Marshal(struct {
		Domain string                  `json:"domain"`
		Tree   CanonicalTreeProjection `json:"tree"`
	}{Domain: "gopdfkit.semantic-template.v1", Tree: projection})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func DecodeCanonicalTree(ctx context.Context, encoded []byte, limits CanonicalTreeLimits) (CanonicalTree, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	limits, err := normalizeCanonicalTreeLimits(limits)
	if err != nil {
		return CanonicalTree{}, err
	}
	if uint64(len(encoded)) > limits.MaxStringBytes*4 {
		return CanonicalTree{}, ErrCanonicalTreeLimit
	}
	d := json.NewDecoder(bytes.NewReader(encoded))
	d.DisallowUnknownFields()
	var p CanonicalTreeProjection
	if err := d.Decode(&p); err != nil {
		return CanonicalTree{}, err
	}
	var extra json.RawMessage
	if err := d.Decode(&extra); err != io.EOF {
		return CanonicalTree{}, ErrCanonicalTreeInvalid
	}
	if p.SchemaVersion != CanonicalTreeSchemaVersion {
		return CanonicalTree{}, ErrCanonicalTreeInvalid
	}
	if uint64(len(p.Nodes)) > uint64(limits.MaxNodes) || uint64(len(p.Children)) > uint64(limits.MaxEdges) || uint64(len(p.Strings)) > uint64(limits.MaxStrings) || uint64(len(p.Styles)) > uint64(limits.MaxStyles) || uint64(len(p.Tracks)) > uint64(limits.MaxTracks) || uint64(len(p.Resources)) > uint64(limits.MaxResources) || uint64(len(p.Semantics)) > uint64(limits.MaxSemantics) {
		return CanonicalTree{}, ErrCanonicalTreeLimit
	}
	work := uint64(len(p.Nodes) + len(p.Children) + len(p.Strings) + len(p.Styles) + len(p.Tracks) + len(p.Resources) + len(p.Semantics))
	if work > limits.MaxWork {
		return CanonicalTree{}, ErrCanonicalTreeLimit
	}
	var stringBytes uint64
	for _, value := range p.Strings {
		stringBytes += uint64(len(value))
		if stringBytes > limits.MaxStringBytes {
			return CanonicalTree{}, ErrCanonicalTreeLimit
		}
	}
	if err := ChargePlanningWork(ctx, "canonical tree decode", work); err != nil {
		return CanonicalTree{}, err
	}
	t := CanonicalTree{nodes: cloneSlice(p.Nodes), children: cloneSlice(p.Children), strings: cloneSlice(p.Strings), styles: cloneSlice(p.Styles), tracks: cloneSlice(p.Tracks), resources: cloneSlice(p.Resources), semantics: cloneSlice(p.Semantics)}
	if err := validateCanonicalTree(t); err != nil {
		return CanonicalTree{}, err
	}
	return t, nil
}

func validateCanonicalTree(t CanonicalTree) error {
	stringsSeen := map[string]struct{}{}
	var bytes uint64
	for _, s := range t.strings {
		if s == "" {
			return ErrCanonicalTreeInvalid
		}
		if err := validateTextIdentity("canonical tree string", s); err != nil {
			return ErrCanonicalTreeInvalid
		}
		if _, ok := stringsSeen[s]; ok {
			return ErrCanonicalTreeCollision
		}
		stringsSeen[s] = struct{}{}
		bytes += uint64(len(s))
	}
	_ = bytes
	incoming := make([]uint8, len(t.nodes))
	ids := map[NodeID]struct{}{}
	keys := map[NodeKey]struct{}{}
	for i, n := range t.nodes {
		if !n.ID.Valid() || n.Key == "" || n.Kind == 0 || uint64(n.Kind) > uint64(len(t.strings)) || uint64(n.ChildStart)+uint64(n.ChildCount) > uint64(len(t.children)) {
			return ErrCanonicalTreeInvalid
		}
		if err := validateTextIdentity("canonical tree node key", string(n.Key)); err != nil {
			return ErrCanonicalTreeInvalid
		}
		if err := n.Source.Validate(); err != nil {
			return ErrCanonicalTreeInvalid
		}
		if _, ok := ids[n.ID]; ok {
			return ErrCanonicalTreeCollision
		}
		ids[n.ID] = struct{}{}
		if _, ok := keys[n.Key]; ok {
			return ErrCanonicalTreeCollision
		}
		keys[n.Key] = struct{}{}
		for _, c := range t.children[n.ChildStart : n.ChildStart+n.ChildCount] {
			if uint64(c) >= uint64(len(t.nodes)) || uint64(c) <= uint64(i) {
				return ErrCanonicalTreeInvalid
			}
			incoming[c]++
			if incoming[c] > 1 {
				return ErrCanonicalTreeInvalid
			}
		}
		if n.Style > TreeStyleID(len(t.styles)) || n.Track > TreeTrackID(len(t.tracks)) || n.Resource > TreeResourceID(len(t.resources)) || n.Semantic > TreeSemanticID(len(t.semantics)) || n.Text > TreeStringID(len(t.strings)) {
			return ErrCanonicalTreeInvalid
		}
	}
	styleSeen := map[TreeStyle]struct{}{}
	for _, s := range t.styles {
		if _, exists := styleSeen[s]; exists {
			return ErrCanonicalTreeCollision
		}
		styleSeen[s] = struct{}{}
		if s.FontSize < 0 || s.LineHeight < 0 {
			return ErrCanonicalTreeInvalid
		}
		if s.FontFamily > TreeStringID(len(t.strings)) || s.Align > TreeStringID(len(t.strings)) {
			return ErrCanonicalTreeInvalid
		}
		for _, v := range s.Margin {
			if !validTreeLength(v) {
				return ErrCanonicalTreeInvalid
			}
		}
	}
	trackSeen := map[TreeTrack]struct{}{}
	for _, v := range t.tracks {
		if _, exists := trackSeen[v]; exists {
			return ErrCanonicalTreeCollision
		}
		trackSeen[v] = struct{}{}
		if v.Name > TreeStringID(len(t.strings)) || !validTreeLength(v.Min) || !validTreeLength(v.Max) {
			return ErrCanonicalTreeInvalid
		}
	}
	resourceSeen := map[TreeResource]struct{}{}
	resourceKeys := map[TreeStringID]TreeResource{}
	for _, v := range t.resources {
		if v.Kind == 0 || v.Key == 0 || v.Digest == 0 || v.Kind > TreeStringID(len(t.strings)) || v.Key > TreeStringID(len(t.strings)) || v.Digest > TreeStringID(len(t.strings)) {
			return ErrCanonicalTreeInvalid
		}
		if _, exists := resourceSeen[v]; exists {
			return ErrCanonicalTreeCollision
		}
		resourceSeen[v] = struct{}{}
		if prior, exists := resourceKeys[v.Key]; exists && prior != v {
			return ErrCanonicalTreeCollision
		}
		resourceKeys[v.Key] = v
	}
	semanticSeen := map[TreeSemantic]struct{}{}
	for _, v := range t.semantics {
		if _, exists := semanticSeen[v]; exists {
			return ErrCanonicalTreeCollision
		}
		semanticSeen[v] = struct{}{}
		if !v.Role.valid() || v.Label > TreeStringID(len(t.strings)) {
			return ErrCanonicalTreeInvalid
		}
	}
	return nil
}
