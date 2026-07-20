// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package papercompile

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"sort"
	"strings"
)

const (
	MaxAssetCatalogResources = 256
	MaxAssetResourceBytes    = 512 << 10
	MaxAssetCatalogBytes     = 8 << 20
	MaxAssetDecodedPixels    = 64 << 20
)

var ErrAssetCatalog = errors.New("papercompile: invalid asset catalog")

// AssetResource is one explicitly supplied, content-addressed resource. Name
// is the human-readable part of an authored `asset:name` reference. Data is
// never loaded from an ambient path by the compiler.
type AssetResource struct {
	Name      string
	MediaType string
	Digest    string
	Data      []byte
	Family    string
	Style     string
	Weight    uint16
	License   string
}

// AssetCatalog is an immutable, canonical name-to-content boundary. Its zero
// value is a valid empty catalog.
type AssetCatalog struct {
	assets []AssetResource
}

// NewAssetCatalog validates, sorts, and detaches all caller-owned bytes.
func NewAssetCatalog(resources []AssetResource) (AssetCatalog, error) {
	if len(resources) > MaxAssetCatalogResources {
		return AssetCatalog{}, fmt.Errorf("%w: resource count %d exceeds %d", ErrAssetCatalog, len(resources), MaxAssetCatalogResources)
	}
	assets := make([]AssetResource, len(resources))
	var total uint64
	for index, resource := range resources {
		if !validAssetName(resource.Name) {
			return AssetCatalog{}, fmt.Errorf("%w: resources[%d].name is not a portable identifier", ErrAssetCatalog, index)
		}
		isImage := resource.MediaType == "image/png" || resource.MediaType == "image/jpeg"
		isFont := resource.MediaType == "font/ttf" || resource.MediaType == "font/otf"
		if !isImage && !isFont {
			return AssetCatalog{}, fmt.Errorf("%w: resources[%d].media_type must be image/png, image/jpeg, font/ttf, or font/otf", ErrAssetCatalog, index)
		}
		if len(resource.Data) == 0 || len(resource.Data) > MaxAssetResourceBytes {
			return AssetCatalog{}, fmt.Errorf("%w: resources[%d].data exceeds its bounded size", ErrAssetCatalog, index)
		}
		total += uint64(len(resource.Data))
		if total > MaxAssetCatalogBytes {
			return AssetCatalog{}, fmt.Errorf("%w: cumulative resource bytes exceed %d", ErrAssetCatalog, MaxAssetCatalogBytes)
		}
		digest := sha256.Sum256(resource.Data)
		actual := hex.EncodeToString(digest[:])
		if len(resource.Digest) != 64 || strings.ToLower(resource.Digest) != resource.Digest || resource.Digest != actual {
			return AssetCatalog{}, fmt.Errorf("%w: resources[%d].digest does not match its bytes", ErrAssetCatalog, index)
		}
		if isFont {
			if !validFontResource(resource) {
				return AssetCatalog{}, fmt.Errorf("%w: resources[%d] is not a bounded signed TrueType/OpenType font", ErrAssetCatalog, index)
			}
			assets[index] = AssetResource{Name: resource.Name, MediaType: resource.MediaType, Digest: actual, Data: append([]byte(nil), resource.Data...), Family: resource.Family, Style: resource.Style, Weight: resource.Weight, License: resource.License}
			continue
		}
		if resource.MediaType == "image/png" && !bytes.HasPrefix(resource.Data, []byte("\x89PNG\r\n\x1a\n")) {
			return AssetCatalog{}, fmt.Errorf("%w: resources[%d] does not have a PNG signature", ErrAssetCatalog, index)
		}
		if resource.MediaType == "image/jpeg" && (len(resource.Data) < 4 || resource.Data[0] != 0xff || resource.Data[1] != 0xd8 || resource.Data[len(resource.Data)-2] != 0xff || resource.Data[len(resource.Data)-1] != 0xd9) {
			return AssetCatalog{}, fmt.Errorf("%w: resources[%d] does not have a complete JPEG envelope", ErrAssetCatalog, index)
		}
		config, decodedFormat, decodeErr := image.DecodeConfig(bytes.NewReader(resource.Data))
		wantFormat := "png"
		if resource.MediaType == "image/jpeg" {
			wantFormat = "jpeg"
		}
		if decodeErr != nil || decodedFormat != wantFormat || config.Width <= 0 || config.Height <= 0 {
			return AssetCatalog{}, fmt.Errorf("%w: resources[%d] is not a decodable %s image", ErrAssetCatalog, index, wantFormat)
		}
		pixels := uint64(config.Width) * uint64(config.Height)
		if pixels > MaxAssetDecodedPixels {
			return AssetCatalog{}, fmt.Errorf("%w: resources[%d] decoded pixels exceed %d", ErrAssetCatalog, index, MaxAssetDecodedPixels)
		}
		assets[index] = AssetResource{Name: resource.Name, MediaType: resource.MediaType, Digest: actual, Data: append([]byte(nil), resource.Data...), Family: resource.Family, Style: resource.Style, Weight: resource.Weight, License: resource.License}
	}
	sort.Slice(assets, func(i, j int) bool { return assets[i].Name < assets[j].Name })
	for index := 1; index < len(assets); index++ {
		if assets[index-1].Name == assets[index].Name {
			return AssetCatalog{}, fmt.Errorf("%w: duplicate resource name %q", ErrAssetCatalog, assets[index].Name)
		}
	}
	return AssetCatalog{assets: assets}, nil
}

func (catalog AssetCatalog) Resolve(name string) (AssetResource, bool) {
	index := sort.Search(len(catalog.assets), func(index int) bool { return catalog.assets[index].Name >= name })
	if index == len(catalog.assets) || catalog.assets[index].Name != name {
		return AssetResource{}, false
	}
	resource := catalog.assets[index]
	resource.Data = append([]byte(nil), resource.Data...)
	return resource, true
}

// ResolveFont finds one explicit TrueType/OpenType resource by manifest name
// or family. Family lookup is deterministic and rejects ambiguous families.
func (catalog AssetCatalog) ResolveFont(name string) (AssetResource, bool) {
	var exact, family AssetResource
	for _, resource := range catalog.assets {
		if resource.MediaType != "font/ttf" && resource.MediaType != "font/otf" {
			continue
		}
		if resource.Name == name {
			if exact.Name != "" {
				return AssetResource{}, false
			}
			exact = resource
			continue
		}
		if resource.Family != name || family.Name != "" && fontResourceRank(resource) >= fontResourceRank(family) {
			continue
		}
		family = resource
	}
	match := exact
	if match.Name == "" {
		match = family
	}
	if match.Name == "" {
		return AssetResource{}, false
	}
	match.Data = append([]byte(nil), match.Data...)
	return match, true
}

func fontResourceRank(resource AssetResource) int {
	rank := 0
	weight := resource.Weight
	if weight == 0 {
		weight = 400
	}
	rank += int(weight)
	if resource.Style != "" && resource.Style != "normal" {
		rank += 10000
	}
	return rank
}

func (catalog AssetCatalog) Len() int { return len(catalog.assets) }

// FontResources returns detached, deterministic font resources for the
// document planner. Images are intentionally omitted from this projection.
func (catalog AssetCatalog) FontResources() []AssetResource {
	result := make([]AssetResource, 0)
	for _, resource := range catalog.assets {
		if resource.MediaType != "font/ttf" && resource.MediaType != "font/otf" {
			continue
		}
		resource.Data = append([]byte(nil), resource.Data...)
		result = append(result, resource)
	}
	return result
}

func validAssetName(name string) bool {
	if len(name) == 0 || len(name) > 128 || name[0] < 'a' || name[0] > 'z' {
		return false
	}
	for index := 1; index < len(name); index++ {
		character := name[index]
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '-' || character == '_' || character == '.' {
			continue
		}
		return false
	}
	return true
}

func validFontResource(resource AssetResource) bool {
	if len(resource.Family) == 0 || len(resource.Family) > 128 || resource.Weight > 1000 {
		return false
	}
	if resource.Weight == 0 {
		resource.Weight = 400
	}
	if resource.Style == "" {
		resource.Style = "normal"
	}
	if resource.Style != "normal" && resource.Style != "italic" && resource.Style != "oblique" {
		return false
	}
	if resource.MediaType == "font/ttf" {
		return bytes.HasPrefix(resource.Data, []byte{0, 1, 0, 0})
	}
	return bytes.HasPrefix(resource.Data, []byte("OTTO")) || bytes.HasPrefix(resource.Data, []byte("true")) || bytes.HasPrefix(resource.Data, []byte("typ1"))
}
