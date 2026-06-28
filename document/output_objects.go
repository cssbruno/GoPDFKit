// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

func (f *Document) newobj() {
	f.n++
	for j := len(f.offsets); j <= f.n; j++ {
		f.offsets = append(f.offsets, 0)
	}
	f.offsets[f.n] = f.buffer.Len()
	if f.hooks.OnOutputObject != nil {
		f.hooks.OnOutputObject(f.n, "object")
	}
	f.outPDFObjHeader(f.n)
}

func (f *Document) putstream(b []byte) {
	if f.protect.encrypted {
		f.protect.rc4(uint32(f.n), &b)
	}
	f.out("stream")
	f.outbytes(b)
	f.out("endstream")
}
