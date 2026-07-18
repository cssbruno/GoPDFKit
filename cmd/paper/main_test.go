// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/paperscenario"
)

const validSource = "document @report:\n" +
	"  page @sheet:\n" +
	"    margin: 24pt\n" +
	"    body @body:\n" +
	"      paragraph @intro:\n" +
	"        text @copy: \"Hello plan\"\n"

const scenarioRepeatSource = `document @doc:
  schema @invoice:
    field @items:
      type: "list"
      item-type: "object"
      max-items: 4
      field @active:
        type: "bool"
      field @name:
        type: "string"
  scenario @preview:
    keyed-list @items:
      object @alpha:
        value @active: true
        value @name: "Alpha"
      object @beta:
        value @active: false
        value @name: "Beta"
      object @gamma:
        value @active: true
        value @name: "Gamma"
  page:
    body:
      repeat @visible:
        source: "@invoice.items"
        instance-prefix: "preview-lines"
        max-items: 2
        when: "active"
        paragraph @line:
          bind: "name"
          text: "Scenario line"
`

const externalDataSource = `document @report:
  language: "pt-BR"
  schema @lab:
    field @patient:
      type: "string"
    field @results:
      type: "list"
      item-type: "object"
      max-items: 6
      field @name:
        type: "string"
      field @value:
        type: "number"
  page:
    size: "A4"
    margin: 24pt
    body:
      heading @patient:
        level: 1
        bind: "@lab.patient"
        text: "Patient"
      repeat @results:
        source: "@lab.results"
        instance-prefix: "results"
        max-items: 6
        paragraph @result:
          bind: "name"
          text: "Result"
`

func invoke(args []string, input string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := run(args, strings.NewReader(input), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

const assetImageSource = `document @report:
  scenario @preview:
    value @approved: true
  page @sheet:
    width: 100pt
    height: 80pt
    margin: 8pt
    body @body:
      image @hero:
        source: "asset:hero-image"
        width: 20pt
        height: 20pt
        alt: "Evidence"
`

func writeAssetFixture(t *testing.T) (manifest, root string) {
	t.Helper()
	manifestDir := t.TempDir()
	root = t.TempDir()
	data, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "hero.png"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	payload, err := json.Marshal(map[string]any{"assets": []map[string]string{{
		"name": "hero-image", "media_type": "image/png", "sha256": hex.EncodeToString(digest[:]), "path": "hero.png",
	}}})
	if err != nil {
		t.Fatal(err)
	}
	manifest = filepath.Join(manifestDir, "assets.json")
	if err := os.WriteFile(manifest, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	return manifest, root
}

func writeFontAssetFixture(t *testing.T) (manifest, root string) {
	t.Helper()
	font, err := os.ReadFile("../../assets/static/font/calligra.ttf")
	if err != nil {
		t.Fatal(err)
	}
	root = t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "body.ttf"), font, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(font)
	payload, err := json.Marshal(map[string]any{"assets": []map[string]any{{
		"name": "body-font", "media_type": "font/ttf", "sha256": hex.EncodeToString(digest[:]), "path": "body.ttf",
		"family": "Specimen Sans", "weight": 400, "style": "normal", "license": "OFL-1.1",
	}}})
	if err != nil {
		t.Fatal(err)
	}
	manifest = filepath.Join(root, "assets.json")
	if err := os.WriteFile(manifest, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	return manifest, root
}

func TestRunFmtAndCheckJSON(t *testing.T) {
	const unformatted = "document:\n  page:\n    size: \"A4\"\n    margin: 10pt\n    body:\n      text: \"hello\"\n"
	code, stdout, stderr := invoke([]string{"fmt", "-"}, unformatted)
	if code != exitOK || stderr != "" || !strings.Contains(stdout, "    margin: 10pt\n    size: \"A4\"") {
		t.Fatalf("fmt = %d, %q, %q", code, stdout, stderr)
	}

	path := filepath.Join(t.TempDir(), "format.paper")
	if err := os.WriteFile(path, []byte(unformatted), 0o640); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr = invoke([]string{"fmt", "-w", "--json", path}, "")
	if code != exitOK || stderr != "" || !strings.Contains(stdout, `"changed":true`) {
		t.Fatalf("fmt -w = %d, %q, %q", code, stdout, stderr)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("formatted mode = %v, %v", info, err)
	}

	code, stdout, stderr = invoke([]string{"check", "--json", "-"}, "document:\n  page\n")
	if code != exitFailure || stderr != "" || !strings.Contains(stdout, `"ok":false`) || !strings.Contains(stdout, `"diagnostics"`) {
		t.Fatalf("invalid check = %d, %q, %q", code, stdout, stderr)
	}
}

func TestRunCheckAndRenderExternalJSONAndGeneratedEdges(t *testing.T) {
	const source = `document @report:
  schema @invoice:
    field @customer:
      type: "string"
    field @items:
      type: "list"
      item-type: "object"
      max-items: 4
      field @name:
        type: "string"
  page:
    body:
      heading:
        bind: "@invoice.customer"
        text: "Customer"
      repeat @items-repeat:
        source: "@invoice.items"
        instance-prefix: "items"
        max-items: 4
        paragraph @item-name:
          bind: "name"
          text: "Item"
`
	dir := t.TempDir()
	paperFile := filepath.Join(dir, "invoice.paper")
	dataFile := filepath.Join(dir, "invoice.json")
	if err := os.WriteFile(paperFile, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dataFile, []byte(`{"customer":"Ana","items":[{"name":"One"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := invoke([]string{"check", "--json", "--data", dataFile, "--schema", "invoice", paperFile}, "")
	if code != exitOK || stderr != "" || !strings.Contains(stdout, `"ok":true`) {
		t.Fatalf("external JSON check = %d, %q, %q", code, stdout, stderr)
	}
	pdfFile := filepath.Join(dir, "invoice.pdf")
	code, stdout, stderr = invoke([]string{"render", "--json", "--data", dataFile, "-o", pdfFile, paperFile}, "")
	if code != exitOK || stderr != "" || !strings.Contains(stdout, `"ok":true`) {
		t.Fatalf("external JSON render = %d, %q, %q", code, stdout, stderr)
	}
	pdf, err := os.ReadFile(pdfFile)
	if err != nil || !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		t.Fatalf("external JSON PDF = %d bytes, %v", len(pdf), err)
	}

	edgeDir := filepath.Join(dir, "edges")
	code, stdout, stderr = invoke([]string{"check", "--json", "--edge-cases", "3", "--seed", "42", "--edge-max-items", "3", "--edge-output", edgeDir, paperFile}, "")
	if code != exitOK || stderr != "" || !strings.Contains(stdout, `"ok":true`) || !strings.Contains(stdout, `"name":"dense-lists"`) {
		t.Fatalf("edge check = %d, %q, %q", code, stdout, stderr)
	}
	jsonFiles, _ := filepath.Glob(filepath.Join(edgeDir, "*.json"))
	pdfFiles, _ := filepath.Glob(filepath.Join(edgeDir, "*.pdf"))
	if len(jsonFiles) != 3 || len(pdfFiles) != 3 {
		t.Fatalf("edge artifacts = %d JSON / %d PDF", len(jsonFiles), len(pdfFiles))
	}

	code, stdout, stderr = invoke([]string{"check", "--json", "--edge-cases", "4", "--seed", "42", paperFile}, "")
	if code != exitFailure || stderr != "" || !strings.Contains(stdout, `"name":"unicode-pt-br"`) || !strings.Contains(stdout, `"code":"PAPER_PLAN_UNSUPPORTED"`) {
		t.Fatalf("failing edge report = %d, %q, %q", code, stdout, stderr)
	}
}

func TestRunRenderWritesDeterministicPDFAtomically(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.pdf")
	second := filepath.Join(dir, "second.pdf")
	for _, output := range []string{first, second} {
		code, stdout, stderr := invoke([]string{"render", "-o", output, "-"}, validSource)
		if code != exitOK || stdout != "" || stderr != "" {
			t.Fatalf("render %s = %d, %q, %q", output, code, stdout, stderr)
		}
	}
	one, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	two, err := os.ReadFile(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(one, []byte("%PDF-")) || !bytes.Equal(one, two) {
		t.Fatal("rendered output is not a deterministic PDF")
	}
	temporary, err := filepath.Glob(filepath.Join(dir, ".*.tmp-*"))
	if err != nil || len(temporary) != 0 {
		t.Fatalf("temporary output leaked: %v, %v", temporary, err)
	}

	code, stdout, stderr := invoke([]string{"render", "--json", "-o", first, "-"}, validSource)
	if code != exitOK || stderr != "" || !strings.Contains(stdout, `"ok":true`) || !strings.Contains(stdout, `"hash"`) {
		t.Fatalf("render JSON = %d, %q, %q", code, stdout, stderr)
	}
}

func TestOperationalCommandsUseExplicitAssetCatalog(t *testing.T) {
	manifest, root := writeAssetFixture(t)
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "asset.pdf")

	code, stdout, stderr := invoke([]string{"render", "--json", "--assets", manifest, "--asset-root", root, "-o", pdfPath, "-"}, assetImageSource)
	if code != exitOK || stderr != "" || !strings.Contains(stdout, `"ok":true`) {
		t.Fatalf("asset render = %d, %q, %q", code, stdout, stderr)
	}
	pdf, err := os.ReadFile(pdfPath)
	if err != nil || !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		t.Fatalf("asset PDF = %d bytes, %v", len(pdf), err)
	}

	code, stdout, stderr = invoke([]string{"capture", "--json", "--scenario", "preview", "--assets", manifest, "--asset-root", root, "-"}, assetImageSource)
	if code != exitOK || stderr != "" || !strings.Contains(stdout, `"plan_hash"`) || !strings.Contains(stdout, `"artifact_count":1`) {
		t.Fatalf("scenario asset capture = %d, %q, %q", code, stdout, stderr)
	}

	code, stdout, stderr = invoke([]string{"check", "--json", "--scenario", "preview", "--assets", manifest, "--asset-root", root, "-"}, assetImageSource)
	if code != exitOK || stderr != "" || !strings.Contains(stdout, `"ok":true`) {
		t.Fatalf("scenario asset check = %d, %q, %q", code, stdout, stderr)
	}
	code, stdout, stderr = invoke([]string{"explain", "--json", "--scenario", "preview", "--key", "@hero", "--assets", manifest, "--asset-root", root, "-"}, assetImageSource)
	if code != exitOK || stderr != "" || !strings.Contains(stdout, `"key":"@hero"`) {
		t.Fatalf("scenario asset explain = %d, %q, %q", code, stdout, stderr)
	}
}

func TestOperationalCommandsPreserveFontManifestMetadata(t *testing.T) {
	manifest, root := writeFontAssetFixture(t)
	const source = `document @report:
  page:
    body:
      paragraph:
        font: "Specimen Sans"
        text: "Embedded font"
`
	output := filepath.Join(t.TempDir(), "font.pdf")
	code, stdout, stderr := invoke([]string{"render", "--json", "--assets", manifest, "--asset-root", root, "-o", output, "-"}, source)
	if code != exitOK || stderr != "" || !strings.Contains(stdout, `"ok":true`) {
		t.Fatalf("font render = %d, %q, %q", code, stdout, stderr)
	}
	pdf, err := os.ReadFile(output)
	if err != nil || !bytes.Contains(pdf, []byte("/FontFile2")) {
		t.Fatalf("font PDF = %d bytes, %v", len(pdf), err)
	}
}

func TestOperationalCommandsRejectUnsafeOrUnboundAssets(t *testing.T) {
	manifest, root := writeAssetFixture(t)
	validManifest, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("missing catalog", func(t *testing.T) {
		code, stdout, stderr := invoke([]string{"check", "--json", "-"}, assetImageSource)
		if code != exitFailure || stderr != "" || !strings.Contains(stdout, `"code":"PAPER_COMPILE_IMAGE_SOURCE"`) {
			t.Fatalf("missing catalog = %d, %q, %q", code, stdout, stderr)
		}
	})

	t.Run("asset root without manifest", func(t *testing.T) {
		code, stdout, stderr := invoke([]string{"check", "--json", "--asset-root", root, "-"}, assetImageSource)
		if code != exitFailure || stderr != "" || !strings.Contains(stdout, "--asset-root requires --assets") {
			t.Fatalf("unbound root = %d, %q, %q", code, stdout, stderr)
		}
	})

	t.Run("digest mismatch", func(t *testing.T) {
		bad := filepath.Join(t.TempDir(), "assets.json")
		payload := bytes.Replace(validManifest, []byte(`"sha256":"`), []byte(`"sha256":"0`), 1)
		if err := os.WriteFile(bad, payload, 0o600); err != nil {
			t.Fatal(err)
		}
		code, stdout, stderr := invoke([]string{"check", "--json", "--assets", bad, "--asset-root", root, "-"}, assetImageSource)
		if code != exitFailure || stderr != "" || !strings.Contains(stdout, "digest does not match") {
			t.Fatalf("digest mismatch = %d, %q, %q", code, stdout, stderr)
		}
	})

	t.Run("traversal", func(t *testing.T) {
		outside := filepath.Join(filepath.Dir(root), "outside.png")
		data, err := os.ReadFile(filepath.Join(root, "hero.png"))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(outside, data, 0o600); err != nil {
			t.Fatal(err)
		}
		payload := bytes.Replace(validManifest, []byte(`"path":"hero.png"`), []byte(`"path":"../outside.png"`), 1)
		bad := filepath.Join(t.TempDir(), "assets.json")
		if err := os.WriteFile(bad, payload, 0o600); err != nil {
			t.Fatal(err)
		}
		code, stdout, stderr := invoke([]string{"capture", "--json", "--assets", bad, "--asset-root", root, "-"}, assetImageSource)
		if code != exitFailure || stderr != "" || !strings.Contains(stdout, "traverses the root") {
			t.Fatalf("traversal = %d, %q, %q", code, stdout, stderr)
		}
	})

	t.Run("symlink", func(t *testing.T) {
		if err := os.Symlink(filepath.Join(root, "hero.png"), filepath.Join(root, "linked.png")); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		payload := bytes.Replace(validManifest, []byte(`"path":"hero.png"`), []byte(`"path":"linked.png"`), 1)
		bad := filepath.Join(t.TempDir(), "assets.json")
		if err := os.WriteFile(bad, payload, 0o600); err != nil {
			t.Fatal(err)
		}
		code, stdout, stderr := invoke([]string{"render", "--json", "--assets", bad, "--asset-root", root, "-o", filepath.Join(t.TempDir(), "bad.pdf"), "-"}, assetImageSource)
		if code != exitFailure || stderr != "" || !strings.Contains(stdout, "symlink components are forbidden") {
			t.Fatalf("symlink = %d, %q, %q", code, stdout, stderr)
		}
	})
}

func TestRunCaptureAndExplainUsePlanTools(t *testing.T) {
	code, stdout, stderr := invoke([]string{"capture", "-"}, validSource)
	if code != exitOK || stderr != "" || !strings.HasPrefix(stdout, "<?xml") || !strings.Contains(stdout, `data-source-mode="geometry_svg"`) {
		t.Fatalf("capture = %d, %q, %q", code, stdout, stderr)
	}

	code, stdout, stderr = invoke([]string{"capture", "--json", "-"}, validSource)
	if code != exitOK || stderr != "" {
		t.Fatalf("capture JSON = %d, %q, %q", code, stdout, stderr)
	}
	var capture struct {
		PlanHash  string `json:"plan_hash"`
		Artifacts []struct {
			SVG string `json:"svg"`
		} `json:"artifacts"`
	}
	if err := json.Unmarshal([]byte(stdout), &capture); err != nil || capture.PlanHash == "" || len(capture.Artifacts) != 1 || !strings.HasPrefix(capture.Artifacts[0].SVG, "<?xml") {
		t.Fatalf("capture bundle = %#v, %v", capture, err)
	}

	code, stdout, stderr = invoke([]string{"explain", "--key", "@intro", "-"}, validSource)
	if code != exitOK || stderr != "" || !strings.Contains(stdout, `"plan_hash"`) || !strings.Contains(stdout, `"key":"@intro"`) {
		t.Fatalf("explain = %d, %q, %q", code, stdout, stderr)
	}
}

func TestRunScenarioSelectsResolvedFixture(t *testing.T) {
	const source = "document:\n" +
		"  scenario @child:\n" +
		"    parent: \"base\"\n" +
		"    locale: \"pt-BR\"\n" +
		"    value @paid: true\n" +
		"  scenario @base:\n" +
		"    value @currency: \"USD\"\n" +
		"  page:\n" +
		"    body:\n" +
		"      text: \"preview\"\n"
	code, stdout, stderr := invoke([]string{"scenario", "--json", "--scenario", "child", "-"}, source)
	if code != exitOK || stderr != "" {
		t.Fatalf("scenario = %d, %q, %q", code, stdout, stderr)
	}
	var fixture paperscenario.Fixture
	if err := json.Unmarshal([]byte(stdout), &fixture); err != nil || fixture.Name != "child" || fixture.Locale != "pt-BR" || len(fixture.Values) != 2 || fixture.Digest == "" {
		t.Fatalf("fixture = %#v, %v", fixture, err)
	}
}

func TestOperationalCommandsSelectScenarioRepeat(t *testing.T) {
	code, stdout, stderr := invoke([]string{"check", "--json", "--scenario", "preview", "-"}, scenarioRepeatSource)
	if code != exitOK || stderr != "" || !strings.Contains(stdout, `"ok":true`) || !strings.Contains(stdout, `"pages":1`) {
		t.Fatalf("scenario check = %d, %q, %q", code, stdout, stderr)
	}

	output := filepath.Join(t.TempDir(), "scenario.pdf")
	code, stdout, stderr = invoke([]string{"render", "--json", "--scenario", "@preview", "-o", output, "-"}, scenarioRepeatSource)
	if code != exitOK || stderr != "" || !strings.Contains(stdout, `"ok":true`) {
		t.Fatalf("scenario render = %d, %q, %q", code, stdout, stderr)
	}
	pdf, err := os.ReadFile(output)
	if err != nil || !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		t.Fatalf("scenario PDF = %d bytes, %v", len(pdf), err)
	}

	code, stdout, stderr = invoke([]string{"capture", "--json", "--scenario", "preview", "-"}, scenarioRepeatSource)
	if code != exitOK || stderr != "" || !strings.Contains(stdout, `"plan_hash"`) || !strings.Contains(stdout, `"artifact_count":1`) {
		t.Fatalf("scenario capture = %d, %q, %q", code, stdout, stderr)
	}

	code, stdout, stderr = invoke([]string{"explain", "--json", "--scenario", "preview", "--instance", "preview-lines[alpha]", "-"}, scenarioRepeatSource)
	if code != exitOK || stderr != "" || !strings.Contains(stdout, `"instance":"preview-lines[alpha]"`) {
		t.Fatalf("scenario explain = %d, %q, %q", code, stdout, stderr)
	}
}

func TestCheckAndRenderAcceptStrictExternalJSONData(t *testing.T) {
	dir := t.TempDir()
	template := filepath.Join(dir, "lab.paper")
	data := filepath.Join(dir, "lab.json")
	output := filepath.Join(dir, "lab.pdf")
	if err := os.WriteFile(template, []byte(externalDataSource), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(data, []byte(`{"patient":"Ana","results":[{"name":"Hemoglobina","value":12.5}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := invoke([]string{"check", "--json", "--data", data, template}, "")
	if code != exitOK || stderr != "" || !strings.Contains(stdout, `"ok":true`) || !strings.Contains(stdout, `"pages":1`) {
		t.Fatalf("data check = %d, %q, %q", code, stdout, stderr)
	}
	code, stdout, stderr = invoke([]string{"render", "--json", "--data", data, "-o", output, template}, "")
	if code != exitOK || stderr != "" || !strings.Contains(stdout, `"ok":true`) {
		t.Fatalf("data render = %d, %q, %q", code, stdout, stderr)
	}
	rendered, err := os.ReadFile(output)
	if err != nil || !bytes.HasPrefix(rendered, []byte("%PDF-")) || !bytes.Contains(rendered, []byte("%%EOF")) {
		t.Fatalf("rendered data PDF = %d bytes, %v", len(rendered), err)
	}

	if err := os.WriteFile(data, []byte(`{"patient":42,"results":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr = invoke([]string{"check", "--json", "--data", data, template}, "")
	if code != exitFailure || stderr != "" || !strings.Contains(stdout, `"code":"PAPER_DATA_JSON"`) || !strings.Contains(stdout, `#/patient`) {
		t.Fatalf("invalid data check = %d, %q, %q", code, stdout, stderr)
	}
}

func TestCheckGeneratesReproducibleEdgeCasesAndCompletePDFs(t *testing.T) {
	dir := t.TempDir()
	template := filepath.Join(dir, "lab.paper")
	if err := os.WriteFile(template, []byte(externalDataSource), 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"check", "--json", "--edge-cases", "3", "--edge-max-items", "4", "--seed", "42", template}
	firstCode, firstOutput, firstError := invoke(args, "")
	secondCode, secondOutput, secondError := invoke(args, "")
	if firstCode != exitOK || secondCode != exitOK || firstError != "" || secondError != "" || firstOutput != secondOutput {
		t.Fatalf("edge checks = (%d, %q), (%d, %q), equal=%v", firstCode, firstError, secondCode, secondError, firstOutput == secondOutput)
	}
	var report edgeCheckResult
	if err := json.Unmarshal([]byte(firstOutput), &report); err != nil || !report.OK || report.Seed != 42 || report.Schema != "@lab" || len(report.Cases) != 3 {
		t.Fatalf("report = %#v, %v", report, err)
	}
	for _, checked := range report.Cases {
		if !checked.OK || checked.PlanHash == "" || checked.Pages == 0 || checked.PDFBytes == 0 {
			t.Fatalf("edge case = %#v", checked)
		}
	}

	outputDir := filepath.Join(dir, "edge-output")
	code, stdout, stderr := invoke([]string{"check", "--json", "--edge-cases", "2", "--seed", "42", "--edge-output", outputDir, template}, "")
	if code != exitOK || stderr != "" {
		t.Fatalf("edge output = %d, %q, %q", code, stdout, stderr)
	}
	files, err := filepath.Glob(filepath.Join(outputDir, "*"))
	if err != nil || len(files) != 4 {
		t.Fatalf("generated files = %#v, %v", files, err)
	}
	for _, name := range files {
		payload, readErr := os.ReadFile(name)
		if readErr != nil || len(payload) == 0 {
			t.Fatalf("generated %s = %d bytes, %v", name, len(payload), readErr)
		}
	}
}

func TestOperationalCommandsDiagnoseInvalidScenario(t *testing.T) {
	output := filepath.Join(t.TempDir(), "missing.pdf")
	tests := []struct {
		name string
		args []string
	}{
		{name: "check", args: []string{"check", "--json", "--scenario", "missing", "-"}},
		{name: "render", args: []string{"render", "--json", "--scenario", "missing", "-o", output, "-"}},
		{name: "capture", args: []string{"capture", "--json", "--scenario", "missing", "-"}},
		{name: "explain", args: []string{"explain", "--json", "--scenario", "missing", "--page", "1", "-"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			code, stdout, stderr := invoke(test.args, scenarioRepeatSource)
			if code != exitFailure || stderr != "" || !strings.Contains(stdout, `"code":"PAPER_REPEAT_SCENARIO_UNKNOWN"`) {
				t.Fatalf("invalid scenario = %d, %q, %q", code, stdout, stderr)
			}
		})
	}
}

func TestRunUsageAndSourceLimit(t *testing.T) {
	code, _, stderr := invoke(nil, "")
	if code != exitUsage || !strings.Contains(stderr, "usage:") {
		t.Fatalf("no args = %d, %q", code, stderr)
	}
	code, stdout, stderr := invoke([]string{"fmt", "--json", "-"}, strings.Repeat("x", maxSourceBytes+1))
	if code != exitFailure || stderr != "" || !strings.Contains(stdout, "source exceeds") {
		t.Fatalf("source limit = %d, %q, %q", code, stdout, stderr)
	}
}

func TestRunWorkflowCompletesApprovedHeadlessEditReviewAndExport(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "workflow.paper")
	literal := filepath.Join(dir, "literal.txt")
	output := filepath.Join(dir, "workflow.pdf")
	if err := os.WriteFile(source, []byte(validSource), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(literal, []byte("Headless edited value"), 0o600); err != nil {
		t.Fatal(err)
	}
	font := "../../assets/static/font/DejaVuSansCondensed.ttf"
	hash := strings.Repeat("a", 64)
	code, stdout, stderr := invoke([]string{"workflow", "--target", "@intro", "--literal-file", literal,
		"--font", font, "-o", output, "--actor", "agent:test", "--scenario-result-hash", hash,
		"--validator-hash", hash, "--approval-nonce", "cli-reviewer-nonce-0001", "--approve", source}, "")
	if code != exitOK || stderr != "" || strings.Contains(stdout, "Headless edited value") ||
		!strings.Contains(stdout, `"acceptance_hash":"`) || !strings.Contains(stdout, `"export_audit_hash":"`) {
		t.Fatalf("workflow = %d, %q, %q", code, stdout, stderr)
	}
	pdf, err := os.ReadFile(output)
	if err != nil || !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		t.Fatalf("workflow PDF = %d bytes, %v", len(pdf), err)
	}
	info, err := os.Stat(output)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("workflow PDF mode = %v, %v", info, err)
	}
}
