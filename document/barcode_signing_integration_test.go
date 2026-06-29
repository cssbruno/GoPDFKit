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
	"os"
	"path/filepath"
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

func TestOutputSignedIgnoresStreamFinalPolicy(t *testing.T) {
	cert, signer := rootTestSigner(t)
	truststore := x509.NewCertPool()
	truststore.AddCert(cert)

	pdf, err := NewDocument(WithOutputPolicy(OutputPolicy{StreamFinal: true}))
	if err != nil {
		t.Fatalf("NewDocument() error = %v", err)
	}
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	pdf.Cell(40, 10, "Signed with StreamFinal policy")

	var signed bytes.Buffer
	err = pdf.OutputSigned(&signed, sign.Options{
		Signer:          signer,
		Certificate:     cert,
		DigestAlgorithm: crypto.SHA256,
		SigningTime:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("OutputSigned() error = %v", err)
	}
	if pdf.streamedOutput {
		t.Fatal("OutputSigned should not consume the document through StreamFinal")
	}
	if _, err := sign.Verify(signed.Bytes(), truststore); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}

	var repeated bytes.Buffer
	if err := pdf.Output(&repeated); err != nil {
		t.Fatalf("Output() after OutputSigned() error = %v", err)
	}
	if !bytes.HasPrefix(repeated.Bytes(), []byte("%PDF-")) {
		t.Fatalf("Output() after OutputSigned() wrote non-PDF prefix %q", repeated.Bytes()[:min(repeated.Len(), 8)])
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

func TestOutputSignedFileDoesNotTruncateDestinationOnSigningError(t *testing.T) {
	fileStr := filepath.Join(t.TempDir(), "signed.pdf")
	original := []byte("previous signed output")
	if err := os.WriteFile(fileStr, original, 0o600); err != nil {
		t.Fatal(err)
	}

	pdf := New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	pdf.Cell(40, 10, "signing failure")

	err := pdf.OutputSignedFile(fileStr, sign.Options{})
	if !errors.Is(err, sign.ErrMissingSigner) {
		t.Fatalf("OutputSignedFile() error = %v, want ErrMissingSigner", err)
	}
	got, err := os.ReadFile(fileStr)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("OutputSignedFile() changed destination on failure: got %q, want %q", got, original)
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
