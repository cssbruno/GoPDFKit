// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const ProtocolVersion1 uint16 = 1

var (
	ErrProtocolAuthentication = errors.New("paperd: protocol authentication failed")
	ErrProtocolVersion        = errors.New("paperd: protocol version negotiation failed")
	ErrProtocolReplay         = errors.New("paperd: protocol request replayed")
	ErrProtocolCapability     = errors.New("paperd: protocol capability denied")
)

// ProtocolCapability is a closed, transport-level permission. These values
// authorize method dispatch only; workspace handles remain independently
// scoped and are never serialized by this envelope.
type ProtocolCapability string

const (
	ProtocolRead   ProtocolCapability = "read"
	ProtocolEdit   ProtocolCapability = "edit"
	ProtocolReview ProtocolCapability = "review"
	ProtocolExport ProtocolCapability = "export"
)

func (capability ProtocolCapability) valid() bool {
	switch capability {
	case ProtocolRead, ProtocolEdit, ProtocolReview, ProtocolExport:
		return true
	default:
		return false
	}
}

// ProtocolEnvelope is the canonical authenticated request boundary. Versions
// MUST be unique and strictly descending; SelectedVersion MUST be the first
// server-supported entry. Authentication covers the complete envelope except
// Authentication itself, which makes removal of a newer offered version a
// detectable authentication failure rather than a silent downgrade.
type ProtocolEnvelope struct {
	Versions         []uint16         `json:"versions"`
	SelectedVersion  uint16           `json:"selected_version"`
	KeyID            string           `json:"key_id"`
	Project          string           `json:"project"`
	PolicyRevision   string           `json:"policy_revision"`
	DisclosureDomain DisclosureDomain `json:"disclosure_domain"`
	Method           string           `json:"method"`
	RequestID        string           `json:"request_id"`
	IssuedAt         time.Time        `json:"issued_at"`
	Payload          json.RawMessage  `json:"payload"`
	Authentication   string           `json:"authentication"`
}

// ProtocolPeer binds one in-memory authentication key to an exact workspace
// partition and an allowlist of dispatch capabilities. Key is copied and is
// never included in responses, diagnostics, or audit records.
type ProtocolPeer struct {
	KeyID            string
	Key              []byte
	Project          string
	PolicyRevision   string
	DisclosureDomain DisclosureDomain
	Capabilities     []ProtocolCapability
}

type ProtocolHandler func(json.RawMessage) (any, error)

type protocolMethod struct {
	capability ProtocolCapability
	handler    ProtocolHandler
}

// ProtocolAuditEntry contains only bounded labels and one-way identities.
// It intentionally excludes payloads, authentication values, keys, and the
// peer's raw capability set.
type ProtocolAuditEntry struct {
	Sequence    uint64    `json:"sequence"`
	At          time.Time `json:"at"`
	PeerHash    string    `json:"peer_hash"`
	RequestHash string    `json:"request_hash"`
	MethodHash  string    `json:"method_hash"`
	Allowed     bool      `json:"allowed"`
	Reason      string    `json:"reason"`
}

type ProtocolResponse struct {
	Version uint16          `json:"version"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

// ProtocolServer is a bounded authenticated dispatcher suitable for a local
// socket, pipe, or HTTP adapter. The adapter supplies bytes; all trust-boundary
// decisions happen here before method code runs.
type ProtocolServer struct {
	mu              sync.Mutex
	versions        []uint16
	peers           map[string]protocolPeerRecord
	methods         map[string]protocolMethod
	replays         map[[sha256.Size]byte]time.Time
	audit           []ProtocolAuditEntry
	nextAudit       uint64
	now             func() time.Time
	maxEnvelope     int
	maxAudit        int
	maxClockSkew    time.Duration
	replayRetention time.Duration
}

type protocolPeerRecord struct {
	key          []byte
	project      string
	policy       string
	disclosure   DisclosureDomain
	capabilities map[ProtocolCapability]struct{}
}

func NewProtocolServer(versions []uint16, peers []ProtocolPeer, maxEnvelope, maxAudit int, now func() time.Time) (*ProtocolServer, error) {
	if maxEnvelope <= 0 || maxEnvelope > MaxRenderBytesHard || maxAudit <= 0 || maxAudit > MaxAuthorizationAuditHard {
		return nil, workspaceError("PROTOCOL_LIMIT", "protocol bounds are invalid", ErrLimit)
	}
	if now == nil {
		now = time.Now
	}
	if !validProtocolVersions(versions) {
		return nil, workspaceError("PROTOCOL_VERSION", "server versions must be unique and strictly descending", ErrProtocolVersion)
	}
	server := &ProtocolServer{versions: append([]uint16(nil), versions...), peers: make(map[string]protocolPeerRecord), methods: make(map[string]protocolMethod), replays: make(map[[sha256.Size]byte]time.Time), now: now, maxEnvelope: maxEnvelope, maxAudit: maxAudit, maxClockSkew: 5 * time.Minute, replayRetention: 10 * time.Minute}
	for _, peer := range peers {
		if !validProtocolLabel(peer.KeyID) || len(peer.Key) < 32 || len(peer.Key) > MaxQueryBytesHard || !validProtocolLabel(peer.Project) || !validProtocolLabel(peer.PolicyRevision) {
			return nil, workspaceError("PROTOCOL_PEER", "protocol peer configuration is invalid", ErrProtocolAuthentication)
		}
		disclosure, _, err := normalizeDisclosureDomain(peer.DisclosureDomain)
		if err != nil {
			return nil, err
		}
		if _, duplicate := server.peers[peer.KeyID]; duplicate {
			return nil, workspaceError("PROTOCOL_PEER", "protocol peer key id is duplicated", ErrProtocolAuthentication)
		}
		capabilities := make(map[ProtocolCapability]struct{}, len(peer.Capabilities))
		for _, capability := range peer.Capabilities {
			if !capability.valid() {
				return nil, workspaceError("PROTOCOL_CAPABILITY", "protocol peer capability is invalid", ErrProtocolCapability)
			}
			capabilities[capability] = struct{}{}
		}
		server.peers[peer.KeyID] = protocolPeerRecord{key: append([]byte(nil), peer.Key...), project: peer.Project, policy: peer.PolicyRevision, disclosure: disclosure, capabilities: capabilities}
	}
	return server, nil
}

func (server *ProtocolServer) Register(method string, capability ProtocolCapability, handler ProtocolHandler) error {
	if server == nil || !validProtocolLabel(method) || !capability.valid() || handler == nil {
		return workspaceError("PROTOCOL_METHOD", "protocol method registration is invalid", ErrInvalidQuery)
	}
	server.mu.Lock()
	defer server.mu.Unlock()
	if _, exists := server.methods[method]; exists {
		return workspaceError("PROTOCOL_METHOD", "protocol method is already registered", ErrInvalidQuery)
	}
	server.methods[method] = protocolMethod{capability: capability, handler: handler}
	return nil
}

// SignProtocolEnvelope canonicalizes and authenticates an envelope. It is
// useful to trusted client adapters and deterministic protocol fixtures.
func SignProtocolEnvelope(envelope ProtocolEnvelope, key []byte, limit int) (ProtocolEnvelope, error) {
	if len(key) < 32 || len(key) > MaxQueryBytesHard {
		return ProtocolEnvelope{}, workspaceError("PROTOCOL_KEY", "protocol authentication key is invalid", ErrProtocolAuthentication)
	}
	envelope.Authentication = ""
	encoded, err := canonicalProtocolEnvelope(envelope, limit)
	if err != nil {
		return ProtocolEnvelope{}, err
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(append([]byte("paperd/protocol-envelope/v1\x00"), encoded...))
	envelope.Authentication = hex.EncodeToString(mac.Sum(nil))
	return envelope, nil
}

func (server *ProtocolServer) Dispatch(encoded []byte) ProtocolResponse {
	if server == nil {
		return protocolError(0, "PROTOCOL_SERVER", "protocol server is unavailable", ErrInvalidHandle)
	}
	if len(encoded) == 0 || len(encoded) > server.maxEnvelope {
		return protocolError(0, "PROTOCOL_LIMIT", "protocol envelope exceeds its byte bound", ErrLimit)
	}
	var envelope ProtocolEnvelope
	if err := decodeStrict(encoded, &envelope); err != nil {
		return protocolError(0, "PROTOCOL_ENVELOPE", "protocol envelope is invalid", ErrInvalidQuery)
	}

	server.mu.Lock()
	now := server.now().UTC()
	server.pruneReplaysLocked(now)
	peer, knownPeer := server.peers[envelope.KeyID]
	requestHash := sha256.Sum256(encoded)
	peerHash := protocolIdentityHash(envelope.KeyID)
	deny := func(code, message, reason string, cause error) ProtocolResponse {
		server.recordAuditLocked(now, peerHash, hex.EncodeToString(requestHash[:]), envelope.Method, false, reason)
		server.mu.Unlock()
		return protocolError(envelope.SelectedVersion, code, message, cause)
	}
	if !knownPeer {
		return deny("PROTOCOL_AUTHENTICATION", "protocol authentication failed", "authentication_denied", ErrProtocolAuthentication)
	}
	if !validProtocolEnvelope(envelope, server.maxEnvelope) {
		return deny("PROTOCOL_ENVELOPE", "protocol envelope fields are invalid", "invalid_envelope", ErrInvalidQuery)
	}
	unsigned := envelope
	unsigned.Authentication = ""
	canonical, err := canonicalProtocolEnvelope(unsigned, server.maxEnvelope)
	if err != nil || !verifyProtocolMAC(envelope.Authentication, canonical, peer.key) {
		return deny("PROTOCOL_AUTHENTICATION", "protocol authentication failed", "authentication_denied", ErrProtocolAuthentication)
	}
	selected, ok := negotiateProtocolVersion(envelope.Versions, server.versions)
	if !ok || selected != envelope.SelectedVersion {
		return deny("PROTOCOL_DOWNGRADE", "selected protocol version is not the highest mutually supported version", "downgrade_denied", ErrProtocolVersion)
	}
	if now.Sub(envelope.IssuedAt) > server.maxClockSkew || envelope.IssuedAt.Sub(now) > server.maxClockSkew {
		return deny("PROTOCOL_TIME", "protocol envelope is outside the accepted clock window", "time_denied", ErrProtocolAuthentication)
	}
	if envelope.Project != peer.project || envelope.PolicyRevision != peer.policy {
		return deny("PROTOCOL_PARTITION", "protocol workspace partition is unavailable", "workspace_denied", ErrWrongWorkspace)
	}
	if envelope.DisclosureDomain != peer.disclosure {
		return deny("PROTOCOL_DISCLOSURE", "protocol disclosure domain is unavailable", "disclosure_denied", ErrDisclosureDenied)
	}
	replayKey := protocolReplayKey(envelope.KeyID, envelope.RequestID)
	if _, replayed := server.replays[replayKey]; replayed {
		return deny("PROTOCOL_REPLAY", "protocol request was already consumed", "replay_denied", ErrProtocolReplay)
	}
	server.replays[replayKey] = now
	registered, exists := server.methods[envelope.Method]
	if !exists {
		return deny("PROTOCOL_METHOD", "protocol method is not registered", "method_denied", ErrInvalidQuery)
	}
	if _, allowed := peer.capabilities[registered.capability]; !allowed {
		return deny("PROTOCOL_CAPABILITY", "protocol method capability is unavailable", "capability_denied", ErrProtocolCapability)
	}
	server.recordAuditLocked(now, peerHash, hex.EncodeToString(requestHash[:]), envelope.Method, true, "dispatched")
	server.mu.Unlock()

	value, handlerErr := invokeProtocolHandler(registered.handler, append(json.RawMessage(nil), envelope.Payload...))
	if handlerErr != nil {
		var typed *Error
		if errors.As(handlerErr, &typed) {
			copyError := *typed
			copyError.cause = nil
			return ProtocolResponse{Version: selected, Error: &copyError}
		}
		return protocolError(selected, "PROTOCOL_HANDLER", "protocol method failed", handlerErr)
	}
	payload, err := json.Marshal(value)
	if err != nil || len(payload) > server.maxEnvelope {
		return protocolError(selected, "PROTOCOL_RESPONSE", "protocol response could not be encoded within bounds", ErrLimit)
	}
	return ProtocolResponse{Version: selected, Payload: payload}
}

func invokeProtocolHandler(handler ProtocolHandler, payload json.RawMessage) (value any, err error) {
	defer func() {
		if recover() != nil {
			value = nil
			err = workspaceError("PROTOCOL_HANDLER_PANIC", "protocol method failed", ErrInvalidQuery)
		}
	}()
	return handler(payload)
}

func (server *ProtocolServer) Audit(limit int) ([]ProtocolAuditEntry, error) {
	if server == nil || limit <= 0 || limit > server.maxAudit {
		return nil, workspaceError("PROTOCOL_AUDIT_LIMIT", "protocol audit limit is invalid", ErrLimit)
	}
	server.mu.Lock()
	defer server.mu.Unlock()
	start := len(server.audit) - limit
	if start < 0 {
		start = 0
	}
	return append([]ProtocolAuditEntry(nil), server.audit[start:]...), nil
}

func (server *ProtocolServer) recordAuditLocked(at time.Time, peerHash, requestHash, method string, allowed bool, reason string) {
	server.nextAudit++
	for len(server.audit) >= server.maxAudit {
		copy(server.audit, server.audit[1:])
		server.audit = server.audit[:len(server.audit)-1]
	}
	server.audit = append(server.audit, ProtocolAuditEntry{Sequence: server.nextAudit, At: at, PeerHash: peerHash, RequestHash: requestHash, MethodHash: protocolIdentityHash("method\x00" + method), Allowed: allowed, Reason: reason})
}

func (server *ProtocolServer) pruneReplaysLocked(now time.Time) {
	for key, at := range server.replays {
		if now.Sub(at) > server.replayRetention {
			delete(server.replays, key)
		}
	}
}

func canonicalProtocolEnvelope(envelope ProtocolEnvelope, limit int) ([]byte, error) {
	encoded, err := json.Marshal(envelope)
	if err != nil || len(encoded) > limit {
		return nil, workspaceError("PROTOCOL_LIMIT", "protocol envelope exceeds its byte bound", ErrLimit)
	}
	return encoded, nil
}

func validProtocolEnvelope(envelope ProtocolEnvelope, limit int) bool {
	return validProtocolVersions(envelope.Versions) && validProtocolLabel(envelope.KeyID) && validProtocolLabel(envelope.Project) && validProtocolLabel(envelope.PolicyRevision) &&
		validProtocolLabel(envelope.Method) && validProtocolLabel(envelope.RequestID) && envelope.IssuedAt.Location() == time.UTC && !envelope.IssuedAt.IsZero() &&
		json.Valid(envelope.Payload) && len(envelope.Payload) <= limit && validSHA256(envelope.Authentication)
}

func validProtocolVersions(versions []uint16) bool {
	if len(versions) == 0 || len(versions) > 16 {
		return false
	}
	for index, version := range versions {
		if version == 0 || (index > 0 && versions[index-1] <= version) {
			return false
		}
	}
	return true
}

func negotiateProtocolVersion(client, server []uint16) (uint16, bool) {
	available := make(map[uint16]struct{}, len(server))
	for _, version := range server {
		available[version] = struct{}{}
	}
	for _, version := range client {
		if _, ok := available[version]; ok {
			return version, true
		}
	}
	return 0, false
}

func verifyProtocolMAC(authentication string, canonical, key []byte) bool {
	want, err := hex.DecodeString(authentication)
	if err != nil || len(want) != sha256.Size {
		return false
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(append([]byte("paperd/protocol-envelope/v1\x00"), canonical...))
	return hmac.Equal(want, mac.Sum(nil))
}

func protocolReplayKey(keyID, requestID string) [sha256.Size]byte {
	return sha256.Sum256([]byte("paperd/protocol-replay/v1\x00" + keyID + "\x00" + requestID))
}

func protocolIdentityHash(value string) string {
	sum := sha256.Sum256([]byte("paperd/protocol-identity/v1\x00" + value))
	return hex.EncodeToString(sum[:])
}

func validProtocolLabel(value string) bool {
	return value != "" && len(value) <= MaxQueryBytesHard && utf8.ValidString(value) && strings.TrimSpace(value) == value && !strings.ContainsAny(value, "\r\n\x00")
}

func protocolError(version uint16, code, message string, cause error) ProtocolResponse {
	return ProtocolResponse{Version: version, Error: &Error{Code: code, Message: message, cause: cause}}
}

// CanonicalProtocolResponse emits a deterministic, bounded transport response.
func CanonicalProtocolResponse(response ProtocolResponse, limit int) ([]byte, error) {
	if response.Error != nil {
		response.Error.cause = nil
	}
	encoded, err := json.Marshal(response)
	if err != nil || len(encoded) > limit {
		return nil, workspaceError("PROTOCOL_RESPONSE", "protocol response exceeds its byte bound", ErrLimit)
	}
	return bytes.Clone(encoded), nil
}

// SupportedProtocolVersions returns a detached sorted copy for adapters.
func (server *ProtocolServer) SupportedProtocolVersions() []uint16 {
	if server == nil {
		return nil
	}
	server.mu.Lock()
	defer server.mu.Unlock()
	result := append([]uint16(nil), server.versions...)
	sort.Slice(result, func(i, j int) bool { return result[i] > result[j] })
	return result
}
