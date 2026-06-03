// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"strings"
)

// Attachment defines content to include in the PDF in one of the following ways:
//   - associated with the document as a whole; see SetAttachments()
//   - accessible through a link anchored on a page; see AddAttachmentAnnotation()
type Attachment struct {
	// Content contains the bytes embedded in the PDF.
	Content []byte

	// Filename is the displayed name of the attachment.
	Filename string

	// Description is only displayed when using AddAttachmentAnnotation(),
	// and might be modified by the PDF reader.
	Description string

	objectNumber int // Filled when the content is embedded.
}

// checksum returns the hex-encoded checksum of data.
func checksum(data []byte) string {
	tmp := md5.Sum(data)
	return hex.EncodeToString(tmp[:])
}

// writeCompressedFileObject writes a deflate-compressed /EmbeddedFile object
// with length, compressed length, and MD5 checksum metadata.
func (f *Document) writeCompressedFileObject(content []byte) {
	lenUncompressed := len(content)
	sum := checksum(content)
	compressed := f.compressBytes(content)
	if f.err != nil {
		return
	}
	lenCompressed := len(compressed)
	f.newobj()
	f.outf("<< /Type /EmbeddedFile /Length %d /Filter /FlateDecode /Params << /CheckSum <%s> /Size %d >> >>\n",
		lenCompressed, sum, lenUncompressed)
	f.putstream(compressed)
	f.out("endobj")
}

// embed includes the attachment content and updates its internal reference.
func (f *Document) embed(a *Attachment) {
	if a.objectNumber != 0 { // Already embedded; object numbers start at 2.
		return
	}
	oldState := f.state
	f.state = 1 // Write file content to the main buffer.
	f.writeCompressedFileObject(a.Content)
	streamID := f.n
	f.newobj()
	f.outf("<< /Type /Filespec /F () /UF %s /EF << /F %d 0 R >> /Desc %s\n>>",
		f.textstring(utf8toutf16(a.Filename)),
		streamID,
		f.textstring(utf8toutf16(a.Description)))
	f.out("endobj")
	a.objectNumber = f.n
	f.state = oldState
}

// SetAttachments writes attachments as embedded files attached to the document.
// These attachments are global; see AddAttachmentAnnotation() for links
// anchored on a page. Only the last call to SetAttachments is used; previous
// calls are discarded. Be aware that not all PDF readers
// support document attachments. See the SetAttachment example for a
// demonstration of this method.
func (f *Document) SetAttachments(as []Attachment) {
	f.attachments = as
}

// putAttachments embeds the current attachments and stores their object numbers
// for later use by getEmbeddedFiles().
func (f *Document) putAttachments() {
	for i, a := range f.attachments {
		f.embed(&a)
		f.attachments[i] = a
	}
}

// getEmbeddedFiles returns the /EmbeddedFiles name-tree catalog entry.
func (f Document) getEmbeddedFiles() string {
	names := make([]string, len(f.attachments))
	for i, as := range f.attachments {
		names[i] = fmt.Sprintf("(Attachement%d) %d 0 R ", i+1, as.objectNumber)
	}
	nameTree := fmt.Sprintf("<< /Names [\n %s \n] >>", strings.Join(names, "\n"))
	return nameTree
}

// ---------------------------------- Annotations ----------------------------------
type annotationAttach struct {
	*Attachment

	x, y, w, h float64 // PDF coordinates; y has been adjusted and scaled.
}

// AddAttachmentAnnotation puts a link on the current page over the rectangle
// defined by x, y, w, and h. This link points to the content defined in a,
// which is embedded in the document. This method does not draw anything; call a
// method such as Cell() or Rect() to indicate that a link is present. Requiring
// a pointer to an Attachment avoids unnecessary copies in the resulting PDF:
// attachments that point to the same data are included only once and shared
// among all links. Be aware that not all PDF readers support
// annotated attachments. See the AddAttachmentAnnotation example for a
// demonstration of this method.
func (f *Document) AddAttachmentAnnotation(a *Attachment, x, y, w, h float64) {
	if a == nil {
		return
	}
	if !finiteNumbers(x, y, w, h) {
		f.SetErrorf("invalid attachment annotation rectangle")
		return
	}
	f.pageAttachments[f.page] = append(f.pageAttachments[f.page], annotationAttach{
		Attachment: a,
		x:          x * f.k, y: f.hPt - y*f.k, w: w * f.k, h: h * f.k,
	})
}

// putAnnotationsAttachments embeds attachments used by annotations and stores
// their object numbers for putAttachmentAnnotationLinks(), which is called for
// each page.
func (f *Document) putAnnotationsAttachments() {
	// Avoid duplicate embedded attachments.
	m := map[*Attachment]bool{}
	for _, l := range f.pageAttachments {
		for _, an := range l {
			if m[an.Attachment] { // Already embedded.
				continue
			}
			f.embed(an.Attachment)
			m[an.Attachment] = true
		}
	}
}

func (f *Document) putAttachmentAnnotationLinks(out *fmtBuffer, page int) {
	for _, an := range f.pageAttachments[page] {
		x1, y1, x2, y2 := an.x, an.y, an.x+an.w, an.y-an.h
		as := fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [%.2f %.2f %.2f %.2f] /Length 0 >>",
			x1, y1, x2, y2)
		as += "\nstream\nendstream"

		out.printf("<< /Type /Annot /Subtype /FileAttachment /Rect [%.2f %.2f %.2f %.2f] /Border [0 0 0]\n",
			x1, y1, x2, y2)
		out.printf("/Contents %s ", f.textstring(utf8toutf16(an.Description)))
		out.printf("/T %s ", f.textstring(utf8toutf16(an.Filename)))
		out.printf("/AP << /N %s>>", as)
		out.printf("/FS %d 0 R >>\n", an.objectNumber)
	}
}
