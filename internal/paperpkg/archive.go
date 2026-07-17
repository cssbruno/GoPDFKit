// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperpkg

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"

	"golang.org/x/text/cases"
)

const (
	HardMaxArchiveCompressedBytes   uint64 = 1 << 30
	HardMaxArchiveUncompressedBytes uint64 = 2 << 30
	HardMaxArchiveFileBytes         uint64 = 512 << 20
	HardMaxArchiveFiles             uint32 = 65_534
	HardMaxArchivePathDepth         uint16 = 256
	HardMaxArchiveRatio             uint32 = 10_000
)

var (
	ErrArchiveInvalid     = errors.New("paperpkg: invalid package archive")
	ErrArchiveLimit       = errors.New("paperpkg: package archive limit exceeded")
	ErrArchivePath        = errors.New("paperpkg: invalid package archive path")
	ErrArchiveType        = errors.New("paperpkg: unsupported package archive file type")
	ErrArchiveCompression = errors.New("paperpkg: unsupported package archive compression")
	ErrArchiveIntegrity   = errors.New("paperpkg: package archive integrity check failed")
	ErrArchiveOverlap     = errors.New("paperpkg: package archive entries overlap")
)

type ArchiveLimits struct {
	MaxCompressedBytes   uint64
	MaxUncompressedBytes uint64
	MaxFileBytes         uint64
	MaxFiles             uint32
	MaxPathBytes         uint32
	MaxPathDepth         uint16
	MaxCompressionRatio  uint32
}

func DefaultArchiveLimits() ArchiveLimits {
	return ArchiveLimits{MaxCompressedBytes: 64 << 20, MaxUncompressedBytes: 256 << 20,
		MaxFileBytes: 64 << 20, MaxFiles: 10_000, MaxPathBytes: 1 << 10,
		MaxPathDepth: 64, MaxCompressionRatio: 1_000}
}

func (limits ArchiveLimits) validate() error {
	if limits.MaxCompressedBytes == 0 || limits.MaxUncompressedBytes == 0 || limits.MaxFileBytes == 0 ||
		limits.MaxFiles == 0 || limits.MaxPathBytes == 0 || limits.MaxPathDepth == 0 || limits.MaxCompressionRatio == 0 {
		return fmt.Errorf("%w: every archive limit must be positive", ErrArchiveLimit)
	}
	if limits.MaxCompressedBytes > HardMaxArchiveCompressedBytes || limits.MaxUncompressedBytes > HardMaxArchiveUncompressedBytes ||
		limits.MaxFileBytes > HardMaxArchiveFileBytes || limits.MaxFiles > HardMaxArchiveFiles ||
		limits.MaxPathBytes > HardMaxPathBytes || limits.MaxPathDepth > HardMaxArchivePathDepth ||
		limits.MaxCompressionRatio > HardMaxArchiveRatio || limits.MaxFileBytes > limits.MaxUncompressedBytes {
		return fmt.Errorf("%w: archive limit exceeds a hard cap or total", ErrArchiveLimit)
	}
	return nil
}

type ArchiveEntry struct {
	Path              string `json:"path"`
	CompressedBytes   uint64 `json:"compressed_bytes"`
	UncompressedBytes uint64 `json:"uncompressed_bytes"`
	Digest            Digest `json:"digest"`
	Bytes             []byte `json:"-"`
}

type ArchivePlan struct {
	Entries           []ArchiveEntry `json:"entries"`
	CompressedBytes   uint64         `json:"compressed_bytes"`
	UncompressedBytes uint64         `json:"uncompressed_bytes"`
}

func ValidateArchive(ctx context.Context, archive []byte, limits ArchiveLimits) (ArchivePlan, error) {
	return ValidateArchiveReaderAt(ctx, bytes.NewReader(archive), int64(len(archive)), limits)
}

func ValidateArchiveReaderAt(ctx context.Context, source io.ReaderAt, size int64, limits ArchiveLimits) (ArchivePlan, error) {
	if ctx == nil || source == nil || size < 0 {
		return ArchivePlan{}, fmt.Errorf("%w: context, reader, and non-negative size are required", ErrArchiveInvalid)
	}
	if err := ctx.Err(); err != nil {
		return ArchivePlan{}, err
	}
	if err := limits.validate(); err != nil {
		return ArchivePlan{}, err
	}
	if uint64(size) > limits.MaxCompressedBytes {
		return ArchivePlan{}, fmt.Errorf("%w: compressed archive exceeds %d bytes", ErrArchiveLimit, limits.MaxCompressedBytes)
	}
	reader, err := zip.NewReader(source, size)
	if err != nil {
		return ArchivePlan{}, fmt.Errorf("%w: %v", ErrArchiveInvalid, err)
	}
	if uint64(len(reader.File)) > uint64(limits.MaxFiles) {
		return ArchivePlan{}, fmt.Errorf("%w: too many files", ErrArchiveLimit)
	}
	metadata, err := parseZIPMetadata(ctx, source, size, reader.File)
	if err != nil {
		return ArchivePlan{}, err
	}

	files := append([]*zip.File(nil), reader.File...)
	seen := make(map[string]string, len(files))
	fold := cases.Fold()
	var totalCompressed, totalUncompressed uint64
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return ArchivePlan{}, err
		}
		if file.Flags&1 != 0 {
			return ArchivePlan{}, fmt.Errorf("%w: encrypted entry %q", ErrArchiveType, file.Name)
		}
		if file.Method != zip.Store && file.Method != zip.Deflate {
			return ArchivePlan{}, fmt.Errorf("%w: method %d for %q", ErrArchiveCompression, file.Method, file.Name)
		}
		if err := validateRelativePath(file.Name, limits.MaxPathBytes); err != nil {
			return ArchivePlan{}, fmt.Errorf("%w: %q: %v", ErrArchivePath, file.Name, err)
		}
		if depth := len(strings.Split(file.Name, "/")); depth > int(limits.MaxPathDepth) {
			return ArchivePlan{}, fmt.Errorf("%w: %q exceeds path depth", ErrArchiveLimit, file.Name)
		}
		if !file.Mode().IsRegular() {
			return ArchivePlan{}, fmt.Errorf("%w: %q has mode %v", ErrArchiveType, file.Name, file.Mode())
		}
		folded := fold.String(file.Name)
		if previous, duplicate := seen[folded]; duplicate {
			return ArchivePlan{}, fmt.Errorf("%w: %q collides with %q", ErrArchivePath, file.Name, previous)
		}
		seen[folded] = file.Name
		if file.UncompressedSize64 > limits.MaxFileBytes {
			return ArchivePlan{}, fmt.Errorf("%w: %q exceeds per-file bytes", ErrArchiveLimit, file.Name)
		}
		if file.UncompressedSize64 > 0 && file.CompressedSize64 == 0 {
			return ArchivePlan{}, fmt.Errorf("%w: %q has impossible compression sizes", ErrArchiveInvalid, file.Name)
		}
		if file.CompressedSize64 > 0 && file.UncompressedSize64 > file.CompressedSize64*uint64(limits.MaxCompressionRatio) {
			return ArchivePlan{}, fmt.Errorf("%w: %q exceeds compression ratio", ErrArchiveLimit, file.Name)
		}
		if totalCompressed > limits.MaxCompressedBytes-file.CompressedSize64 ||
			totalUncompressed > limits.MaxUncompressedBytes-file.UncompressedSize64 {
			return ArchivePlan{}, fmt.Errorf("%w: aggregate archive bytes", ErrArchiveLimit)
		}
		totalCompressed += file.CompressedSize64
		totalUncompressed += file.UncompressedSize64
	}
	_ = metadata
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	plan := ArchivePlan{Entries: make([]ArchiveEntry, 0, len(files)), CompressedBytes: totalCompressed, UncompressedBytes: totalUncompressed}
	for _, file := range files {
		content, digest, err := readArchiveEntry(ctx, file, limits.MaxFileBytes)
		if err != nil {
			return ArchivePlan{}, err
		}
		plan.Entries = append(plan.Entries, ArchiveEntry{Path: file.Name, CompressedBytes: file.CompressedSize64,
			UncompressedBytes: file.UncompressedSize64, Digest: digest, Bytes: content})
	}
	return plan, nil
}

type zipInterval struct{ start, end uint64 }

func parseZIPMetadata(ctx context.Context, source io.ReaderAt, size int64, files []*zip.File) ([]zipInterval, error) {
	if size < 22 {
		return nil, fmt.Errorf("%w: truncated end record", ErrArchiveInvalid)
	}
	tailSize := int64(22 + 65535)
	if tailSize > size {
		tailSize = size
	}
	tail := make([]byte, tailSize)
	if err := readArchiveAt(source, tail, size-tailSize); err != nil {
		return nil, fmt.Errorf("%w: read end record: %v", ErrArchiveInvalid, err)
	}
	signature := []byte{'P', 'K', 5, 6}
	relativeEOCD := bytes.LastIndex(tail, signature)
	if relativeEOCD < 0 || relativeEOCD+22 > len(tail) {
		return nil, fmt.Errorf("%w: missing end record", ErrArchiveInvalid)
	}
	eocd := tail[relativeEOCD:]
	commentLength := int(binary.LittleEndian.Uint16(eocd[20:22]))
	if relativeEOCD+22+commentLength != len(tail) || binary.LittleEndian.Uint16(eocd[4:6]) != 0 ||
		binary.LittleEndian.Uint16(eocd[6:8]) != 0 || binary.LittleEndian.Uint16(eocd[8:10]) != binary.LittleEndian.Uint16(eocd[10:12]) {
		return nil, fmt.Errorf("%w: unsupported multipart or malformed end record", ErrArchiveInvalid)
	}
	entryCount := uint32(binary.LittleEndian.Uint16(eocd[10:12]))
	centralSize := uint64(binary.LittleEndian.Uint32(eocd[12:16]))
	centralOffset := uint64(binary.LittleEndian.Uint32(eocd[16:20]))
	if entryCount == math.MaxUint16 || centralSize == math.MaxUint32 || centralOffset == math.MaxUint32 || int(entryCount) != len(files) {
		return nil, fmt.Errorf("%w: ZIP64 or entry count mismatch is unsupported", ErrArchiveInvalid)
	}
	absoluteEOCD := uint64(size-tailSize) + uint64(relativeEOCD)
	if centralOffset+centralSize != absoluteEOCD || centralOffset > uint64(size) {
		return nil, fmt.Errorf("%w: invalid central directory bounds", ErrArchiveInvalid)
	}
	central := make([]byte, centralSize)
	if err := readArchiveAt(source, central, int64(centralOffset)); err != nil {
		return nil, fmt.Errorf("%w: read central directory: %v", ErrArchiveInvalid, err)
	}
	intervals := make([]zipInterval, 0, len(files))
	cursor := 0
	for index, file := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if cursor+46 > len(central) || binary.LittleEndian.Uint32(central[cursor:cursor+4]) != 0x02014b50 {
			return nil, fmt.Errorf("%w: malformed central record %d", ErrArchiveInvalid, index)
		}
		nameLength := int(binary.LittleEndian.Uint16(central[cursor+28 : cursor+30]))
		extraLength := int(binary.LittleEndian.Uint16(central[cursor+30 : cursor+32]))
		commentLength := int(binary.LittleEndian.Uint16(central[cursor+32 : cursor+34]))
		recordEnd := cursor + 46 + nameLength + extraLength + commentLength
		if recordEnd > len(central) || string(central[cursor+46:cursor+46+nameLength]) != file.Name ||
			binary.LittleEndian.Uint16(central[cursor+8:cursor+10]) != file.Flags ||
			binary.LittleEndian.Uint16(central[cursor+10:cursor+12]) != file.Method {
			return nil, fmt.Errorf("%w: central metadata mismatch for entry %d", ErrArchiveInvalid, index)
		}
		localOffset := uint64(binary.LittleEndian.Uint32(central[cursor+42 : cursor+46]))
		if localOffset == math.MaxUint32 || localOffset+30 > centralOffset {
			return nil, fmt.Errorf("%w: invalid local header offset for %q", ErrArchiveInvalid, file.Name)
		}
		local := make([]byte, 30)
		if err := readArchiveAt(source, local, int64(localOffset)); err != nil || binary.LittleEndian.Uint32(local[:4]) != 0x04034b50 {
			return nil, fmt.Errorf("%w: invalid local header for %q", ErrArchiveInvalid, file.Name)
		}
		localNameLength := uint64(binary.LittleEndian.Uint16(local[26:28]))
		localExtraLength := uint64(binary.LittleEndian.Uint16(local[28:30]))
		dataStart := localOffset + 30 + localNameLength + localExtraLength
		dataEnd := dataStart + file.CompressedSize64
		if dataStart < localOffset || dataEnd < dataStart || dataEnd > centralOffset ||
			binary.LittleEndian.Uint16(local[6:8]) != file.Flags || binary.LittleEndian.Uint16(local[8:10]) != file.Method {
			return nil, fmt.Errorf("%w: invalid local metadata bounds for %q", ErrArchiveInvalid, file.Name)
		}
		localName := make([]byte, localNameLength)
		if err := readArchiveAt(source, localName, int64(localOffset+30)); err != nil || string(localName) != file.Name {
			return nil, fmt.Errorf("%w: local name mismatch for %q", ErrArchiveInvalid, file.Name)
		}
		intervalEnd := dataEnd
		if file.Flags&8 != 0 {
			descriptorPrefix := make([]byte, 4)
			if err := readArchiveAt(source, descriptorPrefix, int64(dataEnd)); err != nil {
				return nil, fmt.Errorf("%w: missing data descriptor for %q", ErrArchiveInvalid, file.Name)
			}
			descriptorBytes := uint64(12)
			if binary.LittleEndian.Uint32(descriptorPrefix) == 0x08074b50 {
				descriptorBytes = 16
			}
			intervalEnd += descriptorBytes
			if intervalEnd > centralOffset {
				return nil, fmt.Errorf("%w: invalid data descriptor for %q", ErrArchiveInvalid, file.Name)
			}
		}
		intervals = append(intervals, zipInterval{localOffset, intervalEnd})
		cursor = recordEnd
	}
	if cursor != len(central) {
		return nil, fmt.Errorf("%w: trailing central directory metadata", ErrArchiveInvalid)
	}
	sort.Slice(intervals, func(i, j int) bool { return intervals[i].start < intervals[j].start })
	for index := 1; index < len(intervals); index++ {
		if intervals[index].start < intervals[index-1].end {
			return nil, ErrArchiveOverlap
		}
	}
	return intervals, nil
}

func readArchiveAt(source io.ReaderAt, destination []byte, offset int64) error {
	read, err := source.ReadAt(destination, offset)
	if read != len(destination) {
		return io.ErrUnexpectedEOF
	}
	return err
}

func readArchiveEntry(ctx context.Context, file *zip.File, maxBytes uint64) ([]byte, Digest, error) {
	reader, err := file.Open()
	if err != nil {
		return nil, "", fmt.Errorf("%w: open %q: %v", ErrArchiveIntegrity, file.Name, err)
	}
	defer reader.Close()
	hasher := sha256.New()
	var output bytes.Buffer
	if file.UncompressedSize64 > 0 {
		output.Grow(int(file.UncompressedSize64))
	}
	buffer := make([]byte, 32<<10)
	var total uint64
	for {
		if err := ctx.Err(); err != nil {
			return nil, "", err
		}
		read, readErr := reader.Read(buffer)
		if read > 0 {
			if uint64(read) > maxBytes-total {
				return nil, "", fmt.Errorf("%w: %q expanded beyond its limit", ErrArchiveLimit, file.Name)
			}
			total += uint64(read)
			_, _ = hasher.Write(buffer[:read])
			_, _ = output.Write(buffer[:read])
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, "", fmt.Errorf("%w: read %q: %v", ErrArchiveIntegrity, file.Name, readErr)
		}
		if read == 0 {
			return nil, "", fmt.Errorf("%w: read %q: %v", ErrArchiveIntegrity, file.Name, io.ErrNoProgress)
		}
	}
	if total != file.UncompressedSize64 {
		return nil, "", fmt.Errorf("%w: %q size mismatch", ErrArchiveIntegrity, file.Name)
	}
	digest := Digest(hex.EncodeToString(hasher.Sum(nil)))
	return output.Bytes(), digest, nil
}
