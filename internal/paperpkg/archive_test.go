// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperpkg

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"
)

func TestValidateArchiveReturnsDetachedSortedVerifiedPlan(t *testing.T) {
	archive := makeArchive(t,
		archiveFixture{path: "z-last.txt", content: []byte("last"), method: zip.Store},
		archiveFixture{path: "a/first.txt", content: []byte("first"), method: zip.Deflate},
	)
	plan, err := ValidateArchive(context.Background(), archive, DefaultArchiveLimits())
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Entries) != 2 || plan.Entries[0].Path != "a/first.txt" || plan.Entries[1].Path != "z-last.txt" ||
		string(plan.Entries[0].Bytes) != "first" || plan.Entries[0].Digest != shaDigest([]byte("first")) ||
		plan.UncompressedBytes != 9 || plan.CompressedBytes == 0 {
		t.Fatalf("archive plan = %#v", plan)
	}
	archive[0] ^= 0xff
	plan.Entries[0].Bytes[0] = 'X'
	if string(plan.Entries[1].Bytes) != "last" {
		t.Fatal("archive entries alias each other or source bytes")
	}

	original := makeArchive(t, archiveFixture{path: "reader.txt", content: []byte("reader"), method: zip.Store})
	fromReader, err := ValidateArchiveReaderAt(context.Background(), bytes.NewReader(original), int64(len(original)), DefaultArchiveLimits())
	if err != nil || string(fromReader.Entries[0].Bytes) != "reader" {
		t.Fatalf("ValidateArchiveReaderAt() = %#v, %v", fromReader, err)
	}
}

func TestValidateArchiveRejectsPathsDuplicatesCaseFoldAndDepth(t *testing.T) {
	for _, path := range []string{"../escape", "/absolute", `dir\file`, "dir/../file", "https://host/file", "dir//file"} {
		archive := makeArchive(t, archiveFixture{path: path, content: []byte("x"), method: zip.Store})
		if _, err := ValidateArchive(context.Background(), archive, DefaultArchiveLimits()); !errors.Is(err, ErrArchivePath) {
			t.Fatalf("path %q = %v", path, err)
		}
	}
	for _, fixtures := range [][]archiveFixture{
		{{path: "same.txt", content: []byte("a")}, {path: "same.txt", content: []byte("b")}},
		{{path: "Readme.txt", content: []byte("a")}, {path: "README.txt", content: []byte("b")}},
	} {
		if _, err := ValidateArchive(context.Background(), makeArchive(t, fixtures...), DefaultArchiveLimits()); !errors.Is(err, ErrArchivePath) {
			t.Fatalf("collision archive = %v", err)
		}
	}
	limits := DefaultArchiveLimits()
	limits.MaxPathDepth = 2
	if _, err := ValidateArchive(context.Background(), makeArchive(t, archiveFixture{path: "a/b/c.txt", content: []byte("x")}), limits); !errors.Is(err, ErrArchiveLimit) {
		t.Fatalf("depth-limited archive = %v", err)
	}
}

func TestValidateArchiveRejectsEncryptionTypesAndCompression(t *testing.T) {
	regular := makeArchive(t, archiveFixture{path: "file.txt", content: []byte("x"), method: zip.Store})
	encrypted := append([]byte(nil), regular...)
	mutateZIPHeaders(t, encrypted, func(local, central []byte) {
		binary.LittleEndian.PutUint16(local[6:8], binary.LittleEndian.Uint16(local[6:8])|1)
		binary.LittleEndian.PutUint16(central[8:10], binary.LittleEndian.Uint16(central[8:10])|1)
	})
	if _, err := ValidateArchive(context.Background(), encrypted, DefaultArchiveLimits()); !errors.Is(err, ErrArchiveType) {
		t.Fatalf("encrypted archive = %v", err)
	}

	unsupported := append([]byte(nil), regular...)
	mutateZIPHeaders(t, unsupported, func(local, central []byte) {
		binary.LittleEndian.PutUint16(local[8:10], 99)
		binary.LittleEndian.PutUint16(central[10:12], 99)
	})
	if _, err := ValidateArchive(context.Background(), unsupported, DefaultArchiveLimits()); !errors.Is(err, ErrArchiveCompression) {
		t.Fatalf("unsupported compression = %v", err)
	}

	for name, mode := range map[string]fs.FileMode{"directory/": os.ModeDir | 0o755, "symlink": os.ModeSymlink | 0o777, "device": os.ModeDevice | 0o600} {
		archive := makeArchive(t, archiveFixture{path: name, content: []byte("target"), method: zip.Store, mode: mode})
		if _, err := ValidateArchive(context.Background(), archive, DefaultArchiveLimits()); !errors.Is(err, ErrArchiveType) && !errors.Is(err, ErrArchivePath) {
			t.Fatalf("mode %v archive = %v", mode, err)
		}
	}
}

func TestValidateArchiveRejectsIntegrityAndMetadataCorruption(t *testing.T) {
	stored := makeArchive(t, archiveFixture{path: "file.txt", content: []byte("content"), method: zip.Store})
	reader, err := zip.NewReader(bytes.NewReader(stored), int64(len(stored)))
	if err != nil {
		t.Fatal(err)
	}
	offset, err := reader.File[0].DataOffset()
	if err != nil {
		t.Fatal(err)
	}
	crcBad := append([]byte(nil), stored...)
	crcBad[offset] ^= 1
	if plan, err := ValidateArchive(context.Background(), crcBad, DefaultArchiveLimits()); !errors.Is(err, ErrArchiveIntegrity) || len(plan.Entries) != 0 {
		t.Fatalf("CRC-corrupt archive = %#v, %v", plan, err)
	}

	if _, err := ValidateArchive(context.Background(), stored[:len(stored)-4], DefaultArchiveLimits()); !errors.Is(err, ErrArchiveInvalid) {
		t.Fatalf("truncated archive = %v", err)
	}

	overlap := makeArchive(t,
		archiveFixture{path: "same.txt", content: []byte("a"), method: zip.Store},
		archiveFixture{path: "same.txt", content: []byte("b"), method: zip.Store},
	)
	centrals := signatureOffsets(overlap, []byte{'P', 'K', 1, 2})
	if len(centrals) != 2 {
		t.Fatalf("central records = %v", centrals)
	}
	firstOffset := binary.LittleEndian.Uint32(overlap[centrals[0]+42 : centrals[0]+46])
	binary.LittleEndian.PutUint32(overlap[centrals[1]+42:centrals[1]+46], firstOffset)
	if _, err := ValidateArchive(context.Background(), overlap, DefaultArchiveLimits()); !errors.Is(err, ErrArchiveOverlap) {
		t.Fatalf("overlapping archive = %v", err)
	}

	metadataBad := append([]byte(nil), stored...)
	central := signatureOffsets(metadataBad, []byte{'P', 'K', 1, 2})[0]
	binary.LittleEndian.PutUint16(metadataBad[central+8:central+10], binary.LittleEndian.Uint16(metadataBad[central+8:central+10])^8)
	if _, err := ValidateArchive(context.Background(), metadataBad, DefaultArchiveLimits()); !errors.Is(err, ErrArchiveInvalid) {
		t.Fatalf("metadata-mismatch archive = %v", err)
	}
}

func TestValidateArchiveEnforcesCompressedUncompressedFileCountAndRatioLimits(t *testing.T) {
	archive := makeArchive(t, archiveFixture{path: "bomb.txt", content: []byte(strings.Repeat("A", 16<<10)), method: zip.Deflate})
	limits := DefaultArchiveLimits()
	limits.MaxCompressionRatio = 2
	if _, err := ValidateArchive(context.Background(), archive, limits); !errors.Is(err, ErrArchiveLimit) {
		t.Fatalf("ratio-limited archive = %v", err)
	}
	limits = DefaultArchiveLimits()
	limits.MaxCompressedBytes = uint64(len(archive) - 1)
	if _, err := ValidateArchive(context.Background(), archive, limits); !errors.Is(err, ErrArchiveLimit) {
		t.Fatalf("compressed-limited archive = %v", err)
	}
	limits = DefaultArchiveLimits()
	limits.MaxFileBytes = 10
	limits.MaxUncompressedBytes = 20
	if _, err := ValidateArchive(context.Background(), archive, limits); !errors.Is(err, ErrArchiveLimit) {
		t.Fatalf("file-limited archive = %v", err)
	}
	two := makeArchive(t, archiveFixture{path: "a", content: []byte("123456")}, archiveFixture{path: "b", content: []byte("123456")})
	limits = DefaultArchiveLimits()
	limits.MaxUncompressedBytes = 10
	limits.MaxFileBytes = 10
	if _, err := ValidateArchive(context.Background(), two, limits); !errors.Is(err, ErrArchiveLimit) {
		t.Fatalf("uncompressed-limited archive = %v", err)
	}
	limits = DefaultArchiveLimits()
	limits.MaxFiles = 1
	if _, err := ValidateArchive(context.Background(), two, limits); !errors.Is(err, ErrArchiveLimit) {
		t.Fatalf("file-count-limited archive = %v", err)
	}
	if _, err := ValidateArchive(context.Background(), two, ArchiveLimits{}); !errors.Is(err, ErrArchiveLimit) {
		t.Fatalf("zero limits = %v", err)
	}
}

func TestValidateArchiveHonorsCanceledContext(t *testing.T) {
	archive := makeArchive(t, archiveFixture{path: "file", content: []byte("content")})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if plan, err := ValidateArchive(ctx, archive, DefaultArchiveLimits()); !errors.Is(err, context.Canceled) || len(plan.Entries) != 0 {
		t.Fatalf("canceled archive = %#v, %v", plan, err)
	}
}

type archiveFixture struct {
	path    string
	content []byte
	method  uint16
	mode    fs.FileMode
}

func makeArchive(t *testing.T, fixtures ...archiveFixture) []byte {
	t.Helper()
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	for _, fixture := range fixtures {
		header := &zip.FileHeader{Name: fixture.path, Method: fixture.method}
		if header.Method == 0 {
			header.Method = zip.Store
		}
		if fixture.mode != 0 {
			header.SetMode(fixture.mode)
		}
		entry, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if fixture.mode&os.ModeDir == 0 {
			if _, err := entry.Write(fixture.content); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func mutateZIPHeaders(t *testing.T, archive []byte, mutate func(local, central []byte)) {
	t.Helper()
	locals := signatureOffsets(archive, []byte{'P', 'K', 3, 4})
	centrals := signatureOffsets(archive, []byte{'P', 'K', 1, 2})
	if len(locals) != 1 || len(centrals) != 1 {
		t.Fatalf("ZIP signatures local=%v central=%v", locals, centrals)
	}
	mutate(archive[locals[0]:], archive[centrals[0]:])
}

func signatureOffsets(data, signature []byte) []int {
	var offsets []int
	for start := 0; ; {
		index := bytes.Index(data[start:], signature)
		if index < 0 {
			return offsets
		}
		offsets = append(offsets, start+index)
		start += index + len(signature)
	}
}
