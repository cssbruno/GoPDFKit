// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperpkg

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOfflineResolverResolvesVerifiedImportAndLockedAsset(t *testing.T) {
	root := t.TempDir()
	writeResolverFile(t, root, "packages/chart.paper", []byte("chart package"))
	writeResolverFile(t, root, "assets/chart.png", []byte("png bytes"))
	entry := resolverEntry()
	resolver := newTestResolver(t, root, AllowInternalSymlinks)
	defer func() { _ = resolver.Close() }()

	content, err := resolver.ResolveImport(context.Background(), entry, uint64(len("chart package")))
	if err != nil || string(content) != "chart package" {
		t.Fatalf("ResolveImport() = %q, %v", content, err)
	}
	asset, err := resolver.ResolveAsset(context.Background(), entry, "assets/chart.png", uint64(len("png bytes")))
	if err != nil || string(asset) != "png bytes" {
		t.Fatalf("ResolveAsset() = %q, %v", asset, err)
	}
	asset[0] = 'X'
	again, err := resolver.ResolveAsset(context.Background(), entry, "assets/chart.png", uint64(len("png bytes")))
	if err != nil || string(again) != "png bytes" {
		t.Fatalf("second ResolveAsset() = %q, %v", again, err)
	}
	if _, err := resolver.ResolveAsset(context.Background(), entry, "assets/missing.png", 100); !errors.Is(err, ErrResolverNotLocked) {
		t.Fatalf("unlocked ResolveAsset() = %v", err)
	}
}

func TestOfflineResolverRejectsInvalidPathsAndDigestsBeforeOpen(t *testing.T) {
	resolver := newTestResolver(t, t.TempDir(), AllowInternalSymlinks)
	defer func() { _ = resolver.Close() }()
	for _, relative := range []string{"../outside", "/absolute", `dir\file`, "https://example.test/file", "dir/../file"} {
		if _, err := resolver.Resolve(context.Background(), relative, shaDigest([]byte("x")), 1); !errors.Is(err, ErrResolverInvalid) {
			t.Fatalf("Resolve(%q) = %v", relative, err)
		}
	}
	if _, err := resolver.Resolve(context.Background(), "missing", "bad", 1); !errors.Is(err, ErrResolverInvalid) {
		t.Fatalf("Resolve(bad digest) = %v", err)
	}
	if _, err := resolver.Resolve(context.Background(), "missing", shaDigest(nil), 0); !errors.Is(err, ErrResolverInvalid) {
		t.Fatalf("Resolve(zero limit) = %v", err)
	}
}

func TestOfflineResolverAllowsInternalSymlinksOrRejectsThemByPolicy(t *testing.T) {
	root := t.TempDir()
	writeResolverFile(t, root, "real/package.paper", []byte("internal"))
	if err := os.Symlink("real", filepath.Join(root, "alias")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	allow := newTestResolver(t, root, AllowInternalSymlinks)
	defer func() { _ = allow.Close() }()
	content, err := allow.Resolve(context.Background(), "alias/package.paper", shaDigest([]byte("internal")), 64)
	if err != nil || string(content) != "internal" {
		t.Fatalf("internal symlink Resolve() = %q, %v", content, err)
	}

	reject := newTestResolver(t, root, RejectAllSymlinks)
	defer func() { _ = reject.Close() }()
	if _, err := reject.Resolve(context.Background(), "alias/package.paper", shaDigest([]byte("internal")), 64); !errors.Is(err, ErrResolverSymlink) {
		t.Fatalf("reject-all Resolve() = %v", err)
	}
}

func TestOfflineResolverRejectsFinalAndIntermediateSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeResolverFile(t, outside, "secret.paper", []byte("secret"))
	if err := os.Symlink(filepath.Join(outside, "secret.paper"), filepath.Join(root, "final.paper")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "outside-dir")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	resolver := newTestResolver(t, root, AllowInternalSymlinks)
	defer func() { _ = resolver.Close() }()
	for _, relative := range []string{"final.paper", "outside-dir/secret.paper"} {
		if _, err := resolver.Resolve(context.Background(), relative, shaDigest([]byte("secret")), 64); !errors.Is(err, ErrResolverEscape) {
			t.Fatalf("escape Resolve(%q) = %v", relative, err)
		}
	}
}

func TestOfflineResolverRequiresDigestAndExactByteCeiling(t *testing.T) {
	root := t.TempDir()
	content := []byte("four")
	writeResolverFile(t, root, "file.paper", content)
	resolver := newTestResolver(t, root, AllowInternalSymlinks)
	defer func() { _ = resolver.Close() }()
	if bytes, err := resolver.Resolve(context.Background(), "file.paper", shaDigest(content), 4); err != nil || string(bytes) != "four" {
		t.Fatalf("exact-limit Resolve() = %q, %v", bytes, err)
	}
	if bytes, err := resolver.Resolve(context.Background(), "file.paper", shaDigest(content), 3); !errors.Is(err, ErrResolverLimit) || bytes != nil {
		t.Fatalf("oversized Resolve() = %q, %v", bytes, err)
	}
	if bytes, err := resolver.Resolve(context.Background(), "file.paper", shaDigest([]byte("wrong")), 4); !errors.Is(err, ErrResolverDigest) || bytes != nil {
		t.Fatalf("digest-mismatch Resolve() = %q, %v", bytes, err)
	}
}

func TestOfflineResolverRejectsNonRegularFilesAndCanceledContext(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "directory"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeResolverFile(t, root, "file.paper", []byte("content"))
	resolver := newTestResolver(t, root, AllowInternalSymlinks)
	defer func() { _ = resolver.Close() }()
	if _, err := resolver.Resolve(context.Background(), "directory", shaDigest(nil), 64); !errors.Is(err, ErrResolverNotRegular) {
		t.Fatalf("directory Resolve() = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if bytes, err := resolver.Resolve(ctx, "file.paper", shaDigest([]byte("content")), 64); !errors.Is(err, context.Canceled) || bytes != nil {
		t.Fatalf("canceled Resolve() = %q, %v", bytes, err)
	}
}

func TestOfflineResolverRejectAllPolicyRejectsSymlinkProjectRoot(t *testing.T) {
	realRoot := t.TempDir()
	linkRoot := filepath.Join(t.TempDir(), "root-link")
	if err := os.Symlink(realRoot, linkRoot); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if resolver, err := NewOfflineResolver(linkRoot, ResolverOptions{SymlinkPolicy: RejectAllSymlinks}); !errors.Is(err, ErrResolverSymlink) || resolver != nil {
		t.Fatalf("NewOfflineResolver(symlink root) = %#v, %v", resolver, err)
	}
	resolver, err := NewOfflineResolver(linkRoot, ResolverOptions{SymlinkPolicy: AllowInternalSymlinks})
	if err != nil {
		t.Fatalf("allow symlink root = %v", err)
	}
	_ = resolver.Close()
}

func resolverEntry() Entry {
	return Entry{ImportPath: "packages/chart.paper", ContentDigest: shaDigest([]byte("chart package")),
		Assets:          []Asset{{Path: "assets/chart.png", Digest: shaDigest([]byte("png bytes"))}},
		SignaturePolicy: SignatureRequired, OfflinePolicy: OfflineOnly}
}

func newTestResolver(t *testing.T, root string, policy SymlinkPolicy) *OfflineResolver {
	t.Helper()
	resolver, err := NewOfflineResolver(root, ResolverOptions{SymlinkPolicy: policy})
	if err != nil {
		t.Fatal(err)
	}
	return resolver
}

func writeResolverFile(t *testing.T, root, relative string, content []byte) {
	t.Helper()
	filename := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, content, 0o600); err != nil {
		t.Fatal(err)
	}
}

func shaDigest(content []byte) Digest {
	sum := sha256.Sum256(content)
	return Digest(hex.EncodeToString(sum[:]))
}

func TestOfflineResolverEntryValidation(t *testing.T) {
	resolver := newTestResolver(t, t.TempDir(), AllowInternalSymlinks)
	defer func() { _ = resolver.Close() }()
	entry := resolverEntry()
	entry.Assets = append(entry.Assets, Asset{Path: "a-first", Digest: shaDigest(nil)})
	if _, err := resolver.ResolveImport(context.Background(), entry, 64); !errors.Is(err, ErrResolverInvalid) {
		t.Fatalf("ResolveImport(unsorted entry) = %v", err)
	}
	entry = resolverEntry()
	entry.ImportPath = strings.Repeat("x", int(HardMaxPathBytes)+1)
	if _, err := resolver.ResolveImport(context.Background(), entry, 64); !errors.Is(err, ErrResolverInvalid) {
		t.Fatalf("ResolveImport(oversized path) = %v", err)
	}
}
