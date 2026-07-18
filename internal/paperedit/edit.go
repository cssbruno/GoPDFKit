// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperedit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/cssbruno/gopdfkit/internal/paperlang"
)

const (
	ResultSchemaVersion     uint16 = 2
	MaxSourceBytes                 = 8 << 20
	MaxOperations                  = 256
	MaxReplacementBytes            = 1 << 20
	MaxIdempotencyKeyBytes         = 256
	MaxTargetPreconditions         = 256
	MaxDiagnosticCandidates        = 8
)

var (
	ErrRevisionConflict = errors.New("paperedit: source revision conflict")
	ErrTargetConflict   = errors.New("paperedit: target fingerprint conflict")
	ErrInvalidSource    = errors.New("paperedit: source is invalid")
	ErrInvalidOperation = errors.New("paperedit: operation is invalid")
	ErrPatchConflict    = errors.New("paperedit: source patches conflict")
	ErrCandidateInvalid = errors.New("paperedit: edited candidate is invalid")
	ErrLimit            = errors.New("paperedit: edit limit exceeded")
)

// Revision is the lowercase SHA-256 digest of the exact UTF-8 source bytes.
type Revision string

// NodeFingerprint is the lowercase SHA-256 digest of one exact node source
// block, from the start of its header line through its final line ending. It
// includes descendants and authored trivia inside that span.
type NodeFingerprint string

// SourceRevision returns the deterministic optimistic-concurrency token for
// source without parsing or normalization.
func SourceRevision(source string) Revision {
	digest := sha256.Sum256([]byte(source))
	return Revision(hex.EncodeToString(digest[:]))
}

// FingerprintNode returns the fingerprint used by TargetPrecondition. The
// source must parse successfully and target must resolve uniquely.
func FingerprintNode(file, source, target string) (NodeFingerprint, error) {
	if len(source) > MaxSourceBytes {
		return "", ErrLimit
	}
	parsed := paperlang.Parse(file, source)
	if !parsed.OK() {
		return "", ErrInvalidSource
	}
	index := indexSource(parsed.AST.Root, nil)
	node, err := targetNode(index, target)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidOperation, err)
	}
	return fingerprintNodeSource(source, node)
}

// SourceInstance resolves the canonical readable-ID ancestry of one exact
// source node. Expanded component/repeat instances are intentionally absent:
// callers must edit a definition or invocation explicitly.
func SourceInstance(file, source, target string) (string, error) {
	if len(source) > MaxSourceBytes {
		return "", ErrLimit
	}
	parsed := paperlang.Parse(file, source)
	if !parsed.OK() {
		return "", ErrInvalidSource
	}
	index := indexSource(parsed.AST.Root, nil)
	if _, err := targetNode(index, target); err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidOperation, err)
	}
	return index.instances[target], nil
}

// TargetPrecondition guards one readable-ID node independently of the source
// revision. This is useful when an agent prepared an edit from a structural
// query and wants to prove that its target did not change.
type TargetPrecondition struct {
	Target              string          `json:"target"`
	ExpectedFingerprint NodeFingerprint `json:"expected_fingerprint"`
	ExpectedInstance    string          `json:"expected_instance"`
}

type TargetCandidate struct {
	Target   string             `json:"target"`
	Instance string             `json:"instance"`
	Kind     paperlang.NodeKind `json:"kind"`
	Span     paperlang.Span     `json:"span"`
}

// ValueKind identifies a typed scalar authored by SetProperty or NodeSpec.
type ValueKind string

const (
	ValueString ValueKind = "string"
	ValueBool   ValueKind = "bool"
	ValueNumber ValueKind = "number"
	ValueUnit   ValueKind = "unit"
)

// Value is a typed .paper scalar. Text stores a string value or the unit
// suffix; Bool and Number carry their corresponding typed values.
type Value struct {
	Kind   ValueKind
	Text   string
	Bool   bool
	Number float64
}

func StringValue(value string) Value  { return Value{Kind: ValueString, Text: value} }
func BoolValue(value bool) Value      { return Value{Kind: ValueBool, Bool: value} }
func NumberValue(value float64) Value { return Value{Kind: ValueNumber, Number: value} }
func UnitValue(number float64, unit string) Value {
	return Value{Kind: ValueUnit, Number: number, Text: unit}
}

// PropertySpec is one typed property in an inserted node.
type PropertySpec struct {
	Name  string
	Value Value
}

// NodeSpec is a typed recursive component inserted beneath a readable-ID
// parent. Properties are emitted before children in the supplied order.
type NodeSpec struct {
	Kind       paperlang.NodeKind
	ID         string
	Value      *Value
	Properties []PropertySpec
	Children   []NodeSpec
}

// Operation is one typed transactional source edit.
type Operation interface {
	paperEditOperation()
}

type SetProperty struct {
	Target string
	Name   string
	Value  Value
}

func (SetProperty) paperEditOperation() {}

// SetProperties applies a bounded group of properties to one node. Grouping
// the insertions into one source patch keeps an authoring action atomic when
// several new properties share the same insertion point.
type SetProperties struct {
	Target     string
	Properties []PropertySpec
}

func (SetProperties) paperEditOperation() {}

// AppendProperty emits one additional property declaration. It is reserved
// for grammar properties that intentionally repeat, such as document imports.
type AppendProperty struct {
	Target string
	Name   string
	Value  Value
}

func (AppendProperty) paperEditOperation() {}

type ReplaceText struct {
	Target string
	Text   string
}

func (ReplaceText) paperEditOperation() {}

type InsertNode struct {
	Parent string
	Node   NodeSpec
}

func (InsertNode) paperEditOperation() {}

type DeleteNode struct {
	Target string
}

func (DeleteNode) paperEditOperation() {}

// RenameID changes one readable ID declaration. The initial .paper grammar
// has no structural reference token, so string interpolation and authored text
// are intentionally not rewritten. Candidate parsing enforces uniqueness.
type RenameID struct {
	Target string
	NewID  string
}

func (RenameID) paperEditOperation() {}

// MoveNode relocates one exact node source block beneath another readable-ID
// node and adjusts only its indentation to the destination's canonical depth.
type MoveNode struct {
	Target    string
	NewParent string
}

func (MoveNode) paperEditOperation() {}

// WrapNode nests one existing node beneath a newly authored wrapper. Wrapper
// may contain properties, but its Children must be empty because the targeted
// source node becomes its sole child. The targeted node block is preserved
// byte-for-byte except for the indentation required by the new parent.
type WrapNode struct {
	Target  string
	Wrapper NodeSpec
}

func (WrapNode) paperEditOperation() {}

// UnwrapNode removes one structural wrapper and promotes its child node
// blocks to the wrapper's parent. Wrappers with properties are rejected
// because removing those properties would not preserve document semantics.
type UnwrapNode struct {
	Target string
}

func (UnwrapNode) paperEditOperation() {}

// ReplaceNode replaces one exact node block with a typed component. Source
// outside the target span, including surrounding comments and line endings,
// remains untouched.
type ReplaceNode struct {
	Target string
	Node   NodeSpec
}

func (ReplaceNode) paperEditOperation() {}

// ReplaceComponent is the component-oriented spelling of ReplaceNode.
type ReplaceComponent = ReplaceNode

// Transaction binds all operations to one exact source revision.
type Transaction struct {
	File             string
	Source           string
	ExpectedRevision Revision
	// IdempotencyKey is an optional caller-owned replay identity. Apply is a
	// pure function and echoes it on every result so a stateful boundary can
	// cache or deduplicate the deterministic outcome.
	IdempotencyKey string
	// TargetPreconditions are checked against the same parsed source revision
	// used to resolve operations. Their order does not affect the result.
	TargetPreconditions []TargetPrecondition
	// RequireExactTargets rejects every operation target that lacks both an
	// exact node fingerprint and source-instance precondition.
	RequireExactTargets bool
	Operations          []Operation
}

// Diagnostic is a deterministic transaction or parser diagnostic. Operation
// is one-based when the problem belongs to a requested operation and zero for
// source/candidate-wide failures.
type Diagnostic struct {
	Code       string                       `json:"code"`
	Severity   paperlang.DiagnosticSeverity `json:"severity"`
	Message    string                       `json:"message"`
	Operation  uint32                       `json:"operation,omitempty"`
	Target     string                       `json:"target,omitempty"`
	Span       paperlang.Span               `json:"span"`
	Candidates []TargetCandidate            `json:"candidates,omitempty"`
}

// SourcePatch is an exact edit in byte offsets relative to the input source.
// Removed is the original byte sequence in [Start, End), and Replacement is
// the committed byte sequence. Non-overlapping patches can reconstruct the
// result by applying them from the greatest Start offset to the smallest.
type SourcePatch struct {
	Start       uint32 `json:"start"`
	End         uint32 `json:"end"`
	Removed     string `json:"removed"`
	Replacement string `json:"replacement"`
	Operation   uint32 `json:"operation"`
	Target      string `json:"target,omitempty"`
}

// SourceDiff describes one committed change without normalizing source text.
type SourceDiff struct {
	BeforeRevision Revision      `json:"before_revision"`
	AfterRevision  Revision      `json:"after_revision"`
	Patches        []SourcePatch `json:"patches"`
}

// InvalidationScope is deliberately conservative. NodeIDs contains directly
// addressed nodes, structural destinations, and their readable-ID ancestors.
// WholeDocument is true because paginated layout may cascade after any edit.
type InvalidationScope struct {
	WholeDocument bool     `json:"whole_document"`
	NodeIDs       []string `json:"node_ids"`
}

// Result always carries either the atomically committed candidate or the
// unchanged input. Revision is the digest of Source in both cases.
type Result struct {
	SchemaVersion     uint16             `json:"schema_version"`
	File              string             `json:"file"`
	IdempotencyKey    string             `json:"idempotency_key,omitempty"`
	Source            string             `json:"source"`
	Revision          Revision           `json:"revision"`
	Applied           bool               `json:"applied"`
	AppliedOperations uint32             `json:"applied_operations"`
	Diff              *SourceDiff        `json:"diff,omitempty"`
	Invalidation      *InvalidationScope `json:"invalidation,omitempty"`
	Diagnostics       []Diagnostic       `json:"diagnostics,omitempty"`
}

func (result Result) CanonicalJSON() ([]byte, error) { return json.Marshal(result) }

type sourcePatch struct {
	start       int
	end         int
	replacement string
	operation   int
	target      string
}

type sourceIndex struct {
	byID      map[string]*paperlang.Node
	idSpans   map[string]paperlang.Span
	parents   map[string]*paperlang.Node
	instances map[string]string
}

// Apply performs a failure-atomic source transaction. It never mutates the
// caller's source or returns a partially edited candidate.
func Apply(transaction Transaction) (Result, error) {
	actualRevision := SourceRevision(transaction.Source)
	unchanged := Result{
		SchemaVersion: ResultSchemaVersion, File: transaction.File,
		IdempotencyKey: transaction.IdempotencyKey,
		Source:         transaction.Source, Revision: actualRevision,
	}
	if len(transaction.Source) > MaxSourceBytes {
		return failResult(unchanged, ErrLimit, "PAPER_EDIT_SOURCE_LIMIT", "source exceeds the transactional edit limit", 0, "", paperlang.Span{File: transaction.File})
	}
	if len(transaction.IdempotencyKey) > MaxIdempotencyKeyBytes || !utf8.ValidString(transaction.IdempotencyKey) {
		return failResult(unchanged, ErrLimit, "PAPER_EDIT_IDEMPOTENCY_KEY_LIMIT", "idempotency key must be valid UTF-8 within the edit limit", 0, "", paperlang.Span{File: transaction.File})
	}
	if len(transaction.TargetPreconditions) > MaxTargetPreconditions {
		return failResult(unchanged, ErrLimit, "PAPER_EDIT_PRECONDITION_LIMIT", "target preconditions exceed the edit limit", 0, "", paperlang.Span{File: transaction.File})
	}
	if len(transaction.Operations) == 0 || len(transaction.Operations) > MaxOperations {
		return failResult(unchanged, ErrLimit, "PAPER_EDIT_OPERATION_LIMIT", "transaction must contain a bounded non-empty operation list", 0, "", paperlang.Span{File: transaction.File})
	}
	if transaction.ExpectedRevision != actualRevision {
		return failResult(unchanged, ErrRevisionConflict, "PAPER_EDIT_REVISION_CONFLICT", "source changed after the transaction was prepared", 0, "", paperlang.Span{File: transaction.File})
	}

	parsed := paperlang.Parse(transaction.File, transaction.Source)
	if !parsed.OK() {
		unchanged.Diagnostics = parserDiagnostics(parsed.Diagnostics)
		return unchanged, ErrInvalidSource
	}
	lexed := paperlang.Lex(transaction.File, transaction.Source)
	index := indexSource(parsed.AST.Root, lexed.Tokens)
	if result, err := checkTargetPreconditions(unchanged, transaction, index); err != nil {
		return result, err
	}
	if transaction.RequireExactTargets {
		if result, err := requireExactOperationTargets(unchanged, transaction, index); err != nil {
			return result, err
		}
	}
	patches := make([]sourcePatch, 0, len(transaction.Operations)+1)
	for operationIndex, operation := range transaction.Operations {
		resolved, target, err := resolveOperation(transaction.Source, index, operationIndex, operation)
		if err != nil {
			diagnostic := Diagnostic{
				Code: "PAPER_EDIT_INVALID_OPERATION", Severity: paperlang.SeverityError,
				Message: err.Error(), Operation: uint32(operationIndex + 1), Span: paperlang.Span{File: transaction.File},
			}
			if target != "" {
				diagnostic.Target = target
			}
			unchanged.Diagnostics = []Diagnostic{diagnostic}
			return unchanged, fmt.Errorf("%w: operation %d: %v", ErrInvalidOperation, operationIndex+1, err)
		}
		var replacementBytes int
		for _, patch := range resolved {
			replacementBytes += len(patch.replacement)
			if replacementBytes > MaxReplacementBytes {
				return failResult(unchanged, ErrLimit, "PAPER_EDIT_REPLACEMENT_LIMIT", "operation replacement exceeds the edit limit", operationIndex+1, target, paperlang.Span{File: transaction.File})
			}
		}
		patches = append(patches, resolved...)
	}
	if first, second, conflict := conflictingPatches(patches); conflict {
		unchanged.Diagnostics = []Diagnostic{{
			Code: "PAPER_EDIT_PATCH_CONFLICT", Severity: paperlang.SeverityError,
			Message:   fmt.Sprintf("operations %d and %d target overlapping source spans", first.operation+1, second.operation+1),
			Operation: uint32(second.operation + 1), Target: second.target, Span: paperlang.Span{File: transaction.File},
		}}
		return unchanged, ErrPatchConflict
	}

	candidate, err := applyPatches(transaction.Source, patches)
	if err != nil {
		return failResult(unchanged, ErrInvalidOperation, "PAPER_EDIT_PATCH_RANGE", err.Error(), 0, "", paperlang.Span{File: transaction.File})
	}
	if len(candidate) > MaxSourceBytes {
		return failResult(unchanged, ErrLimit, "PAPER_EDIT_SOURCE_LIMIT", "edited candidate exceeds the transactional edit limit", 0, "", paperlang.Span{File: transaction.File})
	}
	candidateParse := paperlang.Parse(transaction.File, candidate)
	if !candidateParse.OK() {
		unchanged.Diagnostics = parserDiagnostics(candidateParse.Diagnostics)
		return unchanged, ErrCandidateInvalid
	}
	result := Result{
		SchemaVersion:     ResultSchemaVersion,
		File:              transaction.File,
		IdempotencyKey:    transaction.IdempotencyKey,
		Source:            candidate,
		Revision:          SourceRevision(candidate),
		Applied:           true,
		AppliedOperations: uint32(len(transaction.Operations)),
	}
	result.Diff = &SourceDiff{
		BeforeRevision: actualRevision,
		AfterRevision:  result.Revision,
		Patches:        exportPatches(transaction.Source, patches),
	}
	result.Invalidation = invalidationScope(index, transaction.Operations)
	return result, nil
}

func checkTargetPreconditions(unchanged Result, transaction Transaction, index sourceIndex) (Result, error) {
	ordered := append([]TargetPrecondition(nil), transaction.TargetPreconditions...)
	sort.Slice(ordered, func(first, second int) bool { return ordered[first].Target < ordered[second].Target })
	for position, precondition := range ordered {
		if position > 0 && ordered[position-1].Target == precondition.Target {
			return failResult(unchanged, ErrInvalidOperation, "PAPER_EDIT_DUPLICATE_PRECONDITION", "target precondition is declared more than once", 0, precondition.Target, paperlang.Span{File: transaction.File})
		}
		if !validNodeFingerprint(precondition.ExpectedFingerprint) {
			return failResult(unchanged, ErrInvalidOperation, "PAPER_EDIT_INVALID_FINGERPRINT", "expected target fingerprint must be a lowercase SHA-256 digest", 0, precondition.Target, paperlang.Span{File: transaction.File})
		}
		node, err := targetNode(index, precondition.Target)
		if err != nil {
			return failResult(unchanged, ErrTargetConflict, "PAPER_EDIT_TARGET_CONFLICT", "precondition target is absent from the source revision", 0, precondition.Target, paperlang.Span{File: transaction.File})
		}
		actual, err := fingerprintNodeSource(transaction.Source, node)
		if err != nil {
			return failResult(unchanged, ErrInvalidSource, "PAPER_EDIT_FINGERPRINT_RANGE", err.Error(), 0, precondition.Target, node.Span)
		}
		if actual != precondition.ExpectedFingerprint {
			return failResult(unchanged, ErrTargetConflict, "PAPER_EDIT_TARGET_CONFLICT", "target changed after the edit was prepared", 0, precondition.Target, node.Span)
		}
		if precondition.ExpectedInstance == "" && transaction.RequireExactTargets {
			return failResult(unchanged, ErrInvalidOperation, "PAPER_EDIT_INSTANCE_PRECONDITION_REQUIRED", "target requires an exact source-instance precondition", 0, precondition.Target, node.Span)
		}
		if precondition.ExpectedInstance == "" {
			continue
		}
		if actualInstance := index.instances[precondition.Target]; precondition.ExpectedInstance != actualInstance {
			result, cause := failResult(unchanged, ErrTargetConflict, "PAPER_EDIT_INSTANCE_CONFLICT", "target source instance changed after the edit was prepared", 0, precondition.Target, node.Span)
			result.Diagnostics[0].Candidates = sourceTargetCandidates(index, precondition.Target)
			return result, cause
		}
	}
	return unchanged, nil
}

func requireExactOperationTargets(unchanged Result, transaction Transaction, index sourceIndex) (Result, error) {
	provided := make(map[string]TargetPrecondition, len(transaction.TargetPreconditions))
	for _, precondition := range transaction.TargetPreconditions {
		provided[precondition.Target] = precondition
	}
	for operationIndex, operation := range transaction.Operations {
		for _, target := range operationTargetIDs(operation) {
			precondition, exists := provided[target]
			if exists && precondition.ExpectedInstance != "" && validNodeFingerprint(precondition.ExpectedFingerprint) {
				continue
			}
			targetNode := index.byID[target]
			covered := false
			for guardedTarget, guarded := range provided {
				guardNode := index.byID[guardedTarget]
				if targetNode != nil && guardNode != nil && guarded.ExpectedInstance != "" && validNodeFingerprint(guarded.ExpectedFingerprint) &&
					targetNode.Span.Start.Offset >= guardNode.Span.Start.Offset && targetNode.Span.End.Offset <= guardNode.Span.End.Offset {
					covered = true
					break
				}
			}
			if covered {
				continue
			}
			result, cause := failResult(unchanged, ErrInvalidOperation, "PAPER_EDIT_PRECONDITION_REQUIRED", "operation target requires exact revision, fingerprint, and source-instance preconditions", operationIndex+1, target, paperlang.Span{File: transaction.File})
			result.Diagnostics[0].Candidates = sourceTargetCandidates(index, target)
			return result, cause
		}
	}
	return unchanged, nil
}

func operationTargetIDs(operation Operation) []string {
	switch edit := operation.(type) {
	case SetProperty:
		return []string{edit.Target}
	case SetProperties:
		return []string{edit.Target}
	case AppendProperty:
		return []string{edit.Target}
	case ReplaceText:
		return []string{edit.Target}
	case InsertNode:
		return []string{edit.Parent}
	case DeleteNode:
		return []string{edit.Target}
	case RenameID:
		return []string{edit.Target}
	case MoveNode:
		return []string{edit.Target, edit.NewParent}
	case WrapNode:
		return []string{edit.Target}
	case UnwrapNode:
		return []string{edit.Target}
	case ReplaceNode:
		return []string{edit.Target}
	default:
		return nil
	}
}

func sourceTargetCandidates(index sourceIndex, target string) []TargetCandidate {
	node := index.byID[target]
	if node == nil {
		return nil
	}
	return []TargetCandidate{{Target: target, Instance: index.instances[target], Kind: node.Kind, Span: node.Span}}
}

func fingerprintNodeSource(source string, node *paperlang.Node) (NodeFingerprint, error) {
	if node == nil {
		return "", errors.New("paperedit: fingerprint node is nil")
	}
	start := lineStart(source, int(node.HeaderSpan.Start.Offset))
	end := lineEndIncludingNewline(source, int(node.Span.End.Offset))
	if start < 0 || end < start || end > len(source) {
		return "", errors.New("paperedit: node fingerprint span is outside source")
	}
	digest := sha256.Sum256([]byte(source[start:end]))
	return NodeFingerprint(hex.EncodeToString(digest[:])), nil
}

func validNodeFingerprint(fingerprint NodeFingerprint) bool {
	if len(fingerprint) != sha256.Size*2 {
		return false
	}
	for _, character := range fingerprint {
		digit := character >= '0' && character <= '9'
		lowerHex := character >= 'a' && character <= 'f'
		if !digit && !lowerHex {
			return false
		}
	}
	return true
}

func failResult(result Result, cause error, code, message string, operation int, target string, span paperlang.Span) (Result, error) {
	diagnostic := Diagnostic{Code: code, Severity: paperlang.SeverityError, Message: message, Target: target, Span: span}
	if operation > 0 {
		diagnostic.Operation = uint32(operation)
	}
	result.Diagnostics = []Diagnostic{diagnostic}
	return result, cause
}

func parserDiagnostics(diagnostics []paperlang.Diagnostic) []Diagnostic {
	if len(diagnostics) == 0 {
		return nil
	}
	result := make([]Diagnostic, len(diagnostics))
	for index, diagnostic := range diagnostics {
		result[index] = Diagnostic{
			Code: diagnostic.Code, Severity: diagnostic.Severity,
			Message: diagnostic.Message, Span: diagnostic.Span,
		}
	}
	return result
}

func indexSource(root *paperlang.Node, tokens []paperlang.Token) sourceIndex {
	index := sourceIndex{
		byID: make(map[string]*paperlang.Node), idSpans: make(map[string]paperlang.Span),
		parents: make(map[string]*paperlang.Node), instances: make(map[string]string),
	}
	for _, token := range tokens {
		if token.Kind == paperlang.TokenReadableID {
			index.idSpans[token.Lexeme] = token.Span
		}
	}
	var walk func(*paperlang.Node, *paperlang.Node)
	walk = func(node, parent *paperlang.Node) {
		if node == nil {
			return
		}
		if node.ID != "" {
			index.byID[node.ID] = node
			index.parents[node.ID] = parent
			parts := []string{node.ID}
			for ancestor := parent; ancestor != nil; ancestor = func() *paperlang.Node {
				if ancestor.ID == "" {
					return nil
				}
				return index.parents[ancestor.ID]
			}() {
				if ancestor.ID != "" {
					parts = append(parts, ancestor.ID)
				}
			}
			for left, right := 0, len(parts)-1; left < right; left, right = left+1, right-1 {
				parts[left], parts[right] = parts[right], parts[left]
			}
			index.instances[node.ID] = "source/" + strings.Join(parts, "/")
		}
		for _, member := range node.Members {
			walk(member.Node, node)
		}
	}
	walk(root, nil)
	return index
}

func resolveOperation(source string, index sourceIndex, operationIndex int, operation Operation) ([]sourcePatch, string, error) {
	patch := sourcePatch{operation: operationIndex}
	switch edit := operation.(type) {
	case SetProperty:
		patch.target = edit.Target
		node, err := targetNode(index, edit.Target)
		if err != nil {
			return nil, patch.target, err
		}
		if !validPropertyName(edit.Name) {
			return nil, patch.target, fmt.Errorf("property name %q is not a .paper identifier", edit.Name)
		}
		value, err := renderValue(edit.Value)
		if err != nil {
			return nil, patch.target, err
		}
		var found *paperlang.Property
		for _, member := range node.Members {
			if member.Property == nil || member.Property.Name != edit.Name {
				continue
			}
			if found != nil {
				return nil, patch.target, fmt.Errorf("property %q is ambiguous on %s", edit.Name, edit.Target)
			}
			found = member.Property
		}
		if found != nil {
			patch.start, patch.end = int(found.Value.Span.Start.Offset), int(found.Value.Span.End.Offset)
			patch.replacement = value
			return []sourcePatch{patch}, patch.target, nil
		}
		indent := childIndent(node)
		point, prefix, newline := insertionPoint(source, int(node.Span.End.Offset))
		patch.start, patch.end = point, point
		patch.replacement = prefix + strings.Repeat(" ", indent) + edit.Name + ": " + value + newline
		return []sourcePatch{patch}, patch.target, nil

	case SetProperties:
		patch.target = edit.Target
		node, err := targetNode(index, edit.Target)
		if err != nil {
			return nil, patch.target, err
		}
		if len(edit.Properties) == 0 || len(edit.Properties) > MaxOperations {
			return nil, patch.target, errors.New("property group must contain a bounded non-empty list")
		}
		seen := make(map[string]struct{}, len(edit.Properties))
		missing := make([]PropertySpec, 0, len(edit.Properties))
		patches := make([]sourcePatch, 0, len(edit.Properties))
		for _, property := range edit.Properties {
			if !validPropertyName(property.Name) {
				return nil, patch.target, fmt.Errorf("property name %q is not a .paper identifier", property.Name)
			}
			if _, exists := seen[property.Name]; exists {
				return nil, patch.target, fmt.Errorf("property %q is duplicated in the group", property.Name)
			}
			seen[property.Name] = struct{}{}
			value, renderErr := renderValue(property.Value)
			if renderErr != nil {
				return nil, patch.target, renderErr
			}
			var found *paperlang.Property
			for _, member := range node.Members {
				if member.Property == nil || member.Property.Name != property.Name {
					continue
				}
				if found != nil {
					return nil, patch.target, fmt.Errorf("property %q is ambiguous on %s", property.Name, edit.Target)
				}
				found = member.Property
			}
			if found != nil {
				patches = append(patches, sourcePatch{start: int(found.Value.Span.Start.Offset), end: int(found.Value.Span.End.Offset), replacement: value, operation: patch.operation, target: patch.target})
				continue
			}
			missing = append(missing, PropertySpec{Name: property.Name, Value: property.Value})
		}
		if len(missing) != 0 {
			indent := childIndent(node)
			point, prefix, newline := insertionPoint(source, int(node.Span.End.Offset))
			var builder strings.Builder
			builder.WriteString(prefix)
			for _, property := range missing {
				value, renderErr := renderValue(property.Value)
				if renderErr != nil {
					return nil, patch.target, renderErr
				}
				builder.WriteString(strings.Repeat(" ", indent))
				builder.WriteString(property.Name)
				builder.WriteString(": ")
				builder.WriteString(value)
				builder.WriteString(newline)
			}
			patches = append(patches, sourcePatch{start: point, end: point, replacement: builder.String(), operation: patch.operation, target: patch.target})
		}
		return patches, patch.target, nil

	case AppendProperty:
		patch.target = edit.Target
		node, err := targetNode(index, edit.Target)
		if err != nil {
			return nil, patch.target, err
		}
		if !validPropertyName(edit.Name) {
			return nil, patch.target, fmt.Errorf("property name %q is not a .paper identifier", edit.Name)
		}
		value, err := renderValue(edit.Value)
		if err != nil {
			return nil, patch.target, err
		}
		indent := childIndent(node)
		point, prefix, newline := insertionPoint(source, int(node.Span.End.Offset))
		patch.start, patch.end = point, point
		patch.replacement = prefix + strings.Repeat(" ", indent) + edit.Name + ": " + value + newline
		return []sourcePatch{patch}, patch.target, nil

	case ReplaceText:
		patch.target = edit.Target
		node, err := targetNode(index, edit.Target)
		if err != nil {
			return nil, patch.target, err
		}
		if node.Kind != paperlang.NodeText || node.Value == nil {
			return nil, patch.target, fmt.Errorf("target %s is not a text node with an inline value", edit.Target)
		}
		patch.start, patch.end = int(node.Value.Span.Start.Offset), int(node.Value.Span.End.Offset)
		patch.replacement = strconv.Quote(edit.Text)
		return []sourcePatch{patch}, patch.target, nil

	case InsertNode:
		patch.target = edit.Parent
		parent, err := targetNode(index, edit.Parent)
		if err != nil {
			return nil, patch.target, err
		}
		indent := childIndent(parent)
		newline := newlineForOffset(source, int(parent.HeaderSpan.End.Offset))
		rendered, err := renderNode(edit.Node, indent, newline)
		if err != nil {
			return nil, patch.target, err
		}
		point, prefix, _ := insertionPoint(source, int(parent.Span.End.Offset))
		patch.start, patch.end = point, point
		patch.replacement = prefix + rendered
		return []sourcePatch{patch}, patch.target, nil

	case DeleteNode:
		patch.target = edit.Target
		node, err := targetNode(index, edit.Target)
		if err != nil {
			return nil, patch.target, err
		}
		patch.start = lineStart(source, int(node.HeaderSpan.Start.Offset))
		patch.end = lineEndIncludingNewline(source, int(node.Span.End.Offset))
		return []sourcePatch{patch}, patch.target, nil

	case RenameID:
		patch.target = edit.Target
		if _, err := targetNode(index, edit.Target); err != nil {
			return nil, patch.target, err
		}
		if !validReadableID(edit.NewID) {
			return nil, patch.target, fmt.Errorf("new ID %q is not a readable @id", edit.NewID)
		}
		span, exists := index.idSpans[edit.Target]
		if !exists {
			return nil, patch.target, fmt.Errorf("target %s has no readable ID declaration span", edit.Target)
		}
		patch.start, patch.end = int(span.Start.Offset), int(span.End.Offset)
		patch.replacement = edit.NewID
		return []sourcePatch{patch}, patch.target, nil

	case MoveNode:
		patch.target = edit.Target
		node, err := targetNode(index, edit.Target)
		if err != nil {
			return nil, patch.target, err
		}
		parent, err := targetNode(index, edit.NewParent)
		if err != nil {
			return nil, patch.target, err
		}
		if edit.Target == edit.NewParent {
			return nil, patch.target, errors.New("a node cannot be moved beneath itself")
		}
		deleteStart := lineStart(source, int(node.HeaderSpan.Start.Offset))
		deleteEnd := lineEndIncludingNewline(source, int(node.Span.End.Offset))
		parentOffset := int(parent.HeaderSpan.Start.Offset)
		if parentOffset >= deleteStart && parentOffset < deleteEnd {
			return nil, patch.target, errors.New("a node cannot be moved beneath its descendant")
		}
		block := source[deleteStart:deleteEnd]
		newline := newlineForOffset(source, int(parent.HeaderSpan.End.Offset))
		adjusted := reindentNodeBlock(block, int(node.HeaderSpan.Start.Column-1), childIndent(parent))
		if adjusted == "" {
			return nil, patch.target, errors.New("node source block is empty")
		}
		if !strings.HasSuffix(adjusted, "\n") {
			adjusted += newline
		}
		point, prefix, _ := insertionPoint(source, int(parent.Span.End.Offset))
		// When the destination is immediately followed by the moving block,
		// the insertion point is the removal boundary. Treat the move as one
		// replacement so the transactional overlap detector does not reject a
		// valid end-of-container drop.
		if point == deleteStart {
			patch.start, patch.end = deleteStart, deleteEnd
			patch.replacement = adjusted
			return []sourcePatch{patch}, patch.target, nil
		}
		remove := sourcePatch{start: deleteStart, end: deleteEnd, operation: operationIndex, target: edit.Target}
		insert := sourcePatch{start: point, end: point, replacement: prefix + adjusted, operation: operationIndex, target: edit.Target}
		return []sourcePatch{remove, insert}, patch.target, nil

	case WrapNode:
		patch.target = edit.Target
		node, err := targetNode(index, edit.Target)
		if err != nil {
			return nil, patch.target, err
		}
		parent := index.parents[edit.Target]
		if parent == nil {
			return nil, patch.target, errors.New("the document root cannot be wrapped")
		}
		if len(edit.Wrapper.Children) != 0 {
			return nil, patch.target, errors.New("a wrapper cannot declare children; the target becomes its sole child")
		}
		if edit.Wrapper.ID != "" && edit.Wrapper.ID == edit.Target {
			return nil, patch.target, errors.New("a wrapper cannot reuse the target node's readable ID")
		}
		if edit.Wrapper.Value != nil {
			return nil, patch.target, errors.New("a structural wrapper cannot have an inline value")
		}
		if !validNodeKind(edit.Wrapper.Kind) || edit.Wrapper.Kind == paperlang.NodeDocument || edit.Wrapper.Kind == paperlang.NodeText {
			return nil, patch.target, fmt.Errorf("node kind %q cannot be used as a structural wrapper", edit.Wrapper.Kind)
		}
		if !editorAllowedChild(parent.Kind, edit.Wrapper.Kind) {
			return nil, patch.target, fmt.Errorf("%s cannot contain wrapper %s", parent.Kind, edit.Wrapper.Kind)
		}
		if !editorAllowedChild(edit.Wrapper.Kind, node.Kind) {
			return nil, patch.target, fmt.Errorf("wrapper %s cannot contain %s", edit.Wrapper.Kind, node.Kind)
		}
		start := lineStart(source, int(node.HeaderSpan.Start.Offset))
		end := lineEndIncludingNewline(source, int(node.Span.End.Offset))
		if start < 0 || end < start || end > len(source) {
			return nil, patch.target, errors.New("target node source block is outside the source revision")
		}
		oldIndent := int(node.HeaderSpan.Start.Column - 1)
		newline := newlineForOffset(source, int(node.HeaderSpan.End.Offset))
		wrapper, err := renderNode(edit.Wrapper, oldIndent, newline)
		if err != nil {
			return nil, patch.target, err
		}
		nested := reindentNodeBlock(source[start:end], oldIndent, oldIndent+2)
		if nested == "" {
			return nil, patch.target, errors.New("target node source block is empty")
		}
		patch.start, patch.end = start, end
		patch.replacement = wrapper + nested
		return []sourcePatch{patch}, patch.target, nil

	case UnwrapNode:
		patch.target = edit.Target
		node, err := targetNode(index, edit.Target)
		if err != nil {
			return nil, patch.target, err
		}
		parent := index.parents[edit.Target]
		if parent == nil {
			return nil, patch.target, errors.New("the document root cannot be unwrapped")
		}
		children := make([]*paperlang.Node, 0, len(node.Members))
		for _, member := range node.Members {
			if member.Property != nil {
				return nil, patch.target, fmt.Errorf("wrapper %s has property %q; unwrapping would discard it", edit.Target, member.Property.Name)
			}
			if member.Node != nil {
				children = append(children, member.Node)
			}
		}
		if len(children) == 0 {
			return nil, patch.target, fmt.Errorf("wrapper %s has no child nodes to promote", edit.Target)
		}
		for _, child := range children {
			if !editorAllowedChild(parent.Kind, child.Kind) {
				return nil, patch.target, fmt.Errorf("%s cannot contain promoted %s child", parent.Kind, child.Kind)
			}
		}
		start := lineStart(source, int(node.HeaderSpan.Start.Offset))
		bodyStart := lineEndIncludingNewline(source, int(node.HeaderSpan.End.Offset))
		end := lineEndIncludingNewline(source, int(node.Span.End.Offset))
		if bodyStart < start || end < bodyStart || end > len(source) {
			return nil, patch.target, errors.New("wrapper source block is outside the source revision")
		}
		childSource := source[bodyStart:end]
		oldIndent := int(children[0].HeaderSpan.Start.Column - 1)
		newIndent := int(node.HeaderSpan.Start.Column - 1)
		promoted := reindentNodeBlock(childSource, oldIndent, newIndent)
		if promoted == "" {
			return nil, patch.target, errors.New("wrapper child source block is empty")
		}
		patch.start, patch.end = start, end
		patch.replacement = promoted
		return []sourcePatch{patch}, patch.target, nil

	case ReplaceNode:
		patch.target = edit.Target
		node, err := targetNode(index, edit.Target)
		if err != nil {
			return nil, patch.target, err
		}
		parent := index.parents[edit.Target]
		if parent == nil && edit.Node.Kind != paperlang.NodeDocument {
			return nil, patch.target, errors.New("the document root can only be replaced by a document node")
		}
		if parent != nil && !editorAllowedChild(parent.Kind, edit.Node.Kind) {
			return nil, patch.target, fmt.Errorf("%s cannot contain replacement %s", parent.Kind, edit.Node.Kind)
		}
		start := lineStart(source, int(node.HeaderSpan.Start.Offset))
		end := lineEndIncludingNewline(source, int(node.Span.End.Offset))
		newline := newlineForOffset(source, int(node.HeaderSpan.End.Offset))
		rendered, err := renderNode(edit.Node, int(node.HeaderSpan.Start.Column-1), newline)
		if err != nil {
			return nil, patch.target, err
		}
		if end == len(source) && !strings.HasSuffix(source[start:end], "\n") {
			rendered = strings.TrimSuffix(rendered, newline)
		}
		patch.start, patch.end = start, end
		patch.replacement = rendered
		return []sourcePatch{patch}, patch.target, nil

	case nil:
		return nil, patch.target, errors.New("operation is nil")
	default:
		return nil, patch.target, fmt.Errorf("unsupported operation type %T", operation)
	}
}

func editorAllowedChild(parent, child paperlang.NodeKind) bool {
	switch parent {
	case paperlang.NodeDocument:
		return child == paperlang.NodePage || child == paperlang.NodeComponent || child == paperlang.NodeSchema || child == paperlang.NodeScenario || child == paperlang.NodeTheme
	case paperlang.NodePage:
		return child == paperlang.NodeBody
	case paperlang.NodeBody:
		return child == paperlang.NodeHeading || child == paperlang.NodeParagraph || child == paperlang.NodeList ||
			child == paperlang.NodePageBreak || child == paperlang.NodeText || child == paperlang.NodeRow ||
			child == paperlang.NodeColumn || child == paperlang.NodeUse || child == paperlang.NodeRepeat
	case paperlang.NodeRow, paperlang.NodeColumn:
		return child == paperlang.NodeHeading || child == paperlang.NodeParagraph || child == paperlang.NodeUse
	case paperlang.NodeList:
		return child == paperlang.NodeItem
	case paperlang.NodeItem:
		return child == paperlang.NodeParagraph || child == paperlang.NodeText || child == paperlang.NodeUse
	case paperlang.NodeHeading, paperlang.NodeParagraph:
		return child == paperlang.NodeText
	case paperlang.NodeComponent:
		return editorComponentBodyKind(child) || child == paperlang.NodeSlot
	case paperlang.NodeSlot, paperlang.NodeFill:
		return editorComponentBodyKind(child)
	case paperlang.NodeUse:
		return child == paperlang.NodeFill
	case paperlang.NodeRepeat:
		return editorComponentBodyKind(child) || child == paperlang.NodeRepeat
	case paperlang.NodeScenario, paperlang.NodeObject, paperlang.NodeKeyedList:
		return child == paperlang.NodeValue || child == paperlang.NodeObject || child == paperlang.NodeKeyedList
	case paperlang.NodeSchema:
		return child == paperlang.NodeField
	default:
		return false
	}
}

func editorComponentBodyKind(kind paperlang.NodeKind) bool {
	return kind == paperlang.NodeHeading || kind == paperlang.NodeParagraph || kind == paperlang.NodeList ||
		kind == paperlang.NodePageBreak || kind == paperlang.NodeText || kind == paperlang.NodeRow ||
		kind == paperlang.NodeColumn || kind == paperlang.NodeUse
}

func targetNode(index sourceIndex, target string) (*paperlang.Node, error) {
	if target == "" || target[0] != '@' {
		return nil, fmt.Errorf("target %q is not a readable @id", target)
	}
	node := index.byID[target]
	if node == nil {
		return nil, fmt.Errorf("target %s was not found in the source revision", target)
	}
	return node, nil
}

func renderValue(value Value) (string, error) {
	switch value.Kind {
	case ValueString:
		return strconv.Quote(value.Text), nil
	case ValueBool:
		return strconv.FormatBool(value.Bool), nil
	case ValueNumber:
		if math.IsNaN(value.Number) || math.IsInf(value.Number, 0) {
			return "", errors.New("number value must be finite")
		}
		return strconv.FormatFloat(value.Number, 'g', -1, 64), nil
	case ValueUnit:
		if math.IsNaN(value.Number) || math.IsInf(value.Number, 0) || !validUnit(value.Text) {
			return "", fmt.Errorf("unit value %g%s is invalid", value.Number, value.Text)
		}
		return strconv.FormatFloat(value.Number, 'g', -1, 64) + value.Text, nil
	default:
		return "", fmt.Errorf("value kind %q is invalid", value.Kind)
	}
}

func renderNode(node NodeSpec, indent int, newline string) (string, error) {
	if !validNodeKind(node.Kind) {
		return "", fmt.Errorf("node kind %q is invalid", node.Kind)
	}
	if node.ID != "" && !validReadableID(node.ID) {
		return "", fmt.Errorf("node ID %q is not a readable @id", node.ID)
	}
	var builder strings.Builder
	pad := strings.Repeat(" ", indent)
	builder.WriteString(pad)
	builder.WriteString(string(node.Kind))
	if node.ID != "" {
		builder.WriteByte(' ')
		builder.WriteString(node.ID)
	}
	builder.WriteByte(':')
	if node.Value != nil {
		value, err := renderValue(*node.Value)
		if err != nil {
			return "", err
		}
		builder.WriteByte(' ')
		builder.WriteString(value)
	}
	builder.WriteString(newline)
	for _, property := range node.Properties {
		if !validPropertyName(property.Name) {
			return "", fmt.Errorf("property name %q is not a .paper identifier", property.Name)
		}
		value, err := renderValue(property.Value)
		if err != nil {
			return "", err
		}
		builder.WriteString(strings.Repeat(" ", indent+2))
		builder.WriteString(property.Name)
		builder.WriteString(": ")
		builder.WriteString(value)
		builder.WriteString(newline)
	}
	for _, child := range node.Children {
		rendered, err := renderNode(child, indent+2, newline)
		if err != nil {
			return "", err
		}
		builder.WriteString(rendered)
	}
	return builder.String(), nil
}

func validNodeKind(kind paperlang.NodeKind) bool {
	switch kind {
	case paperlang.NodeDocument, paperlang.NodePage, paperlang.NodeBody,
		paperlang.NodeHeading, paperlang.NodeText, paperlang.NodeParagraph,
		paperlang.NodeList, paperlang.NodeItem, paperlang.NodePageBreak,
		paperlang.NodeRow, paperlang.NodeColumn, paperlang.NodeComponent,
		paperlang.NodeSlot, paperlang.NodeUse, paperlang.NodeFill, paperlang.NodeRepeat,
		paperlang.NodeSchema, paperlang.NodeField, paperlang.NodeScenario, paperlang.NodeValue, paperlang.NodeObject, paperlang.NodeKeyedList:
		return true
	default:
		return false
	}
}

func validPropertyName(name string) bool {
	if name == "" || !identifierStart(name[0]) {
		return false
	}
	for index := 1; index < len(name); index++ {
		if !identifierContinue(name[index]) {
			return false
		}
	}
	return true
}

func identifierStart(value byte) bool {
	return value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z' || value == '_'
}

func identifierContinue(value byte) bool {
	return identifierStart(value) || value >= '0' && value <= '9' || value == '-'
}

func validReadableID(value string) bool {
	if len(value) < 2 || value[0] != '@' || !identifierStart(value[1]) || value[1] == '_' {
		return false
	}
	for index := 2; index < len(value); index++ {
		if !identifierContinue(value[index]) {
			return false
		}
	}
	return true
}

func validUnit(unit string) bool {
	switch unit {
	case "pt", "mm", "cm", "in", "px", "pc", "em", "rem", "vh", "vw", "%":
		return true
	default:
		return false
	}
}

func childIndent(node *paperlang.Node) int {
	for _, member := range node.Members {
		span := paperlang.Span{}
		if member.Node != nil {
			span = member.Node.HeaderSpan
		} else if member.Property != nil {
			span = member.Property.Span
		}
		if span.Start.Column > 0 {
			return int(span.Start.Column - 1)
		}
	}
	if node.HeaderSpan.Start.Column > 0 {
		return int(node.HeaderSpan.Start.Column-1) + 2
	}
	return 2
}

func newlineForOffset(source string, offset int) string {
	if offset < 0 {
		offset = 0
	}
	if offset > len(source) {
		offset = len(source)
	}
	lineEnd := strings.IndexByte(source[offset:], '\n')
	if lineEnd >= 0 {
		position := offset + lineEnd
		if position > 0 && source[position-1] == '\r' {
			return "\r\n"
		}
		return "\n"
	}
	if strings.Contains(source, "\r\n") {
		return "\r\n"
	}
	return "\n"
}

func insertionPoint(source string, spanEnd int) (point int, prefix, newline string) {
	newline = newlineForOffset(source, spanEnd)
	point = lineEndIncludingNewline(source, spanEnd)
	if point == len(source) && spanEnd == len(source) && len(source) > 0 && source[len(source)-1] != '\n' {
		prefix = newline
	}
	return point, prefix, newline
}

func lineStart(source string, offset int) int {
	if offset < 0 {
		return 0
	}
	if offset > len(source) {
		offset = len(source)
	}
	if index := strings.LastIndexByte(source[:offset], '\n'); index >= 0 {
		return index + 1
	}
	return 0
}

func lineEndIncludingNewline(source string, offset int) int {
	if offset < 0 {
		return 0
	}
	if offset > len(source) {
		return len(source)
	}
	if index := strings.IndexByte(source[offset:], '\n'); index >= 0 {
		return offset + index + 1
	}
	return len(source)
}

func reindentNodeBlock(block string, oldIndent, newIndent int) string {
	if block == "" || oldIndent < 0 || newIndent < 0 {
		return ""
	}
	var builder strings.Builder
	for start := 0; start < len(block); {
		end := strings.IndexByte(block[start:], '\n')
		if end < 0 {
			end = len(block)
		} else {
			end += start + 1
		}
		line := block[start:end]
		contentEnd := len(line)
		if contentEnd > 0 && line[contentEnd-1] == '\n' {
			contentEnd--
			if contentEnd > 0 && line[contentEnd-1] == '\r' {
				contentEnd--
			}
		}
		leading := 0
		for leading < contentEnd && line[leading] == ' ' {
			leading++
		}
		if leading == contentEnd {
			builder.WriteString(line)
		} else {
			relative := leading - oldIndent
			if relative < 0 {
				relative = 0
			}
			builder.WriteString(strings.Repeat(" ", newIndent+relative))
			builder.WriteString(line[leading:])
		}
		start = end
	}
	return builder.String()
}

func conflictingPatches(patches []sourcePatch) (sourcePatch, sourcePatch, bool) {
	ordered := append([]sourcePatch(nil), patches...)
	sort.Slice(ordered, func(first, second int) bool {
		if ordered[first].start != ordered[second].start {
			return ordered[first].start < ordered[second].start
		}
		if ordered[first].end != ordered[second].end {
			return ordered[first].end < ordered[second].end
		}
		return ordered[first].operation < ordered[second].operation
	})
	for first := 0; first < len(ordered); first++ {
		for second := first + 1; second < len(ordered); second++ {
			if ordered[second].start > ordered[first].end {
				break
			}
			if patchesOverlap(ordered[first], ordered[second]) {
				return ordered[first], ordered[second], true
			}
		}
	}
	return sourcePatch{}, sourcePatch{}, false
}

func patchesOverlap(first, second sourcePatch) bool {
	firstEmpty := first.start == first.end
	secondEmpty := second.start == second.end
	switch {
	case firstEmpty && secondEmpty:
		return first.start == second.start
	case firstEmpty:
		return first.start >= second.start && first.start < second.end
	case secondEmpty:
		return second.start >= first.start && second.start < first.end
	default:
		return first.start < second.end && second.start < first.end
	}
}

func applyPatches(source string, patches []sourcePatch) (string, error) {
	ordered := append([]sourcePatch(nil), patches...)
	sort.Slice(ordered, func(first, second int) bool {
		if ordered[first].start != ordered[second].start {
			return ordered[first].start > ordered[second].start
		}
		return ordered[first].operation > ordered[second].operation
	})
	result := source
	for _, patch := range ordered {
		if patch.start < 0 || patch.end < patch.start || patch.end > len(result) {
			return "", fmt.Errorf("operation %d resolved outside the source revision", patch.operation+1)
		}
		result = result[:patch.start] + patch.replacement + result[patch.end:]
	}
	return result, nil
}

func exportPatches(source string, patches []sourcePatch) []SourcePatch {
	ordered := append([]sourcePatch(nil), patches...)
	sort.Slice(ordered, func(first, second int) bool {
		if ordered[first].start != ordered[second].start {
			return ordered[first].start < ordered[second].start
		}
		if ordered[first].end != ordered[second].end {
			return ordered[first].end < ordered[second].end
		}
		return ordered[first].operation < ordered[second].operation
	})
	result := make([]SourcePatch, len(ordered))
	for index, patch := range ordered {
		removed := ""
		if patch.start >= 0 && patch.end >= patch.start && patch.end <= len(source) {
			removed = source[patch.start:patch.end]
		}
		result[index] = SourcePatch{
			Start: uint32(patch.start), End: uint32(patch.end), Removed: removed,
			Replacement: patch.replacement, Operation: uint32(patch.operation + 1), Target: patch.target,
		}
	}
	return result
}

func invalidationScope(index sourceIndex, operations []Operation) *InvalidationScope {
	ids := make(map[string]struct{})
	addNodeAndAncestors := func(id string) {
		node := index.byID[id]
		if node == nil {
			if id != "" {
				ids[id] = struct{}{}
			}
			return
		}
		for node != nil {
			if node.ID != "" {
				ids[node.ID] = struct{}{}
			}
			if node.ID == "" {
				break
			}
			node = index.parents[node.ID]
		}
	}
	addSubtree := func(rootID string) {
		root := index.byID[rootID]
		var walk func(*paperlang.Node)
		walk = func(node *paperlang.Node) {
			if node == nil {
				return
			}
			if node.ID != "" {
				ids[node.ID] = struct{}{}
			}
			for _, member := range node.Members {
				walk(member.Node)
			}
		}
		walk(root)
	}
	var addSpec func(NodeSpec)
	addSpec = func(spec NodeSpec) {
		if spec.ID != "" {
			ids[spec.ID] = struct{}{}
		}
		for _, child := range spec.Children {
			addSpec(child)
		}
	}
	for _, operation := range operations {
		switch edit := operation.(type) {
		case SetProperty:
			addNodeAndAncestors(edit.Target)
		case SetProperties:
			addNodeAndAncestors(edit.Target)
		case AppendProperty:
			addNodeAndAncestors(edit.Target)
		case ReplaceText:
			addNodeAndAncestors(edit.Target)
		case InsertNode:
			addNodeAndAncestors(edit.Parent)
			addSpec(edit.Node)
		case DeleteNode:
			addNodeAndAncestors(edit.Target)
			addSubtree(edit.Target)
		case RenameID:
			addNodeAndAncestors(edit.Target)
			if edit.NewID != "" {
				ids[edit.NewID] = struct{}{}
			}
		case MoveNode:
			addNodeAndAncestors(edit.Target)
			addSubtree(edit.Target)
			addNodeAndAncestors(edit.NewParent)
		case WrapNode:
			addNodeAndAncestors(edit.Target)
			addSubtree(edit.Target)
			addSpec(edit.Wrapper)
		case UnwrapNode:
			addNodeAndAncestors(edit.Target)
			addSubtree(edit.Target)
		case ReplaceNode:
			addNodeAndAncestors(edit.Target)
			addSubtree(edit.Target)
			addSpec(edit.Node)
		}
	}
	nodeIDs := make([]string, 0, len(ids))
	for id := range ids {
		nodeIDs = append(nodeIDs, id)
	}
	sort.Strings(nodeIDs)
	return &InvalidationScope{WholeDocument: true, NodeIDs: nodeIDs}
}
