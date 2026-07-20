// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import "strconv"

type pdfResourceName string

func (name pdfResourceName) String() string {
	return string(name)
}

type pdfResourceRef struct {
	name         pdfResourceName
	objectNumber int
}

func fontPDFResourceName(font fontDefinition) pdfResourceName {
	return pdfResourceName("/F" + font.i)
}

func fontPDFResourceRef(font fontDefinition) pdfResourceRef {
	return pdfResourceRef{name: fontPDFResourceName(font), objectNumber: font.N}
}

func imagePDFResourceName(image *ImageInfo) pdfResourceName {
	if image == nil {
		return ""
	}
	return pdfResourceName("/I" + image.i)
}

func imagePDFResourceRef(image *ImageInfo) pdfResourceRef {
	if image == nil {
		return pdfResourceRef{}
	}
	return pdfResourceRef{name: imagePDFResourceName(image), objectNumber: image.n}
}

func templatePDFResourceName(id string) pdfResourceName {
	return pdfResourceName("/TPL" + id)
}

func templatePDFResourceRef(id string, objectNumber int) pdfResourceRef {
	return pdfResourceRef{name: templatePDFResourceName(id), objectNumber: objectNumber}
}

func importedPagePDFResourceName(id int) pdfResourceName {
	return pdfResourceName("/IPG" + strconv.Itoa(id))
}

func importedPagePDFResourceRef(id, objectNumber int) pdfResourceRef {
	return pdfResourceRef{name: importedPagePDFResourceName(id), objectNumber: objectNumber}
}

func graphicsStatePDFResourceName(id int) pdfResourceName {
	return pdfResourceName("/GS" + strconv.Itoa(id))
}

func graphicsStatePDFResourceRef(id, objectNumber int) pdfResourceRef {
	return pdfResourceRef{name: graphicsStatePDFResourceName(id), objectNumber: objectNumber}
}

func shadingPDFResourceName(id int) pdfResourceName {
	return pdfResourceName("/Sh" + strconv.Itoa(id))
}

func shadingPDFResourceRef(id, objectNumber int) pdfResourceRef {
	return pdfResourceRef{name: shadingPDFResourceName(id), objectNumber: objectNumber}
}

func spotColorPDFResourceName(id int) pdfResourceName {
	return pdfResourceName("/CS" + strconv.Itoa(id))
}

func spotColorPDFResourceRef(id, objectNumber int) pdfResourceRef {
	return pdfResourceRef{name: spotColorPDFResourceName(id), objectNumber: objectNumber}
}

func optionalContentPDFResourceName(id int) pdfResourceName {
	return pdfResourceName("/OC" + strconv.Itoa(id))
}

func optionalContentPDFResourceRef(id, objectNumber int) pdfResourceRef {
	return pdfResourceRef{name: optionalContentPDFResourceName(id), objectNumber: objectNumber}
}

func appendPDFResourceRefValue(buf []byte, ref pdfResourceRef) []byte {
	return appendPDFResourceNameRef(buf, ref.name, ref.objectNumber)
}

func appendPDFResourceNameRef(buf []byte, name pdfResourceName, objNum int) []byte {
	buf = append(buf, string(name)...)
	buf = append(buf, ' ')
	buf = strconv.AppendInt(buf, int64(objNum), 10)
	buf = append(buf, " 0 R"...)
	return buf
}
