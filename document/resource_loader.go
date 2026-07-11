// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"
)

type ResourceKind string

const (
	ResourceImage      ResourceKind = "image"
	ResourceFont       ResourceKind = "font"
	ResourceAttachment ResourceKind = "attachment"
	ResourcePDFImport  ResourceKind = "pdf-import"
)

type ResourceInfo struct {
	Size    int64
	ModTime time.Time
	// StableID identifies the intrinsic resource bytes for cache reuse. Leave it
	// empty when the loader cannot guarantee the ID changes whenever content
	// changes.
	StableID string
}

type ResourceLoader interface {
	OpenResource(ctx context.Context, kind ResourceKind, name string) (io.ReadCloser, ResourceInfo, error)
}

type ResourceLoaderFunc func(context.Context, ResourceKind, string) (io.ReadCloser, ResourceInfo, error)

func (f ResourceLoaderFunc) OpenResource(ctx context.Context, kind ResourceKind, name string) (io.ReadCloser, ResourceInfo, error) {
	if f == nil {
		return nil, ResourceInfo{}, fmt.Errorf("resource loader is nil")
	}
	return f(ctx, kind, name)
}

type FileResourceLoader struct{}

func (FileResourceLoader) OpenResource(ctx context.Context, _ ResourceKind, name string) (io.ReadCloser, ResourceInfo, error) {
	if err := outputCanceledError(ctx); err != nil {
		return nil, ResourceInfo{}, err
	}
	file, err := os.Open(name) // #nosec G304 -- FileResourceLoader intentionally implements explicit file-path loading.
	if err != nil {
		return nil, ResourceInfo{}, err
	}
	info := ResourceInfo{Size: -1}
	if stat, statErr := file.Stat(); statErr == nil {
		info.ModTime = stat.ModTime()
		if stat.Mode().IsRegular() {
			info.Size = stat.Size()
		}
	}
	return file, info, nil
}

func (f *Document) SetResourceLoader(loader ResourceLoader) {
	f.resourceLoader = loader
}
