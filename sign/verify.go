// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package sign

import (
	"context"
	"crypto"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
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

// Verify verifies the latest supported CMS signature found in a signed PDF.
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

// VerifyIntegrity verifies the latest supported CMS signature's cryptographic integrity
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

// ExtractByteRange returns the latest supported PDF signature ByteRange.
func ExtractByteRange(input []byte) ([]int, error) {
	byteRange, signature, err := latestSignatureByteRange(input)
	if err != nil {
		return nil, err
	}
	if err := validateByteRange(input, byteRange); err != nil {
		return nil, err
	}
	if _, _, err := signatureContentsRange(input, signature, byteRange); err != nil {
		return nil, err
	}
	return append([]int(nil), byteRange...), nil
}

// ExtractSingleSignature extracts a PDF signature when exactly one ByteRange exists.
func ExtractSingleSignature(input []byte) (*PDFSignatureContents, error) {
	signatures, err := scanSignatureDictionaries(input)
	if err != nil {
		return nil, err
	}
	switch count := len(signatures); count {
	case 0:
		return nil, errors.New("pdfsigning: ByteRange not found")
	case 1:
		return extractSignatureDictionary(input, signatures[0])
	default:
		return nil, errors.New("pdfsigning: ambiguous ByteRange markers")
	}
}

// ExtractSignature extracts a PDF signature dictionary's CMS bytes and signed content.
func ExtractSignature(input []byte) (*PDFSignatureContents, error) {
	signatures, err := scanSignatureDictionaries(input)
	if err != nil {
		return nil, err
	}
	if len(signatures) == 0 {
		return nil, errors.New("pdfsigning: ByteRange not found")
	}
	return extractSignatureDictionary(input, signatures[len(signatures)-1])
}

// SignatureCount returns the number of supported signature values reachable
// through the current catalog's AcroForm field tree. Names in strings,
// comments, stream data, unrelated dictionaries, and unreferenced objects are
// ignored.
func SignatureCount(input []byte) int {
	signatures, _ := scanSignatureDictionaries(input)
	return len(signatures)
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
	byteRange, _, err := latestSignatureByteRange(input)
	return byteRange, err
}

func latestSignatureByteRange(input []byte) ([]int, pdfSignatureDictionary, error) {
	signatures, err := scanSignatureDictionaries(input)
	if err != nil {
		return nil, pdfSignatureDictionary{}, err
	}
	if len(signatures) == 0 {
		return nil, pdfSignatureDictionary{}, errors.New("pdfsigning: ByteRange not found")
	}
	latest := signatures[len(signatures)-1]
	byteRange, err := parseByteRangeValue(input[latest.Start+latest.ByteRange.ValueStart : latest.Start+latest.ByteRange.ValueEnd])
	return byteRange, latest, err
}

func parseByteRangeValue(value []byte) ([]int, error) {
	pos := skipPDFSpaces(value, 0)
	if pos >= len(value) || value[pos] != '[' {
		return nil, errors.New("pdfsigning: invalid ByteRange")
	}
	pos++
	values := make([]int, 0, 4)
	for {
		pos = skipPDFSpaces(value, pos)
		if pos >= len(value) {
			return nil, errors.New("pdfsigning: invalid ByteRange")
		}
		if value[pos] == ']' {
			if skipPDFSpaces(value, pos+1) != len(value) {
				return nil, errors.New("pdfsigning: invalid ByteRange")
			}
			return values, nil
		}
		if len(values) == 4 {
			return nil, errors.New("pdfsigning: unsupported ByteRange")
		}
		start := pos
		if value[pos] == '-' {
			pos++
		}
		digitStart := pos
		for pos < len(value) && value[pos] >= '0' && value[pos] <= '9' {
			pos++
		}
		if pos == digitStart || pos-start > byteRangeWidth+1 {
			return nil, errors.New("pdfsigning: invalid ByteRange value")
		}
		parsed, err := strconv.Atoi(string(value[start:pos]))
		if err != nil {
			return nil, fmt.Errorf("pdfsigning: invalid ByteRange value: %w", err)
		}
		values = append(values, parsed)
	}
}

type pdfSignatureDictionary struct {
	Start     int
	ByteRange pdfDictionaryEntry
	Contents  pdfDictionaryEntry
	HasCMS    bool
}

func scanSignatureDictionaries(input []byte) ([]pdfSignatureDictionary, error) {
	ctx := context.Background()
	reachable, err := reachableSignatureDictionaryStarts(ctx, input)
	if err != nil {
		return nil, err
	}
	starts := make([]int, 0, len(reachable))
	for start := range reachable {
		starts = append(starts, start)
	}
	sort.Ints(starts)
	signatures := make([]pdfSignatureDictionary, 0, len(starts))
	for _, start := range starts {
		signature, ok, err := signatureDictionaryAt(ctx, input, start)
		if err != nil {
			return nil, err
		}
		if ok {
			signatures = append(signatures, signature)
		}
	}
	return signatures, nil
}

func signatureDictionaryAt(ctx context.Context, input []byte, start int) (pdfSignatureDictionary, bool, error) {
	if start < 0 || start+1 >= len(input) || input[start] != '<' || input[start+1] != '<' {
		return pdfSignatureDictionary{}, false, errors.New("pdfsigning: reachable signature value is not a dictionary")
	}
	end, err := findDictionaryEndContext(ctx, input[start:])
	if err != nil {
		return pdfSignatureDictionary{}, false, err
	}
	dict := input[start : start+end]
	typeEntry, hasType, err := findDictionaryEntryContext(ctx, dict, "/Type")
	if err != nil {
		return pdfSignatureDictionary{}, false, err
	}
	if hasType {
		typeName, present, err := directOptionalNameEntry(dict, typeEntry)
		if err != nil {
			return pdfSignatureDictionary{}, false, fmt.Errorf("%w: invalid signature /Type: %w", ErrUnsupportedPDF, err)
		}
		if present && typeName != "/Sig" {
			return pdfSignatureDictionary{}, false, nil
		}
	}
	byteRange, hasByteRange, err := findDictionaryEntryContext(ctx, dict, "/ByteRange")
	if err != nil {
		return pdfSignatureDictionary{}, false, err
	}
	if !hasByteRange {
		return pdfSignatureDictionary{}, false, nil
	}
	contents, hasContents, err := findDictionaryEntryContext(ctx, dict, "/Contents")
	if err != nil {
		return pdfSignatureDictionary{}, false, err
	}
	return pdfSignatureDictionary{Start: start, ByteRange: byteRange, Contents: contents, HasCMS: hasContents}, true, nil
}

const maxVerificationFieldObjects = 10_000

type verificationField struct {
	ref          pdfRef
	inheritedSig bool
	value        inheritedFieldValue
}

type inheritedFieldValue struct {
	defined    bool
	owner      []byte
	ownerStart int
	entry      pdfDictionaryEntry
}

func reachableSignatureDictionaryStarts(ctx context.Context, input []byte) (map[int]struct{}, error) {
	xrefOffset, err := findStartXref(input)
	if err != nil {
		return nil, err
	}
	trailer, err := xrefTrailerContext(ctx, input, xrefOffset)
	if err != nil {
		return nil, err
	}
	size, root, err := parseTrailerContext(ctx, trailer, DefaultMaxXrefEntries, 0)
	if err != nil {
		return nil, err
	}
	resolver, err := newPDFXrefResolverContext(ctx, input, xrefOffset, DefaultMaxXrefChainLength, DefaultMaxXrefEntries)
	if err != nil {
		return nil, err
	}
	if size <= resolver.maxObject {
		return nil, fmt.Errorf("%w: trailer /Size %d does not cover xref object %d", ErrUnsupportedPDF, size, resolver.maxObject)
	}
	rootOffset, err := resolver.objectOffsetContext(ctx, root)
	if err != nil {
		return nil, err
	}
	rootDict, rootStart, err := readObjectDictPositionContext(ctx, input, root, rootOffset)
	if err != nil {
		return nil, err
	}
	acroEntry, hasAcroForm, err := findDictionaryEntryContext(ctx, rootDict, "/AcroForm")
	if err != nil {
		return nil, err
	}
	if !hasAcroForm {
		return map[int]struct{}{}, nil
	}
	acroDict, _, hasAcroForm, err := resolveOptionalDictionaryEntry(ctx, resolver, rootDict, rootStart, acroEntry)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid catalog /AcroForm: %w", ErrUnsupportedPDF, err)
	}
	if !hasAcroForm {
		return map[int]struct{}{}, nil
	}
	if len(acroDict) == 0 {
		return nil, fmt.Errorf("%w: catalog /AcroForm resolved to an empty dictionary", ErrUnsupportedPDF)
	}
	fieldsEntry, hasFields, err := findDictionaryEntryContext(ctx, acroDict, "/Fields")
	if err != nil {
		return nil, err
	}
	if !hasFields {
		return map[int]struct{}{}, nil
	}
	fieldRefs, hasFields, err := resolveOptionalReferenceArrayEntry(ctx, resolver, acroDict, fieldsEntry)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid AcroForm /Fields: %w", ErrUnsupportedPDF, err)
	}
	if !hasFields {
		return map[int]struct{}{}, nil
	}
	queue := make([]verificationField, 0, len(fieldRefs))
	scheduled := make(map[pdfRef]struct{}, len(fieldRefs))
	enqueue := func(field verificationField) error {
		if _, duplicate := scheduled[field.ref]; duplicate {
			return errors.New("pdfsigning: AcroForm field tree contains a cycle or duplicate reference")
		}
		if len(scheduled) >= maxVerificationFieldObjects {
			return errors.New("pdfsigning: AcroForm field tree exceeds maximum size")
		}
		scheduled[field.ref] = struct{}{}
		queue = append(queue, field)
		return nil
	}
	for _, ref := range fieldRefs {
		if err := enqueue(verificationField{ref: ref}); err != nil {
			return nil, err
		}
	}
	reachable := make(map[int]struct{})
	for len(queue) > 0 {
		field := queue[0]
		queue = queue[1:]
		offset, err := resolver.objectOffsetContext(ctx, field.ref)
		if err != nil {
			return nil, err
		}
		dict, dictStart, err := readObjectDictPositionContext(ctx, input, field.ref, offset)
		if err != nil {
			return nil, err
		}
		isSignatureField := field.inheritedSig
		ftEntry, hasFT, err := findDictionaryEntryContext(ctx, dict, "/FT")
		if err != nil {
			return nil, err
		}
		if hasFT {
			fieldType, present, err := resolveOptionalNameEntry(ctx, resolver, dict, ftEntry)
			if err != nil {
				return nil, fmt.Errorf("%w: invalid field /FT: %w", ErrUnsupportedPDF, err)
			}
			if present {
				isSignatureField = fieldType == "/Sig"
			}
		}
		value := field.value
		valueEntry, hasValue, err := findDictionaryEntryContext(ctx, dict, "/V")
		if err != nil {
			return nil, err
		}
		if hasValue {
			isNull, err := dictionaryEntryResolvesToNull(ctx, resolver, dict, valueEntry)
			if err != nil {
				return nil, fmt.Errorf("%w: invalid field /V: %w", ErrUnsupportedPDF, err)
			}
			if !isNull {
				value = inheritedFieldValue{defined: true, owner: dict, ownerStart: dictStart, entry: valueEntry}
			}
		}
		kidsEntry, hasKids, err := findDictionaryEntryContext(ctx, dict, "/Kids")
		if err != nil {
			return nil, err
		}
		hasChildren := false
		if hasKids {
			kids, present, err := resolveOptionalReferenceArrayEntry(ctx, resolver, dict, kidsEntry)
			if err != nil {
				return nil, fmt.Errorf("%w: invalid field /Kids: %w", ErrUnsupportedPDF, err)
			}
			if present {
				hasChildren = len(kids) > 0
				for _, kid := range kids {
					if err := enqueue(verificationField{ref: kid, inheritedSig: isSignatureField, value: value}); err != nil {
						return nil, err
					}
				}
			}
		}
		if !hasChildren && isSignatureField && value.defined {
			_, signatureStart, signed, err := resolveOptionalDictionaryEntry(ctx, resolver, value.owner, value.ownerStart, value.entry)
			if err != nil {
				return nil, fmt.Errorf("%w: invalid signature field /V: %w", ErrUnsupportedPDF, err)
			}
			if signed {
				reachable[signatureStart] = struct{}{}
			}
		}
	}
	return reachable, nil
}

func directOptionalNameEntry(owner []byte, entry pdfDictionaryEntry) (string, bool, error) {
	value := owner[entry.ValueStart:entry.ValueEnd]
	pos := skipPDFSpaces(value, 0)
	if isPDFNull(value[pos:]) {
		return "", false, nil
	}
	if pos >= len(value) || value[pos] != '/' {
		return "", false, errors.New("value is neither null nor a direct name")
	}
	name, end, err := decodePDFNameToken(value, pos)
	if err != nil {
		return "", false, err
	}
	if skipPDFSpaces(value, end) != len(value) {
		return "", false, errors.New("name value contains extra data")
	}
	return name, true, nil
}

func resolveOptionalNameEntry(ctx context.Context, resolver *pdfXrefResolver, owner []byte, entry pdfDictionaryEntry) (string, bool, error) {
	value := owner[entry.ValueStart:entry.ValueEnd]
	pos := skipPDFSpaces(value, 0)
	if isPDFNull(value[pos:]) {
		return "", false, nil
	}
	if pos < len(value) && value[pos] == '/' {
		name, end, err := decodePDFNameToken(value, pos)
		if err != nil {
			return "", false, err
		}
		if skipPDFSpaces(value, end) != len(value) {
			return "", false, errors.New("name value contains extra data")
		}
		return name, true, nil
	}
	ref, ok := parseReferenceValue(value)
	if !ok {
		return "", false, errors.New("value is neither null, a name, nor an indirect reference")
	}
	indirect, _, err := resolver.indirectValueContext(ctx, ref)
	if err != nil {
		return "", false, err
	}
	indirectPos := skipPDFSpaces(indirect, 0)
	if isPDFNull(indirect[indirectPos:]) {
		return "", false, nil
	}
	if indirectPos >= len(indirect) || indirect[indirectPos] != '/' {
		return "", false, errors.New("indirect value is neither null nor a name")
	}
	name, end, err := decodePDFNameToken(indirect, indirectPos)
	if err != nil {
		return "", false, err
	}
	if skipPDFSpaces(indirect, end) != len(indirect) {
		return "", false, errors.New("indirect name value contains extra data")
	}
	return name, true, nil
}

func dictionaryEntryResolvesToNull(ctx context.Context, resolver *pdfXrefResolver, owner []byte, entry pdfDictionaryEntry) (bool, error) {
	value := owner[entry.ValueStart:entry.ValueEnd]
	pos := skipPDFSpaces(value, 0)
	if isPDFNull(value[pos:]) {
		return true, nil
	}
	ref, ok := parseReferenceValue(value)
	if !ok {
		return false, nil
	}
	indirect, _, err := resolver.indirectValueContext(ctx, ref)
	if err != nil {
		return false, err
	}
	indirectPos := skipPDFSpaces(indirect, 0)
	return isPDFNull(indirect[indirectPos:]), nil
}

func resolveOptionalDictionaryEntry(ctx context.Context, resolver *pdfXrefResolver, owner []byte, ownerStart int, entry pdfDictionaryEntry) ([]byte, int, bool, error) {
	value := owner[entry.ValueStart:entry.ValueEnd]
	pos := skipPDFSpaces(value, 0)
	if isPDFNull(value[pos:]) {
		return nil, 0, false, nil
	}
	if pos+1 < len(value) && value[pos] == '<' && value[pos+1] == '<' {
		return value[pos:], ownerStart + entry.ValueStart + pos, true, nil
	}
	ref, ok := parseReferenceValue(value)
	if !ok {
		return nil, 0, false, errors.New("value is neither null, a dictionary, nor an indirect reference")
	}
	indirect, start, err := resolver.indirectValueContext(ctx, ref)
	if err != nil {
		return nil, 0, false, err
	}
	indirectPos := skipPDFSpaces(indirect, 0)
	if isPDFNull(indirect[indirectPos:]) {
		return nil, 0, false, nil
	}
	if indirectPos+1 >= len(indirect) || indirect[indirectPos] != '<' || indirect[indirectPos+1] != '<' {
		return nil, 0, false, errors.New("indirect value is neither null nor a dictionary")
	}
	return indirect[indirectPos:], start + indirectPos, true, nil
}

func isPDFNull(value []byte) bool {
	return hasPDFKeywordAt(value, 0, "null") && skipPDFSpaces(value, len("null")) == len(value)
}

func resolveOptionalReferenceArrayEntry(ctx context.Context, resolver *pdfXrefResolver, owner []byte, entry pdfDictionaryEntry) ([]pdfRef, bool, error) {
	value := owner[entry.ValueStart:entry.ValueEnd]
	pos := skipPDFSpaces(value, 0)
	if isPDFNull(value[pos:]) {
		return nil, false, nil
	}
	if pos < len(value) && value[pos] == '[' {
		refs, err := parseReferenceArray(value[pos:])
		return refs, true, err
	}
	ref, ok := parseReferenceValue(value)
	if !ok {
		return nil, false, errors.New("array value is neither null, direct, nor an indirect reference")
	}
	indirect, _, err := resolver.indirectValueContext(ctx, ref)
	if err != nil {
		return nil, false, err
	}
	indirectPos := skipPDFSpaces(indirect, 0)
	if isPDFNull(indirect[indirectPos:]) {
		return nil, false, nil
	}
	refs, err := parseReferenceArray(indirect[indirectPos:])
	return refs, true, err
}

func (resolver *pdfXrefResolver) indirectValueContext(ctx context.Context, ref pdfRef) ([]byte, int, error) {
	if resolver == nil {
		return nil, 0, errors.New("pdfsigning: xref resolver not initialized")
	}
	if err := signContextErr(ctx); err != nil {
		return nil, 0, err
	}
	if cached, ok := resolver.indirectValues[ref]; ok {
		return cached.data, cached.start, nil
	}
	offset, err := resolver.objectOffsetContext(ctx, ref)
	if err != nil {
		return nil, 0, err
	}
	data, start, err := readIndirectValuePositionContext(ctx, resolver.input, ref, offset)
	if err != nil {
		return nil, 0, err
	}
	if resolver.indirectValues == nil {
		resolver.indirectValues = make(map[pdfRef]cachedIndirectValue)
	}
	resolver.indirectValues[ref] = cachedIndirectValue{data: data, start: start}
	return data, start, nil
}

func parseReferenceArray(value []byte) ([]pdfRef, error) {
	pos := skipPDFSpaces(value, 0)
	if pos >= len(value) || value[pos] != '[' {
		return nil, errors.New("reference array start not found")
	}
	pos++
	refs := make([]pdfRef, 0, 1)
	for {
		pos = skipPDFSpaces(value, pos)
		if pos >= len(value) {
			return nil, errors.New("reference array end not found")
		}
		if value[pos] == ']' {
			if skipPDFSpaces(value, pos+1) != len(value) {
				return nil, errors.New("extra data after reference array")
			}
			return refs, nil
		}
		if len(refs) >= maxVerificationFieldObjects {
			return nil, errors.New("reference array exceeds maximum size")
		}
		object, next, ok := parseLeadingInt(value, pos)
		if !ok {
			return nil, errors.New("invalid object number in reference array")
		}
		generation, next, ok := parseLeadingInt(value, skipPDFSpaces(value, next))
		if !ok {
			return nil, errors.New("invalid generation in reference array")
		}
		marker := skipPDFSpaces(value, next)
		if marker >= len(value) || value[marker] != 'R' || !isPDFTokenEnd(value, marker+1) {
			return nil, errors.New("invalid indirect reference in array")
		}
		refs = append(refs, pdfRef{Object: object, Generation: generation})
		pos = marker + 1
	}
}

func readIndirectValuePositionContext(ctx context.Context, input []byte, ref pdfRef, offset int) ([]byte, int, error) {
	if offset < 0 || offset >= len(input) {
		return nil, 0, errors.New("pdfsigning: object offset outside PDF")
	}
	object, next, ok := parseLeadingInt(input, offset)
	if !ok {
		return nil, 0, errors.New("pdfsigning: object header not found")
	}
	pos := skipPDFSpaces(input, next)
	generation, next, ok := parseLeadingInt(input, pos)
	if !ok {
		return nil, 0, errors.New("pdfsigning: object generation not found")
	}
	pos = skipPDFSpaces(input, next)
	if !hasPDFKeywordAt(input, pos, "obj") {
		return nil, 0, errors.New("pdfsigning: object header marker not found")
	}
	if object != ref.Object || generation != ref.Generation {
		return nil, 0, fmt.Errorf("pdfsigning: xref points to object %d %d, want %d %d", object, generation, ref.Object, ref.Generation)
	}
	start := skipPDFSpaces(input, pos+len("obj"))
	end, err := pdfValueEndContext(ctx, input, start)
	if err != nil {
		return nil, 0, err
	}
	endobj := skipPDFSpaces(input, end)
	if !hasPDFKeywordAt(input, endobj, "endobj") {
		return nil, 0, errors.New("pdfsigning: object terminator not found after value")
	}
	return input[start:end], start, nil
}

func extractSignatureDictionary(input []byte, signature pdfSignatureDictionary) (*PDFSignatureContents, error) {
	byteRange, err := parseByteRangeValue(input[signature.Start+signature.ByteRange.ValueStart : signature.Start+signature.ByteRange.ValueEnd])
	if err != nil {
		return nil, err
	}
	if err := validateByteRange(input, byteRange); err != nil {
		return nil, err
	}
	contentsStart, contentsEnd, err := signatureContentsRange(input, signature, byteRange)
	if err != nil {
		return nil, err
	}
	cms, err := extractSignatureContents(input, contentsStart, contentsEnd)
	if err != nil {
		return nil, err
	}
	return &PDFSignatureContents{
		ByteRange:     append([]int(nil), byteRange...),
		ContentsStart: contentsStart,
		ContentsEnd:   contentsEnd,
		CMS:           cms,
		SignedContent: signedContent(input, byteRange),
	}, nil
}

func signatureContentsRange(input []byte, signature pdfSignatureDictionary, byteRange []int) (int, int, error) {
	if !signature.HasCMS {
		return 0, 0, errors.New("pdfsigning: signature /Contents not found")
	}
	contentsStart := signature.Start + signature.Contents.ValueStart
	contentsEnd := signature.Start + signature.Contents.ValueEnd
	if contentsStart < 0 || contentsEnd > len(input) || contentsStart >= contentsEnd {
		return 0, 0, errors.New("pdfsigning: invalid signature Contents range")
	}
	if len(byteRange) != byteRangeLength || byteRange[1] != contentsStart || byteRange[2] != contentsEnd {
		return 0, 0, errors.New("pdfsigning: ByteRange does not select its signature Contents")
	}
	if input[contentsStart] != '<' || input[contentsEnd-1] != '>' {
		return 0, 0, errors.New("pdfsigning: signature contents is not a hex string")
	}
	return contentsStart, contentsEnd, nil
}

func parseReferenceValue(value []byte) (pdfRef, bool) {
	pos := skipPDFSpaces(value, 0)
	object, next, ok := parseLeadingInt(value, pos)
	if !ok {
		return pdfRef{}, false
	}
	generation, next, ok := parseLeadingInt(value, skipPDFSpaces(value, next))
	if !ok {
		return pdfRef{}, false
	}
	marker := skipPDFSpaces(value, next)
	if marker >= len(value) || value[marker] != 'R' || skipPDFSpaces(value, marker+1) != len(value) {
		return pdfRef{}, false
	}
	return pdfRef{Object: object, Generation: generation}, true
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
