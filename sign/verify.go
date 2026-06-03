// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package sign

import (
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
)

// PDFSignature contains a verified PDF signature summary.
type PDFSignature struct {
	// ByteRange is the signed byte range declared by the PDF signature.
	ByteRange []int
	// CMS contains the verified CMS/PKCS7 signature details.
	CMS *PKCS7VerifyResult
}

// Verify verifies the first CMS/PKCS7 signature found in a signed PDF.
func Verify(input []byte, truststore *x509.CertPool) (*PDFSignature, error) {
	byteRange, err := extractByteRange(input)
	if err != nil {
		return nil, err
	}
	if len(byteRange) != 4 {
		return nil, errors.New("pdfsigning: unsupported ByteRange")
	}
	if byteRange[0] != 0 || byteRange[1] < 0 || byteRange[2] < 0 || byteRange[3] < 0 {
		return nil, errors.New("pdfsigning: invalid ByteRange values")
	}
	if byteRange[1] > len(input) || byteRange[2] > len(input) || byteRange[3] > len(input)-byteRange[2] {
		return nil, errors.New("pdfsigning: ByteRange exceeds PDF size")
	}
	if byteRange[1] > byteRange[2] {
		return nil, errors.New("pdfsigning: ByteRange overlaps signature contents")
	}
	if byteRange[2]+byteRange[3] != len(input) {
		return nil, errors.New("pdfsigning: ByteRange does not cover full PDF")
	}
	signedContent := make([]byte, 0, byteRange[1]+byteRange[3])
	signedContent = append(signedContent, input[:byteRange[1]]...)
	signedContent = append(signedContent, input[byteRange[2]:byteRange[2]+byteRange[3]]...)
	cms, err := extractSignatureContents(input, byteRange[1], byteRange[2])
	if err != nil {
		return nil, err
	}
	result, err := VerifyDetachedPKCS7(cms, signedContent, truststore)
	if err != nil {
		return nil, err
	}
	if !result.Detached {
		return nil, errors.New("pdfsigning: PDF signature CMS must be detached")
	}
	return &PDFSignature{ByteRange: byteRange, CMS: result}, nil
}

func extractByteRange(input []byte) ([]int, error) {
	idx := findLastPDFName(input, "/ByteRange")
	if idx < 0 {
		return nil, errors.New("pdfsigning: ByteRange not found")
	}
	pos := skipPDFSpaces(input, idx+len("/ByteRange"))
	if pos >= len(input) || input[pos] != '[' {
		return nil, errors.New("pdfsigning: invalid ByteRange")
	}
	pos++
	values := make([]int, 0, 4)
	for {
		pos = skipPDFSpaces(input, pos)
		if pos >= len(input) {
			return nil, errors.New("pdfsigning: invalid ByteRange")
		}
		if input[pos] == ']' {
			return values, nil
		}
		if len(values) == 4 {
			return nil, errors.New("pdfsigning: unsupported ByteRange")
		}
		start := pos
		if input[pos] == '-' {
			pos++
		}
		digitStart := pos
		for pos < len(input) && input[pos] >= '0' && input[pos] <= '9' {
			pos++
		}
		if pos == digitStart || pos-start > byteRangeWidth+1 {
			return nil, errors.New("pdfsigning: invalid ByteRange value")
		}
		value, err := strconv.Atoi(string(input[start:pos]))
		if err != nil {
			return nil, fmt.Errorf("pdfsigning: invalid ByteRange value: %w", err)
		}
		values = append(values, value)
	}
}

func extractSignatureContents(input []byte, contentsStart, contentsEnd int) ([]byte, error) {
	if contentsStart < 0 || contentsEnd > len(input) || contentsStart >= contentsEnd {
		return nil, errors.New("pdfsigning: invalid signature contents range")
	}
	if input[contentsStart] != '<' || input[contentsEnd-1] != '>' {
		return nil, errors.New("pdfsigning: signature contents is not a hex string")
	}
	hexBytes := input[contentsStart+1 : contentsEnd-1]
	if hex.DecodedLen(len(hexBytes)) > maxCMSPackageBytes {
		return nil, errors.New("pdfsigning: signature contents exceeds maximum size")
	}
	out := make([]byte, hex.DecodedLen(len(hexBytes)))
	if _, err := hex.Decode(out, hexBytes); err != nil {
		return nil, fmt.Errorf("decode signature contents: %w", err)
	}
	value, rest, err := readDER(out)
	if err != nil {
		return nil, err
	}
	for _, b := range rest {
		if b != 0 {
			return nil, errors.New("pdfsigning: non-zero trailing signature contents")
		}
	}
	return value.Full, nil
}
