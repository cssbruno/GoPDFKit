// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperpkg

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
)

const (
	CacheManifestSchemaVersion uint16 = 1
	HardMaxCacheManifestBytes  uint64 = 16 << 20
	HardMaxCacheStateBytes     uint64 = 2 << 30
)

var (
	ErrCacheInvalid  = errors.New("paperpkg: invalid package cache input")
	ErrCacheLimit    = errors.New("paperpkg: package cache limit exceeded")
	ErrCacheCorrupt  = errors.New("paperpkg: package cache content is corrupt")
	ErrCacheMismatch = errors.New("paperpkg: package cache content digest mismatch")
	ErrCacheNotFound = errors.New("paperpkg: package cache item not found")
)

type CacheLimits struct {
	MaxManifestBytes uint64
	MaxStateBytes    uint64
	MaxFiles         uint32
	MaxFileBytes     uint64
	MaxTotalBytes    uint64
	MaxPathBytes     uint32
}

func DefaultCacheLimits() CacheLimits {
	return CacheLimits{MaxManifestBytes: 4 << 20, MaxStateBytes: 512 << 20, MaxFiles: 10_000,
		MaxFileBytes: 64 << 20, MaxTotalBytes: 256 << 20, MaxPathBytes: 1 << 10}
}

func (limits CacheLimits) validate() error {
	if limits.MaxManifestBytes == 0 || limits.MaxStateBytes == 0 || limits.MaxFiles == 0 ||
		limits.MaxFileBytes == 0 || limits.MaxTotalBytes == 0 || limits.MaxPathBytes == 0 {
		return fmt.Errorf("%w: every cache limit must be positive", ErrCacheLimit)
	}
	if limits.MaxManifestBytes > HardMaxCacheManifestBytes || limits.MaxStateBytes > HardMaxCacheStateBytes ||
		limits.MaxFiles > HardMaxArchiveFiles || limits.MaxFileBytes > HardMaxArchiveFileBytes ||
		limits.MaxTotalBytes > HardMaxArchiveUncompressedBytes || limits.MaxPathBytes > HardMaxPathBytes ||
		limits.MaxFileBytes > limits.MaxTotalBytes {
		return fmt.Errorf("%w: cache limit exceeds a hard cap or total", ErrCacheLimit)
	}
	return nil
}

type CacheFile struct {
	Path   string `json:"path"`
	Bytes  uint64 `json:"bytes"`
	Digest Digest `json:"digest"`
}

type CachePackage struct {
	ProjectDigest Digest      `json:"project_digest"`
	ContentDigest Digest      `json:"content_digest"`
	Files         []CacheFile `json:"files"`
}

type cacheManifest struct {
	SchemaVersion uint16      `json:"schema_version"`
	ProjectDigest Digest      `json:"project_digest"`
	ContentDigest Digest      `json:"content_digest"`
	Files         []CacheFile `json:"files"`
}

type PackageCache struct {
	root     *os.Root
	rootPath string
	limits   CacheLimits
}

const cacheInstallLockStripes = 64

var cacheInstallLocks [cacheInstallLockStripes]chan struct{}
var cacheInstallLocksOnce sync.Once

func acquireCacheInstallLock(ctx context.Context, key string) (func(), error) {
	cacheInstallLocksOnce.Do(func() {
		for index := range cacheInstallLocks {
			cacheInstallLocks[index] = make(chan struct{}, 1)
			cacheInstallLocks[index] <- struct{}{}
		}
	})
	sum := sha256.Sum256([]byte(key))
	lock := cacheInstallLocks[int(sum[0])%len(cacheInstallLocks)]
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-lock:
		return func() { lock <- struct{}{} }, nil
	}
}

func NewPackageCache(cacheRoot string, limits CacheLimits) (*PackageCache, error) {
	if cacheRoot == "" {
		return nil, fmt.Errorf("%w: cache root is required", ErrCacheInvalid)
	}
	if err := limits.validate(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cacheRoot, 0o700); err != nil {
		return nil, fmt.Errorf("%w: create cache root: %v", ErrCacheInvalid, err)
	}
	if err := os.Chmod(cacheRoot, 0o700); err != nil {
		return nil, fmt.Errorf("%w: restrict cache root: %v", ErrCacheInvalid, err)
	}
	root, err := os.OpenRoot(cacheRoot)
	if err != nil {
		return nil, fmt.Errorf("%w: open cache root: %v", ErrCacheInvalid, err)
	}
	rootPath, err := filepath.EvalSymlinks(cacheRoot)
	if err != nil {
		_ = root.Close()
		return nil, fmt.Errorf("%w: resolve cache root: %v", ErrCacheInvalid, err)
	}
	rootPath, err = filepath.Abs(rootPath)
	if err != nil {
		_ = root.Close()
		return nil, fmt.Errorf("%w: resolve cache root: %v", ErrCacheInvalid, err)
	}
	return &PackageCache{root: root, rootPath: filepath.Clean(rootPath), limits: limits}, nil
}

func (cache *PackageCache) Close() error {
	if cache == nil || cache.root == nil {
		return nil
	}
	return cache.root.Close()
}

// ArchiveContentDigest is the canonical extracted-tree identity used as the
// cache content address. Compression metadata does not affect this digest.
func ArchiveContentDigest(plan ArchivePlan) (Digest, error) {
	files, _, err := validateArchivePlan(plan, CacheLimits{MaxManifestBytes: HardMaxCacheManifestBytes,
		MaxStateBytes: HardMaxCacheStateBytes, MaxFiles: HardMaxArchiveFiles, MaxFileBytes: HardMaxArchiveFileBytes,
		MaxTotalBytes: HardMaxArchiveUncompressedBytes, MaxPathBytes: HardMaxPathBytes})
	if err != nil {
		return "", err
	}
	return archiveContentDigestFromFiles(files), nil
}

func archiveContentDigestFromFiles(files []CacheFile) Digest {
	projection := struct {
		Domain string      `json:"domain"`
		Files  []CacheFile `json:"files"`
	}{Domain: "gopdfkit.paperpkg.archive-content.v1", Files: files}
	encoded, _ := json.Marshal(projection)
	sum := sha256.Sum256(encoded)
	return Digest(hex.EncodeToString(sum[:]))
}

func (cache *PackageCache) Install(ctx context.Context, projectDigest, contentDigest Digest, plan ArchivePlan) error {
	if cache == nil || cache.root == nil || ctx == nil {
		return fmt.Errorf("%w: cache and context are required", ErrCacheInvalid)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateDigest(projectDigest); err != nil {
		return fmt.Errorf("%w: project digest: %v", ErrCacheInvalid, err)
	}
	if err := validateDigest(contentDigest); err != nil {
		return fmt.Errorf("%w: content digest: %v", ErrCacheInvalid, err)
	}
	files, _, err := validateArchivePlan(plan, cache.limits)
	if err != nil {
		return err
	}
	actual, err := ArchiveContentDigest(plan)
	if err != nil {
		return err
	}
	if actual != contentDigest {
		return fmt.Errorf("%w: expected %s, got %s", ErrCacheMismatch, contentDigest, actual)
	}
	base := string(projectDigest) + "/" + string(contentDigest)
	release, err := acquireCacheInstallLock(ctx, cache.rootPath+"\x00"+base)
	if err != nil {
		return err
	}
	defer release()
	if _, found, err := cache.lookup(ctx, projectDigest, contentDigest); found || err != nil {
		return err
	}
	project := string(projectDigest)
	if err := cache.root.MkdirAll(project, 0o700); err != nil {
		return fmt.Errorf("paperpkg: create project cache directory: %w", err)
	}
	if info, err := cache.root.Lstat(project); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: project cache directory is unsafe", ErrCacheCorrupt)
	}
	stage, err := cache.createStage(project, contentDigest)
	if err != nil {
		return err
	}
	published := false
	directories := map[string]bool{stage: true}
	defer func() {
		if !published {
			_ = cache.root.RemoveAll(stage)
		}
	}()
	for index, entry := range plan.Entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		target := stage + "/files/" + entry.Path
		directory := path.Dir(target)
		if err := cache.root.MkdirAll(directory, 0o700); err != nil {
			return fmt.Errorf("paperpkg: create staged directory: %w", err)
		}
		for current := directory; current != project && current != "."; current = path.Dir(current) {
			directories[current] = true
			if current == stage {
				break
			}
		}
		if err := cache.writeFile(ctx, target, entry.Bytes); err != nil {
			return err
		}
		if uint64(index+1) > uint64(cache.limits.MaxFiles) {
			return ErrCacheLimit
		}
	}
	directoryNames := make([]string, 0, len(directories))
	for directory := range directories {
		directoryNames = append(directoryNames, directory)
	}
	sort.Slice(directoryNames, func(i, j int) bool {
		return strings.Count(directoryNames[i], "/") > strings.Count(directoryNames[j], "/")
	})
	for _, directory := range directoryNames {
		if err := syncRootDirectory(cache.root, directory); err != nil {
			return err
		}
	}
	manifest := cacheManifest{SchemaVersion: CacheManifestSchemaVersion, ProjectDigest: projectDigest, ContentDigest: contentDigest, Files: files}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil || uint64(len(manifestJSON)) > cache.limits.MaxManifestBytes {
		return fmt.Errorf("%w: manifest encoding or byte budget", ErrCacheLimit)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := cache.writeFile(ctx, stage+"/manifest.json", manifestJSON); err != nil {
		return err
	}
	if err := syncRootDirectory(cache.root, stage); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := cache.root.Rename(stage, base); err != nil {
		if _, found, verifyErr := cache.lookup(ctx, projectDigest, contentDigest); found && verifyErr == nil {
			return nil
		} else if verifyErr != nil {
			return verifyErr
		}
		return fmt.Errorf("paperpkg: publish package cache: %w", err)
	}
	published = true
	if err := syncRootDirectory(cache.root, project); err != nil {
		return err
	}
	_, found, err := cache.lookup(ctx, projectDigest, contentDigest)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("%w: published package disappeared", ErrCacheCorrupt)
	}
	return nil
}

func (cache *PackageCache) Lookup(ctx context.Context, projectDigest, contentDigest Digest) (CachePackage, bool, error) {
	return cache.lookup(ctx, projectDigest, contentDigest)
}

func (cache *PackageCache) ReadFile(ctx context.Context, projectDigest, contentDigest Digest, relativePath string) ([]byte, error) {
	packageInfo, found, err := cache.lookup(ctx, projectDigest, contentDigest)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrCacheNotFound
	}
	index := sort.Search(len(packageInfo.Files), func(index int) bool { return packageInfo.Files[index].Path >= relativePath })
	if index == len(packageInfo.Files) || packageInfo.Files[index].Path != relativePath {
		return nil, fmt.Errorf("%w: file %q", ErrCacheNotFound, relativePath)
	}
	base := string(projectDigest) + "/" + string(contentDigest) + "/files/" + relativePath
	return cache.readAndVerify(ctx, base, packageInfo.Files[index])
}

func (cache *PackageCache) lookup(ctx context.Context, projectDigest, contentDigest Digest) (CachePackage, bool, error) {
	if cache == nil || cache.root == nil || ctx == nil {
		return CachePackage{}, false, fmt.Errorf("%w: cache and context are required", ErrCacheInvalid)
	}
	if err := ctx.Err(); err != nil {
		return CachePackage{}, false, err
	}
	if validateDigest(projectDigest) != nil || validateDigest(contentDigest) != nil {
		return CachePackage{}, false, ErrCacheInvalid
	}
	base := string(projectDigest) + "/" + string(contentDigest)
	manifestBytes, err := cache.readBounded(ctx, base+"/manifest.json", cache.limits.MaxManifestBytes)
	if errors.Is(err, fs.ErrNotExist) {
		if _, statErr := cache.root.Lstat(base); errors.Is(statErr, fs.ErrNotExist) {
			return CachePackage{}, false, nil
		} else if statErr != nil {
			return CachePackage{}, false, fmt.Errorf("%w: inspect content directory: %v", ErrCacheCorrupt, statErr)
		}
		return CachePackage{}, false, fmt.Errorf("%w: content directory has no manifest", ErrCacheCorrupt)
	}
	if err != nil {
		return CachePackage{}, false, fmt.Errorf("%w: manifest: %v", ErrCacheCorrupt, err)
	}
	manifest, err := decodeCacheManifest(manifestBytes, cache.limits)
	if err != nil {
		return CachePackage{}, false, err
	}
	if manifest.ProjectDigest != projectDigest || manifest.ContentDigest != contentDigest {
		return CachePackage{}, false, fmt.Errorf("%w: manifest identity does not match its address", ErrCacheCorrupt)
	}
	if archiveContentDigestFromFiles(manifest.Files) != contentDigest {
		return CachePackage{}, false, fmt.Errorf("%w: manifest file tree does not match its content address", ErrCacheCorrupt)
	}
	expectedPaths := map[string]bool{"manifest.json": true, "files": true}
	for _, file := range manifest.Files {
		for directory := path.Dir(file.Path); directory != "."; directory = path.Dir(directory) {
			expectedPaths["files/"+directory] = true
		}
		expectedPaths["files/"+file.Path] = true
		if _, err := cache.readAndVerify(ctx, base+"/files/"+file.Path, file); err != nil {
			return CachePackage{}, false, err
		}
	}
	err = fs.WalkDir(cache.root.FS(), base, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		relative := strings.TrimPrefix(name, base+"/")
		if name != base && !expectedPaths[relative] || entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("unexpected or unsafe path %q", relative)
		}
		info, err := entry.Info()
		if err != nil || runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
			return fmt.Errorf("unsafe permissions for %q", relative)
		}
		return nil
	})
	if err != nil {
		return CachePackage{}, false, fmt.Errorf("%w: %v", ErrCacheCorrupt, err)
	}
	files := append([]CacheFile(nil), manifest.Files...)
	return CachePackage{ProjectDigest: projectDigest, ContentDigest: contentDigest, Files: files}, true, nil
}

func validateArchivePlan(plan ArchivePlan, limits CacheLimits) ([]CacheFile, uint64, error) {
	if err := limits.validate(); err != nil {
		return nil, 0, err
	}
	if uint64(len(plan.Entries)) > uint64(limits.MaxFiles) {
		return nil, 0, ErrCacheLimit
	}
	files := make([]CacheFile, len(plan.Entries))
	var total, compressed uint64
	previous := ""
	for index, entry := range plan.Entries {
		if err := validateRelativePath(entry.Path, limits.MaxPathBytes); err != nil || index > 0 && entry.Path <= previous {
			return nil, 0, fmt.Errorf("%w: archive plan path %q", ErrCacheInvalid, entry.Path)
		}
		previous = entry.Path
		if uint64(len(entry.Bytes)) != entry.UncompressedBytes || entry.UncompressedBytes > limits.MaxFileBytes ||
			total > limits.MaxTotalBytes-entry.UncompressedBytes {
			return nil, 0, ErrCacheLimit
		}
		sum := sha256.Sum256(entry.Bytes)
		digest := Digest(hex.EncodeToString(sum[:]))
		if digest != entry.Digest {
			return nil, 0, fmt.Errorf("%w: archive plan file %q", ErrCacheMismatch, entry.Path)
		}
		total += entry.UncompressedBytes
		if compressed > ^uint64(0)-entry.CompressedBytes {
			return nil, 0, fmt.Errorf("%w: compressed byte total overflows", ErrCacheInvalid)
		}
		compressed += entry.CompressedBytes
		files[index] = CacheFile{Path: entry.Path, Bytes: entry.UncompressedBytes, Digest: entry.Digest}
	}
	if total != plan.UncompressedBytes || compressed != plan.CompressedBytes || total > limits.MaxStateBytes {
		return nil, 0, fmt.Errorf("%w: archive plan aggregate metadata", ErrCacheMismatch)
	}
	return files, total, nil
}

func decodeCacheManifest(encoded []byte, limits CacheLimits) (cacheManifest, error) {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var manifest cacheManifest
	if err := decoder.Decode(&manifest); err != nil {
		return manifest, fmt.Errorf("%w: decode manifest: %v", ErrCacheCorrupt, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return manifest, fmt.Errorf("%w: trailing manifest data", ErrCacheCorrupt)
	}
	if manifest.SchemaVersion != CacheManifestSchemaVersion || manifest.Files == nil || uint64(len(manifest.Files)) > uint64(limits.MaxFiles) {
		return manifest, fmt.Errorf("%w: manifest schema or file count", ErrCacheCorrupt)
	}
	previous := ""
	var state uint64
	for index, file := range manifest.Files {
		if validateRelativePath(file.Path, limits.MaxPathBytes) != nil || validateDigest(file.Digest) != nil ||
			file.Bytes > limits.MaxFileBytes || index > 0 && file.Path <= previous {
			return manifest, fmt.Errorf("%w: invalid manifest file %q", ErrCacheCorrupt, file.Path)
		}
		previous = file.Path
		state += uint64(len(file.Path)) + file.Bytes + 96
		if state > limits.MaxStateBytes || state > limits.MaxTotalBytes+limits.MaxManifestBytes {
			return manifest, ErrCacheLimit
		}
	}
	canonical, _ := json.Marshal(manifest)
	if !bytes.Equal(encoded, canonical) {
		return manifest, fmt.Errorf("%w: manifest is not canonical", ErrCacheCorrupt)
	}
	return manifest, nil
}

func (cache *PackageCache) createStage(project string, content Digest) (string, error) {
	for range 32 {
		random := make([]byte, 12)
		if _, err := rand.Read(random); err != nil {
			return "", err
		}
		name := project + "/." + string(content) + ".stage-" + hex.EncodeToString(random)
		if err := cache.root.Mkdir(name, 0o700); err == nil {
			return name, nil
		} else if !errors.Is(err, fs.ErrExist) {
			return "", err
		}
	}
	return "", fmt.Errorf("%w: could not allocate staging directory", ErrCacheInvalid)
}

func (cache *PackageCache) writeFile(ctx context.Context, name string, content []byte) error {
	file, err := cache.root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	for offset := 0; offset < len(content); {
		if err := ctx.Err(); err != nil {
			return err
		}
		end := offset + 32<<10
		if end > len(content) {
			end = len(content)
		}
		written, err := file.Write(content[offset:end])
		if err != nil {
			return err
		}
		offset += written
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("paperpkg: fsync cache file: %w", err)
	}
	return file.Close()
}

func (cache *PackageCache) readAndVerify(ctx context.Context, name string, metadata CacheFile) ([]byte, error) {
	content, err := cache.readBounded(ctx, name, metadata.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrCacheCorrupt, metadata.Path, err)
	}
	if uint64(len(content)) != metadata.Bytes {
		return nil, fmt.Errorf("%w: %s size mismatch", ErrCacheCorrupt, metadata.Path)
	}
	sum := sha256.Sum256(content)
	if Digest(hex.EncodeToString(sum[:])) != metadata.Digest {
		return nil, fmt.Errorf("%w: %s digest mismatch", ErrCacheCorrupt, metadata.Path)
	}
	return content, nil
}

func (cache *PackageCache) readBounded(ctx context.Context, name string, maxBytes uint64) ([]byte, error) {
	file, err := cache.root.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() < 0 || uint64(info.Size()) > maxBytes ||
		(runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0) {
		return nil, fmt.Errorf("unsafe mode, type, or size")
	}
	output := make([]byte, 0, info.Size())
	buffer := make([]byte, 32<<10)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		read, readErr := file.Read(buffer)
		if uint64(len(output)+read) > maxBytes {
			return nil, ErrCacheLimit
		}
		output = append(output, buffer[:read]...)
		if errors.Is(readErr, io.EOF) {
			return output, nil
		}
		if readErr != nil {
			return nil, readErr
		}
	}
}

func syncRootDirectory(root *os.Root, name string) error {
	directory, err := root.Open(name)
	if err != nil {
		return err
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil && !errors.Is(err, syscall.EINVAL) && !errors.Is(err, syscall.ENOTSUP) && !errors.Is(err, syscall.ENOSYS) {
		return fmt.Errorf("paperpkg: fsync cache directory: %w", err)
	}
	return nil
}
