// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

// Command compliance-check performs local structural checks for generated
// compliance fixtures. It is not a standards validator; external PDF/A,
// PDF/UA, and Arlington tools remain authoritative for conformance.
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type tokenCheck struct {
	token string
	why   string
}

func main() {
	paths, err := expandArgs(os.Args[1:])
	if err != nil {
		exitErr(err)
	}
	if len(paths) == 0 {
		exitErr(errors.New("usage: compliance-check <pdf-or-directory> [...]"))
	}
	for _, path := range paths {
		if err := checkPDF(path); err != nil {
			exitErr(err)
		}
		fmt.Printf("local-check: ok %s\n", path)
	}
}

func expandArgs(args []string) ([]string, error) {
	var paths []string
	for _, arg := range args {
		info, err := os.Stat(arg) // #nosec G703 -- this CLI intentionally inspects paths supplied by its operator.
		if err != nil {
			return nil, err
		}
		if info.IsDir() {
			matches, err := filepath.Glob(filepath.Join(arg, "*.pdf"))
			if err != nil {
				return nil, err
			}
			paths = append(paths, matches...)
			continue
		}
		paths = append(paths, arg)
	}
	return paths, nil
}

func checkPDF(path string) error {
	data, err := os.ReadFile(path) // #nosec G304,G703 -- path is an explicit compliance-check CLI argument.
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	text := string(data)
	base := filepath.Base(path)
	if strings.Contains(base, "pdfua2") {
		return checkPDFUA2(path, text)
	}
	if strings.Contains(base, "pdfa4") {
		return checkPDFA4(path, text)
	}
	return checkCommon(path, text)
}

func checkCommon(path, text string) error {
	return requireTokens(path, text, []tokenCheck{
		{token: "%PDF-2.0", why: "PDF 2.0 header"},
		{token: "/Metadata", why: "catalog metadata reference"},
	})
}

// #nosec G101 -- These are PDF syntax markers checked in generated fixtures, not credentials.
func checkPDFA4(path, text string) error {
	checks := []tokenCheck{
		{token: "%PDF-2.0", why: "PDF 2.0 header"},
		{token: "/Metadata", why: "catalog metadata reference"},
		{token: "<pdfaid:part>4</pdfaid:part>", why: "PDF/A-4 XMP identifier"},
		{token: "/OutputIntents", why: "PDF/A output intent"},
		{token: "/DestOutputProfile", why: "embedded ICC profile reference"},
	}
	return requireTokens(path, text, checks)
}

// #nosec G101 -- These are PDF syntax markers checked in generated fixtures, not credentials.
func checkPDFUA2(path, text string) error {
	checks := []tokenCheck{
		{token: "%PDF-2.0", why: "PDF 2.0 header"},
		{token: "/Metadata", why: "catalog metadata reference"},
		{token: "<pdfuaid:part>2</pdfuaid:part>", why: "PDF/UA-2 XMP identifier"},
		{token: "<pdfuaid:rev>2024</pdfuaid:rev>", why: "PDF/UA-2 revision identifier"},
		{token: "<gopdfkit:ArlingtonValidationRequired>True</gopdfkit:ArlingtonValidationRequired>", why: "Arlington validation marker"},
		{token: "/MarkInfo << /Marked true >>", why: "marked catalog flag"},
		{token: "/Lang", why: "document language"},
		{token: "/ViewerPreferences << /DisplayDocTitle true >>", why: "display document title preference"},
		{token: "/StructTreeRoot", why: "structure tree root reference"},
		{token: "/Type /StructTreeRoot", why: "structure tree root object"},
		{token: "/ParentTree", why: "parent tree reference"},
		{token: "/StructParents", why: "page or annotation structure parent index"},
		{token: "/Type /StructElem", why: "structure element objects"},
		{token: "/S /Document", why: "document structure element"},
		{token: "/S /H1", why: "heading structure role"},
		{token: "/S /P", why: "paragraph structure role"},
		{token: "/S /Link", why: "link structure role"},
		{token: "/S /Figure", why: "figure structure role"},
		{token: "/S /L", why: "list structure role"},
		{token: "/S /LI", why: "list item structure role"},
		{token: "/S /Lbl", why: "list label structure role"},
		{token: "/S /LBody", why: "list body structure role"},
		{token: "/S /Table", why: "table structure role"},
		{token: "/S /Caption", why: "table caption structure role"},
		{token: "/S /TR", why: "table row structure role"},
		{token: "/S /TH", why: "table header cell structure role"},
		{token: "/S /TD", why: "table data cell structure role"},
		{token: "/A << /O /Table /Scope /Column", why: "table column-header scope attribute"},
		{token: "/A << /O /Table /Scope /Row /RowSpan 2", why: "table row-header rowspan attributes"},
		{token: "/A << /O /Table /ColSpan 2", why: "table colspan attribute"},
		{token: "/MCID ", why: "marked content IDs"},
		{token: "/Type /OBJR /Obj", why: "link annotation object reference"},
		{token: "/Type /Annot /Subtype /Link", why: "indirect link annotation"},
		{token: "/Alt (", why: "figure alternate text"},
		{token: "/Artifact BMC", why: "artifact marked content"},
	}
	if err := requireTokens(path, text, checks); err != nil {
		return err
	}
	for _, check := range []struct {
		token string
		min   int
		why   string
	}{
		{token: "/S /L", min: 3, why: "top-level and nested table-cell list structures"},
		{token: "/S /LI", min: 4, why: "top-level and nested table-cell list items"},
		{token: "/S /Lbl", min: 4, why: "top-level and nested table-cell list labels"},
		{token: "/S /LBody", min: 4, why: "top-level and nested table-cell list bodies"},
		{token: "/S /Table", min: 2, why: "outer and nested table structures"},
		{token: "/S /TR", min: 4, why: "outer and nested table rows"},
		{token: "/S /TD", min: 4, why: "outer and nested table data cells"},
		{token: "/S /P", min: 4, why: "paragraphs including mixed table-cell block content"},
	} {
		if got := strings.Count(text, check.token); got < check.min {
			return fmt.Errorf("%s: expected at least %d occurrences of %q (%s), got %d", path, check.min, check.token, check.why, got)
		}
	}
	bdc := strings.Count(text, "BDC")
	bmc := strings.Count(text, "BMC")
	emc := strings.Count(text, "EMC")
	if bdc+bmc != emc {
		return fmt.Errorf("%s: unbalanced marked content wrappers: BDC=%d BMC=%d EMC=%d", path, bdc, bmc, emc)
	}
	return nil
}

func requireTokens(path, text string, checks []tokenCheck) error {
	var missing []string
	for _, check := range checks {
		if !strings.Contains(text, check.token) {
			missing = append(missing, fmt.Sprintf("%q (%s)", check.token, check.why))
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%s: missing required local compliance markers:\n- %s", path, strings.Join(missing, "\n- "))
	}
	return nil
}

func exitErr(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
