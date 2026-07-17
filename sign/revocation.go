// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package sign

import (
	"encoding/asn1"
	"errors"
	"fmt"
)

// RevocationInfo contains Adobe revocation-info archival evidence carried in a
// CMS signed attribute for PAdES workflows.
type RevocationInfo struct {
	// CRL contains DER-encoded certificate revocation lists.
	CRL []asn1.RawValue `asn1:"tag:0,optional,explicit"`
	// OCSP contains DER-encoded OCSP responses.
	OCSP []asn1.RawValue `asn1:"tag:1,optional,explicit"`
	// Other contains additional DER-encoded revocation evidence.
	Other []OtherRevocation `asn1:"tag:2,optional,explicit"`
}

// OtherRevocation contains non-CRL and non-OCSP revocation evidence.
type OtherRevocation struct {
	// Type identifies the revocation evidence type.
	Type asn1.ObjectIdentifier
	// Value contains DER-encoded revocation evidence.
	Value []byte
}

// AddCRL appends DER-encoded CRL evidence.
func (r *RevocationInfo) AddCRL(value []byte) error {
	r.CRL = append(r.CRL, asn1.RawValue{FullBytes: value})

	return nil
}

// AddOCSP appends DER-encoded OCSP evidence.
func (r *RevocationInfo) AddOCSP(value []byte) error {
	r.OCSP = append(r.OCSP, asn1.RawValue{FullBytes: value})

	return nil
}

// AdobeRevocationInfoArchivalOID returns Adobe's revocation-info archival OID.
func AdobeRevocationInfoArchivalOID() asn1.ObjectIdentifier {
	return asn1.ObjectIdentifier{1, 2, 840, 113583, 1, 1, 8}
}

// DecodeAdobeRevocationInfo decodes one Adobe revocation-info archival signed
// attribute value.
func DecodeAdobeRevocationInfo(value asn1.RawValue) (RevocationInfo, error) {
	if len(value.FullBytes) == 0 {
		return RevocationInfo{}, errors.New("pdfsigning: revocation attribute is empty")
	}

	var revInfo RevocationInfo
	if _, err := asn1.Unmarshal(value.FullBytes, &revInfo); err != nil {
		return RevocationInfo{}, fmt.Errorf("pdfsigning: decode revocation attribute: %w", err)
	}

	return revInfo, nil
}

// ExtractAdobeRevocationInfo extracts Adobe revocation-info archival data from
// CMS SignedData.
func ExtractAdobeRevocationInfo(signature []byte) (RevocationInfo, error) {
	values, err := SignedAttributeValues(signature, AdobeRevocationInfoArchivalOID())
	if err != nil {
		return RevocationInfo{}, fmt.Errorf("pdfsigning: read revocation attribute: %w", err)
	}

	if len(values) == 0 {
		return RevocationInfo{}, nil
	}

	revInfo, err := DecodeAdobeRevocationInfo(values[0])
	if err != nil {
		return RevocationInfo{}, err
	}

	return revInfo, nil
}
