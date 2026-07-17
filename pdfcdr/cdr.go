// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package pdfcdr

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/cssbruno/gopdfkit/importpdf"
)

const (
	// DefaultMaxDecodedBytes limits aggregate decoded page content retained by
	// one reconstruction operation.
	DefaultMaxDecodedBytes int64 = 256 * 1024 * 1024
	// DefaultMaxResourceBytes limits aggregate sanitized resource object bytes.
	DefaultMaxResourceBytes int64 = 256 * 1024 * 1024
	// DefaultMaxOutputBytes limits the complete reconstructed PDF.
	DefaultMaxOutputBytes int64 = 512 * 1024 * 1024
	// DefaultMaxObjects limits all objects in the reconstructed PDF.
	DefaultMaxObjects = 100000
)

// ErrInvalidSource reports that the input is not a supported PDF source.
var ErrInvalidSource = errors.New("pdf cdr source is invalid")

// ErrNoPages reports that the source contains no pages that can be
// reconstructed.
var ErrNoPages = errors.New("pdf cdr source has no pages")

// ErrMaxPagesExceeded reports that the configured reconstruction page limit
// was exceeded.
var ErrMaxPagesExceeded = errors.New("pdf cdr page limit exceeded")

// ErrLimitExceeded reports that an aggregate reconstruction limit was
// exceeded.
var ErrLimitExceeded = errors.New("pdf cdr reconstruction limit exceeded")

// Options controls parser and reconstruction limits. Zero values use package
// defaults; parser fields use the importpdf defaults.
type Options struct {
	MaxSourceBytes       int64
	MaxReferencedObjects int
	MaxPages             int
	MaxDecodedBytes      int64
	MaxResourceBytes     int64
	MaxOutputBytes       int64
	MaxObjects           int
}

func (o Options) normalized() (Options, error) {
	if o.MaxPages == 0 {
		o.MaxPages = importpdf.MaxPages
	}
	if o.MaxDecodedBytes == 0 {
		o.MaxDecodedBytes = DefaultMaxDecodedBytes
	}
	if o.MaxResourceBytes == 0 {
		o.MaxResourceBytes = DefaultMaxResourceBytes
	}
	if o.MaxOutputBytes == 0 {
		o.MaxOutputBytes = DefaultMaxOutputBytes
	}
	if o.MaxObjects == 0 {
		o.MaxObjects = DefaultMaxObjects
	}
	if o.MaxPages < 0 || o.MaxDecodedBytes < 0 || o.MaxResourceBytes < 0 || o.MaxOutputBytes < 0 || o.MaxObjects < 0 {
		return Options{}, errors.New("pdf cdr reconstruction limits must not be negative")
	}
	maxInt := int64(^uint(0) >> 1)
	if o.MaxDecodedBytes > maxInt || o.MaxResourceBytes > maxInt || o.MaxOutputBytes > maxInt {
		return Options{}, errors.New("pdf cdr reconstruction byte limit is too large for this platform")
	}
	if o.MaxOutputBytes > 9999999999 {
		return Options{}, errors.New("pdf cdr output limit exceeds classic xref capacity")
	}
	return o, nil
}

func (o Options) importOptions() importpdf.ImportOptions {
	return importpdf.ImportOptions{
		MaxSourceBytes:       o.MaxSourceBytes,
		MaxReferencedObjects: o.MaxReferencedObjects,
	}
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
	options, err := options.normalized()
	if err != nil {
		return nil, err
	}
	src, err := openSourceContext(ctx, source, options.importOptions())
	if err != nil {
		if ctxErr := contextError(ctx); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, fmt.Errorf("%w: %w", ErrInvalidSource, err)
	}
	if src.PageCount() == 0 {
		return nil, ErrNoPages
	}
	if src.PageCount() > options.MaxPages {
		return nil, fmt.Errorf("%w: page count %d exceeds maximum %d", ErrMaxPagesExceeded, src.PageCount(), options.MaxPages)
	}
	return reconstruct(ctx, src, options)
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
	if err := tmp.Sync(); err != nil {
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

type pdfObject struct {
	body []byte
}

type pdfBuilder struct {
	objects        []pdfObject
	maxObjects     int
	maxOutputBytes int64
	bodyBytes      int64
	err            error
}

func (b *pdfBuilder) reserve() int {
	if b.err != nil {
		return 0
	}
	if b.maxObjects > 0 && len(b.objects) >= b.maxObjects {
		b.err = fmt.Errorf("%w: object count exceeds maximum %d", ErrLimitExceeded, b.maxObjects)
		return 0
	}
	b.objects = append(b.objects, pdfObject{})
	return len(b.objects)
}

func (b *pdfBuilder) set(id int, body []byte) {
	if b.err != nil {
		return
	}
	if id <= 0 || id > len(b.objects) {
		b.err = errors.New("PDF builder object ID is invalid")
		return
	}
	previous := int64(len(b.objects[id-1].body))
	next := b.bodyBytes - previous + int64(len(body))
	if next < 0 || (b.maxOutputBytes > 0 && next > b.maxOutputBytes) {
		b.err = fmt.Errorf("%w: output bytes exceed maximum %d", ErrLimitExceeded, b.maxOutputBytes)
		return
	}
	b.bodyBytes = next
	b.objects[id-1].body = body
}

type reconstructionBudget struct {
	maxDecodedBytes  int64
	maxResourceBytes int64
	decodedBytes     int64
	resourceBytes    int64
}

func (b *reconstructionBudget) addDecoded(size int) error {
	return addReconstructionBytes(&b.decodedBytes, b.maxDecodedBytes, size, "decoded page content")
}

func (b *reconstructionBudget) addResource(size int) error {
	return addReconstructionBytes(&b.resourceBytes, b.maxResourceBytes, size, "resource data")
}

func addReconstructionBytes(current *int64, maximum int64, size int, label string) error {
	if size < 0 || int64(size) > maximum-*current {
		return fmt.Errorf("%w: %s exceeds maximum %d", ErrLimitExceeded, label, maximum)
	}
	*current += int64(size)
	return nil
}

func reconstruct(ctx context.Context, source *importpdf.Source, options Options) ([]byte, error) {
	builder := &pdfBuilder{maxObjects: options.MaxObjects, maxOutputBytes: options.MaxOutputBytes}
	reserve := func() (int, error) {
		id := builder.reserve()
		return id, builder.err
	}
	set := func(id int, body []byte) error {
		builder.set(id, body)
		return builder.err
	}
	budget := reconstructionBudget{
		maxDecodedBytes:  options.MaxDecodedBytes,
		maxResourceBytes: options.MaxResourceBytes,
	}
	catalogID, err := reserve()
	if err != nil {
		return nil, err
	}
	pagesID, err := reserve()
	if err != nil {
		return nil, err
	}
	type retainedResourceObject struct {
		body         []byte
		id           int
		preserveKeys bool
	}
	type retainedResourceValue struct {
		digest       [sha256.Size]byte
		length       int
		preserveKeys bool
	}
	type directResourceObject struct {
		body []byte
		id   int
	}
	retainedResources := make(map[importpdf.ObjRef]*retainedResourceObject)
	retainedResourceValues := make(map[retainedResourceValue][]*retainedResourceObject)
	directResourceIDs := make(map[retainedResourceValue][]directResourceObject)
	retainDirectResource := func(body []byte) (int, error) {
		value := retainedResourceValue{digest: sha256.Sum256(body), length: len(body)}
		for _, retained := range directResourceIDs[value] {
			if bytes.Equal(retained.body, body) {
				return retained.id, nil
			}
		}
		if err := budget.addResource(len(body)); err != nil {
			return 0, err
		}
		id, err := reserve()
		if err != nil {
			return 0, err
		}
		if err := set(id, body); err != nil {
			return 0, err
		}
		directResourceIDs[value] = append(directResourceIDs[value], directResourceObject{body: body, id: id})
		return id, nil
	}

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
		content, err = standalonePageContent(content)
		if err != nil {
			return nil, fmt.Errorf("reconstruct page %d content: %w", pageNumber, err)
		}
		if err := budget.addDecoded(len(content)); err != nil {
			return nil, fmt.Errorf("reconstruct page %d: %w", pageNumber, err)
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
		for _, object := range reachable {
			retained, ok := retainedResources[object.ref]
			if !ok {
				continue
			}
			if retained.preserveKeys != object.preserveKeys {
				return nil, fmt.Errorf(
					"reconstruct page %d resources: %w",
					pageNumber,
					ambiguousResourceReferenceError(refKey{
						objectNumber: object.ref.ObjectNumber(),
						generation:   object.ref.Generation(),
					}),
				)
			}
			if !bytes.Equal(retained.body, object.body) {
				return nil, fmt.Errorf("reconstruct page %d resources: object %s changed between page graphs", pageNumber, object.ref)
			}
		}

		var resourceRef importpdf.ObjRef
		var indirectResources, nullResourceRoot bool
		if root, ok := singleIndirectRefKey(resources); ok {
			indirectResources = true
			resourceRef, ok = resourceObjectRef(root, objectBodies)
			if !ok {
				return nil, fmt.Errorf("reconstruct page %d resources: referenced object %d %d R is missing", pageNumber, root.objectNumber, root.generation)
			}
			for _, object := range reachable {
				if object.ref == resourceRef {
					nullResourceRoot = bytes.Equal(bytes.TrimSpace(object.body), []byte("null"))
					break
				}
			}
		}

		pageID, err := reserve()
		if err != nil {
			return nil, err
		}
		contentID, err := reserve()
		if err != nil {
			return nil, err
		}
		pageIDs = append(pageIDs, pageID)

		refMap := make(map[importpdf.ObjRef]int, len(reachable))
		newObjects := make([]reachableObject, 0, len(reachable))
		for _, object := range reachable {
			retained, ok := retainedResources[object.ref]
			if !ok {
				value := retainedResourceValue{
					digest:       sha256.Sum256(object.body),
					length:       len(object.body),
					preserveKeys: object.preserveKeys,
				}
				for _, candidate := range retainedResourceValues[value] {
					if bytes.Equal(candidate.body, object.body) {
						retained = candidate
						break
					}
				}
				if retained == nil {
					retained = &retainedResourceObject{
						body:         object.body,
						preserveKeys: object.preserveKeys,
					}
					retainedResourceValues[value] = append(retainedResourceValues[value], retained)
				}
				retainedResources[object.ref] = retained
			}
			// A null indirect /Resources value is replaced by one shared empty
			// dictionary. Do not retain the now-unreachable null object unless
			// another resource graph actually references it.
			if retained.id == 0 && (!nullResourceRoot || object.ref != resourceRef) {
				retained.id, err = reserve()
				if err != nil {
					return nil, err
				}
				newObjects = append(newObjects, object)
			}
			if retained.id != 0 {
				refMap[object.ref] = retained.id
			}
		}
		var resourceID int
		if indirectResources {
			if nullResourceRoot {
				resourceID, err = retainDirectResource([]byte("<< >>"))
			} else {
				resourceID = refMap[resourceRef]
				if resourceID == 0 {
					return nil, fmt.Errorf("reconstruct page %d resources: referenced object %s was not retained", pageNumber, resourceRef)
				}
			}
		} else {
			resources = importpdf.RewriteIndirectRefs(resources, refMap)
			resourceID, err = retainDirectResource(resources)
		}
		if err != nil {
			return nil, fmt.Errorf("reconstruct page %d resources: %w", pageNumber, err)
		}
		if err := set(contentID, pdfStreamBody(content)); err != nil {
			return nil, err
		}
		if err := set(pageID, []byte(fmt.Sprintf(
			"<< /Type /Page /Parent %d 0 R /MediaBox [0 0 %s %s] /Resources %d 0 R /Contents %d 0 R >>",
			pagesID, pdfNumber(width), pdfNumber(height), resourceID, contentID,
		))); err != nil {
			return nil, err
		}

		for _, object := range newObjects {
			if err := contextError(ctx); err != nil {
				return nil, err
			}
			body := importpdf.RewriteIndirectRefs(object.body, refMap)
			if err := budget.addResource(len(body)); err != nil {
				return nil, fmt.Errorf("reconstruct page %d object %s: %w", pageNumber, object.ref, err)
			}
			retained := retainedResources[object.ref]
			if retained == nil || retained.id == 0 {
				return nil, fmt.Errorf("reconstruct page %d object %s: retained resource is missing", pageNumber, object.ref)
			}
			if err := set(retained.id, body); err != nil {
				return nil, err
			}
		}
	}

	var kids strings.Builder
	kids.Grow(len(pageIDs) * 12)
	for _, pageID := range pageIDs {
		kids.WriteString(strconv.Itoa(pageID))
		kids.WriteString(" 0 R ")
	}
	if err := set(pagesID, []byte(fmt.Sprintf(
		"<< /Type /Pages /Kids [%s] /Count %d >>", kids.String(), len(pageIDs),
	))); err != nil {
		return nil, err
	}
	if err := set(catalogID, []byte(fmt.Sprintf("<< /Type /Catalog /Pages %d 0 R >>", pagesID))); err != nil {
		return nil, err
	}
	return builder.bytes(ctx, catalogID)
}

// importpdf encloses normalized page content in q/Q because its primary use is
// embedding the page as a Form XObject. A reconstructed page has no content
// before or after this stream, so retaining that private wrapper would add one
// nested graphics-state pair on every CDR pass and make sanitization
// non-idempotent.
func standalonePageContent(content []byte) ([]byte, error) {
	if len(content) < len("q\nQ") {
		return nil, errors.New("imported page content wrapper is invalid")
	}
	if !bytes.HasPrefix(content, []byte("q\n")) {
		return nil, errors.New("imported page content wrapper is invalid")
	}
	if !bytes.HasSuffix(content, []byte("Q")) {
		return nil, errors.New("imported page content wrapper is invalid")
	}
	content = bytes.TrimPrefix(content, []byte("q\n"))
	content = bytes.TrimSuffix(content, []byte("Q"))
	content = bytes.TrimSuffix(content, []byte("\n"))
	return content, nil
}

type reachableObject struct {
	ref          importpdf.ObjRef
	body         []byte
	preserveKeys bool
}

func reachableObjects(resources []byte, objects map[importpdf.ObjRef][]byte) ([]reachableObject, error) {
	byKey := make(map[refKey]importpdf.ObjRef, len(objects))
	for ref := range objects {
		byKey[refKey{objectNumber: ref.ObjectNumber(), generation: ref.Generation()}] = ref
	}
	root, err := sanitizePDFObjectWithReferences(resources, false)
	if err != nil {
		return nil, err
	}
	if ref, ok := conflictingReferenceRole(root.resourceNameRefs, root.normalRefs); ok {
		return nil, ambiguousResourceReferenceError(ref)
	}
	type queuedReference struct {
		key          refKey
		preserveKeys bool
	}
	queue := make([]queuedReference, 0, len(root.resourceNameRefs)+len(root.normalRefs))
	roles := make(map[refKey]bool, cap(queue))
	enqueue := func(refs map[refKey]bool, preserveKeys bool) error {
		for key := range refs {
			if previous, exists := roles[key]; exists {
				if previous != preserveKeys {
					return ambiguousResourceReferenceError(key)
				}
				continue
			}
			roles[key] = preserveKeys
			queue = append(queue, queuedReference{key: key, preserveKeys: preserveKeys})
		}
		return nil
	}
	if err := enqueue(root.resourceNameRefs, true); err != nil {
		return nil, err
	}
	if err := enqueue(root.normalRefs, false); err != nil {
		return nil, err
	}
	processed := make(map[importpdf.ObjRef]bool, len(queue))
	resultByRef := make(map[importpdf.ObjRef][]byte, len(objects))
	for len(queue) > 0 {
		pending := queue[0]
		queue = queue[1:]
		key := pending.key
		ref, ok := byKey[key]
		if !ok {
			return nil, fmt.Errorf("referenced object %d %d R is missing", key.objectNumber, key.generation)
		}
		if processed[ref] {
			continue
		}
		body, ok := objects[ref]
		if !ok {
			return nil, fmt.Errorf("referenced object %s is missing", ref)
		}
		sanitized, err := sanitizePDFObjectWithReferences(body, pending.preserveKeys)
		if err != nil {
			return nil, fmt.Errorf("object %s: %w", ref, err)
		}
		if child, ok := conflictingReferenceRole(sanitized.resourceNameRefs, sanitized.normalRefs); ok {
			return nil, fmt.Errorf("object %s: %w", ref, ambiguousResourceReferenceError(child))
		}
		processed[ref] = true
		resultByRef[ref] = sanitized.value
		if err := enqueue(sanitized.resourceNameRefs, true); err != nil {
			return nil, fmt.Errorf("object %s: %w", ref, err)
		}
		if err := enqueue(sanitized.normalRefs, false); err != nil {
			return nil, fmt.Errorf("object %s: %w", ref, err)
		}
	}
	result := make([]reachableObject, 0, len(resultByRef))
	for ref, body := range resultByRef {
		result = append(result, reachableObject{
			ref:          ref,
			body:         body,
			preserveKeys: roles[refKey{objectNumber: ref.ObjectNumber(), generation: ref.Generation()}],
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ref.ObjectNumber() == result[j].ref.ObjectNumber() {
			return result[i].ref.Generation() < result[j].ref.Generation()
		}
		return result[i].ref.ObjectNumber() < result[j].ref.ObjectNumber()
	})
	return result, nil
}

func conflictingReferenceRole(resourceRefs, normalRefs map[refKey]bool) (refKey, bool) {
	for ref := range resourceRefs {
		if normalRefs[ref] {
			return ref, true
		}
	}
	return refKey{}, false
}

func ambiguousResourceReferenceError(ref refKey) error {
	return fmt.Errorf(
		"PDF object %d %d R is used both as a resource-name dictionary and as an ordinary object",
		ref.objectNumber,
		ref.generation,
	)
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
	if b.err != nil {
		return nil, b.err
	}
	var out bytes.Buffer
	estimated := int64(len("%PDF-1.7\n%\xE2\xE3\xCF\xD3\n") + 128 + len(b.objects)*20)
	for id, object := range b.objects {
		addition := int64(len(object.body) + len(strconv.Itoa(id+1)) + len(" 0 obj\nendobj\n"))
		if addition > int64(^uint(0)>>1)-estimated {
			return nil, fmt.Errorf("%w: output size overflow", ErrLimitExceeded)
		}
		estimated += addition
	}
	if b.maxOutputBytes > 0 && estimated > b.maxOutputBytes {
		return nil, fmt.Errorf("%w: output bytes exceed maximum %d", ErrLimitExceeded, b.maxOutputBytes)
	}
	out.Grow(int(estimated))
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
		if b.maxOutputBytes > 0 && int64(out.Len()) > b.maxOutputBytes {
			return nil, fmt.Errorf("%w: output bytes exceed maximum %d", ErrLimitExceeded, b.maxOutputBytes)
		}
	}
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n", len(b.objects)+1)
	out.WriteString("0000000000 65535 f \n")
	for id := 1; id < len(offsets); id++ {
		fmt.Fprintf(&out, "%010d 00000 n \n", offsets[id])
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root %d 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(b.objects)+1, rootID, xref)
	if b.maxOutputBytes > 0 && int64(out.Len()) > b.maxOutputBytes {
		return nil, fmt.Errorf("%w: output bytes exceed maximum %d", ErrLimitExceeded, b.maxOutputBytes)
	}
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
