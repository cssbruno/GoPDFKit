// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

type ProvenanceID uint32

func (id ProvenanceID) Valid() bool { return id != 0 }

// ProvenanceEntry is one interned identity/source tuple. Canonical order is
// deterministic first occurrence across fragments followed by line sources.
type ProvenanceEntry struct {
	Node     NodeID     `json:"node"`
	Key      NodeKey    `json:"key"`
	Instance InstanceID `json:"instance"`
	Source   SourceSpan `json:"source"`
}

func buildCompactProvenance(fragments []Fragment, lines []PlannedLine) ([]ProvenanceEntry, []ProvenanceID, []ProvenanceID) {
	table := make([]ProvenanceEntry, 0, len(fragments))
	ids := make(map[ProvenanceEntry]ProvenanceID, len(fragments))
	intern := func(entry ProvenanceEntry) ProvenanceID {
		if id := ids[entry]; id.Valid() {
			return id
		}
		id := ProvenanceID(len(table) + 1)
		table = append(table, entry)
		ids[entry] = id
		return id
	}
	fragmentRefs := make([]ProvenanceID, len(fragments))
	fragmentsByID := make(map[FragmentID]Fragment, len(fragments))
	for index, fragment := range fragments {
		fragmentRefs[index] = intern(ProvenanceEntry{Node: fragment.Node, Key: fragment.Key, Instance: fragment.Instance, Source: fragment.Source})
		fragmentsByID[fragment.ID] = fragment
	}
	lineRefs := make([]ProvenanceID, len(lines))
	for index, line := range lines {
		fragment := fragmentsByID[line.Fragment]
		lineRefs[index] = intern(ProvenanceEntry{Node: fragment.Node, Key: fragment.Key, Instance: fragment.Instance, Source: line.Source})
	}
	return table, fragmentRefs, lineRefs
}

// ResolveProvenance returns a detached compact-table entry.
func (p LayoutPlan) ResolveProvenance(id ProvenanceID) (ProvenanceEntry, bool) {
	if !id.Valid() || uint64(id) > uint64(len(p.provenance)) {
		return ProvenanceEntry{}, false
	}
	return p.provenance[id-1], true
}
