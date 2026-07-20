// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"sort"

	"github.com/cssbruno/paperrune/internal/layoutengine"
	"github.com/cssbruno/paperrune/layout"
)

func htmlUnifiedMixedSVGMetas(compiled *CompiledHTML) ([]htmlUnifiedSVGMeta, error) {
	if compiled == nil || len(compiled.inlineSVGs) == 0 {
		return nil, nil
	}
	tokens := make([]int, 0, len(compiled.inlineSVGs))
	for token := range compiled.inlineSVGs {
		tokens = append(tokens, token)
	}
	sort.Ints(tokens)
	metas := make([]htmlUnifiedSVGMeta, 0, len(tokens))
	for _, token := range tokens {
		meta, err := htmlUnifiedInlineSVGMetaAt(compiled, token, true)
		if err != nil {
			return nil, err
		}
		if meta.link != "" {
			return nil, htmlPlanUnsupported("svg", token, "linked SVG is supported only when the SVG is the fragment's sole content")
		}
		// Text and embedded images need additional line/font/image resource
		// remapping. Keep the first mixed-flow cohort vector-only; the exact
		// sole-SVG path continues to support both rich element kinds.
		for _, element := range meta.svg.Elements {
			if element.Kind == "text" && !element.Text.Style.Hidden {
				return nil, htmlPlanUnsupported("svg", token, "SVG text is supported only when the SVG is the fragment's sole content")
			}
			if element.Kind == "image" && !element.Image.Style.Hidden {
				return nil, htmlPlanUnsupported("svg", token, "embedded SVG images are supported only when the SVG is the fragment's sole content")
			}
		}
		metas = append(metas, meta)
	}
	return metas, nil
}

func htmlUnifiedSVGPlaceholder() ([]byte, layoutengine.ImageContentDigest, error) {
	placeholder := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	placeholder.SetNRGBA(0, 0, color.NRGBA{R: 19, G: 71, B: 233, A: 0})
	placeholder.SetNRGBA(1, 0, color.NRGBA{R: 211, G: 37, B: 101, A: 0})
	placeholder.SetNRGBA(2, 0, color.NRGBA{R: 83, G: 197, B: 29, A: 0})
	placeholder.SetNRGBA(0, 1, color.NRGBA{R: 157, G: 43, B: 181, A: 0})
	placeholder.SetNRGBA(1, 1, color.NRGBA{R: 7, G: 229, B: 149, A: 0})
	placeholder.SetNRGBA(2, 1, color.NRGBA{R: 251, G: 113, B: 17, A: 0})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, placeholder); err != nil {
		return nil, "", fmt.Errorf("document: encode internal SVG placeholder: %w", err)
	}
	data := encoded.Bytes()
	digest := sha256.Sum256(data)
	return data, layoutengine.ImageContentDigest(hex.EncodeToString(digest[:])), nil
}

func lowerCompiledHTMLMixedSVGUnitsBounds(ctx context.Context, compiled *CompiledHTML, lineHeight float64, pointsToUnits func(float64) float64, availableWidth, availableHeight float64, metas []htmlUnifiedSVGMeta) (*layout.LayoutDocument, layoutengine.ImageContentDigest, error) {
	data, digest, err := htmlUnifiedSVGPlaceholder()
	if err != nil {
		return nil, "", err
	}
	blocks := make(map[int]layout.ImageBlock, len(metas))
	for _, meta := range metas {
		blocks[meta.token] = layout.ImageBlock{
			Data: append([]byte(nil), data...), Format: "png", Alt: meta.label, Decorative: meta.artifact,
			Width: pointsToUnits(meta.width), Height: pointsToUnits(meta.height),
		}
	}
	model := &layout.LayoutDocument{}
	textBytes := 0
	state := &htmlPlanLoweringState{boxContainingHeights: map[int]float64{0: availableHeight}, inlineSVGBlocks: blocks}
	body, err := lowerHTMLPlanBlockRangeWidthState(ctx, compiled, 0, len(compiled.tokens), lineHeight, &textBytes, 0, pointsToUnits, availableWidth, state)
	if err != nil {
		return nil, "", err
	}
	if len(body) == 0 {
		return nil, "", htmlPlanUnsupported("fragment", 0, "fragment has no plannable content")
	}
	model.Body = body
	return model, digest, nil
}

func htmlPlanGeometryOnly(projection layoutengine.LayoutPlanProjection) (layoutengine.LayoutPlan, error) {
	pages := append([]layoutengine.PlannedPage(nil), projection.Pages...)
	for index := range pages {
		pages[index].Commands = layoutengine.IndexRange{}
	}
	return layoutengine.NewLayoutPlan(layoutengine.LayoutPlanInput{
		Pages: pages, Fragments: projection.Fragments, Lines: projection.Lines,
		PageRegions: projection.PageRegions, GridTracks: projection.GridTracks,
		Breaks: projection.Breaks, Diagnostics: projection.Diagnostics,
		SemanticNodes: projection.SemanticNodes, SemanticFragments: projection.SemanticFragments, ReadingOrder: projection.ReadingOrder,
	})
}

func composeHTMLMixedSVGPlan(ctx context.Context, base layoutengine.LayoutPlan, metas []htmlUnifiedSVGMeta, placeholder layoutengine.ImageContentDigest) (layoutengine.LayoutPlan, error) {
	projection := base.Projection()
	geometry, err := htmlPlanGeometryOnly(projection)
	if err != nil {
		return layoutengine.LayoutPlan{}, fmt.Errorf("document: build mixed HTML/SVG geometry: %w", err)
	}
	fragmentPages := make(map[layoutengine.FragmentID]uint32, len(projection.Fragments))
	for _, fragment := range projection.Fragments {
		fragmentPages[fragment.ID] = fragment.Page
	}

	placeholderImages := make(map[uint32]bool)
	placements := make([]layoutengine.PlannedImage, 0, len(metas))
	for index, image := range projection.Images {
		if !image.Resource.Valid() || uint64(image.Resource) > uint64(len(projection.ImageResources)) {
			continue
		}
		if projection.ImageResources[image.Resource-1].Digest == placeholder {
			placeholderImages[uint32(index)] = true
			placements = append(placements, image)
		}
	}
	if len(placements) != len(metas) {
		return layoutengine.LayoutPlan{}, htmlPlanUnsupported("svg", 0, fmt.Sprintf("mixed SVG placeholder count changed during planning: %d != %d", len(placements), len(metas)))
	}

	replacements := make([]layoutengine.LayoutPlanProjection, len(metas))
	for index, meta := range metas {
		image := placements[index]
		page := fragmentPages[image.Fragment]
		scaleX := image.Bounds.Width.Points() / meta.svg.Wd
		scaleY := image.Bounds.Height.Points() / meta.svg.Ht
		if page == 0 || !finiteNumbers(scaleX, scaleY) || scaleX <= 0 || math.Abs(scaleX-scaleY) > 1.0/1024.0 {
			return layoutengine.LayoutPlan{}, htmlPlanUnsupported("svg", meta.token, "mixed-flow SVG placement lost its intrinsic aspect ratio")
		}
		attached, attachErr := AttachSVGDisplayPlanContext(ctx, geometry, meta.svg, SVGDisplayPlanPlacement{
			Page: page, Fragment: image.Fragment, X: image.Bounds.X.Points(), Y: image.Bounds.Y.Points(), Scale: scaleX, LinkURI: meta.link,
		}, DefaultSVGDisplayPlanLimits())
		if attachErr != nil {
			return layoutengine.LayoutPlan{}, htmlPlanUnsupported("svg", meta.token, attachErr.Error())
		}
		replacements[index] = attached.Projection()
	}

	input := layoutengine.DisplayListInput{
		Fonts: projection.Fonts, GlyphRuns: projection.GlyphRuns,
		Destinations: projection.Destinations, Links: projection.Links,
		Paths: projection.Paths, Transforms: projection.Transforms, Clips: projection.Clips,
		Fills: projection.Fills, Strokes: projection.Strokes,
	}
	resourceMap := make(map[layoutengine.ImageResourceID]layoutengine.ImageResourceID, len(projection.ImageResources))
	for _, resource := range projection.ImageResources {
		if resource.Digest == placeholder {
			continue
		}
		oldID := resource.ID
		resource.ID = layoutengine.ImageResourceID(len(input.ImageResources) + 1) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
		resourceMap[oldID] = resource.ID
		input.ImageResources = append(input.ImageResources, resource)
	}
	imageMap := make(map[uint32]uint32, len(projection.Images))
	for index, image := range projection.Images {
		if placeholderImages[uint32(index)] {
			continue
		}
		image.Resource = resourceMap[image.Resource]
		if !image.Resource.Valid() {
			return layoutengine.LayoutPlan{}, fmt.Errorf("document: mixed HTML image resource remap is missing")
		}
		imageMap[uint32(index)] = uint32(len(input.Images)) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
		input.Images = append(input.Images, image)
	}

	replacementIndex := 0
	for pageIndex, page := range projection.Pages {
		start, end := int(page.Commands.Start), int(page.Commands.Start+page.Commands.Count)
		for _, command := range projection.Commands[start:end] {
			if command.Kind == layoutengine.CommandImage && placeholderImages[command.Payload] {
				if replacementIndex >= len(replacements) {
					return layoutengine.LayoutPlan{}, htmlPlanUnsupported("svg", 0, "mixed SVG command order exceeds its metadata")
				}
				if err := appendHTMLMixedSVGDisplay(&input, replacements[replacementIndex], uint32(pageIndex+1)); err != nil {
					return layoutengine.LayoutPlan{}, err
				}
				replacementIndex++
				continue
			}
			payload := command.Payload
			if command.Kind == layoutengine.CommandImage {
				var ok bool
				payload, ok = imageMap[payload]
				if !ok {
					return layoutengine.LayoutPlan{}, fmt.Errorf("document: mixed HTML image command remap is missing")
				}
			}
			input.Items = append(input.Items, layoutengine.DisplayItem{Kind: command.Kind, Payload: payload, Page: uint32(pageIndex + 1)})
		}
	}
	if replacementIndex != len(replacements) {
		return layoutengine.LayoutPlan{}, htmlPlanUnsupported("svg", 0, "mixed SVG commands were not all composed")
	}
	return layoutengine.AttachDisplayList(geometry, input)
}

func appendHTMLMixedSVGDisplay(input *layoutengine.DisplayListInput, projection layoutengine.LayoutPlanProjection, page uint32) error {
	if len(projection.Fonts) != 0 || len(projection.GlyphRuns) != 0 || len(projection.ImageResources) != 0 || len(projection.Images) != 0 || len(projection.Destinations) != 0 {
		return htmlPlanUnsupported("svg", 0, "rich SVG resources are outside the mixed-flow compositor")
	}
	pathBase := uint32(len(input.Paths))           // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
	transformBase := uint32(len(input.Transforms)) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
	clipBase := uint32(len(input.Clips))           // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
	fillBase := uint32(len(input.Fills))           // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
	strokeBase := uint32(len(input.Strokes))       // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
	linkBase := uint32(len(input.Links))           // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
	input.Paths = append(input.Paths, projection.Paths...)
	input.Transforms = append(input.Transforms, projection.Transforms...)
	for _, clip := range projection.Clips {
		clip.Path += pathBase
		input.Clips = append(input.Clips, clip)
	}
	for _, fill := range projection.Fills {
		fill.Path += pathBase
		input.Fills = append(input.Fills, fill)
	}
	for _, stroke := range projection.Strokes {
		stroke.Path += pathBase
		input.Strokes = append(input.Strokes, stroke)
	}
	input.Links = append(input.Links, projection.Links...)
	for _, command := range projection.Commands {
		payload := command.Payload
		switch command.Kind {
		case layoutengine.CommandSaveState, layoutengine.CommandRestoreState:
			payload = 0
		case layoutengine.CommandTransform:
			payload += transformBase
		case layoutengine.CommandClip:
			payload += clipBase
		case layoutengine.CommandFillPath:
			payload += fillBase
		case layoutengine.CommandStrokePath:
			payload += strokeBase
		case layoutengine.CommandLink:
			payload += linkBase
		default:
			return htmlPlanUnsupported("svg", 0, fmt.Sprintf("mixed-flow SVG command %q is unsupported", command.Kind))
		}
		input.Items = append(input.Items, layoutengine.DisplayItem{Kind: command.Kind, Payload: payload, Page: page})
	}
	return nil
}

func withoutHTMLSVGPlaceholder(sources plannedImageSources, placeholder layoutengine.ImageContentDigest) plannedImageSources {
	if len(sources) == 0 {
		return sources
	}
	result := make(plannedImageSources, len(sources))
	for digest, data := range sources {
		if digest != placeholder {
			result[digest] = data
		}
	}
	return result
}
