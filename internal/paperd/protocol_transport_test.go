// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package paperd

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

var protocolFixtureKey = []byte("0123456789abcdef0123456789abcdef")

func protocolFixtureServer(t *testing.T, capabilities ...ProtocolCapability) *ProtocolServer {
	t.Helper()
	now := func() time.Time { return time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC) }
	server, err := NewProtocolServer([]uint16{2, ProtocolVersion1}, []ProtocolPeer{{KeyID: "agent-fixture", Key: protocolFixtureKey, Project: "project-a", PolicyRevision: "policy-v3", DisclosureDomain: DisclosureRestricted, Capabilities: capabilities}}, 64<<10, 32, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Register("paper.inspect", ProtocolRead, func(payload json.RawMessage) (any, error) {
		return struct {
			Digest string `json:"digest"`
		}{Digest: protocolIdentityHash(string(payload))}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := server.Register("paper.apply", ProtocolEdit, func(json.RawMessage) (any, error) { return struct{}{}, nil }); err != nil {
		t.Fatal(err)
	}
	return server
}

func signedProtocolFixture(t *testing.T, requestID string) ([]byte, ProtocolEnvelope) {
	t.Helper()
	envelope, err := SignProtocolEnvelope(ProtocolEnvelope{
		Versions: []uint16{2, 1}, SelectedVersion: 2, KeyID: "agent-fixture", Project: "project-a", PolicyRevision: "policy-v3",
		DisclosureDomain: DisclosureRestricted, Method: "paper.inspect", RequestID: requestID,
		IssuedAt: time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC), Payload: json.RawMessage(`{"query":"outline"}`),
	}, protocolFixtureKey, 64<<10)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	return encoded, envelope
}

func TestProtocolAuthenticatedNegotiationDispatchReplayAndAudit(t *testing.T) {
	server := protocolFixtureServer(t, ProtocolRead)
	encoded, envelope := signedProtocolFixture(t, "request-00000001")
	response := server.Dispatch(encoded)
	if response.Error != nil || response.Version != 2 || !json.Valid(response.Payload) {
		t.Fatalf("dispatch = %#v", response)
	}
	if replay := server.Dispatch(encoded); replay.Error == nil || !errors.Is(replay.Error, ErrProtocolReplay) || replay.Error.Code != "PROTOCOL_REPLAY" {
		t.Fatalf("replay = %#v", replay)
	}

	downgraded := envelope
	downgraded.SelectedVersion = 1
	downgraded.RequestID = "request-00000002"
	downgraded, err := SignProtocolEnvelope(downgraded, protocolFixtureKey, 64<<10)
	if err != nil {
		t.Fatal(err)
	}
	downgradeBytes, _ := json.Marshal(downgraded)
	if response := server.Dispatch(downgradeBytes); response.Error == nil || !errors.Is(response.Error, ErrProtocolVersion) || response.Error.Code != "PROTOCOL_DOWNGRADE" {
		t.Fatalf("authenticated downgrade = %#v", response)
	}

	tampered := envelope
	tampered.Versions = []uint16{1}
	tampered.SelectedVersion = 1
	tampered.RequestID = "request-00000003"
	tamperedBytes, _ := json.Marshal(tampered)
	if response := server.Dispatch(tamperedBytes); response.Error == nil || !errors.Is(response.Error, ErrProtocolAuthentication) {
		t.Fatalf("unauthenticated downgrade = %#v", response)
	}

	denied := envelope
	denied.Method, denied.RequestID = "paper.apply", "request-00000004"
	denied, _ = SignProtocolEnvelope(denied, protocolFixtureKey, 64<<10)
	deniedBytes, _ := json.Marshal(denied)
	if response := server.Dispatch(deniedBytes); response.Error == nil || !errors.Is(response.Error, ErrProtocolCapability) {
		t.Fatalf("capability filtering = %#v", response)
	}

	audit, err := server.Audit(32)
	if err != nil || len(audit) != 5 || !audit[0].Allowed || audit[1].Reason != "replay_denied" || audit[2].Reason != "downgrade_denied" || audit[4].Reason != "capability_denied" {
		t.Fatalf("audit = %#v, %v", audit, err)
	}
	auditJSON, _ := json.Marshal(audit)
	for _, forbidden := range [][]byte{protocolFixtureKey, []byte(envelope.Authentication), envelope.Payload, []byte("agent-fixture"), []byte("export")} {
		if bytes.Contains(auditJSON, forbidden) {
			t.Fatalf("audit leaked transport secret/capability/payload %q: %s", forbidden, auditJSON)
		}
	}
}

func TestProtocolRejectsCrossWorkspaceAndDisclosureAndAuditsBoth(t *testing.T) {
	server := protocolFixtureServer(t, ProtocolRead)
	_, base := signedProtocolFixture(t, "partition-00000001")
	cases := []struct {
		name, code, reason string
		mutate             func(*ProtocolEnvelope)
		cause              error
	}{
		{name: "workspace", code: "PROTOCOL_PARTITION", reason: "workspace_denied", cause: ErrWrongWorkspace, mutate: func(value *ProtocolEnvelope) { value.Project = "project-b" }},
		{name: "policy", code: "PROTOCOL_PARTITION", reason: "workspace_denied", cause: ErrWrongWorkspace, mutate: func(value *ProtocolEnvelope) { value.PolicyRevision = "policy-v4" }},
		{name: "disclosure", code: "PROTOCOL_DISCLOSURE", reason: "disclosure_denied", cause: ErrDisclosureDenied, mutate: func(value *ProtocolEnvelope) { value.DisclosureDomain = DisclosurePublic }},
	}
	for index, item := range cases {
		value := base
		value.RequestID = "partition-0000000" + string(rune('2'+index))
		item.mutate(&value)
		value, _ = SignProtocolEnvelope(value, protocolFixtureKey, 64<<10)
		encoded, _ := json.Marshal(value)
		response := server.Dispatch(encoded)
		if response.Error == nil || response.Error.Code != item.code || !errors.Is(response.Error, item.cause) {
			t.Fatalf("%s response = %#v", item.name, response)
		}
	}
	audit, _ := server.Audit(32)
	if len(audit) != len(cases) {
		t.Fatalf("audit entries = %#v", audit)
	}
	for index, item := range cases {
		if audit[index].Allowed || audit[index].Reason != item.reason || !validSHA256(audit[index].PeerHash) || !validSHA256(audit[index].RequestHash) || !validSHA256(audit[index].MethodHash) {
			t.Fatalf("audit[%d] = %#v", index, audit[index])
		}
	}
}

func TestProtocolAdversarialCorpusFailsClosedBeforeDispatch(t *testing.T) {
	server := protocolFixtureServer(t, ProtocolRead)
	_, base := signedProtocolFixture(t, "corpus-base")
	cases := []struct {
		name string
		code string
		edit func(*ProtocolEnvelope)
		sign bool
	}{
		{name: "bad authentication", code: "PROTOCOL_AUTHENTICATION", edit: func(value *ProtocolEnvelope) { value.Authentication = strings.Repeat("0", 64) }},
		{name: "unsupported versions", code: "PROTOCOL_DOWNGRADE", sign: true, edit: func(value *ProtocolEnvelope) { value.Versions, value.SelectedVersion = []uint16{99}, 99 }},
		{name: "duplicate versions", code: "PROTOCOL_ENVELOPE", sign: true, edit: func(value *ProtocolEnvelope) { value.Versions = []uint16{2, 2, 1} }},
		{name: "stale request", code: "PROTOCOL_TIME", sign: true, edit: func(value *ProtocolEnvelope) { value.IssuedAt = value.IssuedAt.Add(-time.Hour) }},
		{name: "cross workspace", code: "PROTOCOL_PARTITION", sign: true, edit: func(value *ProtocolEnvelope) { value.Project = "other-project" }},
		{name: "cross policy", code: "PROTOCOL_PARTITION", sign: true, edit: func(value *ProtocolEnvelope) { value.PolicyRevision = "other-policy" }},
		{name: "cross disclosure", code: "PROTOCOL_DISCLOSURE", sign: true, edit: func(value *ProtocolEnvelope) { value.DisclosureDomain = "private-customer-scope" }},
		{name: "unknown method", code: "PROTOCOL_METHOD", sign: true, edit: func(value *ProtocolEnvelope) { value.Method = "paper.unknown" }},
	}
	for index, item := range cases {
		value := base
		value.RequestID = fmt.Sprintf("corpus-%02d", index)
		item.edit(&value)
		if item.sign {
			value, _ = SignProtocolEnvelope(value, protocolFixtureKey, 64<<10)
		}
		encoded, _ := json.Marshal(value)
		response := server.Dispatch(encoded)
		if response.Error == nil || response.Error.Code != item.code {
			t.Fatalf("%s = %#v", item.name, response)
		}
		canonical, err := CanonicalProtocolResponse(response, 4<<10)
		if err != nil || bytes.Contains(canonical, protocolFixtureKey) || bytes.Contains(canonical, value.Payload) || bytes.Contains(canonical, []byte(value.Authentication)) || bytes.Contains(canonical, []byte("private-customer-scope")) {
			t.Fatalf("%s response disclosure = %v, %s", item.name, err, canonical)
		}
	}
}

func TestProtocolHandlerPanicIsContainedAndRedacted(t *testing.T) {
	server := protocolFixtureServer(t, ProtocolRead)
	if err := server.Register("paper.panic", ProtocolRead, func(json.RawMessage) (any, error) { panic("private handler data") }); err != nil {
		t.Fatal(err)
	}
	_, envelope := signedProtocolFixture(t, "panic-request")
	envelope.Method = "paper.panic"
	envelope, _ = SignProtocolEnvelope(envelope, protocolFixtureKey, 64<<10)
	encoded, _ := json.Marshal(envelope)
	response := server.Dispatch(encoded)
	canonical, err := CanonicalProtocolResponse(response, 4<<10)
	if response.Error == nil || response.Error.Code != "PROTOCOL_HANDLER_PANIC" || err != nil || bytes.Contains(canonical, []byte("private handler data")) {
		t.Fatalf("panic response = %#v, %v, %s", response, err, canonical)
	}
}

func TestProtocolVersionFixtureIsStableAcrossProcesses(t *testing.T) {
	if os.Getenv("GOPDFKIT_PROTOCOL_TRANSPORT_FIXTURE") == "1" {
		encoded, _ := signedProtocolFixture(t, "fixture-transport-v1")
		_, _ = os.Stdout.Write(encoded)
		os.Exit(0)
	}
	run := func() []byte {
		command := exec.Command(os.Args[0], "-test.run=^TestProtocolVersionFixtureIsStableAcrossProcesses$")
		command.Env = append(os.Environ(), "GOPDFKIT_PROTOCOL_TRANSPORT_FIXTURE=1")
		output, err := command.Output()
		if err != nil {
			t.Fatalf("fixture process: %v", err)
		}
		return output
	}
	first, second := run(), run()
	if !bytes.Equal(first, second) || !json.Valid(first) {
		t.Fatalf("cross-process fixture differs:\n%s\n%s", first, second)
	}
	sum := sha256.Sum256(first)
	const fixtureSHA256 = "d7b3c9f58abd18ba5b7864c29a909710973e8a771a4dc4ec1667de0071fff637"
	if got := hex.EncodeToString(sum[:]); got != fixtureSHA256 {
		t.Fatalf("protocol v1 fixture hash = %s", got)
	}
}
