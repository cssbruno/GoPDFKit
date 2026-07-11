module github.com/cssbruno/gopdfkit/examples/external-qr-code

go 1.26.5

require (
	github.com/boombuler/barcode v1.1.0
	github.com/cssbruno/gopdfkit v0.0.0
)

require golang.org/x/image v0.43.0 // indirect

replace github.com/cssbruno/gopdfkit => ../..
