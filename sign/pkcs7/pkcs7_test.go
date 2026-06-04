// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package pkcs7

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"math/big"
	"testing"
	"time"
)

var oidDocumentSigning = asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 3, 36}

func TestCreateAndVerifyDetached(t *testing.T) {
	cert, signer := testSigner(t)
	content := []byte("legacy terminology wrapper")
	truststore := x509.NewCertPool()
	truststore.AddCert(cert)

	cms, err := Create(content, Options{
		Signer:          signer,
		Certificate:     cert,
		DigestAlgorithm: crypto.SHA256,
		Detached:        true,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	result, err := VerifyDetached(cms, content, truststore)
	if err != nil {
		t.Fatalf("VerifyDetached() error = %v", err)
	}
	if !result.ValidSignature || !result.Detached {
		t.Fatalf("VerifyDetached() result = %+v, want valid detached signature", result)
	}
}

func testSigner(t *testing.T) (*x509.Certificate, crypto.Signer) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}

	now := time.Now().UTC()
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "Test PDF Signer",
		},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageContentCommitment,
		UnknownExtKeyUsage:    []asn1.ObjectIdentifier{oidDocumentSigning},
		BasicConstraintsValid: true,
	}

	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate() error = %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("ParseCertificate() error = %v", err)
	}
	return cert, key
}
