// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"hash"
)

type fontBoxType struct {
	Xmin, Ymin, Xmax, Ymax int
}

// Font flags for FontDescriptor.Flags as defined in the PDF specification.
const (
	// FontFlagFixedPitch is set if all glyphs have the same width (as
	// opposed to proportional or variable-pitch fonts, which have
	// different widths).
	FontFlagFixedPitch = 1 << 0
	// FontFlagSerif is set if glyphs have serifs, which are short
	// strokes drawn at an angle on the top and bottom of glyph stems.
	// (Sans serif fonts do not have serifs.)
	FontFlagSerif = 1 << 1
	// FontFlagSymbolic is set if the font contains glyphs outside the
	// Adobe standard Latin character set. This flag and the
	// Nonsymbolic flag shall not both be set or both be clear.
	FontFlagSymbolic = 1 << 2
	// FontFlagScript is set if glyphs resemble cursive handwriting.
	FontFlagScript = 1 << 3
	// FontFlagNonsymbolic is set if the font uses the Adobe standard
	// Latin character set or a subset of it.
	FontFlagNonsymbolic = 1 << 5
	// FontFlagItalic is set if glyphs have dominant vertical strokes
	// that are slanted.
	FontFlagItalic = 1 << 6
	// FontFlagAllCap is set if the font contains no lowercase letters;
	// typically used for display purposes, such as for titles or
	// headlines.
	FontFlagAllCap = 1 << 16
	// SmallCap is set if the font contains both uppercase and lowercase
	// letters. The uppercase letters are similar to those in the
	// regular version of the same typeface family. The glyphs for the
	// lowercase letters have the same shapes as the corresponding
	// uppercase letters, but they are sized and their proportions
	// adjusted so that they have the same size and stroke weight as
	// lowercase glyphs in the same typeface family.
	SmallCap = 1 << 18
	// ForceBold determines whether bold glyphs shall be painted with
	// extra pixels even at very small text sizes by a conforming
	// reader. If the ForceBold flag is set, features of bold glyphs
	// may be thickened at small text sizes.
	ForceBold = 1 << 18
)

// FontDescriptor specifies metrics and other attributes of a font, as distinct
// from the metrics of individual glyphs, as defined in the PDF specification.
type FontDescriptor struct {
	// The maximum height above the baseline reached by glyphs in this
	// font (for example for "S"). The height of glyphs for accented
	// characters shall be excluded.
	Ascent int
	// The maximum depth below the baseline reached by glyphs in this
	// font. The value shall be a negative number.
	Descent int
	// The vertical coordinate of the top of flat capital letters,
	// measured from the baseline (for example "H").
	CapHeight int
	// A collection of flags defining various characteristics of the
	// font. (See the FontFlag* constants.)
	Flags int
	// A rectangle, expressed in the glyph coordinate system, that
	// shall specify the font bounding box. This should be the smallest
	// rectangle enclosing the shape that would result if all of the
	// glyphs of the font were placed with their origins coincident
	// and then filled.
	FontBBox fontBoxType
	// The angle, expressed in degrees counterclockwise from the
	// vertical, of the dominant vertical strokes of the font. (The
	// 9-o’clock position is 90 degrees, and the 3-o’clock position
	// is –90 degrees.) The value shall be negative for fonts that
	// slope to the right, as almost all italic fonts do.
	ItalicAngle int
	// The thickness, measured horizontally, of the dominant vertical
	// stems of glyphs in the font.
	StemV int
	// The width to use for character codes whose widths are not
	// specified in a font dictionary’s Widths array. This shall have
	// a predictable effect only if all such codes map to glyphs whose
	// actual widths are the same as the value of the MissingWidth
	// entry. (Default value: 0.)
	MissingWidth int
}

type fontDefinition struct {
	Tp           string         // "Core", "TrueType", ...
	Name         string         // "Courier-Bold", ...
	Desc         FontDescriptor // Font descriptor
	Up           int            // Underline position
	Ut           int            // Underline thickness
	Cw           []int          // Character width by ordinal
	Enc          string         // "cp1252", ...
	Diff         string         // Differences from reference encoding
	File         string         // "Redressed.z"
	Size1, Size2 int            // Type1 values
	OriginalSize int            // Size of uncompressed font file
	N            int            // Set by font loader
	DiffN        int            // Position of diff in app array, set by font loader
	i            string         // 1-based position in font list, set by font loader, not this program
	utf8File     *utf8FontFile  // UTF-8 font
	usedRunes    map[int]int    // Array of used runes
}

// generateFontID generates a font ID from the font definition.
func generateFontID(fdt fontDefinition) (string, error) {
	// File can differ when the same font is generated in a different instance.
	fdt.File = ""
	h := sha1.New()
	w := hashJSONNoFinalNewline{hash: h}
	if err := json.NewEncoder(&w).Encode(&fdt); err != nil {
		return "", err
	}
	w.Flush()
	return hex.EncodeToString(h.Sum(nil)), nil
}

type hashJSONNoFinalNewline struct {
	hash    hash.Hash
	pending byte
	hasByte bool
}

func (w *hashJSONNoFinalNewline) Write(p []byte) (int, error) {
	written := len(p)
	if len(p) == 0 {
		return written, nil
	}
	if w.hasByte {
		_, _ = w.hash.Write([]byte{w.pending})
		w.hasByte = false
	}
	if len(p) > 1 {
		_, _ = w.hash.Write(p[:len(p)-1])
	}
	w.pending = p[len(p)-1]
	w.hasByte = true
	return written, nil
}

func (w *hashJSONNoFinalNewline) Flush() {
	if w.hasByte && w.pending != '\n' {
		_, _ = w.hash.Write([]byte{w.pending})
	}
	w.hasByte = false
}
