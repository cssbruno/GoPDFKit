// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package pdfverify

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/cssbruno/paperrune/inspect"
)

const TagReportVersion uint16 = 1

type TagInspectionLimits struct {
	MaxPDFBytes uint64
	MaxNodes    uint32
	MaxDepth    uint16
}

func DefaultTagInspectionLimits() TagInspectionLimits {
	return TagInspectionLimits{MaxPDFBytes: 64 << 20, MaxNodes: 100_000, MaxDepth: 256}
}

func (limits TagInspectionLimits) valid() bool {
	hard := DefaultTagInspectionLimits()
	return limits.MaxPDFBytes > 0 && limits.MaxPDFBytes <= hard.MaxPDFBytes && limits.MaxNodes > 0 && limits.MaxNodes <= hard.MaxNodes && limits.MaxDepth > 0 && limits.MaxDepth <= hard.MaxDepth
}

type TagNodeEvidence struct {
	Object        uint32 `json:"object"`
	Parent        uint32 `json:"parent"`
	Role          string `json:"role"`
	Depth         uint16 `json:"depth"`
	PageObject    uint32 `json:"page_object,omitempty"`
	MarkedContent uint32 `json:"marked_content"`
	Children      uint32 `json:"children"`
	HasAlt        bool   `json:"has_alt,omitempty"`
	HasActualText bool   `json:"has_actual_text,omitempty"`
	HasLanguage   bool   `json:"has_language,omitempty"`
}

type TagReport struct {
	Version           uint16            `json:"version"`
	PDFSHA256         string            `json:"pdf_sha256"`
	StructureRoot     uint32            `json:"structure_root,omitempty"`
	ParentTree        uint32            `json:"parent_tree,omitempty"`
	DocumentElement   uint32            `json:"document_element,omitempty"`
	Marked            bool              `json:"marked"`
	MarkedContent     uint32            `json:"marked_content"`
	ContentMarked     uint32            `json:"content_marked"`
	ArtifactContent   uint32            `json:"artifact_content"`
	ContentEnds       uint32            `json:"content_ends"`
	StructureElements uint32            `json:"structure_elements"`
	Nodes             []TagNodeEvidence `json:"nodes,omitempty"`
	Failures          []string          `json:"failures,omitempty"`
	Passed            bool              `json:"passed"`
}

type tagObject struct {
	number uint32
	body   []byte
}

type parsedTagNode struct {
	evidence TagNodeEvidence
	children []uint32
}

var (
	tagObjectHeader = regexp.MustCompile(`(?m)(?:^|\r?\n)([1-9][0-9]*)[ \t]+0[ \t]+obj(?:[ \t]*\r?\n|[ \t]+)`)
	tagRootType     = regexp.MustCompile(`/Type[ \t\r\n]*/StructTreeRoot\b`)
	tagElementType  = regexp.MustCompile(`/Type[ \t\r\n]*/StructElem\b`)
	tagRole         = regexp.MustCompile(`/S[ \t\r\n]*/([A-Za-z0-9]+)\b`)
	tagParent       = regexp.MustCompile(`/P[ \t\r\n]+([1-9][0-9]*)[ \t\r\n]+0[ \t\r\n]+R\b`)
	tagPage         = regexp.MustCompile(`/Pg[ \t\r\n]+([1-9][0-9]*)[ \t\r\n]+0[ \t\r\n]+R\b`)
	tagRootKid      = regexp.MustCompile(`/K[ \t\r\n]+([1-9][0-9]*)[ \t\r\n]+0[ \t\r\n]+R\b`)
	tagParentTree   = regexp.MustCompile(`/ParentTree[ \t\r\n]+([1-9][0-9]*)[ \t\r\n]+0[ \t\r\n]+R\b`)
	tagMCID         = regexp.MustCompile(`/MCID[ \t\r\n]+[0-9]+\b`)
	tagAlt          = regexp.MustCompile(`/Alt\b`)
	tagActualText   = regexp.MustCompile(`/ActualText\b`)
	tagLanguage     = regexp.MustCompile(`/Lang\b`)
)

// InspectTags independently inspects the committed classic-PDF bytes. It does
// not consume layout-plan semantics and therefore cannot invent a tag that is
// absent from the serialized structure tree.
func InspectTags(ctx context.Context, pdf []byte, limits TagInspectionLimits) (TagReport, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if limits == (TagInspectionLimits{}) {
		limits = DefaultTagInspectionLimits()
	}
	if !limits.valid() || len(pdf) == 0 || uint64(len(pdf)) > limits.MaxPDFBytes {
		return TagReport{}, ErrLimit
	}
	if err := ctx.Err(); err != nil {
		return TagReport{}, err
	}
	digest := sha256.Sum256(pdf)
	report := TagReport{Version: TagReportVersion, PDFSHA256: hex.EncodeToString(digest[:]),
		Marked: bytes.Contains(pdf, []byte("/MarkInfo")) && bytes.Contains(pdf, []byte("/Marked true"))}
	objects, err := scanTagObjects(ctx, pdf, limits.MaxNodes+16)
	if err != nil {
		return TagReport{}, err
	}
	objectByNumber := make(map[uint32]tagObject, len(objects))
	nodes := make(map[uint32]*parsedTagNode)
	for _, object := range objects {
		objectByNumber[object.number] = object
		dictionary := object.body
		if marker := bytes.Index(dictionary, []byte("\nstream")); marker >= 0 {
			dictionary = dictionary[:marker]
		}
		switch {
		case tagRootType.Match(dictionary):
			if report.StructureRoot != 0 {
				report.Failures = append(report.Failures, "multiple structure-tree roots")
				continue
			}
			report.StructureRoot = object.number
			report.DocumentElement = firstTagReference(tagRootKid, dictionary)
			report.ParentTree = firstTagReference(tagParentTree, dictionary)
		case tagElementType.Match(dictionary):
			if uint32(len(nodes)) >= limits.MaxNodes { // #nosec G115 -- nodes are appended only after this MaxNodes bound check.
				return TagReport{}, ErrLimit
			}
			role := firstTagName(tagRole, dictionary)
			if role == "" || !validTagRole(role) {
				report.Failures = append(report.Failures, "structure element has an invalid role")
				continue
			}
			nodes[object.number] = &parsedTagNode{evidence: TagNodeEvidence{
				Object: object.number, Parent: firstTagReference(tagParent, dictionary), Role: role,
				PageObject: firstTagReference(tagPage, dictionary), MarkedContent: uint32(len(tagMCID.FindAll(dictionary, -1))), // #nosec G115 -- the bounded tag parser limits matched marked-content entries.
				HasAlt: tagAlt.Match(dictionary), HasActualText: tagActualText.Match(dictionary), HasLanguage: tagLanguage.Match(dictionary),
			}}
		}
	}
	report.StructureElements = uint32(len(nodes)) // #nosec G115 -- nodes are bounded by MaxNodes during parsing.
	if !report.Marked {
		report.Failures = append(report.Failures, "catalog is not marked as tagged")
	}
	if report.StructureRoot == 0 {
		report.Failures = append(report.Failures, "structure-tree root is absent")
	}
	if report.ParentTree == 0 || objectByNumber[report.ParentTree].number == 0 {
		report.Failures = append(report.Failures, "parent tree is absent")
	}
	document := nodes[report.DocumentElement]
	if report.DocumentElement == 0 || document == nil || document.evidence.Role != "Document" || document.evidence.Parent != report.StructureRoot {
		report.Failures = append(report.Failures, "structure-tree root does not reference one Document element")
	}
	for number, node := range nodes {
		if number == report.DocumentElement {
			continue
		}
		parent := nodes[node.evidence.Parent]
		if parent == nil {
			report.Failures = append(report.Failures, "structure element has a missing parent")
			continue
		}
		parent.children = append(parent.children, number)
	}
	if document != nil {
		visited := make(map[uint32]bool, len(nodes))
		active := make(map[uint32]bool, len(nodes))
		var walk func(uint32, uint16)
		walk = func(number uint32, depth uint16) {
			if active[number] {
				report.Failures = append(report.Failures, "structure tree contains a cycle")
				return
			}
			if visited[number] {
				return
			}
			if depth > limits.MaxDepth {
				report.Failures = append(report.Failures, "structure tree exceeds the depth limit")
				return
			}
			node := nodes[number]
			if node == nil {
				return
			}
			active[number], visited[number] = true, true
			sort.Slice(node.children, func(i, j int) bool { return node.children[i] < node.children[j] })
			node.evidence.Depth, node.evidence.Children = depth, uint32(len(node.children)) // #nosec G115 -- children originate from the bounded parsed node graph.
			report.MarkedContent += node.evidence.MarkedContent
			report.Nodes = append(report.Nodes, node.evidence)
			for _, child := range node.children {
				walk(child, depth+1)
			}
			active[number] = false
		}
		walk(report.DocumentElement, 0)
		if len(visited) != len(nodes) {
			report.Failures = append(report.Failures, "structure tree contains unreachable elements")
		}
	}
	streams, streamErr := inspect.DecodedStreamsContext(ctx, pdf)
	if streamErr != nil {
		return TagReport{}, streamErr
	}
	for _, stream := range streams {
		report.ContentMarked += countPDFOperator(stream, "BDC")
		report.ArtifactContent += countPDFOperator(stream, "BMC")
		report.ContentEnds += countPDFOperator(stream, "EMC")
	}
	if report.ContentMarked != report.MarkedContent {
		report.Failures = append(report.Failures, "marked-content streams do not match structure-tree MCR entries")
	}
	if report.ContentEnds != report.ContentMarked+report.ArtifactContent {
		report.Failures = append(report.Failures, "marked-content stream operators are unbalanced")
	}
	report.Failures = uniqueSortedStrings(report.Failures)
	report.Passed = len(report.Failures) == 0
	return report, nil
}

func countPDFOperator(stream []byte, operator string) uint32 {
	needle := []byte(operator)
	var count uint32
	for offset := 0; offset <= len(stream)-len(needle); {
		index := bytes.Index(stream[offset:], needle)
		if index < 0 {
			break
		}
		index += offset
		end := index + len(needle)
		if (index == 0 || isPDFOperatorSpace(stream[index-1])) && (end == len(stream) || isPDFOperatorSpace(stream[end])) {
			count++
		}
		offset = end
	}
	return count
}

func isPDFOperatorSpace(value byte) bool {
	switch value {
	case 0, '\t', '\n', '\f', '\r', ' ':
		return true
	default:
		return false
	}
}

func scanTagObjects(ctx context.Context, pdf []byte, max uint32) ([]tagObject, error) {
	matches := tagObjectHeader.FindAllSubmatchIndex(pdf, -1)
	if uint32(len(matches)) > max { // #nosec G115 -- the regex match count is compared to the caller's bounded limit.
		return nil, ErrLimit
	}
	objects := make([]tagObject, 0, len(matches))
	seen := make(map[uint32]struct{}, len(matches))
	for index, match := range matches {
		if index&255 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		number64, err := strconv.ParseUint(string(pdf[match[2]:match[3]]), 10, 32)
		if err != nil || number64 == 0 {
			return nil, ErrInvalid
		}
		number := uint32(number64)
		if _, duplicate := seen[number]; duplicate {
			return nil, ErrInvalid
		}
		seen[number] = struct{}{}
		end := len(pdf)
		if index+1 < len(matches) {
			end = matches[index+1][0]
		}
		body := pdf[match[1]:end]
		if marker := bytes.LastIndex(body, []byte("endobj")); marker >= 0 {
			body = body[:marker]
		}
		objects = append(objects, tagObject{number: number, body: body})
	}
	return objects, nil
}

func firstTagReference(pattern *regexp.Regexp, body []byte) uint32 {
	match := pattern.FindSubmatch(body)
	if len(match) != 2 {
		return 0
	}
	value, _ := strconv.ParseUint(string(match[1]), 10, 32)
	return uint32(value)
}

func firstTagName(pattern *regexp.Regexp, body []byte) string {
	match := pattern.FindSubmatch(body)
	if len(match) != 2 {
		return ""
	}
	return string(match[1])
}

func validTagRole(role string) bool {
	if role == "" || len(role) > 64 || !utf8.ValidString(role) || strings.TrimSpace(role) != role {
		return false
	}
	for _, r := range role {
		if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return true
}

func uniqueSortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	sort.Strings(values)
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}
