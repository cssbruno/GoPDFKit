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

func appendPDFIndirectRef(dst []byte, value int) []byte {
	dst = appendPDFInt(dst, value)
	return append(dst, " 0 R"...)
}

func appendPDFXrefEntry(dst []byte, offset int) []byte {
	digits := pdfIntDigits(offset)
	for zeros := 10 - digits; zeros > 0; zeros-- {
		dst = append(dst, '0')
	}
	dst = appendPDFInt(dst, offset)
	return append(dst, " 00000 n "...)
}

func pdfIntDigits(value int) int {
	if value < 0 {
		return len(strconv.Itoa(value))
	}
	digits := 1
	for value >= 10 {
		value /= 10
		digits++
	}
	return digits
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

func appendPDFMoveTo(dst []byte, x, y float64, precision int) []byte {
	dst = appendPDFNumberSpace(dst, x, precision)
	dst = appendPDFNumberSpace(dst, y, precision)
	return append(dst, 'm')
}

func appendPDFLineTo(dst []byte, x, y float64, precision int) []byte {
	dst = appendPDFNumberSpace(dst, x, precision)
	dst = appendPDFNumberSpace(dst, y, precision)
	return append(dst, 'l')
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

func appendPDFMatrix(dst []byte, a, b, c, d, e, f float64) []byte {
	dst = appendPDFNumberSpace(dst, a, 5)
	dst = appendPDFNumberSpace(dst, b, 5)
	dst = appendPDFNumberSpace(dst, c, 5)
	dst = appendPDFNumberSpace(dst, d, 5)
	dst = appendPDFNumberSpace(dst, e, 5)
	dst = appendPDFNumberSpace(dst, f, 5)
	return append(dst, "cm"...)
}

func appendPDFScaleTranslateCM(dst []byte, w, h, x, y float64) []byte {
	dst = appendPDFNumberSpace(dst, w, 5)
	dst = append(dst, "0 0 "...)
	dst = appendPDFNumberSpace(dst, h, 5)
	dst = appendPDFNumberSpace(dst, x, 5)
	dst = appendPDFNumberSpace(dst, y, 5)
	return append(dst, "cm"...)
}

func appendPDFImageCM(dst []byte, w, h, x, y float64, imageID string) []byte {
	dst = append(dst, "q "...)
	dst = appendPDFScaleTranslateCM(dst, w, h, x, y)
	dst = append(dst, ' ')
	dst = append(dst, "/I"...)
	dst = append(dst, imageID...)
	return append(dst, " Do Q"...)
}

func appendPDFFontResourceRef(dst []byte, fontID string, objNum int) []byte {
	dst = append(dst, "/F"...)
	dst = append(dst, fontID...)
	dst = append(dst, ' ')
	return appendPDFIndirectRef(dst, objNum)
}

func appendPDFIntResourceRef(dst []byte, prefix string, id, objNum int) []byte {
	dst = append(dst, prefix...)
	dst = appendPDFInt(dst, id)
	dst = append(dst, ' ')
	return appendPDFIndirectRef(dst, objNum)
}

func appendPDFStringResourceRef(dst []byte, prefix, id string, objNum int) []byte {
	dst = append(dst, prefix...)
	dst = append(dst, id...)
	dst = append(dst, ' ')
	return appendPDFIndirectRef(dst, objNum)
}

func appendPDFRectArray(dst []byte, prefix string, x, y, w, h float64, precision int) []byte {
	dst = append(dst, prefix...)
	dst = appendPDFNumberSpace(dst, x, precision)
	dst = appendPDFNumberSpace(dst, y, precision)
	dst = appendPDFNumberSpace(dst, w, precision)
	dst = appendPDFNumber(dst, h, precision)
	return append(dst, ']')
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

// outPDFKeyInt writes a "<prefix><value><suffix>" line without fmt, using a
// stack scratch buffer. prefix/suffix are constant literals at every call site.
func (f *Document) outPDFKeyInt(prefix string, value int, suffix string) {
	var scratch [64]byte
	buf := append(scratch[:0], prefix...)
	buf = appendPDFInt(buf, value)
	buf = append(buf, suffix...)
	f.outbytes(buf)
}

func (f *Document) outPDFKeyIndirectRef(prefix string, value int) {
	var scratch [64]byte
	buf := append(scratch[:0], prefix...)
	buf = appendPDFIndirectRef(buf, value)
	f.outbytes(buf)
}

func (f *Document) outPDFKeyIndirectRefSuffix(prefix string, value int, suffix string) {
	var scratch [80]byte
	buf := append(scratch[:0], prefix...)
	buf = appendPDFIndirectRef(buf, value)
	buf = append(buf, suffix...)
	f.outbytes(buf)
}

func (f *Document) outPDFIntResourceRef(prefix string, id, objNum int) {
	var scratch [64]byte
	buf := appendPDFIntResourceRef(scratch[:0], prefix, id, objNum)
	f.outbytes(buf)
}

func (f *Document) outPDFStringResourceRef(prefix, id string, objNum int) {
	buf := make([]byte, 0, len(prefix)+len(id)+24)
	buf = appendPDFStringResourceRef(buf, prefix, id, objNum)
	f.outbytes(buf)
}

// outPDFMediaBox writes a "/MediaBox [0 0 wd ht]" line without fmt.
func (f *Document) outPDFMediaBox(wd, ht float64) {
	var scratch [48]byte
	buf := append(scratch[:0], "/MediaBox [0 0 "...)
	buf = appendPDFNumberSpace(buf, wd, 2)
	buf = appendPDFNumber(buf, ht, 2)
	buf = append(buf, ']')
	f.outbytes(buf)
}
