// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

// Package paperd provides the bounded, in-process state model used by an
// agent-facing .paper workspace. It deliberately owns no network transport:
// callers authenticate, authorize, and serialize requests outside this
// package, then use the opaque workspace-scoped handles returned here.
package paperd

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/cssbruno/paperrune/document"
	"github.com/cssbruno/paperrune/internal/papercompile"
	"github.com/cssbruno/paperrune/internal/paperedit"
	"github.com/cssbruno/paperrune/internal/paperlang"
	"github.com/cssbruno/paperrune/internal/paperscenario"
)

const (
	MaxSourceBytesHard                = paperedit.MaxSourceBytes
	MaxRevisionsHard                  = 65536
	MaxScenarioRevisionsHard          = 65536
	MaxSemanticTemplateRevisionsHard  = 65536
	MaxPolicyRevisionsHard            = 65536
	MaxCandidatesHard                 = 4096
	MaxScenarioCandidatesHard         = 4096
	MaxSemanticTemplateCandidatesHard = 4096
	MaxPolicyCandidatesHard           = 4096
	MaxScenarioOperationsHard         = 1024
	MaxScenarioPathBytesHard          = 4096
	MaxScenarioValueNodesHard         = 100000
	MaxScenarioWorkHard               = 1000000
	MaxPlansHard                      = 4096
	MaxOpenDocumentsHard              = 4096
	MaxNodesHard                      = 100000
	MaxSearchResultsHard              = 256
	MaxQueryBytesHard                 = 4096
	MaxRenderBytesHard                = 64 << 20
	MaxFileBytesHard                  = 4096
	MaxPlanTTLHard                    = 24 * time.Hour
	MaxContextBytesHard               = 4 << 20
	MaxRevocationsHard                = 65536
	MaxHandleTTLHard                  = 24 * time.Hour
	MaxPersistenceBytesHard           = 64 << 20
	MaxMutationAuthoritiesHard        = 4096
	MaxAuthorizationEffectsHard       = 100000
	MaxAuthorizationAuditHard         = 65536
)

// Limits bounds both retained workspace state and individual operations.
// Zero fields receive conservative defaults in NewWorkspace.
type Limits struct {
	MaxSourceBytes                int
	MaxRevisions                  int
	MaxScenarioRevisions          int
	MaxSemanticTemplateRevisions  int
	MaxPolicyRevisions            int
	MaxCandidates                 int
	MaxScenarioCandidates         int
	MaxSemanticTemplateCandidates int
	MaxPolicyCandidates           int
	MaxPlans                      int
	MaxOpenDocuments              int
	MaxNodes                      int
	MaxSearchResults              int
	MaxQueryBytes                 int
	MaxRenderBytes                int
	MaxFileBytes                  int
	MaxOperations                 int
	MaxScenarioOperations         int
	MaxScenarioPathBytes          int
	MaxScenarioValueNodes         int
	MaxScenarioWork               int
	MaxContextBytes               int
	MaxRevocations                int
	MaxPersistenceBytes           int
	MaxMutationAuthorities        int
	MaxAuthorizationEffects       int
	MaxAuthorizationAudit         int
}

func DefaultLimits() Limits {
	return Limits{
		MaxSourceBytes: 1 << 20, MaxRevisions: 1024, MaxScenarioRevisions: 1024, MaxSemanticTemplateRevisions: 1024, MaxPolicyRevisions: 1024,
		MaxCandidates: 128, MaxScenarioCandidates: 128, MaxSemanticTemplateCandidates: 128, MaxPolicyCandidates: 128, MaxPlans: 128, MaxOpenDocuments: 128,
		MaxNodes: 20000, MaxSearchResults: 128, MaxQueryBytes: 1024,
		MaxRenderBytes: 16 << 20, MaxFileBytes: 1024, MaxOperations: 128,
		MaxScenarioOperations: 128, MaxScenarioPathBytes: 1024, MaxScenarioValueNodes: 20000, MaxScenarioWork: 100000, MaxContextBytes: 64 << 10,
		MaxRevocations: 1024, MaxPersistenceBytes: 8 << 20,
		MaxMutationAuthorities: 128, MaxAuthorizationEffects: 20000, MaxAuthorizationAudit: 4096,
	}
}

var (
	ErrInvalidLimits       = errors.New("paperd: invalid limits")
	ErrInvalidHandle       = errors.New("paperd: invalid handle")
	ErrWrongWorkspace      = errors.New("paperd: handle belongs to another workspace")
	ErrRevisionNotFound    = errors.New("paperd: revision not found")
	ErrCandidateNotFound   = errors.New("paperd: candidate not found")
	ErrPlanNotFound        = errors.New("paperd: plan not found")
	ErrPlanExpired         = errors.New("paperd: plan handle expired")
	ErrRevisionConflict    = errors.New("paperd: revision conflict")
	ErrInvalidSource       = errors.New("paperd: source is invalid")
	ErrLimit               = errors.New("paperd: limit exceeded")
	ErrInvalidQuery        = errors.New("paperd: invalid query")
	ErrHandleExpired       = errors.New("paperd: handle expired")
	ErrHandleRevoked       = errors.New("paperd: handle revoked")
	ErrDisclosureDenied    = errors.New("paperd: disclosure domain denied")
	ErrPersistence         = errors.New("paperd: persistence failure")
	ErrPersistenceCorrupt  = errors.New("paperd: persisted workspace is corrupt")
	ErrPersistenceConflict = errors.New("paperd: persisted workspace generation conflict")
)

// Error is a stable machine-readable workspace error. Message contains no
// process-specific handle values, so equivalent failures are deterministic.
type Error struct {
	Code       string                      `json:"code"`
	Message    string                      `json:"message"`
	Candidates []paperedit.TargetCandidate `json:"candidates,omitempty"`
	cause      error
}

func (e *Error) Error() string { return "paperd: " + e.Code + ": " + e.Message }
func (e *Error) Unwrap() error { return e.cause }

func workspaceError(code, message string, cause error) error {
	return &Error{Code: code, Message: message, cause: cause}
}

type scopedHandle struct {
	scope      uint64
	serial     uint64
	nonce      uint64
	domain     uint64
	kind       handleKind
	capability handleCapability
}

// RevisionHandle is an opaque reference to one immutable source snapshot.
// Its fields are intentionally private; only the creating Workspace can open
// it. The zero value is invalid.
type RevisionHandle struct{ value scopedHandle }

// ScenarioRevisionHandle belongs to the fixture domain and cannot be used as
// a source revision handle even when its serial happens to match.
type ScenarioRevisionHandle struct{ value scopedHandle }

// ScenarioCandidateHandle is an opaque mutable head in the scenario domain.
// It is not interchangeable with source candidates or scenario revisions.
type ScenarioCandidateHandle struct{ value scopedHandle }

// SemanticTemplateRevisionHandle belongs only to immutable semantic-template
// documents and is never interchangeable with source or scenario revisions.
type SemanticTemplateRevisionHandle struct{ value scopedHandle }

type SemanticTemplateCandidateHandle struct{ value scopedHandle }

// PolicyRevisionHandle belongs only to immutable authored policy documents.
// WorkspaceOptions.PolicyRevision remains the outer cache-partition identity.
type PolicyRevisionHandle struct{ value scopedHandle }

type PolicyCandidateHandle struct{ value scopedHandle }

// CandidateHandle is an opaque reference to one mutable head pointer. Source
// revisions referenced by the head remain immutable.
type CandidateHandle struct{ value scopedHandle }

// PlanHandle is an opaque reference to one immutable plan derived from one
// exact retained source revision.
type PlanHandle struct{ value scopedHandle }

// OpenHandle is an opaque, immutable capability pinned to one exact source
// revision. It never follows a candidate head implicitly.
type OpenHandle struct{ value scopedHandle }

// MutationAuthorityHandle is an operation- and node-scoped actor grant. It is
// separate from an edit-capable OpenHandle and cannot edit by itself.
type MutationAuthorityHandle struct{ value scopedHandle }

// SensitiveAuthorityHandle is a single-operation capability. Candidate
// acceptance, export, publish, attachment, production capture, and signing
// handles are deliberately not interchangeable, even for the same actor.
type SensitiveAuthorityHandle struct{ value scopedHandle }

// SensitiveApprovalHandle is a one-use approval bound to exact review
// evidence. Its opaque identity is not an evidence or source digest.
type SensitiveApprovalHandle struct{ value scopedHandle }

var nextWorkspaceScope atomic.Uint64

type revisionRecord struct {
	handle     RevisionHandle
	file       string
	source     string
	revision   paperedit.Revision
	parsed     paperlang.ParseResult
	compiled   papercompile.Result
	nodes      int
	expires    time.Time
	disclosure DisclosureDomain
	partition  cachePartition
}

type candidateRecord struct {
	handle                CandidateHandle
	head                  RevisionHandle
	journal               *paperedit.Journal
	idempotency           map[string]sourceIdempotencyRecord
	acceptance            *candidateAcceptanceRecord
	acceptanceIdempotency map[string]candidateAcceptanceIdempotencyRecord
	expires               time.Time
	disclosure            DisclosureDomain
	partition             cachePartition
}

type planRecord struct {
	handle     PlanHandle
	revision   RevisionHandle
	digest     paperedit.Revision
	plan       document.PaperPlan
	result     document.PaperPlanResult
	expires    time.Time
	disclosure DisclosureDomain
	partition  cachePartition
}

type openRecord struct {
	handle     OpenHandle
	candidate  CandidateHandle
	revision   RevisionHandle
	digest     paperedit.Revision
	mode       CapabilityMode
	expires    time.Time
	disclosure DisclosureDomain
	partition  cachePartition
}

type scenarioRevisionRecord struct {
	handle     ScenarioRevisionHandle
	fixtures   []paperscenario.Fixture
	digest     string
	expires    time.Time
	disclosure DisclosureDomain
	partition  cachePartition
}

type scenarioCandidateRecord struct {
	handle      ScenarioCandidateHandle
	head        ScenarioRevisionHandle
	idempotency map[string]scenarioIdempotencyRecord
	expires     time.Time
	disclosure  DisclosureDomain
	partition   cachePartition
}

type semanticTemplateRevisionRecord struct {
	handle     SemanticTemplateRevisionHandle
	content    string
	digest     string
	expires    time.Time
	disclosure DisclosureDomain
	partition  cachePartition
}

type semanticTemplateCandidateRecord struct {
	handle      SemanticTemplateCandidateHandle
	head        SemanticTemplateRevisionHandle
	idempotency map[string]semanticTemplateIdempotencyRecord
	expires     time.Time
	disclosure  DisclosureDomain
	partition   cachePartition
}

type policyRevisionRecord struct {
	handle     PolicyRevisionHandle
	content    string
	digest     string
	expires    time.Time
	disclosure DisclosureDomain
	partition  cachePartition
}

type policyCandidateRecord struct {
	handle      PolicyCandidateHandle
	head        PolicyRevisionHandle
	idempotency map[string]policyIdempotencyRecord
	expires     time.Time
	disclosure  DisclosureDomain
	partition   cachePartition
}

type mutationAuthorityRecord struct {
	handle         MutationAuthorityHandle
	open           OpenHandle
	candidate      CandidateHandle
	actor          string
	operations     map[MutationOperation]struct{}
	nodeScopes     map[string]struct{}
	protectedNodes map[string]struct{}
	expires        time.Time
	disclosure     DisclosureDomain
	partition      cachePartition
}

type sensitiveAuthorityRecord struct {
	handle     SensitiveAuthorityHandle
	open       OpenHandle
	candidate  CandidateHandle
	actor      string
	operation  SensitiveOperation
	expires    time.Time
	disclosure DisclosureDomain
	partition  cachePartition
}

type sensitiveApprovalRecord struct {
	handle       SensitiveApprovalHandle
	authority    SensitiveAuthorityHandle
	candidate    CandidateHandle
	expectedHead RevisionHandle
	operation    SensitiveOperation
	actor        string
	policy       string
	evidenceHash string
	used         bool
	expires      time.Time
	disclosure   DisclosureDomain
	partition    cachePartition
}

// Workspace is safe for concurrent use. Parsing, compilation, editing, and
// rendering are performed without holding its state mutex; candidate commits
// use a compare-and-swap check when they publish a new immutable revision.
type Workspace struct {
	mu                            sync.RWMutex
	scope                         uint64
	limits                        Limits
	nextRevision                  uint64
	nextScenarioRevision          uint64
	nextSemanticTemplateRevision  uint64
	nextPolicyRevision            uint64
	nextCandidate                 uint64
	nextScenarioCandidate         uint64
	nextSemanticTemplateCandidate uint64
	nextPolicyCandidate           uint64
	nextPlan                      uint64
	nextOpen                      uint64
	nextMutationAuthority         uint64
	nextSensitiveAuthority        uint64
	nextSensitiveApproval         uint64
	revisions                     map[uint64]*revisionRecord
	scenarioRevisions             map[uint64]*scenarioRevisionRecord
	semanticTemplateRevisions     map[uint64]*semanticTemplateRevisionRecord
	policyRevisions               map[uint64]*policyRevisionRecord
	candidates                    map[uint64]*candidateRecord
	scenarioCandidates            map[uint64]*scenarioCandidateRecord
	semanticTemplateCandidates    map[uint64]*semanticTemplateCandidateRecord
	policyCandidates              map[uint64]*policyCandidateRecord
	plans                         map[uint64]*planRecord
	opens                         map[uint64]*openRecord
	mutationAuthorities           map[uint64]*mutationAuthorityRecord
	sensitiveAuthorities          map[uint64]*sensitiveAuthorityRecord
	sensitiveApprovals            map[uint64]*sensitiveApprovalRecord
	planTTL                       time.Duration
	handleTTL                     time.Duration
	now                           func() time.Time
	disclosureDomain              DisclosureDomain
	disclosureTag                 uint64
	revocations                   map[scopedHandle]revocationRecord
	revocationOrder               []scopedHandle
	projectID                     string
	policyRevision                string
	partition                     cachePartition
	persistenceRoot               string
	persistenceAuthenticationKey  []byte
	persistenceGeneration         uint64
	disclosureAuditSink           func(DisclosureAuditEntry)
	disclosureAudit               []DisclosureAuditEntry
	nextDisclosureAudit           uint64
	requireMutationAuthority      bool
	protectedNodeIDs              map[string]struct{}
	authorizationAudit            []AuthorizationAuditEntry
	nextAuthorizationAudit        uint64
	sensitiveApprovalNonces       map[[32]byte]struct{}
	sensitiveAudit                []SensitiveAuditEntry
	nextSensitiveAudit            uint64
	sensitiveAuditRoot            string
	sensitiveAuditAnchors         []SensitiveAuditAnchor
	acceptancePolicy              CandidateAcceptancePolicy
	acceptancePolicyHash          string
	assetCatalog                  papercompile.AssetCatalog
	importResolver                papercompile.ImportResolver
}

func NewWorkspace(limits Limits) (*Workspace, error) {
	return NewWorkspaceWithOptions(WorkspaceOptions{Limits: limits})
}

// WorkspaceOptions keeps lifecycle policy separate from deterministic source
// and plan inputs. Now is injectable for tests and embedders; it is never part
// of plan identity or layout output.
type WorkspaceOptions struct {
	Limits           Limits
	PlanTTL          time.Duration
	HandleTTL        time.Duration
	Now              func() time.Time
	DisclosureDomain DisclosureDomain
	ProjectID        string
	PolicyRevision   string
	PersistenceRoot  string
	// PersistenceAuthenticationKey authenticates snapshot manifests with
	// HMAC-SHA-256. It must contain at least 32 bytes when set and is retained
	// in memory only. Supplying it on recovery rejects unsigned generations;
	// omitting it preserves compatibility with existing local snapshots.
	PersistenceAuthenticationKey []byte
	// DisclosureAuditSink receives detached hash-only denial records. The
	// callback is best-effort and panic-isolated; raw disclosure labels,
	// capabilities, source, and payloads are never supplied to it.
	DisclosureAuditSink      func(DisclosureAuditEntry)
	RequireMutationAuthority bool
	ProtectedNodeIDs         []string
	CandidateAcceptance      CandidateAcceptancePolicy
	// AssetResources is an explicit immutable catalog used only for semantic
	// compilation; the workspace never searches paths or the network.
	AssetResources []papercompile.AssetResource
	// ImportResolver is the explicit source boundary for reusable .paper
	// themes and styles. It is never inferred from ambient process state.
	ImportResolver papercompile.ImportResolver
}

func NewWorkspaceWithOptions(options WorkspaceOptions) (*Workspace, error) {
	limits := options.Limits
	normalized, err := normalizeLimits(limits)
	if err != nil {
		return nil, err
	}
	planTTL := options.PlanTTL
	if planTTL == 0 {
		planTTL = 30 * time.Minute
	}
	if planTTL < 0 || planTTL > MaxPlanTTLHard {
		return nil, workspaceError("INVALID_LIMITS", "PlanTTL must be positive and no greater than 24 hours", ErrInvalidLimits)
	}
	handleTTL := options.HandleTTL
	if handleTTL == 0 {
		handleTTL = 30 * time.Minute
	}
	if handleTTL < 0 || handleTTL > MaxHandleTTLHard {
		return nil, workspaceError("INVALID_LIMITS", "HandleTTL must be positive and no greater than 24 hours", ErrInvalidLimits)
	}
	disclosureDomain, disclosureTag, err := normalizeDisclosureDomain(options.DisclosureDomain)
	if err != nil {
		emitDisclosureAudit(options.DisclosureAuditSink, DisclosureAuditEntry{At: time.Now().UTC(), Action: "workspace.create", RequestedHash: disclosureIdentityHash(string(options.DisclosureDomain)), Reason: "invalid_disclosure"})
		return nil, err
	}
	projectID, policyRevision, partition, err := normalizeCachePartition(options.ProjectID, options.PolicyRevision, disclosureDomain)
	if err != nil {
		return nil, err
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	protectedNodeIDs, err := normalizeProtectedNodeIDs(options.ProtectedNodeIDs, normalized.MaxAuthorizationEffects)
	if err != nil {
		return nil, err
	}
	acceptancePolicy, acceptancePolicyHash, err := normalizeCandidateAcceptancePolicy(options.CandidateAcceptance, normalized.MaxQueryBytes)
	if err != nil {
		return nil, err
	}
	assetCatalog, err := papercompile.NewAssetCatalog(options.AssetResources)
	if err != nil {
		return nil, workspaceError("INVALID_ASSET_CATALOG", "workspace asset catalog is invalid", err)
	}
	if len(options.PersistenceAuthenticationKey) != 0 && (len(options.PersistenceAuthenticationKey) < 32 || len(options.PersistenceAuthenticationKey) > normalized.MaxQueryBytes) {
		return nil, workspaceError("INVALID_PERSISTENCE_KEY", "persistence authentication key must contain between 32 bytes and the configured query limit", ErrInvalidLimits)
	}
	scope := nextWorkspaceScope.Add(1)
	return &Workspace{
		scope: scope, limits: normalized, planTTL: planTTL, handleTTL: handleTTL, now: now,
		disclosureDomain: disclosureDomain, disclosureTag: disclosureTag,
		projectID: projectID, policyRevision: policyRevision, partition: partition, persistenceRoot: options.PersistenceRoot,
		persistenceAuthenticationKey: append([]byte(nil), options.PersistenceAuthenticationKey...),
		disclosureAuditSink:          options.DisclosureAuditSink,
		revisions:                    make(map[uint64]*revisionRecord),
		scenarioRevisions:            make(map[uint64]*scenarioRevisionRecord),
		semanticTemplateRevisions:    make(map[uint64]*semanticTemplateRevisionRecord),
		policyRevisions:              make(map[uint64]*policyRevisionRecord),
		candidates:                   make(map[uint64]*candidateRecord),
		scenarioCandidates:           make(map[uint64]*scenarioCandidateRecord),
		semanticTemplateCandidates:   make(map[uint64]*semanticTemplateCandidateRecord),
		policyCandidates:             make(map[uint64]*policyCandidateRecord),
		plans:                        make(map[uint64]*planRecord),
		opens:                        make(map[uint64]*openRecord),
		mutationAuthorities:          make(map[uint64]*mutationAuthorityRecord),
		sensitiveAuthorities:         make(map[uint64]*sensitiveAuthorityRecord),
		sensitiveApprovals:           make(map[uint64]*sensitiveApprovalRecord),
		sensitiveApprovalNonces:      make(map[[32]byte]struct{}),
		revocations:                  make(map[scopedHandle]revocationRecord),
		requireMutationAuthority:     options.RequireMutationAuthority || len(protectedNodeIDs) != 0,
		protectedNodeIDs:             protectedNodeIDs,
		acceptancePolicy:             acceptancePolicy,
		acceptancePolicyHash:         acceptancePolicyHash,
		assetCatalog:                 assetCatalog,
		importResolver:               options.ImportResolver,
	}, nil
}

func normalizeLimits(limits Limits) (Limits, error) {
	defaults := DefaultLimits()
	values := []*int{
		&limits.MaxSourceBytes, &limits.MaxRevisions, &limits.MaxScenarioRevisions, &limits.MaxSemanticTemplateRevisions, &limits.MaxPolicyRevisions,
		&limits.MaxCandidates, &limits.MaxScenarioCandidates, &limits.MaxSemanticTemplateCandidates, &limits.MaxPolicyCandidates, &limits.MaxPlans, &limits.MaxOpenDocuments,
		&limits.MaxNodes, &limits.MaxSearchResults, &limits.MaxQueryBytes,
		&limits.MaxRenderBytes, &limits.MaxFileBytes, &limits.MaxOperations,
		&limits.MaxScenarioOperations, &limits.MaxScenarioPathBytes, &limits.MaxScenarioValueNodes, &limits.MaxScenarioWork, &limits.MaxContextBytes, &limits.MaxRevocations, &limits.MaxPersistenceBytes,
		&limits.MaxMutationAuthorities, &limits.MaxAuthorizationEffects, &limits.MaxAuthorizationAudit,
	}
	defaultValues := []int{
		defaults.MaxSourceBytes, defaults.MaxRevisions, defaults.MaxScenarioRevisions, defaults.MaxSemanticTemplateRevisions, defaults.MaxPolicyRevisions,
		defaults.MaxCandidates, defaults.MaxScenarioCandidates, defaults.MaxSemanticTemplateCandidates, defaults.MaxPolicyCandidates, defaults.MaxPlans, defaults.MaxOpenDocuments,
		defaults.MaxNodes, defaults.MaxSearchResults, defaults.MaxQueryBytes,
		defaults.MaxRenderBytes, defaults.MaxFileBytes, defaults.MaxOperations,
		defaults.MaxScenarioOperations, defaults.MaxScenarioPathBytes, defaults.MaxScenarioValueNodes, defaults.MaxScenarioWork, defaults.MaxContextBytes, defaults.MaxRevocations, defaults.MaxPersistenceBytes,
		defaults.MaxMutationAuthorities, defaults.MaxAuthorizationEffects, defaults.MaxAuthorizationAudit,
	}
	maximums := []int{
		MaxSourceBytesHard, MaxRevisionsHard, MaxScenarioRevisionsHard, MaxSemanticTemplateRevisionsHard, MaxPolicyRevisionsHard,
		MaxCandidatesHard, MaxScenarioCandidatesHard, MaxSemanticTemplateCandidatesHard, MaxPolicyCandidatesHard, MaxPlansHard, MaxOpenDocumentsHard, MaxNodesHard,
		MaxSearchResultsHard, MaxQueryBytesHard, MaxRenderBytesHard,
		MaxFileBytesHard, paperedit.MaxOperations,
		MaxScenarioOperationsHard, MaxScenarioPathBytesHard, MaxScenarioValueNodesHard, MaxScenarioWorkHard, MaxContextBytesHard, MaxRevocationsHard, MaxPersistenceBytesHard,
		MaxMutationAuthoritiesHard, MaxAuthorizationEffectsHard, MaxAuthorizationAuditHard,
	}
	names := []string{
		"MaxSourceBytes", "MaxRevisions", "MaxScenarioRevisions", "MaxSemanticTemplateRevisions", "MaxPolicyRevisions",
		"MaxCandidates", "MaxScenarioCandidates", "MaxSemanticTemplateCandidates", "MaxPolicyCandidates", "MaxPlans", "MaxOpenDocuments", "MaxNodes",
		"MaxSearchResults", "MaxQueryBytes", "MaxRenderBytes", "MaxFileBytes",
		"MaxOperations", "MaxScenarioOperations", "MaxScenarioPathBytes", "MaxScenarioValueNodes", "MaxScenarioWork", "MaxContextBytes", "MaxRevocations", "MaxPersistenceBytes",
		"MaxMutationAuthorities", "MaxAuthorizationEffects", "MaxAuthorizationAudit",
	}
	for index, value := range values {
		if *value == 0 {
			*value = defaultValues[index]
		}
		if *value < 1 || *value > maximums[index] {
			return Limits{}, workspaceError("INVALID_LIMITS", fmt.Sprintf("%s must be between 1 and %d", names[index], maximums[index]), ErrInvalidLimits)
		}
	}
	return limits, nil
}

// CreateRevision parses and retains an immutable copy of source. Invalid
// syntax is still a useful agent revision and is retained with diagnostics;
// Compile and Render reject it until an edit produces a valid candidate.
func (w *Workspace) CreateRevision(file, source string) (RevisionSnapshot, error) {
	if w == nil {
		return RevisionSnapshot{}, workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	record, err := w.prepareRevision(file, source)
	if err != nil {
		return RevisionSnapshot{}, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pruneExpiredHandlesLocked(w.now())
	if len(w.revisions) >= w.limits.MaxRevisions {
		return RevisionSnapshot{}, workspaceError("REVISION_LIMIT", "workspace revision capacity is exhausted", ErrLimit)
	}
	w.nextRevision++
	record.handle = RevisionHandle{value: w.newHandle(handleRevision, capabilityRead, w.nextRevision)}
	record.expires = w.expiresAt(w.handleTTL)
	record.disclosure = w.disclosureDomain
	record.partition = w.partition
	w.revisions[w.nextRevision] = record
	return snapshotOf(record), nil
}

func (w *Workspace) prepareRevision(file, source string) (*revisionRecord, error) {
	if len(file) == 0 || len(file) > w.limits.MaxFileBytes {
		return nil, workspaceError("FILE_LIMIT", "file name must be non-empty and within the configured byte limit", ErrLimit)
	}
	if len(source) > w.limits.MaxSourceBytes {
		return nil, workspaceError("SOURCE_LIMIT", "source exceeds the configured byte limit", ErrLimit)
	}
	parsed := paperlang.Parse(file, source)
	nodes := countASTNodes(parsed.AST.Root, w.limits.MaxNodes+1)
	if nodes > w.limits.MaxNodes {
		return nil, workspaceError("NODE_LIMIT", "source exceeds the configured syntax-node limit", ErrLimit)
	}
	record := &revisionRecord{
		file: file, source: source, revision: paperedit.SourceRevision(source),
		parsed: parsed, nodes: nodes,
	}
	if parsed.OK() {
		record.compiled = papercompile.CompileWithAssetsAndResolver(parsed.AST, w.assetCatalog, w.importResolver)
	}
	return record, nil
}

// OpenRevision returns a detached snapshot; changing its diagnostic slices
// cannot mutate retained workspace state.
func (w *Workspace) OpenRevision(handle RevisionHandle) (RevisionSnapshot, error) {
	record, err := w.revision(handle)
	if err != nil {
		return RevisionSnapshot{}, err
	}
	return snapshotOf(record), nil
}

func (w *Workspace) NewCandidate(base RevisionHandle) (CandidateSnapshot, error) {
	baseRecord, err := w.revision(base)
	if err != nil {
		return CandidateSnapshot{}, err
	}
	journal, err := paperedit.NewJournal(baseRecord.file, baseRecord.source, w.journalLimits())
	if err != nil {
		return CandidateSnapshot{}, workspaceError("JOURNAL_LIMIT", "candidate working-copy journal cannot retain its initial source", ErrLimit)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pruneExpiredHandlesLocked(w.now())
	if _, err := w.revisionLocked(base); err != nil {
		return CandidateSnapshot{}, err
	}
	if len(w.candidates) >= w.limits.MaxCandidates {
		return CandidateSnapshot{}, workspaceError("CANDIDATE_LIMIT", "workspace candidate capacity is exhausted", ErrLimit)
	}
	w.nextCandidate++
	handle := CandidateHandle{value: w.newHandle(handleCandidate, capabilityEdit, w.nextCandidate)}
	record := &candidateRecord{handle: handle, head: base, journal: journal, idempotency: make(map[string]sourceIdempotencyRecord), acceptanceIdempotency: make(map[string]candidateAcceptanceIdempotencyRecord), expires: w.expiresAt(w.handleTTL), disclosure: w.disclosureDomain, partition: w.partition}
	w.candidates[w.nextCandidate] = record
	return snapshotCandidate(record), nil
}

func (w *Workspace) Candidate(handle CandidateHandle) (CandidateSnapshot, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	record, err := w.candidateLocked(handle)
	if err != nil {
		return CandidateSnapshot{}, err
	}
	if _, err := w.revisionLocked(record.head); err != nil {
		return CandidateSnapshot{}, err
	}
	return snapshotCandidate(record), nil
}

// ApplyRequest requires both the expected opaque head and its exact SHA-256
// source revision. This prevents stale or accidentally cross-document edits.
type ApplyRequest struct {
	Candidate           CandidateHandle
	ExpectedHead        RevisionHandle
	ExpectedRevision    paperedit.Revision
	IdempotencyKey      string
	Group               string
	TargetPreconditions []paperedit.TargetPrecondition
	Operations          []paperedit.Operation
}

func (w *Workspace) journalLimits() paperedit.JournalLimits {
	limits := paperedit.DefaultJournalLimits()
	if limits.MaxEntries > w.limits.MaxRevisions {
		limits.MaxEntries = w.limits.MaxRevisions
	}
	if limits.MaxBytes < w.limits.MaxSourceBytes {
		limits.MaxBytes = w.limits.MaxSourceBytes
	}
	return limits
}

type ApplyResult struct {
	Candidate CandidateSnapshot
	Revision  RevisionSnapshot
	Edit      paperedit.Result
}

func (w *Workspace) Apply(request ApplyRequest) (ApplyResult, error) {
	if len(request.Operations) == 0 || len(request.Operations) > w.limits.MaxOperations {
		return ApplyResult{}, workspaceError("OPERATION_LIMIT", "operation count is outside the configured bounds", ErrLimit)
	}
	if len(request.Group) > w.limits.MaxQueryBytes || !utf8.ValidString(request.Group) {
		return ApplyResult{}, workspaceError("GROUP_LIMIT", "semantic edit group exceeds the configured query limit", ErrLimit)
	}
	operations := cloneOperations(request.Operations)
	preconditions := append([]paperedit.TargetPrecondition(nil), request.TargetPreconditions...)
	fingerprint, err := sourceApplyFingerprint(request, operations, preconditions)
	if err != nil {
		return ApplyResult{}, err
	}
	w.mu.RLock()
	candidate, err := w.candidateLocked(request.Candidate)
	if err != nil {
		w.mu.RUnlock()
		return ApplyResult{}, err
	}
	if cached, exists := candidate.idempotency[request.IdempotencyKey]; request.IdempotencyKey != "" && exists {
		w.mu.RUnlock()
		return sourceCachedResult(cached, fingerprint)
	}
	if candidate.head != request.ExpectedHead {
		w.mu.RUnlock()
		return ApplyResult{}, workspaceError("REVISION_CONFLICT", "candidate head changed", ErrRevisionConflict)
	}
	base, err := w.revisionLocked(request.ExpectedHead)
	if err != nil {
		w.mu.RUnlock()
		return ApplyResult{}, err
	}
	file, source, revision := base.file, base.source, base.revision
	w.mu.RUnlock()
	if request.ExpectedRevision != revision {
		return ApplyResult{}, workspaceError("REVISION_CONFLICT", "exact source revision does not match the candidate head", ErrRevisionConflict)
	}

	edit, editErr := paperedit.Apply(paperedit.Transaction{
		File: file, Source: source, ExpectedRevision: revision,
		IdempotencyKey: request.IdempotencyKey, TargetPreconditions: preconditions,
		RequireExactTargets: true, Operations: operations,
	})
	if editErr != nil {
		return ApplyResult{Edit: cloneEditResult(edit)}, wrapEditError(editErr)
	}
	prepared, err := w.prepareRevision(file, edit.Source)
	if err != nil {
		return ApplyResult{Edit: cloneEditResult(edit)}, err
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	w.pruneExpiredHandlesLocked(w.now())
	candidate, err = w.candidateLocked(request.Candidate)
	if err != nil {
		return ApplyResult{Edit: cloneEditResult(edit)}, err
	}
	if cached, exists := candidate.idempotency[request.IdempotencyKey]; request.IdempotencyKey != "" && exists {
		return sourceCachedResult(cached, fingerprint)
	}
	if candidate.head != request.ExpectedHead {
		return ApplyResult{Edit: cloneEditResult(edit)}, workspaceError("REVISION_CONFLICT", "candidate head changed", ErrRevisionConflict)
	}
	if len(w.revisions) >= w.limits.MaxRevisions {
		return ApplyResult{Edit: cloneEditResult(edit)}, workspaceError("REVISION_LIMIT", "workspace revision capacity is exhausted", ErrLimit)
	}
	journalCheckpoint := candidate.journal.ExportState()
	journalEdit, _, journalErr := candidate.journal.ApplySemantic(paperedit.SemanticJournalRequest{
		ExpectedRevision: request.ExpectedRevision, Group: request.Group, IdempotencyKey: request.IdempotencyKey,
		TargetPreconditions: preconditions, RequireExactTargets: true, Operations: operations,
	})
	if journalErr != nil {
		return ApplyResult{Edit: cloneEditResult(journalEdit)}, wrapJournalError(journalErr)
	}
	if !journalEdit.Applied || journalEdit.Diff == nil {
		return ApplyResult{Candidate: snapshotCandidate(candidate), Revision: snapshotOf(base), Edit: cloneEditResult(journalEdit)}, nil
	}
	if err := reconcilePreparedJournalLocked(candidate, journalCheckpoint, journalEdit, prepared); err != nil {
		return ApplyResult{Edit: cloneEditResult(journalEdit)}, err
	}
	w.nextRevision++
	prepared.handle = RevisionHandle{value: w.newHandle(handleRevision, capabilityRead, w.nextRevision)}
	prepared.expires = w.expiresAt(w.handleTTL)
	prepared.disclosure = w.disclosureDomain
	prepared.partition = w.partition
	w.revisions[w.nextRevision] = prepared
	candidate.head = prepared.handle
	candidate.clearHeadCachesLocked()
	result := ApplyResult{
		Candidate: snapshotCandidate(candidate),
		Revision:  snapshotOf(prepared), Edit: cloneEditResult(edit),
	}
	if request.IdempotencyKey != "" {
		candidate.idempotency[request.IdempotencyKey] = sourceIdempotencyRecord{fingerprint: fingerprint, result: cloneApplyResult(result)}
	}
	return cloneApplyResult(result), nil
}

func (candidate *candidateRecord) clearAcceptanceLocked() {
	candidate.acceptance = nil
	candidate.acceptanceIdempotency = make(map[string]candidateAcceptanceIdempotencyRecord)
}

func (candidate *candidateRecord) clearHeadCachesLocked() {
	candidate.idempotency = make(map[string]sourceIdempotencyRecord)
	candidate.clearAcceptanceLocked()
}

func reconcilePreparedJournalLocked(candidate *candidateRecord, checkpoint paperedit.JournalState, edit paperedit.Result, prepared *revisionRecord) error {
	if edit.Revision == prepared.revision && edit.Source == prepared.source {
		return nil
	}
	restored, err := paperedit.RestoreJournal(checkpoint)
	if err != nil {
		return workspaceError("JOURNAL_CORRUPT", "working-copy journal could not restore its pre-commit checkpoint", ErrPersistenceCorrupt)
	}
	candidate.journal = restored
	return workspaceError("JOURNAL_DIVERGENCE", "working-copy journal diverged from the prepared semantic edit", ErrRevisionConflict)
}

func wrapJournalError(err error) error {
	switch {
	case errors.Is(err, paperedit.ErrJournalConflict), errors.Is(err, paperedit.ErrExternalReload):
		return workspaceError("REVISION_CONFLICT", "working-copy journal head changed", ErrRevisionConflict)
	case errors.Is(err, paperedit.ErrJournalLimit):
		return workspaceError("JOURNAL_LIMIT", "working-copy journal exceeds its configured bound", ErrLimit)
	default:
		return wrapEditError(err)
	}
}

func wrapEditError(err error) error {
	switch {
	case errors.Is(err, paperedit.ErrRevisionConflict):
		return workspaceError("REVISION_CONFLICT", "edit source revision changed", ErrRevisionConflict)
	case errors.Is(err, paperedit.ErrLimit):
		return workspaceError("EDIT_LIMIT", "edit exceeds a transactional limit", ErrLimit)
	case errors.Is(err, paperedit.ErrInvalidSource):
		return workspaceError("INVALID_SOURCE", "candidate source has parse errors", ErrInvalidSource)
	case errors.Is(err, paperedit.ErrCandidateInvalid):
		return workspaceError("INVALID_CANDIDATE", "edit would produce invalid source", ErrInvalidSource)
	default:
		return workspaceError("EDIT_REJECTED", "edit transaction was rejected", err)
	}
}

func (w *Workspace) revision(handle RevisionHandle) (*revisionRecord, error) {
	if w == nil {
		return nil, workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.revisionLocked(handle)
}

func (w *Workspace) revisionLocked(handle RevisionHandle) (*revisionRecord, error) {
	if err := w.validateHandle(handle.value, handleRevision, capabilityRead, false); err != nil {
		return nil, err
	}
	record := w.revisions[handle.value.serial]
	if record == nil || record.handle != handle || !w.ownsPartition(record.partition) {
		return nil, w.unavailableHandle(handle.value, ErrRevisionNotFound)
	}
	if err := w.ensureLive(handle.value, record.expires); err != nil {
		return nil, err
	}
	return record, nil
}

func (w *Workspace) candidateLocked(handle CandidateHandle) (*candidateRecord, error) {
	if err := w.validateHandle(handle.value, handleCandidate, capabilityEdit, false); err != nil {
		return nil, err
	}
	record := w.candidates[handle.value.serial]
	if record == nil || record.handle != handle || !w.ownsPartition(record.partition) {
		return nil, w.unavailableHandle(handle.value, ErrCandidateNotFound)
	}
	if err := w.ensureLive(handle.value, record.expires); err != nil {
		return nil, err
	}
	return record, nil
}

func (w *Workspace) plan(handle PlanHandle) (*planRecord, error) {
	if w == nil {
		return nil, workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.planLocked(handle)
}

func (w *Workspace) planLocked(handle PlanHandle) (*planRecord, error) {
	if err := w.validateHandle(handle.value, handlePlan, capabilityRender, false); err != nil {
		return nil, err
	}
	record := w.plans[handle.value.serial]
	if record == nil || record.handle != handle || !w.ownsPartition(record.partition) {
		return nil, w.unavailableHandle(handle.value, ErrPlanNotFound)
	}
	if !record.expires.After(w.now()) {
		return nil, workspaceError("PLAN_EXPIRED", "handle is unavailable", ErrPlanExpired)
	}
	return record, nil
}

func countASTNodes(root *paperlang.Node, stop int) int {
	count := 0
	var walk func(*paperlang.Node)
	walk = func(node *paperlang.Node) {
		if node == nil || count >= stop {
			return
		}
		count++
		for _, member := range node.Members {
			if member.Property != nil {
				count++
			}
			walk(member.Node)
			if count >= stop {
				return
			}
		}
	}
	walk(root)
	return count
}

func cloneOperations(operations []paperedit.Operation) []paperedit.Operation {
	cloned := make([]paperedit.Operation, len(operations))
	for index, operation := range operations {
		switch value := operation.(type) {
		case paperedit.SetProperty, paperedit.ReplaceText, paperedit.DeleteNode,
			paperedit.RenameID, paperedit.MoveNode:
			cloned[index] = value
		case paperedit.InsertNode:
			value.Node = cloneNodeSpec(value.Node)
			cloned[index] = value
		case paperedit.WrapNode:
			value.Wrapper = cloneNodeSpec(value.Wrapper)
			cloned[index] = value
		case paperedit.ReplaceNode:
			value.Node = cloneNodeSpec(value.Node)
			cloned[index] = value
		default:
			cloned[index] = operation
		}
	}
	return cloned
}

func cloneNodeSpec(spec paperedit.NodeSpec) paperedit.NodeSpec {
	cloned := spec
	if spec.Value != nil {
		value := *spec.Value
		cloned.Value = &value
	}
	cloned.Properties = append([]paperedit.PropertySpec(nil), spec.Properties...)
	if len(spec.Children) != 0 {
		cloned.Children = make([]paperedit.NodeSpec, len(spec.Children))
		for index, child := range spec.Children {
			cloned.Children[index] = cloneNodeSpec(child)
		}
	}
	return cloned
}

func cloneEditResult(result paperedit.Result) paperedit.Result {
	result.Diagnostics = append([]paperedit.Diagnostic(nil), result.Diagnostics...)
	if result.Diff != nil {
		diff := *result.Diff
		diff.Patches = append([]paperedit.SourcePatch(nil), result.Diff.Patches...)
		result.Diff = &diff
	}
	if result.Invalidation != nil {
		invalidation := *result.Invalidation
		invalidation.NodeIDs = append([]string(nil), result.Invalidation.NodeIDs...)
		result.Invalidation = &invalidation
	}
	return result
}
