// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/cssbruno/gopdfkit/document"
	"github.com/cssbruno/gopdfkit/internal/paperlang"
	"github.com/cssbruno/gopdfkit/internal/pdfverify"
)

const (
	studioReviewFormatVersion = 1
	studioReviewBodyLimit     = 4096
	studioReviewArtifactLimit = 100_000
	studioReviewMaxBytes      = 8 << 20
)

// Review metadata deliberately lives beside the source instead of inside the
// .paper CST. Formatting and semantic movement therefore cannot detach a
// comment or annotation from its authored @id, and source edits remain one
// readable semantic patch.
type studioReviewAnnotation struct {
	ID             string    `json:"id"`
	SourceRevision string    `json:"source_revision"`
	Target         string    `json:"target"`
	Page           uint32    `json:"page"`
	X              float64   `json:"x"`
	Y              float64   `json:"y"`
	Width          float64   `json:"width"`
	Height         float64   `json:"height"`
	Transform      []float64 `json:"transform"`
	Label          string    `json:"label,omitempty"`
	Note           string    `json:"note,omitempty"`
	Resolved       bool      `json:"resolved"`
}

type studioReviewComment struct {
	ID             string  `json:"id"`
	SourceRevision string  `json:"source_revision"`
	Target         string  `json:"target"`
	Page           uint32  `json:"page"`
	X              float64 `json:"x"`
	Y              float64 `json:"y"`
	Body           string  `json:"body"`
	Author         string  `json:"author,omitempty"`
	CreatedAt      string  `json:"created_at"`
	Resolved       bool    `json:"resolved"`
}

type studioReviewReference struct {
	Kind          string    `json:"kind"`
	Digest        string    `json:"digest"`
	Page          uint32    `json:"page"`
	Width         uint32    `json:"width"`
	Height        uint32    `json:"height"`
	Transform     []float64 `json:"transform"`
	Calibrated    bool      `json:"calibrated"`
	DiffStatus    string    `json:"diff_status,omitempty"`
	ChangedPixels uint64    `json:"changed_pixels"`
	DiffDigest    string    `json:"diff_digest,omitempty"`
	CreatedAt     string    `json:"created_at"`
}

type studioReviewSidecar struct {
	FormatVersion uint16                   `json:"format_version"`
	Annotations   []studioReviewAnnotation `json:"annotations,omitempty"`
	Comments      []studioReviewComment    `json:"comments,omitempty"`
	Reference     *studioReviewReference   `json:"reference,omitempty"`
}

type studioReviewResponse struct {
	FormatVersion  uint16                     `json:"format_version"`
	Revision       string                     `json:"revision"`
	SourceRevision string                     `json:"source_revision"`
	PlanHash       string                     `json:"plan_hash"`
	Scenario       string                     `json:"scenario,omitempty"`
	Accessibility  *studioReviewAccessibility `json:"accessibility,omitempty"`
	Annotations    []studioReviewAnnotation   `json:"annotations"`
	Comments       []studioReviewComment      `json:"comments"`
	Reference      *studioReviewReference     `json:"reference,omitempty"`
}

type studioReviewAccessibility struct {
	Status   string   `json:"status"`
	Evidence string   `json:"evidence"`
	Passed   bool     `json:"passed"`
	Failures []string `json:"failures,omitempty"`
}

type studioReviewMutation struct {
	SourceRevision  string    `json:"source_revision"`
	PlanRevision    string    `json:"plan_revision"`
	Scenario        string    `json:"scenario,omitempty"`
	Kind            string    `json:"kind"`
	Target          string    `json:"target,omitempty"`
	Page            uint32    `json:"page,omitempty"`
	X               float64   `json:"x,omitempty"`
	Y               float64   `json:"y,omitempty"`
	Width           float64   `json:"width,omitempty"`
	Height          float64   `json:"height,omitempty"`
	Transform       []float64 `json:"transform,omitempty"`
	Label           string    `json:"label,omitempty"`
	Note            string    `json:"note,omitempty"`
	Body            string    `json:"body,omitempty"`
	Author          string    `json:"author,omitempty"`
	ReferenceKind   string    `json:"reference_kind,omitempty"`
	ReferenceDigest string    `json:"reference_digest,omitempty"`
	ReferenceData   string    `json:"reference_data_base64,omitempty"`
}

func (s *studioServer) handleReview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), studioAPITimeout)
	defer cancel()
	snapshot, err := s.current(ctx, r.URL.Query().Get("scenario"))
	if err != nil {
		writeStudioError(w, http.StatusUnprocessableEntity, err)
		return
	}
	if r.Method == http.MethodGet {
		if err := validateReviewRevisions(snapshot, r.URL.Query().Get("revision"), r.URL.Query().Get("source_revision")); err != nil {
			writeStudioError(w, http.StatusConflict, err)
			return
		}
		review, err := s.readReviewSidecar()
		if err != nil {
			writeStudioError(w, http.StatusUnprocessableEntity, err)
			return
		}
		writeStudioJSON(w, http.StatusOK, projectStudioReview(ctx, snapshot, review))
		return
	}
	var mutation studioReviewMutation
	if err := decodeStudioJSON(r, &mutation); err != nil {
		writeStudioError(w, http.StatusBadRequest, err)
		return
	}
	if err := validateReviewMutation(snapshot, mutation); err != nil {
		writeStudioError(w, http.StatusBadRequest, err)
		return
	}
	s.reviewMu.Lock()
	defer s.reviewMu.Unlock()
	review, err := s.readReviewSidecar()
	if err != nil {
		writeStudioError(w, http.StatusUnprocessableEntity, err)
		return
	}
	if err := s.applyReviewMutation(ctx, snapshot, &review, mutation); err != nil {
		writeStudioError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.writeReviewSidecar(review); err != nil {
		writeStudioError(w, http.StatusInternalServerError, err)
		return
	}
	writeStudioJSON(w, http.StatusOK, projectStudioReview(ctx, snapshot, review))
}

func validateReviewRevisions(snapshot *studioSnapshot, revision, sourceRevision string) error {
	if snapshot == nil || revision == "" || sourceRevision == "" || revision != snapshot.revision || sourceRevision != studioSourceRevision(snapshot.source) {
		return errors.New("paper-studio: review metadata belongs to a stale source or plan")
	}
	return nil
}

func validateReviewMutation(snapshot *studioSnapshot, mutation studioReviewMutation) error {
	if mutation.SourceRevision != studioSourceRevision(snapshot.source) || mutation.PlanRevision != snapshot.revision {
		return errors.New("paper-studio: review mutation belongs to a stale source or plan")
	}
	if mutation.Scenario != snapshot.scenario {
		return errors.New("paper-studio: review mutation scenario is stale")
	}
	if mutation.Kind != "annotation" && mutation.Kind != "comment" && mutation.Kind != "reference" {
		return errors.New("paper-studio: review kind is outside the closed vocabulary")
	}
	if mutation.Kind == "reference" {
		return validateReviewReference(mutation)
	}
	if mutation.Target == "" || mutation.Target[0] != '@' || strings.ContainsAny(mutation.Target, " \t\r\n") {
		return errors.New("paper-studio: review target must be one readable authored @id")
	}
	parsed := paperlang.Parse(snapshot.file, snapshot.source)
	if !parsed.OK() {
		return errors.New("paper-studio: review target cannot be resolved from invalid source")
	}
	node, _ := studioSourceTarget(parsed.AST.Root, mutation.Target)
	if node == nil {
		return errors.New("paper-studio: review target does not exist in the exact source revision")
	}
	if mutation.Page == 0 || int(mutation.Page) > snapshot.pages {
		return errors.New("paper-studio: review page is outside the exact plan")
	}
	for _, value := range []float64{mutation.X, mutation.Y, mutation.Width, mutation.Height} {
		if !finiteReviewNumber(value) || math.Abs(value) > studioReviewArtifactLimit {
			return errors.New("paper-studio: review coordinates are outside the bounded finite range")
		}
	}
	if mutation.Width < 0 || mutation.Height < 0 {
		return errors.New("paper-studio: annotation dimensions cannot be negative")
	}
	if len(mutation.Transform) != 6 {
		return errors.New("paper-studio: review annotation requires a six-value affine transform")
	}
	for _, value := range mutation.Transform {
		if !finiteReviewNumber(value) || math.Abs(value) > studioReviewArtifactLimit {
			return errors.New("paper-studio: review transform is outside the bounded finite range")
		}
	}
	if len(mutation.Label) > 128 || len(mutation.Note) > studioReviewBodyLimit || len(mutation.Author) > 128 || len(mutation.Body) > studioReviewBodyLimit {
		return errors.New("paper-studio: review text exceeds its bound")
	}
	if mutation.Kind == "comment" && strings.TrimSpace(mutation.Body) == "" {
		return errors.New("paper-studio: comment body cannot be empty")
	}
	return nil
}

func validateReviewReference(mutation studioReviewMutation) error {
	if mutation.ReferenceKind != "image/png" && mutation.ReferenceKind != "image/jpeg" && mutation.ReferenceKind != "application/pdf" {
		return errors.New("paper-studio: reference must be a PNG, JPEG, or PDF artifact")
	}
	if len(mutation.ReferenceDigest) != sha256.Size*2 {
		return errors.New("paper-studio: reference digest is malformed")
	}
	if _, err := hex.DecodeString(mutation.ReferenceDigest); err != nil {
		return errors.New("paper-studio: reference digest is malformed")
	}
	if mutation.Page == 0 || mutation.Page > studioReviewArtifactLimit || mutation.Width == 0 || mutation.Height == 0 || mutation.Width > studioReviewArtifactLimit || mutation.Height > studioReviewArtifactLimit || math.Trunc(mutation.Width) != mutation.Width || math.Trunc(mutation.Height) != mutation.Height {
		return errors.New("paper-studio: reference dimensions are outside the bounded range")
	}
	if len(mutation.Transform) != 6 {
		return errors.New("paper-studio: calibrated reference requires a six-value affine transform")
	}
	for _, value := range mutation.Transform {
		if !finiteReviewNumber(value) || math.Abs(value) > studioReviewArtifactLimit {
			return errors.New("paper-studio: reference transform is outside the bounded finite range")
		}
	}
	return nil
}

func finiteReviewNumber(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func (s *studioServer) applyReviewMutation(ctx context.Context, snapshot *studioSnapshot, review *studioReviewSidecar, mutation studioReviewMutation) error {
	review.FormatVersion = studioReviewFormatVersion
	id, err := newReviewID(mutation.Kind, mutation.Target)
	if err != nil {
		return err
	}
	if mutation.Kind == "annotation" {
		review.Annotations = append(review.Annotations, studioReviewAnnotation{
			ID: id, SourceRevision: mutation.SourceRevision, Target: mutation.Target, Page: mutation.Page,
			X: mutation.X, Y: mutation.Y, Width: mutation.Width, Height: mutation.Height,
			Transform: append([]float64(nil), mutation.Transform...), Label: mutation.Label, Note: mutation.Note,
		})
		return nil
	}
	if mutation.Kind == "comment" {
		review.Comments = append(review.Comments, studioReviewComment{
			ID: id, SourceRevision: mutation.SourceRevision, Target: mutation.Target, Page: mutation.Page,
			X: mutation.X, Y: mutation.Y, Body: strings.TrimSpace(mutation.Body), Author: mutation.Author,
			CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		})
		return nil
	}
	reference := &studioReviewReference{Kind: mutation.ReferenceKind, Digest: mutation.ReferenceDigest, Page: mutation.Page, Width: uint32(mutation.Width), Height: uint32(mutation.Height), Transform: append([]float64(nil), mutation.Transform...), Calibrated: true, DiffStatus: "not-run", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if mutation.ReferenceData != "" {
		data, err := decodeReviewArtifact(mutation.ReferenceData)
		if err != nil {
			return err
		}
		actualDigest := sha256.Sum256(data)
		if hex.EncodeToString(actualDigest[:]) != mutation.ReferenceDigest {
			return errors.New("paper-studio: reference bytes do not match the declared digest")
		}
		if err := s.writeReviewArtifact(mutation.ReferenceDigest, data); err != nil {
			return err
		}
		if mutation.ReferenceKind == "application/pdf" {
			if !bytes.HasPrefix(data, []byte("%PDF-")) {
				return errors.New("paper-studio: PDF reference does not have a PDF signature")
			}
			diff, width, height, changed, err := s.diffReviewPDF(ctx, snapshot, mutation.Page, data)
			if err != nil {
				return err
			}
			reference.Width, reference.Height, reference.ChangedPixels = width, height, changed
			reference.DiffStatus = "verified"
			diffDigest := sha256.Sum256(diff)
			reference.DiffDigest = hex.EncodeToString(diffDigest[:])
			if err := s.writeReviewArtifact(reference.DiffDigest, diff); err != nil {
				return err
			}
		} else {
			diff, width, height, changed, err := s.diffReviewImage(ctx, snapshot, mutation.Page, data)
			if err != nil {
				return err
			}
			reference.Width, reference.Height, reference.ChangedPixels = width, height, changed
			reference.DiffStatus = "verified"
			diffDigest := sha256.Sum256(diff)
			reference.DiffDigest = hex.EncodeToString(diffDigest[:])
			if err := s.writeReviewArtifact(reference.DiffDigest, diff); err != nil {
				return err
			}
		}
	}
	review.Reference = reference
	return nil
}

func decodeReviewArtifact(encoded string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, errors.New("paper-studio: reference artifact is not valid base64")
	}
	if len(data) == 0 || len(data) > studioReviewMaxBytes {
		return nil, errors.New("paper-studio: reference artifact exceeds its bound")
	}
	return data, nil
}

func (s *studioServer) reviewArtifactPath(digest string) string {
	return filepath.Join(s.reviewSidecarPath()+".assets", digest+".bin")
}

func (s *studioServer) writeReviewArtifact(digest string, data []byte) error {
	if len(data) == 0 || len(data) > studioReviewMaxBytes || len(digest) != sha256.Size*2 {
		return errors.New("paper-studio: review artifact is outside its bound")
	}
	decoded, err := hex.DecodeString(digest)
	if err != nil || len(decoded) != sha256.Size {
		return errors.New("paper-studio: review artifact digest is malformed")
	}
	actual := sha256.Sum256(data)
	if !bytes.Equal(decoded, actual[:]) {
		return errors.New("paper-studio: review artifact digest does not match its bytes")
	}
	directory := filepath.Dir(s.reviewArtifactPath(digest))
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".review-artifact-*.tmp")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := io.Copy(temporary, bytes.NewReader(data)); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryName, s.reviewArtifactPath(digest))
}

func (s *studioServer) readReviewArtifact(digest string) ([]byte, error) {
	if len(digest) != sha256.Size*2 {
		return nil, errors.New("paper-studio: review artifact digest is malformed")
	}
	if _, err := hex.DecodeString(digest); err != nil {
		return nil, errors.New("paper-studio: review artifact digest is malformed")
	}
	data, err := os.ReadFile(s.reviewArtifactPath(digest))
	if err != nil {
		return nil, err
	}
	if len(data) > studioReviewMaxBytes {
		return nil, errors.New("paper-studio: review artifact exceeds its bound")
	}
	actual := sha256.Sum256(data)
	if hex.EncodeToString(actual[:]) != digest {
		return nil, errors.New("paper-studio: stored review artifact failed digest verification")
	}
	return data, nil
}

func (s *studioServer) diffReviewImage(ctx context.Context, snapshot *studioSnapshot, page uint32, reference []byte) ([]byte, uint32, uint32, uint64, error) {
	referenceImage, _, err := image.Decode(bytes.NewReader(reference))
	if err != nil {
		return nil, 0, 0, 0, fmt.Errorf("paper-studio: decode image reference: %w", err)
	}
	raster, err := snapshot.plan.CaptureRasterPages(ctx, document.DefaultPaperPlanRasterRequest())
	if err != nil {
		return nil, 0, 0, 0, fmt.Errorf("paper-studio: capture plan for reference diff: %w", err)
	}
	if page == 0 || int(page) > len(raster.Pages) {
		return nil, 0, 0, 0, errors.New("paper-studio: reference page is absent from the exact plan")
	}
	actualImage, err := png.Decode(bytes.NewReader(raster.Pages[page-1].PNG))
	if err != nil {
		return nil, 0, 0, 0, fmt.Errorf("paper-studio: decode exact plan raster: %w", err)
	}
	return diffReviewImages(ctx, actualImage, referenceImage)
}

func (s *studioServer) diffReviewPDF(ctx context.Context, snapshot *studioSnapshot, page uint32, reference []byte) ([]byte, uint32, uint32, uint64, error) {
	raster, err := snapshot.plan.CaptureRasterPages(ctx, document.DefaultPaperPlanRasterRequest())
	if err != nil {
		return nil, 0, 0, 0, fmt.Errorf("paper-studio: capture plan for PDF reference diff: %w", err)
	}
	if page == 0 || int(page) > len(raster.Pages) {
		return nil, 0, 0, 0, errors.New("paper-studio: PDF reference page is absent from the exact plan")
	}
	dimensions := make([]image.Point, len(raster.Pages))
	for index, planPage := range raster.Pages {
		config, _, err := image.DecodeConfig(bytes.NewReader(planPage.PNG))
		if err != nil {
			return nil, 0, 0, 0, fmt.Errorf("paper-studio: decode exact plan raster dimensions: %w", err)
		}
		dimensions[index] = image.Point{X: config.Width, Y: config.Height}
	}
	binary, err := exec.LookPath("pdftoppm")
	if err != nil {
		return nil, 0, 0, 0, errors.New("paper-studio: PDF reference diff requires pdftoppm")
	}
	limits := pdfverify.DefaultLimits()
	output, err := (pdfverify.PopplerRasterizer{
		Binary: binary, Version: "26.05.0", TempRoot: filepath.Dir(s.reviewSidecarPath()),
	}).Rasterize(ctx, reference, document.DefaultPaperPlanRasterRequest().DPI, dimensions, limits)
	if err != nil {
		return nil, 0, 0, 0, fmt.Errorf("paper-studio: rasterize PDF reference with pinned renderer: %w", err)
	}
	if int(page) > len(output.Pages) {
		return nil, 0, 0, 0, errors.New("paper-studio: PDF reference raster omitted the requested page")
	}
	actualImage, err := png.Decode(bytes.NewReader(raster.Pages[page-1].PNG))
	if err != nil {
		return nil, 0, 0, 0, fmt.Errorf("paper-studio: decode exact plan raster for PDF diff: %w", err)
	}
	referenceImage, err := png.Decode(bytes.NewReader(output.Pages[page-1]))
	if err != nil {
		return nil, 0, 0, 0, fmt.Errorf("paper-studio: decode rasterized PDF reference: %w", err)
	}
	diff, width, height, changed, err := diffReviewImages(ctx, actualImage, referenceImage)
	return diff, width, height, changed, err
}

func diffReviewImages(ctx context.Context, actualImage, referenceImage image.Image) ([]byte, uint32, uint32, uint64, error) {
	if referenceImage.Bounds().Dx() != actualImage.Bounds().Dx() || referenceImage.Bounds().Dy() != actualImage.Bounds().Dy() {
		return nil, 0, 0, 0, errors.New("paper-studio: reference image dimensions do not match the exact plan raster")
	}
	bounds := actualImage.Bounds()
	diff := image.NewRGBA(bounds)
	var changed uint64
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		if y&127 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, 0, 0, 0, err
			}
		}
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			expected := color.RGBAModel.Convert(referenceImage.At(x, y)).(color.RGBA)
			actual := color.RGBAModel.Convert(actualImage.At(x, y)).(color.RGBA)
			delta := maxReviewChannel(absReviewChannel(expected.R, actual.R), maxReviewChannel(absReviewChannel(expected.G, actual.G), absReviewChannel(expected.B, actual.B)))
			if delta != 0 {
				changed++
				diff.SetRGBA(x, y, color.RGBA{R: 255, G: 255 - delta/2, B: 0, A: 255})
			} else {
				diff.SetRGBA(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
			}
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, diff); err != nil {
		return nil, 0, 0, 0, err
	}
	return encoded.Bytes(), uint32(bounds.Dx()), uint32(bounds.Dy()), changed, nil
}

func absReviewChannel(left, right uint8) uint8 {
	if left > right {
		return left - right
	}
	return right - left
}

func maxReviewChannel(left, right uint8) uint8 {
	if left > right {
		return left
	}
	return right
}

func (s *studioServer) handleReviewReference(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), studioAPITimeout)
	defer cancel()
	snapshot, err := s.current(ctx, r.URL.Query().Get("scenario"))
	if err != nil {
		writeStudioError(w, http.StatusUnprocessableEntity, err)
		return
	}
	if err := validateReviewRevisions(snapshot, r.URL.Query().Get("revision"), r.URL.Query().Get("source_revision")); err != nil {
		writeStudioError(w, http.StatusConflict, err)
		return
	}
	review, err := s.readReviewSidecar()
	if err != nil || review.Reference == nil {
		if errors.Is(err, os.ErrNotExist) || review.Reference == nil {
			writeStudioError(w, http.StatusNotFound, errors.New("paper-studio: no calibrated review reference is retained"))
		} else {
			writeStudioError(w, http.StatusUnprocessableEntity, err)
		}
		return
	}
	digest := review.Reference.Digest
	contentType := review.Reference.Kind
	switch r.URL.Query().Get("artifact") {
	case "":
	case "diff":
		digest = review.Reference.DiffDigest
		if digest == "" {
			writeStudioError(w, http.StatusNotFound, errors.New("paper-studio: reference diff is not available"))
			return
		}
		contentType = "image/png"
	default:
		writeStudioError(w, http.StatusBadRequest, errors.New("paper-studio: unknown review reference artifact"))
		return
	}
	data, err := s.readReviewArtifact(digest)
	if err != nil {
		writeStudioError(w, http.StatusNotFound, err)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Paper-Plan-Revision", snapshot.revision)
	w.Header().Set("X-Paper-Source-Revision", studioSourceRevision(snapshot.source))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func newReviewID(kind, target string) (string, error) {
	var entropy [16]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return "", fmt.Errorf("paper-studio: create review identity: %w", err)
	}
	hash := sha256.Sum256(append([]byte(kind+":"+target+":"), entropy[:]...))
	return "review-" + hex.EncodeToString(hash[:12]), nil
}

func projectStudioReview(ctx context.Context, snapshot *studioSnapshot, review studioReviewSidecar) studioReviewResponse {
	projected := review
	projected.FormatVersion = studioReviewFormatVersion
	projected.Annotations = append([]studioReviewAnnotation(nil), review.Annotations...)
	projected.Comments = append([]studioReviewComment(nil), review.Comments...)
	parsed := paperlang.Parse(snapshot.file, snapshot.source)
	for index := range projected.Annotations {
		projected.Annotations[index].Resolved = reviewTargetExists(parsed.AST.Root, projected.Annotations[index].Target)
	}
	for index := range projected.Comments {
		projected.Comments[index].Resolved = reviewTargetExists(parsed.AST.Root, projected.Comments[index].Target)
	}
	return studioReviewResponse{FormatVersion: studioReviewFormatVersion, Revision: snapshot.revision, SourceRevision: studioSourceRevision(snapshot.source), PlanHash: snapshot.plan.Hash(), Scenario: snapshot.scenario, Accessibility: inspectReviewAccessibility(ctx, snapshot), Annotations: projected.Annotations, Comments: projected.Comments, Reference: projected.Reference}
}

func inspectReviewAccessibility(ctx context.Context, snapshot *studioSnapshot) *studioReviewAccessibility {
	result := &studioReviewAccessibility{Status: "unavailable", Evidence: "final_serialized_pdf"}
	if snapshot == nil || snapshot.pages == 0 {
		return result
	}
	pdf, err := renderStudioTaggedPDF(ctx, snapshot.plan)
	if err != nil {
		result.Failures = []string{"final PDF could not be serialized for accessibility review"}
		return result
	}
	report, err := pdfverify.InspectTags(ctx, pdf, pdfverify.TagInspectionLimits{})
	if err != nil {
		result.Failures = []string{"final PDF tag inspection was unavailable"}
		return result
	}
	result.Passed = report.Passed
	if report.Passed {
		result.Status = "verified"
	} else {
		result.Status = "failed"
		result.Failures = append([]string(nil), report.Failures...)
	}
	return result
}

func reviewTargetExists(root *paperlang.Node, target string) bool {
	if root == nil || target == "" {
		return false
	}
	node, _ := studioSourceTarget(root, target)
	return node != nil
}

func (s *studioServer) reviewSidecarPath() string { return filepath.Clean(s.file + ".review.json") }

func (s *studioServer) readReviewSidecar() (studioReviewSidecar, error) {
	data, err := os.ReadFile(s.reviewSidecarPath())
	if errors.Is(err, os.ErrNotExist) {
		return studioReviewSidecar{FormatVersion: studioReviewFormatVersion}, nil
	}
	if err != nil {
		return studioReviewSidecar{}, err
	}
	if len(data) > studioJSONLimit {
		return studioReviewSidecar{}, errors.New("paper-studio: review sidecar exceeds its bound")
	}
	var review studioReviewSidecar
	if err := json.Unmarshal(data, &review); err != nil {
		return studioReviewSidecar{}, fmt.Errorf("paper-studio: decode review sidecar: %w", err)
	}
	if review.FormatVersion != studioReviewFormatVersion {
		return studioReviewSidecar{}, errors.New("paper-studio: unsupported review sidecar version")
	}
	return review, nil
}

func (s *studioServer) writeReviewSidecar(review studioReviewSidecar) error {
	if review.FormatVersion == 0 {
		review.FormatVersion = studioReviewFormatVersion
	}
	data, err := json.MarshalIndent(review, "", "  ")
	if err != nil {
		return err
	}
	if len(data) > studioJSONLimit {
		return errors.New("paper-studio: review sidecar exceeds its bound")
	}
	dir := filepath.Dir(s.reviewSidecarPath())
	temporary, err := os.CreateTemp(dir, ".paper-studio-review-*.tmp")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryName, s.reviewSidecarPath())
}
