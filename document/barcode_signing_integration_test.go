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
	"errors"
	"io"
	"math/big"
	"testing"
	"time"

	"github.com/cssbruno/gopdfkit/sign"
)

type shortSignedWriter struct{}

func (shortSignedWriter) Write(p []byte) (int, error) {
	return len(p) / 2, nil
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

func TestOutputSignedRejectsNilWriter(t *testing.T) {
	pdf := New("P", "mm", "A4", "")

	if err := pdf.OutputSigned(nil, sign.Options{}); !errors.Is(err, ErrNilWriter) {
		t.Fatalf("OutputSigned(nil) error = %v, want ErrNilWriter", err)
	}
}

func TestOutputSignedDetectsShortWrite(t *testing.T) {
	cert, signer := rootTestSigner(t)

	pdf := New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	pdf.Cell(40, 10, "short write")

	err := pdf.OutputSigned(shortSignedWriter{}, sign.Options{
		Signer:          signer,
		Certificate:     cert,
		DigestAlgorithm: crypto.SHA256,
		SigningTime:     time.Now().UTC(),
	})
	if !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("OutputSigned(short writer) error = %v, want ErrShortWrite", err)
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
