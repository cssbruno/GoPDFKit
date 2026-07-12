// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package pdfcdr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/cssbruno/gopdfkit/importpdf"
)

// ErrInvalidSource reports that the input is not a supported PDF source.
var ErrInvalidSource = errors.New("pdf cdr source is invalid")

// ErrNoPages reports that the source contains no pages that can be
// reconstructed.
var ErrNoPages = errors.New("pdf cdr source has no pages")

// ErrMaxPagesExceeded reports that the configured reconstruction page limit
// was exceeded.
var ErrMaxPagesExceeded = errors.New("pdf cdr page limit exceeded")

// Options controls parser and reconstruction limits. Zero values use the
// importpdf package defaults, except MaxPages, which defaults to the parser's
// maximum page count.
type Options struct {
	MaxSourceBytes       int64
	MaxReferencedObjects int
	MaxPages             int
}

func (o Options) importOptions() (importpdf.ImportOptions, error) {
	if o.MaxPages < 0 {
		return importpdf.ImportOptions{}, fmt.Errorf("pdf cdr max pages is invalid: %d", o.MaxPages)
	}
	return importpdf.ImportOptions{
		MaxSourceBytes:       o.MaxSourceBytes,
		MaxReferencedObjects: o.MaxReferencedObjects,
	}, nil
}

// Sanitize reconstructs a PDF from source. source may be a file path string,
// []byte, io.Reader, or *importpdf.Source. A []byte source must not be
// modified concurrently while sanitization is in progress.
func Sanitize(source any) ([]byte, error) {
	return SanitizeContext(context.Background(), source, Options{})
}

// SanitizeContext reconstructs a PDF from source while honoring ctx during
// bounded parsing, page processing, and output assembly.
func SanitizeContext(ctx context.Context, source any, options Options) ([]byte, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	importOptions, err := options.importOptions()
	if err != nil {
		return nil, err
	}
	src, err := openSourceContext(ctx, source, importOptions)
	if err != nil {
		if ctxErr := contextError(ctx); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, fmt.Errorf("%w: %w", ErrInvalidSource, err)
	}
	if src.PageCount() == 0 {
		return nil, ErrNoPages
	}
	if options.MaxPages > 0 && src.PageCount() > options.MaxPages {
		return nil, fmt.Errorf("%w: page count %d exceeds maximum %d", ErrMaxPagesExceeded, src.PageCount(), options.MaxPages)
	}
	return reconstruct(ctx, src)
}

func openSourceContext(ctx context.Context, source any, options importpdf.ImportOptions) (*importpdf.Source, error) {
	if data, ok := source.([]byte); ok {
		return importpdf.OpenBytesImmutableWithOptionsContext(ctx, data, options)
	}
	return importpdf.OpenWithOptionsContext(ctx, source, options)
}

// SanitizeFile reads inputPath and atomically writes a reconstructed PDF to
// outputPath. The output file is created with mode 0600 before the final
// rename.
func SanitizeFile(inputPath, outputPath string) error {
	return SanitizeFileContext(context.Background(), inputPath, outputPath, Options{})
}

// SanitizeFileContext is the context-aware form of SanitizeFile.
func SanitizeFileContext(ctx context.Context, inputPath, outputPath string, options Options) error {
	if strings.TrimSpace(outputPath) == "" {
		return errors.New("pdf cdr output path is empty")
	}
	data, err := SanitizeContext(ctx, inputPath, options)
	if err != nil {
		return err
	}
	dir := filepath.Dir(outputPath)
	tmp, err := os.CreateTemp(dir, ".pdfcdr-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return err
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, outputPath); err != nil {
		return err
	}
	return nil
}

type pdfObject struct {
	body []byte
}

type pdfBuilder struct {
	objects []pdfObject
}

func (b *pdfBuilder) reserve() int {
	b.objects = append(b.objects, pdfObject{})
	return len(b.objects)
}

func (b *pdfBuilder) set(id int, body []byte) {
	b.objects[id-1].body = body
}

func reconstruct(ctx context.Context, source *importpdf.Source) ([]byte, error) {
	builder := &pdfBuilder{}
	catalogID := builder.reserve()
	pagesID := builder.reserve()

	pageIDs := make([]int, 0, source.PageCount())
	for pageNumber := 1; pageNumber <= source.PageCount(); pageNumber++ {
		if err := contextError(ctx); err != nil {
			return nil, err
		}
		page, err := source.PageContext(ctx, pageNumber, "MediaBox")
		if err != nil {
			return nil, fmt.Errorf("reconstruct page %d: %w", pageNumber, err)
		}
		width := page.WidthPoints()
		height := page.HeightPoints()
		if width <= 0 || height <= 0 || !finite(width) || !finite(height) {
			return nil, fmt.Errorf("reconstruct page %d: invalid page size", pageNumber)
		}
		content, err := page.ContentBorrowedWithContext(ctx)
		if err != nil {
			return nil, fmt.Errorf("reconstruct page %d content: %w", pageNumber, err)
		}
		objectBodies := make(map[importpdf.ObjRef][]byte, page.ObjectCount())
		if err := page.ForEachObjectBorrowed(func(ref importpdf.ObjRef, body []byte) error {
			objectBodies[ref] = body
			return nil
		}); err != nil {
			return nil, fmt.Errorf("reconstruct page %d objects: %w", pageNumber, err)
		}

		resources, err := sanitizePDFObject(page.ResourcesBorrowed())
		if err != nil {
			return nil, fmt.Errorf("reconstruct page %d resources: %w", pageNumber, err)
		}
		reachable, err := reachableObjects(resources, objectBodies)
		if err != nil {
			return nil, fmt.Errorf("reconstruct page %d resources: %w", pageNumber, err)
		}

		pageID := builder.reserve()
		contentID := builder.reserve()
		pageIDs = append(pageIDs, pageID)

		refMap := make(map[importpdf.ObjRef]int, len(reachable))
		for _, object := range reachable {
			refMap[object.ref] = builder.reserve()
		}
		var resourceID int
		if root, ok := singleIndirectRefKey(resources); ok {
			resourceRef, exists := resourceObjectRef(root, objectBodies)
			if !exists {
				return nil, fmt.Errorf("reconstruct page %d resources: referenced object %d %d R is missing", pageNumber, root.objectNumber, root.generation)
			}
			resourceID = refMap[resourceRef]
			for _, object := range reachable {
				if object.ref == resourceRef && bytes.Equal(bytes.TrimSpace(object.body), []byte("null")) {
					resourceID = builder.reserve()
					builder.set(resourceID, []byte("<<>>"))
					break
				}
			}
		} else {
			resourceID = builder.reserve()
			resources = importpdf.RewriteIndirectRefs(resources, refMap)
			builder.set(resourceID, resources)
		}
		builder.set(contentID, pdfStreamBody(content))
		builder.set(pageID, []byte(fmt.Sprintf(
			"<< /Type /Page /Parent %d 0 R /MediaBox [0 0 %s %s] /Resources %d 0 R /Contents %d 0 R >>",
			pagesID, pdfNumber(width), pdfNumber(height), resourceID, contentID,
		)))

		for _, object := range reachable {
			if err := contextError(ctx); err != nil {
				return nil, err
			}
			body := importpdf.RewriteIndirectRefs(object.body, refMap)
			builder.set(refMap[object.ref], body)
		}
	}

	var kids strings.Builder
	kids.Grow(len(pageIDs) * 12)
	for _, pageID := range pageIDs {
		kids.WriteString(strconv.Itoa(pageID))
		kids.WriteString(" 0 R ")
	}
	builder.set(pagesID, []byte(fmt.Sprintf(
		"<< /Type /Pages /Kids [%s] /Count %d >>", kids.String(), len(pageIDs),
	)))
	builder.set(catalogID, []byte(fmt.Sprintf("<< /Type /Catalog /Pages %d 0 R >>", pagesID)))
	return builder.bytes(ctx, catalogID)
}

type reachableObject struct {
	ref  importpdf.ObjRef
	body []byte
}

func reachableObjects(resources []byte, objects map[importpdf.ObjRef][]byte) ([]reachableObject, error) {
	byKey := make(map[refKey]importpdf.ObjRef, len(objects))
	for ref := range objects {
		byKey[refKey{objectNumber: ref.ObjectNumber(), generation: ref.Generation()}] = ref
	}
	queue := indirectRefKeys(resources)
	seen := make(map[importpdf.ObjRef]bool, len(queue))
	result := make([]reachableObject, 0, len(objects))
	for len(queue) > 0 {
		key := queue[0]
		queue = queue[1:]
		ref, ok := byKey[key]
		if !ok {
			return nil, fmt.Errorf("referenced object %d %d R is missing", key.objectNumber, key.generation)
		}
		if seen[ref] {
			continue
		}
		seen[ref] = true
		body, ok := objects[ref]
		if !ok {
			return nil, fmt.Errorf("referenced object %s is missing", ref)
		}
		sanitized, err := sanitizePDFObject(body)
		if err != nil {
			return nil, fmt.Errorf("object %s: %w", ref, err)
		}
		result = append(result, reachableObject{ref: ref, body: sanitized})
		for _, child := range indirectRefKeys(sanitized) {
			if !seen[byKey[child]] {
				queue = append(queue, child)
			}
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ref.ObjectNumber() == result[j].ref.ObjectNumber() {
			return result[i].ref.Generation() < result[j].ref.Generation()
		}
		return result[i].ref.ObjectNumber() < result[j].ref.ObjectNumber()
	})
	return result, nil
}

func resourceObjectRef(key refKey, objects map[importpdf.ObjRef][]byte) (importpdf.ObjRef, bool) {
	for ref := range objects {
		if ref.ObjectNumber() == key.objectNumber && ref.Generation() == key.generation {
			return ref, true
		}
	}
	return importpdf.ObjRef{}, false
}

func (b *pdfBuilder) bytes(ctx context.Context, rootID int) ([]byte, error) {
	var out bytes.Buffer
	estimated := len("%PDF-1.7\n%\xE2\xE3\xCF\xD3\n") + 128 + len(b.objects)*20
	for id, object := range b.objects {
		estimated += len(object.body) + len(strconv.Itoa(id+1)) + len(" 0 obj\nendobj\n")
	}
	out.Grow(estimated)
	out.WriteString("%PDF-1.7\n%\xE2\xE3\xCF\xD3\n")
	offsets := make([]int, len(b.objects)+1)
	for id, object := range b.objects {
		if err := contextError(ctx); err != nil {
			return nil, err
		}
		if object.body == nil {
			return nil, fmt.Errorf("PDF object %d has no body", id+1)
		}
		offsets[id+1] = out.Len()
		fmt.Fprintf(&out, "%d 0 obj\n", id+1)
		out.Write(object.body)
		if len(object.body) == 0 || object.body[len(object.body)-1] != '\n' {
			out.WriteByte('\n')
		}
		out.WriteString("endobj\n")
	}
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n", len(b.objects)+1)
	out.WriteString("0000000000 65535 f \n")
	for id := 1; id < len(offsets); id++ {
		fmt.Fprintf(&out, "%010d 00000 n \n", offsets[id])
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root %d 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(b.objects)+1, rootID, xref)
	return out.Bytes(), nil
}

func pdfStreamBody(content []byte) []byte {
	var out bytes.Buffer
	out.Grow(len(content) + 32)
	fmt.Fprintf(&out, "<< /Length %d >>\nstream\n", len(content))
	out.Write(content)
	if len(content) == 0 || content[len(content)-1] != '\n' {
		out.WriteByte('\n')
	}
	out.WriteString("endstream")
	return out.Bytes()
}

func pdfNumber(value float64) string {
	return strconv.FormatFloat(value, 'f', 5, 64)
}

func finite(value float64) bool {
	return value == value && value < 1.7976931348623157e+308 && value > -1.7976931348623157e+308
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}
