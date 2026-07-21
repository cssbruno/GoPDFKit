// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package sign

import (
	"crypto/x509"
	"encoding/asn1"
	"errors"
)

// SignedAttribute is one CMS signed attribute and its ASN.1 values.
type SignedAttribute struct {
	// OID identifies the signed attribute.
	OID asn1.ObjectIdentifier
	// Values contains the attribute set values as raw ASN.1 values.
	Values []asn1.RawValue
}

// CMSInfo contains CMS metadata useful for policy checks.
type CMSInfo struct {
	// Certificates contains certificates embedded in the CMS payload.
	Certificates []*x509.Certificate
	// Signer is the certificate referenced by the first SignerInfo.
	Signer *x509.Certificate
	// SignedAttributes contains signed attributes from the first SignerInfo.
	SignedAttributes []SignedAttribute
}

// InspectCMS parses CMS metadata without verifying the signature.
func InspectCMS(signature []byte) (*CMSInfo, error) {
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

	return inspectSignedData(signedDataValue)
}

// SignedAttributeValues returns values for the first CMS signed attribute matching oid.
func SignedAttributeValues(signature []byte, oid asn1.ObjectIdentifier) ([]asn1.RawValue, error) {
	if len(oid) == 0 {
		return nil, errors.New("pdfsigning: signed attribute OID is required")
	}

	info, err := InspectCMS(signature)
	if err != nil {
		return nil, err
	}
	for _, attr := range info.SignedAttributes {
		if attr.OID.Equal(oid) {
			return cloneRawValues(attr.Values), nil
		}
	}
	return nil, nil
}

// SignerCertificate returns the certificate referenced by the first SignerInfo.
func SignerCertificate(signature []byte) (*x509.Certificate, error) {
	info, err := InspectCMS(signature)
	if err != nil {
		return nil, err
	}
	if info.Signer == nil {
		return nil, errors.New("pdfsigning: signer certificate not found")
	}
	return info.Signer, nil
}

func inspectSignedData(signedDataValue derValue) (*CMSInfo, error) {
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

	certificates, signerInfos, err := parseCertificatesAndSigners(children[3:])
	if err != nil {
		return nil, err
	}
	if len(signerInfos) == 0 {
		return nil, errors.New("pdfsigning: CMS package has no signer info")
	}

	signerChildren, err := readSignerInfoChildren(signerInfos[0])
	if err != nil {
		return nil, err
	}
	issuer, serial, err := decodeIssuerAndSerial(signerChildren[1])
	if err != nil {
		return nil, err
	}
	signedAttrs, err := inspectSignedAttributes(signerChildren)
	if err != nil {
		return nil, err
	}

	return &CMSInfo{
		Certificates:     append([]*x509.Certificate(nil), certificates...),
		Signer:           findSignerCertificate(certificates, issuer, serial),
		SignedAttributes: signedAttrs,
	}, nil
}

func readSignerInfoChildren(signerInfo derValue) ([]derValue, error) {
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
	return children, nil
}

func inspectSignedAttributes(signerInfoChildren []derValue) ([]SignedAttribute, error) {
	if signerInfoChildren[3].Tag != 0xa0 {
		return nil, errors.New("pdfsigning: signed attributes are required")
	}

	rawAttrs, err := readDERChildren(signerInfoChildren[3].Content)
	if err != nil {
		return nil, err
	}

	attrs := make([]SignedAttribute, 0, len(rawAttrs))
	for _, rawAttr := range rawAttrs {
		if err := expectTag(rawAttr, 0x30, "signed attribute"); err != nil {
			return nil, err
		}

		attrChildren, err := readDERChildren(rawAttr.Content)
		if err != nil {
			return nil, err
		}
		if len(attrChildren) != 2 {
			return nil, errors.New("pdfsigning: invalid signed attribute")
		}

		oid, err := decodeOID(attrChildren[0])
		if err != nil {
			return nil, err
		}
		if err := expectTag(attrChildren[1], 0x31, "signed attribute values"); err != nil {
			return nil, err
		}

		rawValues, err := readDERChildren(attrChildren[1].Content)
		if err != nil {
			return nil, err
		}
		values := make([]asn1.RawValue, 0, len(rawValues))
		for _, raw := range rawValues {
			var value asn1.RawValue
			if _, err := asn1.Unmarshal(raw.Full, &value); err != nil {
				return nil, err
			}
			values = append(values, value)
		}

		attrs = append(attrs, SignedAttribute{OID: oid, Values: values})
	}

	return attrs, nil
}

func cloneRawValues(values []asn1.RawValue) []asn1.RawValue {
	if len(values) == 0 {
		return nil
	}
	out := make([]asn1.RawValue, len(values))
	copy(out, values)
	return out
}
