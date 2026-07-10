// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package sign

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestBytesRequiresSigner(t *testing.T) {
	cert, _ := testSigner(t)
	_, err := Bytes(testPDFBytes(t), Options{Certificate: cert})
	if !errors.Is(err, ErrMissingSigner) {
		t.Fatalf("Bytes() error = %v, want ErrMissingSigner", err)
	}
}

func TestBytesRejectsSHA1Digest(t *testing.T) {
	cert, signer := testSigner(t)
	_, err := Bytes(testPDFBytes(t), Options{
		Signer:          signer,
		Certificate:     cert,
		DigestAlgorithm: crypto.SHA1,
	})
	if err == nil {
		t.Fatal("Bytes() accepted SHA-1 digest")
	}
	if !strings.Contains(err.Error(), "insecure digest") {
		t.Fatalf("Bytes() error = %v, want insecure digest error", err)
	}
}

func TestBytesSupportsAdobePKCS7DetachedSubFilter(t *testing.T) {
	cert, signer := testSigner(t)
	signedPDF, err := Bytes(testPDFBytes(t), Options{
		Signer:          signer,
		Certificate:     cert,
		DigestAlgorithm: crypto.SHA256,
		SubFilter:       SubFilterAdobePKCS7Detached,
		SigningTime:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("Bytes() error = %v", err)
	}
	if !bytes.Contains(signedPDF, []byte("/SubFilter /adbe.pkcs7.detached")) {
		t.Fatal("signed PDF does not contain requested Adobe PKCS#7 detached SubFilter")
	}
	if bytes.Contains(signedPDF, []byte("/Extensions << /ESIC")) {
		t.Fatal("Adobe PKCS#7 detached signature should not add the PAdES extension marker")
	}
}

func TestBytesRejectsUnsupportedSubFilter(t *testing.T) {
	cert, signer := testSigner(t)
	_, err := Bytes(testPDFBytes(t), Options{
		Signer:          signer,
		Certificate:     cert,
		DigestAlgorithm: crypto.SHA256,
		SubFilter:       "unsupported",
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported SubFilter") {
		t.Fatalf("Bytes() error = %v, want unsupported SubFilter error", err)
	}
}

func TestBytesAndVerify(t *testing.T) {
	cert, signer := testSigner(t)
	truststore := x509.NewCertPool()
	truststore.AddCert(cert)

	signedPDF, err := Bytes(testPDFBytes(t), Options{
		Signer:          signer,
		Certificate:     cert,
		DigestAlgorithm: crypto.SHA256,
		Name:            "Test Signer",
		Reason:          "Unit test",
		SigningTime:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("Bytes() error = %v", err)
	}
	if !bytes.Contains(signedPDF, []byte("/ByteRange")) {
		t.Fatal("signed PDF does not contain a signature byte range")
	}
	if !bytes.Contains(signedPDF, []byte("/AcroForm")) {
		t.Fatal("signed PDF does not contain an AcroForm")
	}
	if !bytes.Contains(signedPDF, []byte("/SubFilter /ETSI.CAdES.detached")) {
		t.Fatal("signed PDF does not advertise a CMS/CAdES detached signature")
	}
	if !bytes.Contains(signedPDF, []byte("/Extensions << /ESIC << /BaseVersion /1.7 /ExtensionLevel 1 >> >>")) {
		t.Fatal("signed PDF does not advertise the ETSI PAdES developer extension")
	}

	signature, err := Verify(signedPDF, truststore)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if len(signature.ByteRange) != 4 {
		t.Fatalf("len(ByteRange) = %d, want 4", len(signature.ByteRange))
	}
	if !signature.CMS.ValidSignature {
		t.Fatal("CMS signature is not valid")
	}
	if !signature.CMS.TrustedSigner {
		t.Fatal("CMS signer is not trusted")
	}
}

func TestExtractSignatureAndEmbedDetachedCMS(t *testing.T) {
	cert, signer := testSigner(t)
	truststore := x509.NewCertPool()
	truststore.AddCert(cert)

	signedPDF, err := Bytes(testPDFBytes(t), Options{
		Signer:          signer,
		Certificate:     cert,
		DigestAlgorithm: crypto.SHA256,
		SigningTime:     time.Now().UTC(),
		SignatureSize:   64 << 10,
	})
	if err != nil {
		t.Fatalf("Bytes() error = %v", err)
	}

	extracted, err := ExtractSignature(signedPDF)
	if err != nil {
		t.Fatalf("ExtractSignature() error = %v", err)
	}
	if extracted.MaxSignatureBytes() < len(extracted.CMS) {
		t.Fatalf("MaxSignatureBytes() = %d, CMS len = %d", extracted.MaxSignatureBytes(), len(extracted.CMS))
	}

	cms, err := CreateCMS(extracted.SignedContent, CMSOptions{
		Signer:          signer,
		Certificate:     cert,
		DigestAlgorithm: crypto.SHA256,
		Detached:        true,
		SigningTime:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateCMS() error = %v", err)
	}

	embedded, err := EmbedDetachedCMS(signedPDF, cms)
	if err != nil {
		t.Fatalf("EmbedDetachedCMS() error = %v", err)
	}
	if _, err := Verify(embedded, truststore); err != nil {
		t.Fatalf("Verify(embedded) error = %v", err)
	}

	reextracted, err := ExtractSignature(embedded)
	if err != nil {
		t.Fatalf("ExtractSignature(embedded) error = %v", err)
	}
	if !bytes.Equal(reextracted.CMS, cms) {
		t.Fatal("embedded CMS was not extracted exactly")
	}
}

func TestDigestHexMatchesSignedContent(t *testing.T) {
	cert, signer := testSigner(t)
	signedPDF, err := Bytes(testPDFBytes(t), Options{
		Signer:          signer,
		Certificate:     cert,
		DigestAlgorithm: crypto.SHA256,
		SigningTime:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("Bytes() error = %v", err)
	}

	digestHex, err := DigestHex(signedPDF, crypto.SHA256)
	if err != nil {
		t.Fatalf("DigestHex() error = %v", err)
	}
	extracted, err := ExtractSignature(signedPDF)
	if err != nil {
		t.Fatalf("ExtractSignature() error = %v", err)
	}
	sum := sha256.Sum256(extracted.SignedContent)
	if digestHex != hex.EncodeToString(sum[:]) {
		t.Fatalf("DigestHex() = %s, want %s", digestHex, hex.EncodeToString(sum[:]))
	}
}

func TestByteRangeHelpersAndExtractSingleSignature(t *testing.T) {
	cert, signer := testSigner(t)
	signedPDF, err := Bytes(testPDFBytes(t), Options{
		Signer:          signer,
		Certificate:     cert,
		DigestAlgorithm: crypto.SHA256,
		SigningTime:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("Bytes() error = %v", err)
	}

	if count := SignatureCount(signedPDF); count != 1 {
		t.Fatalf("SignatureCount() = %d, want 1", count)
	}

	extracted, err := ExtractSingleSignature(signedPDF)
	if err != nil {
		t.Fatalf("ExtractSingleSignature() error = %v", err)
	}

	byteRange64, err := extracted.ByteRange64()
	if err != nil {
		t.Fatalf("ByteRange64() error = %v", err)
	}

	byteRangeInts, err := ByteRangeToInts(byteRange64)
	if err != nil {
		t.Fatalf("ByteRangeToInts() error = %v", err)
	}
	if !slices.Equal(byteRangeInts, extracted.ByteRange) {
		t.Fatalf("ByteRangeToInts() = %v, want %v", byteRangeInts, extracted.ByteRange)
	}

	content, err := SignedContentForByteRange(signedPDF, byteRange64)
	if err != nil {
		t.Fatalf("SignedContentForByteRange() error = %v", err)
	}
	if !bytes.Equal(content, extracted.SignedContent) {
		t.Fatal("SignedContentForByteRange() did not match extracted content")
	}

	digestHex, err := DigestHexForByteRange(signedPDF, byteRange64, crypto.SHA256)
	if err != nil {
		t.Fatalf("DigestHexForByteRange() error = %v", err)
	}
	sum := sha256.Sum256(extracted.SignedContent)
	if digestHex != hex.EncodeToString(sum[:]) {
		t.Fatalf("DigestHexForByteRange() = %s, want %s", digestHex, hex.EncodeToString(sum[:]))
	}
}

func TestExtractSingleSignatureRejectsAmbiguousByteRanges(t *testing.T) {
	pdf := []byte("%PDF-1.7\n1 0 obj\n<< /ByteRange [ 0 10 20 10 ] /Contents<00> >>\nendobj\n2 0 obj\n<< /ByteRange [ 0 10 20 10 ] /Contents<00> >>\nendobj\n")

	if count := SignatureCount(pdf); count != 2 {
		t.Fatalf("SignatureCount() = %d, want 2", count)
	}

	_, err := ExtractSingleSignature(pdf)
	if err == nil || !strings.Contains(err.Error(), "ambiguous ByteRange markers") {
		t.Fatalf("ExtractSingleSignature() error = %v, want ambiguous ByteRange error", err)
	}
}

func TestDigestHexForByteRangeRejectsOverflow(t *testing.T) {
	_, err := DigestHexForByteRange([]byte("pdf"), [4]int64{9223372036854775807, 1, 0, 0}, crypto.SHA256)
	if err == nil || !strings.Contains(err.Error(), "ByteRange") {
		t.Fatalf("DigestHexForByteRange() error = %v, want ByteRange error", err)
	}
}

func TestDecodeCMS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		input        string
		want         string
		wantEncoding string
	}{
		{
			name:         "base64 std",
			input:        "c2lnbmF0dXJl",
			want:         "signature",
			wantEncoding: "base64/std",
		},
		{
			name:         "data url base64",
			input:        "data:application/cms-signature;base64,c2lnbmF0dXJl",
			want:         "signature",
			wantEncoding: "data-url/base64/std",
		},
		{
			name: "pem",
			input: string(pem.EncodeToMemory(&pem.Block{
				Type:  "CMS",
				Bytes: []byte("signature"),
			})),
			want:         "signature",
			wantEncoding: "pem",
		},
		{
			name:         "whitespace",
			input:        "c2ln\nbmF0dXJl\t",
			want:         "signature",
			wantEncoding: "base64/std",
		},
		{
			name:         "raw base64",
			input:        "dGVzdA",
			want:         "test",
			wantEncoding: "base64/raw",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			out, encoding, err := DecodeCMS(tt.input)
			if err != nil {
				t.Fatalf("DecodeCMS() error = %v", err)
			}
			if string(out) != tt.want {
				t.Fatalf("DecodeCMS() = %q, want %q", string(out), tt.want)
			}
			if encoding != tt.wantEncoding {
				t.Fatalf("encoding = %q, want %q", encoding, tt.wantEncoding)
			}
		})
	}
}

func TestDecodeCMSRejectsEmptyInput(t *testing.T) {
	t.Parallel()

	_, encoding, err := DecodeCMS(" \n\t")
	if err == nil {
		t.Fatal("DecodeCMS() error = nil, want error")
	}
	if encoding != "empty" {
		t.Fatalf("encoding = %q, want empty", encoding)
	}
}

func TestInspectCMSReturnsSignerAndAttributes(t *testing.T) {
	cert, signer := testSigner(t)
	content := []byte("content for CMS inspection")
	cms, err := CreateCMS(content, CMSOptions{
		Signer:          signer,
		Certificate:     cert,
		DigestAlgorithm: crypto.SHA256,
		Detached:        true,
		SigningTime:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateCMS() error = %v", err)
	}

	info, err := InspectCMS(cms)
	if err != nil {
		t.Fatalf("InspectCMS() error = %v", err)
	}
	if info.Signer == nil || !info.Signer.Equal(cert) {
		t.Fatal("InspectCMS() did not return the signer certificate")
	}
	if len(info.SignedAttributes) == 0 {
		t.Fatal("InspectCMS() returned no signed attributes")
	}

	signerCert, err := SignerCertificate(cms)
	if err != nil {
		t.Fatalf("SignerCertificate() error = %v", err)
	}
	if !signerCert.Equal(cert) {
		t.Fatal("SignerCertificate() returned a different certificate")
	}

	values, err := SignedAttributeValues(cms, oidMessageDigest)
	if err != nil {
		t.Fatalf("SignedAttributeValues() error = %v", err)
	}
	if len(values) != 1 || values[0].Tag != asn1.TagOctetString {
		t.Fatalf("messageDigest values = %#v, want one octet string", values)
	}
	sum := sha256.Sum256(content)
	if !bytes.Equal(values[0].Bytes, sum[:]) {
		t.Fatal("messageDigest attribute does not match content digest")
	}
}

func TestVerifyRejectsSignedRangeTampering(t *testing.T) {
	cert, signer := testSigner(t)
	truststore := x509.NewCertPool()
	truststore.AddCert(cert)

	signedPDF, err := Bytes(testPDFBytes(t), Options{
		Signer:          signer,
		Certificate:     cert,
		DigestAlgorithm: crypto.SHA256,
		SigningTime:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("Bytes() error = %v", err)
	}
	signature, err := Verify(signedPDF, truststore)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if signature.ByteRange[1] < 20 {
		t.Fatalf("signed range too small: %v", signature.ByteRange)
	}

	tamperedPDF := append([]byte(nil), signedPDF...)
	tamperedPDF[signature.ByteRange[1]-10] ^= 0x01
	if _, err := Verify(tamperedPDF, truststore); err == nil {
		t.Fatal("Verify() accepted tampered signed content")
	}
}

func TestFileWritesOwnerOnlyOutput(t *testing.T) {
	cert, signer := testSigner(t)
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.pdf")
	outputPath := filepath.Join(dir, "signed.pdf")
	if err := os.WriteFile(inputPath, testPDFBytes(t), 0o600); err != nil {
		t.Fatalf("WriteFile(input) error = %v", err)
	}
	if err := os.WriteFile(outputPath, []byte("old"), 0o644); err != nil {
		t.Fatalf("WriteFile(output) error = %v", err)
	}

	err := File(inputPath, outputPath, Options{
		Signer:          signer,
		Certificate:     cert,
		DigestAlgorithm: crypto.SHA256,
		SigningTime:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("File() error = %v", err)
	}
	info, err := os.Stat(outputPath)
	if err != nil {
		t.Fatalf("Stat(output) error = %v", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("output permissions = %o, want no group/other permissions", info.Mode().Perm())
	}
}

func TestVerifyRejectsUnsignedTrailingData(t *testing.T) {
	cert, signer := testSigner(t)
	truststore := x509.NewCertPool()
	truststore.AddCert(cert)

	signedPDF, err := Bytes(testPDFBytes(t), Options{
		Signer:          signer,
		Certificate:     cert,
		DigestAlgorithm: crypto.SHA256,
		SigningTime:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("Bytes() error = %v", err)
	}

	tamperedPDF := append(append([]byte(nil), signedPDF...), []byte("\n% unsigned trailing update\n")...)
	_, err = Verify(tamperedPDF, truststore)
	if err == nil {
		t.Fatal("Verify() accepted unsigned trailing data")
	}
	if !strings.Contains(err.Error(), "does not cover full PDF") {
		t.Fatalf("Verify() error = %v, want full PDF coverage error", err)
	}
}

func TestDecodeDigestAlgorithmRejectsSHA1(t *testing.T) {
	value, rest, err := readDER(derSequence(derOID(oidSHA1)))
	if err != nil {
		t.Fatalf("readDER() error = %v", err)
	}
	if len(rest) != 0 {
		t.Fatalf("readDER() rest length = %d, want 0", len(rest))
	}
	_, err = decodeDigestAlgorithm(value)
	if err == nil {
		t.Fatal("decodeDigestAlgorithm() accepted SHA-1")
	}
	if !strings.Contains(err.Error(), "insecure digest") {
		t.Fatalf("decodeDigestAlgorithm() error = %v, want insecure digest error", err)
	}
}

func TestReadDERRejectsOverflowLength(t *testing.T) {
	input := []byte{0x30, 0x88, 0x7f, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
	if _, _, err := readDER(input); err == nil {
		t.Fatal("readDER() accepted overflowing DER length")
	}
}

func TestBytesRejectsHugeSignatureSize(t *testing.T) {
	cert, signer := testSigner(t)
	_, err := Bytes(testPDFBytes(t), Options{
		Signer:          signer,
		Certificate:     cert,
		DigestAlgorithm: crypto.SHA256,
		SignatureSize:   maxSignatureBytes + 1,
	})
	if err == nil {
		t.Fatal("Bytes() accepted huge SignatureSize")
	}
}

func TestCreateAndVerifyCMS(t *testing.T) {
	cert, signer := testSigner(t)
	content := []byte("signed content")
	truststore := x509.NewCertPool()
	truststore.AddCert(cert)

	attached, err := CreateCMS(content, CMSOptions{
		Signer:          signer,
		Certificate:     cert,
		DigestAlgorithm: crypto.SHA256,
	})
	if err != nil {
		t.Fatalf("CreateCMS() attached error = %v", err)
	}
	parsed, err := VerifyCMS(attached, truststore)
	if err != nil {
		t.Fatalf("VerifyCMS() error = %v", err)
	}
	if !bytes.Equal(parsed.Content, content) {
		t.Fatalf("verified content = %q, want %q", parsed.Content, content)
	}

	detached, err := CreateCMS(content, CMSOptions{
		Signer:          signer,
		Certificate:     cert,
		DigestAlgorithm: crypto.SHA256,
		Detached:        true,
	})
	if err != nil {
		t.Fatalf("CreateCMS() detached error = %v", err)
	}
	detachedResult, err := VerifyDetachedCMS(detached, content, truststore)
	if err != nil {
		t.Fatalf("VerifyDetachedCMS() error = %v", err)
	}
	if !detachedResult.Detached {
		t.Fatal("detached result was not marked detached")
	}
}

func TestCreateCMSVerifiesWithOpenSSL(t *testing.T) {
	if _, err := exec.LookPath("openssl"); err != nil {
		t.Skip("openssl is not available")
	}
	cert, signer := testSigner(t)
	content := []byte("signed content verified by openssl")
	cms, err := CreateCMS(content, CMSOptions{
		Signer:          signer,
		Certificate:     cert,
		DigestAlgorithm: crypto.SHA256,
		Detached:        true,
		SigningTime:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreateCMS() error = %v", err)
	}

	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	cmsPath := filepath.Join(dir, "signature.der")
	contentPath := filepath.Join(dir, "content.bin")
	outputPath := filepath.Join(dir, "verified.bin")
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		t.Fatalf("WriteFile(cert) error = %v", err)
	}
	if err := os.WriteFile(cmsPath, cms, 0o600); err != nil {
		t.Fatalf("WriteFile(cms) error = %v", err)
	}
	if err := os.WriteFile(contentPath, content, 0o600); err != nil {
		t.Fatalf("WriteFile(content) error = %v", err)
	}

	cmd := exec.CommandContext(t.Context(), "openssl", "cms", "-verify", "-binary", "-inform", "DER",
		"-in", cmsPath, "-content", contentPath, "-CAfile", certPath, "-purpose", "any", "-out", outputPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("openssl cms -verify failed: %v\n%s", err, output)
	}
	verified, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile(output) error = %v", err)
	}
	if !bytes.Equal(verified, content) {
		t.Fatalf("openssl output = %q, want %q", verified, content)
	}
}

func TestVerifyDetachedCMSRejectsAttachedCMS(t *testing.T) {
	cert, signer := testSigner(t)
	truststore := x509.NewCertPool()
	truststore.AddCert(cert)
	attached, err := CreateCMS([]byte("embedded content"), CMSOptions{
		Signer:          signer,
		Certificate:     cert,
		DigestAlgorithm: crypto.SHA256,
	})
	if err != nil {
		t.Fatalf("CreateCMS() error = %v", err)
	}
	if _, err := VerifyDetachedCMS(attached, []byte("detached content"), truststore); err == nil {
		t.Fatal("VerifyDetachedCMS() accepted attached CMS")
	}
}

func TestCreateCMSRejectsMismatchedSignerCertificate(t *testing.T) {
	cert, _ := testSigner(t)
	_, otherSigner := testSigner(t)
	_, err := CreateCMS([]byte("content"), CMSOptions{
		Signer:          otherSigner,
		Certificate:     cert,
		DigestAlgorithm: crypto.SHA256,
		Detached:        true,
	})
	if err == nil {
		t.Fatal("CreateCMS() accepted mismatched signer and certificate")
	}
}

func TestVerifyCMSRejectsWrongCertificateUsage(t *testing.T) {
	cert, signer := testSignerWithTemplate(t, &x509.Certificate{
		SerialNumber: big.NewInt(10),
		Subject: pkix.Name{
			CommonName: "Server Auth Only",
		},
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	})
	truststore := x509.NewCertPool()
	truststore.AddCert(cert)
	cms, err := CreateCMS([]byte("content"), CMSOptions{
		Signer:          signer,
		Certificate:     cert,
		DigestAlgorithm: crypto.SHA256,
		Detached:        true,
	})
	if err != nil {
		t.Fatalf("CreateCMS() error = %v", err)
	}
	if _, err := VerifyDetachedCMS(cms, []byte("content"), truststore); err == nil {
		t.Fatal("VerifyDetachedCMS() accepted non-document-signing certificate")
	}
}

func TestVerifyCMSRejectsOversizedCMS(t *testing.T) {
	if _, err := VerifyCMS(bytes.Repeat([]byte{0x30}, maxCMSPackageBytes+1), nil); err == nil {
		t.Fatal("VerifyCMS() accepted oversized CMS package")
	}
}

func TestDecodeSignedAttributesRequiresContentType(t *testing.T) {
	attrs := derSet(derSequence(derOID(oidMessageDigest), derSet(derOctetString([]byte("digest")))))
	if _, _, err := decodeSignedAttributes(attrs); err == nil {
		t.Fatal("decodeSignedAttributes() accepted missing contentType")
	}
}

func TestSignatureAlgorithmIDUsesRSAWithDigestOID(t *testing.T) {
	_, signer := testSigner(t)
	alg, err := signatureAlgorithmID(signer.Public(), crypto.SHA256)
	if err != nil {
		t.Fatalf("signatureAlgorithmID() error = %v", err)
	}
	value, rest, err := readDER(alg)
	if err != nil {
		t.Fatalf("readDER() error = %v", err)
	}
	if len(rest) != 0 {
		t.Fatalf("readDER() rest length = %d, want 0", len(rest))
	}
	children, err := readDERChildren(value.Content)
	if err != nil {
		t.Fatalf("readDERChildren() error = %v", err)
	}
	if len(children) == 0 {
		t.Fatal("readDERChildren() returned no children")
	}
	oid, err := decodeOID(children[0])
	if err != nil {
		t.Fatalf("decodeOID() error = %v", err)
	}
	if !oid.Equal(oidSHA256WithRSA) {
		t.Fatalf("signature algorithm OID = %v, want %v", oid, oidSHA256WithRSA)
	}
}

func TestReadDERChildrenLimitsNodeCount(t *testing.T) {
	input := bytes.Repeat([]byte{0x05, 0x00}, maxDERChildren+1)
	if _, err := readDERChildren(input); err == nil || !strings.Contains(err.Error(), "child count") {
		t.Fatalf("readDERChildren() limit error = %v", err)
	}
}

func TestAddAnnotationRejectsIndirectAnnotsWithoutUsingLaterArray(t *testing.T) {
	dict := []byte("<< /Type /Page /Annots 12 0 R /MediaBox [0 0 10 10] >>")
	_, err := addAnnotation(dict, "20 0 R")
	if err == nil {
		t.Fatal("addAnnotation() accepted indirect /Annots")
	}
	if !strings.Contains(err.Error(), "referenced /Annots") {
		t.Fatalf("addAnnotation() error = %v, want referenced Annots error", err)
	}
}

func TestBytesUsesIncrementPlaceholders(t *testing.T) {
	cert, signer := testSigner(t)
	truststore := x509.NewCertPool()
	truststore.AddCert(cert)
	fakePlaceholder := "\n% /Contents <" + strings.Repeat("0", defaultSignatureBytes*2) + ">\n"
	input := append(append([]byte(nil), testPDFBytes(t)...), []byte(fakePlaceholder)...)

	signedPDF, err := Bytes(input, Options{
		Signer:          signer,
		Certificate:     cert,
		DigestAlgorithm: crypto.SHA256,
		SigningTime:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("Bytes() error = %v", err)
	}
	if _, err := Verify(signedPDF, truststore); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
}

func TestXrefOffsetsPointToObjectStart(t *testing.T) {
	cert, signer := testSigner(t)
	signedPDF, err := Bytes(testPDFBytes(t), Options{
		Signer:          signer,
		Certificate:     cert,
		DigestAlgorithm: crypto.SHA256,
		SigningTime:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("Bytes() error = %v", err)
	}
	xref, err := findStartXref(signedPDF)
	if err != nil {
		t.Fatalf("findStartXref() error = %v", err)
	}
	offsets, err := parseXrefTable(signedPDF, xref)
	if err != nil {
		t.Fatalf("parseXrefTable() error = %v", err)
	}
	for object, offset := range offsets {
		prefix := []byte(fmt.Sprintf("%d ", object))
		if !bytes.HasPrefix(signedPDF[offset:], prefix) {
			t.Fatalf("xref object %d offset %d does not point to object start", object, offset)
		}
	}
}

func TestReadObjectDictRejectsWrongObjectHeader(t *testing.T) {
	pdf := []byte("2 0 obj\n<< /Type /Page >>\nendobj\n")
	_, err := readObjectDict(pdf, pdfRef{Object: 1, Generation: 0}, 0)
	if err == nil {
		t.Fatal("readObjectDict() accepted mismatched object header")
	}
	if !strings.Contains(err.Error(), "xref points to object") {
		t.Fatalf("readObjectDict() error = %v, want xref object mismatch", err)
	}
}

func TestPreservedTrailerEntriesKeepsInfoAndID(t *testing.T) {
	trailer := []byte("trailer\n<< /Size 4 /Root 1 0 R /Info 2 0 R /ID [<001122> <334455>] >>\n")
	entries, err := preservedTrailerEntries(trailer)
	if err != nil {
		t.Fatalf("preservedTrailerEntries() error = %v", err)
	}
	if !strings.Contains(entries, "/Info 2 0 R") {
		t.Fatalf("preserved entries = %q, want /Info", entries)
	}
	if !strings.Contains(entries, "/ID [<001122> <334455>]") {
		t.Fatalf("preserved entries = %q, want /ID", entries)
	}
}

func TestAnalyzePDFFollowsIncrementalPrevChain(t *testing.T) {
	cert, signer := testSigner(t)
	signedPDF, err := Bytes(testPDFBytes(t), Options{
		Signer:          signer,
		Certificate:     cert,
		DigestAlgorithm: crypto.SHA256,
		SigningTime:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("Bytes() error = %v", err)
	}
	if _, err := analyzePDF(signedPDF); err != nil {
		t.Fatalf("analyzePDF() error = %v", err)
	}
}

func TestAnalyzePDFContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := analyzePDFContext(ctx, testPDFBytes(t))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("analyzePDFContext() error = %v, want context.Canceled", err)
	}
}

func TestSigningScannersHonorContextInsideLoops(t *testing.T) {
	ctx := &errAfterContext{Context: context.Background(), remaining: 1}
	input := []byte("<<" + strings.Repeat("/LongName 1 ", 4096))
	if _, err := findDictionaryEndContext(ctx, input); !errors.Is(err, context.Canceled) {
		t.Fatalf("findDictionaryEndContext() error = %v, want context.Canceled", err)
	}

	ctx = &errAfterContext{Context: context.Background(), remaining: 1}
	if _, err := findPDFNameContext(ctx, []byte(strings.Repeat("/Other 1 ", 4096)), "/Missing"); !errors.Is(err, context.Canceled) {
		t.Fatalf("findPDFNameContext() error = %v, want context.Canceled", err)
	}

	ctx = &errAfterContext{Context: context.Background(), remaining: 1}
	if _, err := pdfValueEndContext(ctx, []byte("("+strings.Repeat("x", 4096)), 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("pdfValueEndContext() error = %v, want context.Canceled", err)
	}
}

func TestFindDictionaryEndSkipsStringAndCommentDelimiters(t *testing.T) {
	input := []byte("<< /Literal (>>) /Nested << /Hex <3E3E> >> % >>\n /Value 1 >>")
	end, err := findDictionaryEnd(input)
	if err != nil {
		t.Fatalf("findDictionaryEnd() error = %v", err)
	}
	if end != len(input) {
		t.Fatalf("findDictionaryEnd() = %d, want %d", end, len(input))
	}
}

type errAfterContext struct {
	context.Context
	remaining int
}

func (ctx *errAfterContext) Err() error {
	if ctx.remaining <= 0 {
		return context.Canceled
	}
	ctx.remaining--
	return nil
}

func testPDFBytes(t *testing.T) []byte {
	t.Helper()
	return minimalPDFBytes()
}

func minimalPDFBytes() []byte {
	var output bytes.Buffer
	output.WriteString("%PDF-1.4\n")
	offsets := []int{0}
	addObject := func(body string) {
		offsets = append(offsets, output.Len())
		fmt.Fprintf(&output, "%d 0 obj\n%s\nendobj\n", len(offsets)-1, body)
	}
	addObject("<< /Type /Catalog /Pages 2 0 R >>")
	addObject("<< /Type /Pages /Kids [3 0 R] /Count 1 >>")
	addObject("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] >>")
	xrefOffset := output.Len()
	fmt.Fprintf(&output, "xref\n0 %d\n", len(offsets))
	output.WriteString("0000000000 65535 f \n")
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&output, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&output, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), xrefOffset)
	return output.Bytes()
}

func testSigner(t *testing.T) (*x509.Certificate, crypto.Signer) {
	t.Helper()
	return testSignerWithTemplate(t, &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "Test PDF Signer",
		},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageContentCommitment,
		UnknownExtKeyUsage:    []asn1.ObjectIdentifier{oidDocumentSigning},
		BasicConstraintsValid: true,
	})
}

func testSignerWithTemplate(t *testing.T, template *x509.Certificate) (*x509.Certificate, crypto.Signer) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	now := time.Now().UTC()
	template.NotBefore = now.Add(-time.Hour)
	template.NotAfter = now.Add(time.Hour)
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate() error = %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("ParseCertificate() error = %v", err)
	}
	return cert, key
}
