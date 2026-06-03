// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package sign

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
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

func TestCreateAndVerifyPKCS7(t *testing.T) {
	cert, signer := testSigner(t)
	content := []byte("signed content")
	truststore := x509.NewCertPool()
	truststore.AddCert(cert)

	attached, err := CreatePKCS7(content, PKCS7Options{
		Signer:          signer,
		Certificate:     cert,
		DigestAlgorithm: crypto.SHA256,
	})
	if err != nil {
		t.Fatalf("CreatePKCS7() attached error = %v", err)
	}
	parsed, err := VerifyPKCS7(attached, truststore)
	if err != nil {
		t.Fatalf("VerifyPKCS7() error = %v", err)
	}
	if !bytes.Equal(parsed.Content, content) {
		t.Fatalf("verified content = %q, want %q", parsed.Content, content)
	}

	detached, err := CreatePKCS7(content, PKCS7Options{
		Signer:          signer,
		Certificate:     cert,
		DigestAlgorithm: crypto.SHA256,
		Detached:        true,
	})
	if err != nil {
		t.Fatalf("CreatePKCS7() detached error = %v", err)
	}
	detachedResult, err := VerifyDetachedPKCS7(detached, content, truststore)
	if err != nil {
		t.Fatalf("VerifyDetachedPKCS7() error = %v", err)
	}
	if !detachedResult.Detached {
		t.Fatal("detached result was not marked detached")
	}
}

func TestCreatePKCS7VerifiesWithOpenSSL(t *testing.T) {
	if _, err := exec.LookPath("openssl"); err != nil {
		t.Skip("openssl is not available")
	}
	cert, signer := testSigner(t)
	content := []byte("signed content verified by openssl")
	cms, err := CreatePKCS7(content, PKCS7Options{
		Signer:          signer,
		Certificate:     cert,
		DigestAlgorithm: crypto.SHA256,
		Detached:        true,
		SigningTime:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("CreatePKCS7() error = %v", err)
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

	cmd := exec.Command("openssl", "cms", "-verify", "-binary", "-inform", "DER",
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

func TestVerifyDetachedPKCS7RejectsAttachedCMS(t *testing.T) {
	cert, signer := testSigner(t)
	truststore := x509.NewCertPool()
	truststore.AddCert(cert)
	attached, err := CreatePKCS7([]byte("embedded content"), PKCS7Options{
		Signer:          signer,
		Certificate:     cert,
		DigestAlgorithm: crypto.SHA256,
	})
	if err != nil {
		t.Fatalf("CreatePKCS7() error = %v", err)
	}
	if _, err := VerifyDetachedPKCS7(attached, []byte("detached content"), truststore); err == nil {
		t.Fatal("VerifyDetachedPKCS7() accepted attached CMS")
	}
}

func TestCreatePKCS7RejectsMismatchedSignerCertificate(t *testing.T) {
	cert, _ := testSigner(t)
	_, otherSigner := testSigner(t)
	_, err := CreatePKCS7([]byte("content"), PKCS7Options{
		Signer:          otherSigner,
		Certificate:     cert,
		DigestAlgorithm: crypto.SHA256,
		Detached:        true,
	})
	if err == nil {
		t.Fatal("CreatePKCS7() accepted mismatched signer and certificate")
	}
}

func TestVerifyPKCS7RejectsWrongCertificateUsage(t *testing.T) {
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
	cms, err := CreatePKCS7([]byte("content"), PKCS7Options{
		Signer:          signer,
		Certificate:     cert,
		DigestAlgorithm: crypto.SHA256,
		Detached:        true,
	})
	if err != nil {
		t.Fatalf("CreatePKCS7() error = %v", err)
	}
	if _, err := VerifyDetachedPKCS7(cms, []byte("content"), truststore); err == nil {
		t.Fatal("VerifyDetachedPKCS7() accepted non-document-signing certificate")
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
