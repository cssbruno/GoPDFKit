// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"fmt"
	"maps"
	"strings"
)

// GetPageSize returns the current page's width and height. This is the paper's
// size. To compute the size of the area being used, subtract the margins (see
// GetMargins()).
func (f *Document) GetPageSize() (width, height float64) {
	width = f.w
	height = f.h
	return
}

// GetPageWidth returns the current page width in the units established in
// New().
func (f *Document) GetPageWidth() float64 {
	return f.w
}

// GetPageHeight returns the current page height in the units established in
// New().
func (f *Document) GetPageHeight() float64 {
	return f.h
}

// GetMargins returns the left, top, right, and bottom margins. The first three
// are set with the SetMargins() method. The bottom margin is set with the
// SetAutoPageBreak() method.
func (f *Document) GetMargins() (left, top, right, bottom float64) {
	left = f.lMargin
	top = f.tMargin
	right = f.rMargin
	bottom = f.bMargin
	return
}

// SetMargins defines the left, top and right margins. By default, they equal 1
// cm. Call this method to change them. If the value of the right margin is
// less than zero, it is set to the same as the left margin.
func (f *Document) SetMargins(left, top, right float64) {
	f.lMargin = left
	f.tMargin = top
	if right < 0 {
		right = left
	}
	f.rMargin = right
}

// SetLeftMargin defines the left margin. The method can be called before
// creating the first page. If the current abscissa gets out of page, it is
// brought back to the margin.
func (f *Document) SetLeftMargin(margin float64) {
	f.lMargin = margin
	if f.page > 0 && f.x < margin {
		f.x = margin
	}
}

// GetCellMargin returns the cell margin. This is the amount of space before
// and after the text within a cell that's left blank, and is in units passed
// to New(). It defaults to 1mm.
func (f *Document) GetCellMargin() float64 {
	return f.cMargin
}

// SetCellMargin sets the cell margin. This is the amount of space before and
// after the text within a cell that's left blank, and is in units passed to
// New().
func (f *Document) SetCellMargin(margin float64) {
	f.cMargin = margin
}

// SetPage sets the current page to that of a valid page in the PDF document.
// pageNum is one-based. The SetPage() example demonstrates this method.
func (f *Document) SetPage(pageNum int) {
	_ = f.SetPageE(pageNum)
}

// SetPageE sets the current page and reports invalid page numbers directly.
func (f *Document) SetPageE(pageNum int) error {
	if pageNum <= 0 || pageNum >= len(f.pages) {
		f.SetErrorf("invalid page number: %d", pageNum)
		return f.err
	}
	f.page = pageNum
	return nil
}

// PageCount returns the number of pages currently in the document. Since page
// numbers in gopdfkit are one-based, the page count is the same as the page
// number of the current last page.
func (f *Document) PageCount() int {
	return len(f.pages) - 1
}

// SetHeaderFuncMode sets the function that lets the application render the
// page header. See SetHeaderFunc() for more details. The value for homeMode
// should be set to true to have the current position set to the left and top
// margin after the header function is called.
func (f *Document) SetHeaderFuncMode(fnc func(), homeMode bool) {
	f.headerFnc = fnc
	f.headerHomeMode = homeMode
}

// SetHeaderFunc sets the function that lets the application render the page
// header. The specified function is automatically called by AddPage() and
// should not be called directly by the application. The implementation in Document
// is empty, so you have to provide an appropriate function if you want page
// headers. fnc will typically be a closure that has access to the Document
// instance and other document generation variables.
//
// A header is a convenient place to put background content that repeats on
// each page such as a watermark. When this is done, remember to reset the X
// and Y values so the normal content begins where expected. Including a
// watermark on each page is demonstrated in the example for TransformRotate.
//
// This method is demonstrated in the example for AddPage().
func (f *Document) SetHeaderFunc(fnc func()) {
	f.headerFnc = fnc
}

// SetFooterFunc sets the function that lets the application render the page
// footer. The specified function is automatically called by AddPage() and
// Close() and should not be called directly by the application. The
// implementation in Document is empty, so you have to provide an appropriate
// function if you want page footers. fnc will typically be a closure that has
// access to the Document instance and other document generation variables. See
// SetFooterFuncLpi for a similar function that passes a last page indicator.
//
// This method is demonstrated in the example for AddPage().
func (f *Document) SetFooterFunc(fnc func()) {
	f.footerFnc = fnc
	f.footerFncLpi = nil
}

// SetFooterFuncLpi sets the function that lets the application render the page
// footer. The specified function is automatically called by AddPage() and
// Close() and should not be called directly by the application. It is passed a
// boolean that is true if the last page of the document is being rendered. The
// implementation in Document is empty, so you have to provide an appropriate
// function if you want page footers. fnc will typically be a closure that has
// access to the Document instance and other document generation variables.
func (f *Document) SetFooterFuncLpi(fnc func(lastPage bool)) {
	f.footerFncLpi = fnc
	f.footerFnc = nil
}

// SetTopMargin defines the top margin. The method can be called before
// creating the first page.
func (f *Document) SetTopMargin(margin float64) {
	f.tMargin = margin
}

// SetRightMargin defines the right margin. The method can be called before
// creating the first page.
func (f *Document) SetRightMargin(margin float64) {
	f.rMargin = margin
}

// GetAutoPageBreak returns true if automatic pages breaks are enabled, false
// otherwise. This is followed by the triggering limit from the bottom of the
// page. This value applies only if automatic page breaks are enabled.
func (f *Document) GetAutoPageBreak() (auto bool, margin float64) {
	auto = f.autoPageBreak
	margin = f.bMargin
	return
}

// SetAutoPageBreak enables or disables the automatic page breaking mode. When
// enabling, the second parameter is the distance from the bottom of the page
// that defines the triggering limit. By default, the mode is on and the margin
// is 2 cm.
func (f *Document) SetAutoPageBreak(auto bool, margin float64) {
	f.autoPageBreak = auto
	f.bMargin = margin
	f.pageBreakTrigger = f.h - margin
}

// PageSize returns the width and height of the specified page in the units
// established in New(). These return values are followed by the unit of
// measure itself. If pageNum is zero or otherwise out of bounds, it returns
// the default page size, that is, the size of the page that would be added by
// AddPage().
func (f *Document) PageSize(pageNum int) (wd, ht float64, unitStr string) {
	sz, ok := f.pageSizes[pageNum]
	if ok {
		sz.Wd, sz.Ht = sz.Wd/f.k, sz.Ht/f.k
	} else {
		sz = f.defPageSize
	}
	return sz.Wd, sz.Ht, f.unitStr
}

// AddPageFormat adds a new page with non-default orientation or size. See
// AddPage() for more details.
//
// See New() for a description of orientationStr.
//
// size specifies the size of the new page in the units established in New().
//
// The PageSize() example demonstrates this method.
func (f *Document) AddPageFormat(orientationStr string, size Size) {
	f.addPageFormatRotation(orientationStr, size, 0)
}

// AddPageFormatRotation adds a new page with non-default orientation, size, or
// page dictionary rotation. The rotation must be a multiple of 90 degrees.
func (f *Document) AddPageFormatRotation(orientationStr string, size Size, rotation int) {
	f.addPageFormatRotation(orientationStr, size, rotation)
}

func (f *Document) addPageFormatRotation(orientationStr string, size Size, rotation int) {
	if f.err != nil {
		return
	}
	if rotation%90 != 0 {
		f.err = fmt.Errorf("incorrect rotation value: %d", rotation)
		return
	}
	orientationStr, err := f.normalizePageOrientation(orientationStr)
	if err != nil {
		f.err = err
		return
	}
	if err := validatePageSize(size); err != nil {
		f.err = err
		return
	}
	if err := f.checkPageLimitForAdd(); err != nil {
		return
	}
	if f.page != len(f.pages)-1 {
		f.page = len(f.pages) - 1
	}
	if f.state == documentStateUnopened {
		f.open()
	}
	familyStr := f.fontFamily
	style := f.fontStyle
	if f.underline {
		style += "U"
	}
	if f.strikeout {
		style += "S"
	}
	fontsize := f.fontSizePt
	lw := f.lineWidth
	dc := f.color.draw
	fc := f.color.fill
	tc := f.color.text
	cf := f.colorFlag
	if f.page > 0 {
		f.inFooter = true
		if f.footerFnc != nil {
			f.footerFnc()
		} else if f.footerFncLpi != nil {
			f.footerFncLpi(false)
		}
		f.inFooter = false
		f.endpage()
	}
	f.beginpage(orientationStr, size, rotation)
	f.outf("%d J", f.capStyle)
	f.outf("%d j", f.joinStyle)
	f.lineWidth = lw
	f.outf("%.2f w", lw*f.k)
	if len(f.dashArray) > 0 {
		f.outputDashPattern()
	}
	if familyStr != "" {
		f.SetFont(familyStr, style, fontsize)
		if f.err != nil {
			return
		}
	}
	f.color.draw = dc
	if dc.str != "0 G" {
		f.out(dc.str)
	}
	f.color.fill = fc
	if fc.str != "0 g" {
		f.out(fc.str)
	}
	f.color.text = tc
	f.colorFlag = cf
	if f.headerFnc != nil {
		f.inHeader = true
		f.headerFnc()
		f.inHeader = false
		if f.headerHomeMode {
			f.SetHomeXY()
		}
	}
	if f.lineWidth != lw {
		f.lineWidth = lw
		f.outf("%.2f w", lw*f.k)
	}
	if familyStr != "" {
		f.SetFont(familyStr, style, fontsize)
		if f.err != nil {
			return
		}
	}
	if f.color.draw.str != dc.str {
		f.color.draw = dc
		f.out(dc.str)
	}
	if f.color.fill.str != fc.str {
		f.color.fill = fc
		f.out(fc.str)
	}
	f.color.text = tc
	f.colorFlag = cf
}

// AddPage adds a new page to the document. If a page is already present, the
// Footer() method is called first to output the footer. Then the page is
// added, the current position set to the top-left corner according to the left
// and top margins, and Header() is called to display the header.
//
// The font which was set before calling is automatically restored. There is no
// need to call SetFont() again if you want to continue with the same font. The
// same is true for colors and line width.
//
// The origin of the coordinate system is at the top-left corner and increasing
// ordinates go downwards.
//
// See AddPageFormat() for a version of this method that allows the page size
// and orientation to be different than the default.
func (f *Document) AddPage() {
	if f.err != nil {
		return
	}
	f.addPageFormatRotation(f.defOrientation, f.defPageSize, 0)
}

// AddPageRotation adds a new page with the default orientation and size and
// sets its page dictionary rotation. The rotation must be a multiple of 90
// degrees.
func (f *Document) AddPageRotation(rotation int) {
	if f.err != nil {
		return
	}
	f.addPageFormatRotation(f.defOrientation, f.defPageSize, rotation)
}

// PageNo returns the current page number.
//
// See the example for AddPage() for a demonstration of this method.
func (f *Document) PageNo() int {
	return f.page
}

// GetConversionRatio returns the conversion ratio based on the unit given when
// creating the PDF.
func (f *Document) GetConversionRatio() float64 {
	return f.k
}

// GetXY returns the abscissa and ordinate of the current position.
//
// Note: the value returned for the abscissa will be affected by the current
// cell margin. To account for this, you may need to either add the value
// returned by GetCellMargin() to it or call SetCellMargin(0) to remove the
// cell margin.
func (f *Document) GetXY() (float64, float64) {
	return f.x, f.y
}

// GetX returns the abscissa of the current position.
//
// Note: the value returned will be affected by the current cell margin. To
// account for this, you may need to either add the value returned by
// GetCellMargin() to it or call SetCellMargin(0) to remove the cell margin.
func (f *Document) GetX() float64 {
	return f.x
}

// SetX defines the abscissa of the current position. If the passed value is
// negative, it is relative to the right of the page.
func (f *Document) SetX(x float64) {
	if x >= 0 {
		f.x = x
	} else {
		f.x = f.w + x
	}
}

// GetY returns the ordinate of the current position.
func (f *Document) GetY() float64 {
	return f.y
}

// SetY moves the current abscissa back to the left margin and sets the
// ordinate. If the passed value is negative, it is relative to the bottom of
// the page.
func (f *Document) SetY(y float64) {
	f.SetYWithResetX(y, true)
}

// SetYWithResetX sets the ordinate and optionally moves the current abscissa
// back to the left margin. This is the Go-friendly equivalent of FPDF 1.8+'s
// SetY($y, $resetX) parameter.
func (f *Document) SetYWithResetX(y float64, resetX bool) {
	if y >= 0 {
		f.y = y
	} else {
		f.y = f.h + y
	}
	if resetX {
		f.x = f.lMargin
	}
}

// SetHomeXY is a convenience method that sets the current position to the left
// and top margins.
func (f *Document) SetHomeXY() {
	f.SetY(f.tMargin)
	f.SetX(f.lMargin)
}

// SetXY defines the abscissa and ordinate of the current position. If the
// passed values are negative, they are relative respectively to the right and
// bottom of the page.
func (f *Document) SetXY(x, y float64) {
	f.SetX(x)
	f.SetYWithResetX(y, false)
}

func (f *Document) getpagesizestr(sizeStr string) (size Size) {
	if f.err != nil {
		return
	}
	sizeStr = strings.ToLower(sizeStr)
	var ok bool
	size, ok = f.stdPageSizes[sizeStr]
	if ok {
		size.Wd /= f.k
		size.Ht /= f.k
	} else {
		f.err = fmt.Errorf("unknown page size %s", sizeStr)
	}
	return
}

func (f *Document) normalizePageOrientation(orientationStr string) (string, error) {
	if strings.TrimSpace(orientationStr) == "" {
		return f.defOrientation, nil
	}
	switch strings.ToLower(strings.TrimSpace(orientationStr)) {
	case "p", "portrait":
		return "P", nil
	case "l", "landscape":
		return "L", nil
	default:
		return "", fmt.Errorf("incorrect orientation: %s", orientationStr)
	}
}

func validatePageSize(size Size) error {
	if !finiteNumbers(size.Wd, size.Ht) || size.Wd <= 0 || size.Ht <= 0 {
		return fmt.Errorf("%w: %.2f x %.2f", ErrInvalidPageSize, size.Wd, size.Ht)
	}
	return nil
}

// GetPageSizeStr returns the Size for the given sizeStr (that is A4, A3, etc..)
func (f *Document) GetPageSizeStr(sizeStr string) (size Size) {
	return f.getpagesizestr(sizeStr)
}

func (f *Document) beginpage(orientationStr string, size Size, rotation int) {
	if f.err != nil {
		return
	}
	orientationStr, err := f.normalizePageOrientation(orientationStr)
	if err != nil {
		f.err = err
		return
	}
	if err := validatePageSize(size); err != nil {
		f.err = err
		return
	}
	if err := f.checkPageLimitForAdd(); err != nil {
		return
	}
	f.page++
	f.pageBoxes[f.page] = make(map[string]PageBox)
	maps.Copy(f.pageBoxes[f.page], f.defPageBoxes)
	f.pages = append(f.pages, bytes.NewBufferString(""))
	f.aliasPages = append(f.aliasPages, false)
	f.pageLinks = append(f.pageLinks, make([]pageLink, 0))
	f.pageAttachments = append(f.pageAttachments, []annotationAttach{})
	f.state = documentStatePageOpen
	f.taggedBeginPage(f.page)
	f.x = f.lMargin
	f.y = f.tMargin
	f.fontFamily = ""
	if orientationStr != f.curOrientation || size.Wd != f.curPageSize.Wd || size.Ht != f.curPageSize.Ht {
		if orientationStr == "P" {
			f.w = size.Wd
			f.h = size.Ht
		} else {
			f.w = size.Ht
			f.h = size.Wd
		}
		f.wPt = f.w * f.k
		f.hPt = f.h * f.k
		f.pageBreakTrigger = f.h - f.bMargin
		f.curOrientation = orientationStr
		f.curPageSize = size
	}
	if orientationStr != f.defOrientation || size.Wd != f.defPageSize.Wd || size.Ht != f.defPageSize.Ht {
		f.pageSizes[f.page] = Size{f.wPt, f.hPt}
	}
	if rotation != 0 {
		f.pageRotations[f.page] = rotation
	}
	f.curRotation = rotation
}

func (f *Document) endpage() {
	f.EndLayer()
	f.state = documentStateOpen
}
