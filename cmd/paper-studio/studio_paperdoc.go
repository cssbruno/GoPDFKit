// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/cssbruno/paperrune/document"
	"github.com/cssbruno/paperrune/internal/paperdoc"
	"github.com/cssbruno/paperrune/internal/paperlang"
	"github.com/cssbruno/paperrune/internal/paperpkg"
)

func isStudioPaperDocument(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".paperdoc")
}

func (s *studioServer) loadPaperDocument() error {
	document, _, err := readStudioPaperDocument(s.file)
	if err != nil {
		return err
	}
	return s.setProjectResources(document.Resources)
}

func readStudioPaperDocumentSource(file string) (string, [32]byte, error) {
	document, _, err := readStudioPaperDocument(file)
	if err != nil {
		return "", [32]byte{}, err
	}
	source := []byte(document.Source)
	return document.Source, sha256.Sum256(source), nil
}

func readStudioPaperDocument(file string) (paperdoc.Document, [32]byte, error) {
	input, err := os.Open(file) // #nosec G304,G703 -- file is the explicit Studio document selected by the caller.
	if err != nil {
		return paperdoc.Document{}, [32]byte{}, err
	}
	defer func() { _ = input.Close() }()
	limit := paperpkg.DefaultArchiveLimits().MaxCompressedBytes
	encoded, err := io.ReadAll(io.LimitReader(input, int64(limit)+1)) // #nosec G115 -- the default limit is a small bounded constant.
	if err != nil {
		return paperdoc.Document{}, [32]byte{}, err
	}
	if uint64(len(encoded)) > limit {
		return paperdoc.Document{}, [32]byte{}, fmt.Errorf("paper-studio: Paper Document exceeds %d bytes", limit)
	}
	document, err := paperdoc.Decode(context.Background(), encoded)
	if err != nil {
		return paperdoc.Document{}, [32]byte{}, err
	}
	return document, sha256.Sum256(encoded), nil
}

func (s *studioServer) handleExportPaperDocument(w http.ResponseWriter, r *http.Request) {
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
	if revision := r.URL.Query().Get("revision"); revision != "" && revision != snapshot.revision {
		writeStudioError(w, http.StatusConflict, errors.New("paper-studio: stale Paper Document export revision"))
		return
	}
	imports, err := s.paperDocumentImports(snapshot)
	if err != nil {
		writeStudioError(w, http.StatusUnprocessableEntity, err)
		return
	}
	resources, _ := s.resourceSnapshot()
	encoded, err := paperdoc.Encode(paperdoc.Document{Source: snapshot.source, Imports: imports, Resources: resources})
	if err != nil {
		writeStudioError(w, http.StatusUnprocessableEntity, err)
		return
	}
	w.Header().Set("Content-Type", paperdoc.MediaType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+paperdoc.BaseName(snapshot.file)+`"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(encoded)
}

func writeStudioPaperDocumentSourceCAS(file string, expected [32]byte, source string) error {
	document, packageHash, err := readStudioPaperDocument(file)
	if err != nil {
		return err
	}
	if sha256.Sum256([]byte(document.Source)) != expected {
		return fmt.Errorf("%w: Paper Document changed before commit", errStudioStaleEdit)
	}
	document.Source = source
	encoded, err := paperdoc.Encode(document)
	if err != nil {
		return err
	}
	info, err := os.Stat(file)
	if err != nil {
		return err
	}
	directory := filepath.Dir(file)
	temporary, err := os.CreateTemp(directory, ".paper-studio-*.paperdoc")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer func() { _ = os.Remove(temporaryName) }()
	if err := temporary.Chmod(info.Mode().Perm()); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(encoded); err != nil {
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
	_, actualPackageHash, err := readStudioPaperDocument(file)
	if err != nil {
		return err
	}
	if actualPackageHash != packageHash {
		return fmt.Errorf("%w: Paper Document changed during commit", errStudioStaleEdit)
	}
	if err := os.Rename(temporaryName, file); err != nil {
		return err
	}
	if directoryHandle, openErr := os.Open(directory); openErr == nil { // #nosec G304 -- directory is the selected document's parent.
		_ = directoryHandle.Sync()
		_ = directoryHandle.Close()
	}
	return nil
}

func (s *studioServer) paperDocumentImports(snapshot *studioSnapshot) (map[string]string, error) {
	if isStudioPaperDocument(snapshot.file) {
		document, _, err := readStudioPaperDocument(snapshot.file)
		if err != nil {
			return nil, err
		}
		return document.Imports, nil
	}
	return collectStudioPaperImports(snapshot.file, snapshot.source)
}

func collectStudioPaperImports(rootFile, rootSource string) (map[string]string, error) {
	root := filepath.Dir(rootFile)
	imports := make(map[string]string)
	visiting := make(map[string]bool)
	var visit func(string, string) error
	visit = func(importerRelative, source string) error {
		for _, authored := range studioPaperImportPaths(paperlang.Parse(importerRelative, source).AST) {
			normalized := strings.ReplaceAll(authored, "\\", "/")
			relative := path.Clean(path.Join(path.Dir(importerRelative), normalized))
			if relative == "." || relative == ".." || strings.HasPrefix(relative, "../") || path.IsAbs(relative) || path.Ext(relative) != ".paper" {
				return fmt.Errorf("paper-studio: import %q cannot be embedded safely", authored)
			}
			if visiting[relative] {
				return fmt.Errorf("paper-studio: import cycle includes %q", relative)
			}
			if _, loaded := imports[relative]; loaded {
				continue
			}
			full := filepath.Join(root, filepath.FromSlash(relative))
			importSource, _, err := readStudioSource(full)
			if err != nil {
				return fmt.Errorf("paper-studio: embed import %q: %w", authored, err)
			}
			imports[relative] = importSource
			visiting[relative] = true
			if err := visit(relative, importSource); err != nil {
				return err
			}
			delete(visiting, relative)
		}
		return nil
	}
	if err := visit("document.paper", rootSource); err != nil {
		return nil, err
	}
	return imports, nil
}

func studioPaperImportPaths(ast paperlang.AST) []string {
	if ast.Root == nil {
		return nil
	}
	var paths []string
	for _, member := range ast.Root.Members {
		if member.Property != nil && member.Property.Name == "import" && member.Property.Value.StringValue != nil {
			paths = append(paths, *member.Property.Value.StringValue)
		}
	}
	return paths
}

func (s *studioServer) studioImportResolver() document.PaperImportResolver {
	if !isStudioPaperDocument(s.file) {
		return studioFileImportResolver()
	}
	packaged, _, loadErr := readStudioPaperDocument(s.file)
	prefix := s.file + "!/"
	return func(importerFile, importPath string) (string, string, error) {
		if loadErr != nil {
			return "", "", loadErr
		}
		importerRelative := "document.paper"
		if strings.HasPrefix(importerFile, prefix) {
			importerRelative = strings.TrimPrefix(importerFile, prefix)
		}
		relative := path.Clean(path.Join(path.Dir(importerRelative), strings.ReplaceAll(importPath, "\\", "/")))
		if relative == "." || relative == ".." || strings.HasPrefix(relative, "../") || path.IsAbs(relative) {
			return "", "", errors.New("paper-studio: packaged import path escapes the document")
		}
		source, ok := packaged.Imports[relative]
		if !ok {
			return "", "", fmt.Errorf("paper-studio: packaged import %q is missing", relative)
		}
		return prefix + relative, source, nil
	}
}
