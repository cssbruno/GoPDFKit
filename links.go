// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package gopdfkit

// AddLink creates a new internal link and returns its identifier. An internal
// link is a clickable area that points to another place within the document.
// The identifier can then be passed to Cell, Write, ImageOptions, or Link. The
// destination is defined with SetLink.
func (f *Fpdf) AddLink() int {
	f.links = append(f.links, internalLink{})
	return len(f.links) - 1
}

// SetLink defines the page and position that link points to. See AddLink.
func (f *Fpdf) SetLink(link int, y float64, page int) {
	if !f.validLinkID(link) {
		f.SetErrorf("invalid link id: %d", link)
		return
	}
	if y == -1 {
		y = f.y
	}
	if page == -1 {
		page = f.page
	}
	if page <= 0 || page > f.page {
		f.SetErrorf("invalid link destination page: %d", page)
		return
	}
	if !finiteNumbers(y) {
		f.SetErrorf("invalid link destination position")
		return
	}
	f.links[link] = internalLink{page, y}
}

// newLink adds a new clickable link on the current page.
func (f *Fpdf) newLink(x, y, w, h float64, link int, linkStr string) {
	if link != 0 && !f.validLinkID(link) {
		f.SetErrorf("invalid link id: %d", link)
		return
	}
	if !finiteNumbers(x, y, w, h) {
		f.SetErrorf("invalid link rectangle")
		return
	}
	f.pageLinks[f.page] = append(f.pageLinks[f.page], pageLink{x * f.k, f.hPt - y*f.k, w * f.k, h * f.k, link, linkStr})
}

func (f *Fpdf) validLinkID(link int) bool {
	return link > 0 && link < len(f.links)
}

// Link puts a link on a rectangular area of the page. Text or image links are
// generally put via Cell, Write, or ImageOptions, but this method can be useful
// to define a clickable area inside an image. link is the value returned by
// AddLink.
func (f *Fpdf) Link(x, y, w, h float64, link int) {
	f.newLink(x, y, w, h, link, "")
}

// LinkString puts a link on a rectangular area of the page. Text or image
// links are generally put via Cell, Write, or ImageOptions, but this method can
// be useful to define a clickable area inside an image. linkStr is the target
// URL.
func (f *Fpdf) LinkString(x, y, w, h float64, linkStr string) {
	f.newLink(x, y, w, h, 0, linkStr)
}

// Bookmark sets a bookmark that will be displayed in a sidebar outline. txtStr
// is the title of the bookmark. level specifies the level of the bookmark in
// the outline; 0 is the top level, 1 is just below, and so on. y specifies the
// vertical position of the bookmark destination in the current page; -1
// indicates the current position.
func (f *Fpdf) Bookmark(txtStr string, level int, y float64) {
	if y == -1 {
		y = f.y
	}
	if !finiteNumbers(y) {
		f.SetErrorf("invalid bookmark position")
		return
	}
	if f.isCurrentUTF8 {
		txtStr = utf8toutf16(txtStr)
	}
	f.outlines = append(f.outlines, outlineEntry{text: txtStr, level: level, y: y, p: f.PageNo(), prev: -1, last: -1, next: -1, first: -1})
}
