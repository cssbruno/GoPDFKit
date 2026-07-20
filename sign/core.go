// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package sign

import (
	"bytes"
	"context"
	"crypto"
	_ "crypto/sha256" // Register SHA-256 algorithms with crypto.Hash.
	_ "crypto/sha512" // Register SHA-512 algorithms with crypto.Hash.
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultSignatureBytes = 16384
	maxSignatureBytes     = 1 << 20
	// DefaultMaxSourceBytes bounds PDF inputs accepted by signing APIs.
	DefaultMaxSourceBytes int64 = 128 * 1024 * 1024
	// DefaultMaxXrefChainLength bounds incremental PDF revision chains.
	DefaultMaxXrefChainLength = 128
	// DefaultMaxXrefEntries bounds the declared/scanned objects in one xref table.
	DefaultMaxXrefEntries = 1_000_000
	byteRangeWidth        = 20
	refPatternFormat      = `%s\s+(\d+)\s+(\d+)\s+R`
)

const (
	// SubFilterETSI_CAdESDetached advertises a CAdES detached PDF signature.
	SubFilterETSI_CAdESDetached = "ETSI.CAdES.detached"
	// SubFilterAdobePKCS7Detached advertises a detached PKCS#7/CMS PDF signature.
	SubFilterAdobePKCS7Detached = "adbe.pkcs7.detached"
)

var (
	// ErrMissingInput is returned when a PDF input path or byte slice is empty.
	ErrMissingInput = errors.New("pdfsigning: input is required")
	// ErrMissingOutput is returned when the signed PDF output path is empty.
	ErrMissingOutput = errors.New("pdfsigning: output is required")
	// ErrMissingSigner is returned when no signing key is configured.
	ErrMissingSigner = errors.New("pdfsigning: signer is required")
	// ErrMissingCertificate is returned when no signing certificate is configured.
	ErrMissingCertificate = errors.New("pdfsigning: certificate is required")
	// ErrUnsupportedPDF is returned when a source PDF uses a structure that the
	// intentionally narrow signing parser cannot preserve safely.
	ErrUnsupportedPDF = errors.New("pdfsigning: unsupported PDF structure")
)

// Options configures a PDF signature.
type Options struct {
	// Signer signs the CMS payload for the PDF signature.
	Signer crypto.Signer
	// Certificate is the signing certificate and must match Signer.
	Certificate *x509.Certificate
	// CertificateChain contains optional intermediate certificates to include.
	CertificateChain []*x509.Certificate
	// DigestAlgorithm selects the message digest. A zero value uses SHA-256.
	DigestAlgorithm crypto.Hash
	// Name is the signer name stored in the PDF signature dictionary.
	Name string
	// Location is the signing location stored in the PDF signature dictionary.
	Location string
	// Reason is the signing reason stored in the PDF signature dictionary.
	Reason string
	// ContactInfo is signer contact information stored in the signature dictionary.
	ContactInfo string
	// SubFilter selects the PDF signature SubFilter. A zero value uses
	// ETSI.CAdES.detached for backward-compatible PAdES-oriented output.
	SubFilter string
	// FieldName is the PDF signature field name. A zero value uses "Signature1".
	FieldName string
	// SigningTime sets the signature timestamp. A zero value uses now.
	SigningTime time.Time
	// SignatureSize is the reserved CMS signature size in bytes.
	SignatureSize int
	// MaxSourceBytes bounds the source PDF. Zero uses DefaultMaxSourceBytes.
	MaxSourceBytes int64
	// MaxXrefChainLength bounds incremental xref revisions. Zero uses
	// DefaultMaxXrefChainLength.
	MaxXrefChainLength int
	// MaxXrefEntries bounds declared and scanned classic xref entries. Zero uses
	// DefaultMaxXrefEntries.
	MaxXrefEntries int
}

type preparedOptions struct {
	Options
	DigestAlgorithm crypto.Hash
	SubFilter       string
	SigningTime     time.Time
	SignatureBytes  int
	MaxSourceBytes  int64
	MaxXrefChain    int
	MaxXrefEntries  int
}

// Bytes signs a PDF byte slice and returns a new signed PDF.
func Bytes(input []byte, options Options) ([]byte, error) {
	return BytesContext(context.Background(), input, options)
}

// BytesContext signs a PDF byte slice and checks ctx around parsing and
// signing. Cancellation during a crypto.Signer implementation depends on that
// signer returning.
func BytesContext(ctx context.Context, input []byte, options Options) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(input) == 0 {
		return nil, ErrMissingInput
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	prepared, err := prepareSigningOptions(options)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return signBytesPrepared(ctx, input, prepared, false)
}

// AppendBytes signs a PDF byte slice and may reuse input's backing array for
// the returned signed PDF. Callers must not use input after calling AppendBytes.
func AppendBytes(input []byte, options Options) ([]byte, error) {
	return AppendBytesContext(context.Background(), input, options)
}

// AppendBytesContext signs a PDF byte slice and checks ctx around parsing and
// signing. Cancellation during a crypto.Signer implementation depends on that
// signer returning.
func AppendBytesContext(ctx context.Context, input []byte, options Options) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(input) == 0 {
		return nil, ErrMissingInput
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	prepared, err := prepareSigningOptions(options)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return signBytesPrepared(ctx, input, prepared, true)
}

// File signs inputPath and writes the signed PDF to outputPath.
func File(inputPath, outputPath string, options Options) error {
	return FileContext(context.Background(), inputPath, outputPath, options)
}

// FileContext signs inputPath with bounded reads and context cancellation, then
// writes the signed PDF to outputPath.
func FileContext(ctx context.Context, inputPath, outputPath string, options Options) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if inputPath == "" {
		return ErrMissingInput
	}
	if outputPath == "" {
		return ErrMissingOutput
	}
	prepared, err := prepareSigningOptions(options)
	if err != nil {
		return err
	}
	input, err := readSigningFileContext(ctx, inputPath, prepared.MaxSourceBytes)
	if err != nil {
		return fmt.Errorf("read PDF: %w", err)
	}
	signed, err := signBytesPrepared(ctx, input, prepared, false)
	if err != nil {
		return err
	}
	if err := writeFilePrivate(outputPath, signed); err != nil {
		return fmt.Errorf("write signed PDF: %w", err)
	}
	return nil
}

func signBytesPrepared(ctx context.Context, input []byte, prepared preparedOptions, reuseInput bool) ([]byte, error) {
	if len(input) == 0 {
		return nil, ErrMissingInput
	}
	if int64(len(input)) > prepared.MaxSourceBytes {
		return nil, fmt.Errorf("pdfsigning: source PDF exceeds maximum size")
	}
	pdfCtx, err := analyzePDFContext(ctx, input, prepared.MaxXrefChain, prepared.MaxXrefEntries)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return signPDFContext(ctx, pdfCtx, prepared, reuseInput)
}

func readSigningFileContext(ctx context.Context, path string, maxBytes int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	file, err := os.Open(path) // #nosec G304 -- FileContext is an explicit caller-path API; reads are size-bounded.
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	if info, statErr := file.Stat(); statErr == nil && info.Mode().IsRegular() && info.Size() > maxBytes {
		return nil, errors.New("pdfsigning: source PDF exceeds maximum size")
	}
	data, err := io.ReadAll(io.LimitReader(signingContextReader{ctx: ctx, r: file}, maxBytes+1))
	if err == nil && int64(len(data)) > maxBytes {
		err = errors.New("pdfsigning: source PDF exceeds maximum size")
	}
	return data, err
}

type signingContextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r signingContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.r.Read(p)
	if err == nil {
		err = r.ctx.Err()
	}
	return n, err
}

func prepareSigningOptions(options Options) (preparedOptions, error) {
	if options.Signer == nil {
		return preparedOptions{}, ErrMissingSigner
	}
	if options.Certificate == nil {
		return preparedOptions{}, ErrMissingCertificate
	}
	if !publicKeysEqual(options.Signer.Public(), options.Certificate.PublicKey) {
		return preparedOptions{}, errors.New("pdfsigning: signer public key does not match certificate")
	}
	subFilter, err := normalizeSubFilter(options.SubFilter)
	if err != nil {
		return preparedOptions{}, err
	}
	digest, err := normalizeDigest(options.DigestAlgorithm)
	if err != nil {
		return preparedOptions{}, err
	}
	signatureBytes := options.SignatureSize
	if signatureBytes == 0 {
		signatureBytes = defaultSignatureBytes
	}
	if signatureBytes < 2048 || signatureBytes > maxSignatureBytes {
		return preparedOptions{}, fmt.Errorf("pdfsigning: SignatureSize must be between 2048 and %d bytes", maxSignatureBytes)
	}
	maxSourceBytes := options.MaxSourceBytes
	if maxSourceBytes == 0 {
		maxSourceBytes = DefaultMaxSourceBytes
	}
	if maxSourceBytes < 0 || maxSourceBytes > int64(math.MaxInt)-1 {
		return preparedOptions{}, fmt.Errorf("pdfsigning: invalid MaxSourceBytes %d", maxSourceBytes)
	}
	maxXrefChain := options.MaxXrefChainLength
	if maxXrefChain == 0 {
		maxXrefChain = DefaultMaxXrefChainLength
	}
	if maxXrefChain < 0 {
		return preparedOptions{}, fmt.Errorf("pdfsigning: invalid MaxXrefChainLength %d", maxXrefChain)
	}
	maxXrefEntries := options.MaxXrefEntries
	if maxXrefEntries == 0 {
		maxXrefEntries = DefaultMaxXrefEntries
	}
	if maxXrefEntries < 0 || maxXrefEntries > maxPDFObjectCount {
		return preparedOptions{}, fmt.Errorf("pdfsigning: invalid MaxXrefEntries %d", maxXrefEntries)
	}
	signingTime := options.SigningTime
	if signingTime.IsZero() {
		signingTime = time.Now().UTC()
	}
	options.SubFilter = subFilter
	options.DigestAlgorithm = digest
	options.SigningTime = signingTime
	options.SignatureSize = signatureBytes
	return preparedOptions{
		Options:         options,
		DigestAlgorithm: digest,
		SubFilter:       subFilter,
		SigningTime:     signingTime,
		SignatureBytes:  signatureBytes,
		MaxSourceBytes:  maxSourceBytes,
		MaxXrefChain:    maxXrefChain,
		MaxXrefEntries:  maxXrefEntries,
	}, nil
}

func normalizeSubFilter(subFilter string) (string, error) {
	switch subFilter {
	case "":
		return SubFilterETSI_CAdESDetached, nil
	case SubFilterETSI_CAdESDetached, SubFilterAdobePKCS7Detached:
		return subFilter, nil
	default:
		return "", fmt.Errorf("pdfsigning: unsupported SubFilter: %s", subFilter)
	}
}

func signPDFContext(ctx context.Context, pdfCtx pdfContext, options preparedOptions, reuseInput bool) ([]byte, error) {
	if err := signContextErr(ctx); err != nil {
		return nil, err
	}
	placeholderHex := make([]byte, options.SignatureBytes*2)
	for i := range placeholderHex {
		if i%1024 == 0 {
			if err := signContextErr(ctx); err != nil {
				return nil, err
			}
		}
		placeholderHex[i] = '0'
	}
	byteRangePlaceholder := byteRangePlaceholder()
	increment, err := buildIncrementContext(ctx, pdfCtx, options, byteRangePlaceholder, placeholderHex)
	if err != nil {
		return nil, err
	}
	var output []byte
	if reuseInput {
		output = append(pdfCtx.Data, increment.data...)
	} else {
		output = make([]byte, 0, len(pdfCtx.Data)+len(increment.data))
		output = append(output, pdfCtx.Data...)
		output = append(output, increment.data...)
	}
	if err := signContextErr(ctx); err != nil {
		return nil, err
	}
	incrementStart := len(pdfCtx.Data)
	contentsStart := incrementStart + increment.contentsStart
	contentsEnd := incrementStart + increment.contentsEnd
	byteRange := []int{0, contentsStart, contentsEnd, len(output) - contentsEnd}
	byteRangeValue := formatByteRange(byteRange)
	byteRangeOffset := incrementStart + increment.byteRangeOffset
	copy(output[byteRangeOffset:byteRangeOffset+len(byteRangeValue)], byteRangeValue)
	if err := signContextErr(ctx); err != nil {
		return nil, err
	}
	contentDigest := digestByteRange(output, contentsStart, contentsEnd, options.DigestAlgorithm)
	if err := signContextErr(ctx); err != nil {
		return nil, err
	}
	cms, err := createDetachedCMSWithPreparedDigest(contentDigest, options)
	if err != nil {
		return nil, err
	}
	if err := signContextErr(ctx); err != nil {
		return nil, err
	}
	cmsHex := make([]byte, hex.EncodedLen(len(cms)))
	hex.Encode(cmsHex, cms)
	if len(cmsHex) > len(placeholderHex) {
		return nil, fmt.Errorf("pdfsigning: CMS signature needs %d hex bytes, placeholder has %d", len(cmsHex), len(placeholderHex))
	}
	contentsOffset := contentsStart + 1
	copy(output[contentsOffset:contentsOffset+len(cmsHex)], cmsHex)
	return output, nil
}

func digestByteRange(output []byte, contentsStart, contentsEnd int, digest crypto.Hash) []byte {
	h := digest.New()
	h.Write(output[:contentsStart])
	h.Write(output[contentsEnd:])
	return h.Sum(nil)
}

type signingIncrement struct {
	data            []byte
	byteRangeOffset int
	contentsStart   int
	contentsEnd     int
}

type signatureDictionaryData struct {
	body            []byte
	byteRangeOffset int
	contentsStart   int
	contentsEnd     int
}

func buildIncrementContext(cancelCtx context.Context, ctx pdfContext, options preparedOptions, byteRangePlaceholder string, contentsPlaceholder []byte) (signingIncrement, error) {
	if err := signContextErr(cancelCtx); err != nil {
		return signingIncrement{}, err
	}
	rootOffset, err := ctx.Xref.objectOffsetContext(cancelCtx, ctx.Root)
	if err != nil {
		return signingIncrement{}, fmt.Errorf("read root object: %w", err)
	}
	rootDict, err := readObjectDictContext(cancelCtx, ctx.Data, ctx.Root, rootOffset)
	if err != nil {
		return signingIncrement{}, fmt.Errorf("read root object: %w", err)
	}
	pageOffset, err := ctx.Xref.objectOffsetContext(cancelCtx, ctx.Page)
	if err != nil {
		return signingIncrement{}, fmt.Errorf("read page object: %w", err)
	}
	pageDict, err := readObjectDictContext(cancelCtx, ctx.Data, ctx.Page, pageOffset)
	if err != nil {
		return signingIncrement{}, fmt.Errorf("read page object: %w", err)
	}
	if err := signContextErr(cancelCtx); err != nil {
		return signingIncrement{}, err
	}
	acroObject := ctx.Size
	fieldObject := ctx.Size + 1
	signatureObject := ctx.Size + 2
	rootEntries := []pdfDictEntry{{
		key:   "/AcroForm",
		value: fmt.Sprintf("%d 0 R", acroObject),
	}}
	if options.SubFilter == SubFilterETSI_CAdESDetached {
		rootEntries = append([]pdfDictEntry{{
			key:          "/Extensions",
			value:        "<< /ESIC << /BaseVersion /1.7 /ExtensionLevel 1 >> >>",
			skipExisting: true,
		}}, rootEntries...)
	}
	rootDict, err = addDictEntries(rootDict, rootEntries...)
	if err != nil {
		return signingIncrement{}, err
	}
	if err := signContextErr(cancelCtx); err != nil {
		return signingIncrement{}, err
	}
	pageDict, err = addAnnotation(pageDict, fmt.Sprintf("%d 0 R", fieldObject))
	if err != nil {
		return signingIncrement{}, err
	}
	fieldName := options.FieldName
	if fieldName == "" {
		fieldName = "Signature1"
	}
	var buf bytes.Buffer
	entries := make([]xrefEntry, 0, 5)
	addObjectBytes := func(ref pdfRef, body []byte) int {
		buf.WriteByte('\n')
		entries = append(entries, xrefEntry{Object: ref.Object, Generation: ref.Generation, Offset: len(ctx.Data) + buf.Len()})
		writeBufferInt(&buf, ref.Object)
		buf.WriteByte(' ')
		writeBufferInt(&buf, ref.Generation)
		buf.WriteString(" obj\n")
		bodyStart := buf.Len()
		buf.Write(body)
		buf.WriteString("\nendobj\n")
		return bodyStart
	}
	addObject := func(ref pdfRef, body string) {
		addObjectBytes(ref, []byte(body))
	}
	addObjectBytes(ctx.Root, rootDict)
	addObjectBytes(ctx.Page, pageDict)
	if err := signContextErr(cancelCtx); err != nil {
		return signingIncrement{}, err
	}
	addObject(pdfRef{Object: acroObject}, fmt.Sprintf("<< /Fields [%d 0 R] /SigFlags 3 >>", fieldObject))
	addObject(pdfRef{Object: fieldObject}, fmt.Sprintf("<< /Type /Annot /Subtype /Widget /FT /Sig /T %s /Rect [0 0 0 0] /F 132 /P %d %d R /V %d 0 R >>", pdfString(fieldName), ctx.Page.Object, ctx.Page.Generation, signatureObject))
	signatureDict := signatureDictionaryBytes(options, byteRangePlaceholder, contentsPlaceholder)
	signatureBodyStart := addObjectBytes(pdfRef{Object: signatureObject}, signatureDict.body)
	byteRangeOffset := signatureBodyStart + signatureDict.byteRangeOffset
	contentsStart := signatureBodyStart + signatureDict.contentsStart
	contentsEnd := signatureBodyStart + signatureDict.contentsEnd
	xrefOffset := len(ctx.Data) + buf.Len()
	writeXref(&buf, entries)
	if err := signContextErr(cancelCtx); err != nil {
		return signingIncrement{}, err
	}
	trailerExtras, err := preservedTrailerEntriesContext(cancelCtx, ctx.Trailer)
	if err != nil {
		return signingIncrement{}, err
	}
	buf.WriteString("trailer\n<< /Size ")
	writeBufferInt(&buf, ctx.Size+3)
	buf.WriteString(" /Root ")
	writeBufferInt(&buf, ctx.Root.Object)
	buf.WriteByte(' ')
	writeBufferInt(&buf, ctx.Root.Generation)
	buf.WriteString(" R")
	buf.WriteString(trailerExtras)
	buf.WriteString(" /Prev ")
	writeBufferInt(&buf, ctx.PreviousXref)
	buf.WriteString(" >>\nstartxref\n")
	writeBufferInt(&buf, xrefOffset)
	buf.WriteString("\n%%EOF\n")
	return signingIncrement{data: buf.Bytes(), byteRangeOffset: byteRangeOffset, contentsStart: contentsStart, contentsEnd: contentsEnd}, nil
}

func signatureDictionaryBytes(options preparedOptions, byteRangePlaceholder string, contentsPlaceholder []byte) signatureDictionaryData {
	buf := make([]byte, 0, len(contentsPlaceholder)+256)
	buf = append(buf, "<< /Type /Sig /Filter /Adobe.PPKLite /SubFilter /"...)
	buf = append(buf, options.SubFilter...)
	buf = append(buf, " /ByteRange "...)
	byteRangeOffset := len(buf)
	buf = append(buf, byteRangePlaceholder...)
	buf = append(buf, " /Contents <"...)
	contentsStart := len(buf) - 1
	buf = append(buf, contentsPlaceholder...)
	contentsEnd := len(buf) + 1
	buf = append(buf, "> /M "...)
	buf = append(buf, pdfString(pdfDate(options.SigningTime))...)
	if options.Name != "" {
		buf = append(buf, " /Name "...)
		buf = append(buf, pdfString(options.Name)...)
	}
	if options.Location != "" {
		buf = append(buf, " /Location "...)
		buf = append(buf, pdfString(options.Location)...)
	}
	if options.Reason != "" {
		buf = append(buf, " /Reason "...)
		buf = append(buf, pdfString(options.Reason)...)
	}
	if options.ContactInfo != "" {
		buf = append(buf, " /ContactInfo "...)
		buf = append(buf, pdfString(options.ContactInfo)...)
	}
	buf = append(buf, " >>"...)
	return signatureDictionaryData{body: buf, byteRangeOffset: byteRangeOffset, contentsStart: contentsStart, contentsEnd: contentsEnd}
}

func writeXref(buf *bytes.Buffer, entries []xrefEntry) {
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Object < entries[j].Object
	})
	buf.WriteString("xref\n")
	for i := 0; i < len(entries); {
		first := entries[i].Object
		j := i + 1
		for j < len(entries) && entries[j].Object == entries[j-1].Object+1 {
			j++
		}
		writeBufferInt(buf, first)
		buf.WriteByte(' ')
		writeBufferInt(buf, j-i)
		buf.WriteByte('\n')
		for _, entry := range entries[i:j] {
			writeBufferPaddedInt(buf, entry.Offset, 10)
			buf.WriteByte(' ')
			writeBufferPaddedInt(buf, entry.Generation, 5)
			buf.WriteString(" n \n")
		}
		i = j
	}
}

func writeBufferInt(buf *bytes.Buffer, value int) {
	var scratch [20]byte
	buf.Write(strconv.AppendInt(scratch[:0], int64(value), 10))
}

func writeBufferPaddedInt(buf *bytes.Buffer, value, width int) {
	var scratch [20]byte
	raw := strconv.AppendInt(scratch[:0], int64(value), 10)
	for padding := width - len(raw); padding > 0; padding-- {
		buf.WriteByte('0')
	}
	buf.Write(raw)
}

func byteRangePlaceholder() string {
	return fmt.Sprintf("[%0*d %0*d %0*d %0*d]", byteRangeWidth, 0, byteRangeWidth, 0, byteRangeWidth, 0, byteRangeWidth, 0)
}

func formatByteRange(values []int) string {
	return fmt.Sprintf("[%0*d %0*d %0*d %0*d]", byteRangeWidth, values[0], byteRangeWidth, values[1], byteRangeWidth, values[2], byteRangeWidth, values[3])
}

func writeFilePrivate(outputPath string, data []byte) error {
	dir := filepath.Dir(outputPath)
	tmp, err := os.CreateTemp(dir, ".pdfsigning-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, outputPath); err != nil {
		return err
	}
	return syncOutputDirectory(dir)
}

func pdfString(value string) string {
	var buf strings.Builder
	buf.WriteByte('(')
	for _, r := range value {
		switch r {
		case '\\', '(', ')':
			buf.WriteByte('\\')
			buf.WriteRune(r)
		case '\n':
			buf.WriteString(`\n`)
		case '\r':
			buf.WriteString(`\r`)
		case '\t':
			buf.WriteString(`\t`)
		default:
			if r < 32 || r > 126 {
				buf.WriteByte('?')
			} else {
				buf.WriteRune(r)
			}
		}
	}
	buf.WriteByte(')')
	return buf.String()
}

func pdfDate(t time.Time) string {
	return t.UTC().Format("D:20060102150405+00'00'")
}

func normalizeDigest(digest crypto.Hash) (crypto.Hash, error) {
	if digest == 0 {
		digest = crypto.SHA256
	}
	switch digest {
	case crypto.SHA1:
		return 0, fmt.Errorf("pdfsigning: insecure digest algorithm %v", digest)
	case crypto.SHA256, crypto.SHA384, crypto.SHA512:
		if !digest.Available() {
			return 0, fmt.Errorf("pdfsigning: digest algorithm %v is not available", digest)
		}
		return digest, nil
	default:
		return 0, fmt.Errorf("pdfsigning: unsupported digest algorithm %v", digest)
	}
}
