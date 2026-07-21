// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperpkg

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestPackageCacheAtomicInstallLookupReadAndIdempotence(t *testing.T) {
	root := t.TempDir()
	cache := newTestPackageCache(t, root, DefaultCacheLimits())
	defer cache.Close()
	plan := cacheTestPlan(t, "first")
	project := digest('e')
	content, err := ArchiveContentDigest(plan)
	if err != nil {
		t.Fatal(err)
	}
	if err := cache.Install(context.Background(), project, content, plan); err != nil {
		t.Fatal(err)
	}
	if err := cache.Install(context.Background(), project, content, plan); err != nil {
		t.Fatalf("idempotent Install() = %v", err)
	}
	item, found, err := cache.Lookup(context.Background(), project, content)
	if err != nil || !found || len(item.Files) != 2 || item.Files[0].Path != "a/first.txt" {
		t.Fatalf("Lookup() = %#v, %v, %v", item, found, err)
	}
	item.Files[0].Path = "mutated"
	again, found, err := cache.Lookup(context.Background(), project, content)
	if err != nil || !found || again.Files[0].Path != "a/first.txt" {
		t.Fatalf("detached Lookup() = %#v, %v, %v", again, found, err)
	}
	bytes, err := cache.ReadFile(context.Background(), project, content, "a/first.txt")
	if err != nil || string(bytes) != "first-first" {
		t.Fatalf("ReadFile() = %q, %v", bytes, err)
	}
	bytes[0] = 'X'
	bytes, _ = cache.ReadFile(context.Background(), project, content, "a/first.txt")
	if string(bytes) != "first-first" {
		t.Fatal("ReadFile returned aliased cache storage")
	}

	base := filepath.Join(root, string(project), string(content))
	for _, name := range []string{"manifest.json", filepath.Join("files", "a", "first.txt"), filepath.Join("files", "z.txt")} {
		info, err := os.Stat(filepath.Join(base, name))
		if err != nil || info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("restrictive mode %s = %v, %v", name, info, err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(root, string(project)))
	if err != nil || len(entries) != 1 || entries[0].Name() != string(content) {
		t.Fatalf("project publications = %v, %v", entries, err)
	}
}

func TestPackageCacheConcurrentInstallIsIdempotent(t *testing.T) {
	cache := newTestPackageCache(t, t.TempDir(), DefaultCacheLimits())
	defer cache.Close()
	plan := cacheTestPlan(t, "concurrent")
	content, _ := ArchiveContentDigest(plan)
	project := digest('d')
	const workers = 24
	start := make(chan struct{})
	errorsSeen := make(chan error, workers)
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			errorsSeen <- cache.Install(context.Background(), project, content, plan)
		}()
	}
	close(start)
	wait.Wait()
	close(errorsSeen)
	for err := range errorsSeen {
		if err != nil {
			t.Errorf("concurrent Install() = %v", err)
		}
	}
	if _, found, err := cache.Lookup(context.Background(), project, content); err != nil || !found {
		t.Fatalf("concurrent Lookup() = %v, %v", found, err)
	}
}

func TestPackageCacheConcurrentInstallAcrossHandlesPublishesOneCompleteTree(t *testing.T) {
	root := t.TempDir()
	first := newTestPackageCache(t, root, DefaultCacheLimits())
	defer first.Close()
	second := newTestPackageCache(t, root, DefaultCacheLimits())
	defer second.Close()
	plan := cacheTestPlan(t, "multi-handle")
	content, _ := ArchiveContentDigest(plan)
	project := digest('7')
	caches := []*PackageCache{first, second}
	const workers = 32
	start := make(chan struct{})
	errs := make(chan error, workers)
	var wait sync.WaitGroup
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func(cache *PackageCache) {
			defer wait.Done()
			<-start
			errs <- cache.Install(context.Background(), project, content, plan)
		}(caches[index%len(caches)])
	}
	close(start)
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("multi-handle Install()=%v", err)
		}
	}
	if item, found, err := first.Lookup(context.Background(), project, content); err != nil || !found || len(item.Files) != 2 {
		t.Fatalf("multi-handle Lookup()=%#v %v %v", item, found, err)
	}
	entries, err := os.ReadDir(filepath.Join(root, string(project)))
	if err != nil || len(entries) != 1 || entries[0].Name() != string(content) {
		t.Fatalf("multi-handle publications=%v %v", entries, err)
	}
}

func TestPackageCacheInstallLockCancellationAndStaleStageRecovery(t *testing.T) {
	root := t.TempDir()
	cache := newTestPackageCache(t, root, DefaultCacheLimits())
	defer cache.Close()
	plan := cacheTestPlan(t, "recovery")
	content, _ := ArchiveContentDigest(plan)
	project := digest('6')
	base := string(project) + "/" + string(content)
	release, err := acquireCacheInstallLock(context.Background(), cache.rootPath+"\x00"+base)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := cache.Install(ctx, project, content, plan); !errors.Is(err, context.Canceled) {
		t.Fatalf("waiting canceled Install()=%v", err)
	}
	release()
	projectPath := filepath.Join(root, string(project))
	stale := filepath.Join(projectPath, "."+string(content)+".stage-crashed")
	if err := os.MkdirAll(filepath.Join(stale, "files"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stale, "files", "partial"), []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := cache.Install(context.Background(), project, content, plan); err != nil {
		t.Fatalf("recovery Install()=%v", err)
	}
	if _, found, err := cache.Lookup(context.Background(), project, content); err != nil || !found {
		t.Fatalf("recovery Lookup()=%v %v", found, err)
	}
}

func TestPackageCacheCancellationPreventsPublication(t *testing.T) {
	root := t.TempDir()
	cache := newTestPackageCache(t, root, DefaultCacheLimits())
	defer cache.Close()
	plan := cacheTestPlan(t, "cancel")
	content, _ := ArchiveContentDigest(plan)
	project := digest('c')
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := cache.Install(ctx, project, content, plan); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Install() = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, string(project), string(content))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canceled publication exists: %v", err)
	}
}

func TestPackageCacheNeverOverwritesCorruptExistingContent(t *testing.T) {
	root := t.TempDir()
	cache := newTestPackageCache(t, root, DefaultCacheLimits())
	defer cache.Close()
	plan := cacheTestPlan(t, "original")
	content, _ := ArchiveContentDigest(plan)
	project := digest('b')
	if err := cache.Install(context.Background(), project, content, plan); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(root, string(project), string(content), "files", "z.txt")
	if err := os.WriteFile(filename, []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := cache.Lookup(context.Background(), project, content); !errors.Is(err, ErrCacheCorrupt) {
		t.Fatalf("corrupt Lookup() = %v", err)
	}
	if err := cache.Install(context.Background(), project, content, plan); !errors.Is(err, ErrCacheCorrupt) {
		t.Fatalf("corrupt existing Install() = %v", err)
	}
	bytes, err := os.ReadFile(filename)
	if err != nil || string(bytes) != "corrupt" {
		t.Fatalf("existing corruption was overwritten: %q, %v", bytes, err)
	}
}

func TestPackageCacheDiagnosesMissingManifestExtraFilesAndUnsafeModes(t *testing.T) {
	root := t.TempDir()
	cache := newTestPackageCache(t, root, DefaultCacheLimits())
	defer cache.Close()
	project := digest('a')
	content := digest('b')
	partial := filepath.Join(root, string(project), string(content))
	if err := os.MkdirAll(partial, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, _, err := cache.Lookup(context.Background(), project, content); !errors.Is(err, ErrCacheCorrupt) {
		t.Fatalf("missing-manifest Lookup() = %v", err)
	}
	if err := os.RemoveAll(filepath.Join(root, string(project))); err != nil {
		t.Fatal(err)
	}
	plan := cacheTestPlan(t, "diagnostic")
	content, _ = ArchiveContentDigest(plan)
	if err := cache.Install(context.Background(), project, content, plan); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(root, string(project), string(content))
	if err := os.WriteFile(filepath.Join(base, "extra"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := cache.Lookup(context.Background(), project, content); !errors.Is(err, ErrCacheCorrupt) {
		t.Fatalf("extra-file Lookup() = %v", err)
	}
	if err := os.Remove(filepath.Join(base, "extra")); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(base, "manifest.json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := cache.Lookup(context.Background(), project, content); !errors.Is(err, ErrCacheCorrupt) {
		t.Fatalf("unsafe-mode Lookup() = %v", err)
	}
}

func TestPackageCacheValidatesAddressPlanAndEveryLimit(t *testing.T) {
	cache := newTestPackageCache(t, t.TempDir(), DefaultCacheLimits())
	defer cache.Close()
	plan := cacheTestPlan(t, "limits")
	content, _ := ArchiveContentDigest(plan)
	project := digest('9')
	if err := cache.Install(context.Background(), project, digest('8'), plan); !errors.Is(err, ErrCacheMismatch) {
		t.Fatalf("mismatched-address Install() = %v", err)
	}
	bad := plan
	bad.Entries = append([]ArchiveEntry(nil), plan.Entries...)
	bad.Entries[0].Bytes = []byte("tampered")
	if err := cache.Install(context.Background(), project, content, bad); !errors.Is(err, ErrCacheMismatch) && !errors.Is(err, ErrCacheLimit) {
		t.Fatalf("tampered-plan Install() = %v", err)
	}
	bad = plan
	bad.Entries = append([]ArchiveEntry(nil), plan.Entries...)
	bad.Entries[0].Path = "../escape"
	if err := cache.Install(context.Background(), project, content, bad); !errors.Is(err, ErrCacheInvalid) {
		t.Fatalf("bad-path Install() = %v", err)
	}

	limits := DefaultCacheLimits()
	limits.MaxFiles = 1
	limited := newTestPackageCache(t, t.TempDir(), limits)
	defer limited.Close()
	if err := limited.Install(context.Background(), project, content, plan); !errors.Is(err, ErrCacheLimit) {
		t.Fatalf("file-limited Install() = %v", err)
	}
	limits = DefaultCacheLimits()
	limits.MaxFileBytes = 4
	limits.MaxTotalBytes = 16
	limitedBytes := newTestPackageCache(t, t.TempDir(), limits)
	defer limitedBytes.Close()
	if err := limitedBytes.Install(context.Background(), project, content, plan); !errors.Is(err, ErrCacheLimit) {
		t.Fatalf("byte-limited Install() = %v", err)
	}
	if cache, err := NewPackageCache(t.TempDir(), CacheLimits{}); !errors.Is(err, ErrCacheLimit) || cache != nil {
		t.Fatalf("zero-limit NewPackageCache() = %#v, %v", cache, err)
	}
}

func cacheTestPlan(t *testing.T, suffix string) ArchivePlan {
	t.Helper()
	archive := makeArchive(t,
		archiveFixture{path: "z.txt", content: []byte("last-" + suffix)},
		archiveFixture{path: "a/first.txt", content: []byte("first-" + suffix), method: 8},
	)
	plan, err := ValidateArchive(context.Background(), archive, DefaultArchiveLimits())
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func newTestPackageCache(t *testing.T, root string, limits CacheLimits) *PackageCache {
	t.Helper()
	cache, err := NewPackageCache(root, limits)
	if err != nil {
		t.Fatal(err)
	}
	return cache
}
