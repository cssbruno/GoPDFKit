// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

func (f *Document) newobj() {
	f.n++
	for j := len(f.offsets); j <= f.n; j++ {
		f.offsets = append(f.offsets, 0)
	}
	f.offsets[f.n] = f.finalOutputOffset()
	if f.hooks.OnOutputObject != nil {
		f.hooks.OnOutputObject(f.n, "object")
	}
	f.outPDFObjHeader(f.n)
}

func (f *Document) beginPDFObject(objNum int) {
	for j := len(f.offsets); j <= objNum; j++ {
		f.offsets = append(f.offsets, 0)
	}
	f.offsets[objNum] = f.finalOutputOffset()
	f.outPDFObjHeader(objNum)
}

func (f *Document) newPDFDictObject() {
	f.newobj()
	f.beginPDFDict()
}

func (f *Document) beginPDFDict() {
	f.out("<<")
}

func (f *Document) endPDFDict() {
	f.out(">>")
}

func (f *Document) endPDFObject() {
	f.out("endobj")
}

func (f *Document) beginPDFStream() {
	f.out("stream")
}

func (f *Document) endPDFStream() {
	f.out("endstream")
}

func (f *Document) putstream(b []byte) {
	if f.protect.encrypted {
		f.protect.rc4(uint32(f.n), &b)
	}
	f.beginPDFStream()
	f.outbytes(b)
	f.endPDFStream()
}
