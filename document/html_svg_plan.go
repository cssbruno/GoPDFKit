// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"context"
	"fmt"
	"strings"

	"github.com/cssbruno/paperrune/internal/layoutengine"
	"github.com/cssbruno/paperrune/internal/papercompile"
	"github.com/cssbruno/paperrune/layout"
)

const htmlUnifiedSVGMaxLabelBytes = 16 << 10

type htmlUnifiedSVGMeta struct {
	token    int
	end      int
	svg      *SVG
	label    string
	artifact bool
	link     string
	width    float64
	height   float64
}

func htmlUnifiedTokenInsideSVG(compiled *CompiledHTML, token int) bool {
	if compiled == nil || token < 0 || token >= len(compiled.tokenNode) {
		return false
	}
	for node := compiled.tokenNode[token]; node >= 0; node = compiled.nodeIndexes[node].Parent {
		if compiled.tokens[compiled.nodeIndexes[node].Token].Str == "svg" {
			return true
		}
	}
	return false
}

func htmlUnifiedInlineSVGMeta(compiled *CompiledHTML) (htmlUnifiedSVGMeta, error) {
	if compiled == nil || len(compiled.inlineSVGs) != 1 {
		return htmlUnifiedSVGMeta{}, htmlPlanUnsupported("svg", 0, "the initial inline SVG cohort requires exactly one SVG")
	}
	for token := range compiled.inlineSVGs {
		return htmlUnifiedInlineSVGMetaAt(compiled, token, false)
	}
	return htmlUnifiedSVGMeta{}, htmlPlanUnsupported("svg", 0, "compiled inline SVG is missing")
}

func htmlUnifiedInlineSVGMetaAt(compiled *CompiledHTML, svgToken int, allowOutside bool) (htmlUnifiedSVGMeta, error) {
	if compiled == nil {
		return htmlUnifiedSVGMeta{}, htmlPlanUnsupported("svg", svgToken, "compiled inline SVG is missing")
	}
	entry, ok := compiled.inlineSVGs[svgToken]
	if !ok {
		return htmlUnifiedSVGMeta{}, htmlPlanUnsupported("svg", svgToken, "compiled inline SVG is missing")
	}
	meta := htmlUnifiedSVGMeta{token: svgToken, end: entry.end, svg: entry.svg}
	if meta.svg == nil || meta.svg.Wd <= 0 || meta.svg.Ht <= 0 {
		return htmlUnifiedSVGMeta{}, htmlPlanUnsupported("svg", meta.token, "compiled SVG has invalid extents")
	}
	root := compiled.tokens[meta.token]
	for name := range root.Attr {
		lower := strings.ToLower(strings.TrimSpace(name))
		if strings.HasPrefix(lower, "on") || lower == "href" || lower == "xlink:href" {
			return htmlUnifiedSVGMeta{}, htmlPlanUnsupported("svg", meta.token, fmt.Sprintf("SVG root attribute %q is outside the secure unified cohort", name))
		}
	}
	meta.label = strings.TrimSpace(root.Attr["aria-label"])
	if len(meta.label) > htmlUnifiedSVGMaxLabelBytes {
		return htmlUnifiedSVGMeta{}, htmlPlanUnsupported("svg", meta.token, "SVG accessibility label exceeds its byte limit")
	}
	role := strings.ToLower(strings.TrimSpace(root.Attr["role"]))
	hidden := strings.ToLower(strings.TrimSpace(root.Attr["aria-hidden"]))
	if hidden != "" && hidden != "true" && hidden != "false" {
		return htmlUnifiedSVGMeta{}, htmlPlanUnsupported("svg", meta.token, "aria-hidden must be true or false")
	}
	switch role {
	case "", "img", "graphics-document":
	case "presentation", "none":
		meta.artifact = true
	default:
		return htmlUnifiedSVGMeta{}, htmlPlanUnsupported("svg", meta.token, fmt.Sprintf("SVG role %q is unsupported", role))
	}
	meta.artifact = meta.artifact || hidden == "true"
	if meta.artifact && meta.label != "" {
		return htmlUnifiedSVGMeta{}, htmlPlanUnsupported("svg", meta.token, "decorative SVG cannot also provide an accessibility label")
	}
	if !meta.artifact && meta.label == "" {
		return htmlUnifiedSVGMeta{}, htmlPlanUnsupported("svg", meta.token, "informative SVG requires aria-label; decorative SVG requires role=presentation or aria-hidden=true")
	}

	internalAnchorNode := -1
	for index := meta.token + 1; index < meta.end; index++ {
		token := compiled.tokens[index]
		if token.Cat == 'O' && token.Str == "a" {
			if internalAnchorNode >= 0 || meta.link != "" {
				return htmlUnifiedSVGMeta{}, htmlPlanUnsupported("svg", index, "only one SVG anchor is supported")
			}
			for name := range token.Attr {
				lower := strings.ToLower(strings.TrimSpace(name))
				if lower != "href" && lower != "xlink:href" {
					return htmlUnifiedSVGMeta{}, htmlPlanUnsupported("svg", index, fmt.Sprintf("SVG anchor attribute %q is unsupported", name))
				}
			}
			href := token.Attr["href"]
			if href == "" {
				href = token.Attr["xlink:href"]
			}
			link, linkErr := htmlLinkTarget(href)
			if linkErr != nil || link == "" || strings.HasPrefix(link, "#") {
				return htmlUnifiedSVGMeta{}, htmlPlanUnsupported("svg", index, "SVG anchor requires one external http, https, or mailto href")
			}
			meta.link = link
			internalAnchorNode = compiled.tokenNode[index]
		}
		if token.Cat == 'O' {
			for name := range token.Attr {
				if strings.HasPrefix(strings.ToLower(strings.TrimSpace(name)), "on") {
					return htmlUnifiedSVGMeta{}, htmlPlanUnsupported("svg", index, fmt.Sprintf("event attribute %q is forbidden", name))
				}
			}
		}
	}
	if internalAnchorNode >= 0 {
		drawable := map[string]bool{"path": true, "rect": true, "circle": true, "ellipse": true, "line": true, "polyline": true, "polygon": true, "text": true, "image": true, "use": true}
		for index := meta.token + 1; index < meta.end; index++ {
			token := compiled.tokens[index]
			if token.Cat != 'O' || !drawable[strings.ToLower(token.Str)] {
				continue
			}
			insideAnchor, insideDefinition := false, false
			for node := compiled.tokenNode[index]; node >= 0; node = compiled.nodeIndexes[node].Parent {
				if node == internalAnchorNode {
					insideAnchor = true
				}
				ancestor := strings.ToLower(compiled.tokens[compiled.nodeIndexes[node].Token].Str)
				if ancestor == "defs" || ancestor == "clippath" || ancestor == "pattern" || strings.HasSuffix(ancestor, "gradient") {
					insideDefinition = true
				}
			}
			if !insideDefinition && !insideAnchor {
				return htmlUnifiedSVGMeta{}, htmlPlanUnsupported("svg", index, "the SVG anchor must contain every drawable element")
			}
		}
	}

	svgNode := compiled.tokenNode[meta.token]
	ancestors := make(map[int]bool)
	for node := compiled.nodeIndexes[svgNode].Parent; node >= 0; node = compiled.nodeIndexes[node].Parent {
		ancestors[compiled.nodeIndexes[node].Token] = true
		opening := compiled.tokens[compiled.nodeIndexes[node].Token]
		switch opening.Str {
		case "a":
			if meta.link != "" {
				return htmlUnifiedSVGMeta{}, htmlPlanUnsupported("a", compiled.nodeIndexes[node].Token, "nested SVG anchors are unsupported")
			}
			for name := range opening.Attr {
				if name != "href" {
					return htmlUnifiedSVGMeta{}, htmlPlanUnsupported("a", compiled.nodeIndexes[node].Token, fmt.Sprintf("anchor attribute %q is unsupported", name))
				}
			}
			link, err := htmlLinkTarget(opening.Attr["href"])
			if err != nil || link == "" || strings.HasPrefix(link, "#") {
				return htmlUnifiedSVGMeta{}, htmlPlanUnsupported("a", compiled.nodeIndexes[node].Token, "SVG wrapper requires one external http, https, or mailto href")
			}
			meta.link = link
		case "html", "body", "main", "section", "article", "div":
			if len(opening.Attr) != 0 {
				return htmlUnifiedSVGMeta{}, htmlPlanUnsupported(opening.Str, compiled.nodeIndexes[node].Token, "SVG structural wrapper attributes are unsupported")
			}
		default:
			return htmlUnifiedSVGMeta{}, htmlPlanUnsupported(opening.Str, compiled.nodeIndexes[node].Token, "SVG has an unsupported HTML wrapper")
		}
	}
	for index, token := range compiled.tokens {
		if allowOutside {
			break
		}
		if index >= meta.token && index <= meta.end {
			continue
		}
		if token.Cat == 'T' && strings.TrimSpace(token.Str) == "" {
			continue
		}
		if token.Cat == 'O' && ancestors[index] {
			continue
		}
		if token.Cat == 'C' {
			supported := token.Str == "a" || token.Str == "html" || token.Str == "body" || token.Str == "main" || token.Str == "section" || token.Str == "article" || token.Str == "div"
			if supported {
				continue
			}
		}
		return htmlUnifiedSVGMeta{}, htmlPlanUnsupported("svg", index, "fragment contains content outside the single SVG")
	}
	meta.width, meta.height = meta.svg.Wd*72.0/96.0, meta.svg.Ht*72.0/96.0
	return meta, nil
}

func (f *Document) planCompiledHTMLInlineSVGContext(ctx context.Context, compiled *CompiledHTML) (LayoutDocumentPlan, error) {
	meta, err := htmlUnifiedInlineSVGMeta(compiled)
	if err != nil {
		return LayoutDocumentPlan{}, err
	}
	model := &layout.LayoutDocument{Body: []layout.Block{layout.ImageBlock{Width: meta.width, Height: meta.height, Alt: meta.label}}}
	if err := f.validateLayoutDocumentPlanEnvelope(model); err != nil {
		return LayoutDocumentPlan{}, layoutDocumentPlanError(err)
	}
	left, top, right, bottom := typedShadowMargins(f, layout.Spacing{})
	pageSize, body, err := typedShadowFixedGeometry(f, left, top, f.w-left-right, f.h-top-bottom)
	if err != nil {
		return LayoutDocumentPlan{}, htmlPlanUnsupported("svg", meta.token, err.Error())
	}
	planned, err := f.planUnifiedInlineSVGAtBody(ctx, meta, pageSize, body)
	if err != nil {
		return LayoutDocumentPlan{}, err
	}
	tree, err := papercompile.LowerLayoutDocumentTreeContext(ctx, model, layoutengine.CanonicalTreeLimits{})
	if err != nil {
		return LayoutDocumentPlan{}, fmt.Errorf("document: lower HTML SVG canonical tree: %w", err)
	}
	planned, err = bindTypedDeterministicInputs(planned, tree, model)
	if err != nil {
		return LayoutDocumentPlan{}, fmt.Errorf("document: bind HTML SVG deterministic inputs: %w", err)
	}
	envelope, err := f.snapshotLayoutDocumentEnvelope(model)
	if err != nil {
		return LayoutDocumentPlan{}, layoutDocumentPlanError(err)
	}
	layoutHash, err := planned.Hash()
	if err != nil {
		return LayoutDocumentPlan{}, err
	}
	hash, err := hashTypedLayoutDocumentEnvelope(layoutHash.String(), envelope)
	if err != nil {
		return LayoutDocumentPlan{}, err
	}
	return LayoutDocumentPlan{plan: planned, tree: tree, hash: hash, pages: 1, imageSources: svgDisplayImageSources(meta.svg), envelope: envelope}, nil
}

func (f *Document) planUnifiedInlineSVGAtBody(ctx context.Context, meta htmlUnifiedSVGMeta, pageSize layoutengine.Size, body layoutengine.Rect) (layoutengine.LayoutPlan, error) {
	width, err := layoutengine.FixedFromPoints(meta.width)
	if err != nil || width <= 0 || width > body.Width {
		return layoutengine.LayoutPlan{}, htmlPlanUnsupported("svg", meta.token, "SVG width is invalid or exceeds the body")
	}
	height, err := layoutengine.FixedFromPoints(meta.height)
	if err != nil || height <= 0 || height > body.Height {
		return layoutengine.LayoutPlan{}, htmlPlanUnsupported("svg", meta.token, "SVG height is invalid or exceeds the available body")
	}
	box, err := layoutengine.NewRect(body.X, body.Y, width, height)
	if err != nil {
		return layoutengine.LayoutPlan{}, err
	}
	base, err := layoutengine.NewLayoutPlan(layoutengine.LayoutPlanInput{
		Pages: []layoutengine.PlannedPage{{Number: 1, Size: pageSize, Fragments: layoutengine.IndexRange{Count: 1}}},
		Fragments: []layoutengine.Fragment{{ID: 1, Node: 1, Key: "@html-svg", Instance: "@html-svg", Page: 1,
			Region: layoutengine.RegionBody, BorderBox: box, ContentBox: box, Continuation: layoutengine.ContinuationWhole}},
	})
	if err != nil {
		return layoutengine.LayoutPlan{}, err
	}
	planned, err := AttachSVGDisplayPlan(base, meta.svg, SVGDisplayPlanPlacement{
		Page: 1, Fragment: 1, X: body.X.Points(), Y: body.Y.Points(), Scale: 72.0 / 96.0, LinkURI: meta.link,
	})
	if err != nil {
		return layoutengine.LayoutPlan{}, htmlPlanUnsupported("svg", meta.token, err.Error())
	}
	role := layoutengine.SemanticRoleFigure
	if meta.artifact {
		role = layoutengine.SemanticRoleArtifact
	}
	parent := layoutengine.SemanticNodeID(1)
	nodes := []layoutengine.SemanticNode{{ID: 1, Role: layoutengine.SemanticRoleDocument, Key: "@html-document", Instance: "@html-document"}}
	if meta.link != "" {
		nodes = append(nodes, layoutengine.SemanticNode{ID: 2, Parent: 1, Role: layoutengine.SemanticRoleLink, Key: "@html-svg-link", Instance: "@html-svg-link"})
		parent = 2
	}
	owner := layoutengine.SemanticNodeID(len(nodes) + 1) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
	nodes = append(nodes, layoutengine.SemanticNode{ID: owner, Parent: parent, Role: role, Key: "@html-svg", Instance: "@html-svg",
		Attributes: layoutengine.SemanticAttributes{AlternateText: meta.label}})
	associations := []layoutengine.SemanticFragmentAssociation{{Semantic: owner, Page: 1, Fragment: 1}}
	var reading []layoutengine.ReadingOccurrence
	if !meta.artifact {
		reading = []layoutengine.ReadingOccurrence{{Semantic: owner, Page: 1, Fragment: 1}}
	}
	planned, err = layoutengine.AttachSemantics(planned, nodes, associations, reading)
	if err != nil {
		return layoutengine.LayoutPlan{}, err
	}
	return planned, nil
}

func (html *HTML) planCompiledInlineSVGFragmentContext(ctx context.Context, compiled *CompiledHTML, frame htmlStartFrame) (htmlFragmentPlan, error) {
	meta, err := htmlUnifiedInlineSVGMeta(compiled)
	if err != nil {
		return htmlFragmentPlan{}, err
	}
	pageSize, fullBody, err := typedShadowFixedGeometry(html.pdf, frame.left, frame.top,
		html.pdf.w-frame.left-frame.right, html.pdf.h-frame.top-frame.bottom)
	if err != nil {
		return htmlFragmentPlan{}, err
	}
	cursor, err := fixedFromDocumentUnits(html.pdf, frame.y)
	if err != nil {
		return htmlFragmentPlan{}, err
	}
	fullBottom, _ := fullBody.Bottom()
	if cursor < fullBody.Y || cursor > fullBottom {
		return htmlFragmentPlan{}, htmlPlanUnsupported("svg", meta.token, "live cursor is outside the page body")
	}
	firstHeight, err := fullBottom.Sub(cursor)
	if err != nil {
		return htmlFragmentPlan{}, err
	}
	firstBody, err := layoutengine.NewRect(fullBody.X, cursor, fullBody.Width, firstHeight)
	if err != nil {
		return htmlFragmentPlan{}, err
	}
	height, err := layoutengine.FixedFromPoints(meta.height)
	if err != nil || height <= 0 {
		return htmlFragmentPlan{}, htmlPlanUnsupported("svg", meta.token, "SVG height is invalid")
	}
	leadingBreak := height > firstBody.Height
	body := firstBody
	if leadingBreak {
		body = fullBody
	}
	planned, err := html.pdf.planUnifiedInlineSVGAtBody(ctx, meta, pageSize, body)
	if err != nil {
		return htmlFragmentPlan{}, err
	}
	model := &layout.LayoutDocument{Body: []layout.Block{layout.ImageBlock{Width: meta.width, Height: meta.height, Alt: meta.label}}}
	model.PageTemplate.Margins = layout.Spacing{Left: frame.left, Top: frame.top, Right: frame.right, Bottom: frame.bottom}
	tree, err := papercompile.LowerLayoutDocumentTreeContext(ctx, model, layoutengine.CanonicalTreeLimits{})
	if err != nil {
		return htmlFragmentPlan{}, fmt.Errorf("document: lower HTML SVG canonical tree: %w", err)
	}
	hash, err := planned.Hash()
	if err != nil {
		return htmlFragmentPlan{}, err
	}
	addedPages := 0
	if leadingBreak {
		addedPages = 1
	}
	if html.pdf.limits.MaxPages > 0 && frame.pageCount+addedPages > html.pdf.limits.MaxPages {
		return htmlFragmentPlan{}, fmt.Errorf("%w: %d > %d", ErrPageLimitExceeded, frame.pageCount+addedPages, html.pdf.limits.MaxPages)
	}
	if addedPages+1 > html.maxGeneratedPages() {
		return htmlFragmentPlan{}, fmt.Errorf("%w: SVG rendering exceeded maximum generated pages", ErrHTMLLimitExceeded)
	}
	bottom, err := body.Y.Add(height)
	if err != nil {
		return htmlFragmentPlan{}, err
	}
	bottom++
	plan := LayoutDocumentPlan{plan: planned, tree: tree, hash: hash.String(), pages: 1, imageSources: svgDisplayImageSources(meta.svg)}
	return htmlFragmentPlan{
		plan: plan, start: frame, reuseCurrentPage: !leadingBreak,
		final: htmlFinalFrame{page: frame.page + addedPages, x: frame.left, y: html.pdf.PointConvert(bottom.Points())},
	}, nil
}
