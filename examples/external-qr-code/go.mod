module github.com/cssbruno/gopdfkit/examples/external-qr-code

go 1.25.0

require (
	github.com/boombuler/barcode v1.0.0
	github.com/cssbruno/gopdfkit v0.0.0
)

require golang.org/x/image v0.41.0 // indirect

replace github.com/cssbruno/gopdfkit => ../..
