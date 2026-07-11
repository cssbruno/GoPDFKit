// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package sign

import (
	"crypto"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
)

const byteRangeLength = 4

// PDFSignature contains a verified PDF signature summary.
type PDFSignature struct {
	// ByteRange is the signed byte range declared by the PDF signature.
	ByteRange []int
	// CMS contains the verified CMS signature details.
	CMS *CMSVerifyResult
}

// PDFSignatureContents contains the raw signature bytes and the signed content
// selected by a PDF signature dictionary.
type PDFSignatureContents struct {
	// ByteRange is the signed byte range declared by the PDF signature.
	ByteRange []int
	// ContentsStart is the byte offset of the /Contents opening "<".
	ContentsStart int
	// ContentsEnd is the byte offset immediately after the /Contents closing ">".
	ContentsEnd int
	// CMS is the DER CMS signature from /Contents, without zero padding.
	CMS []byte
	// SignedContent is the concatenated PDF bytes covered by ByteRange.
	SignedContent []byte
}

// MaxSignatureBytes returns the maximum DER CMS size that fits in /Contents.
func (contents *PDFSignatureContents) MaxSignatureBytes() int {
	hexLen := contents.ContentsHexLen()
	if hexLen <= 0 {
		return 0
	}
	return hexLen / 2
}

// ContentsHexLen returns the reserved /Contents hex-string length.
func (contents *PDFSignatureContents) ContentsHexLen() int {
	if contents == nil || contents.ContentsEnd <= contents.ContentsStart+2 {
		return 0
	}
	return contents.ContentsEnd - contents.ContentsStart - 2
}

// ByteRange64 returns ByteRange as the fixed-width int64 tuple used by many APIs.
func (contents *PDFSignatureContents) ByteRange64() ([4]int64, error) {
	if contents == nil {
		return [4]int64{}, errors.New("pdfsigning: signature contents are required")
	}
	return ByteRangeFromInts(contents.ByteRange)
}

// Verify verifies the first CMS signature found in a signed PDF.
func Verify(input []byte, truststore *x509.CertPool) (*PDFSignature, error) {
	if truststore == nil {
		return nil, ErrTrustStoreRequired
	}
	extracted, err := ExtractSignature(input)
	if err != nil {
		return nil, err
	}
	result, err := VerifyDetachedCMS(extracted.CMS, extracted.SignedContent, truststore)
	if err != nil {
		return nil, err
	}
	if !result.Detached {
		return nil, errors.New("pdfsigning: PDF signature CMS must be detached")
	}
	return &PDFSignature{ByteRange: extracted.ByteRange, CMS: result}, nil
}

// VerifyIntegrity verifies the first CMS signature's cryptographic integrity
// without establishing signer trust. It must not be used for authorization.
func VerifyIntegrity(input []byte) (*PDFSignature, error) {
	extracted, err := ExtractSignature(input)
	if err != nil {
		return nil, err
	}
	result, err := VerifyDetachedCMSIntegrity(extracted.CMS, extracted.SignedContent)
	if err != nil {
		return nil, err
	}
	if !result.Detached {
		return nil, errors.New("pdfsigning: PDF signature CMS must be detached")
	}
	return &PDFSignature{ByteRange: extracted.ByteRange, CMS: result}, nil
}

// ExtractByteRange returns the first supported PDF signature ByteRange.
func ExtractByteRange(input []byte) ([]int, error) {
	byteRange, err := extractByteRange(input)
	if err != nil {
		return nil, err
	}
	if err := validateByteRange(input, byteRange); err != nil {
		return nil, err
	}
	return append([]int(nil), byteRange...), nil
}

// ExtractSingleSignature extracts a PDF signature when exactly one ByteRange exists.
func ExtractSingleSignature(input []byte) (*PDFSignatureContents, error) {
	switch count := SignatureCount(input); count {
	case 0:
		return nil, errors.New("pdfsigning: ByteRange not found")
	case 1:
		return ExtractSignature(input)
	default:
		return nil, errors.New("pdfsigning: ambiguous ByteRange markers")
	}
}

// ExtractSignature extracts a PDF signature dictionary's CMS bytes and signed content.
func ExtractSignature(input []byte) (*PDFSignatureContents, error) {
	byteRange, err := ExtractByteRange(input)
	if err != nil {
		return nil, err
	}
	cms, err := extractSignatureContents(input, byteRange[1], byteRange[2])
	if err != nil {
		return nil, err
	}
	return &PDFSignatureContents{
		ByteRange:     byteRange,
		ContentsStart: byteRange[1],
		ContentsEnd:   byteRange[2],
		CMS:           cms,
		SignedContent: signedContent(input, byteRange),
	}, nil
}

// SignatureCount returns the number of PDF signature ByteRange markers.
func SignatureCount(input []byte) int {
	return countPDFName(input, "/ByteRange")
}

// ByteRangeFromInts converts a parsed ByteRange into a fixed int64 tuple.
func ByteRangeFromInts(byteRange []int) ([4]int64, error) {
	if len(byteRange) != byteRangeLength {
		return [4]int64{}, errors.New("pdfsigning: unsupported ByteRange")
	}
	if byteRange[0] < 0 || byteRange[1] < 0 || byteRange[2] < 0 || byteRange[3] < 0 {
		return [4]int64{}, errors.New("pdfsigning: invalid ByteRange values")
	}
	return [4]int64{int64(byteRange[0]), int64(byteRange[1]), int64(byteRange[2]), int64(byteRange[3])}, nil
}

// ByteRangeToInts converts a fixed int64 ByteRange tuple into parser indexes.
func ByteRangeToInts(byteRange [4]int64) ([]int, error) {
	const maxInt = int64(^uint(0) >> 1)

	for _, value := range byteRange {
		if value < 0 || value > maxInt {
			return nil, errors.New("pdfsigning: invalid ByteRange values")
		}
	}
	return []int{int(byteRange[0]), int(byteRange[1]), int(byteRange[2]), int(byteRange[3])}, nil
}

// SignedContent returns the PDF bytes covered by byteRange.
func SignedContent(input []byte, byteRange []int) ([]byte, error) {
	if err := validateByteRange(input, byteRange); err != nil {
		return nil, err
	}
	return signedContent(input, byteRange), nil
}

// SignedContentForByteRange returns the PDF bytes covered by byteRange.
func SignedContentForByteRange(input []byte, byteRange [4]int64) ([]byte, error) {
	ints, err := ByteRangeToInts(byteRange)
	if err != nil {
		return nil, err
	}
	return SignedContent(input, ints)
}

// DigestHex returns the hex digest of the PDF bytes covered by /ByteRange.
func DigestHex(input []byte, digest crypto.Hash) (string, error) {
	digest, err := normalizeDigest(digest)
	if err != nil {
		return "", err
	}
	extracted, err := ExtractSignature(input)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(hashBytes(digest, extracted.SignedContent)), nil
}

// DigestHexForByteRange returns the hex digest of the PDF bytes covered by byteRange.
func DigestHexForByteRange(input []byte, byteRange [4]int64, digest crypto.Hash) (string, error) {
	digest, err := normalizeDigest(digest)
	if err != nil {
		return "", err
	}
	content, err := SignedContentForByteRange(input, byteRange)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(hashBytes(digest, content)), nil
}

// EmbedDetachedCMS replaces the PDF /Contents hex string with signature.
func EmbedDetachedCMS(input, signature []byte) ([]byte, error) {
	if len(input) == 0 {
		return nil, ErrMissingInput
	}
	if len(signature) == 0 {
		return nil, errors.New("pdfsigning: CMS signature is required")
	}
	byteRange, err := ExtractByteRange(input)
	if err != nil {
		return nil, err
	}
	contentsStart := byteRange[1]
	contentsEnd := byteRange[2]
	if contentsStart >= contentsEnd || input[contentsStart] != '<' || input[contentsEnd-1] != '>' {
		return nil, errors.New("pdfsigning: signature contents is not a hex string")
	}
	hexStart := contentsStart + 1
	hexLen := contentsEnd - contentsStart - 2
	if hexLen <= 0 || hexLen%2 != 0 {
		return nil, errors.New("pdfsigning: invalid signature contents range")
	}
	if hex.EncodedLen(len(signature)) > hexLen {
		return nil, errors.New("pdfsigning: CMS signature too large for placeholder")
	}

	out := make([]byte, len(input))
	copy(out, input)
	written := hex.Encode(out[hexStart:], signature)
	for i := hexStart + written; i < hexStart+hexLen; i++ {
		out[i] = '0'
	}
	return out, nil
}

func validateByteRange(input []byte, byteRange []int) error {
	if len(byteRange) != byteRangeLength {
		return errors.New("pdfsigning: unsupported ByteRange")
	}
	if byteRange[0] != 0 || byteRange[1] < 0 || byteRange[2] < 0 || byteRange[3] < 0 {
		return errors.New("pdfsigning: invalid ByteRange values")
	}
	if byteRange[1] > len(input) || byteRange[2] > len(input) || byteRange[3] > len(input)-byteRange[2] {
		return errors.New("pdfsigning: ByteRange exceeds PDF size")
	}
	if byteRange[1] > byteRange[2] {
		return errors.New("pdfsigning: ByteRange overlaps signature contents")
	}
	if byteRange[2]+byteRange[3] != len(input) {
		return errors.New("pdfsigning: ByteRange does not cover full PDF")
	}
	return nil
}

func signedContent(input []byte, byteRange []int) []byte {
	content := make([]byte, 0, byteRange[1]+byteRange[3])
	content = append(content, input[:byteRange[1]]...)
	content = append(content, input[byteRange[2]:byteRange[2]+byteRange[3]]...)
	return content
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
