/****************************************************************************
 * Software: GoPDFKit                                                         *
 * License:  MIT License                                                    *
 *                                                                          *
 * Copyright (c) 2026 cssBruno                                              *
 ****************************************************************************/

package gopdfkit

import (
	"fmt"
	"strings"
)

// SetPageBoxRec sets the page box for the current page, and any following
// pages. Allowable types are trim, trimbox, crop, cropbox, bleed, bleedbox,
// art and artbox box types are case insensitive. See SetPageBox() for a method
// that specifies the coordinates and extent of the page box individually.

func (f *Fpdf) SetPageBoxRec(t string, pb PageBox) {
	if !finiteNumbers(pb.X, pb.Y, pb.Wd, pb.Ht) {
		f.err = fmt.Errorf("invalid page box coordinates")
		return
	}
	switch strings.ToLower(t) {
	case "trim":
		fallthrough
	case "trimbox":
		t = "TrimBox"
	case "crop":
		fallthrough
	case "cropbox":
		t = "CropBox"
	case "bleed":
		fallthrough
	case "bleedbox":
		t = "BleedBox"
	case "art":
		fallthrough
	case "artbox":
		t = "ArtBox"
	default:
		f.err = fmt.Errorf("%s is not a valid page box type", t)
		return
	}
	pb.X = pb.X * f.k
	pb.Y = pb.Y * f.k
	pb.Wd = (pb.Wd * f.k) + pb.X
	pb.Ht = (pb.Ht * f.k) + pb.Y
	if f.page > 0 {
		f.pageBoxes[f.page][t] = pb
	}
	f.defPageBoxes[t] = pb
}

// SetPageBox sets the page box for the current page, and any following pages.
// Allowable types are trim, trimbox, crop, cropbox, bleed, bleedbox, art and
// artbox box types are case insensitive.

func (f *Fpdf) SetPageBox(t string, x, y, wd, ht float64) {
	f.SetPageBoxRec(t, PageBox{Size{Wd: wd, Ht: ht}, Point{X: x, Y: y}})
}
