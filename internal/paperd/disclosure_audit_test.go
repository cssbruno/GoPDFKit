// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
)

func TestDisclosureDenialsAreHashOnlyAuditedPersistedAndEmitted(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	var emitted []DisclosureAuditEntry
	options := persistenceOptions(root)
	options.DisclosureAuditSink = func(entry DisclosureAuditEntry) { emitted = append(emitted, entry) }
	workspace, err := OpenWorkspace(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	created, err := workspace.PaperCreate(PaperCreateRequest{File: "audit.paper", Source: workspaceFixture})
	if err != nil {
		t.Fatal(err)
	}
	_, err = workspace.PaperOpen(PaperOpenRequest{Revision: created.Revision.Handle, ExpectedDigest: created.Revision.Revision, Mode: CapabilityRead, DisclosureDomain: DisclosureDomain("customer-secret-domain")})
	if !errors.Is(err, ErrDisclosureDenied) {
		t.Fatalf("disclosure denial = %v", err)
	}
	audit, err := workspace.DisclosureAudit(8)
	if err != nil || len(audit) != 1 || len(emitted) != 1 || audit[0] != emitted[0] || audit[0].Reason != "domain_mismatch" || !validSHA256(audit[0].RequestedHash) || !validSHA256(audit[0].ExpectedHash) {
		t.Fatalf("audit/emitted = %#v / %#v / %v", audit, emitted, err)
	}
	encoded, err := json.Marshal(audit)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("customer-secret-domain")) || bytes.Contains(encoded, []byte(DisclosureRestricted)) {
		t.Fatalf("disclosure audit leaked raw domains: %s", encoded)
	}
	if err := workspace.SaveSnapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	recovered, err := OpenWorkspace(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := recovered.DisclosureAudit(8)
	if err != nil || len(restored) != 1 || restored[0] != audit[0] {
		t.Fatalf("restored audit = %#v, %v", restored, err)
	}
}

func TestDisclosureAuditSinkPanicCannotChangeDenial(t *testing.T) {
	workspace, err := NewWorkspaceWithOptions(WorkspaceOptions{DisclosureDomain: DisclosureRestricted, DisclosureAuditSink: func(DisclosureAuditEntry) { panic("sink secret") }})
	if err != nil {
		t.Fatal(err)
	}
	created, err := workspace.PaperCreate(PaperCreateRequest{File: "audit.paper", Source: workspaceFixture})
	if err != nil {
		t.Fatal(err)
	}
	_, err = workspace.PaperOpen(PaperOpenRequest{Revision: created.Revision.Handle, ExpectedDigest: created.Revision.Revision, Mode: CapabilityRead, DisclosureDomain: DisclosurePublic})
	if !errors.Is(err, ErrDisclosureDenied) {
		t.Fatalf("denial after sink panic = %v", err)
	}
	audit, _ := workspace.DisclosureAudit(1)
	if len(audit) != 1 {
		t.Fatalf("audit after sink panic = %#v", audit)
	}
}
