// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package sign

import (
	"bytes"
	"crypto"
	_ "crypto/sha256" // Register SHA-256 algorithms with crypto.Hash.
	_ "crypto/sha512" // Register SHA-512 algorithms with crypto.Hash.
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	defaultSignatureBytes = 16384
	maxSignatureBytes     = 1 << 20
	byteRangeWidth        = 20
	refPatternFormat      = `%s\s+(\d+)\s+(\d+)\s+R`
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
	// FieldName is the PDF signature field name. A zero value uses "Signature1".
	FieldName string
	// SigningTime sets the signature timestamp. A zero value uses now.
	SigningTime time.Time
	// SignatureSize is the reserved CMS signature size in bytes.
	SignatureSize int
}

// Bytes signs a PDF byte slice and returns a new signed PDF.
func Bytes(input []byte, options Options) ([]byte, error) {
	if len(input) == 0 {
		return nil, ErrMissingInput
	}
	if err := options.validate(); err != nil {
		return nil, err
	}
	ctx, err := analyzePDF(input)
	if err != nil {
		return nil, err
	}
	return signPDFContext(ctx, options)
}

// File signs inputPath and writes the signed PDF to outputPath.
func File(inputPath, outputPath string, options Options) error {
	if inputPath == "" {
		return ErrMissingInput
	}
	if outputPath == "" {
		return ErrMissingOutput
	}
	input, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("read PDF: %w", err)
	}
	signed, err := Bytes(input, options)
	if err != nil {
		return err
	}
	if err := writeFilePrivate(outputPath, signed); err != nil {
		return fmt.Errorf("write signed PDF: %w", err)
	}
	return nil
}

func (options Options) validate() error {
	if options.Signer == nil {
		return ErrMissingSigner
	}
	if options.Certificate == nil {
		return ErrMissingCertificate
	}
	if !publicKeysEqual(options.Signer.Public(), options.Certificate.PublicKey) {
		return errors.New("pdfsigning: signer public key does not match certificate")
	}
	_, err := normalizeDigest(options.DigestAlgorithm)
	return err
}

func signPDFContext(ctx pdfContext, options Options) ([]byte, error) {
	signatureBytes := options.SignatureSize
	if signatureBytes == 0 {
		signatureBytes = defaultSignatureBytes
	}
	if signatureBytes < 2048 || signatureBytes > maxSignatureBytes {
		return nil, fmt.Errorf("pdfsigning: SignatureSize must be between 2048 and %d bytes", maxSignatureBytes)
	}
	signingTime := options.SigningTime
	if signingTime.IsZero() {
		signingTime = time.Now().UTC()
	}
	options.SigningTime = signingTime
	placeholderHex := strings.Repeat("0", signatureBytes*2)
	byteRangePlaceholder := byteRangePlaceholder()
	increment, err := buildIncrement(ctx, options, byteRangePlaceholder, placeholderHex)
	if err != nil {
		return nil, err
	}
	output := make([]byte, 0, len(ctx.Data)+len(increment))
	output = append(output, ctx.Data...)
	output = append(output, increment...)
	incrementStart := len(ctx.Data)
	contentsStart, contentsEnd, err := findContentsRange(output[incrementStart:], placeholderHex)
	if err != nil {
		return nil, err
	}
	contentsStart += incrementStart
	contentsEnd += incrementStart
	byteRange := []int{0, contentsStart, contentsEnd, len(output) - contentsEnd}
	byteRangeValue := formatByteRange(byteRange)
	byteRangeOffset := bytes.Index(output[incrementStart:], []byte(byteRangePlaceholder))
	if byteRangeOffset < 0 {
		return nil, errors.New("pdfsigning: ByteRange placeholder not found")
	}
	byteRangeOffset += incrementStart
	copy(output[byteRangeOffset:byteRangeOffset+len(byteRangeValue)], byteRangeValue)
	signedContent := make([]byte, 0, byteRange[1]+byteRange[3])
	signedContent = append(signedContent, output[:contentsStart]...)
	signedContent = append(signedContent, output[contentsEnd:]...)
	cms, err := CreatePKCS7(signedContent, PKCS7Options{Signer: options.Signer, Certificate: options.Certificate, CertificateChain: options.CertificateChain, DigestAlgorithm: options.DigestAlgorithm, Detached: true, SigningTime: signingTime})
	if err != nil {
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

func buildIncrement(ctx pdfContext, options Options, byteRangePlaceholder, contentsPlaceholder string) ([]byte, error) {
	rootDict, err := readObjectDict(ctx.Data, ctx.Root, ctx.ObjectOffsets[ctx.Root.Object])
	if err != nil {
		return nil, fmt.Errorf("read root object: %w", err)
	}
	pageDict, err := readObjectDict(ctx.Data, ctx.Page, ctx.ObjectOffsets[ctx.Page.Object])
	if err != nil {
		return nil, fmt.Errorf("read page object: %w", err)
	}
	acroObject := ctx.Size
	fieldObject := ctx.Size + 1
	signatureObject := ctx.Size + 2
	rootDict, err = addDictEntry(rootDict, "/AcroForm", fmt.Sprintf("%d 0 R", acroObject))
	if err != nil {
		return nil, err
	}
	pageDict, err = addAnnotation(pageDict, fmt.Sprintf("%d 0 R", fieldObject))
	if err != nil {
		return nil, err
	}
	fieldName := options.FieldName
	if fieldName == "" {
		fieldName = "Signature1"
	}
	signingTime := options.SigningTime
	if signingTime.IsZero() {
		signingTime = time.Now().UTC()
	}
	var buf bytes.Buffer
	entries := make([]xrefEntry, 0, 5)
	addObject := func(ref pdfRef, body string) {
		buf.WriteByte('\n')
		entries = append(entries, xrefEntry{Object: ref.Object, Generation: ref.Generation, Offset: len(ctx.Data) + buf.Len()})
		fmt.Fprintf(&buf, "%d %d obj\n%s\nendobj\n", ref.Object, ref.Generation, body)
	}
	addObject(ctx.Root, string(rootDict))
	addObject(ctx.Page, string(pageDict))
	addObject(pdfRef{Object: acroObject}, fmt.Sprintf("<< /Fields [%d 0 R] /SigFlags 3 >>", fieldObject))
	addObject(pdfRef{Object: fieldObject}, fmt.Sprintf("<< /Type /Annot /Subtype /Widget /FT /Sig /T %s /Rect [0 0 0 0] /F 132 /P %d %d R /V %d 0 R >>", pdfString(fieldName), ctx.Page.Object, ctx.Page.Generation, signatureObject))
	addObject(pdfRef{Object: signatureObject}, signatureDictionary(options, byteRangePlaceholder, contentsPlaceholder, signingTime))
	xrefOffset := len(ctx.Data) + buf.Len()
	writeXref(&buf, entries)
	trailerExtras, err := preservedTrailerEntries(ctx.Trailer)
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root %d %d R%s /Prev %d >>\nstartxref\n%d\n%%%%EOF\n", ctx.Size+3, ctx.Root.Object, ctx.Root.Generation, trailerExtras, ctx.PreviousXref, xrefOffset)
	return buf.Bytes(), nil
}

func signatureDictionary(options Options, byteRangePlaceholder, contentsPlaceholder string, signingTime time.Time) string {
	fields := []string{"<< /Type /Sig", "/Filter /Adobe.PPKLite", "/SubFilter /adbe.pkcs7.detached", "/ByteRange " + byteRangePlaceholder, "/Contents <" + contentsPlaceholder + ">", "/M " + pdfString(pdfDate(signingTime))}
	if options.Name != "" {
		fields = append(fields, "/Name "+pdfString(options.Name))
	}
	if options.Location != "" {
		fields = append(fields, "/Location "+pdfString(options.Location))
	}
	if options.Reason != "" {
		fields = append(fields, "/Reason "+pdfString(options.Reason))
	}
	if options.ContactInfo != "" {
		fields = append(fields, "/ContactInfo "+pdfString(options.ContactInfo))
	}
	fields = append(fields, ">>")
	return strings.Join(fields, " ")
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
		fmt.Fprintf(buf, "%d %d\n", first, j-i)
		for _, entry := range entries[i:j] {
			fmt.Fprintf(buf, "%010d %05d n \n", entry.Offset, entry.Generation)
		}
		i = j
	}
}

func findContentsRange(input []byte, placeholderHex string) (int, int, error) {
	needle := []byte("/Contents <" + placeholderHex + ">")
	idx := bytes.Index(input, needle)
	if idx < 0 {
		return 0, 0, errors.New("pdfsigning: signature contents placeholder not found")
	}
	start := idx + len("/Contents ")
	end := start + 1 + len(placeholderHex) + 1
	return start, end, nil
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
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, outputPath)
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
