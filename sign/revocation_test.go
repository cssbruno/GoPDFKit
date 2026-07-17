// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package sign

import (
	"bytes"
	"crypto"
	"encoding/asn1"
	"testing"
	"time"
)

func TestAdobeRevocationInfoArchivalOID(t *testing.T) {
	if !AdobeRevocationInfoArchivalOID().Equal(asn1.ObjectIdentifier{1, 2, 840, 113583, 1, 1, 8}) {
		t.Fatalf("AdobeRevocationInfoArchivalOID() = %v", AdobeRevocationInfoArchivalOID())
	}
}

func TestDecodeAdobeRevocationInfo(t *testing.T) {
	ocspDER := []byte{0x30, 0x03, 0x02, 0x01, 0x01}
	crlDER := []byte{0x30, 0x03, 0x02, 0x01, 0x02}

	var info RevocationInfo
	if err := info.AddOCSP(ocspDER); err != nil {
		t.Fatalf("AddOCSP() error = %v", err)
	}
	if err := info.AddCRL(crlDER); err != nil {
		t.Fatalf("AddCRL() error = %v", err)
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
	if len(decoded.CRL) != 1 || !bytes.Equal(decoded.CRL[0].FullBytes, crlDER) {
		t.Fatalf("DecodeAdobeRevocationInfo().CRL = %#v, want %x", decoded.CRL, crlDER)
	}
}

func TestDecodeAdobeRevocationInfoRejectsEmptyValue(t *testing.T) {
	if _, err := DecodeAdobeRevocationInfo(asn1.RawValue{}); err == nil {
		t.Fatal("DecodeAdobeRevocationInfo() error = nil, want error")
	}
}

func TestExtractAdobeRevocationInfoReturnsEmptyWhenAttributeMissing(t *testing.T) {
	cert, signer := testSigner(t)
	cms, err := CreateCMS([]byte("content without revocation attribute"), CMSOptions{
		Signer:          signer,
		Certificate:     cert,
		DigestAlgorithm: crypto.SHA256,
		Detached:        true,
		SigningTime:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateCMS() error = %v", err)
	}

	info, err := ExtractAdobeRevocationInfo(cms)
	if err != nil {
		t.Fatalf("ExtractAdobeRevocationInfo() error = %v", err)
	}
	if len(info.OCSP) != 0 {
		t.Fatalf("len(OCSP) = %d, want 0", len(info.OCSP))
	}
	if len(info.CRL) != 0 {
		t.Fatalf("len(CRL) = %d, want 0", len(info.CRL))
	}
}
