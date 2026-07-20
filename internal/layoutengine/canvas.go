// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"container/heap"
	"context"
	"errors"
	"fmt"
	"strconv"
)

var (
	ErrCanvasConstraintInvalid = errors.New("layoutengine: canvas anchor constraint is invalid")
	ErrCanvasCycle             = errors.New("layoutengine: canvas anchor graph contains a cycle")
	ErrCanvasOverdetermined    = errors.New("layoutengine: canvas node position is overdetermined")
	ErrCanvasUnsatisfiable     = errors.New("layoutengine: canvas anchor constraint is unsatisfiable")
	ErrCanvasWorkLimit         = errors.New("layoutengine: canvas planning work limit exceeded")
	ErrCanvasResourceLimit     = errors.New("layoutengine: canvas planning resource limit exceeded")
	ErrCanvasLimitsInvalid     = errors.New("layoutengine: canvas planning limits are invalid")
)

type CanvasAnchor string

const (
	CanvasAnchorLeft     CanvasAnchor = "left"
	CanvasAnchorRight    CanvasAnchor = "right"
	CanvasAnchorCenterX  CanvasAnchor = "center_x"
	CanvasAnchorTop      CanvasAnchor = "top"
	CanvasAnchorBottom   CanvasAnchor = "bottom"
	CanvasAnchorCenterY  CanvasAnchor = "center_y"
	CanvasAnchorBaseline CanvasAnchor = "baseline"
)

type canvasAxis uint8

const (
	canvasHorizontal canvasAxis = 1
	canvasVertical   canvasAxis = 2
)

func (anchor CanvasAnchor) axis() canvasAxis {
	switch anchor {
	case CanvasAnchorLeft, CanvasAnchorRight, CanvasAnchorCenterX:
		return canvasHorizontal
	case CanvasAnchorTop, CanvasAnchorBottom, CanvasAnchorCenterY, CanvasAnchorBaseline:
		return canvasVertical
	default:
		return 0
	}
}

type CanvasConstraint struct {
	Anchor       CanvasAnchor `json:"anchor"`
	TargetNode   NodeID       `json:"target_node,omitempty"` // zero selects the container
	TargetAnchor CanvasAnchor `json:"target_anchor"`
	Offset       Fixed        `json:"offset,omitempty"`
}

type CanvasNode struct {
	Node        NodeID             `json:"node"`
	Key         NodeKey            `json:"key"`
	Instance    InstanceID         `json:"instance"`
	Source      SourceSpan         `json:"source"`
	Size        Size               `json:"size"`
	Baseline    Fixed              `json:"baseline,omitempty"` // distance from top; zero means absent
	Constraints []CanvasConstraint `json:"constraints,omitempty"`
}

type CanvasDefaults struct {
	Horizontal CanvasAnchor `json:"horizontal"`
	Vertical   CanvasAnchor `json:"vertical"`
}

type CanvasPlanInput struct {
	PageSize  Size           `json:"page_size"`
	Container Rect           `json:"container"`
	Defaults  CanvasDefaults `json:"defaults"`
	Nodes     []CanvasNode   `json:"nodes"`
}

type CanvasPlanLimits struct {
	MaxNodes uint64
	MaxEdges uint64
	MaxBytes uint64
	MaxWork  uint64
}

const (
	hardMaxCanvasNodes uint64 = 1 << 20
	hardMaxCanvasEdges uint64 = 4 << 20
	hardMaxCanvasBytes uint64 = 64 << 20
	hardMaxCanvasWork  uint64 = 32 << 20
	// These conservative charges cover retained input headers, graph state,
	// resolved positions, fragments, diagnostics bookkeeping, and slice/map
	// overhead without depending on unsafe.Sizeof or platform word size.
	canvasRetainedNodeBytes uint64 = 384
	canvasRetainedEdgeBytes uint64 = 64
)

func DefaultCanvasPlanLimits() CanvasPlanLimits {
	return CanvasPlanLimits{MaxNodes: 1 << 18, MaxEdges: 1 << 20, MaxBytes: 16 << 20, MaxWork: 8 << 20}
}

type canvasBudget struct {
	ctx   context.Context
	limit uint64
	used  uint64
}

func (budget *canvasBudget) charge(amount uint64) error {
	if err := ChargePlanningWork(budget.ctx, "canvas planning", amount); err != nil {
		return err
	}
	if err := budget.ctx.Err(); err != nil {
		return newPlanningError(err, Diagnostic{Code: DiagnosticCanceled, Severity: SeverityError, Stage: StageLayout, Message: "canvas planning was canceled"})
	}
	if amount > budget.limit-budget.used {
		return newPlanningError(ErrCanvasWorkLimit, Diagnostic{
			Code: DiagnosticWorkLimit, Severity: SeverityError, Stage: StageLayout,
			Message:  "canvas planning exceeded its deterministic work limit",
			Evidence: []DiagnosticEvidence{{Key: "work_limit", Value: strconv.FormatUint(budget.limit, 10)}},
		})
	}
	budget.used += amount
	return nil
}

// PlanCanvas resolves a local, measured, same-axis anchor DAG. It deliberately
// does not implement inequalities, percentages, priorities, or a general
// linear solver. Input node order is canonical paint order; graph order only
// controls when positions become available.
func PlanCanvas(ctx context.Context, input CanvasPlanInput, limits CanvasPlanLimits) (LayoutPlan, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	limits, err := normalizeCanvasLimits(limits)
	if err != nil {
		return LayoutPlan{}, err
	}
	budget := &canvasBudget{ctx: ctx, limit: limits.MaxWork}
	nodeIndex, edgeCount, err := validateCanvasInput(input, limits, budget)
	if err != nil {
		return LayoutPlan{}, err
	}
	order, err := canvasTopologicalOrder(input.Nodes, nodeIndex, edgeCount, budget)
	if err != nil {
		return LayoutPlan{}, err
	}
	positions := make([]Point, len(input.Nodes))
	resolved := make([]bool, len(input.Nodes))
	for _, index := range order {
		if err := budget.charge(uint64(len(input.Nodes[index].Constraints)) + 1); err != nil {
			return LayoutPlan{}, err
		}
		position, err := resolveCanvasNode(input, index, nodeIndex, positions, resolved)
		if err != nil {
			return LayoutPlan{}, err
		}
		positions[index], resolved[index] = position, true
	}
	return buildCanvasPlan(input, positions)
}

func normalizeCanvasLimits(limits CanvasPlanLimits) (CanvasPlanLimits, error) {
	if limits == (CanvasPlanLimits{}) {
		return DefaultCanvasPlanLimits(), nil
	}
	if limits.MaxNodes == 0 || limits.MaxEdges == 0 || limits.MaxBytes == 0 || limits.MaxWork == 0 {
		return CanvasPlanLimits{}, fmt.Errorf("%w: all bounds must be positive", ErrCanvasLimitsInvalid)
	}
	if limits.MaxNodes > hardMaxCanvasNodes || limits.MaxEdges > hardMaxCanvasEdges || limits.MaxBytes > hardMaxCanvasBytes || limits.MaxWork > hardMaxCanvasWork {
		return CanvasPlanLimits{}, fmt.Errorf("%w: caller bounds exceed hard caps", ErrCanvasLimitsInvalid)
	}
	return limits, nil
}

func validateCanvasInput(input CanvasPlanInput, limits CanvasPlanLimits, budget *canvasBudget) (map[NodeID]int, uint64, error) {
	if err := validateVerticalFlowInput(VerticalFlowInput{PageSize: input.PageSize, Body: input.Container}); err != nil {
		return nil, 0, err
	}
	if input.Defaults.Horizontal.axis() != canvasHorizontal || input.Defaults.Vertical.axis() != canvasVertical || input.Defaults.Vertical == CanvasAnchorBaseline {
		return nil, 0, fmt.Errorf("%w: defaults must select container horizontal and non-baseline vertical anchors", ErrCanvasConstraintInvalid)
	}
	if uint64(len(input.Nodes)) > limits.MaxNodes {
		return nil, 0, canvasResourceError("canvas node count exceeds its limit", len(input.Nodes), 0, 0)
	}
	if err := budget.charge(uint64(len(input.Nodes))); err != nil {
		return nil, 0, err
	}
	var edges, stringBytes uint64
	for _, node := range input.Nodes {
		edges += uint64(len(node.Constraints))
		stringBytes += uint64(len(node.Key) + len(node.Instance) + len(node.Source.File))
		if edges > limits.MaxEdges {
			return nil, 0, canvasResourceError("canvas edge count exceeds its limit", len(input.Nodes), edges, stringBytes)
		}
	}
	bytes := uint64(len(input.Nodes))*canvasRetainedNodeBytes + edges*canvasRetainedEdgeBytes
	if stringBytes > limits.MaxBytes || bytes > limits.MaxBytes-stringBytes {
		return nil, 0, canvasResourceError("canvas retained state exceeds its byte limit", len(input.Nodes), edges, bytes+stringBytes)
	}
	if err := budget.charge(edges); err != nil {
		return nil, 0, err
	}
	nodeIndex := make(map[NodeID]int, len(input.Nodes))
	for index, node := range input.Nodes {
		if !node.Node.Valid() || node.Key == "" || !node.Instance.Valid() || node.Size.Validate() != nil || node.Baseline < 0 || node.Baseline > node.Size.Height ||
			(node.Baseline > 0 && node.Size.Height == 0) || node.Source.Validate() != nil ||
			validateTextIdentity("canvas node key", string(node.Key)) != nil || validateTextIdentity("canvas instance", string(node.Instance)) != nil {
			return nil, 0, fmt.Errorf("%w: node %d has invalid identity, measurement, baseline, or provenance", ErrCanvasConstraintInvalid, index)
		}
		if _, exists := nodeIndex[node.Node]; exists {
			return nil, 0, fmt.Errorf("%w: duplicate node ID %d", ErrCanvasConstraintInvalid, node.Node)
		}
		nodeIndex[node.Node] = index
	}
	for index, node := range input.Nodes {
		for constraintIndex, constraint := range node.Constraints {
			if constraint.Anchor.axis() == 0 || constraint.Anchor.axis() != constraint.TargetAnchor.axis() {
				return nil, 0, fmt.Errorf("%w: node %d constraint %d crosses or has an invalid axis", ErrCanvasConstraintInvalid, index, constraintIndex)
			}
			if !constraint.TargetNode.Valid() {
				if constraint.TargetAnchor == CanvasAnchorBaseline {
					return nil, 0, fmt.Errorf("%w: container has no baseline", ErrCanvasUnsatisfiable)
				}
			} else if _, exists := nodeIndex[constraint.TargetNode]; !exists {
				return nil, 0, fmt.Errorf("%w: node %d constraint %d targets a missing node", ErrCanvasConstraintInvalid, index, constraintIndex)
			}
			if constraint.Anchor == CanvasAnchorBaseline && node.Baseline == 0 {
				return nil, 0, fmt.Errorf("%w: node %d has no baseline", ErrCanvasUnsatisfiable, index)
			}
			if constraint.TargetNode.Valid() && constraint.TargetAnchor == CanvasAnchorBaseline && input.Nodes[nodeIndex[constraint.TargetNode]].Baseline == 0 {
				return nil, 0, fmt.Errorf("%w: target node has no baseline", ErrCanvasUnsatisfiable)
			}
		}
	}
	return nodeIndex, edges, nil
}

type canvasIndexHeap []int

func (values canvasIndexHeap) Len() int           { return len(values) }
func (values canvasIndexHeap) Less(i, j int) bool { return values[i] < values[j] }
func (values canvasIndexHeap) Swap(i, j int)      { values[i], values[j] = values[j], values[i] }
func (values *canvasIndexHeap) Push(value any)    { *values = append(*values, value.(int)) }
func (values *canvasIndexHeap) Pop() any {
	old := *values
	value := old[len(old)-1]
	*values = old[:len(old)-1]
	return value
}

func canvasTopologicalOrder(nodes []CanvasNode, indexes map[NodeID]int, edges uint64, budget *canvasBudget) ([]int, error) {
	indegree := make([]uint64, len(nodes))
	outgoing := make([][]int, len(nodes))
	for sourceIndex, node := range nodes {
		for _, constraint := range node.Constraints {
			if constraint.TargetNode.Valid() {
				targetIndex := indexes[constraint.TargetNode]
				indegree[sourceIndex]++
				outgoing[targetIndex] = append(outgoing[targetIndex], sourceIndex)
			}
		}
	}
	if err := budget.charge(uint64(len(nodes)) + edges); err != nil {
		return nil, err
	}
	ready := &canvasIndexHeap{}
	heap.Init(ready)
	for index, degree := range indegree {
		if degree == 0 {
			heap.Push(ready, index)
		}
	}
	order := make([]int, 0, len(nodes))
	for ready.Len() != 0 {
		index := heap.Pop(ready).(int)
		order = append(order, index)
		for _, dependent := range outgoing[index] {
			indegree[dependent]--
			if indegree[dependent] == 0 {
				heap.Push(ready, dependent)
			}
		}
	}
	if len(order) != len(nodes) {
		for index, degree := range indegree {
			if degree != 0 {
				node := nodes[index]
				return nil, newPlanningError(ErrCanvasCycle, Diagnostic{
					Code: DiagnosticConstraintCycle, Severity: SeverityError, Stage: StageLayout,
					Message:  "canvas anchor dependencies contain a cycle",
					Location: DiagnosticLocation{Node: node.Node, Key: node.Key, Instance: node.Instance, Source: node.Source},
					Evidence: []DiagnosticEvidence{{Key: "unresolved_nodes", Value: strconv.Itoa(len(nodes) - len(order))}},
				})
			}
		}
	}
	return order, nil
}

func resolveCanvasNode(input CanvasPlanInput, index int, indexes map[NodeID]int, positions []Point, resolved []bool) (Point, error) {
	node := input.Nodes[index]
	var horizontal, vertical *Fixed
	for constraintIndex, constraint := range node.Constraints {
		target, err := canvasTargetCoordinate(input, constraint, indexes, positions, resolved)
		if err != nil {
			return Point{}, err
		}
		sourceOffset, err := canvasNodeAnchorOffset(node, constraint.Anchor)
		if err != nil {
			return Point{}, err
		}
		proposal, err := target.Add(constraint.Offset)
		if err == nil {
			proposal, err = proposal.Sub(sourceOffset)
		}
		if err != nil {
			return Point{}, fmt.Errorf("layoutengine: canvas node %d constraint %d: %w", index, constraintIndex, err)
		}
		selected := &horizontal
		if constraint.Anchor.axis() == canvasVertical {
			selected = &vertical
		}
		if *selected != nil && **selected != proposal {
			return Point{}, canvasOverdetermined(node, constraint.Anchor, **selected, proposal)
		}
		if *selected == nil {
			value := proposal
			*selected = &value
		}
	}
	if horizontal == nil {
		value, err := canvasDefaultCoordinate(input.Container, node, input.Defaults.Horizontal)
		if err != nil {
			return Point{}, err
		}
		horizontal = &value
	}
	if vertical == nil {
		value, err := canvasDefaultCoordinate(input.Container, node, input.Defaults.Vertical)
		if err != nil {
			return Point{}, err
		}
		vertical = &value
	}
	return Point{X: *horizontal, Y: *vertical}, nil
}

func canvasTargetCoordinate(input CanvasPlanInput, constraint CanvasConstraint, indexes map[NodeID]int, positions []Point, resolved []bool) (Fixed, error) {
	if !constraint.TargetNode.Valid() {
		return canvasRectAnchor(input.Container, constraint.TargetAnchor)
	}
	index := indexes[constraint.TargetNode]
	if !resolved[index] {
		return 0, ErrCanvasUnsatisfiable
	}
	offset, err := canvasNodeAnchorOffset(input.Nodes[index], constraint.TargetAnchor)
	if err != nil {
		return 0, err
	}
	if constraint.TargetAnchor.axis() == canvasHorizontal {
		return positions[index].X.Add(offset)
	}
	return positions[index].Y.Add(offset)
}

func canvasNodeAnchorOffset(node CanvasNode, anchor CanvasAnchor) (Fixed, error) {
	switch anchor {
	case CanvasAnchorLeft, CanvasAnchorTop:
		return 0, nil
	case CanvasAnchorRight:
		return node.Size.Width, nil
	case CanvasAnchorBottom:
		return node.Size.Height, nil
	case CanvasAnchorCenterX:
		return node.Size.Width.DivInt(2)
	case CanvasAnchorCenterY:
		return node.Size.Height.DivInt(2)
	case CanvasAnchorBaseline:
		if node.Baseline == 0 {
			return 0, ErrCanvasUnsatisfiable
		}
		return node.Baseline, nil
	default:
		return 0, ErrCanvasConstraintInvalid
	}
}

func canvasRectAnchor(rect Rect, anchor CanvasAnchor) (Fixed, error) {
	switch anchor {
	case CanvasAnchorLeft:
		return rect.X, nil
	case CanvasAnchorRight:
		return rect.Right()
	case CanvasAnchorCenterX:
		half, _ := rect.Width.DivInt(2)
		return rect.X.Add(half)
	case CanvasAnchorTop:
		return rect.Y, nil
	case CanvasAnchorBottom:
		return rect.Bottom()
	case CanvasAnchorCenterY:
		half, _ := rect.Height.DivInt(2)
		return rect.Y.Add(half)
	default:
		return 0, ErrCanvasConstraintInvalid
	}
}

func canvasDefaultCoordinate(container Rect, node CanvasNode, anchor CanvasAnchor) (Fixed, error) {
	target, err := canvasRectAnchor(container, anchor)
	if err != nil {
		return 0, err
	}
	offset, err := canvasNodeAnchorOffset(node, anchor)
	if err != nil {
		return 0, err
	}
	return target.Sub(offset)
}

func canvasOverdetermined(node CanvasNode, anchor CanvasAnchor, first, second Fixed) error {
	return newPlanningError(ErrCanvasOverdetermined, Diagnostic{
		Code: DiagnosticConstraintOverdetermined, Severity: SeverityError, Stage: StageLayout,
		Message:  "canvas constraints resolve the same axis to different positions",
		Location: DiagnosticLocation{Node: node.Node, Key: node.Key, Instance: node.Instance, Source: node.Source},
		Evidence: []DiagnosticEvidence{
			{Key: "anchor", Value: string(anchor)}, {Key: "first_position", Value: strconv.FormatInt(int64(first), 10)},
			{Key: "second_position", Value: strconv.FormatInt(int64(second), 10)},
		},
	})
}

func buildCanvasPlan(input CanvasPlanInput, positions []Point) (LayoutPlan, error) {
	planInput := LayoutPlanInput{Pages: []PlannedPage{{Number: 1, Size: input.PageSize, Fragments: IndexRange{Count: uint32(len(input.Nodes))}}}} // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
	for index, node := range input.Nodes {
		bounds, err := NewRect(positions[index].X, positions[index].Y, node.Size.Width, node.Size.Height)
		if err != nil {
			return LayoutPlan{}, err
		}
		fragment := Fragment{
			ID: FragmentID(index + 1), Node: node.Node, Key: node.Key, Instance: node.Instance,
			Page: 1, Region: RegionBody, BorderBox: bounds, ContentBox: bounds,
			Source: node.Source, Continuation: ContinuationWhole,
		}
		planInput.Fragments = append(planInput.Fragments, fragment)
		intersection, err := bounds.Intersect(input.Container)
		if err != nil {
			return LayoutPlan{}, err
		}
		if intersection != bounds {
			diagnostic, err := canvasOverflowDiagnostic(fragment, input.Container)
			if err != nil {
				return LayoutPlan{}, err
			}
			planInput.Diagnostics = append(planInput.Diagnostics, diagnostic)
		}
	}
	return NewLayoutPlan(planInput)
}

func canvasOverflowDiagnostic(fragment Fragment, container Rect) (Diagnostic, error) {
	left, err := canvasPositiveDifference(container.X, fragment.BorderBox.X)
	if err != nil {
		return Diagnostic{}, err
	}
	top, err := canvasPositiveDifference(container.Y, fragment.BorderBox.Y)
	if err != nil {
		return Diagnostic{}, err
	}
	containerRight, _ := container.Right()
	containerBottom, _ := container.Bottom()
	fragmentRight, _ := fragment.BorderBox.Right()
	fragmentBottom, _ := fragment.BorderBox.Bottom()
	right, err := canvasPositiveDifference(fragmentRight, containerRight)
	if err != nil {
		return Diagnostic{}, err
	}
	bottom, err := canvasPositiveDifference(fragmentBottom, containerBottom)
	if err != nil {
		return Diagnostic{}, err
	}
	return Diagnostic{
		Code: DiagnosticCanvasNodeOverflow, Severity: SeverityWarning, Stage: StageLayout,
		Message:  "canvas node extends outside its local container",
		Location: DiagnosticLocation{Node: fragment.Node, Key: fragment.Key, Instance: fragment.Instance, Source: fragment.Source, Fragment: fragment.ID, Page: 1, Region: RegionBody, Bounds: fragment.BorderBox, HasBounds: true},
		Evidence: []DiagnosticEvidence{
			{Key: "left", Value: strconv.FormatInt(int64(left), 10)}, {Key: "top", Value: strconv.FormatInt(int64(top), 10)},
			{Key: "right", Value: strconv.FormatInt(int64(right), 10)}, {Key: "bottom", Value: strconv.FormatInt(int64(bottom), 10)},
		},
	}, nil
}

func canvasPositiveDifference(high, low Fixed) (Fixed, error) {
	if high <= low {
		return 0, nil
	}
	return high.Sub(low)
}

func canvasResourceError(message string, nodes int, edges, bytes uint64) error {
	return newPlanningError(ErrCanvasResourceLimit, Diagnostic{
		Code: DiagnosticResourceLimit, Severity: SeverityError, Stage: StageLayout, Message: message,
		Evidence: []DiagnosticEvidence{
			{Key: "nodes", Value: strconv.Itoa(nodes)}, {Key: "edges", Value: strconv.FormatUint(edges, 10)},
			{Key: "bytes", Value: strconv.FormatUint(bytes, 10)},
		},
	})
}
