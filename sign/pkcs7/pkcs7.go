// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

// Package pkcs7 keeps legacy PKCS #7 terminology separate from the CMS-first
// signing API in package sign.
package pkcs7

import (
	"crypto/x509"
	"encoding/asn1"

	"github.com/cssbruno/gopdfkit/sign"
)

// Options is kept for callers that still use the historical PKCS #7 name.
// New code should use sign.CMSOptions.
type Options = sign.CMSOptions

// VerifyResult is kept for callers that still use the historical PKCS #7 name.
// New code should use sign.CMSVerifyResult.
type VerifyResult = sign.CMSVerifyResult

// Info is kept for callers that still use the historical PKCS #7 name.
// New code should use sign.CMSInfo.
type Info = sign.CMSInfo

// RevocationInfo is kept for callers that still use the historical PKCS #7 name.
// New code should use sign.RevocationInfo.
type RevocationInfo = sign.RevocationInfo

// OtherRevocation is kept for callers that still use the historical PKCS #7 name.
// New code should use sign.OtherRevocation.
type OtherRevocation = sign.OtherRevocation

// Create creates CMS SignedData.
func Create(content []byte, options Options) ([]byte, error) {
	return sign.CreateCMS(content, options)
}

// Verify verifies attached CMS SignedData.
func Verify(signature []byte, truststore *x509.CertPool) (*VerifyResult, error) {
	return sign.VerifyCMS(signature, truststore)
}

// VerifyDetached verifies detached CMS SignedData against content.
func VerifyDetached(signature, content []byte, truststore *x509.CertPool) (*VerifyResult, error) {
	return sign.VerifyDetachedCMS(signature, content, truststore)
}

// Inspect parses CMS metadata without verifying the signature.
func Inspect(signature []byte) (*Info, error) {
	return sign.InspectCMS(signature)
}

// SignedAttributeValues returns values for the first signed attribute matching oid.
func SignedAttributeValues(signature []byte, oid asn1.ObjectIdentifier) ([]asn1.RawValue, error) {
	return sign.SignedAttributeValues(signature, oid)
}

// SignerCertificate returns the certificate referenced by the first SignerInfo.
func SignerCertificate(signature []byte) (*x509.Certificate, error) {
	return sign.SignerCertificate(signature)
}

// AdobeRevocationInfoArchivalOID returns the Adobe revocation-info archival OID.
func AdobeRevocationInfoArchivalOID() asn1.ObjectIdentifier {
	return sign.AdobeRevocationInfoArchivalOID()
}

// DecodeAdobeRevocationInfo decodes one Adobe revocation-info archival attribute value.
func DecodeAdobeRevocationInfo(value asn1.RawValue) (RevocationInfo, error) {
	return sign.DecodeAdobeRevocationInfo(value)
}

// ExtractAdobeRevocationInfo extracts Adobe revocation-info archival data from CMS SignedData.
func ExtractAdobeRevocationInfo(signature []byte) (RevocationInfo, error) {
	return sign.ExtractAdobeRevocationInfo(signature)
}

// EmbedDetached replaces the PDF /Contents hex string with signature.
func EmbedDetached(input, signature []byte) ([]byte, error) {
	return sign.EmbedDetachedCMS(input, signature)
}
