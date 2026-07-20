// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"
)

func TestDocumentStatsSummarizeResources(t *testing.T) {
	pdf := MustNew()
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	pdf.Cell(20, 5, "stats")
	pdf.RegisterImageOptionsReader("pixel", ImageOptions{ImageType: "png"}, bytes.NewReader(decodeTinyPNG(t)))
	pdf.SetAttachments([]Attachment{{Content: []byte("global"), Filename: "global.txt"}})
	pdf.AddAttachmentAnnotation(&Attachment{Content: []byte("annotated"), Filename: "annotated.txt"}, 10, 10, 20, 5)
	if err := pdf.Error(); err != nil {
		t.Fatalf("document setup error = %v", err)
	}

	stats := pdf.Stats()
	if stats.Pages != 1 {
		t.Fatalf("Pages = %d, want 1", stats.Pages)
	}
	if stats.Images != 1 {
		t.Fatalf("Images = %d, want 1", stats.Images)
	}
	if stats.Fonts == 0 {
		t.Fatal("Fonts = 0, want registered font count")
	}
	if stats.Attachments != 2 {
		t.Fatalf("Attachments = %d, want global and annotation attachments", stats.Attachments)
	}
	if stats.EstimatedMemoryBytes <= 0 {
		t.Fatalf("EstimatedMemoryBytes = %d, want positive estimate", stats.EstimatedMemoryBytes)
	}
}

func TestResourceStoreOwnsDocumentResources(t *testing.T) {
	pdf := MustNew()
	resources := pdf.ensureResourceStore()
	if resources == nil {
		t.Fatal("resources store is nil")
	}

	info := &ImageInfo{i: "img"}
	resources.setImage("img", info)
	if got, ok := resources.image("img"); !ok || got != info {
		t.Fatal("resourceStore did not retain image")
	}

	resources.setFont("font", fontDefinition{Tp: "core"})
	if _, ok := resources.font("font"); !ok {
		t.Fatal("resourceStore did not retain font")
	}

	resources.addImportedObject("object", []byte("payload"))
	if string(resources.importedObjectData("object")) != "payload" {
		t.Fatal("resourceStore did not retain imported object")
	}
}

func TestResourceStoreReceivesRegisteredResourceWrites(t *testing.T) {
	pdf := MustNew()

	info, err := pdf.RegisterImageOptionsReaderContext(t.Context(), "pixel", ImageOptions{ImageType: "png"}, bytes.NewReader(decodeTinyPNG(t)))
	if err != nil {
		t.Fatalf("RegisterImageOptionsReaderContext() error = %v", err)
	}
	if pdf.resources.images["pixel"] != info {
		t.Fatal("registered image was not stored in resourceStore")
	}

	tpl := statsTestTemplateView{id: "template", size: Size{Wd: 10, Ht: 10}, data: []byte("q\nQ")}
	pdf.registerTemplate(tpl)
	if pdf.resources.templates["template"] == nil {
		t.Fatal("registered template was not stored in resourceStore")
	}

	objs := map[string][]byte{"object": []byte("payload")}
	pos := map[string]map[int]string{"object": {1: "old"}}
	pdf.ImportObjects(objs)
	pdf.ImportObjPos(pos)
	pdf.ImportTemplates(map[string]string{"/Tpl1": "object"})

	objs["object"][0] = 'X'
	pos["object"][1] = "new"

	if got := string(pdf.resources.importedObjs["object"]); got != "payload" {
		t.Fatalf("imported object in resourceStore = %q, want payload", got)
	}
	if got := pdf.resources.importedObjPos["object"][1]; got != "old" {
		t.Fatalf("imported object position in resourceStore = %q, want old", got)
	}
	if got := pdf.resources.importedTplObjs["/Tpl1"]; got != "object" {
		t.Fatalf("imported template in resourceStore = %q, want object", got)
	}
}

func TestResourceStoreReceivesAttachmentCacheWrites(t *testing.T) {
	pdf := MustNew()
	pdf.AddPage()
	pdf.SetAttachments([]Attachment{{
		Content:     []byte("attachment payload"),
		Filename:    "payload.txt",
		Description: "payload",
	}})

	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	if len(pdf.resources.attachments.streams) == 0 {
		t.Fatal("attachment stream object cache was not stored in resourceStore")
	}
	if len(pdf.resources.attachments.files) == 0 {
		t.Fatal("attachment filespec object cache was not stored in resourceStore")
	}
	if len(pdf.resources.attachments.compressed) == 0 {
		t.Fatal("compressed attachment cache was not stored in resourceStore")
	}
}

func TestResourceStoreReceivesFontWrites(t *testing.T) {
	pdf := MustNew()
	pdf.SetFont("Helvetica", "", 12)
	if _, ok := pdf.resources.font("helvetica"); !ok {
		t.Fatal("core font was not stored in resourceStore")
	}

	cache := NewFontCache()
	cache.fonts[getFontKey("cached", "")] = cachedUTF8Font{
		def:  fontDefinition{Tp: "UTF8", Name: "cached", Cw: []int{0, 500}},
		data: []byte("font-data"),
	}
	pdf = MustNew()
	if err := pdf.AddUTF8FontFromCacheError("cached", "", cache); err != nil {
		t.Fatalf("AddUTF8FontFromCacheError() error = %v", err)
	}
	font, ok := pdf.resources.font("cached")
	if !ok {
		t.Fatal("cached UTF-8 font was not stored in resourceStore")
	}
	if font.Tp != "UTF8" || font.utf8File == nil {
		t.Fatalf("cached font = %#v, want UTF8 font with runtime file", font)
	}
}

func TestResourceStoreDeterministicIterationHelpers(t *testing.T) {
	store := newResourceStore()
	store.setFont("z", fontDefinition{i: "2", Name: "z"})
	store.setFont("a", fontDefinition{i: "1", Name: "a"})
	store.setFont("same-z", fontDefinition{i: "same", Name: "same-z"})
	store.setFont("same-a", fontDefinition{i: "same", Name: "same-a"})
	fonts := store.fontsByResourceID(true)
	if len(fonts) != 4 ||
		fonts[0].i != "1" ||
		fonts[1].i != "2" ||
		fonts[2].Name != "same-a" ||
		fonts[3].Name != "same-z" {
		t.Fatalf("fontsByResourceID() = %#v, want resource-id then key order", fonts)
	}
	fonts = store.fontsByKey(true)
	if len(fonts) != 4 || fonts[0].Name != "a" || fonts[3].Name != "z" {
		t.Fatalf("fontsByKey() = %#v, want key order", fonts)
	}

	store.setImage("wide", &ImageInfo{i: "b", w: 20})
	store.setImage("narrow", &ImageInfo{i: "a", w: 10})
	store.setImage("same-z", &ImageInfo{i: "same", w: 30, n: 30})
	store.setImage("same-a", &ImageInfo{i: "same", w: 30, n: 31})
	images := store.imagesForOutput(true)
	if len(images) != 4 ||
		images[0].i != "a" ||
		images[1].i != "b" ||
		images[2].n != 31 ||
		images[3].n != 30 {
		t.Fatalf("imagesForOutput() = %#v, want width, resource-id, then key order", images)
	}
	images = store.imagesByResourceID(true)
	if len(images) != 4 ||
		images[0].i != "a" ||
		images[1].i != "b" ||
		images[2].n != 31 ||
		images[3].n != 30 {
		t.Fatalf("imagesByResourceID() = %#v, want resource-id then key order", images)
	}
	if got := store.templateOutputImage("missing", "missing", &ImageInfo{i: "same"}); got == nil || got.n != 31 {
		t.Fatalf("templateOutputImage() = %#v, want deterministic same-a image", got)
	}

	store.addImportedObject("z", []byte("z"))
	store.addImportedObject("a", []byte("a"))
	hashes := store.importedObjectHashes(true)
	if len(hashes) != 2 || hashes[0] != "a" || hashes[1] != "z" {
		t.Fatalf("importedObjectHashes() = %#v, want sorted hashes", hashes)
	}

	store.addImportedTemplates(map[string]string{"/TplZ": "z", "/TplA": "a"})
	store.setImportedTemplateObjectID("z", 12)
	store.setImportedTemplateObjectID("a", 11)
	refs := store.importedTemplateResourceRefs(true)
	if len(refs) != 2 || refs[0].name != "/TplA" || refs[0].objectNumber != 11 || refs[1].name != "/TplZ" || refs[1].objectNumber != 12 {
		t.Fatalf("importedTemplateResourceRefs() = %#v, want sorted PDF resource refs", refs)
	}
}

func TestResourceLoaderRegistersImages(t *testing.T) {
	var gotKind ResourceKind
	var gotName string
	loader := ResourceLoaderFunc(func(ctx context.Context, kind ResourceKind, name string) (io.ReadCloser, ResourceInfo, error) {
		if err := outputCanceledError(ctx); err != nil {
			return nil, ResourceInfo{}, err
		}
		gotKind = kind
		gotName = name
		data := decodeTinyPNG(t)
		return io.NopCloser(bytes.NewReader(data)), ResourceInfo{Size: int64(len(data)), StableID: "tiny"}, nil
	})
	pdf, err := NewDocument(WithResourceLoader(loader))
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}
	info, err := pdf.RegisterImageOptionsError("loader.png", ImageOptions{})
	if err != nil {
		t.Fatalf("RegisterImageOptionsError() error = %v", err)
	}
	if gotKind != ResourceImage || gotName != "loader.png" {
		t.Fatalf("loader call = (%q, %q), want (%q, loader.png)", gotKind, gotName, ResourceImage)
	}
	if info == nil || pdf.resources.images["loader.png"] != info {
		t.Fatal("resource-loaded image was not stored in resourceStore")
	}
}

func TestResourceLoaderStableIDCachesImages(t *testing.T) {
	cache := NewImageCache()
	data := decodeTinyPNG(t)
	var opens int
	var bytesRead int
	loader := ResourceLoaderFunc(func(ctx context.Context, kind ResourceKind, name string) (io.ReadCloser, ResourceInfo, error) {
		if err := outputCanceledError(ctx); err != nil {
			return nil, ResourceInfo{}, err
		}
		if kind != ResourceImage {
			t.Fatalf("loader kind = %q, want %q", kind, ResourceImage)
		}
		opens++
		return countingReadCloser{
			Reader: bytes.NewReader(data),
			onRead: func(n int) {
				bytesRead += n
			},
		}, ResourceInfo{Size: int64(len(data)), StableID: "tiny-image-v1"}, nil
	})

	first, err := NewDocument(WithResourceLoader(loader), WithImageCache(cache))
	if err != nil {
		t.Fatalf("NewDocument(first) error = %v", err)
	}
	if _, err := first.RegisterImageOptionsError("virtual.png", ImageOptions{}); err != nil {
		t.Fatalf("first RegisterImageOptionsError() error = %v", err)
	}
	firstBytesRead := bytesRead
	if firstBytesRead == 0 {
		t.Fatal("first resource-loaded image was not read")
	}

	second, err := NewDocument(WithResourceLoader(loader), WithImageCache(cache))
	if err != nil {
		t.Fatalf("NewDocument(second) error = %v", err)
	}
	if _, err := second.RegisterImageOptionsError("virtual.png", ImageOptions{}); err != nil {
		t.Fatalf("second RegisterImageOptionsError() error = %v", err)
	}
	if opens != 2 {
		t.Fatalf("loader opens = %d, want 2 metadata opens", opens)
	}
	if bytesRead != firstBytesRead {
		t.Fatalf("bytes read after cache hit = %d, want %d", bytesRead, firstBytesRead)
	}
}

func TestResourceLoaderImageSourceLimit(t *testing.T) {
	loader := ResourceLoaderFunc(func(context.Context, ResourceKind, string) (io.ReadCloser, ResourceInfo, error) {
		return io.NopCloser(bytes.NewReader(nil)), ResourceInfo{Size: 128}, nil
	})
	pdf, err := NewDocument(
		WithResourceLoader(loader),
		WithLimits(Limits{MaxImageSourceBytes: 16}),
	)
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}
	_, err = pdf.RegisterImageOptionsError("too-large.png", ImageOptions{})
	if !errors.Is(err, ErrImageTooLarge) {
		t.Fatalf("RegisterImageOptionsError() error = %v, want ErrImageTooLarge", err)
	}
}

func TestResourceLoaderLoadsFileBackedAttachments(t *testing.T) {
	var gotKind ResourceKind
	var gotName string
	loader := ResourceLoaderFunc(func(ctx context.Context, kind ResourceKind, name string) (io.ReadCloser, ResourceInfo, error) {
		if err := outputCanceledError(ctx); err != nil {
			return nil, ResourceInfo{}, err
		}
		gotKind = kind
		gotName = name
		data := []byte("loaded attachment")
		return io.NopCloser(bytes.NewReader(data)), ResourceInfo{Size: int64(len(data))}, nil
	})
	pdf, err := NewDocument(WithResourceLoader(loader))
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}
	pdf.AddPage()
	pdf.SetAttachments([]Attachment{AttachmentFromFile("virtual.txt")})

	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	if gotKind != ResourceAttachment || gotName != "virtual.txt" {
		t.Fatalf("loader call = (%q, %q), want (%q, virtual.txt)", gotKind, gotName, ResourceAttachment)
	}
}

func TestResourceLoaderLoadsUTF8Fonts(t *testing.T) {
	fontBytes, err := os.ReadFile("../assets/static/font/DejaVuSansCondensed.ttf")
	if err != nil {
		t.Fatalf("ReadFile(font) error = %v", err)
	}
	var gotKind ResourceKind
	var gotName string
	loader := ResourceLoaderFunc(func(ctx context.Context, kind ResourceKind, name string) (io.ReadCloser, ResourceInfo, error) {
		if err := outputCanceledError(ctx); err != nil {
			return nil, ResourceInfo{}, err
		}
		gotKind = kind
		gotName = name
		return io.NopCloser(bytes.NewReader(fontBytes)), ResourceInfo{Size: int64(len(fontBytes)), StableID: "dejavu"}, nil
	})
	pdf, err := NewDocument(WithResourceLoader(loader))
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}
	if err := pdf.AddUTF8FontError("LoaderFont", "", "virtual.ttf"); err != nil {
		t.Fatalf("AddUTF8FontError() error = %v", err)
	}
	if gotKind != ResourceFont || gotName != "virtual.ttf" {
		t.Fatalf("loader call = (%q, %q), want (%q, virtual.ttf)", gotKind, gotName, ResourceFont)
	}
	if _, ok := pdf.resources.font("loaderfont"); !ok {
		t.Fatal("resource-loaded UTF-8 font was not stored in resourceStore")
	}
}

func TestResourceLoaderStableIDCachesSharedUTF8Fonts(t *testing.T) {
	ClearSharedCaches()
	t.Cleanup(ClearSharedCaches)

	fontBytes, err := os.ReadFile("../assets/static/font/DejaVuSansCondensed.ttf")
	if err != nil {
		t.Fatalf("ReadFile(font) error = %v", err)
	}
	var opens int
	var bytesRead int
	loader := ResourceLoaderFunc(func(ctx context.Context, kind ResourceKind, name string) (io.ReadCloser, ResourceInfo, error) {
		if err := outputCanceledError(ctx); err != nil {
			return nil, ResourceInfo{}, err
		}
		if kind != ResourceFont {
			t.Fatalf("loader kind = %q, want %q", kind, ResourceFont)
		}
		opens++
		return countingReadCloser{
			Reader: bytes.NewReader(fontBytes),
			onRead: func(n int) {
				bytesRead += n
			},
		}, ResourceInfo{Size: int64(len(fontBytes)), StableID: "dejavu-font-v1"}, nil
	})

	for i := 0; i < 2; i++ {
		pdf, err := NewDocument(
			WithResourceCachePolicy(ResourceCacheShared),
			WithResourceLoader(loader),
		)
		if err != nil {
			t.Fatalf("NewDocument(%d) error = %v", i, err)
		}
		if err := pdf.AddUTF8FontError("LoaderFont", "", "virtual.ttf"); err != nil {
			t.Fatalf("AddUTF8FontError(%d) error = %v", i, err)
		}
		if i == 0 && bytesRead == 0 {
			t.Fatal("first resource-loaded UTF-8 font was not read")
		}
	}
	if opens != 2 {
		t.Fatalf("loader opens = %d, want 2 metadata opens", opens)
	}
	if bytesRead != len(fontBytes) {
		t.Fatalf("bytes read = %d, want one font read of %d bytes", bytesRead, len(fontBytes))
	}
}

func TestResourceLoaderImportsPDFs(t *testing.T) {
	source := MustNew(WithUnit(UnitPoint))
	source.AddPage()
	source.SetFont("Helvetica", "", 12)
	source.Text(72, 96, "resource-loaded import")
	var sourceBytes bytes.Buffer
	if err := source.Output(&sourceBytes); err != nil {
		t.Fatalf("source Output() error = %v", err)
	}

	var gotKind ResourceKind
	var gotName string
	loader := ResourceLoaderFunc(func(ctx context.Context, kind ResourceKind, name string) (io.ReadCloser, ResourceInfo, error) {
		if err := outputCanceledError(ctx); err != nil {
			return nil, ResourceInfo{}, err
		}
		gotKind = kind
		gotName = name
		data := sourceBytes.Bytes()
		return io.NopCloser(bytes.NewReader(data)), ResourceInfo{Size: int64(len(data))}, nil
	})
	pdf, err := NewDocument(WithResourceLoader(loader))
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}
	pageID, err := pdf.ImportPageError("virtual.pdf", 1, "MediaBox")
	if err != nil {
		t.Fatalf("ImportPageError() error = %v", err)
	}
	if pageID == 0 {
		t.Fatal("ImportPageError() page ID = 0")
	}
	if gotKind != ResourcePDFImport || gotName != "virtual.pdf" {
		t.Fatalf("loader call = (%q, %q), want (%q, virtual.pdf)", gotKind, gotName, ResourcePDFImport)
	}
	sizes := pdf.GetPageSizes("virtual.pdf")
	if pdf.Err() || len(sizes) != 1 {
		t.Fatalf("GetPageSizes() = %#v, error = %v; want one page", sizes, pdf.Error())
	}
}

type countingReadCloser struct {
	*bytes.Reader
	onRead func(int)
}

func (r countingReadCloser) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if n > 0 && r.onRead != nil {
		r.onRead(n)
	}
	return n, err
}

func (r countingReadCloser) Close() error {
	return nil
}

func TestStatsInitializesResourceStoreForBareDocument(t *testing.T) {
	pdf := &Document{}
	stats := pdf.Stats()
	if stats != (DocumentStats{}) {
		t.Fatalf("Stats() for bare document = %#v, want zero stats", stats)
	}
	if pdf.resources == nil {
		t.Fatal("Stats() did not initialize resource store for bare document")
	}
	if pdf.resources.images == nil || pdf.resources.fonts == nil || pdf.resources.templates == nil || pdf.resources.importedPages == nil {
		t.Fatal("Stats() did not initialize resource maps for bare document")
	}
}

type statsTestTemplateView struct {
	id   string
	size Size
	data []byte
}

func (t statsTestTemplateView) ID() string { return t.id }

func (t statsTestTemplateView) Size() (Point, Size) { return Point{}, t.size }

func (t statsTestTemplateView) Bytes() []byte { return append([]byte(nil), t.data...) }

func (t statsTestTemplateView) Images() map[string]*ImageInfo { return nil }

func TestExplicitCacheStatsAndClear(t *testing.T) {
	cache := NewImageCache()
	if _, err := cache.RegisterImageOptionsReader("pixel", ImageOptions{ImageType: "png"}, bytes.NewReader(decodeTinyPNG(t))); err != nil {
		t.Fatalf("RegisterImageOptionsReader() error = %v", err)
	}
	if stats := cache.Stats(); stats.Entries != 1 || stats.Bytes <= 0 {
		t.Fatalf("image cache Stats() = %#v, want one retained image", stats)
	}
	cache.Clear()
	if stats := cache.Stats(); stats.Entries != 0 || stats.Bytes != 0 {
		t.Fatalf("image cache after Clear() = %#v, want empty", stats)
	}
}

func TestImageCacheStatsDeduplicateFileAliases(t *testing.T) {
	cache := NewImageCache()
	path := t.TempDir() + "/pixel.png"
	if err := os.WriteFile(path, decodeTinyPNG(t), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := cache.RegisterImageOptions("pixel", path, ImageOptions{}); err != nil {
		t.Fatalf("RegisterImageOptions(pixel) error = %v", err)
	}
	if _, err := cache.RegisterImageOptions("pixel-alias", path, ImageOptions{}); err != nil {
		t.Fatalf("RegisterImageOptions(pixel-alias) error = %v", err)
	}
	info, ok := cache.Get("pixel")
	if !ok {
		t.Fatal("cached image was not available by name")
	}
	if stats := cache.Stats(); stats.Entries != 1 || stats.Bytes != imageInfoCacheBytes(info) {
		t.Fatalf("image cache Stats() = %#v, want one unique retained image payload", stats)
	}
}

func TestClearSharedCaches(t *testing.T) {
	ClearSharedCaches()
	if stats := SharedCacheStats(); stats.Images.Entries != 0 || stats.Fonts.Entries != 0 || stats.HTML.Entries != 0 {
		t.Fatalf("SharedCacheStats() = %#v, want empty shared caches", stats)
	}
}
