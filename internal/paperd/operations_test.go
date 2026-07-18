// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestOperationalSnapshotIsAggregatePrunesExpiredAndReportsSaturation(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxRevisions = 1
	workspace, err := NewWorkspace(limits)
	if err != nil {
		t.Fatal(err)
	}
	created, err := workspace.PaperCreate(PaperCreateRequest{File: "private.paper", Source: workspaceFixture})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := workspace.OperationalSnapshot()
	if snapshot.Revisions != (OperationalCapacity{Current: 1, Limit: 1}) || snapshot.Candidates.Current != 1 || len(snapshot.Saturated) != 1 || snapshot.Saturated[0] != "revisions" {
		t.Fatalf("operational snapshot = %+v", snapshot)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"private.paper", workspaceFixture, string(created.Revision.Revision)} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("operational snapshot leaked %q: %s", secret, encoded)
		}
	}
	if nilSnapshot := (*Workspace)(nil).OperationalSnapshot(); nilSnapshot.Revisions != (OperationalCapacity{}) || len(nilSnapshot.Saturated) != 0 {
		t.Fatalf("nil snapshot = %+v", nilSnapshot)
	}
}
