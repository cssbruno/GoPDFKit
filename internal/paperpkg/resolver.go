// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperpkg

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const HardMaxResolvedFileBytes uint64 = 1 << 30

var (
	ErrResolverInvalid    = errors.New("paperpkg: invalid offline resolution request")
	ErrResolverEscape     = errors.New("paperpkg: resolved path escapes the project root")
	ErrResolverSymlink    = errors.New("paperpkg: symlink rejected by resolver policy")
	ErrResolverLimit      = errors.New("paperpkg: resolved file byte limit exceeded")
	ErrResolverNotRegular = errors.New("paperpkg: resolved path is not a regular file")
	ErrResolverDigest     = errors.New("paperpkg: resolved content digest mismatch")
	ErrResolverNotLocked  = errors.New("paperpkg: asset is not present in the lock entry")
)

type SymlinkPolicy string

const (
	AllowInternalSymlinks SymlinkPolicy = "allow_internal"
	RejectAllSymlinks     SymlinkPolicy = "reject_all"
)

func (policy SymlinkPolicy) valid() bool {
	return policy == AllowInternalSymlinks || policy == RejectAllSymlinks
}

type ResolverOptions struct {
	SymlinkPolicy SymlinkPolicy
}

func DefaultResolverOptions() ResolverOptions {
	return ResolverOptions{SymlinkPolicy: AllowInternalSymlinks}
}

// OfflineResolver reads only through an os.Root anchored to one explicit
// project directory. It never performs network access. A resolver owns the
// root handle and must be closed when no longer needed.
type OfflineResolver struct {
	root          *os.Root
	rootPath      string
	canonicalRoot string
	policy        SymlinkPolicy
}

func NewOfflineResolver(projectRoot string, options ResolverOptions) (*OfflineResolver, error) {
	if projectRoot == "" || !options.SymlinkPolicy.valid() {
		return nil, fmt.Errorf("%w: project root and symlink policy are required", ErrResolverInvalid)
	}
	absolute, err := filepath.Abs(projectRoot)
	if err != nil {
		return nil, fmt.Errorf("%w: absolute project root: %w", ErrResolverInvalid, err)
	}
	rootInfo, err := os.Lstat(absolute)
	if err != nil {
		return nil, fmt.Errorf("%w: inspect project root: %w", ErrResolverInvalid, err)
	}
	if options.SymlinkPolicy == RejectAllSymlinks && rootInfo.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%w: project root is a symlink", ErrResolverSymlink)
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, fmt.Errorf("%w: resolve project root: %w", ErrResolverInvalid, err)
	}
	info, err := os.Stat(canonical)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("%w: project root is not a directory", ErrResolverInvalid)
	}
	root, err := os.OpenRoot(absolute)
	if err != nil {
		return nil, fmt.Errorf("%w: open project root: %w", ErrResolverInvalid, err)
	}
	return &OfflineResolver{root: root, rootPath: absolute, canonicalRoot: canonical, policy: options.SymlinkPolicy}, nil
}

func (resolver *OfflineResolver) Close() error {
	if resolver == nil || resolver.root == nil {
		return nil
	}
	return resolver.root.Close()
}

// ResolveImport loads and verifies the file pinned by entry.ImportPath.
func (resolver *OfflineResolver) ResolveImport(ctx context.Context, entry Entry, maxBytes uint64) ([]byte, error) {
	if err := validateResolverEntry(entry); err != nil {
		return nil, err
	}
	return resolver.Resolve(ctx, entry.ImportPath, entry.ContentDigest, maxBytes)
}

// ResolveAsset loads one project-root-relative asset pinned by entry. Asset
// lookup is exact and uses the lockfile's canonical sorted order.
func (resolver *OfflineResolver) ResolveAsset(ctx context.Context, entry Entry, assetPath string, maxBytes uint64) ([]byte, error) {
	if err := validateResolverEntry(entry); err != nil {
		return nil, err
	}
	index := sort.Search(len(entry.Assets), func(index int) bool { return entry.Assets[index].Path >= assetPath })
	if index == len(entry.Assets) || entry.Assets[index].Path != assetPath {
		return nil, fmt.Errorf("%w: %q", ErrResolverNotLocked, assetPath)
	}
	return resolver.Resolve(ctx, entry.Assets[index].Path, entry.Assets[index].Digest, maxBytes)
}

// Resolve safely opens one normalized project-relative path, streams at most
// maxBytes, and returns bytes only after the complete SHA-256 digest matches.
func (resolver *OfflineResolver) Resolve(ctx context.Context, relativePath string, expected Digest, maxBytes uint64) ([]byte, error) {
	if resolver == nil || resolver.root == nil || ctx == nil {
		return nil, fmt.Errorf("%w: resolver and context are required", ErrResolverInvalid)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if maxBytes == 0 || maxBytes > HardMaxResolvedFileBytes {
		return nil, fmt.Errorf("%w: max bytes must be between 1 and %d", ErrResolverInvalid, HardMaxResolvedFileBytes)
	}
	if err := validateRelativePath(relativePath, HardMaxPathBytes); err != nil {
		return nil, fmt.Errorf("%w: path: %w", ErrResolverInvalid, err)
	}
	if err := validateDigest(expected); err != nil {
		return nil, fmt.Errorf("%w: digest: %w", ErrResolverInvalid, err)
	}
	if err := resolver.preflightPath(ctx, relativePath); err != nil {
		return nil, err
	}
	file, err := resolver.root.Open(filepath.FromSlash(relativePath))
	if err != nil {
		return nil, fmt.Errorf("paperpkg: open offline path %q: %w", relativePath, err)
	}
	defer func() { _ = file.Close() }()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("paperpkg: stat offline path %q: %w", relativePath, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %q", ErrResolverNotRegular, relativePath)
	}
	if info.Size() < 0 || uint64(info.Size()) > maxBytes {
		return nil, fmt.Errorf("%w: %q exceeds %d bytes", ErrResolverLimit, relativePath, maxBytes)
	}
	return readVerifiedFile(ctx, file, relativePath, expected, maxBytes, info.Size())
}

func (resolver *OfflineResolver) preflightPath(ctx context.Context, relativePath string) error {
	if resolver.policy == RejectAllSymlinks {
		current := ""
		for _, component := range strings.Split(relativePath, "/") {
			if err := ctx.Err(); err != nil {
				return err
			}
			if current == "" {
				current = component
			} else {
				current += "/" + component
			}
			info, err := resolver.root.Lstat(filepath.FromSlash(current))
			if err != nil {
				return fmt.Errorf("paperpkg: inspect offline path %q: %w", current, err)
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("%w: %q", ErrResolverSymlink, current)
			}
		}
		return nil
	}

	// EvalSymlinks provides a precise escape error for callers. The subsequent
	// os.Root.Open is the actual containment boundary and remains authoritative
	// if the tree changes between this diagnostic preflight and open.
	resolved, err := filepath.EvalSymlinks(filepath.Join(resolver.rootPath, filepath.FromSlash(relativePath)))
	if err != nil {
		return fmt.Errorf("paperpkg: resolve offline path %q: %w", relativePath, err)
	}
	relative, err := filepath.Rel(resolver.canonicalRoot, resolved)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return fmt.Errorf("%w: %q", ErrResolverEscape, relativePath)
	}
	return ctx.Err()
}

func readVerifiedFile(ctx context.Context, file *os.File, relativePath string, expected Digest, maxBytes uint64, size int64) ([]byte, error) {
	hasher := sha256.New()
	var output bytes.Buffer
	if size > 0 {
		output.Grow(int(size))
	}
	buffer := make([]byte, 32<<10)
	var total uint64
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		read, err := file.Read(buffer)
		if read > 0 {
			if uint64(read) > maxBytes-total {
				return nil, fmt.Errorf("%w: %q exceeds %d bytes", ErrResolverLimit, relativePath, maxBytes)
			}
			total += uint64(read)
			_, _ = hasher.Write(buffer[:read])
			_, _ = output.Write(buffer[:read])
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("paperpkg: read offline path %q: %w", relativePath, err)
		}
		if read == 0 {
			return nil, fmt.Errorf("paperpkg: read offline path %q: %w", relativePath, io.ErrNoProgress)
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	actual := Digest(hex.EncodeToString(hasher.Sum(nil)))
	if actual != expected {
		return nil, fmt.Errorf("%w: %q expected %s, got %s", ErrResolverDigest, relativePath, expected, actual)
	}
	return output.Bytes(), nil
}

func validateResolverEntry(entry Entry) error {
	lockfile := Lockfile{SchemaVersion: LockfileSchemaVersion, Entries: []Entry{cloneEntry(entry)}}
	limits := Limits{MaxLockfileBytes: HardMaxLockfileBytes, MaxStateBytes: HardMaxStateBytes,
		MaxEntries: HardMaxEntries, MaxAssets: HardMaxAssets, MaxPathBytes: HardMaxPathBytes}
	if err := lockfile.ValidateWithLimits(limits); err != nil {
		return fmt.Errorf("%w: entry: %w", ErrResolverInvalid, err)
	}
	return nil
}
