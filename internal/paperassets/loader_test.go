package paperassets

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadManifestVerifiesExplicitRootDigestAndBytes(t *testing.T) {
	dir := t.TempDir()
	data, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==")
	digest := sha256.Sum256(data)
	if err := os.WriteFile(filepath.Join(dir, "hero.png"), data, 0600); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(dir, "assets.json")
	body := fmt.Sprintf(`{"assets":[{"name":"hero","media_type":"image/png","sha256":"%s","path":"hero.png"}]}`, hex.EncodeToString(digest[:]))
	if err := os.WriteFile(manifest, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	resources, err := LoadManifest(manifest, "")
	if err != nil || len(resources) != 1 || resources[0].Name != "hero" {
		t.Fatalf("resources=%#v %v", resources, err)
	}
	data[0] ^= 0xff
	if resources[0].Data[0] == data[0] {
		t.Fatal("loader aliases caller bytes")
	}
}

func TestLoadManifestRejectsTraversalAbsoluteSymlinkDigestAndLimits(t *testing.T) {
	dir := t.TempDir()
	data, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==")
	_ = os.WriteFile(filepath.Join(dir, "hero.png"), data, 0600)
	_ = os.Symlink("hero.png", filepath.Join(dir, "link.png"))
	digest := sha256.Sum256(data)
	for _, test := range []struct{ name, path, digest string }{{"traversal", "../hero.png", hex.EncodeToString(digest[:])}, {"absolute", filepath.Join(string(filepath.Separator), "tmp", "hero.png"), hex.EncodeToString(digest[:])}, {"symlink", "link.png", hex.EncodeToString(digest[:])}, {"digest", "hero.png", strings.Repeat("0", 64)}} {
		t.Run(test.name, func(t *testing.T) {
			manifest := filepath.Join(dir, test.name+".json")
			body := fmt.Sprintf(`{"assets":[{"name":"hero","media_type":"image/png","sha256":"%s","path":"%s"}]}`, test.digest, filepath.ToSlash(test.path))
			_ = os.WriteFile(manifest, []byte(body), 0600)
			if resources, err := LoadManifest(manifest, dir); err == nil || len(resources) != 0 {
				t.Fatalf("accepted %#v", resources)
			}
		})
	}
	large := filepath.Join(dir, "large.json")
	_ = os.WriteFile(large, []byte(`{"assets":[]}`+strings.Repeat(" ", MaxManifestBytes)), 0600)
	if _, err := LoadManifest(large, dir); err == nil {
		t.Fatal("oversized manifest accepted")
	}
}

func TestLoadProjectManifestValidatesFontsFallbackReplacementAndFocus(t *testing.T) {
	dir := t.TempDir()
	font := []byte{0, 1, 0, 0, 0, 0, 0, 1}
	image, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==")
	for name, data := range map[string][]byte{"primary.ttf": font, "fallback.ttf": font, "old.png": image, "new.png": image} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	fd, id := sha256.Sum256(font), sha256.Sum256(image)
	body := fmt.Sprintf(`{"assets":[
{"name":"body-font","media_type":"font/ttf","sha256":"%s","path":"primary.ttf","family":"Readable Sans","weight":500,"style":"normal","license":"OFL-1.1","fallback":["fallback-font"]},
{"name":"fallback-font","media_type":"font/ttf","sha256":"%s","path":"fallback.ttf","family":"Fallback Sans","license":"OFL-1.1"},
{"name":"old-hero","media_type":"image/png","sha256":"%s","path":"old.png","focus_x":0.25,"focus_y":0.75},
{"name":"new-hero","media_type":"image/png","sha256":"%s","path":"new.png","replaces":"old-hero"}]}`, hex.EncodeToString(fd[:]), hex.EncodeToString(fd[:]), hex.EncodeToString(id[:]), hex.EncodeToString(id[:]))
	manifest := filepath.Join(dir, "project.json")
	if err := os.WriteFile(manifest, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	resources, err := LoadProjectManifest(manifest, dir)
	if err != nil || len(resources) != 4 {
		t.Fatalf("resources=%#v err=%v", resources, err)
	}
	by := map[string]ProjectResource{}
	for _, r := range resources {
		by[r.Name] = r
	}
	if by["body-font"].Family != "Readable Sans" || by["fallback-font"].Weight != 400 || by["new-hero"].Replaces != "old-hero" || by["old-hero"].FocusX == nil {
		t.Fatalf("metadata=%#v", by)
	}
	images, err := LoadManifest(manifest, dir)
	if err != nil || len(images) != 2 {
		t.Fatalf("image catalog=%#v err=%v", images, err)
	}
	production, err := LoadManifestResources(manifest, dir)
	if err != nil || len(production) != 4 || production[0].Family == "" {
		t.Fatalf("production catalog=%#v err=%v", production, err)
	}
}

func TestLoadProjectManifestRejectsLifecycleCycles(t *testing.T) {
	dir := t.TempDir()
	font := []byte{0, 1, 0, 0, 0, 0, 0, 1}
	digest := sha256.Sum256(font)
	_ = os.WriteFile(filepath.Join(dir, "a.ttf"), font, 0600)
	_ = os.WriteFile(filepath.Join(dir, "b.ttf"), font, 0600)
	body := fmt.Sprintf(`{"assets":[{"name":"a","media_type":"font/ttf","sha256":"%s","path":"a.ttf","family":"A","license":"OFL-1.1","fallback":["b"]},{"name":"b","media_type":"font/ttf","sha256":"%s","path":"b.ttf","family":"B","license":"OFL-1.1","fallback":["a"]}]}`, hex.EncodeToString(digest[:]), hex.EncodeToString(digest[:]))
	manifest := filepath.Join(dir, "cycle.json")
	_ = os.WriteFile(manifest, []byte(body), 0600)
	if _, err := LoadProjectManifest(manifest, dir); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("cycle error=%v", err)
	}
}

func TestLoadProjectManifestEnforcesClosedFontLicensePolicy(t *testing.T) {
	dir := t.TempDir()
	font := []byte{0, 1, 0, 0, 0, 0, 0, 1}
	digest := sha256.Sum256(font)
	if err := os.WriteFile(filepath.Join(dir, "font.ttf"), font, 0600); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{"assets":[{"name":"font","media_type":"font/ttf","sha256":"%s","path":"font.ttf","family":"Readable Sans","license":"Unknown-License"}]}`, hex.EncodeToString(digest[:]))
	manifest := filepath.Join(dir, "license.json")
	if err := os.WriteFile(manifest, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadProjectManifest(manifest, dir); err == nil || !strings.Contains(err.Error(), "outside the enforced policy") {
		t.Fatalf("license policy error = %v", err)
	}
}

func TestProjectManifestAddRemovePublishesValidatedCanonicalCatalog(t *testing.T) {
	dir := t.TempDir()
	image, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==")
	if err := os.WriteFile(filepath.Join(dir, "hero.png"), image, 0600); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(dir, "project.json")
	if err := os.WriteFile(manifest, []byte(`{"assets":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	resources, err := AddProjectResource(manifest, dir, ResourceSpec{Name: "hero", MediaType: "image/png", Path: "hero.png", FocusX: floatPointer(0.5), FocusY: floatPointer(0.25)})
	if err != nil || len(resources) != 1 || resources[0].Path != "hero.png" || resources[0].Digest == "" {
		t.Fatalf("added resources=%#v err=%v", resources, err)
	}
	encoded, err := os.ReadFile(manifest)
	if err != nil || !strings.Contains(string(encoded), `"name": "hero"`) || !strings.Contains(string(encoded), `"focus_x": 0.5`) {
		t.Fatalf("published manifest=%s err=%v", encoded, err)
	}
	if _, err := AddProjectResource(manifest, dir, ResourceSpec{Name: "bad", MediaType: "image/png", Path: "../hero.png"}); err == nil {
		t.Fatal("traversal add accepted")
	}
	resources, err = RemoveProjectResource(manifest, dir, "hero")
	if err != nil || len(resources) != 0 {
		t.Fatalf("removed resources=%#v err=%v", resources, err)
	}
	loaded, err := LoadProjectManifest(manifest, dir)
	if err != nil || len(loaded) != 0 {
		t.Fatalf("reloaded resources=%#v err=%v", loaded, err)
	}
}

func floatPointer(value float64) *float64 { return &value }
