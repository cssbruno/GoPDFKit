// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

// This file signs and verifies PDF CMS/PKCS7 signatures.
package sign

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/asn1"
	"errors"
	"fmt"
	"math/big"
	"time"
)

var (
	oidData            = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 1}
	oidSignedData      = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 2}
	oidContentType     = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 3}
	oidMessageDigest   = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 4}
	oidSigningTime     = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 5}
	oidSHA1            = asn1.ObjectIdentifier{1, 3, 14, 3, 2, 26}
	oidSHA256          = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 1}
	oidSHA384          = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 2}
	oidSHA512          = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 3}
	oidRSAEncryption   = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 1}
	oidSHA256WithRSA   = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 11}
	oidSHA384WithRSA   = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 12}
	oidSHA512WithRSA   = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 13}
	oidECDSAWithSHA256 = asn1.ObjectIdentifier{1, 2, 840, 10045, 4, 3, 2}
	oidECDSAWithSHA384 = asn1.ObjectIdentifier{1, 2, 840, 10045, 4, 3, 3}
	oidECDSAWithSHA512 = asn1.ObjectIdentifier{1, 2, 840, 10045, 4, 3, 4}
	oidDocumentSigning = asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 3, 36}
)

const maxCMSPackageBytes = 16 * 1024 * 1024

// PKCS7Options configures CMS/PKCS7 SignedData creation.
type PKCS7Options struct {
	// Signer signs the CMS signed attributes.
	Signer crypto.Signer
	// Certificate is the signing certificate and must match Signer.
	Certificate *x509.Certificate
	// CertificateChain contains optional intermediate certificates to include.
	CertificateChain []*x509.Certificate
	// DigestAlgorithm selects the message digest. A zero value uses SHA-256.
	DigestAlgorithm crypto.Hash
	// Detached omits the signed content from the CMS payload when true.
	Detached bool
	// SigningTime sets the CMS signingTime attribute. A zero value uses now.
	SigningTime time.Time
}

// PKCS7VerifyResult contains the relevant result of a CMS/PKCS7 verification.
type PKCS7VerifyResult struct {
	// Certificates contains the certificates embedded in the CMS payload.
	Certificates []*x509.Certificate
	// Signer is the certificate that produced the verified signature.
	Signer *x509.Certificate
	// Digest is the digest algorithm used by the signer.
	Digest crypto.Hash
	// Detached reports whether the CMS payload is detached from its content.
	Detached bool
	// Content contains the verified content.
	Content []byte
	// SigningTime is the optional signingTime attribute from the CMS payload.
	SigningTime *time.Time
	// ValidSignature reports whether the CMS signature was cryptographically valid.
	ValidSignature bool
	// TrustedSigner reports whether the signer chained to the supplied truststore.
	TrustedSigner bool
}

// CreatePKCS7 creates CMS SignedData using this package's own DER encoder.
func CreatePKCS7(content []byte, options PKCS7Options) ([]byte, error) {
	if options.Signer == nil {
		return nil, ErrMissingSigner
	}
	if options.Certificate == nil {
		return nil, ErrMissingCertificate
	}
	if !publicKeysEqual(options.Signer.Public(), options.Certificate.PublicKey) {
		return nil, errors.New("pdfsigning: signer public key does not match certificate")
	}
	digest, err := normalizeDigest(options.DigestAlgorithm)
	if err != nil {
		return nil, err
	}
	signingTime := options.SigningTime
	if signingTime.IsZero() {
		signingTime = time.Now().UTC()
	}

	contentDigest := hashBytes(digest, content)
	signedAttrs, err := signedAttributes(contentDigest, signingTime)
	if err != nil {
		return nil, err
	}
	signatureDigest := hashBytes(digest, signedAttrs)
	signature, err := options.Signer.Sign(rand.Reader, signatureDigest, digest)
	if err != nil {
		return nil, fmt.Errorf("sign CMS attributes: %w", err)
	}

	digestAlg := digestAlgorithmID(digest)
	signatureAlg, err := signatureAlgorithmID(options.Signer.Public(), digest)
	if err != nil {
		return nil, err
	}
	signedAttrValue, _, err := readDER(signedAttrs)
	if err != nil {
		return nil, err
	}
	signerInfo := derSequence(
		derInteger(1),
		derSequence(
			options.Certificate.RawIssuer,
			derInteger(options.Certificate.SerialNumber),
		),
		digestAlg,
		der(0xa0, signedAttrValue.Content),
		signatureAlg,
		derOctetString(signature),
	)

	certs := make([][]byte, 0, 1+len(options.CertificateChain))
	certs = append(certs, options.Certificate.Raw)
	for _, cert := range options.CertificateChain {
		if cert != nil {
			certs = append(certs, cert.Raw)
		}
	}

	encapContentInfo := derSequence(derOID(oidData))
	if !options.Detached {
		encapContentInfo = derSequence(
			derOID(oidData),
			der(0xa0, derOctetString(content)),
		)
	}
	signedData := derSequence(
		derInteger(1),
		derSet(digestAlg),
		encapContentInfo,
		der(0xa0, derValueContent(derSet(certs...))),
		derSet(signerInfo),
	)
	return derSequence(derOID(oidSignedData), der(0xa0, signedData)), nil
}

func derValueContent(encoded []byte) []byte {
	value, rest, err := readDER(encoded)
	if err != nil || len(rest) != 0 {
		panic("invalid DER value")
	}
	return value.Content
}

// VerifyPKCS7 verifies attached CMS/PKCS7 SignedData.
func VerifyPKCS7(signature []byte, truststore *x509.CertPool) (*PKCS7VerifyResult, error) {
	return verifyPKCS7(signature, nil, truststore, false)
}

// VerifyDetachedPKCS7 verifies detached CMS/PKCS7 SignedData against content.
func VerifyDetachedPKCS7(signature, content []byte, truststore *x509.CertPool) (*PKCS7VerifyResult, error) {
	return verifyPKCS7(signature, content, truststore, true)
}

func verifyPKCS7(signature []byte, detachedContent []byte, truststore *x509.CertPool, requireDetached bool) (*PKCS7VerifyResult, error) {
	if len(signature) > maxCMSPackageBytes {
		return nil, errors.New("pdfsigning: CMS package exceeds maximum size")
	}
	outer, rest, err := readDER(signature)
	if err != nil {
		return nil, err
	}
	if len(rest) != 0 {
		return nil, errors.New("pdfsigning: trailing bytes after CMS package")
	}
	if err := expectTag(outer, 0x30, "CMS ContentInfo"); err != nil {
		return nil, err
	}
	outerChildren, err := readDERChildren(outer.Content)
	if err != nil {
		return nil, err
	}
	if len(outerChildren) != 2 {
		return nil, errors.New("pdfsigning: invalid CMS ContentInfo")
	}
	contentType, err := decodeOID(outerChildren[0])
	if err != nil {
		return nil, err
	}
	if !contentType.Equal(oidSignedData) {
		return nil, errors.New("pdfsigning: CMS content is not SignedData")
	}
	if err := expectTag(outerChildren[1], 0xa0, "CMS SignedData wrapper"); err != nil {
		return nil, err
	}
	signedDataValue, rest, err := readDER(outerChildren[1].Content)
	if err != nil {
		return nil, err
	}
	if len(rest) != 0 {
		return nil, errors.New("pdfsigning: invalid SignedData wrapper")
	}
	return verifySignedData(signedDataValue, detachedContent, truststore, requireDetached)
}

func verifySignedData(signedDataValue derValue, detachedContent []byte, truststore *x509.CertPool, requireDetached bool) (*PKCS7VerifyResult, error) {
	if err := expectTag(signedDataValue, 0x30, "SignedData"); err != nil {
		return nil, err
	}
	children, err := readDERChildren(signedDataValue.Content)
	if err != nil {
		return nil, err
	}
	if len(children) < 5 {
		return nil, errors.New("pdfsigning: incomplete SignedData")
	}

	encapChildren, err := readDERChildren(children[2].Content)
	if err != nil {
		return nil, err
	}
	if len(encapChildren) == 0 {
		return nil, errors.New("pdfsigning: missing encapsulated content info")
	}
	contentOID, err := decodeOID(encapChildren[0])
	if err != nil {
		return nil, err
	}
	if !contentOID.Equal(oidData) {
		return nil, errors.New("pdfsigning: unsupported CMS content type")
	}

	content := detachedContent
	detached := true
	if len(encapChildren) > 1 {
		if requireDetached {
			return nil, errors.New("pdfsigning: detached CMS verification requires detached content")
		}
		embedded, rest, err := readDER(encapChildren[1].Content)
		if err != nil {
			return nil, err
		}
		if len(rest) != 0 {
			return nil, errors.New("pdfsigning: invalid embedded CMS content")
		}
		if err := expectTag(embedded, 0x04, "embedded CMS content"); err != nil {
			return nil, err
		}
		content = embedded.Content
		detached = false
	}
	if content == nil {
		return nil, errors.New("pdfsigning: detached content is required")
	}

	certificates, signerInfos, err := parseCertificatesAndSigners(children[3:])
	if err != nil {
		return nil, err
	}
	if len(certificates) == 0 {
		return nil, errors.New("pdfsigning: CMS package has no certificates")
	}
	if len(signerInfos) == 0 {
		return nil, errors.New("pdfsigning: CMS package has no signer info")
	}

	result, err := verifySignerInfo(signerInfos[0], certificates, content, truststore)
	if err != nil {
		return nil, err
	}
	result.Detached = detached
	result.Content = append([]byte(nil), content...)
	return result, nil
}

func parseCertificatesAndSigners(values []derValue) ([]*x509.Certificate, []derValue, error) {
	var certs []*x509.Certificate
	var signers []derValue
	for _, value := range values {
		switch value.Tag {
		case 0xa0:
			rawCerts, err := readDERChildren(value.Content)
			if err != nil {
				return nil, nil, err
			}
			for _, raw := range rawCerts {
				cert, err := x509.ParseCertificate(raw.Full)
				if err != nil {
					return nil, nil, fmt.Errorf("parse CMS certificate: %w", err)
				}
				certs = append(certs, cert)
			}
		case 0x31:
			rawSigners, err := readDERChildren(value.Content)
			if err != nil {
				return nil, nil, err
			}
			signers = append(signers, rawSigners...)
		}
	}
	return certs, signers, nil
}

func verifySignerInfo(signerInfo derValue, certificates []*x509.Certificate, content []byte, truststore *x509.CertPool) (*PKCS7VerifyResult, error) {
	if err := expectTag(signerInfo, 0x30, "SignerInfo"); err != nil {
		return nil, err
	}
	children, err := readDERChildren(signerInfo.Content)
	if err != nil {
		return nil, err
	}
	if len(children) < 6 {
		return nil, errors.New("pdfsigning: incomplete SignerInfo")
	}
	issuer, serial, err := decodeIssuerAndSerial(children[1])
	if err != nil {
		return nil, err
	}
	signer := findSignerCertificate(certificates, issuer, serial)
	if signer == nil {
		return nil, errors.New("pdfsigning: signer certificate not found")
	}
	digest, err := decodeDigestAlgorithm(children[2])
	if err != nil {
		return nil, err
	}
	if children[3].Tag != 0xa0 {
		return nil, errors.New("pdfsigning: signed attributes are required")
	}
	signedAttrsDER := der(0x31, children[3].Content)
	messageDigest, signingTime, err := decodeSignedAttributes(signedAttrsDER)
	if err != nil {
		return nil, err
	}
	expectedDigest := hashBytes(digest, content)
	if !bytes.Equal(messageDigest, expectedDigest) {
		return nil, errors.New("pdfsigning: CMS message digest mismatch")
	}

	if err := verifySignatureAlgorithm(children[4], signer.PublicKey, digest); err != nil {
		return nil, err
	}
	signature := children[5]
	if err := expectTag(signature, 0x04, "CMS signature"); err != nil {
		return nil, err
	}
	signedAttrsDigest := hashBytes(digest, signedAttrsDER)
	if err := verifyPublicKeySignature(signer.PublicKey, digest, signedAttrsDigest, signature.Content); err != nil {
		return nil, err
	}

	trusted := false
	if truststore != nil {
		if err := validateDocumentSignerCertificate(signer); err != nil {
			return nil, err
		}
		intermediates := x509.NewCertPool()
		for _, cert := range certificates {
			if !cert.Equal(signer) {
				intermediates.AddCert(cert)
			}
		}
		_, err := signer.Verify(x509.VerifyOptions{
			Roots:         truststore,
			Intermediates: intermediates,
			KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
		})
		if err != nil {
			return nil, fmt.Errorf("verify signer certificate: %w", err)
		}
		trusted = true
	}

	return &PKCS7VerifyResult{
		Certificates:   certificates,
		Signer:         signer,
		Digest:         digest,
		SigningTime:    signingTime,
		ValidSignature: true,
		TrustedSigner:  trusted,
	}, nil
}

func signedAttributes(messageDigest []byte, signingTime time.Time) ([]byte, error) {
	timeDER, err := asn1.Marshal(signingTime.UTC())
	if err != nil {
		return nil, err
	}
	return derSet(
		derSequence(derOID(oidContentType), derSet(derOID(oidData))),
		derSequence(derOID(oidMessageDigest), derSet(derOctetString(messageDigest))),
		derSequence(derOID(oidSigningTime), derSet(timeDER)),
	), nil
}

func decodeSignedAttributes(attrsDER []byte) ([]byte, *time.Time, error) {
	attrs, rest, err := readDER(attrsDER)
	if err != nil {
		return nil, nil, err
	}
	if len(rest) != 0 {
		return nil, nil, errors.New("pdfsigning: invalid signed attributes")
	}
	if err := expectTag(attrs, 0x31, "signed attributes"); err != nil {
		return nil, nil, err
	}
	children, err := readDERChildren(attrs.Content)
	if err != nil {
		return nil, nil, err
	}
	var messageDigest []byte
	contentTypeSeen := false
	messageDigestSeen := false
	var signingTime *time.Time
	var previousAttr []byte
	for _, attr := range children {
		if previousAttr != nil && bytes.Compare(previousAttr, attr.Full) > 0 {
			return nil, nil, errors.New("pdfsigning: signed attributes are not DER sorted")
		}
		previousAttr = attr.Full
		attrChildren, err := readDERChildren(attr.Content)
		if err != nil {
			return nil, nil, err
		}
		if len(attrChildren) != 2 {
			return nil, nil, errors.New("pdfsigning: invalid signed attribute")
		}
		oid, err := decodeOID(attrChildren[0])
		if err != nil {
			return nil, nil, err
		}
		values, err := readDERChildren(attrChildren[1].Content)
		if err != nil {
			return nil, nil, err
		}
		if len(values) == 0 {
			continue
		}
		switch {
		case oid.Equal(oidContentType):
			if contentTypeSeen {
				return nil, nil, errors.New("pdfsigning: duplicate contentType attribute")
			}
			if len(values) != 1 {
				return nil, nil, errors.New("pdfsigning: invalid contentType attribute")
			}
			contentType, err := decodeOID(values[0])
			if err != nil {
				return nil, nil, err
			}
			if !contentType.Equal(oidData) {
				return nil, nil, fmt.Errorf("pdfsigning: unsupported contentType attribute %v", contentType)
			}
			contentTypeSeen = true
		case oid.Equal(oidMessageDigest):
			if messageDigestSeen {
				return nil, nil, errors.New("pdfsigning: duplicate messageDigest attribute")
			}
			if len(values) != 1 {
				return nil, nil, errors.New("pdfsigning: invalid messageDigest attribute")
			}
			if err := expectTag(values[0], 0x04, "messageDigest attribute"); err != nil {
				return nil, nil, err
			}
			messageDigest = append([]byte(nil), values[0].Content...)
			messageDigestSeen = true
		case oid.Equal(oidSigningTime):
			if len(values) != 1 {
				return nil, nil, errors.New("pdfsigning: invalid signingTime attribute")
			}
			var t time.Time
			if _, err := asn1.Unmarshal(values[0].Full, &t); err != nil {
				return nil, nil, err
			}
			signingTime = &t
		}
	}
	if !contentTypeSeen {
		return nil, nil, errors.New("pdfsigning: missing contentType attribute")
	}
	if !messageDigestSeen || len(messageDigest) == 0 {
		return nil, nil, errors.New("pdfsigning: missing messageDigest attribute")
	}
	return messageDigest, signingTime, nil
}

func digestAlgorithmID(digest crypto.Hash) []byte {
	return derSequence(derOID(digestOID(digest)))
}

func signatureAlgorithmID(publicKey any, digest crypto.Hash) ([]byte, error) {
	switch publicKey.(type) {
	case *rsa.PublicKey:
		switch digest {
		case crypto.SHA256:
			return derSequence(derOID(oidSHA256WithRSA), derNull()), nil
		case crypto.SHA384:
			return derSequence(derOID(oidSHA384WithRSA), derNull()), nil
		case crypto.SHA512:
			return derSequence(derOID(oidSHA512WithRSA), derNull()), nil
		}
	case *ecdsa.PublicKey:
		switch digest {
		case crypto.SHA256:
			return derSequence(derOID(oidECDSAWithSHA256)), nil
		case crypto.SHA384:
			return derSequence(derOID(oidECDSAWithSHA384)), nil
		case crypto.SHA512:
			return derSequence(derOID(oidECDSAWithSHA512)), nil
		}
	}
	return nil, fmt.Errorf("pdfsigning: unsupported signer public key %T", publicKey)
}

func verifySignatureAlgorithm(value derValue, publicKey any, digest crypto.Hash) error {
	children, err := readDERChildren(value.Content)
	if err != nil {
		return err
	}
	if len(children) == 0 {
		return errors.New("pdfsigning: invalid signature algorithm")
	}
	oid, err := decodeOID(children[0])
	if err != nil {
		return err
	}
	if len(children) > 1 && !isDERNull(children[1]) {
		return errors.New("pdfsigning: unsupported signature algorithm parameters")
	}
	switch publicKey.(type) {
	case *rsa.PublicKey:
		if oid.Equal(oidRSAEncryption) {
			return nil
		}
		switch digest {
		case crypto.SHA256:
			if oid.Equal(oidSHA256WithRSA) {
				return nil
			}
		case crypto.SHA384:
			if oid.Equal(oidSHA384WithRSA) {
				return nil
			}
		case crypto.SHA512:
			if oid.Equal(oidSHA512WithRSA) {
				return nil
			}
		}
	case *ecdsa.PublicKey:
		switch digest {
		case crypto.SHA256:
			if oid.Equal(oidECDSAWithSHA256) {
				return nil
			}
		case crypto.SHA384:
			if oid.Equal(oidECDSAWithSHA384) {
				return nil
			}
		case crypto.SHA512:
			if oid.Equal(oidECDSAWithSHA512) {
				return nil
			}
		}
	}
	return errors.New("pdfsigning: signature algorithm does not match signer key and digest")
}

func verifyPublicKeySignature(publicKey any, digest crypto.Hash, hashed, signature []byte) error {
	switch key := publicKey.(type) {
	case *rsa.PublicKey:
		return rsa.VerifyPKCS1v15(key, digest, hashed, signature)
	case *ecdsa.PublicKey:
		if !ecdsa.VerifyASN1(key, hashed, signature) {
			return errors.New("pdfsigning: invalid ECDSA signature")
		}
		return nil
	default:
		return fmt.Errorf("pdfsigning: unsupported signer public key %T", publicKey)
	}
}

func hashBytes(digest crypto.Hash, input []byte) []byte {
	h := digest.New()
	_, _ = h.Write(input)
	return h.Sum(nil)
}

func digestOID(digest crypto.Hash) asn1.ObjectIdentifier {
	switch digest {
	case crypto.SHA1:
		return oidSHA1
	case crypto.SHA256:
		return oidSHA256
	case crypto.SHA384:
		return oidSHA384
	case crypto.SHA512:
		return oidSHA512
	default:
		panic("unsupported digest")
	}
}

func decodeOID(value derValue) (asn1.ObjectIdentifier, error) {
	var oid asn1.ObjectIdentifier
	if _, err := asn1.Unmarshal(value.Full, &oid); err != nil {
		return nil, err
	}
	return oid, nil
}

func decodeDigestAlgorithm(value derValue) (crypto.Hash, error) {
	children, err := readDERChildren(value.Content)
	if err != nil {
		return 0, err
	}
	if len(children) == 0 {
		return 0, errors.New("pdfsigning: invalid algorithm identifier")
	}
	if len(children) > 2 || (len(children) == 2 && !isDERNull(children[1])) {
		return 0, errors.New("pdfsigning: unsupported digest algorithm parameters")
	}
	oid, err := decodeOID(children[0])
	if err != nil {
		return 0, err
	}
	switch {
	case oid.Equal(oidSHA1):
		return 0, fmt.Errorf("pdfsigning: insecure digest OID %v", oid)
	case oid.Equal(oidSHA256):
		return crypto.SHA256, nil
	case oid.Equal(oidSHA384):
		return crypto.SHA384, nil
	case oid.Equal(oidSHA512):
		return crypto.SHA512, nil
	default:
		return 0, fmt.Errorf("pdfsigning: unsupported digest OID %v", oid)
	}
}

func isDERNull(value derValue) bool {
	return value.Tag == 0x05 && len(value.Content) == 0
}

func publicKeysEqual(a, b any) bool {
	switch left := a.(type) {
	case *rsa.PublicKey:
		right, ok := b.(*rsa.PublicKey)
		return ok && left.E == right.E && left.N.Cmp(right.N) == 0
	case *ecdsa.PublicKey:
		right, ok := b.(*ecdsa.PublicKey)
		return ok && left.Curve == right.Curve && left.X.Cmp(right.X) == 0 && left.Y.Cmp(right.Y) == 0
	default:
		return false
	}
}

func validateDocumentSignerCertificate(cert *x509.Certificate) error {
	if cert.IsCA {
		return errors.New("pdfsigning: signer certificate must not be a CA")
	}
	if cert.KeyUsage != 0 && cert.KeyUsage&(x509.KeyUsageDigitalSignature|x509.KeyUsageContentCommitment) == 0 {
		return errors.New("pdfsigning: signer certificate is not allowed for digital signatures")
	}
	if len(cert.ExtKeyUsage) == 0 && len(cert.UnknownExtKeyUsage) == 0 {
		return nil
	}
	for _, oid := range cert.UnknownExtKeyUsage {
		if oid.Equal(oidDocumentSigning) {
			return nil
		}
	}
	return errors.New("pdfsigning: signer certificate is not allowed for document signing")
}

func decodeIssuerAndSerial(value derValue) ([]byte, *big.Int, error) {
	children, err := readDERChildren(value.Content)
	if err != nil {
		return nil, nil, err
	}
	if len(children) != 2 {
		return nil, nil, errors.New("pdfsigning: invalid issuer and serial")
	}
	var serial *big.Int
	if _, err := asn1.Unmarshal(children[1].Full, &serial); err != nil {
		return nil, nil, err
	}
	return children[0].Full, serial, nil
}

func findSignerCertificate(certs []*x509.Certificate, issuer []byte, serial *big.Int) *x509.Certificate {
	for _, cert := range certs {
		if bytes.Equal(cert.RawIssuer, issuer) && cert.SerialNumber.Cmp(serial) == 0 {
			return cert
		}
	}
	return nil
}
