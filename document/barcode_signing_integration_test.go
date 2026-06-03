// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"math/big"
	"testing"
	"time"

	"github.com/cssbruno/gopdfkit/barcode"
	"github.com/cssbruno/gopdfkit/sign"
)

func TestRootBarcodeIntegration(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.AddPage()

	key, err := barcode.QR("https://example.test/verify", barcode.High, barcode.Unicode)
	if err != nil {
		t.Fatalf("barcode.QR() error = %v", err)
	}
	if key == "" {
		t.Fatal("barcode.QR() key is empty")
	}
	width, height := pdf.GetUnscaledBarcodeDimensions(key)
	if width <= 0 || height <= 0 {
		t.Fatalf("GetUnscaledBarcodeDimensions() = %v, %v; want positive dimensions", width, height)
	}
	pdf.Barcode(key, 10, 10, 24, 24, false)

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	if !bytes.Contains(output.Bytes(), []byte("/Subtype /Image")) {
		t.Fatal("barcode output did not embed an image")
	}
}

func TestOutputSignedIntegration(t *testing.T) {
	cert, signer := rootTestSigner(t)
	truststore := x509.NewCertPool()
	truststore.AddCert(cert)

	pdf := New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	pdf.Cell(40, 10, "Signed from document API")

	var signed bytes.Buffer
	err := pdf.OutputSigned(&signed, sign.Options{
		Signer:          signer,
		Certificate:     cert,
		DigestAlgorithm: crypto.SHA256,
		Name:            "Root API Signer",
		SigningTime:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("OutputSigned() error = %v", err)
	}

	signature, err := sign.Verify(signed.Bytes(), truststore)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if !signature.CMS.ValidSignature {
		t.Fatal("signed output CMS signature is not valid")
	}
	if !signature.CMS.TrustedSigner {
		t.Fatal("signed output CMS signer is not trusted")
	}
}

func rootTestSigner(t *testing.T) (*x509.Certificate, crypto.Signer) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Root API PDF Signer"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageContentCommitment,
		UnknownExtKeyUsage:    []asn1.ObjectIdentifier{{1, 3, 6, 1, 5, 5, 7, 3, 36}},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	if err != nil {
		t.Fatalf("CreateCertificate() error = %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("ParseCertificate() error = %v", err)
	}
	return cert, key
}
