# Security Policy

## Supported versions

Security fixes are made on the current `main` branch and released from the
latest version line. Older pre-1.0 releases are not maintained in parallel.

## Reporting a vulnerability

Use the repository's private GitHub security-advisory reporting flow. Please do
not open a public issue for a vulnerability before a coordinated fix is
available. Include a minimal reproducer, affected API, impact, and any relevant
resource or platform assumptions. Do not include real credentials, private
keys, or sensitive documents.

## PDF trust boundaries

Treat imported PDFs as hostile. Use bounded `importpdf` options and `pdfcdr`
before retaining or opening uploaded documents. `pdfcdr` reconstructs the
supported classic-xref subset; it is not a rasterizer and does not protect a
downstream reader from vulnerabilities in image, font, or rendering decoders.

`document.ServerSafePolicy` disables PDF import and other higher-risk features
unless the caller explicitly opts in. Applications remain responsible for
request-body limits, deadlines, process isolation appropriate to their threat
model, and keeping PDF readers and the Go toolchain patched.
