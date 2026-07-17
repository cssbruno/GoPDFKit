# Compliance Baselines

These reports are generated from `cmd/compliance-fixtures` with:

* veraPDF CLI 1.30.2 for PDF/A-4, PDF/A-4e, PDF/A-4f, signed PDF/A-4f,
  PDF/UA-2, and signed PDF/UA-2
* veraPDF Arlington REST 1.30.1 for the Arlington PDF 2.0 profile against all
  generated compliance fixtures
* `/usr/share/color/icc/colord/sRGB.icc` as the real sRGB ICC output profile

They are fixture baselines for external validation. Regenerate them after
changing compliance output and keep `REQUIRE_COMPLIANCE_TOOLS=1` passing in CI.

`make compliance-fixtures SRGB_ICC=/path/to/a/real/sRGB.icc` also writes the
unsigned fixtures' deterministic local structural report to
`artifacts/compliance/characterization.json`. It records the reproduction
command and runtime fingerprint. The local report complements these archived
external reports and is not itself a PDF/A or PDF/UA validator result.
