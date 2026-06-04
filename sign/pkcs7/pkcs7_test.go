// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package pkcs7

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

func TestAdobeRevocationInfoWrappers(t *testing.T) {
	if !AdobeRevocationInfoArchivalOID().Equal(asn1.ObjectIdentifier{1, 2, 840, 113583, 1, 1, 8}) {
		t.Fatalf("AdobeRevocationInfoArchivalOID() = %v", AdobeRevocationInfoArchivalOID())
	}

	ocspDER := []byte{0x30, 0x03, 0x02, 0x01, 0x01}
	var info RevocationInfo
	if err := info.AddOCSP(ocspDER); err != nil {
		t.Fatalf("AddOCSP() error = %v", err)
	}

	encoded, err := asn1.Marshal(info)
	if err != nil {
		t.Fatalf("asn1.Marshal() error = %v", err)
	}
	decoded, err := DecodeAdobeRevocationInfo(asn1.RawValue{FullBytes: encoded})
	if err != nil {
		t.Fatalf("DecodeAdobeRevocationInfo() error = %v", err)
	}
	if len(decoded.OCSP) != 1 || !bytes.Equal(decoded.OCSP[0].FullBytes, ocspDER) {
		t.Fatalf("DecodeAdobeRevocationInfo().OCSP = %#v, want %x", decoded.OCSP, ocspDER)
	}

	extracted, err := ExtractAdobeRevocationInfo([]byte("not cms"))
	if err == nil || len(extracted.OCSP) != 0 {
		t.Fatalf("ExtractAdobeRevocationInfo() = (%+v, %v), want error", extracted, err)
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
