// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"encoding/xml"
	"errors"
	"strconv"
	"strings"
	"time"
)

// PDFAMode identifies the PDF/A metadata profile to advertise.
type PDFAMode string

const (
	// PDFAModeNone disables PDF/A standards metadata.
	PDFAModeNone PDFAMode = ""
	// PDFAMode4 advertises base PDF/A-4 metadata.
	PDFAMode4 PDFAMode = "4"
	// PDFAMode4F advertises PDF/A-4f metadata.
	PDFAMode4F PDFAMode = "4f"
	// PDFAMode4E advertises PDF/A-4e metadata.
	PDFAMode4E PDFAMode = "4e"
)

// ComplianceMetadata controls standards identifiers, catalog markers, and
// tagged-PDF output.
//
// This API writes standards metadata and tagged-PDF structure support. It does
// not replace external PDF/A-4, PDF/UA-2, or Arlington PDF Model validation.
type ComplianceMetadata struct {
	PDFA       PDFAMode
	PDFUA2     bool
	Arlington  bool
	Lang       string
	Title      string
	Identifier string
}

type outputIntent struct {
	iccProfile []byte
	identifier string
	info       string
	registry   string
}

// ComplianceValidationSeverity classifies an external validator issue.
type ComplianceValidationSeverity string

const (
	// ComplianceValidationInfo is informational validator output.
	ComplianceValidationInfo ComplianceValidationSeverity = "info"
	// ComplianceValidationWarning is non-fatal validator output.
	ComplianceValidationWarning ComplianceValidationSeverity = "warning"
	// ComplianceValidationError is a failing validator finding.
	ComplianceValidationError ComplianceValidationSeverity = "error"
)

// ComplianceValidationIssue stores one finding from an external standards
// validator such as veraPDF, a PDF/UA checker, or the Arlington checker.
type ComplianceValidationIssue struct {
	Standard string
	Severity ComplianceValidationSeverity
	Rule     string
	Message  string
	Object   string
}

// ComplianceValidationReport stores external validation findings separately
// from Document generation errors.
type ComplianceValidationReport struct {
	Issues []ComplianceValidationIssue
}

// Add appends one external validation issue to the report.
func (report *ComplianceValidationReport) Add(issue ComplianceValidationIssue) {
	if strings.TrimSpace(string(issue.Severity)) == "" {
		issue.Severity = ComplianceValidationError
	}
	report.Issues = append(report.Issues, issue)
}

// Failed reports whether any issue has error severity.
func (report ComplianceValidationReport) Failed() bool {
	for _, issue := range report.Issues {
		if issue.Severity == ComplianceValidationError {
			return true
		}
	}
	return false
}

func (profile ComplianceMetadata) enabled() bool {
	return profile.PDFA != PDFAModeNone || profile.PDFUA2 || profile.Arlington
}

// SetComplianceMetadata enables generated XMP standards identifiers and basic
// catalog markers for standards-oriented output. It does not replace external
// validation with veraPDF, a PDF/UA checker, or the Arlington PDF Model checker.
func (f *Document) SetComplianceMetadata(profile ComplianceMetadata) {
	if profile.PDFA != PDFAModeNone {
		switch profile.PDFA {
		case PDFAMode4, PDFAMode4F, PDFAMode4E:
		default:
			f.SetErrorf("unsupported PDF/A metadata mode: %s", profile.PDFA)
			return
		}
	}
	if profile.PDFUA2 && strings.TrimSpace(profile.Lang) == "" {
		profile.Lang = "en-US"
	}
	f.compliance = profile
	if profile.PDFA != PDFAModeNone || profile.PDFUA2 || profile.Arlington {
		f.setMinimumPDFVersion("2.0")
	}
	if profile.PDFUA2 {
		f.EnableTaggedPDF()
	}
}

// SetOutputIntent embeds an ICC output profile and references it from the
// document catalog. PDF/A output requires a real output intent; callers should
// pass a valid ICC profile, such as an sRGB profile for DeviceRGB workflows.
func (f *Document) SetOutputIntent(iccProfile []byte, identifier string) error {
	if len(iccProfile) == 0 {
		err := errors.New("output intent ICC profile is empty")
		f.SetError(err)
		return err
	}
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		identifier = "sRGB IEC61966-2.1"
	}
	f.outputIntent = outputIntent{
		iccProfile: append([]byte(nil), iccProfile...),
		identifier: identifier,
		info:       identifier,
	}
	f.nOutputIntentICC = 0
	return nil
}

func (f *Document) setMinimumPDFVersion(version string) {
	if pdfVersionLess(f.pdfVersion, version) {
		f.pdfVersion = version
	}
}

func pdfVersionLess(current, minimum string) bool {
	currentMajor, currentMinor, currentOK := parsePDFVersion(current)
	minimumMajor, minimumMinor, minimumOK := parsePDFVersion(minimum)
	if !currentOK || !minimumOK {
		return current < minimum
	}
	if currentMajor != minimumMajor {
		return currentMajor < minimumMajor
	}
	return currentMinor < minimumMinor
}

func parsePDFVersion(version string) (int, int, bool) {
	majorStr, minorStr, ok := strings.Cut(strings.TrimSpace(version), ".")
	if !ok {
		return 0, 0, false
	}
	major, err := strconv.Atoi(majorStr)
	if err != nil {
		return 0, 0, false
	}
	minor, err := strconv.Atoi(minorStr)
	if err != nil {
		return 0, 0, false
	}
	return major, minor, true
}

func (f *Document) ensureComplianceMetadata() {
	if !f.compliance.enabled() || len(f.xmp) > 0 {
		return
	}
	f.xmp = f.buildComplianceXMP()
}

func (f *Document) validateComplianceMetadata() {
	if f.compliance.PDFA != PDFAModeNone {
		if f.protect.encrypted {
			f.SetErrorf("PDF/A metadata mode does not allow encrypted output")
			return
		}
		if f.javascript != nil {
			f.SetErrorf("PDF/A metadata mode does not allow JavaScript actions")
			return
		}
		if len(f.outputIntent.iccProfile) == 0 {
			f.SetErrorf("PDF/A metadata mode requires an ICC output intent")
			return
		}
		if !f.pdfAAllowsAttachments() && f.hasEmbeddedAttachments() {
			f.SetErrorf("PDF/A-4 metadata mode requires PDF/A-4f or PDF/A-4e for attachments")
			return
		}
		for _, font := range f.fonts {
			if font.Tp != "UTF8" || font.utf8File == nil {
				f.SetErrorf("PDF/A metadata mode requires embedded UTF-8 fonts with ToUnicode maps")
				return
			}
		}
	}
	if f.compliance.PDFUA2 {
		if strings.TrimSpace(f.compliance.Lang) == "" {
			f.SetErrorf("PDF/UA-2 metadata mode requires a document language")
			return
		}
		if strings.TrimSpace(firstNonEmpty(f.compliance.Title, f.title)) == "" {
			f.SetErrorf("PDF/UA-2 metadata mode requires a document title")
			return
		}
		if !f.tagged.enabled {
			f.SetErrorf("PDF/UA-2 metadata mode requires tagged PDF output")
			return
		}
		if len(f.tagged.elems) == 0 {
			f.SetErrorf("PDF/UA-2 metadata mode requires tagged content")
			return
		}
		if len(f.tagged.stack) > 0 {
			f.SetErrorf("PDF/UA-2 metadata mode has unclosed structure elements")
			return
		}
		if f.tagged.artifactDepth > 0 || f.tagged.pathArtifactOpen {
			f.SetErrorf("PDF/UA-2 metadata mode has unclosed artifact content")
			return
		}
	}
}

func (f *Document) pdfAAllowsAttachments() bool {
	return f.compliance.PDFA == PDFAMode4F || f.compliance.PDFA == PDFAMode4E
}

func (f *Document) hasEmbeddedAttachments() bool {
	if len(f.attachments) > 0 {
		return true
	}
	for _, annotations := range f.pageAttachments {
		if len(annotations) > 0 {
			return true
		}
	}
	return false
}

func (f *Document) buildComplianceXMP() []byte {
	var out bytes.Buffer
	creation := timeOrNow(f.creationDate)
	mod := timeOrNow(f.modDate)
	title := firstNonEmpty(f.compliance.Title, f.title)
	out.Grow(f.estimateComplianceXMPSize(title))

	out.WriteString(`<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?>` + "\n")
	out.WriteString(`<x:xmpmeta xmlns:x="adobe:ns:meta/">` + "\n")
	out.WriteString(`<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">` + "\n")
	out.WriteString(`<rdf:Description rdf:about=""`)
	out.WriteString(` xmlns:dc="http://purl.org/dc/elements/1.1/"`)
	out.WriteString(` xmlns:xmp="http://ns.adobe.com/xap/1.0/"`)
	out.WriteString(` xmlns:pdf="http://ns.adobe.com/pdf/1.3/"`)
	if f.compliance.PDFA != PDFAModeNone {
		out.WriteString(` xmlns:pdfaid="http://www.aiim.org/pdfa/ns/id/"`)
	}
	if f.compliance.PDFUA2 {
		out.WriteString(` xmlns:pdfuaid="http://www.aiim.org/pdfua/ns/id/"`)
	}
	if f.compliance.Arlington {
		out.WriteString(` xmlns:gopdfkit="https://github.com/cssbruno/gopdfkit/ns/compliance/1.0/"`)
	}
	out.WriteString(`>` + "\n")
	if title != "" {
		out.WriteString(`<dc:title><rdf:Alt><rdf:li xml:lang="x-default">`)
		xml.EscapeText(&out, []byte(title))
		out.WriteString(`</rdf:li></rdf:Alt></dc:title>` + "\n")
	}
	if f.author != "" {
		out.WriteString(`<dc:creator><rdf:Seq><rdf:li>`)
		xml.EscapeText(&out, []byte(f.author))
		out.WriteString(`</rdf:li></rdf:Seq></dc:creator>` + "\n")
	}
	if f.subject != "" {
		out.WriteString(`<dc:description><rdf:Alt><rdf:li xml:lang="x-default">`)
		xml.EscapeText(&out, []byte(f.subject))
		out.WriteString(`</rdf:li></rdf:Alt></dc:description>` + "\n")
	}
	if f.keywords != "" {
		out.WriteString(`<pdf:Keywords>`)
		xml.EscapeText(&out, []byte(f.keywords))
		out.WriteString(`</pdf:Keywords>` + "\n")
	}
	if f.producer != "" {
		out.WriteString(`<pdf:Producer>`)
		xml.EscapeText(&out, []byte(f.producer))
		out.WriteString(`</pdf:Producer>` + "\n")
	}
	if f.creator != "" {
		out.WriteString(`<xmp:CreatorTool>`)
		xml.EscapeText(&out, []byte(f.creator))
		out.WriteString(`</xmp:CreatorTool>` + "\n")
	}
	out.WriteString(`<xmp:CreateDate>`)
	out.WriteString(creation.Format(time.RFC3339))
	out.WriteString(`</xmp:CreateDate>` + "\n")
	out.WriteString(`<xmp:ModifyDate>`)
	out.WriteString(mod.Format(time.RFC3339))
	out.WriteString(`</xmp:ModifyDate>` + "\n")
	if f.compliance.Identifier != "" {
		out.WriteString(`<xmp:Identifier><rdf:Bag><rdf:li>`)
		xml.EscapeText(&out, []byte(f.compliance.Identifier))
		out.WriteString(`</rdf:li></rdf:Bag></xmp:Identifier>` + "\n")
	}
	if f.compliance.PDFA != PDFAModeNone {
		out.WriteString(`<pdfaid:part>4</pdfaid:part>` + "\n")
		out.WriteString(`<pdfaid:rev>2020</pdfaid:rev>` + "\n")
		if conformance := pdfA4Conformance(f.compliance.PDFA); conformance != "" {
			out.WriteString(`<pdfaid:conformance>`)
			out.WriteString(conformance)
			out.WriteString(`</pdfaid:conformance>` + "\n")
		}
	}
	if f.compliance.PDFUA2 {
		out.WriteString(`<pdfuaid:part>2</pdfuaid:part>` + "\n")
		out.WriteString(`<pdfuaid:rev>2024</pdfuaid:rev>` + "\n")
	}
	if f.compliance.Arlington {
		out.WriteString(`<gopdfkit:ArlingtonValidationRequired>True</gopdfkit:ArlingtonValidationRequired>` + "\n")
	}
	out.WriteString(`</rdf:Description>` + "\n")
	out.WriteString(`</rdf:RDF>` + "\n")
	out.WriteString(`</x:xmpmeta>` + "\n")
	out.WriteString(`<?xpacket end="w"?>`)
	return out.Bytes()
}

func (f *Document) estimateComplianceXMPSize(title string) int {
	size := 640
	size += 2 * (len(title) + len(f.author) + len(f.subject) + len(f.keywords) + len(f.producer) + len(f.creator) + len(f.compliance.Identifier))
	if f.compliance.PDFA != PDFAModeNone {
		size += 160
	}
	if f.compliance.PDFUA2 {
		size += 128
	}
	if f.compliance.Arlington {
		size += 128
	}
	return size
}

func pdfA4Conformance(mode PDFAMode) string {
	switch mode {
	case PDFAMode4F:
		return "F"
	case PDFAMode4E:
		return "E"
	default:
		return ""
	}
}
