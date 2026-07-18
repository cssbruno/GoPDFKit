// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
)

type plannedImageSources map[layoutengine.ImageContentDigest][]byte
type plannedFontSources map[layoutengine.CoreFontMetricsDigest][]byte

func chargePlannedFontSourceBytes(seen map[layoutengine.CoreFontMetricsDigest]bool, digest layoutengine.CoreFontMetricsDigest, size, limit uint64, used *uint64) error {
	if seen[digest] {
		return nil
	}
	if used == nil || *used > limit || size > limit-*used {
		return fmt.Errorf("%w: cumulative embedded font source bytes exceed limit", errCoreLayoutPlanPaintUnsupported)
	}
	*used += size
	seen[digest] = true
	return nil
}

type plannedImageLookupBudget struct {
	remainingLookups uint64
	remainingBytes   uint64
}

func newPlannedImageLookupBudget(resources int, maxBytes int) (plannedImageLookupBudget, error) {
	if resources < 0 || maxBytes <= 0 {
		return plannedImageLookupBudget{}, fmt.Errorf("%w: invalid planned image lookup limits", errCoreLayoutPlanPaintUnsupported)
	}
	return plannedImageLookupBudget{remainingLookups: uint64(resources), remainingBytes: uint64(maxBytes)}, nil
}

func lookupPlannedImageSourceContext(ctx context.Context, sources plannedImageSources, digest layoutengine.ImageContentDigest, budget *plannedImageLookupBudget) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if budget == nil || budget.remainingLookups == 0 {
		return nil, fmt.Errorf("%w: planned image lookup count exceeds limit", errCoreLayoutPlanPaintUnsupported)
	}
	budget.remainingLookups--
	encoded, exists := sources[digest]
	if !exists || len(encoded) == 0 {
		return nil, fmt.Errorf("%w: image %s bytes are unavailable", errCoreLayoutPlanPaintUnsupported, digest)
	}
	if uint64(len(encoded)) > budget.remainingBytes {
		return nil, fmt.Errorf("%w: cumulative planned image source bytes exceed limit", errCoreLayoutPlanPaintUnsupported)
	}
	budget.remainingBytes -= uint64(len(encoded))
	return encoded, nil
}

type preparedDisplayImage struct {
	resource   layoutengine.ImageResource
	key        string
	info       *ImageInfo
	minVersion string
}

type preparedDisplayCommand struct {
	kind      layoutengine.DisplayCommandKind
	run       layoutengine.CoreGlyphRun
	font      preparedCorePlanFont
	image     layoutengine.PlannedImage
	asset     preparedDisplayImage
	crop      preparedDisplayImageCrop
	link      layoutengine.PlannedLink
	path      layoutengine.PlannedPath
	transform layoutengine.Transform
	clip      layoutengine.PlannedClip
	fill      layoutengine.PlannedFill
	stroke    layoutengine.PlannedStroke
	semantic  []preparedDisplaySemantic
}

type preparedDisplaySemantic struct {
	id               layoutengine.SemanticNodeID
	role             string
	alt              string
	actual           string
	lang             string
	header           bool
	scope            string
	rowSpan, colSpan uint32
}

type preparedDisplayImageCrop struct {
	enabled                        bool
	clipX, clipY, clipW, clipH     float64
	imageX, imageY, imageW, imageH float64
}

type preparedDisplayPage struct {
	page     layoutengine.PlannedPage
	commands []preparedDisplayCommand
}

type preparedDisplayPlanPDF struct {
	fonts            map[layoutengine.FontResourceID]preparedCorePlanFont
	fontOrder        []layoutengine.FontResourceID
	images           map[layoutengine.ImageResourceID]preparedDisplayImage
	imageOrder       []layoutengine.ImageResourceID
	pages            []preparedDisplayPage
	destinations     []layoutengine.PlannedDestination
	documentLanguage string
}

// paintDisplayLayoutPlanPDF is the initial mixed text/image production sink.
// All plan validation, byte-digest verification, image decoding, intrinsic
// dimension checks, and resource preparation complete before target mutation.
func (f *Document) paintDisplayLayoutPlanPDF(plan layoutengine.LayoutPlan, sources plannedImageSources) error {
	prepared, err := f.preflightDisplayLayoutPlanPDF(plan, sources)
	if err != nil {
		return err
	}
	return f.paintPreparedDisplayLayoutPlanPDF(prepared)
}

func (f *Document) paintPreparedDisplayLayoutPlanPDF(prepared preparedDisplayPlanPDF) error {
	return f.paintPreparedDisplayLayoutPlanPDFAtCurrentPage(prepared, false, 0, false)
}

func typedPDFSemanticRole(node layoutengine.SemanticNode) string {
	switch node.Role {
	case layoutengine.SemanticRoleSection:
		return "Sect"
	case layoutengine.SemanticRoleHeading:
		if node.Attributes.HeadingLevel >= 1 && node.Attributes.HeadingLevel <= 6 {
			return fmt.Sprintf("H%d", node.Attributes.HeadingLevel)
		}
		return "H"
	case layoutengine.SemanticRoleParagraph:
		if strings.Contains(string(node.Key), "caption") {
			return "Caption"
		}
		return "P"
	case layoutengine.SemanticRoleList:
		return "L"
	case layoutengine.SemanticRoleListItem:
		return "LI"
	case layoutengine.SemanticRoleTable:
		return "Table"
	case layoutengine.SemanticRoleRow:
		return "TR"
	case layoutengine.SemanticRoleCell:
		if node.Attributes.TableHeader {
			return "TH"
		}
		return "TD"
	case layoutengine.SemanticRoleFigure:
		return "Figure"
	case layoutengine.SemanticRoleLink:
		return "Link"
	default:
		return ""
	}
}

// ensureTaggedListBody adds the PDF/UA list-item children that are implicit in
// the renderer-neutral semantic tree. The planned text remains owned by the
// existing list item content, but the tagged PDF gets the required Lbl/LBody
// siblings without changing layout geometry or plan identity.
func (f *Document) ensureTaggedListBody(item *taggedElement) *taggedElement {
	if item == nil || item.Role != taggedRoleLI {
		return item
	}
	var body *taggedElement
	for _, child := range item.Children {
		if child == nil {
			continue
		}
		switch child.Role {
		case taggedRoleLbl:
		case taggedRoleLBody:
			body = child
		}
	}
	if body != nil {
		return body
	}
	label := &taggedElement{Role: taggedRoleLbl, MCID: -1, Parent: item}
	body = &taggedElement{Role: taggedRoleLBody, MCID: -1, Parent: item}
	item.Children = append(item.Children, label, body)
	f.tagged.elems = append(f.tagged.elems, label, body)
	return body
}

func (f *Document) beginPreparedSemantic(path []preparedDisplaySemantic, elements map[layoutengine.SemanticNodeID]*taggedElement) func() {
	if !f.tagged.enabled || len(path) == 0 {
		return func() {}
	}
	leaf := path[len(path)-1]
	if leaf.role == "Artifact" {
		f.BeginArtifact()
		return f.EndArtifact
	}
	var parent *taggedElement
	for _, semantic := range path {
		var elem *taggedElement
		if semantic.id.Valid() {
			elem = elements[semantic.id]
		}
		if elem == nil {
			elem = &taggedElement{Role: semantic.role, MCID: -1, Alt: semantic.alt,
				ActualText: semantic.actual, Lang: semantic.lang}
			if semantic.header || semantic.scope != "" || semantic.rowSpan > 1 || semantic.colSpan > 1 {
				elem.Table = normalizeTaggedTableAttributes(semantic.role, taggedTableAttributes{
					Scope: semantic.scope, RowSpan: semantic.rowSpan, ColSpan: semantic.colSpan,
				})
			}
			if parent != nil {
				elem.Parent = parent
				parent.Children = append(parent.Children, elem)
			}
			f.tagged.elems = append(f.tagged.elems, elem)
			if semantic.id.Valid() {
				elements[semantic.id] = elem
			}
		}
		if semantic.role == taggedRoleLI {
			parent = f.ensureTaggedListBody(elem)
			continue
		}
		parent = elem
	}
	if parent == nil {
		return func() {}
	}
	mcid := f.registerPreparedSemanticElement(parent)
	if mcid < 0 {
		return func() {}
	}
	if leaf.role == taggedRoleLink {
		f.tagged.pendingLinkElem = parent
	}
	begin := f.beginPreparedTaggedContent(parent.Role, mcid)
	if len(begin) != 0 {
		f.outbytes(begin)
	}
	return func() {
		if len(begin) != 0 {
			f.outbytes(taggedEndMarkedContent)
		}
	}
}

func (f *Document) paintPreparedSemanticContent(path []preparedDisplaySemantic, elements map[layoutengine.SemanticNodeID]*taggedElement, content []byte) {
	if len(path) == 0 || !f.tagged.enabled {
		f.outbytes(content)
		return
	}
	closeSemantic := f.beginPreparedSemantic(path, elements)
	f.outbytes(content)
	closeSemantic()
}

func (f *Document) paintPreparedDisplayLayoutPlanPDFAtCurrentPage(prepared preparedDisplayPlanPDF, reuseCurrent bool, pageOffset int, preserveAuthoredText bool) error {
	resources := f.ensureResourceStore()
	for _, id := range prepared.fontOrder {
		font := prepared.fonts[id]
		resources.setFont(font.key, font.font)
	}
	for _, id := range prepared.imageOrder {
		image := prepared.images[id]
		resources.setImage(image.key, image.info)
		f.requirePDFVersion(image.minVersion)
	}
	destinationLinks := make(map[layoutengine.DestinationID]int, len(prepared.destinations))
	semanticElements := make(map[layoutengine.SemanticNodeID]*taggedElement)
	f.tagged.documentLanguage = prepared.documentLanguage
	for _, destination := range prepared.destinations {
		destinationLinks[destination.ID] = f.AddLink()
	}
	for pageIndex, page := range prepared.pages {
		size := Size{Wd: f.PointConvert(page.page.Size.Width.Points()), Ht: f.PointConvert(page.page.Size.Height.Points())}
		if !reuseCurrent || pageIndex != 0 {
			f.AddPageFormat("P", size)
			if f.err != nil {
				return f.err
			}
		}
		var previousRun layoutengine.CoreGlyphRun
		previousRunSet := false
		for _, command := range page.commands {
			switch command.kind {
			case layoutengine.CommandSaveState:
				f.out("q")
			case layoutengine.CommandRestoreState:
				f.out("Q")
			case layoutengine.CommandTransform:
				f.paintPlannedTransform(page.page.Size.Height, command.transform)
			case layoutengine.CommandClip:
				f.paintPlannedClip(page.page.Size.Height, command.path, command.clip)
			case layoutengine.CommandFillPath:
				f.paintPlannedFill(page.page.Size.Height, command.path, command.fill)
			case layoutengine.CommandStrokePath:
				f.paintPlannedStroke(page.page.Size.Height, command.path, command.stroke)
			case layoutengine.CommandGlyphRun:
				var content []byte
				run := command.run
				if preserveAuthoredText && !run.LeadingSpace && previousRunSet {
					run.LeadingSpace = previousRun.TrailingSpace
				}
				if command.font.resource.EmbeddedUTF8 != nil {
					if preserveAuthoredText {
						content = appendPlannedUTF8GlyphRunActualText(nil, command.font.font, page.page.Size.Height, run)
					} else {
						content = appendPlannedUTF8GlyphRun(nil, command.font.font, page.page.Size.Height, run)
					}
				} else if preserveAuthoredText {
					content = appendPlannedCoreGlyphRunExactTJ(content, command.font.font, page.page.Size.Height, run)
				} else {
					content = appendPlannedCoreGlyphRun(nil, command.font.font, page.page.Size.Height, run)
				}
				if run.Opacity != 0 {
					f.out("q")
					f.SetAlpha(run.Opacity.Points(), "Normal")
				}
				f.paintPreparedSemanticContent(command.semantic, semanticElements, content)
				if run.Opacity != 0 {
					f.out("Q")
				}
				previousRun, previousRunSet = command.run, true
			case layoutengine.CommandImage:
				closeSemantic := f.beginPreparedSemantic(command.semantic, semanticElements)
				if command.image.Opacity != 0 {
					f.out("q")
					f.SetAlpha(command.image.Opacity.Points(), "Normal")
				}
				if command.crop.enabled {
					f.ClipRect(command.crop.clipX, command.crop.clipY, command.crop.clipW, command.crop.clipH, false)
					f.drawImageXObject(command.asset.info.i,
						command.crop.imageX, command.crop.imageY, command.crop.imageW, command.crop.imageH)
					f.ClipEnd()
				} else {
					bounds := command.image.Bounds
					f.drawImageXObject(command.asset.info.i,
						f.PointConvert(bounds.X.Points()), f.PointConvert(bounds.Y.Points()),
						f.PointConvert(bounds.Width.Points()), f.PointConvert(bounds.Height.Points()))
				}
				if command.image.Opacity != 0 {
					f.out("Q")
				}
				closeSemantic()
			case layoutengine.CommandLink:
				closeSemantic := f.beginPreparedSemantic(append(command.semantic, preparedDisplaySemantic{role: "Link"}), semanticElements)
				bounds := command.link.Bounds
				x := f.PointConvert(bounds.X.Points())
				y := f.PointConvert(bounds.Y.Points())
				width := f.PointConvert(bounds.Width.Points())
				height := f.PointConvert(bounds.Height.Points())
				if command.link.Destination.Valid() {
					f.newLink(x, y, width, height, destinationLinks[command.link.Destination], "")
				} else {
					f.newLink(x, y, width, height, 0, command.link.URI)
				}
				closeSemantic()
			}
		}
	}
	for _, destination := range prepared.destinations {
		f.setPlannedLink(destinationLinks[destination.ID], f.PointConvert(destination.Point.X.Points()),
			f.PointConvert(destination.Point.Y.Points()), int(destination.Page)+pageOffset)
	}
	return f.err
}

func (f *Document) preflightDisplayLayoutPlanPDF(plan layoutengine.LayoutPlan, sources plannedImageSources) (preparedDisplayPlanPDF, error) {
	return f.preflightDisplayLayoutPlanPDFContext(context.Background(), plan, sources)
}

func (f *Document) preflightDisplayLayoutPlanPDFContext(ctx context.Context, plan layoutengine.LayoutPlan, sources plannedImageSources) (preparedDisplayPlanPDF, error) {
	return f.preflightDisplayLayoutPlanPDFResourcesContextForTarget(ctx, plan, sources, nil, false)
}

func (f *Document) preflightDisplayLayoutPlanPDFContextForTarget(ctx context.Context, plan layoutengine.LayoutPlan, sources plannedImageSources, allowActivePage bool) (preparedDisplayPlanPDF, error) {
	return f.preflightDisplayLayoutPlanPDFResourcesContextForTarget(ctx, plan, sources, nil, allowActivePage)
}

func (f *Document) preflightDisplayLayoutPlanPDFResourcesContextForTarget(ctx context.Context, plan layoutengine.LayoutPlan, sources plannedImageSources, fontSources plannedFontSources, allowActivePage bool) (preparedDisplayPlanPDF, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return preparedDisplayPlanPDF{}, err
	}
	if f == nil || f.err != nil || (!allowActivePage && (f.page != 0 || f.state != documentStateUnopened)) ||
		(allowActivePage && (f.page <= 0 || f.state != documentStatePageOpen)) ||
		f.k <= 0 || !isFiniteFloat(f.k) || f.clipNest != 0 || f.transformNest != 0 {
		return preparedDisplayPlanPDF{}, fmt.Errorf("%w: requires a fresh error-free document", errCoreLayoutPlanPaintUnsupported)
	}
	if f.headerFnc != nil || f.footerFnc != nil || f.footerFncLpi != nil || f.pageAddGuard != nil ||
		len(f.aliasMap) != 0 || f.aliasNbPagesStr != "" {
		return preparedDisplayPlanPDF{}, fmt.Errorf("%w: custom or deferred page behavior is present", errCoreLayoutPlanPaintUnsupported)
	}
	if err := layoutengine.ValidateDisplayPaintPlan(plan, layoutengine.DefaultDisplayPaintLimits()); err != nil {
		return preparedDisplayPlanPDF{}, fmt.Errorf("document: preflight display plan: %w", err)
	}
	projection := plan.Projection()
	semanticByFragment := make(map[layoutengine.FragmentID][]preparedDisplaySemantic, len(projection.SemanticFragments))
	semanticNodes := make(map[layoutengine.SemanticNodeID]layoutengine.SemanticNode, len(projection.SemanticNodes))
	documentLanguage := ""
	for _, node := range projection.SemanticNodes {
		semanticNodes[node.ID] = node
		if node.Role == layoutengine.SemanticRoleDocument {
			documentLanguage = node.Attributes.Language
		}
	}
	for _, association := range projection.SemanticFragments {
		if semanticNodes[association.Semantic].Role == layoutengine.SemanticRoleArtifact {
			semanticByFragment[association.Fragment] = []preparedDisplaySemantic{{id: association.Semantic, role: "Artifact"}}
			continue
		}
		var reversed []preparedDisplaySemantic
		for id := association.Semantic; id.Valid(); {
			node := semanticNodes[id]
			if node.Role != layoutengine.SemanticRoleDocument && node.Role != layoutengine.SemanticRoleArtifact {
				role := typedPDFSemanticRole(node)
				if role != "" {
					reversed = append(reversed, preparedDisplaySemantic{id: node.ID, role: role,
						alt: node.Attributes.AlternateText, actual: node.Attributes.ActualText,
						lang: node.Attributes.Language, header: node.Attributes.TableHeader,
						scope: node.Attributes.TableScope, rowSpan: node.Attributes.TableRowSpan,
						colSpan: node.Attributes.TableColumnSpan})
				}
			}
			id = node.Parent
		}
		path := make([]preparedDisplaySemantic, len(reversed))
		for index := range reversed {
			path[index] = reversed[len(reversed)-1-index]
		}
		semanticByFragment[association.Fragment] = path
	}
	if f.limits.MaxPages > 0 && len(projection.Pages) > f.limits.MaxPages {
		return preparedDisplayPlanPDF{}, fmt.Errorf("%w: %d > %d", ErrPageLimitExceeded, len(projection.Pages), f.limits.MaxPages)
	}
	prepared := preparedDisplayPlanPDF{
		fonts:            make(map[layoutengine.FontResourceID]preparedCorePlanFont, len(projection.Fonts)),
		fontOrder:        make([]layoutengine.FontResourceID, 0, len(projection.Fonts)),
		images:           make(map[layoutengine.ImageResourceID]preparedDisplayImage, len(projection.ImageResources)),
		imageOrder:       make([]layoutengine.ImageResourceID, 0, len(projection.ImageResources)),
		pages:            make([]preparedDisplayPage, 0, len(projection.Pages)),
		destinations:     append([]layoutengine.PlannedDestination(nil), projection.Destinations...),
		documentLanguage: documentLanguage,
	}
	var fontSourceBytes uint64
	seenFontSources := make(map[layoutengine.CoreFontMetricsDigest]bool, len(projection.Fonts))
	for index, resource := range projection.Fonts {
		if index&31 == 0 {
			if err := ctx.Err(); err != nil {
				return preparedDisplayPlanPDF{}, err
			}
		}
		if resource.EmbeddedUTF8 != nil {
			data := fontSources[resource.EmbeddedUTF8.Digest]
			if err := chargePlannedFontSourceBytes(seenFontSources, resource.EmbeddedUTF8.Digest, uint64(len(data)), uint64(maxFontSourceBytes), &fontSourceBytes); err != nil {
				return preparedDisplayPlanPDF{}, err
			}
		}
		font, err := f.preflightPlanFontContext(ctx, resource, fontSources)
		if err != nil {
			return preparedDisplayPlanPDF{}, err
		}
		prepared.fonts[resource.ID] = font
		prepared.fontOrder = append(prepared.fontOrder, resource.ID)
	}
	lookupBudget, err := newPlannedImageLookupBudget(len(projection.ImageResources), f.imageSourceLimit())
	if err != nil {
		return preparedDisplayPlanPDF{}, err
	}
	var decodedTotal uint64
	for index, resource := range projection.ImageResources {
		if index&15 == 0 {
			if err := ctx.Err(); err != nil {
				return preparedDisplayPlanPDF{}, err
			}
		}
		encoded, err := lookupPlannedImageSourceContext(ctx, sources, resource.Digest, &lookupBudget)
		if err != nil {
			return preparedDisplayPlanPDF{}, err
		}
		pixels := uint64(resource.PixelWidth) * uint64(resource.PixelHeight)
		if pixels > ^uint64(0)/4 {
			return preparedDisplayPlanPDF{}, fmt.Errorf("%w: planned image decoded size overflows", errCoreLayoutPlanPaintUnsupported)
		}
		decoded := pixels * 4
		if decoded > uint64(f.imageDecodedLimit())-decodedTotal {
			return preparedDisplayPlanPDF{}, fmt.Errorf("%w: cumulative planned image decoded bytes exceed limit", errCoreLayoutPlanPaintUnsupported)
		}
		decodedTotal += decoded
		asset, err := f.preflightDisplayImageContext(ctx, resource, encoded)
		if err != nil {
			return preparedDisplayPlanPDF{}, err
		}
		prepared.images[resource.ID] = asset
		prepared.imageOrder = append(prepared.imageOrder, resource.ID)
	}
	for pageIndex, page := range projection.Pages {
		if pageIndex&31 == 0 {
			if err := ctx.Err(); err != nil {
				return preparedDisplayPlanPDF{}, err
			}
		}
		commandEnd := uint64(page.Commands.Start) + uint64(page.Commands.Count)
		if commandEnd > uint64(len(projection.Commands)) {
			return preparedDisplayPlanPDF{}, errors.New("document: display plan page has invalid command range")
		}
		pageOutput := preparedDisplayPage{page: page, commands: make([]preparedDisplayCommand, 0, page.Commands.Count)}
		for index := uint64(page.Commands.Start); index < commandEnd; index++ {
			command := projection.Commands[index]
			preparedCommand := preparedDisplayCommand{kind: command.Kind, semantic: append([]preparedDisplaySemantic(nil), semanticByFragment[command.Fragment]...)}
			switch command.Kind {
			case layoutengine.CommandSaveState, layoutengine.CommandRestoreState:
			case layoutengine.CommandTransform:
				preparedCommand.transform = projection.Transforms[command.Payload]
			case layoutengine.CommandClip:
				preparedCommand.clip = projection.Clips[command.Payload]
				preparedCommand.path = projection.Paths[preparedCommand.clip.Path]
			case layoutengine.CommandFillPath:
				preparedCommand.fill = projection.Fills[command.Payload]
				preparedCommand.path = projection.Paths[preparedCommand.fill.Path]
			case layoutengine.CommandStrokePath:
				preparedCommand.stroke = projection.Strokes[command.Payload]
				preparedCommand.path = projection.Paths[preparedCommand.stroke.Path]
			case layoutengine.CommandGlyphRun:
				preparedCommand.run = projection.GlyphRuns[command.Payload]
				preparedCommand.font = prepared.fonts[preparedCommand.run.Font]
			case layoutengine.CommandImage:
				preparedCommand.image = projection.Images[command.Payload]
				preparedCommand.asset = prepared.images[preparedCommand.image.Resource]
				crop, cropErr := f.preflightDisplayImageCrop(preparedCommand.image)
				if cropErr != nil {
					return preparedDisplayPlanPDF{}, cropErr
				}
				preparedCommand.crop = crop
			case layoutengine.CommandLink:
				preparedCommand.link = projection.Links[command.Payload]
				if preparedCommand.link.URI != "" {
					checked, checkErr := checkedExternalLinkTarget(preparedCommand.link.URI)
					if checkErr != nil || checked != preparedCommand.link.URI {
						if checkErr == nil {
							checkErr = errors.New("target is not canonical")
						}
						return preparedDisplayPlanPDF{}, fmt.Errorf("document: invalid planned external link target: %w", checkErr)
					}
				}
			default:
				return preparedDisplayPlanPDF{}, errors.New("document: display plan contains an unsupported command")
			}
			pageOutput.commands = append(pageOutput.commands, preparedCommand)
		}
		prepared.pages = append(prepared.pages, pageOutput)
	}
	return prepared, nil
}

func (f *Document) preflightDisplayImageCrop(image layoutengine.PlannedImage) (preparedDisplayImageCrop, error) {
	if image.Crop == nil {
		return preparedDisplayImageCrop{}, nil
	}
	destination := image.Crop.Clip
	source := image.Crop.Source
	intrinsic := image.Crop.Intrinsic
	destX := f.PointConvert(destination.X.Points())
	destY := f.PointConvert(destination.Y.Points())
	destW := f.PointConvert(destination.Width.Points())
	destH := f.PointConvert(destination.Height.Points())
	imageW := destW * float64(intrinsic.Width) / float64(source.Width)
	imageH := destH * float64(intrinsic.Height) / float64(source.Height)
	imageX := destX - destW*float64(source.X)/float64(source.Width)
	imageY := destY - destH*float64(source.Y)/float64(source.Height)
	if !finiteNumbers(destX, destY, destW, destH, imageX, imageY, imageW, imageH) ||
		destW <= 0 || destH <= 0 || imageW <= 0 || imageH <= 0 {
		return preparedDisplayImageCrop{}, fmt.Errorf("%w: invalid planned image crop transform", errCoreLayoutPlanPaintUnsupported)
	}
	return preparedDisplayImageCrop{
		enabled: true,
		clipX:   destX, clipY: destY, clipW: destW, clipH: destH,
		imageX: imageX, imageY: imageY, imageW: imageW, imageH: imageH,
	}, nil
}

func (f *Document) preflightDisplayImage(resource layoutengine.ImageResource, encoded []byte) (preparedDisplayImage, error) {
	return f.preflightDisplayImageContext(context.Background(), resource, encoded)
}

func (f *Document) preflightDisplayImageContext(ctx context.Context, resource layoutengine.ImageResource, encoded []byte) (preparedDisplayImage, error) {
	if len(encoded) == 0 {
		return preparedDisplayImage{}, fmt.Errorf("%w: image %s bytes are unavailable", errCoreLayoutPlanPaintUnsupported, resource.Digest)
	}
	digest := sha256.New()
	for offset := 0; offset < len(encoded); offset += 64 << 10 {
		if err := ctx.Err(); err != nil {
			return preparedDisplayImage{}, err
		}
		end := offset + (64 << 10)
		if end > len(encoded) {
			end = len(encoded)
		}
		_, _ = digest.Write(encoded[offset:end])
	}
	if hex.EncodeToString(digest.Sum(nil)) != string(resource.Digest) {
		return preparedDisplayImage{}, fmt.Errorf("%w: image %s content digest mismatch", errCoreLayoutPlanPaintUnsupported, resource.Digest)
	}
	imageType := string(resource.Format)
	if resource.Format == layoutengine.ImageJPEG {
		imageType = "jpg"
	}
	info, minVersion, err := parseImageOptionsReaderWithLimitsContext(ctx,
		ImageOptions{ImageType: imageType}, bytes.NewReader(encoded), f.k, f.compressLevel, f.pdfVersion,
		f.imageSourceLimit(), f.imageDecodedLimit(),
	)
	if err != nil {
		return preparedDisplayImage{}, fmt.Errorf("document: decode planned image %s: %w", resource.Digest, err)
	}
	if info.w != float64(resource.PixelWidth) || info.h != float64(resource.PixelHeight) {
		return preparedDisplayImage{}, fmt.Errorf("%w: image %s intrinsic dimensions mismatch", errCoreLayoutPlanPaintUnsupported, resource.Digest)
	}
	if info.i, err = generateImageID(info); err != nil {
		return preparedDisplayImage{}, fmt.Errorf("document: identify planned image %s: %w", resource.Digest, err)
	}
	key := "plan-image-" + string(resource.Digest)
	if f.resources != nil {
		if existing, exists := f.resources.image(key); exists && (existing == nil || existing.i != info.i) {
			return preparedDisplayImage{}, fmt.Errorf("%w: document image shadows %s", errCoreLayoutPlanPaintUnsupported, resource.Digest)
		}
	}
	return preparedDisplayImage{resource: resource, key: key, info: info, minVersion: minVersion}, nil
}
