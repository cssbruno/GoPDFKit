// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import "strings"

// ExtractHTMLFooterBlock removes the first HTML footer element from an HTML
// fragment and returns it as a shared FooterBlock.
func ExtractHTMLFooterBlock(htmlStr string) (bodyHTML string, footer *FooterBlock) {
	tokens := HTMLTokenize(htmlStr)
	bodyTokens, footerTokens, found := splitHTMLFooterTokens(tokens)
	if !found {
		return htmlStr, nil
	}
	text := strings.TrimSpace(htmlPlainText(footerTokens))
	bodyHTML = strings.TrimSpace(htmlSerializeTokens(bodyTokens))
	if text == "" {
		return bodyHTML, &FooterBlock{ReservePageArea: true}
	}
	return bodyHTML, &FooterBlock{
		Blocks: []Block{
			ParagraphBlock{
				Segments: []TextSegment{{Text: text}},
				Style:    TextStyle{FontFamily: "Helvetica", FontSize: 9, Align: "C"},
			},
		},
		Height:          8,
		ReservePageArea: true,
	}
}

func splitHTMLFooterTokens(tokens []HTMLSegmentType) (body []HTMLSegmentType, footer []HTMLSegmentType, found bool) {
	for i := 0; i < len(tokens); i++ {
		token := tokens[i]
		if !found && token.Cat == 'O' && token.Str == "footer" {
			collected, end := htmlCollectElementTokens(tokens, i, "footer")
			if len(collected) >= 2 {
				footer = collected[1 : len(collected)-1]
			}
			found = true
			i = end
			continue
		}
		body = append(body, token)
	}
	return body, footer, found
}
