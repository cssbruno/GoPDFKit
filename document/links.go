// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"fmt"
	"net/url"
	"strings"
)

func allowedExternalLinkScheme(scheme string) bool {
	switch strings.ToLower(scheme) {
	case "http", "https", "mailto":
		return true
	default:
		return false
	}
}

func checkedExternalLinkTarget(target string) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", nil
	}
	if strings.HasPrefix(target, "#") {
		return "", fmt.Errorf("fragment links are not supported: %s", target)
	}
	u, err := url.Parse(target)
	if err != nil {
		return "", fmt.Errorf("invalid link target: %w", err)
	}
	if !allowedExternalLinkScheme(u.Scheme) {
		return "", fmt.Errorf("unsupported link scheme: %s", u.Scheme)
	}
	return target, nil
}

// AddLink creates a new internal link and returns its identifier. An internal
// link is a clickable area that points to another place within the document.
// The identifier can then be passed to Cell, Write, ImageOptions, or Link. The
// destination is defined with SetLink.
func (f *Document) AddLink() int {
	f.links = append(f.links, internalLink{})
	return len(f.links) - 1
}

// SetLink defines the page and position that link points to. See AddLink.
func (f *Document) SetLink(link int, y float64, page int) {
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
func (f *Document) newLink(x, y, w, h float64, link int, linkStr string) {
	if f.page <= 0 {
		f.SetErrorf("link requires an active page")
		return
	}
	if link != 0 && !f.validLinkID(link) {
		f.SetErrorf("invalid link id: %d", link)
		return
	}
	if link == 0 {
		checked, err := checkedExternalLinkTarget(linkStr)
		if err != nil {
			f.SetError(err)
			return
		}
		if checked == "" {
			return
		}
		linkStr = checked
	}
	if !finiteNumbers(x, y, w, h) {
		f.SetErrorf("invalid link rectangle")
		return
	}
	elem, structParent := f.taggedLinkAnnotation()
	f.pageLinks[f.page] = append(f.pageLinks[f.page], pageLink{
		x:            x * f.k,
		y:            f.hPt - y*f.k,
		wd:           w * f.k,
		ht:           h * f.k,
		link:         link,
		linkStr:      linkStr,
		structParent: structParent,
		structElem:   elem,
	})
}

func (f *Document) validLinkID(link int) bool {
	return link > 0 && link < len(f.links)
}

// Link puts a link on a rectangular area of the page. Text or image links are
// generally put via Cell, Write, or ImageOptions, but this method can be useful
// to define a clickable area inside an image. link is the value returned by
// AddLink.
func (f *Document) Link(x, y, w, h float64, link int) {
	f.newLink(x, y, w, h, link, "")
}

// LinkString puts a link on a rectangular area of the page. Text or image
// links are generally put via Cell, Write, or ImageOptions, but this method can
// be useful to define a clickable area inside an image. linkStr is the target
// URL.
func (f *Document) LinkString(x, y, w, h float64, linkStr string) {
	f.newLink(x, y, w, h, 0, linkStr)
}

// Bookmark sets a bookmark that will be displayed in a sidebar outline. txtStr
// is the title of the bookmark. level specifies the level of the bookmark in
// the outline; 0 is the top level, 1 is just below, and so on. y specifies the
// vertical position of the bookmark destination in the current page; -1
// indicates the current position.
func (f *Document) Bookmark(txtStr string, level int, y float64) {
	if f.err != nil {
		return
	}
	if f.page <= 0 {
		f.SetErrorf("bookmark requires an active page")
		return
	}
	if level < 0 {
		f.SetErrorf("invalid bookmark level: %d", level)
		return
	}
	if len(f.outlines) == 0 && level != 0 {
		f.SetErrorf("first bookmark level must be 0")
		return
	}
	if len(f.outlines) > 0 && level > f.outlines[len(f.outlines)-1].level+1 {
		f.SetErrorf("bookmark level cannot jump from %d to %d", f.outlines[len(f.outlines)-1].level, level)
		return
	}
	if y == -1 {
		y = f.y
	}
	if !finiteNumbers(y) {
		f.SetErrorf("invalid bookmark position")
		return
	}
	f.outlines = append(f.outlines, outlineEntry{text: txtStr, level: level, y: y, p: f.PageNo(), utf8: f.isCurrentUTF8, prev: -1, last: -1, next: -1, first: -1})
}
