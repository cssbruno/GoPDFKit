# GoPDFKit Examples

Each directory is a runnable example:

```sh
go run ./examples/hello-world
go run ./examples/drawing
go run ./examples/headers-footers
go run ./examples/html-fragment
go run ./examples/image-from-memory
go run ./examples/import-page
go run ./examples/protection-attachments
go run ./examples/structured-report
go run ./examples/sign-pdf
go run ./examples/templates
go run ./examples/thumbnail
go run ./examples/utf8-font
```

The `external-qr-code` example is a separate module because it intentionally
uses a barcode dependency outside GoPDFKit:

```sh
cd examples/external-qr-code
go run .
```

Generated PDFs are written beside each example's `main.go`.
