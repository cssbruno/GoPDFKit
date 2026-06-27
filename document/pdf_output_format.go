// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import "strconv"

func appendPDFNumber(dst []byte, value float64, precision int) []byte {
	return strconv.AppendFloat(dst, value, 'f', precision, 64)
}

func appendPDFNumberSpace(dst []byte, value float64, precision int) []byte {
	dst = appendPDFNumber(dst, value, precision)
	return append(dst, ' ')
}

func appendPDFInt(dst []byte, value int) []byte {
	return strconv.AppendInt(dst, int64(value), 10)
}

func ensurePDFBuffer(dst []byte, capacity int) []byte {
	if dst != nil {
		return dst
	}
	return make([]byte, 0, capacity)
}

func appendPDFRectPaint(dst []byte, x, y, w, h float64, op string, trailingSpace bool) []byte {
	dst = appendPDFNumberSpace(dst, x, 2)
	dst = appendPDFNumberSpace(dst, y, 2)
	dst = appendPDFNumberSpace(dst, w, 2)
	dst = appendPDFNumberSpace(dst, h, 2)
	dst = append(dst, "re "...)
	dst = append(dst, op...)
	if trailingSpace {
		dst = append(dst, ' ')
	}
	return dst
}

func appendPDFLine(dst []byte, x1, y1, x2, y2 float64, precision int, trailingSpace bool) []byte {
	dst = appendPDFNumberSpace(dst, x1, precision)
	dst = appendPDFNumberSpace(dst, y1, precision)
	dst = append(dst, "m "...)
	dst = appendPDFNumberSpace(dst, x2, precision)
	dst = appendPDFNumberSpace(dst, y2, precision)
	dst = append(dst, "l S"...)
	if trailingSpace {
		dst = append(dst, ' ')
	}
	return dst
}

func appendPDFCubicCurve(dst []byte, cx0, cy0, cx1, cy1, x, y float64) []byte {
	dst = appendPDFNumberSpace(dst, cx0, 5)
	dst = appendPDFNumberSpace(dst, cy0, 5)
	dst = appendPDFNumberSpace(dst, cx1, 5)
	dst = appendPDFNumberSpace(dst, cy1, 5)
	dst = appendPDFNumberSpace(dst, x, 5)
	dst = appendPDFNumberSpace(dst, y, 5)
	return append(dst, 'c')
}

func appendPDFFontSelect(dst []byte, fontID string, size float64) []byte {
	dst = append(dst, "BT /F"...)
	dst = append(dst, fontID...)
	dst = append(dst, ' ')
	dst = appendPDFNumber(dst, size, 2)
	return append(dst, " Tf ET"...)
}

func (f *Document) outPDFFontSelect() {
	buf := make([]byte, 0, len(f.currentFont.i)+20)
	buf = appendPDFFontSelect(buf, f.currentFont.i, f.fontSizePt)
	f.outbytes(buf)
}

func (f *Document) outPDFLineWidth(width float64) {
	var scratch [32]byte
	buf := appendPDFNumber(scratch[:0], width, 2)
	buf = append(buf, " w"...)
	f.outbytes(buf)
}

func (f *Document) outPDFIntOperator(value int, operator byte) {
	var scratch [24]byte
	buf := appendPDFInt(scratch[:0], value)
	buf = append(buf, ' ', operator)
	f.outbytes(buf)
}

func (f *Document) outPDFObjHeader(n int) {
	var scratch [32]byte
	buf := appendPDFInt(scratch[:0], n)
	buf = append(buf, " 0 obj"...)
	f.outbytes(buf)
}

func (f *Document) outPDFXrefRange(count int) {
	var scratch [32]byte
	buf := append(scratch[:0], '0', ' ')
	buf = appendPDFInt(buf, count)
	f.outbytes(buf)
}

func (f *Document) outPDFXrefOffset(offset int) {
	var scratch [32]byte
	buf := appendPDFPaddedInt(scratch[:0], offset, 10)
	buf = append(buf, " 00000 n "...)
	f.outbytes(buf)
}

func (f *Document) outPDFIntLine(value int) {
	var scratch [24]byte
	f.outbytes(appendPDFInt(scratch[:0], value))
}

func appendPDFPaddedInt(dst []byte, value, width int) []byte {
	var scratch [32]byte
	raw := appendPDFInt(scratch[:0], value)
	for i := len(raw); i < width; i++ {
		dst = append(dst, '0')
	}
	return append(dst, raw...)
}
