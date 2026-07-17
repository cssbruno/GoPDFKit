// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cssbruno/gopdfkit/internal/paperlang"
)

const (
	studioReviewFormatVersion = 1
	studioReviewBodyLimit     = 4096
	studioReviewArtifactLimit = 100_000
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
	Kind       string    `json:"kind"`
	Digest     string    `json:"digest"`
	Page       uint32    `json:"page"`
	Width      uint32    `json:"width"`
	Height     uint32    `json:"height"`
	Transform  []float64 `json:"transform"`
	Calibrated bool      `json:"calibrated"`
	CreatedAt  string    `json:"created_at"`
}

type studioReviewSidecar struct {
	FormatVersion uint16                   `json:"format_version"`
	Annotations   []studioReviewAnnotation `json:"annotations,omitempty"`
	Comments      []studioReviewComment    `json:"comments,omitempty"`
	Reference     *studioReviewReference   `json:"reference,omitempty"`
}

type studioReviewResponse struct {
	FormatVersion  uint16                   `json:"format_version"`
	Revision       string                   `json:"revision"`
	SourceRevision string                   `json:"source_revision"`
	PlanHash       string                   `json:"plan_hash"`
	Scenario       string                   `json:"scenario,omitempty"`
	Annotations    []studioReviewAnnotation `json:"annotations"`
	Comments       []studioReviewComment    `json:"comments"`
	Reference      *studioReviewReference   `json:"reference,omitempty"`
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
		writeStudioJSON(w, http.StatusOK, projectStudioReview(snapshot, review))
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
	if err := applyReviewMutation(snapshot, &review, mutation); err != nil {
		writeStudioError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.writeReviewSidecar(review); err != nil {
		writeStudioError(w, http.StatusInternalServerError, err)
		return
	}
	writeStudioJSON(w, http.StatusOK, projectStudioReview(snapshot, review))
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

func applyReviewMutation(snapshot *studioSnapshot, review *studioReviewSidecar, mutation studioReviewMutation) error {
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
	review.Reference = &studioReviewReference{Kind: mutation.ReferenceKind, Digest: mutation.ReferenceDigest, Page: mutation.Page, Width: uint32(mutation.Width), Height: uint32(mutation.Height), Transform: append([]float64(nil), mutation.Transform...), Calibrated: true, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	_ = snapshot
	return nil
}

func newReviewID(kind, target string) (string, error) {
	var entropy [16]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return "", fmt.Errorf("paper-studio: create review identity: %w", err)
	}
	hash := sha256.Sum256(append([]byte(kind+":"+target+":"), entropy[:]...))
	return "review-" + hex.EncodeToString(hash[:12]), nil
}

func projectStudioReview(snapshot *studioSnapshot, review studioReviewSidecar) studioReviewResponse {
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
	return studioReviewResponse{FormatVersion: studioReviewFormatVersion, Revision: snapshot.revision, SourceRevision: studioSourceRevision(snapshot.source), PlanHash: snapshot.plan.Hash(), Scenario: snapshot.scenario, Annotations: projected.Annotations, Comments: projected.Comments, Reference: projected.Reference}
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
