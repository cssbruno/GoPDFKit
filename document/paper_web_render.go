// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"context"
	"errors"
	"fmt"

	"github.com/cssbruno/paperrune/internal/layoutengine"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gobolditalic"
	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/gofont/gomonobolditalic"
	"golang.org/x/image/font/gofont/gomonoitalic"
	"golang.org/x/image/font/gofont/goregular"
)

// PaperPlanWebRenderRequest selects one immutable page and a bounded raster
// density for a browser WASM renderer. No layout choices are accepted here.
type PaperPlanWebRenderRequest struct {
	Page uint32
	DPI  uint32
}

func DefaultPaperPlanWebRenderRequest(page uint32) PaperPlanWebRenderRequest {
	return PaperPlanWebRenderRequest{Page: page, DPI: layoutengine.DefaultDisplayRasterProfile().DPI}
}

// WebDisplayRenderPayload returns a detached, self-verifying payload consumed
// by the Go WASM renderer. It contains the canonical immutable plan and only
// content-addressed renderer resources; source text is never included.
func (p PaperPlan) WebDisplayRenderPayload(ctx context.Context, request PaperPlanWebRenderRequest) ([]byte, error) {
	if ctx == nil {
		return nil, errors.New("document: nil web render context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if p.hash == "" || p.pages <= 0 || request.Page == 0 || uint64(request.Page) > uint64(p.pages) {
		return nil, errors.New("document: invalid paper plan web render page")
	}
	profile := layoutengine.DefaultDisplayRasterProfile()
	profile.DPI = request.DPI
	limits := layoutengine.DefaultDisplayRasterLimits()
	pageProfile := ""
	if inputs, ok := p.plan.DeterministicInputs(); ok {
		pageProfile = inputs.PageProfile.ID
	}
	if pageProfile == "" {
		return nil, errors.New("document: web render plan has no deterministic page profile")
	}
	projection := p.plan.Projection()
	fonts := make(map[layoutengine.CoreFontMetricsDigest][]byte, len(projection.Fonts))
	for _, resource := range projection.Fonts {
		if resource.EmbeddedUTF8 == nil {
			fonts[resource.MetricsDigest] = paperWebCoreFontProgram(resource.Face)
			continue
		}
		program := p.fontSources[resource.EmbeddedUTF8.Digest]
		if len(program) == 0 {
			return nil, fmt.Errorf("document: web render embedded font %s is unavailable", resource.EmbeddedUTF8.Name)
		}
		fonts[resource.MetricsDigest] = append([]byte(nil), program...)
	}
	images := make(layoutengine.DisplaySVGImageSources, len(p.imageSources))
	for digest, source := range p.imageSources {
		images[digest] = append([]byte(nil), source...)
	}
	return layoutengine.EncodeWebDisplayRenderPayload(p.plan, layoutengine.DisplayRasterSources{FontPrograms: fonts, Images: images},
		layoutengine.DisplayRasterRequest{Page: request.Page, Profile: profile, Limits: limits, Revisions: p.revisions, PageProfile: pageProfile})
}

// Standard-14 fonts do not carry programs in a PDF plan. The browser preview
// uses deterministic Go outlines with matching weight/slant and, critically,
// a monospace family for Courier while retaining the exact planned advances.
func paperWebCoreFontProgram(face layoutengine.CoreFontFace) []byte {
	switch face {
	case layoutengine.CoreFontCourier:
		return gomono.TTF
	case layoutengine.CoreFontCourierBold:
		return gomonobold.TTF
	case layoutengine.CoreFontCourierOblique:
		return gomonoitalic.TTF
	case layoutengine.CoreFontCourierBoldOblique:
		return gomonobolditalic.TTF
	case layoutengine.CoreFontHelveticaBold, layoutengine.CoreFontTimesBold:
		return gobold.TTF
	case layoutengine.CoreFontHelveticaOblique, layoutengine.CoreFontTimesItalic:
		return goitalic.TTF
	case layoutengine.CoreFontHelveticaBoldOblique, layoutengine.CoreFontTimesBoldItalic:
		return gobolditalic.TTF
	default:
		return goregular.TTF
	}
}
