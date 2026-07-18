// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

type htmlTextStyle struct {
	bold               bool
	italic             bool
	underline          bool
	strike             bool
	preserveWhitespace bool
	href               string
	align              string
	verticalAlign      string
	fontFamily         string
	fontSize           float64
	lineHeight         float64
	color              CSSColorType
	role               string
	list               string
	listStyleType      string
	script             int
}

type htmlListState struct {
	kind      string
	styleType string
	counter   int
	indent    float64
}

func htmlClosePops(tag string) bool {
	switch tag {
	case "br", "img", "hr", "meta", "link", "input":
		return false
	default:
		return true
	}
}
