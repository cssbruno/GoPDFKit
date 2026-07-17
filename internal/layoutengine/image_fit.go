// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/bits"
	"strconv"
)

var (
	ErrImageIntrinsicMissing = errors.New("layoutengine: image intrinsic dimensions are missing")
	ErrImageDimensionInvalid = errors.New("layoutengine: image dimensions are invalid")
	ErrImageFitPolicyInvalid = errors.New("layoutengine: image fit policy is invalid")
	ErrImageAlignmentInvalid = errors.New("layoutengine: image alignment is invalid")
	ErrImageFitWorkLimit     = errors.New("layoutengine: image fitting work limit exceeded")
	ErrImageFitLimitsInvalid = errors.New("layoutengine: image fitting limits are invalid")
	ErrImageFitStateLimit    = errors.New("layoutengine: image fitting state limit exceeded")
)

const (
	hardMaxImageFitDimension Fixed  = 1 << 52
	hardMaxImageFitWork      uint64 = 64
)

// ImageLengthKind distinguishes an intrinsic, aspect-preserving automatic
// dimension from an exact authored dimension.
type ImageLengthKind string

const (
	ImageLengthAuto  ImageLengthKind = "auto"
	ImageLengthFixed ImageLengthKind = "fixed"
)

// ImageLength is one image box dimension. Auto must carry a zero Value;
// Fixed must carry a positive fixed-point Value.
type ImageLength struct {
	Kind  ImageLengthKind `json:"kind"`
	Value Fixed           `json:"value,omitempty"`
}

// ImageFitPolicy controls how intrinsic image content is mapped into its
// resolved image box. Scale-down chooses none when the intrinsic image fits
// and contain otherwise.
type ImageFitPolicy string

const (
	ImageFitContain   ImageFitPolicy = "contain"
	ImageFitCover     ImageFitPolicy = "cover"
	ImageFitFill      ImageFitPolicy = "fill"
	ImageFitNone      ImageFitPolicy = "none"
	ImageFitScaleDown ImageFitPolicy = "scale-down"
)

// ImageAxisAlignment controls deterministic placement of unused space or
// selection of cropped content. Center assigns an odd remainder to the end.
type ImageAxisAlignment string

const (
	ImageAlignStart  ImageAxisAlignment = "start"
	ImageAlignCenter ImageAxisAlignment = "center"
	ImageAlignEnd    ImageAxisAlignment = "end"
)

type ImageAlignment struct {
	Horizontal ImageAxisAlignment `json:"horizontal"`
	Vertical   ImageAxisAlignment `json:"vertical"`
}

// ImageFitLimits bound retained geometry and deterministic arithmetic work.
// A zero value selects DefaultImageFitLimits; partially zero limits are
// rejected so a caller cannot accidentally disable a bound.
type ImageFitLimits struct {
	MaxDimension Fixed  `json:"max_dimension"`
	MaxWork      uint64 `json:"max_work"`
}

func DefaultImageFitLimits() ImageFitLimits {
	return ImageFitLimits{MaxDimension: hardMaxImageFitDimension, MaxWork: hardMaxImageFitWork}
}

// ImageFitInput contains fixed geometry only. Intrinsic dimensions use the
// image's natural coordinate space; SourceCrop in the result uses the same
// space. Position is the resolved box's top-left page-space coordinate.
type ImageFitInput struct {
	Resource  ImageResourceID
	Fragment  FragmentID
	Node      NodeID
	Key       NodeKey
	Instance  InstanceID
	Source    SourceSpan
	Position  Point
	Intrinsic Size
	Width     ImageLength
	Height    ImageLength
	Fit       ImageFitPolicy
	Alignment ImageAlignment
}

// ImageFitResult is the canonical bridge between sizing and painting. Box is
// the resolved image layout box. ObjectBounds is the image before clipping.
// Destination is the visible page-space rectangle. SourceCrop is the matching
// rectangle in intrinsic coordinates. Results that require no crop can be
// passed to existing display-list construction; crop-aware painters consume
// SourceCrop for the remaining results.
type ImageFitResult struct {
	Resource     ImageResourceID `json:"resource"`
	Fragment     FragmentID      `json:"fragment"`
	Node         NodeID          `json:"node"`
	Key          NodeKey         `json:"key"`
	Instance     InstanceID      `json:"instance"`
	Source       SourceSpan      `json:"source"`
	Intrinsic    Size            `json:"intrinsic"`
	Box          Rect            `json:"box"`
	ObjectBounds Rect            `json:"object_bounds"`
	Destination  Rect            `json:"destination"`
	SourceCrop   Rect            `json:"source_crop"`
	RequestedFit ImageFitPolicy  `json:"requested_fit"`
	EffectiveFit ImageFitPolicy  `json:"effective_fit"`
	RequiresCrop bool            `json:"requires_crop"`
	Work         uint64          `json:"work"`
}

// PlannedImage returns an exact crop-aware placement. Its optional crop
// payload remains zero only for placements authored through the legacy API.
func (result ImageFitResult) PlannedImage() PlannedImage {
	return PlannedImage{
		Resource: result.Resource, Fragment: result.Fragment,
		Bounds: result.Destination,
		Crop: &ImageCrop{
			Intrinsic: result.Intrinsic, Source: result.SourceCrop, Clip: result.Destination,
		},
		Source: result.Source,
	}
}

type imageFitBudget struct {
	ctx      context.Context
	limit    uint64
	used     uint64
	location DiagnosticLocation
}

func (budget *imageFitBudget) charge(amount uint64) error {
	if err := ChargePlanningWork(budget.ctx, "image fitting", amount); err != nil {
		return err
	}
	if err := budget.ctx.Err(); err != nil {
		return newPlanningError(err, Diagnostic{
			Code: DiagnosticCanceled, Severity: SeverityError, Stage: StageLayout,
			Message: "image fitting was canceled", Location: budget.location,
		})
	}
	if amount > budget.limit-budget.used {
		return newPlanningError(ErrImageFitWorkLimit, Diagnostic{
			Code: DiagnosticWorkLimit, Severity: SeverityError, Stage: StageLayout,
			Message: "image fitting exceeded its deterministic work limit", Location: budget.location,
			Evidence: []DiagnosticEvidence{
				{Key: "work_limit", Value: strconv.FormatUint(budget.limit, 10)},
				{Key: "work_used", Value: strconv.FormatUint(budget.used, 10)},
				{Key: "work_requested", Value: strconv.FormatUint(amount, 10)},
			},
		})
	}
	budget.used += amount
	return nil
}

// FitImage deterministically resolves automatic dimensions and object fitting
// without floating-point arithmetic. All final coordinates remain Fixed.
func FitImage(ctx context.Context, input ImageFitInput, limits ImageFitLimits) (ImageFitResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	limits, err := normalizeImageFitLimits(limits)
	if err != nil {
		return ImageFitResult{}, err
	}
	location := imageFitDiagnosticLocation(input)
	budget := imageFitBudget{ctx: ctx, limit: limits.MaxWork, location: location}
	if err := budget.charge(1); err != nil {
		return ImageFitResult{}, err
	}
	if err := validateImageFitInput(input, limits, location); err != nil {
		return ImageFitResult{}, err
	}
	if err := budget.charge(1); err != nil {
		return ImageFitResult{}, err
	}
	boxSize, err := resolveImageBox(input.Intrinsic, input.Width, input.Height)
	if err != nil {
		return ImageFitResult{}, imageDimensionError(err, "image box dimensions could not be resolved", location)
	}
	if boxSize.Width > limits.MaxDimension || boxSize.Height > limits.MaxDimension {
		return ImageFitResult{}, imageStateLimitError(input, limits, "resolved image box exceeds the dimension limit", location)
	}
	box, err := NewRect(input.Position.X, input.Position.Y, boxSize.Width, boxSize.Height)
	if err != nil {
		return ImageFitResult{}, imageDimensionError(err, "resolved image box is outside fixed geometry", location)
	}
	if err := budget.charge(1); err != nil {
		return ImageFitResult{}, err
	}
	object, destination, sourceCrop, effective, err := fitImageObject(box, input.Intrinsic, input.Fit, input.Alignment)
	if err != nil {
		return ImageFitResult{}, imageDimensionError(err, "image fit geometry could not be resolved", location)
	}
	if err := budget.charge(1); err != nil {
		return ImageFitResult{}, err
	}
	fullSource := Rect{Width: input.Intrinsic.Width, Height: input.Intrinsic.Height}
	return ImageFitResult{
		Resource: input.Resource, Fragment: input.Fragment, Node: input.Node,
		Key: input.Key, Instance: input.Instance, Source: input.Source, Intrinsic: input.Intrinsic,
		Box: box, ObjectBounds: object, Destination: destination, SourceCrop: sourceCrop,
		RequestedFit: input.Fit, EffectiveFit: effective,
		RequiresCrop: sourceCrop != fullSource || destination != object,
		Work:         budget.used,
	}, nil
}

func normalizeImageFitLimits(limits ImageFitLimits) (ImageFitLimits, error) {
	if limits == (ImageFitLimits{}) {
		return DefaultImageFitLimits(), nil
	}
	if limits.MaxDimension <= 0 || limits.MaxWork == 0 {
		return ImageFitLimits{}, fmt.Errorf("%w: all bounds must be positive", ErrImageFitLimitsInvalid)
	}
	if limits.MaxDimension > hardMaxImageFitDimension || limits.MaxWork > hardMaxImageFitWork {
		return ImageFitLimits{}, fmt.Errorf("%w: caller bounds exceed implementation hard caps", ErrImageFitLimitsInvalid)
	}
	return limits, nil
}

func validateImageFitInput(input ImageFitInput, limits ImageFitLimits, location DiagnosticLocation) error {
	if !input.Resource.Valid() {
		return newPlanningError(ErrImageIntrinsicMissing, Diagnostic{
			Code: DiagnosticImageMissing, Severity: SeverityError, Stage: StageLayout,
			Message: "image resource is missing", Location: location,
		})
	}
	if !input.Fragment.Valid() || !input.Node.Valid() || input.Key == "" || !input.Instance.Valid() {
		return imageDimensionError(ErrImageDimensionInvalid, "image semantic provenance is incomplete", location)
	}
	if err := validateTextIdentity("image fit node key", string(input.Key)); err != nil {
		return imageDimensionError(err, "image node key is invalid", location)
	}
	if err := validateTextIdentity("image fit instance", string(input.Instance)); err != nil {
		return imageDimensionError(err, "image instance is invalid", location)
	}
	if err := input.Source.Validate(); err != nil {
		return imageDimensionError(err, "image source provenance is invalid", location)
	}
	if input.Intrinsic.Width == 0 || input.Intrinsic.Height == 0 {
		return newPlanningError(ErrImageIntrinsicMissing, Diagnostic{
			Code: DiagnosticImageMissing, Severity: SeverityError, Stage: StageLayout,
			Message: "image intrinsic dimensions are missing", Location: location,
			Evidence: []DiagnosticEvidence{
				{Key: "intrinsic_width", Value: strconv.FormatInt(int64(input.Intrinsic.Width), 10)},
				{Key: "intrinsic_height", Value: strconv.FormatInt(int64(input.Intrinsic.Height), 10)},
			},
		})
	}
	if input.Intrinsic.Width < 0 || input.Intrinsic.Height < 0 {
		return imageDimensionError(ErrImageDimensionInvalid, "image intrinsic dimensions are invalid", location)
	}
	if input.Intrinsic.Width > limits.MaxDimension || input.Intrinsic.Height > limits.MaxDimension {
		return imageStateLimitError(input, limits, "image intrinsic dimensions exceed the dimension limit", location)
	}
	if err := validateImageLength(input.Width); err != nil {
		return imageDimensionError(err, "image width is invalid", location)
	}
	if err := validateImageLength(input.Height); err != nil {
		return imageDimensionError(err, "image height is invalid", location)
	}
	if input.Width.Value > limits.MaxDimension || input.Height.Value > limits.MaxDimension {
		return imageStateLimitError(input, limits, "explicit image dimensions exceed the dimension limit", location)
	}
	if !input.Fit.valid() {
		return newPlanningError(ErrImageFitPolicyInvalid, Diagnostic{
			Code: DiagnosticImageFitInvalid, Severity: SeverityError, Stage: StageLayout,
			Message: "image fit policy is invalid", Location: location,
		})
	}
	if !input.Alignment.Horizontal.valid() || !input.Alignment.Vertical.valid() {
		return newPlanningError(ErrImageAlignmentInvalid, Diagnostic{
			Code: DiagnosticImageFitInvalid, Severity: SeverityError, Stage: StageLayout,
			Message: "image alignment is invalid", Location: location,
		})
	}
	return nil
}

func validateImageLength(length ImageLength) error {
	switch length.Kind {
	case ImageLengthAuto:
		if length.Value != 0 {
			return ErrImageDimensionInvalid
		}
	case ImageLengthFixed:
		if length.Value <= 0 {
			return ErrImageDimensionInvalid
		}
	default:
		return ErrImageDimensionInvalid
	}
	return nil
}

func (policy ImageFitPolicy) valid() bool {
	switch policy {
	case ImageFitContain, ImageFitCover, ImageFitFill, ImageFitNone, ImageFitScaleDown:
		return true
	default:
		return false
	}
}

func (alignment ImageAxisAlignment) valid() bool {
	return alignment == ImageAlignStart || alignment == ImageAlignCenter || alignment == ImageAlignEnd
}

func resolveImageBox(intrinsic Size, width, height ImageLength) (Size, error) {
	switch {
	case width.Kind == ImageLengthAuto && height.Kind == ImageLengthAuto:
		return intrinsic, nil
	case width.Kind == ImageLengthFixed && height.Kind == ImageLengthFixed:
		return NewSize(width.Value, height.Value)
	case width.Kind == ImageLengthFixed:
		resolvedHeight, err := imageMulDivNearest(intrinsic.Height, width.Value, intrinsic.Width)
		if err != nil {
			return Size{}, err
		}
		return NewSize(width.Value, positiveImageExtent(resolvedHeight))
	default:
		resolvedWidth, err := imageMulDivNearest(intrinsic.Width, height.Value, intrinsic.Height)
		if err != nil {
			return Size{}, err
		}
		return NewSize(positiveImageExtent(resolvedWidth), height.Value)
	}
}

func fitImageObject(box Rect, intrinsic Size, policy ImageFitPolicy, alignment ImageAlignment) (Rect, Rect, Rect, ImageFitPolicy, error) {
	effective := policy
	if policy == ImageFitScaleDown {
		if intrinsic.Width <= box.Width && intrinsic.Height <= box.Height {
			effective = ImageFitNone
		} else {
			effective = ImageFitContain
		}
	}
	fullSource := Rect{Width: intrinsic.Width, Height: intrinsic.Height}
	switch effective {
	case ImageFitFill:
		return box, box, fullSource, effective, nil
	case ImageFitContain:
		size, err := containImageSize(box.Size(), intrinsic)
		if err != nil {
			return Rect{}, Rect{}, Rect{}, "", err
		}
		object, err := alignImageRect(box, size, alignment)
		return object, object, fullSource, effective, err
	case ImageFitCover:
		crop, err := coverImageCrop(box.Size(), intrinsic, alignment)
		if err != nil {
			return Rect{}, Rect{}, Rect{}, "", err
		}
		size, err := coverImageSize(box.Size(), intrinsic)
		if err != nil {
			return Rect{}, Rect{}, Rect{}, "", err
		}
		object, err := alignImageRect(box, size, alignment)
		return object, box, crop, effective, err
	case ImageFitNone:
		object, err := alignImageRect(box, intrinsic, alignment)
		if err != nil {
			return Rect{}, Rect{}, Rect{}, "", err
		}
		destination, err := box.Intersect(object)
		if err != nil {
			return Rect{}, Rect{}, Rect{}, "", err
		}
		if destination.IsEmpty() {
			return Rect{}, Rect{}, Rect{}, "", ErrImageDimensionInvalid
		}
		cropX, err := destination.X.Sub(object.X)
		if err != nil {
			return Rect{}, Rect{}, Rect{}, "", err
		}
		cropY, err := destination.Y.Sub(object.Y)
		if err != nil {
			return Rect{}, Rect{}, Rect{}, "", err
		}
		crop, err := NewRect(cropX, cropY, destination.Width, destination.Height)
		return object, destination, crop, effective, err
	default:
		return Rect{}, Rect{}, Rect{}, "", ErrImageFitPolicyInvalid
	}
}

func containImageSize(box, intrinsic Size) (Size, error) {
	comparison := compareImageProducts(box.Width, intrinsic.Height, box.Height, intrinsic.Width)
	if comparison <= 0 {
		height, err := imageMulDivFloor(intrinsic.Height, box.Width, intrinsic.Width)
		if err != nil {
			return Size{}, err
		}
		return NewSize(box.Width, positiveImageExtent(height))
	}
	width, err := imageMulDivFloor(intrinsic.Width, box.Height, intrinsic.Height)
	if err != nil {
		return Size{}, err
	}
	return NewSize(positiveImageExtent(width), box.Height)
}

func coverImageSize(box, intrinsic Size) (Size, error) {
	comparison := compareImageProducts(box.Width, intrinsic.Height, box.Height, intrinsic.Width)
	if comparison <= 0 {
		width, err := imageMulDivCeil(intrinsic.Width, box.Height, intrinsic.Height)
		if err != nil {
			return Size{}, err
		}
		return NewSize(positiveImageExtent(width), box.Height)
	}
	height, err := imageMulDivCeil(intrinsic.Height, box.Width, intrinsic.Width)
	if err != nil {
		return Size{}, err
	}
	return NewSize(box.Width, positiveImageExtent(height))
}

func coverImageCrop(box, intrinsic Size, alignment ImageAlignment) (Rect, error) {
	comparison := compareImageProducts(box.Width, intrinsic.Height, box.Height, intrinsic.Width)
	crop := Rect{Width: intrinsic.Width, Height: intrinsic.Height}
	var err error
	if comparison <= 0 {
		crop.Width, err = imageMulDivFloor(intrinsic.Height, box.Width, box.Height)
		if err != nil {
			return Rect{}, err
		}
		crop.Width = positiveImageExtent(crop.Width)
		if crop.Width > intrinsic.Width {
			crop.Width = intrinsic.Width
		}
		crop.X, err = imageAlignedOffset(intrinsic.Width, crop.Width, alignment.Horizontal)
	} else {
		crop.Height, err = imageMulDivFloor(intrinsic.Width, box.Height, box.Width)
		if err != nil {
			return Rect{}, err
		}
		crop.Height = positiveImageExtent(crop.Height)
		if crop.Height > intrinsic.Height {
			crop.Height = intrinsic.Height
		}
		crop.Y, err = imageAlignedOffset(intrinsic.Height, crop.Height, alignment.Vertical)
	}
	if err != nil {
		return Rect{}, err
	}
	if err := crop.Validate(); err != nil {
		return Rect{}, err
	}
	return crop, nil
}

func alignImageRect(box Rect, size Size, alignment ImageAlignment) (Rect, error) {
	dx, err := imageAlignedOffset(box.Width, size.Width, alignment.Horizontal)
	if err != nil {
		return Rect{}, err
	}
	dy, err := imageAlignedOffset(box.Height, size.Height, alignment.Vertical)
	if err != nil {
		return Rect{}, err
	}
	x, err := box.X.Add(dx)
	if err != nil {
		return Rect{}, err
	}
	y, err := box.Y.Add(dy)
	if err != nil {
		return Rect{}, err
	}
	return NewRect(x, y, size.Width, size.Height)
}

func imageAlignedOffset(container, object Fixed, alignment ImageAxisAlignment) (Fixed, error) {
	space, err := container.Sub(object)
	if err != nil {
		return 0, err
	}
	switch alignment {
	case ImageAlignStart:
		return 0, nil
	case ImageAlignCenter:
		return space.DivInt(2)
	case ImageAlignEnd:
		return space, nil
	default:
		return 0, ErrImageAlignmentInvalid
	}
}

func compareImageProducts(a, b, c, d Fixed) int {
	aHigh, aLow := bits.Mul64(uint64(a), uint64(b))
	bHigh, bLow := bits.Mul64(uint64(c), uint64(d))
	if aHigh < bHigh || (aHigh == bHigh && aLow < bLow) {
		return -1
	}
	if aHigh > bHigh || (aHigh == bHigh && aLow > bLow) {
		return 1
	}
	return 0
}

func imageMulDivFloor(a, b, divisor Fixed) (Fixed, error) {
	return imageMulDiv(a, b, divisor, false)
}

func imageMulDivNearest(a, b, divisor Fixed) (Fixed, error) {
	return imageMulDiv(a, b, divisor, true)
}

func imageMulDivCeil(a, b, divisor Fixed) (Fixed, error) {
	value, err := imageMulDivFloor(a, b, divisor)
	if err != nil {
		return 0, err
	}
	high, low := bits.Mul64(uint64(a), uint64(b))
	_, remainder := bits.Div64(high, low, uint64(divisor))
	if remainder == 0 {
		return value, nil
	}
	return value.Add(1)
}

func imageMulDiv(a, b, divisor Fixed, nearest bool) (Fixed, error) {
	if a < 0 || b < 0 || divisor <= 0 {
		return 0, ErrImageDimensionInvalid
	}
	high, low := bits.Mul64(uint64(a), uint64(b))
	if high >= uint64(divisor) {
		return 0, ErrGeometryOverflow
	}
	quotient, remainder := bits.Div64(high, low, uint64(divisor))
	if nearest && remainder*2 >= uint64(divisor) {
		if quotient == math.MaxInt64 {
			return 0, ErrGeometryOverflow
		}
		quotient++
	}
	if quotient > math.MaxInt64 {
		return 0, ErrGeometryOverflow
	}
	return Fixed(quotient), nil
}

func positiveImageExtent(value Fixed) Fixed {
	if value < 1 {
		return 1
	}
	return value
}

func imageFitDiagnosticLocation(input ImageFitInput) DiagnosticLocation {
	location := DiagnosticLocation{Node: input.Node, Fragment: input.Fragment}
	if validateTextIdentity("image fit node key", string(input.Key)) == nil {
		location.Key = input.Key
	}
	if validateTextIdentity("image fit instance", string(input.Instance)) == nil {
		location.Instance = input.Instance
	}
	if input.Source.Validate() == nil {
		location.Source = input.Source
	}
	return location
}

func imageDimensionError(cause error, message string, location DiagnosticLocation) error {
	return newPlanningError(cause, Diagnostic{
		Code: DiagnosticImageDimensionInvalid, Severity: SeverityError, Stage: StageLayout,
		Message: message, Location: location,
	})
}

func imageStateLimitError(input ImageFitInput, limits ImageFitLimits, message string, location DiagnosticLocation) error {
	return newPlanningError(ErrImageFitStateLimit, Diagnostic{
		Code: DiagnosticResourceLimit, Severity: SeverityError, Stage: StageLayout,
		Message: message, Location: location,
		Evidence: []DiagnosticEvidence{
			{Key: "intrinsic_width", Value: strconv.FormatInt(int64(input.Intrinsic.Width), 10)},
			{Key: "intrinsic_height", Value: strconv.FormatInt(int64(input.Intrinsic.Height), 10)},
			{Key: "max_dimension", Value: strconv.FormatInt(int64(limits.MaxDimension), 10)},
		},
	})
}
