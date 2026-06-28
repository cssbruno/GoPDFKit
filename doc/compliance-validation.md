# Compliance Validation Workflow

GoPDFKit can emit standards metadata, tagged PDF structure objects, and several
required catalog objects, but full PDF/A-4, PDF/UA-2, and Arlington conformance
must be verified with external validators. Treat `SetComplianceMetadata` as
generation support, not as a replacement for validation.

## Generate a Candidate PDF

For PDF/A metadata, set a real ICC output profile and use embedded UTF-8 fonts:

```go
pdf := document.New("P", "mm", "A4", "")
pdf.SetComplianceMetadata(document.ComplianceMetadata{
    PDFA:       document.PDFAMode4F,
    PDFUA2:     true,
    Arlington:  true,
    Lang:       "en-US",
    Identifier: "urn:uuid:example",
})
if err := pdf.SetOutputIntent(srgbICCBytes, "sRGB IEC61966-2.1"); err != nil {
    return err
}
pdf.AddUTF8Font("DejaVu", "", "DejaVuSansCondensed.ttf")
```

Small validator fixtures can be generated from the repository root:

```shell
make compliance-fixtures SRGB_ICC=/path/to/sRGB.icc
```

Without `SRGB_ICC`, the fixture command generates only the PDF/UA-2 and
Arlington metadata-foundation candidate. PDF/A fixtures are skipped because a
real ICC output profile is required.

## Validate PDF/A

Use veraPDF or another PDF/A validator against the generated file:

```shell
verapdf --format text output.pdf
```

PDF/A failures should block release when `PDFA` is enabled.

The repository validation wrapper runs veraPDF when it is installed:

```shell
make compliance-validate SRGB_ICC=/path/to/sRGB.icc VERAPDF='verapdf --format text'
```

For a repeatable Docker-based run, use the repository wrapper:

```shell
make compliance-validate \
  SRGB_ICC=/usr/share/color/icc/colord/sRGB.icc \
  VERAPDF='tools/verapdf-docker.sh 0'
```

`tools/verapdf-docker.sh` defaults to `verapdf/cli:v1.30.2`. Override
`VERAPDF_DOCKER_IMAGE` when intentionally testing a different validator build.

Before optional external tools run, the wrapper also executes
`go run ./cmd/compliance-check` against generated fixtures. This local checker
asserts that the expected PDF 2.0 header, metadata references, structure tree,
parent tree, MCIDs, link annotation references, image alternate text, list/table
roles, and artifact wrappers were emitted. It is a regression check, not a
replacement for standards validators.

## Validate PDF/UA

Run a PDF/UA-2-aware checker. PDF/UA validation requires tagged content,
structure-tree semantics, reading order, alternate text, table semantics, and
link annotations. Metadata alone is not enough.

```shell
pdfua-check output.pdf
```

Use the checker available in your environment; command names differ by vendor.
The Make target intentionally requires the checker command to be supplied:

```shell
make compliance-validate PDFUA_CHECKER='pdfua-check'
```

veraPDF 1.30+ can validate the generated PDF/UA-2 fixture:

```shell
make compliance-validate PDFUA_CHECKER='tools/verapdf-docker.sh ua2'
```

The generated PDF/UA fixture includes tagged PDF internals: structure tree,
marked content IDs, parent tree, role mapping, reading order by structure order,
link annotation object references, image/table/list roles, inline SVG text role
propagation, table `/Scope`/`/RowSpan`/`/ColSpan` structure attributes, artifact
marking for drawing/raw decorative content, nested list semantics inside HTML
table cells, nested table semantics inside HTML table cells, paragraph/block
semantics inside HTML table cells, templates, and imported pages, plus alternate
text for meaningful images. Passing PDF/UA-2 still depends on the external
checker and on whether the specific document uses only covered rendering paths.

## Validate Arlington PDF Model

Use the Arlington PDF Model checker, such as the veraPDF Arlington checker or
the official Arlington model tooling:

```shell
arlington-checker output.pdf
```

Arlington findings are grammar/model findings. Keep them separate from PDF/A and
PDF/UA findings because they answer a different question.

Run the wrapper with the checker available in your environment:

```shell
make compliance-validate ARLINGTON_CHECKER='arlington-checker'
```

The veraPDF Arlington Docker image exposes a REST service. Start it, then point
the wrapper at the service:

```shell
docker run -d --rm --name gopdfkit-arlington -p 8080:8080 verapdf/arlington:v1.30.1
make compliance-validate \
  ARLINGTON_CHECKER='tools/arlington-validate.sh' \
  ARLINGTON_URL='http://localhost:8080' \
  ARLINGTON_PROFILE='arlington2.0'
```

Set `REQUIRE_COMPLIANCE_TOOLS=1` in CI when missing validators should fail the
job instead of being reported as skipped.

## Track Results

Passing external validator baselines from the repository fixtures are stored in
`testdata/compliance/`:

* `verapdf-pdfa4.xml`
* `verapdf-pdfa4e.xml`
* `verapdf-pdfa4f.xml`
* `verapdf-pdfua2.xml`
* `verapdf-signed-pdfa4f-pdfua2-arlington.xml`
* `verapdf-signed-pdfua2.xml`
* `arlington-pdfa4.json`
* `arlington-pdfa4e.json`
* `arlington-pdfa4f.json`
* `arlington-pdf20.json`
* `arlington-signed-pdfa4f-pdfua2.json`

Regenerate them after compliance-output changes with the Docker wrappers above.
The local helper below regenerates fixture PDFs and validator reports into
`artifacts/compliance` using the same wrapper commands as CI:

```shell
SRGB_ICC=/usr/share/color/icc/colord/sRGB.icc make compliance-regenerate
```

Compare the current generated validator output against committed baselines with:

```shell
COMPLIANCE_OUT=artifacts/compliance make compliance-baseline-check
```

CI uploads generated compliance PDFs and validator reports from
`artifacts/compliance` as workflow artifacts, including failure cases. The
workflow has separate `Compliance Smoke` and `Compliance Strict` jobs; protected
branches should require `Compliance Strict` before merging because that job runs
the external validators and baseline comparison.

Store external findings in `document.ComplianceValidationReport`. `v0.9.0` also
provides the shorter `document.ValidationReport`, `document.ValidationIssue`,
and `document.Validator` names for production validation adapters:

```go
var report document.ValidationReport
report.Add(document.ValidationIssue{
    Standard: "Arlington",
    Severity: document.ComplianceValidationError,
    Rule:     "Catalog::Pages",
    Message:  "validator message",
    Object:   "1 0 obj",
})
if report.Failed() {
    return errors.New("compliance validation failed")
}
```

A validator adapter can implement:

```go
type Validator interface {
    ValidatePDF(data []byte) (document.ValidationReport, error)
}
```

Generation errors from `pdf.Output` and external validation failures are separate
signals. A PDF can generate successfully and still fail one or more standards
validators.
