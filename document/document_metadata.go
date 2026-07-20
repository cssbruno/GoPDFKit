// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

// SetProducer sets the document producer. isUTF8 indicates whether producerStr
// is encoded as ISO-8859-1 (false) or UTF-8 (true).
func (f *Document) SetProducer(producerStr string, isUTF8 bool) {
	if isUTF8 {
		producerStr = utf8toutf16(producerStr)
	}
	f.producer = producerStr
}

// SetTitle sets the document title. isUTF8 indicates whether titleStr is
// encoded as ISO-8859-1 (false) or UTF-8 (true).
func (f *Document) SetTitle(titleStr string, isUTF8 bool) {
	if isUTF8 {
		titleStr = utf8toutf16(titleStr)
	}
	f.title = titleStr
}

// SetSubject sets the document subject. isUTF8 indicates whether subjectStr is
// encoded as ISO-8859-1 (false) or UTF-8 (true).
func (f *Document) SetSubject(subjectStr string, isUTF8 bool) {
	if isUTF8 {
		subjectStr = utf8toutf16(subjectStr)
	}
	f.subject = subjectStr
}

// SetAuthor sets the document author. isUTF8 indicates whether authorStr is
// encoded as ISO-8859-1 (false) or UTF-8 (true).
func (f *Document) SetAuthor(authorStr string, isUTF8 bool) {
	if isUTF8 {
		authorStr = utf8toutf16(authorStr)
	}
	f.author = authorStr
}

// SetKeywords sets the document keywords. keywordsStr is a space-delimited
// string, for example "invoice August". isUTF8 indicates whether keywordsStr
// is encoded as ISO-8859-1 (false) or UTF-8 (true).
func (f *Document) SetKeywords(keywordsStr string, isUTF8 bool) {
	if isUTF8 {
		keywordsStr = utf8toutf16(keywordsStr)
	}
	f.keywords = keywordsStr
}

// SetCreator sets the document creator. isUTF8 indicates whether creatorStr is
// encoded as ISO-8859-1 (false) or UTF-8 (true).
func (f *Document) SetCreator(creatorStr string, isUTF8 bool) {
	if isUTF8 {
		creatorStr = utf8toutf16(creatorStr)
	}
	f.creator = creatorStr
}

// SetXmpMetadata defines XMP metadata that will be embedded with the document.
func (f *Document) SetXmpMetadata(xmpStream []byte) {
	f.xmp = append([]byte(nil), xmpStream...)
	f.nXmp = 0
}
