// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

const (
	WebDisplayRenderPayloadVersion  uint16 = 1
	WebDisplayRenderMaxPayloadBytes        = 96 << 20
)

var ErrWebDisplayRenderPayload = errors.New("layoutengine: invalid web display render payload")

// WebDisplayResourceBinding binds a plan resource identity to a deduplicated,
// content-addressed blob. Font identities are metrics digests; image
// identities are the image content digests retained by the display list.
type WebDisplayResourceBinding struct {
	Kind       string `json:"kind"`
	Identity   string `json:"identity"`
	BlobSHA256 string `json:"blob_sha256"`
}

// WebDisplayResourceBlob is one detached renderer input. JSON encodes Bytes as
// base64; SHA256 is checked before the bytes can reach a decoder or font parser.
type WebDisplayResourceBlob struct {
	SHA256 string `json:"sha256"`
	Bytes  []byte `json:"bytes"`
}

// WebDisplayRenderPayload is the transport-neutral contract consumed by the
// browser WASM renderer. Plan contains canonical LayoutPlan JSON, never source
// text or a browser-specific layout projection.
type WebDisplayRenderPayload struct {
	FormatVersion uint16                      `json:"format_version"`
	Renderer      string                      `json:"renderer"`
	PlanHash      string                      `json:"plan_hash"`
	Page          uint32                      `json:"page"`
	Plan          json.RawMessage             `json:"plan"`
	Profile       DisplayRasterProfile        `json:"profile"`
	Limits        DisplayRasterLimits         `json:"limits"`
	Revisions     ViewerRevisionIdentityInput `json:"revisions"`
	PageProfile   string                      `json:"page_profile"`
	Bindings      []WebDisplayResourceBinding `json:"bindings,omitempty"`
	Blobs         []WebDisplayResourceBlob    `json:"blobs,omitempty"`
}

// EncodeWebDisplayRenderPayload serializes a complete, immutable input for one
// browser render. Resource bytes are detached, deduplicated, and deterministically
// ordered. The renderer still performs full validation before painting.
func EncodeWebDisplayRenderPayload(plan LayoutPlan, sources DisplayRasterSources, request DisplayRasterRequest) ([]byte, error) {
	if err := plan.Validate(); err != nil {
		return nil, fmt.Errorf("%w: plan: %w", ErrWebDisplayRenderPayload, err)
	}
	if request.Page == 0 || uint64(request.Page) > uint64(len(plan.pages)) || request.Crop != nil {
		return nil, fmt.Errorf("%w: page is invalid or crop is unsupported", ErrWebDisplayRenderPayload)
	}
	if err := request.Profile.validate(); err != nil {
		return nil, err
	}
	if err := request.Limits.validate(); err != nil {
		return nil, err
	}
	if err := request.Revisions.validate(); err != nil {
		return nil, err
	}
	if err := validateDigestString(request.PageProfile); err != nil {
		return nil, fmt.Errorf("%w: page profile", ErrWebDisplayRenderPayload)
	}
	canonical, err := plan.CanonicalJSON()
	if err != nil {
		return nil, fmt.Errorf("%w: canonical plan: %w", ErrWebDisplayRenderPayload, err)
	}
	planHash := sha256.Sum256(canonical)
	bindings := make([]WebDisplayResourceBinding, 0, len(sources.FontPrograms)+len(sources.Images))
	blobByHash := make(map[string][]byte)
	bind := func(kind, identity string, value []byte) error {
		if len(value) == 0 {
			return fmt.Errorf("%w: empty %s resource %s", ErrWebDisplayRenderPayload, kind, identity)
		}
		digest := sha256.Sum256(value)
		blobHash := hex.EncodeToString(digest[:])
		bindings = append(bindings, WebDisplayResourceBinding{Kind: kind, Identity: identity, BlobSHA256: blobHash})
		if _, exists := blobByHash[blobHash]; !exists {
			blobByHash[blobHash] = append([]byte(nil), value...)
		}
		return nil
	}
	for identity, value := range sources.FontPrograms {
		if err := bind("font_program", string(identity), value); err != nil {
			return nil, err
		}
	}
	for identity, value := range sources.Images {
		if err := bind("image", string(identity), value); err != nil {
			return nil, err
		}
	}
	sort.Slice(bindings, func(i, j int) bool {
		if bindings[i].Kind != bindings[j].Kind {
			return bindings[i].Kind < bindings[j].Kind
		}
		return bindings[i].Identity < bindings[j].Identity
	})
	blobHashes := make([]string, 0, len(blobByHash))
	for hash := range blobByHash {
		blobHashes = append(blobHashes, hash)
	}
	sort.Strings(blobHashes)
	blobs := make([]WebDisplayResourceBlob, 0, len(blobHashes))
	for _, hash := range blobHashes {
		blobs = append(blobs, WebDisplayResourceBlob{SHA256: hash, Bytes: blobByHash[hash]})
	}
	payload := WebDisplayRenderPayload{
		FormatVersion: WebDisplayRenderPayloadVersion, Renderer: DisplayRasterRendererVersion,
		PlanHash: hex.EncodeToString(planHash[:]), Page: request.Page, Plan: canonical,
		Profile: request.Profile, Limits: request.Limits, Revisions: request.Revisions,
		PageProfile: request.PageProfile, Bindings: bindings, Blobs: blobs,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("%w: encode: %w", ErrWebDisplayRenderPayload, err)
	}
	if len(encoded) > WebDisplayRenderMaxPayloadBytes {
		return nil, fmt.Errorf("%w: payload exceeds %d bytes", ErrWebDisplayRenderPayload, WebDisplayRenderMaxPayloadBytes)
	}
	return encoded, nil
}

// RenderWebDisplayPayload validates and paints an encoded browser payload with
// the shared direct display-list rasterizer. It performs no layout operation.
func RenderWebDisplayPayload(ctx context.Context, encoded []byte) (DisplayRasterArtifact, error) {
	return renderWebDisplayPayload(ctx, encoded, nil)
}

// RenderWebDisplayPayloadCached validates and paints an encoded payload while
// reusing immutable decoded resources from the same plan identity. The cache
// never bypasses payload, digest, limit, or plan validation.
func RenderWebDisplayPayloadCached(ctx context.Context, encoded []byte, cache *WebDisplayRenderCache) (DisplayRasterArtifact, error) {
	return renderWebDisplayPayload(ctx, encoded, cache)
}

func renderWebDisplayPayload(ctx context.Context, encoded []byte, cache *WebDisplayRenderCache) (DisplayRasterArtifact, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(encoded) == 0 || len(encoded) > WebDisplayRenderMaxPayloadBytes {
		return DisplayRasterArtifact{}, fmt.Errorf("%w: byte length", ErrWebDisplayRenderPayload)
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var payload WebDisplayRenderPayload
	if err := decoder.Decode(&payload); err != nil {
		return DisplayRasterArtifact{}, fmt.Errorf("%w: decode: %w", ErrWebDisplayRenderPayload, err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return DisplayRasterArtifact{}, fmt.Errorf("%w: %w", ErrWebDisplayRenderPayload, err)
	}
	canonicalPayload, err := json.Marshal(payload)
	if err != nil || !bytes.Equal(canonicalPayload, encoded) {
		return DisplayRasterArtifact{}, fmt.Errorf("%w: payload is not canonical JSON", ErrWebDisplayRenderPayload)
	}
	if payload.FormatVersion != WebDisplayRenderPayloadVersion || payload.Renderer != DisplayRasterRendererVersion {
		return DisplayRasterArtifact{}, fmt.Errorf("%w: version or renderer mismatch", ErrWebDisplayRenderPayload)
	}
	if payload.Page == 0 || len(payload.Plan) == 0 {
		return DisplayRasterArtifact{}, fmt.Errorf("%w: page or plan is empty", ErrWebDisplayRenderPayload)
	}
	if err := payload.Profile.validate(); err != nil {
		return DisplayRasterArtifact{}, err
	}
	if err := payload.Limits.validate(); err != nil {
		return DisplayRasterArtifact{}, err
	}
	if err := payload.Revisions.validate(); err != nil {
		return DisplayRasterArtifact{}, err
	}
	if err := validateDigestString(payload.PageProfile); err != nil {
		return DisplayRasterArtifact{}, fmt.Errorf("%w: page profile", ErrWebDisplayRenderPayload)
	}
	if err := validateDigestString(payload.PlanHash); err != nil {
		return DisplayRasterArtifact{}, fmt.Errorf("%w: plan hash", ErrWebDisplayRenderPayload)
	}
	actualPlanHash := sha256.Sum256(payload.Plan)
	if hex.EncodeToString(actualPlanHash[:]) != payload.PlanHash {
		return DisplayRasterArtifact{}, fmt.Errorf("%w: plan hash mismatch", ErrWebDisplayRenderPayload)
	}
	cache.prepare(payload.PlanHash)
	plan, err := decodeStoredPlan(payload.Plan, PlanHash(actualPlanHash))
	if err != nil {
		return DisplayRasterArtifact{}, fmt.Errorf("%w: plan: %w", ErrWebDisplayRenderPayload, err)
	}
	blobs := make(map[string][]byte, len(payload.Blobs))
	var sourceBytes uint64
	for _, blob := range payload.Blobs {
		if err := validateDigestString(blob.SHA256); err != nil || len(blob.Bytes) == 0 {
			return DisplayRasterArtifact{}, fmt.Errorf("%w: resource blob", ErrWebDisplayRenderPayload)
		}
		if _, exists := blobs[blob.SHA256]; exists {
			return DisplayRasterArtifact{}, fmt.Errorf("%w: duplicate resource blob", ErrWebDisplayRenderPayload)
		}
		digest := sha256.Sum256(blob.Bytes)
		if hex.EncodeToString(digest[:]) != blob.SHA256 {
			return DisplayRasterArtifact{}, fmt.Errorf("%w: resource blob hash mismatch", ErrWebDisplayRenderPayload)
		}
		if uint64(len(blob.Bytes)) > payload.Limits.MaxSourceBytes-sourceBytes {
			return DisplayRasterArtifact{}, fmt.Errorf("%w: source byte limit", ErrWebDisplayRenderPayload)
		}
		sourceBytes += uint64(len(blob.Bytes))
		blobs[blob.SHA256] = blob.Bytes
	}
	sources := DisplayRasterSources{FontPrograms: make(map[CoreFontMetricsDigest][]byte), Images: make(DisplaySVGImageSources), cache: cache}
	seenBindings := make(map[string]bool, len(payload.Bindings))
	usedBlobs := make(map[string]bool, len(payload.Blobs))
	for _, binding := range payload.Bindings {
		key := binding.Kind + "\x00" + binding.Identity
		blob, exists := blobs[binding.BlobSHA256]
		if seenBindings[key] || !exists {
			return DisplayRasterArtifact{}, fmt.Errorf("%w: duplicate binding or missing blob", ErrWebDisplayRenderPayload)
		}
		seenBindings[key] = true
		usedBlobs[binding.BlobSHA256] = true
		switch binding.Kind {
		case "font_program":
			if err := CoreFontMetricsDigest(binding.Identity).validate(); err != nil {
				return DisplayRasterArtifact{}, fmt.Errorf("%w: font identity", ErrWebDisplayRenderPayload)
			}
			sources.FontPrograms[CoreFontMetricsDigest(binding.Identity)] = blob
		case "image":
			if err := ImageContentDigest(binding.Identity).validate(); err != nil {
				return DisplayRasterArtifact{}, fmt.Errorf("%w: image identity", ErrWebDisplayRenderPayload)
			}
			sources.Images[ImageContentDigest(binding.Identity)] = blob
		default:
			return DisplayRasterArtifact{}, fmt.Errorf("%w: resource kind", ErrWebDisplayRenderPayload)
		}
	}
	if len(usedBlobs) != len(blobs) {
		return DisplayRasterArtifact{}, fmt.Errorf("%w: unbound resource blob", ErrWebDisplayRenderPayload)
	}
	request := DisplayRasterRequest{Page: payload.Page, Profile: payload.Profile, Limits: payload.Limits,
		Revisions: payload.Revisions, PageProfile: payload.PageProfile}
	artifact, err := CaptureDisplayPlanPNGContext(ctx, plan, sources, request)
	if err != nil {
		return DisplayRasterArtifact{}, err
	}
	if artifact.manifest.PlanHash != payload.PlanHash || artifact.manifest.Page != payload.Page {
		return DisplayRasterArtifact{}, fmt.Errorf("%w: rendered identity mismatch", ErrWebDisplayRenderPayload)
	}
	return artifact, nil
}
