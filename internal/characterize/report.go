// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

// Package characterize produces bounded, deterministic compatibility evidence
// from generated PDFs. It is intentionally independent from the layout path:
// a fixture must first render, then this package inspects the committed output.
package characterize

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/cssbruno/gopdfkit/inspect"
)

const ReportSchemaVersion uint16 = 1

var (
	ErrInvalid = errors.New("characterize: invalid input")
	ErrLimit   = errors.New("characterize: limit exceeded")
)

type Limits struct {
	MaxFixtures   uint32
	MaxPDFBytes   uint64
	MaxTotalBytes uint64
	MaxTextBytes  uint64
	MaxNameBytes  uint32
	MaxJSONBytes  uint64
}

func DefaultLimits() Limits {
	return Limits{MaxFixtures: 1024, MaxPDFBytes: 64 << 20, MaxTotalBytes: 512 << 20,
		MaxTextBytes: 32 << 20, MaxNameBytes: 4096, MaxJSONBytes: 128 << 20}
}

func (limits Limits) validate() error {
	hard := DefaultLimits()
	if limits.MaxFixtures == 0 || limits.MaxPDFBytes == 0 || limits.MaxTotalBytes == 0 || limits.MaxTextBytes == 0 ||
		limits.MaxNameBytes == 0 || limits.MaxJSONBytes == 0 || limits.MaxFixtures > hard.MaxFixtures ||
		limits.MaxPDFBytes > hard.MaxPDFBytes || limits.MaxTotalBytes > hard.MaxTotalBytes || limits.MaxTextBytes > hard.MaxTextBytes ||
		limits.MaxNameBytes > hard.MaxNameBytes || limits.MaxJSONBytes > hard.MaxJSONBytes || limits.MaxPDFBytes > limits.MaxTotalBytes {
		return ErrLimit
	}
	return nil
}

type Artifact struct {
	Name string
	PDF  []byte
}

type Fingerprint struct {
	GOOS      string `json:"goos"`
	GOARCH    string `json:"goarch"`
	GoVersion string `json:"go_version"`
	CPUs      int    `json:"cpus"`
}

func RuntimeFingerprint() Fingerprint {
	return Fingerprint{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, GoVersion: runtime.Version(), CPUs: runtime.NumCPU()}
}

// PDFStructure records stable structural assertions used by compatibility
// baselines. Counts are taken from syntax tokens in the complete generated PDF;
// text order is independently extracted from page content streams.
type PDFStructure struct {
	LinkAnnotations   uint32 `json:"link_annotations"`
	URIActions        uint32 `json:"uri_actions"`
	Destinations      uint32 `json:"destinations"`
	Widgets           uint32 `json:"widgets"`
	EmbeddedFiles     uint32 `json:"embedded_files"`
	Attachments       uint32 `json:"attachments"`
	StructureTrees    uint32 `json:"structure_trees"`
	MarkedContent     uint32 `json:"marked_content"`
	OutputIntents     uint32 `json:"output_intents"`
	StructureElements uint32 `json:"structure_elements"`
	ParentTrees       uint32 `json:"parent_trees"`
	AssociatedFiles   uint32 `json:"associated_files"`
	HasAcroForm       bool   `json:"has_acro_form"`
	HasTagMarkInfo    bool   `json:"has_tag_mark_info"`
	HasMetadata       bool   `json:"has_metadata"`
	HasLanguage       bool   `json:"has_language"`
	DisplaysTitle     bool   `json:"displays_title"`
	HasEmbeddedICC    bool   `json:"has_embedded_icc"`
	PDFUA2            bool   `json:"pdfua_2"`
	PDFA4             bool   `json:"pdfa_4"`
	PDFAConformance   string `json:"pdfa_conformance,omitempty"`
	ArlingtonRequired bool   `json:"arlington_required"`
}

type FixtureEvidence struct {
	Name       string       `json:"name"`
	PDFVersion string       `json:"pdf_version"`
	SHA256     string       `json:"sha256"`
	Bytes      uint64       `json:"bytes"`
	Pages      uint32       `json:"pages"`
	Text       string       `json:"text"`
	PageText   []string     `json:"page_text"`
	Structure  PDFStructure `json:"structure"`
}

type Report struct {
	SchemaVersion uint16            `json:"schema_version"`
	Command       string            `json:"command"`
	Fingerprint   Fingerprint       `json:"fingerprint"`
	Fixtures      []FixtureEvidence `json:"fixtures"`
}

func Build(ctx context.Context, artifacts []Artifact, command string, fingerprint Fingerprint, limits Limits) (Report, error) {
	if ctx == nil || strings.TrimSpace(command) == "" || !utf8.ValidString(command) ||
		strings.TrimSpace(fingerprint.GOOS) == "" || strings.TrimSpace(fingerprint.GOARCH) == "" ||
		strings.TrimSpace(fingerprint.GoVersion) == "" || !utf8.ValidString(fingerprint.GOOS) ||
		!utf8.ValidString(fingerprint.GOARCH) || !utf8.ValidString(fingerprint.GoVersion) || fingerprint.CPUs <= 0 {
		return Report{}, ErrInvalid
	}
	if err := limits.validate(); err != nil {
		return Report{}, err
	}
	if len(artifacts) == 0 || uint64(len(artifacts)) > uint64(limits.MaxFixtures) {
		return Report{}, ErrLimit
	}
	ordered := append([]Artifact(nil), artifacts...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Name < ordered[j].Name })
	report := Report{SchemaVersion: ReportSchemaVersion, Command: command, Fingerprint: fingerprint,
		Fixtures: make([]FixtureEvidence, 0, len(ordered))}
	var total uint64
	previous := ""
	for _, artifact := range ordered {
		if artifact.Name == "" || artifact.Name <= previous || strings.TrimSpace(artifact.Name) != artifact.Name ||
			!utf8.ValidString(artifact.Name) || uint64(len(artifact.Name)) > uint64(limits.MaxNameBytes) {
			return Report{}, ErrInvalid
		}
		previous = artifact.Name
		if len(artifact.PDF) == 0 || uint64(len(artifact.PDF)) > limits.MaxPDFBytes || uint64(len(artifact.PDF)) > limits.MaxTotalBytes-total {
			return Report{}, ErrLimit
		}
		total += uint64(len(artifact.PDF))
	}
	for _, artifact := range ordered {
		if err := ctx.Err(); err != nil {
			return Report{}, err
		}
		evidence, err := inspectArtifact(ctx, artifact, limits.MaxTextBytes)
		if err != nil {
			return Report{}, fmt.Errorf("characterize %q: %w", artifact.Name, err)
		}
		report.Fixtures = append(report.Fixtures, evidence)
	}
	if _, err := report.CanonicalJSON(limits.MaxJSONBytes); err != nil {
		return Report{}, err
	}
	return report, nil
}

func inspectArtifact(ctx context.Context, artifact Artifact, maxText uint64) (FixtureEvidence, error) {
	pages, err := inspect.PageCountContext(ctx, artifact.PDF)
	if err != nil || pages <= 0 || uint64(pages) > uint64(^uint32(0)) {
		return FixtureEvidence{}, fmt.Errorf("page count: %w", err)
	}
	pageText := make([]string, pages)
	var text strings.Builder
	for page := 1; page <= pages; page++ {
		value, err := inspect.PageTextContext(ctx, artifact.PDF, page)
		if err != nil {
			return FixtureEvidence{}, fmt.Errorf("page %d text: %w", page, err)
		}
		value = normalizeExtractedText(value)
		if uint64(text.Len()+len(value)) > maxText {
			return FixtureEvidence{}, ErrLimit
		}
		pageText[page-1] = value
		text.WriteString(value)
	}
	digest := sha256.Sum256(artifact.PDF)
	return FixtureEvidence{Name: artifact.Name, PDFVersion: pdfVersion(artifact.PDF), SHA256: hex.EncodeToString(digest[:]),
		Bytes: uint64(len(artifact.PDF)), Pages: uint32(pages), Text: text.String(), PageText: pageText,
		Structure: inspectStructure(artifact.PDF)}, nil
}

func normalizeExtractedText(value string) string {
	raw := []byte(value)
	if len(raw) == 0 || len(raw)%2 != 0 || !bytes.Contains(raw, []byte{0}) {
		return value
	}
	units := make([]uint16, len(raw)/2)
	for index := range units {
		units[index] = uint16(raw[index*2])<<8 | uint16(raw[index*2+1])
	}
	decoded := string(utf16.Decode(units))
	if decoded == "" || strings.ContainsRune(decoded, 0) || !utf8.ValidString(decoded) {
		return value
	}
	return decoded
}

func pdfVersion(data []byte) string {
	if len(data) >= 8 && bytes.HasPrefix(data, []byte("%PDF-")) {
		return string(data[5:8])
	}
	return "unknown"
}

func inspectStructure(data []byte) PDFStructure {
	count := func(token string) uint32 {
		matches := bytes.Count(data, []byte(token))
		if uint64(matches) > uint64(^uint32(0)) {
			return ^uint32(0)
		}
		return uint32(matches)
	}
	return PDFStructure{
		LinkAnnotations: count("/Subtype /Link"), URIActions: count("/URI "), Destinations: count("/Dest "),
		Widgets: count("/Subtype /Widget"), EmbeddedFiles: count("/EmbeddedFiles"), Attachments: count("/Filespec"),
		StructureTrees: count("/StructTreeRoot"), MarkedContent: count(" BDC"), OutputIntents: count("/OutputIntent"),
		StructureElements: count("/Type /StructElem"), ParentTrees: count("/ParentTree"),
		AssociatedFiles: count("/AFRelationship"),
		HasAcroForm:     bytes.Contains(data, []byte("/AcroForm")),
		HasTagMarkInfo:  bytes.Contains(data, []byte("/MarkInfo")) && bytes.Contains(data, []byte("/Marked true")),
		HasMetadata:     bytes.Contains(data, []byte("/Metadata")), HasLanguage: bytes.Contains(data, []byte("/Lang ")),
		DisplaysTitle:     bytes.Contains(data, []byte("/DisplayDocTitle true")),
		HasEmbeddedICC:    bytes.Contains(data, []byte("/DestOutputProfile")),
		PDFUA2:            bytes.Contains(data, []byte("<pdfuaid:part>2</pdfuaid:part>")),
		PDFA4:             bytes.Contains(data, []byte("<pdfaid:part>4</pdfaid:part>")),
		PDFAConformance:   pdfAConformance(data),
		ArlingtonRequired: bytes.Contains(data, []byte("<gopdfkit:ArlingtonValidationRequired>True</gopdfkit:ArlingtonValidationRequired>")),
	}
}

func pdfAConformance(data []byte) string {
	for _, value := range []string{"F", "E"} {
		if bytes.Contains(data, []byte("<pdfaid:conformance>"+value+"</pdfaid:conformance>")) {
			return value
		}
	}
	return ""
}

func (report Report) CanonicalJSON(maxBytes uint64) ([]byte, error) {
	if report.SchemaVersion != ReportSchemaVersion || maxBytes == 0 {
		return nil, ErrInvalid
	}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return nil, err
	}
	encoded = append(encoded, '\n')
	if uint64(len(encoded)) > maxBytes {
		return nil, ErrLimit
	}
	return encoded, nil
}
