// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cssbruno/paperrune/layout"
)

func TestLayoutDocumentPlanSnapshotsAndReplaysAttachmentEnvelope(t *testing.T) {
	content := []byte("immutable attachment payload")
	doc := &layout.LayoutDocument{
		Title: "Envelope",
		Body:  []layout.Block{layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "Body"}}}},
		Attachments: []layout.AttachmentBlock{{
			Name: "evidence.txt", MIMEType: "text/plain", Description: "Evidence", Data: content,
		}},
	}
	planner := MustNew(WithUnit(UnitPoint), WithNoCompression(), WithDeterministicOutput())
	plan, err := planner.PlanLayoutDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	content[0] = 'X'
	doc.Attachments[0].Name = "mutated.txt"
	doc.Attachments[0].Data = []byte("mutated")

	target := MustNew(WithUnit(UnitPoint), WithNoCompression(), WithDeterministicOutput())
	pages, err := target.WriteLayoutDocumentPlan(plan)
	if err != nil || pages != plan.PageCount() {
		t.Fatalf("WriteLayoutDocumentPlan(envelope) = %d, %v", pages, err)
	}
	if len(target.attachments) != 1 || target.attachments[0].Filename != "evidence.txt" ||
		target.attachments[0].MIMEType != "text/plain" || !bytes.Equal(target.attachments[0].Content, []byte("immutable attachment payload")) {
		t.Fatalf("replayed attachment = %#v", target.attachments)
	}
	var pdf bytes.Buffer
	if err := target.OutputWithOptions(&pdf, OutputOptions{Deterministic: true}); err != nil {
		t.Fatal(err)
	}
	for _, want := range [][]byte{[]byte("immutable attachment payload"), []byte("/EmbeddedFiles")} {
		if !bytes.Contains(pdf.Bytes(), want) {
			t.Fatalf("attachment PDF lacks %q", want)
		}
	}
}

func TestLayoutDocumentPlanPreservesConfiguredInlineAttachmentsAndRejectsLiveSources(t *testing.T) {
	configured := []byte("configured before typed planning")
	planner := MustNew(WithUnit(UnitPoint))
	planner.SetAttachments([]Attachment{{
		Content: configured, Filename: "configured.txt", MIMEType: "text/plain",
	}})
	model := &layout.LayoutDocument{Body: []layout.Block{
		layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "Body"}}},
	}}
	plan, err := planner.PlanLayoutDocument(model)
	if err != nil {
		t.Fatal(err)
	}
	configured[0] = 'X'
	planner.attachments[0].Content[1] = 'X'
	target := MustNew(WithUnit(UnitPoint))
	if _, err := target.WriteLayoutDocumentPlan(plan); err != nil {
		t.Fatal(err)
	}
	if len(target.attachments) != 1 || target.attachments[0].Filename != "configured.txt" ||
		!bytes.Equal(target.attachments[0].Content, []byte("configured before typed planning")) {
		t.Fatalf("configured attachment envelope = %#v", target.attachments)
	}

	live := MustNew(WithUnit(UnitPoint))
	live.SetAttachments([]Attachment{{FilePath: "must-not-be-opened.txt", Filename: "live.txt"}})
	if plan, err := live.PlanLayoutDocument(model); !errors.Is(err, ErrLayoutDocumentPlanUnsupported) ||
		!strings.Contains(err.Error(), "attachments[0] contains live external state") || plan.Hash() != "" || live.PageCount() != 0 {
		t.Fatalf("live attachment planning = hash %q pages %d error %v", plan.Hash(), live.PageCount(), err)
	}
}

func TestLayoutDocumentPlanPreservesImmutableMetadataComplianceOutputAndSigningEnvelope(t *testing.T) {
	created := time.Date(2024, 2, 3, 4, 5, 6, 0, time.FixedZone("fixture", -3*60*60))
	updated := created.Add(2 * time.Hour)
	xmp := []byte(`<?xpacket begin="fixture"?><fixture>detached</fixture><?xpacket end="w"?>`)
	icc := []byte("detached fixture ICC profile")
	planner := MustNew(
		WithUnit(UnitPoint),
		WithNoCompression(),
		WithOutputPolicy(OutputPolicy{DisableSync: true, Deterministic: true, StreamFinal: true}),
	)
	planner.SetProducer("Envelope producer", false)
	planner.SetCreator("Envelope creator", false)
	planner.SetKeywords("immutable envelope", false)
	planner.SetXmpMetadata(xmp)
	planner.SetComplianceMetadata(ComplianceMetadata{
		PDFUA2: true, Arlington: true, Lang: "pt-BR",
	})
	if err := planner.SetOutputIntent(icc, "Fixture RGB"); err != nil {
		t.Fatal(err)
	}
	doc := &layout.LayoutDocument{
		Title:    "Relatório",
		Language: "en-GB",
		Metadata: layout.DocumentMetadata{
			Subject: "Preserved subject", Author: "Ada Example", DocumentID: "DOC-42",
			CreatedAt: created, UpdatedAt: updated,
		},
		Body: []layout.Block{layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "Body"}}}},
		Signature: &layout.SignatureBlock{
			PlaceholderReference: " ApprovalIdentity ",
			Rows:                 []layout.SignatureRowBlock{{Columns: []layout.SignatureColumn{{Name: "Ada Example"}}}},
		},
		Attachments: []layout.AttachmentBlock{{
			Name: "evidence.txt", MIMEType: "text/plain", Description: "Evidence", Data: []byte("original evidence"),
		}},
	}
	plan, err := planner.PlanLayoutDocument(doc)
	if err != nil {
		t.Fatal(err)
	}

	// Mutating every reference-bearing source after planning must not affect
	// replay or identity.
	xmp[0] = 'X'
	icc[0] = 'X'
	doc.Attachments[0].Data[0] = 'X'
	doc.Title = "mutated"
	doc.Metadata.Subject = "mutated"
	doc.Metadata.Author = "mutated"
	doc.Signature.PlaceholderReference = "MutatedIdentity"

	target := MustNew(WithUnit(UnitPoint), WithNoCompression(), WithOutputPolicy(OutputPolicy{}))
	pages, err := target.WriteLayoutDocumentPlan(plan)
	if err != nil || pages != plan.PageCount() {
		t.Fatalf("WriteLayoutDocumentPlan(envelope) = %d, %v", pages, err)
	}
	if target.producer != "Envelope producer" || target.creator != "Envelope creator" ||
		target.keywords != "immutable envelope" || target.creationDate != created || target.modDate != updated {
		t.Fatalf("descriptive envelope not preserved: producer=%q creator=%q keywords=%q dates=%v/%v",
			target.producer, target.creator, target.keywords, target.creationDate, target.modDate)
	}
	if !bytes.Equal(target.xmp, []byte(`<?xpacket begin="fixture"?><fixture>detached</fixture><?xpacket end="w"?>`)) ||
		!bytes.Equal(target.outputIntent.iccProfile, []byte("detached fixture ICC profile")) {
		t.Fatalf("binary envelope was not detached: xmp=%q icc=%q", target.xmp, target.outputIntent.iccProfile)
	}
	if target.compliance.PDFA != PDFAModeNone || !target.compliance.PDFUA2 || !target.compliance.Arlington ||
		target.compliance.Lang != "en-GB" || target.compliance.Title != "Relatório" || target.compliance.Identifier != "DOC-42" ||
		!target.tagged.enabled || target.pdfVersion != "2.0" {
		t.Fatalf("compliance envelope = %#v tagged=%v version=%q", target.compliance, target.tagged.enabled, target.pdfVersion)
	}
	if target.outputPolicy != (OutputPolicy{DisableSync: true, Deterministic: true, StreamFinal: true}) {
		t.Fatalf("output policy = %#v", target.outputPolicy)
	}
	if target.signatureFieldName != "ApprovalIdentity" {
		t.Fatalf("signing identity = %q", target.signatureFieldName)
	}
	if len(target.attachments) != 1 || target.attachments[0].Filename != "evidence.txt" ||
		!bytes.Equal(target.attachments[0].Content, []byte("original evidence")) {
		t.Fatalf("attachment envelope = %#v", target.attachments)
	}
}

func TestLayoutDocumentPlanPreservesPDFAModeWithoutClaimingValidation(t *testing.T) {
	planner := MustNew(WithUnit(UnitPoint))
	planner.SetComplianceMetadata(ComplianceMetadata{PDFA: PDFAMode4F, Lang: "en-US", Identifier: "archive-42"})
	if err := planner.SetOutputIntent([]byte("fixture ICC"), "Fixture RGB"); err != nil {
		t.Fatal(err)
	}
	plan, err := planner.PlanLayoutDocument(&layout.LayoutDocument{Body: []layout.Block{
		layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "Archive body"}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	target := MustNew(WithUnit(UnitPoint))
	if _, err := target.WriteLayoutDocumentPlan(plan); err != nil {
		t.Fatal(err)
	}
	if target.compliance.PDFA != PDFAMode4F || target.compliance.Identifier != "archive-42" ||
		!bytes.Equal(target.outputIntent.iccProfile, []byte("fixture ICC")) {
		t.Fatalf("PDF/A envelope = %#v intent=%q", target.compliance, target.outputIntent.iccProfile)
	}
}

func TestLayoutDocumentPlanEnvelopeParticipatesInIdentity(t *testing.T) {
	model := func(title string) *layout.LayoutDocument {
		return &layout.LayoutDocument{Title: title, Body: []layout.Block{
			layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "same spatial body"}}},
		}}
	}
	first, err := MustNew(WithUnit(UnitPoint), WithOutputPolicy(OutputPolicy{})).PlanLayoutDocument(model("First"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := MustNew(WithUnit(UnitPoint), WithOutputPolicy(OutputPolicy{DisableSync: true})).PlanLayoutDocument(model("First"))
	if err != nil {
		t.Fatal(err)
	}
	third, err := MustNew(WithUnit(UnitPoint), WithOutputPolicy(OutputPolicy{})).PlanLayoutDocument(model("Second"))
	if err != nil {
		t.Fatal(err)
	}
	if first.Hash() == second.Hash() || first.Hash() == third.Hash() || second.Hash() == third.Hash() {
		t.Fatalf("envelope identities collided: %q %q %q", first.Hash(), second.Hash(), third.Hash())
	}
	repeated, err := MustNew(WithUnit(UnitPoint), WithOutputPolicy(OutputPolicy{})).PlanLayoutDocument(model("First"))
	if err != nil || repeated.Hash() != first.Hash() {
		t.Fatalf("deterministic envelope identity = %q, %v; want %q", repeated.Hash(), err, first.Hash())
	}
}

func TestLayoutDocumentPlanEnvelopeIsReusableAcrossIndependentWriters(t *testing.T) {
	planner := MustNew(WithUnit(UnitPoint), WithOutputPolicy(OutputPolicy{DisableSync: true}))
	planner.SetCreator("Concurrent envelope", false)
	plan, err := planner.PlanLayoutDocument(&layout.LayoutDocument{
		Title: "Reusable",
		Body:  []layout.Block{layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "Body"}}}},
		Attachments: []layout.AttachmentBlock{{
			Name: "evidence.txt", MIMEType: "text/plain", Data: []byte("immutable"),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	const writers = 12
	errorsByWriter := make(chan error, writers)
	var group sync.WaitGroup
	for index := 0; index < writers; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			target := MustNew(WithUnit(UnitPoint))
			pages, writeErr := target.WriteLayoutDocumentPlan(plan)
			if writeErr != nil {
				errorsByWriter <- writeErr
				return
			}
			if pages != plan.PageCount() || target.creator != "Concurrent envelope" ||
				target.outputPolicy != (OutputPolicy{DisableSync: true}) || len(target.attachments) != 1 ||
				!bytes.Equal(target.attachments[0].Content, []byte("immutable")) {
				errorsByWriter <- errors.New("independent writer received an incomplete envelope")
			}
		}()
	}
	group.Wait()
	close(errorsByWriter)
	for err := range errorsByWriter {
		t.Error(err)
	}
}

func TestLayoutDocumentPlanRejectsUnsafeLiveEnvelopeStateAtomically(t *testing.T) {
	model := &layout.LayoutDocument{
		Body: []layout.Block{layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "Body"}}}},
		Signature: &layout.SignatureBlock{
			PlaceholderReference: "PlannedIdentity",
			Rows:                 []layout.SignatureRowBlock{{Columns: []layout.SignatureColumn{{Name: "Signer"}}}},
		},
	}
	allowProtection := SecurityPolicy{AllowLegacyRC4Protection: true}
	encrypted := MustNew(WithUnit(UnitPoint), WithSecurityPolicy(allowProtection))
	if err := encrypted.SetLegacyProtection(CnProtectCopy, "user", "owner"); err != nil {
		t.Fatal(err)
	}
	if plan, err := encrypted.PlanLayoutDocument(model); !errors.Is(err, ErrLayoutDocumentPlanUnsupported) ||
		!strings.Contains(err.Error(), "live encryption state") || plan.Hash() != "" || encrypted.PageCount() != 0 {
		t.Fatalf("encrypted planning = hash %q pages %d error %v", plan.Hash(), encrypted.PageCount(), err)
	}

	conflicting := MustNew(WithUnit(UnitPoint))
	conflicting.signatureFieldName = "OtherIdentity"
	if plan, err := conflicting.PlanLayoutDocument(model); !errors.Is(err, ErrLayoutDocumentPlanUnsupported) ||
		!strings.Contains(err.Error(), "conflicting signing field identities") || plan.Hash() != "" || conflicting.PageCount() != 0 {
		t.Fatalf("conflicting signing planning = hash %q pages %d error %v", plan.Hash(), conflicting.PageCount(), err)
	}

	planner := MustNew(WithUnit(UnitPoint))
	plan, err := planner.PlanLayoutDocument(model)
	if err != nil {
		t.Fatal(err)
	}
	target := MustNew(WithUnit(UnitPoint))
	target.signatureFieldName = "OtherIdentity"
	if pages, err := target.WriteLayoutDocumentPlan(plan); !errors.Is(err, ErrLayoutDocumentPlanUnsupported) ||
		!strings.Contains(err.Error(), "conflicting signing field identity") || pages != 0 || target.PageCount() != 0 {
		t.Fatalf("conflicting target replay = pages %d target pages %d error %v", pages, target.PageCount(), err)
	}

	nonFresh := MustNew(WithUnit(UnitPoint))
	nonFresh.AddPage()
	before := nonFresh.PageCount()
	if pages, err := nonFresh.WriteLayoutDocumentPlan(plan); !errors.Is(err, ErrLayoutDocumentPlanUnsupported) ||
		!strings.Contains(err.Error(), "fresh error-free target") || pages != 0 || nonFresh.PageCount() != before {
		t.Fatalf("non-fresh replay = pages %d target pages %d error %v", pages, nonFresh.PageCount(), err)
	}
}
