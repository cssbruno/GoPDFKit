/****************************************************************************
 * Software: GoPDFKit                                                         *
 * License:  MIT License                                                    *
 *                                                                          *
 * Copyright (c) 2026 cssBruno                                              *
 ****************************************************************************/

package gopdfkit

import (
	"io"
	"time"
)

type Pdf interface {
	AddFont(familyStr, styleStr, fileStr string)
	AddFontFromBytes(familyStr, styleStr string, jsonFileBytes, zFileBytes []byte)
	AddFontFromReader(familyStr, styleStr string, r io.Reader)
	AddUTF8FontFromCache(familyStr, styleStr string, cache *FontCache)
	AddLayer(name string, visible bool) (layerID int)
	AddLink() int
	AddPage()
	AddPageFormat(orientationStr string, size Size)
	AddPageFormatRotation(orientationStr string, size Size, rotation int)
	AddPageRotation(rotation int)
	AddSpotColor(name string, cyan, magenta, yellow, black byte)
	AliasNbPages(aliasStr string)
	ArcTo(x, y, rx, ry, degRotate, degStart, degEnd float64)
	Arc(x, y, rx, ry, degRotate, degStart, degEnd float64, styleStr string)
	BeginLayer(id int)
	Beziergon(points []Point, styleStr string)
	Bookmark(txtStr string, level int, y float64)
	CellFormat(w, h float64, txtStr, borderStr string, ln int, alignStr string, fill bool, link int, linkStr string)
	Cellf(w, h float64, fmtStr string, args ...any)
	Cell(w, h float64, txtStr string)
	Circle(x, y, r float64, styleStr string)
	ClearError()
	ClipCircle(x, y, r float64, outline bool)
	ClipEllipse(x, y, rx, ry float64, outline bool)
	ClipEnd()
	ClipPolygon(points []Point, outline bool)
	ClipRect(x, y, w, h float64, outline bool)
	ClipRoundedRect(x, y, w, h, r float64, outline bool)
	ClipText(x, y float64, txtStr string, outline bool)
	Close()
	ClosePath()
	CreateTemplateCustom(corner Point, size Size, fn func(*Tpl)) Template
	CreateTemplate(fn func(*Tpl)) Template
	CurveBezierCubicTo(cx0, cy0, cx1, cy1, x, y float64)
	CurveBezierCubic(x0, y0, cx0, cy0, cx1, cy1, x1, y1 float64, styleStr string)
	CurveCubic(x0, y0, cx0, cy0, x1, y1, cx1, cy1 float64, styleStr string)
	CurveTo(cx, cy, x, y float64)
	Curve(x0, y0, cx, cy, x1, y1 float64, styleStr string)
	DrawPath(styleStr string)
	Ellipse(x, y, rx, ry, degRotate float64, styleStr string)
	EndLayer()
	Err() bool
	Error() error
	GetAlpha() (alpha float64, blendModeStr string)
	GetAutoPageBreak() (auto bool, margin float64)
	GetCellMargin() float64
	GetConversionRatio() float64
	GetDrawColor() (int, int, int)
	GetDrawSpotColor() (name string, c, m, y, k byte)
	GetFillColor() (int, int, int)
	GetFillSpotColor() (name string, c, m, y, k byte)
	GetFontDesc(familyStr, styleStr string) FontDescriptor
	GetFontSize() (ptSize, unitSize float64)
	GetImageInfo(imageStr string) (info *ImageInfo)
	GetLineWidth() float64
	GetMargins() (left, top, right, bottom float64)
	GetPageSizeStr(sizeStr string) (size Size)
	GetPageSize() (width, height float64)
	GetPageSizes(source any) map[int]map[string]Size
	GetPageWidth() float64
	GetPageHeight() float64
	GetStringWidth(s string) float64
	GetTextColor() (int, int, int)
	GetTextSpotColor() (name string, c, m, y, k byte)
	GetX() float64
	GetXY() (float64, float64)
	GetY() float64
	HTMLNew() (html HTML)
	ImageOptions(imageNameStr string, x, y, w, h float64, flow bool, options ImageOptions, link int, linkStr string)
	ImageOptionsExtended(imageNameStr string, options ExtendedImageOptions)
	ImageTypeFromMime(mimeStr string) (tp string)
	ImportPage(sourceFile string, pageNo int, box string) int
	ImportPageStream(source io.Reader, pageNo int, box string) int
	ImportPagesFromSource(source any, box string) []int
	LinearGradient(x, y, w, h float64, r1, g1, b1, r2, g2, b2 int, x1, y1, x2, y2 float64)
	LineTo(x, y float64)
	Line(x1, y1, x2, y2 float64)
	LinkString(x, y, w, h float64, linkStr string)
	Link(x, y, w, h float64, link int)
	Ln(h float64)
	MoveTo(x, y float64)
	MultiCell(w, h float64, txtStr, borderStr, alignStr string, fill bool)
	Ok() bool
	OpenLayerPane()
	OutputAndClose(w io.WriteCloser) error
	OutputFileAndClose(fileStr string) error
	Output(w io.Writer) error
	PageCount() int
	PageNo() int
	PageSize(pageNum int) (wd, ht float64, unitStr string)
	PointConvert(pt float64) (u float64)
	PointToUnitConvert(pt float64) (u float64)
	Polygon(points []Point, styleStr string)
	RadialGradient(x, y, w, h float64, r1, g1, b1, r2, g2, b2 int, x1, y1, x2, y2, r float64)
	RawWriteBuf(r io.Reader)
	RawWriteStr(str string)
	Rect(x, y, w, h float64, styleStr string)
	RegisterAlias(alias, replacement string)
	RegisterImageOptions(fileStr string, options ImageOptions) (info *ImageInfo)
	RegisterImageOptionsReader(imgName string, options ImageOptions, r io.Reader) (info *ImageInfo)
	SetAcceptPageBreakFunc(fnc func() bool)
	SetAlpha(alpha float64, blendModeStr string)
	SetAuthor(authorStr string, isUTF8 bool)
	SetAutoPageBreak(auto bool, margin float64)
	SetCatalogSort(flag bool)
	SetCellMargin(margin float64)
	SetCompression(compress bool)
	SetCompressionLevel(level int)
	SetCreationDate(tm time.Time)
	SetCreator(creatorStr string, isUTF8 bool)
	SetDashPattern(dashArray []float64, dashPhase float64)
	SetDisplayMode(zoomStr, layoutStr string)
	SetDrawColor(r, g, b int)
	SetDrawSpotColor(name string, tint byte)
	SetError(err error)
	SetErrorf(fmtStr string, args ...any)
	SetFillColor(r, g, b int)
	SetFillSpotColor(name string, tint byte)
	SetFont(familyStr, styleStr string, size float64)
	SetFontLoader(loader FontLoader)
	SetFontLocation(fontDirStr string)
	SetFontSize(size float64)
	SetFontStyle(styleStr string)
	SetFontUnitSize(size float64)
	SetFooterFunc(fnc func())
	SetFooterFuncLpi(fnc func(lastPage bool))
	SetHeaderFunc(fnc func())
	SetHeaderFuncMode(fnc func(), homeMode bool)
	SetHomeXY()
	SetJavascript(script string)
	SetKeywords(keywordsStr string, isUTF8 bool)
	SetLeftMargin(margin float64)
	SetLineCapStyle(styleStr string)
	SetLineJoinStyle(styleStr string)
	SetLineWidth(width float64)
	SetLink(link int, y float64, page int)
	SetMargins(left, top, right float64)
	SetNoCompression()
	SetPageBoxRec(t string, pb PageBox)
	SetPageBox(t string, x, y, wd, ht float64)
	SetPage(pageNum int)
	SetProtection(actionFlag byte, userPassStr, ownerPassStr string)
	SetRightMargin(margin float64)
	SetSubject(subjectStr string, isUTF8 bool)
	SetTextColor(r, g, b int)
	SetTextSpotColor(name string, tint byte)
	SetTitle(titleStr string, isUTF8 bool)
	SetTopMargin(margin float64)
	SetUnderlineThickness(thickness float64)
	SetXmpMetadata(xmpStream []byte)
	SetX(x float64)
	SetXY(x, y float64)
	SetY(y float64)
	SetYWithResetX(y float64, resetX bool)
	SplitLines(txt []byte, w float64) [][]byte
	String() string
	SVGWrite(sb *SVG, scale float64)
	Text(x, y float64, txtStr string)
	UseImportedPage(pageID int, x, y, w, h float64)
	TransformBegin()
	TransformEnd()
	TransformMirrorHorizontal(x float64)
	TransformMirrorLine(angle, x, y float64)
	TransformMirrorPoint(x, y float64)
	TransformMirrorVertical(y float64)
	TransformRotate(angle, x, y float64)
	TransformScale(scaleWd, scaleHt, x, y float64)
	TransformScaleX(scaleWd, x, y float64)
	TransformScaleXY(s, x, y float64)
	TransformScaleY(scaleHt, x, y float64)
	TransformSkew(angleX, angleY, x, y float64)
	TransformSkewX(angleX, x, y float64)
	TransformSkewY(angleY, x, y float64)
	Transform(tm TransformMatrix)
	TransformTranslate(tx, ty float64)
	TransformTranslateX(tx float64)
	TransformTranslateY(ty float64)
	UnicodeTranslatorFromDescriptor(cpStr string) (rep func(string) string)
	UnitToPointConvert(u float64) (pt float64)
	UseTemplateScaled(t Template, corner Point, size Size)
	UseTemplate(t Template)
	WriteAligned(width, lineHeight float64, textStr, alignStr string)
	Writef(h float64, fmtStr string, args ...any)
	Write(h float64, txtStr string)
	WriteLinkID(h float64, displayStr string, linkID int)
	WriteLinkString(h float64, displayStr, targetStr string)
}
