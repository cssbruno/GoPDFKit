// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"encoding/hex"
	"errors"
	"fmt"
)

type ImageResourceID uint32

func (id ImageResourceID) Valid() bool { return id != 0 }

type ImageFormat string

const (
	ImagePNG  ImageFormat = "png"
	ImageJPEG ImageFormat = "jpeg"
)

func (format ImageFormat) valid() bool { return format == ImagePNG || format == ImageJPEG }

type ImageContentDigest string

func (digest ImageContentDigest) validate() error {
	if len(digest) != 64 {
		return errors.New("image digest is not a lowercase SHA-256 digest")
	}
	decoded, err := hex.DecodeString(string(digest))
	if err != nil || hex.EncodeToString(decoded) != string(digest) {
		return errors.New("image digest is not a lowercase SHA-256 digest")
	}
	allZero := true
	for _, value := range decoded {
		allZero = allZero && value == 0
	}
	if allZero {
		return errors.New("image digest is zero")
	}
	return nil
}

// ImageResource records immutable intrinsic metadata for content-addressed
// encoded bytes. Painters never decode it to make layout decisions.
type ImageResource struct {
	ID          ImageResourceID    `json:"id"`
	Digest      ImageContentDigest `json:"digest"`
	Format      ImageFormat        `json:"format"`
	PixelWidth  uint32             `json:"pixel_width"`
	PixelHeight uint32             `json:"pixel_height"`
}

// ImageCrop is an optional crop-aware paint payload. Its zero value preserves
// the original full-image placement contract. Intrinsic and Source use one
// arbitrary but shared natural-image coordinate space; Clip is page-space and
// must equal the owning PlannedImage Bounds.
type ImageCrop struct {
	Intrinsic Size `json:"intrinsic"`
	Source    Rect `json:"source"`
	Clip      Rect `json:"clip"`
}

func (crop ImageCrop) IsZero() bool { return crop == (ImageCrop{}) }

// PlannedImage is one final page-space placement.
type PlannedImage struct {
	Resource ImageResourceID `json:"resource"`
	Fragment FragmentID      `json:"fragment"`
	Bounds   Rect            `json:"bounds"`
	Opacity  Fixed           `json:"opacity,omitempty"`
	Crop     *ImageCrop      `json:"crop,omitempty"`
	Source   SourceSpan      `json:"source"`
}

func validateImageCrop(path string, image PlannedImage, resource ImageResource) error {
	if image.Crop == nil {
		return nil
	}
	crop := *image.Crop
	if crop.IsZero() {
		return planError(path+".crop", "must be omitted instead of explicitly empty")
	}
	if err := crop.Intrinsic.Validate(); err != nil || crop.Intrinsic.IsEmpty() {
		return planError(path+".crop.intrinsic", "must have positive valid extents")
	}
	if err := crop.Source.Validate(); err != nil || crop.Source.IsEmpty() {
		return planError(path+".crop.source", "must have positive valid extents")
	}
	if err := crop.Clip.Validate(); err != nil || crop.Clip.IsEmpty() {
		return planError(path+".crop.clip", "must have positive valid extents")
	}
	if crop.Clip != image.Bounds {
		return planError(path+".crop.clip", "must equal the visible image bounds")
	}
	right, _ := crop.Source.Right()
	bottom, _ := crop.Source.Bottom()
	if crop.Source.X < 0 || crop.Source.Y < 0 ||
		right > crop.Intrinsic.Width || bottom > crop.Intrinsic.Height {
		return planError(path+".crop.source", "lies outside the intrinsic image")
	}
	if compareImageProducts(crop.Intrinsic.Width, Fixed(resource.PixelHeight),
		crop.Intrinsic.Height, Fixed(resource.PixelWidth)) != 0 {
		return planError(path+".crop.intrinsic", "aspect ratio does not match the image resource pixels")
	}
	return nil
}

func clonePlannedImages(images []PlannedImage) []PlannedImage {
	result := cloneSlice(images)
	for index := range result {
		result[index] = clonePlannedImage(result[index])
	}
	return result
}

func clonePlannedImage(image PlannedImage) PlannedImage {
	if image.Crop != nil {
		crop := *image.Crop
		image.Crop = &crop
	}
	return image
}

func validateImageResources(resources []ImageResource) error {
	digests := make(map[ImageContentDigest]bool, len(resources))
	for index, resource := range resources {
		path := fmt.Sprintf("image_resources[%d]", index)
		if resource.ID != ImageResourceID(index+1) {
			return planError(path, "image resource IDs are not consecutive and one-based")
		}
		if err := resource.Digest.validate(); err != nil {
			return planError(path+".digest", err.Error())
		}
		if digests[resource.Digest] {
			return planError(path, "duplicates an image content digest")
		}
		if !resource.Format.valid() {
			return planError(path+".format", "is not a supported encoded image format")
		}
		if resource.PixelWidth == 0 || resource.PixelHeight == 0 {
			return planError(path, "pixel dimensions must be positive")
		}
		digests[resource.Digest] = true
	}
	return nil
}

// AttachImages lowers exact placements into an image-only display list without
// invoking sizing or layout. Placements must already be ordered by owning page;
// their order within a page is their paint order.
func AttachImages(plan LayoutPlan, resources []ImageResource, images []PlannedImage) (LayoutPlan, error) {
	if err := plan.Validate(); err != nil {
		return LayoutPlan{}, err
	}
	projection := plan.Projection()
	if len(projection.Commands) != 0 || len(projection.Fonts) != 0 || len(projection.GlyphRuns) != 0 ||
		len(projection.ImageResources) != 0 || len(projection.Images) != 0 {
		return LayoutPlan{}, errors.New("layoutengine: image attachment requires a resource-free geometry plan")
	}
	if uint64(len(images)) > uint64(^uint32(0)) {
		return LayoutPlan{}, errors.New("layoutengine: planned image count exceeds plan index capacity")
	}
	fragmentPages := make(map[FragmentID]uint32, len(projection.Fragments))
	for _, fragment := range projection.Fragments {
		fragmentPages[fragment.ID] = fragment.Page
	}
	commands := make([]DisplayCommand, 0, len(images))
	pageCounts := make([]uint32, len(projection.Pages))
	var previousPage uint32
	for index, image := range images {
		page := fragmentPages[image.Fragment]
		if page == 0 || uint64(page) > uint64(len(pageCounts)) {
			return LayoutPlan{}, planError(fmt.Sprintf("images[%d].fragment", index), "has no owning page")
		}
		if previousPage > page {
			return LayoutPlan{}, planError(fmt.Sprintf("images[%d]", index), "placements are not in page paint order")
		}
		previousPage = page
		pageCounts[page-1]++
		commands = append(commands, DisplayCommand{
			Kind: CommandImage, Fragment: image.Fragment, Bounds: image.Bounds, Payload: uint32(index),
		})
	}
	var commandStart uint32
	for index := range projection.Pages {
		projection.Pages[index].Commands = IndexRange{Start: commandStart, Count: pageCounts[index]}
		commandStart += pageCounts[index]
	}
	result, err := NewLayoutPlan(LayoutPlanInput{
		Pages: projection.Pages, Fragments: projection.Fragments, Lines: projection.Lines,
		ImageResources: resources, Images: images, Commands: commands,
		Breaks: projection.Breaks, Diagnostics: projection.Diagnostics,
		SemanticNodes: projection.SemanticNodes, SemanticFragments: projection.SemanticFragments, ReadingOrder: projection.ReadingOrder,
	})
	if err != nil {
		return LayoutPlan{}, err
	}
	return rebindDeterministicResources(result, projection.DeterministicInputs)
}
