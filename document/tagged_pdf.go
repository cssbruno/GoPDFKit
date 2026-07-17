// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import "strings"

const (
	taggedRoleP       = "P"
	taggedRoleFigure  = "Figure"
	taggedRoleLink    = "Link"
	taggedRoleTable   = "Table"
	taggedRoleTR      = "TR"
	taggedRoleTD      = "TD"
	taggedRoleTH      = "TH"
	taggedRoleL       = "L"
	taggedRoleLI      = "LI"
	taggedRoleLbl     = "Lbl"
	taggedRoleLBody   = "LBody"
	taggedRoleCaption = "Caption"
)

var (
	taggedArtifactMarkedContent = []byte("/Artifact BMC\n")
	taggedEndMarkedContent      = []byte("EMC")
)

type taggedPDFState struct {
	enabled           bool
	documentLanguage  string
	structTreeRootObj int
	parentTreeObj     int
	documentElemObj   int
	namespaceObj      int
	nextStructParent  int
	pageStructParents []int
	pageObjNums       []int
	pageElems         [][]*taggedElement
	elems             []*taggedElement
	stack             []*taggedElement
	nextText          taggedContentOptions
	parentTreeObjects []taggedParentTreeObject
	pendingLinkElem   *taggedElement
	pathArtifactOpen  bool
	artifactDepth     int
}

type taggedElement struct {
	ObjNum     int
	Page       int
	MCID       int
	Role       string
	Alt        string
	ActualText string
	Lang       string
	ObjRef     int
	Parent     *taggedElement
	Table      taggedTableAttributes
	Marked     []taggedMarkedContent
	Children   []*taggedElement
}

type taggedTableAttributes struct {
	Scope   string
	RowSpan int
	ColSpan int
}

type taggedMarkedContent struct {
	Page int
	MCID int
}

type taggedParentTreeObject struct {
	Key  int
	Elem *taggedElement
}

type taggedContentOptions struct {
	Role     string
	AltText  string
	Artifact bool
}

// EnableTaggedPDF enables structure-tree and marked-content output. PDF/UA-2
// metadata enables this automatically.
func (f *Document) EnableTaggedPDF() {
	f.tagged.enabled = true
	f.setMinimumPDFVersion("2.0")
}

// SetNextTextRole sets the structure role for the next text operation. Common
// values are P, H1-H6, L, LI, Lbl, LBody, Caption, Span, and Link.
func (f *Document) SetNextTextRole(role string) {
	role = normalizeTaggedRole(role)
	if role == "" {
		f.SetErrorf("invalid tagged PDF role")
		return
	}
	f.tagged.nextText = taggedContentOptions{Role: role}
}

// SetNextTextArtifact marks the next text operation as an artifact.
func (f *Document) SetNextTextArtifact() {
	f.tagged.nextText = taggedContentOptions{Artifact: true}
}

// BeginStructure starts a tagged PDF structure container. Containers define
// reading order and semantic grouping for subsequent marked content.
func (f *Document) BeginStructure(role string) {
	if !f.tagged.enabled {
		return
	}
	role = normalizeTaggedRole(role)
	if role == "" {
		f.SetErrorf("invalid tagged PDF role")
		return
	}
	elem := &taggedElement{Role: role, MCID: -1}
	if parent := f.currentStructureParent(); parent != nil {
		elem.Parent = parent
		parent.Children = append(parent.Children, elem)
	}
	f.tagged.elems = append(f.tagged.elems, elem)
	f.tagged.stack = append(f.tagged.stack, elem)
}

func (f *Document) beginTableCellStructure(role string, attrs taggedTableAttributes) {
	if !f.tagged.enabled {
		return
	}
	f.BeginStructure(role)
	if len(f.tagged.stack) == 0 {
		return
	}
	elem := f.tagged.stack[len(f.tagged.stack)-1]
	elem.Table = normalizeTaggedTableAttributes(role, attrs)
}

// EndStructure closes the most recent structure container.
func (f *Document) EndStructure() {
	if !f.tagged.enabled {
		return
	}
	if len(f.tagged.stack) == 0 {
		f.SetErrorf("tagged PDF structure stack is empty")
		return
	}
	f.tagged.stack = f.tagged.stack[:len(f.tagged.stack)-1]
}

// BeginArtifact marks subsequent low-level drawing operations as artifact
// content until EndArtifact is called.
func (f *Document) BeginArtifact() {
	if f.tagged.enabled {
		if f.tagged.artifactDepth == 0 {
			f.outbytes(f.beginTaggedContent(taggedContentOptions{Artifact: true}))
		}
		f.tagged.artifactDepth++
	}
}

// EndArtifact closes an artifact content span started with BeginArtifact.
func (f *Document) EndArtifact() {
	if f.tagged.enabled {
		if f.tagged.artifactDepth <= 0 {
			f.SetErrorf("tagged PDF artifact stack is empty")
			return
		}
		f.tagged.artifactDepth--
		if f.tagged.artifactDepth == 0 {
			f.out("EMC")
		}
	}
}

func (f *Document) beginPathArtifact() {
	if f.tagged.enabled && !f.tagged.pathArtifactOpen {
		f.BeginArtifact()
		f.tagged.pathArtifactOpen = true
	}
}

func (f *Document) endPathArtifact() {
	if f.tagged.enabled && f.tagged.pathArtifactOpen {
		f.tagged.pathArtifactOpen = false
		f.EndArtifact()
	}
}

func (f *Document) taggedBeginPage(page int) {
	if !f.tagged.enabled {
		return
	}
	f.tagged.pageStructParents = ensureIntSliceLen(f.tagged.pageStructParents, page+1)
	f.tagged.pageElems = ensureTaggedPageElemsLen(f.tagged.pageElems, page+1)
	if f.tagged.pageStructParents[page] < 0 && len(f.tagged.pageElems[page]) == 0 {
		f.tagged.pageStructParents[page] = f.tagged.nextStructParent
		f.tagged.nextStructParent++
	}
}

func (f *Document) taggedPageStructParents(page int) int {
	if !f.tagged.enabled || page <= 0 || page >= len(f.tagged.pageStructParents) {
		return -1
	}
	return f.tagged.pageStructParents[page]
}

func (f *Document) consumeNextTextTag(link bool) taggedContentOptions {
	tag := f.tagged.nextText
	f.tagged.nextText = taggedContentOptions{}
	if !f.tagged.enabled {
		return taggedContentOptions{}
	}
	if tag.Artifact {
		return tag
	}
	if tag.Role == "" {
		if link {
			tag.Role = taggedRoleLink
		} else {
			tag.Role = taggedRoleP
		}
	}
	tag.Role = normalizeTaggedRole(tag.Role)
	if tag.Role == "" {
		tag.Role = taggedRoleP
	}
	return tag
}

func (f *Document) validateTaggedImageOptions(tag taggedContentOptions) bool {
	if !f.tagged.enabled || tag.Artifact {
		return true
	}
	if strings.TrimSpace(tag.AltText) == "" {
		f.SetErrorf("tagged PDF images require alternate text or Artifact=true")
		return false
	}
	return true
}

func (f *Document) outTaggedContent(content []byte, tag taggedContentOptions) {
	if !f.tagged.enabled || len(content) == 0 || (tag.Artifact && f.tagged.artifactDepth > 0) {
		f.outbytes(content)
		return
	}
	begin := f.beginTaggedContent(tag)
	if len(begin) == 0 {
		f.outbytes(content)
		return
	}
	f.writeRawBytes(begin)
	f.writeRawBytes(content)
	if content[len(content)-1] != '\n' {
		f.writeRawByte('\n')
	}
	f.writeRawBytes(taggedEndMarkedContent)
	f.writeRawByte('\n')
}

func (f *Document) beginTaggedContent(tag taggedContentOptions) []byte {
	if !f.tagged.enabled {
		return nil
	}
	if tag.Artifact {
		return taggedArtifactMarkedContent
	}
	role := normalizeTaggedRole(tag.Role)
	if role == "" {
		role = taggedRoleP
	}
	elem, mcid := f.registerTaggedElement(role, tag.AltText)
	if role == taggedRoleLink {
		f.tagged.pendingLinkElem = elem
	}
	out := make([]byte, 0, len(role)+24)
	out = append(out, '/')
	out = append(out, role...)
	out = append(out, " <</MCID "...)
	out = appendPDFInt(out, mcid)
	out = append(out, ">> BDC\n"...)
	return out
}

// registerPreparedSemanticElement attaches a new marked-content reference to
// an already materialized plan semantic element. Unlike registerTaggedElement,
// this keeps one structure element for a semantic node even when its content
// is split across glyph runs, fragments, or pages.
func (f *Document) registerPreparedSemanticElement(elem *taggedElement) int {
	if !f.tagged.enabled || elem == nil || f.page <= 0 {
		return -1
	}
	f.tagged.pageStructParents = ensureIntSliceLen(f.tagged.pageStructParents, f.page+1)
	f.tagged.pageElems = ensureTaggedPageElemsLen(f.tagged.pageElems, f.page+1)
	if f.tagged.pageStructParents[f.page] < 0 && len(f.tagged.pageElems[f.page]) == 0 {
		f.tagged.pageStructParents[f.page] = f.tagged.nextStructParent
		f.tagged.nextStructParent++
	}
	mcid := len(f.tagged.pageElems[f.page])
	elem.Marked = append(elem.Marked, taggedMarkedContent{Page: f.page, MCID: mcid})
	f.tagged.pageElems[f.page] = append(f.tagged.pageElems[f.page], elem)
	return mcid
}

func (f *Document) beginPreparedTaggedContent(role string, mcid int) []byte {
	if !f.tagged.enabled || mcid < 0 {
		return nil
	}
	role = normalizeTaggedRole(role)
	if role == "" {
		role = taggedRoleP
	}
	out := make([]byte, 0, len(role)+32)
	out = append(out, '/')
	out = append(out, role...)
	out = append(out, " <</MCID "...)
	out = appendPDFInt(out, mcid)
	out = append(out, ">> BDC\n"...)
	return out
}

func (f *Document) registerTaggedElement(role, alt string) (*taggedElement, int) {
	page := f.page
	if page <= 0 {
		return &taggedElement{Role: role, MCID: 0}, 0
	}
	alt = strings.TrimSpace(alt)
	f.tagged.pageStructParents = ensureIntSliceLen(f.tagged.pageStructParents, page+1)
	f.tagged.pageElems = ensureTaggedPageElemsLen(f.tagged.pageElems, page+1)
	if f.tagged.pageStructParents[page] < 0 && len(f.tagged.pageElems[page]) == 0 {
		f.tagged.pageStructParents[page] = f.tagged.nextStructParent
		f.tagged.nextStructParent++
	}
	mcid := len(f.tagged.pageElems[page])
	if parent := f.currentStructureParent(); parent != nil && parent.Role == role && alt == "" {
		parent.Marked = append(parent.Marked, taggedMarkedContent{Page: page, MCID: mcid})
		f.tagged.pageElems[page] = append(f.tagged.pageElems[page], parent)
		return parent, mcid
	}
	elem := &taggedElement{Page: page, MCID: mcid, Role: role, Alt: strings.TrimSpace(alt)}
	if parent := f.currentStructureParent(); parent != nil {
		elem.Parent = parent
		parent.Children = append(parent.Children, elem)
	}
	f.tagged.pageElems[page] = append(f.tagged.pageElems[page], elem)
	f.tagged.elems = append(f.tagged.elems, elem)
	return elem, mcid
}

func (f *Document) currentStructureParent() *taggedElement {
	if len(f.tagged.stack) == 0 {
		return nil
	}
	return f.tagged.stack[len(f.tagged.stack)-1]
}

func (f *Document) taggedLinkAnnotation() (*taggedElement, int) {
	if !f.tagged.enabled {
		return nil, -1
	}
	elem := f.tagged.pendingLinkElem
	f.tagged.pendingLinkElem = nil
	if elem == nil {
		elem = &taggedElement{Page: f.page, MCID: -1, Role: taggedRoleLink}
		if parent := f.currentStructureParent(); parent != nil {
			elem.Parent = parent
			parent.Children = append(parent.Children, elem)
		}
		f.tagged.elems = append(f.tagged.elems, elem)
	}
	key := f.tagged.nextStructParent
	f.tagged.nextStructParent++
	f.tagged.parentTreeObjects = append(f.tagged.parentTreeObjects, taggedParentTreeObject{Key: key, Elem: elem})
	return elem, key
}

func (f *Document) putTaggedPDF() {
	if !f.tagged.enabled {
		return
	}
	startObj := f.n + 1
	for i, elem := range f.tagged.elems {
		elem.ObjNum = startObj + i
	}
	f.tagged.documentElemObj = startObj + len(f.tagged.elems)
	f.tagged.parentTreeObj = f.tagged.documentElemObj + 1
	f.tagged.namespaceObj = f.tagged.parentTreeObj + 1
	f.tagged.structTreeRootObj = f.tagged.namespaceObj + 1

	for _, elem := range f.tagged.elems {
		f.putTaggedElement(elem)
	}
	f.putTaggedDocumentElement()
	f.putTaggedParentTree()
	f.putTaggedNamespace()
	f.putTaggedStructTreeRoot()
}

func (f *Document) putTaggedElement(elem *taggedElement) {
	f.newPDFDictObject()
	f.out("/Type /StructElem")
	f.outf("/S /%s", elem.Role)
	if elem.Parent != nil {
		f.outf("/P %d 0 R", elem.Parent.ObjNum)
	} else {
		f.outf("/P %d 0 R", f.tagged.documentElemObj)
	}
	if f.tagged.namespaceObj > 0 {
		f.outf("/NS %d 0 R", f.tagged.namespaceObj)
	}
	if elem.Page > 0 && elem.Page < len(f.tagged.pageObjNums) && f.tagged.pageObjNums[elem.Page] > 0 {
		f.outf("/Pg %d 0 R", f.tagged.pageObjNums[elem.Page])
	}
	if elem.Alt != "" {
		buf := f.appendUTF16TextString([]byte("/Alt "), elem.Alt)
		f.outbytes(buf)
	}
	if elem.ActualText != "" {
		buf := f.appendUTF16TextString([]byte("/ActualText "), elem.ActualText)
		f.outbytes(buf)
	}
	if elem.Lang != "" {
		buf := f.appendUTF16TextString([]byte("/Lang "), elem.Lang)
		f.outbytes(buf)
	}
	if attr := f.taggedTableAttributeString(elem); attr != "" {
		f.outf("/A %s", attr)
	} else if elem.Role == taggedRoleL {
		f.out("/A << /O /List /ListNumbering /Disc >>")
	}
	kidCount := taggedElementKidCount(elem)
	switch kidCount {
	case 0:
		f.out("/K []")
	case 1:
		out := make([]byte, 0, 64)
		out = append(out, "/K "...)
		out = f.appendTaggedElementKids(out, elem, false)
		f.outbytes(out)
	default:
		out := make([]byte, 0, 8+kidCount*32)
		out = append(out, "/K ["...)
		out = f.appendTaggedElementKids(out, elem, true)
		out = append(out, ']')
		f.outbytes(out)
	}
	f.endPDFDict()
	f.endPDFObject()
}

func (f *Document) taggedTableAttributeString(elem *taggedElement) string {
	if elem == nil || (elem.Table.Scope == "" && elem.Table.RowSpan <= 1 && elem.Table.ColSpan <= 1) {
		return ""
	}
	out := make([]byte, 0, 64)
	out = append(out, "<< /O /Table"...)
	if elem.Table.Scope != "" {
		out = append(out, " /Scope /"...)
		out = append(out, elem.Table.Scope...)
	}
	if elem.Table.RowSpan > 1 {
		out = append(out, " /RowSpan "...)
		out = appendPDFInt(out, elem.Table.RowSpan)
	}
	if elem.Table.ColSpan > 1 {
		out = append(out, " /ColSpan "...)
		out = appendPDFInt(out, elem.Table.ColSpan)
	}
	out = append(out, " >>"...)
	return string(out)
}

func normalizeTaggedTableAttributes(role string, attrs taggedTableAttributes) taggedTableAttributes {
	role = normalizeTaggedRole(role)
	if role != taggedRoleTH && role != taggedRoleTD {
		return taggedTableAttributes{}
	}
	if attrs.RowSpan < 1 {
		attrs.RowSpan = 1
	}
	if attrs.ColSpan < 1 {
		attrs.ColSpan = 1
	}
	scope := normalizeTaggedRole(attrs.Scope)
	if role == taggedRoleTH {
		switch scope {
		case "Row", "Column", "Both":
			attrs.Scope = scope
		default:
			attrs.Scope = "Column"
		}
	} else {
		attrs.Scope = ""
	}
	return attrs
}

func taggedElementKidCount(elem *taggedElement) int {
	count := len(elem.Marked) + len(elem.Children)
	if elem.MCID >= 0 {
		count++
	}
	if elem.ObjRef > 0 {
		count++
	}
	return count
}

func (f *Document) appendTaggedElementKids(out []byte, elem *taggedElement, trailingSpaces bool) []byte {
	if elem.MCID >= 0 {
		out = f.appendTaggedMCR(out, elem.Page, elem.MCID)
		if trailingSpaces {
			out = append(out, ' ')
		}
	}
	for _, marked := range elem.Marked {
		out = f.appendTaggedMCR(out, marked.Page, marked.MCID)
		if trailingSpaces {
			out = append(out, ' ')
		}
	}
	for _, child := range elem.Children {
		out = appendPDFObjectRef(out, child.ObjNum)
		if trailingSpaces {
			out = append(out, ' ')
		}
	}
	if elem.ObjRef > 0 {
		out = append(out, "<< /Type /OBJR /Obj "...)
		out = appendPDFObjectRef(out, elem.ObjRef)
		out = append(out, " >>"...)
		if trailingSpaces {
			out = append(out, ' ')
		}
	}
	return out
}

func (f *Document) appendTaggedMCR(out []byte, page, mcid int) []byte {
	out = append(out, "<< /Type /MCR /Pg "...)
	out = appendPDFObjectRef(out, f.tagged.pageObjNums[page])
	out = append(out, " /MCID "...)
	out = appendPDFInt(out, mcid)
	out = append(out, " >>"...)
	return out
}

func (f *Document) putTaggedDocumentElement() {
	f.newPDFDictObject()
	f.out("/Type /StructElem")
	f.out("/S /Document")
	f.outf("/P %d 0 R", f.tagged.structTreeRootObj)
	if language := firstNonEmpty(f.tagged.documentLanguage, f.compliance.Lang); language != "" {
		buf := f.appendUTF16TextString([]byte("/Lang "), language)
		f.outbytes(buf)
	}
	if f.tagged.namespaceObj > 0 {
		f.outf("/NS %d 0 R", f.tagged.namespaceObj)
	}
	if len(f.tagged.elems) > 0 {
		kids := make([]byte, 0, 8+len(f.tagged.elems)*8)
		kids = append(kids, "/K ["...)
		for _, elem := range f.tagged.elems {
			if elem.Parent == nil {
				kids = appendPDFObjectRef(kids, elem.ObjNum)
				kids = append(kids, ' ')
			}
		}
		kids = append(kids, ']')
		f.outbytes(kids)
	} else {
		f.out("/K []")
	}
	f.endPDFDict()
	f.endPDFObject()
}

func (f *Document) putTaggedParentTree() {
	f.newPDFDictObject()
	nums := make([]byte, 0, 64)
	nums = append(nums, "/Nums ["...)
	for page := 1; page < len(f.tagged.pageElems); page++ {
		key := f.taggedPageStructParents(page)
		if key < 0 {
			continue
		}
		nums = appendPDFInt(nums, key)
		nums = append(nums, " ["...)
		for _, elem := range f.tagged.pageElems[page] {
			nums = appendPDFObjectRef(nums, elem.ObjNum)
			nums = append(nums, ' ')
		}
		nums = append(nums, "] "...)
	}
	for _, obj := range f.tagged.parentTreeObjects {
		if obj.Elem == nil {
			continue
		}
		nums = appendPDFInt(nums, obj.Key)
		nums = append(nums, ' ')
		nums = appendPDFObjectRef(nums, obj.Elem.ObjNum)
		nums = append(nums, ' ')
	}
	nums = append(nums, ']')
	f.outbytes(nums)
	f.endPDFDict()
	f.endPDFObject()
}

func (f *Document) putTaggedStructTreeRoot() {
	f.newPDFDictObject()
	f.out("/Type /StructTreeRoot")
	f.outf("/K %d 0 R", f.tagged.documentElemObj)
	f.outf("/ParentTree %d 0 R", f.tagged.parentTreeObj)
	if f.tagged.namespaceObj > 0 {
		f.outf("/Namespaces [%d 0 R]", f.tagged.namespaceObj)
	}
	f.endPDFDict()
	f.endPDFObject()
}

func (f *Document) putTaggedNamespace() {
	f.newPDFDictObject()
	f.out("/Type /Namespace")
	f.out("/NS (http://iso.org/pdf2/ssn)")
	f.endPDFDict()
	f.endPDFObject()
}

func normalizeTaggedRole(role string) string {
	role = strings.TrimSpace(strings.TrimPrefix(role, "/"))
	if role == "" {
		return ""
	}
	for _, r := range role {
		if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			continue
		}
		return ""
	}
	return role
}

func ensureIntSliceLen(in []int, size int) []int {
	for len(in) < size {
		in = append(in, -1)
	}
	return in
}

func ensureTaggedPageElemsLen(in [][]*taggedElement, size int) [][]*taggedElement {
	for len(in) < size {
		in = append(in, nil)
	}
	return in
}
