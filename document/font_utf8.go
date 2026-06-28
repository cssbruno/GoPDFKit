// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"math/bits"
	"slices"
	"sort"
	"strings"
	"sync"
	"unsafe"
)

// Composite glyph flags.
const symbolWords = 1 << 0
const symbolScale = 1 << 3
const symbolContinue = 1 << 5
const symbolAllScale = 1 << 6
const symbol2x2 = 1 << 7

func utf8ToUnicodeCMap() string {
	return utf8ToUnicodeCMapText
}

var utf8ToUnicodeCMapText = buildUTF8ToUnicodeCMap()

func buildUTF8ToUnicodeCMap() string {
	out := make([]byte, 0, 8192)
	out = append(out, "/CIDInit /ProcSet findresource begin\n12 dict begin\nbegincmap\n/CIDSystemInfo\n"...)
	out = append(out, "<</Registry (Adobe)\n/Ordering (UCS)\n/Supplement 0\n>> def\n"...)
	out = append(out, "/CMapName /Adobe-Identity-UCS def\n/CMapType 2 def\n"...)
	out = append(out, "1 begincodespacerange\n<0000> <FFFF>\nendcodespacerange\n"...)
	out = append(out, "256 beginbfrange\n"...)
	for start := 0; start <= 0xff00; start += 0x100 {
		end := start + 0xff
		out = append(out, '<')
		out = appendPDFHexUint16(out, start)
		out = append(out, "> <"...)
		out = appendPDFHexUint16(out, end)
		out = append(out, "> <"...)
		out = appendPDFHexUint16(out, start)
		out = append(out, ">\n"...)
	}
	out = append(out, "endbfrange\nendcmap\nCMapName currentdict /CMap defineresource pop\nend"...)
	return string(out)
}

func appendPDFHexUint16(dst []byte, value int) []byte {
	const hex = "0123456789ABCDEF"
	return append(dst, hex[(value>>12)&0xf], hex[(value>>8)&0xf], hex[(value>>4)&0xf], hex[value&0xf])
}

type utf8FontFile struct {
	fileReader           *fileReader
	LastRune             int
	tableDescriptions    map[string]*tableDescription
	outTablesData        map[string][]byte
	symbolPosition       []int
	charSymbolDictionary map[int]int
	Ascent               int
	Descent              int
	fontElementSize      int
	Bbox                 fontBoxType
	CapHeight            int
	StemV                int
	ItalicAngle          int
	Flags                int
	UnderlinePosition    float64
	UnderlineThickness   float64
	CharWidths           []int
	DefaultWidth         float64
	CodeSymbolDictionary map[int]int
	// static holds parsed tables that depend only on the font file (not the
	// per-document used-rune subset). When non-nil, GenerateCutFont reuses it
	// read-only instead of re-parsing the cmap/loca/table directory for every
	// document. It is shared across documents and must never be mutated.
	static *utf8StaticTables
}

// utf8StaticTables holds the immutable, font-only parse results shared across
// all documents that embed the same UTF-8 font. Every field is read-only after
// construction so concurrent document subsetting is race-free.
type utf8StaticTables struct {
	tableDescriptions    map[string]*tableDescription
	charSymbolDictionary map[int]int
	symbolPosition       []int
	oldMetrics           int // hhea numberOfHMetrics
	numSymbols           int // maxp numGlyphs
	locaFormat           int // head indexToLocFormat
}

type utf8SubsetCacheKey struct {
	fontPtr uintptr
	hash    uint64
	hash2   uint64
	count   int
}

type utf8SubsetCacheValue struct {
	data                 []byte
	codeSymbolDictionary map[int]int
	lastRune             int
}

var utf8SubsetCache = struct {
	sync.Mutex
	entries map[utf8SubsetCacheKey]utf8SubsetCacheValue
	order   []utf8SubsetCacheKey
}{
	entries: make(map[utf8SubsetCacheKey]utf8SubsetCacheValue),
}

const maxUTF8SubsetCacheEntries = 32

type tableDescription struct {
	name     string
	checksum []int
	position int
	size     int
}

type fileReader struct {
	readerPosition int64
	array          []byte
	err            error
}

var fontReadZeroPadding [4096]byte

func (fr *fileReader) Read(s int) []byte {
	if s < 0 {
		fr.err = fmt.Errorf("invalid font read length: %d", s)
		return []byte{}
	}
	start := fr.readerPosition
	end := start + int64(s)
	if start < 0 || end < start || end > int64(len(fr.array)) {
		if fr.err == nil {
			fr.err = errors.New("unexpected end of font data")
		}
		if start >= int64(len(fr.array)) {
			fr.readerPosition = int64(len(fr.array))
			if s <= len(fontReadZeroPadding) {
				return fontReadZeroPadding[:s]
			}
			return make([]byte, s)
		}
		out := make([]byte, s)
		if start < 0 {
			start = 0
		}
		if start < int64(len(fr.array)) {
			availableEnd := min(end, int64(len(fr.array)))
			copy(out, fr.array[start:availableEnd])
		}
		fr.readerPosition = int64(len(fr.array))
		return out
	}
	b := fr.array[start:end]
	fr.readerPosition = end
	return b
}

func (fr *fileReader) seek(shift int64, flag int) (int64, error) {
	next := fr.readerPosition
	switch flag {
	case 0:
		next = shift
	case 1:
		next += shift
	case 2:
		next = int64(len(fr.array)) - shift
	}
	if next < 0 || next > int64(len(fr.array)) {
		err := errors.New("font seek out of range")
		if fr.err == nil {
			fr.err = err
		}
		return fr.readerPosition, err
	}
	fr.readerPosition = next
	return fr.readerPosition, nil
}

func newUTF8Font(reader *fileReader) *utf8FontFile {
	utf := utf8FontFile{
		fileReader: reader,
	}
	return &utf
}

func (utf *utf8FontFile) parseFile() error {
	utf.fileReader.readerPosition = 0
	utf.symbolPosition = make([]int, 0)
	utf.charSymbolDictionary = make(map[int]int)
	utf.tableDescriptions = make(map[string]*tableDescription)
	utf.outTablesData = make(map[string][]byte)
	utf.Ascent = 0
	utf.Descent = 0
	codeType := uint32(utf.readUint32())
	if utf.fileReader.err != nil {
		return utf.fileReader.err
	}
	if codeType == 0x4F54544F {
		return errors.New("OpenType/CFF fonts are not supported by AddUTF8Font; use font.Make and AddFont for single-byte encodings")
	}
	if codeType == 0x74746366 {
		return errors.New("OpenType font collections are not supported")
	}
	if codeType != 0x00010000 && codeType != 0x74727565 {
		return fmt.Errorf("not a TrueType-outline font: codeType=%v", codeType)
	}
	utf.generateTableDescriptions()
	utf.parseTables()
	if utf.fileReader.err != nil {
		return utf.fileReader.err
	}
	return nil
}

func (utf *utf8FontFile) generateTableDescriptions() {
	tablesCount := utf.readUint16()
	_ = utf.readUint16()
	_ = utf.readUint16()
	_ = utf.readUint16()
	utf.tableDescriptions = make(map[string]*tableDescription)

	for range tablesCount {
		record := tableDescription{
			name:     utf.readTableName(),
			checksum: []int{utf.readUint16(), utf.readUint16()},
			position: utf.readUint32(),
			size:     utf.readUint32(),
		}
		utf.tableDescriptions[record.name] = &record
	}
}

func (utf *utf8FontFile) readTableName() string {
	return string(utf.fileReader.Read(4))
}

func (utf *utf8FontFile) readUint16() int {
	s := utf.fileReader.Read(2)
	return (int(s[0]) << 8) + int(s[1])
}

func (utf *utf8FontFile) readUint32() int {
	s := utf.fileReader.Read(4)
	return (int(s[0]) * 16777216) + (int(s[1]) << 16) + (int(s[2]) << 8) + int(s[3]) // 16777216 = 1 << 24.
}

func (utf *utf8FontFile) calcInt32(x, y [2]int) [2]int {
	answer := [2]int{}
	if y[1] > x[1] {
		x[1] += 1 << 16
		x[0]++
	}
	answer[1] = x[1] - y[1]
	if y[0] > x[0] {
		x[0] += 1 << 16
	}
	answer[0] = x[0] - y[0]
	answer[0] &= 0xFFFF
	return answer
}

func (utf *utf8FontFile) generateChecksum(data []byte) [2]int {
	answer := [2]int{}
	i := 0
	for ; i+4 <= len(data); i += 4 {
		answer[0] += (int(data[i]) << 8) + int(data[i+1])
		answer[1] += (int(data[i+2]) << 8) + int(data[i+3])
		answer[0] += answer[1] >> 16
		answer[1] &= 0xFFFF
		answer[0] &= 0xFFFF
	}
	var tail [4]byte
	if i < len(data) {
		copy(tail[:], data[i:])
		answer[0] += (int(tail[0]) << 8) + int(tail[1])
		answer[1] += (int(tail[2]) << 8) + int(tail[3])
		answer[0] += answer[1] >> 16
		answer[1] &= 0xFFFF
		answer[0] &= 0xFFFF
	}
	return answer
}

func (utf *utf8FontFile) seek(shift int) {
	_, _ = utf.fileReader.seek(int64(shift), 0)
}

func (utf *utf8FontFile) skip(delta int) {
	_, _ = utf.fileReader.seek(int64(delta), 1)
}

// SeekTable moves to the start of the named table.
func (utf *utf8FontFile) SeekTable(name string) int {
	return utf.seekTable(name, 0)
}

func (utf *utf8FontFile) seekTable(name string, offsetInTable int) int {
	desc := utf.tableDescriptions[name]
	if desc == nil {
		if utf.fileReader.err == nil {
			utf.fileReader.err = fmt.Errorf("missing required font table: %s", name)
		}
		return int(utf.fileReader.readerPosition)
	}
	_, _ = utf.fileReader.seek(int64(desc.position+offsetInTable), 0)
	return int(utf.fileReader.readerPosition)
}

func (utf *utf8FontFile) readInt16() int16 {
	s := utf.fileReader.Read(2)
	a := (int16(s[0]) << 8) + int16(s[1])
	if (int(a) & (1 << 15)) == 0 {
		a = int16(int(a) - (1 << 16))
	}
	return a
}

func (utf *utf8FontFile) getUint16(pos int) int {
	_, _ = utf.fileReader.seek(int64(pos), 0)
	s := utf.fileReader.Read(2)
	return (int(s[0]) << 8) + int(s[1])
}

func (utf *utf8FontFile) patchBytes(stream []byte, offset int, value []byte) []byte {
	if offset < 0 || offset+len(value) > len(stream) {
		utf.fileReader.err = errors.New("font table patch out of range")
		return stream
	}
	copy(stream[offset:offset+len(value)], value)
	return stream
}

func (utf *utf8FontFile) insertUint16(stream []byte, offset int, value int) []byte {
	if offset < 0 || offset+2 > len(stream) {
		utf.fileReader.err = errors.New("font table uint16 patch out of range")
		return stream
	}
	binary.BigEndian.PutUint16(stream[offset:], uint16(value))
	return stream
}

func (utf *utf8FontFile) getRange(pos, length int) []byte {
	_, _ = utf.fileReader.seek(int64(pos), 0)
	if length < 1 {
		return make([]byte, 0)
	}
	s := utf.fileReader.Read(length)
	return s
}

func (utf *utf8FontFile) getTableData(name string) []byte {
	desckrip := utf.tableDescriptions[name]
	if desckrip == nil {
		return []byte{}
	}
	if desckrip.size == 0 {
		return []byte{}
	}
	_, _ = utf.fileReader.seek(int64(desckrip.position), 0)
	s := utf.fileReader.Read(desckrip.size)
	return s
}

func (utf *utf8FontFile) setOutTable(name string, data []byte) {
	if data == nil {
		return
	}
	if name == "head" {
		data = utf.patchBytes(data, 8, []byte{0, 0, 0, 0})
	}
	utf.outTablesData[name] = data
}

func arrayKeys(arr map[int]string) []int {
	answer := make([]int, len(arr))
	i := 0
	for key := range arr {
		answer[i] = key
		i++
	}
	return answer
}

func inArray(s int, arr []int) bool {
	return slices.Contains(arr, s)
}

func (utf *utf8FontFile) parseNAMETable() int {
	namePosition := utf.SeekTable("name")
	format := utf.readUint16()
	if format != 0 {
		fmt.Printf("Illegal format %d\n", format)
		return format
	}
	nameCount := utf.readUint16()
	stringDataPosition := namePosition + utf.readUint16()
	names := map[int]string{1: "", 2: "", 3: "", 4: "", 6: ""}
	keys := arrayKeys(names)
	counter := len(names)
	for range nameCount {
		system := utf.readUint16()
		code := utf.readUint16()
		local := utf.readUint16()
		nameID := utf.readUint16()
		size := utf.readUint16()
		position := utf.readUint16()
		if !inArray(nameID, keys) {
			continue
		}
		currentName := ""
		if system == 3 && code == 1 && local == 0x409 {
			oldPos := utf.fileReader.readerPosition
			utf.seek(stringDataPosition + position)
			if size%2 != 0 {
				fmt.Printf("name is not binar byte format\n")
				return format
			}
			size /= 2
			currentName = ""
			var currentNameSb343 strings.Builder
			for size > 0 {
				char := utf.readUint16()
				currentNameSb343.WriteRune(rune(char))
				size--
			}
			currentName += currentNameSb343.String()
			utf.fileReader.readerPosition = oldPos
			utf.seek(int(oldPos))
		} else if system == 1 && code == 0 && local == 0 {
			oldPos := utf.fileReader.readerPosition
			currentName = string(utf.getRange(stringDataPosition+position, size))
			utf.fileReader.readerPosition = oldPos
			utf.seek(int(oldPos))
		}
		if currentName != "" && names[nameID] == "" {
			names[nameID] = currentName
			counter--
			if counter == 0 {
				break
			}
		}
	}
	return format
}

func (utf *utf8FontFile) parseHEADTable() {
	utf.SeekTable("head")
	utf.skip(18)
	utf.fontElementSize = utf.readUint16()
	scale := 1000.0 / float64(utf.fontElementSize)
	utf.skip(16)
	xMin := utf.readInt16()
	yMin := utf.readInt16()
	xMax := utf.readInt16()
	yMax := utf.readInt16()
	utf.Bbox = fontBoxType{int(float64(xMin) * scale), int(float64(yMin) * scale), int(float64(xMax) * scale), int(float64(yMax) * scale)}
	utf.skip(3 * 2)
	_ = utf.readUint16()
	symbolDataFormat := utf.readUint16()
	if symbolDataFormat != 0 {
		fmt.Printf("Unknown symbol data format %d\n", symbolDataFormat)
		return
	}
}

func (utf *utf8FontFile) parseHHEATable() int {
	metricsCount := 0
	if _, OK := utf.tableDescriptions["hhea"]; OK {
		scale := 1000.0 / float64(utf.fontElementSize)
		utf.SeekTable("hhea")
		utf.skip(4)
		hheaAscender := utf.readInt16()
		hheaDescender := utf.readInt16()
		utf.Ascent = int(float64(hheaAscender) * scale)
		utf.Descent = int(float64(hheaDescender) * scale)
		utf.skip(24)
		metricDataFormat := utf.readUint16()
		if metricDataFormat != 0 {
			fmt.Printf("Unknown horizontal metric data format %d\n", metricDataFormat)
			return 0
		}
		metricsCount = utf.readUint16()
		if metricsCount == 0 {
			fmt.Printf("Number of horizontal metrics is 0\n")
			return 0
		}
	}
	return metricsCount
}

func (utf *utf8FontFile) parseOS2Table() int {
	var weightType int
	scale := 1000.0 / float64(utf.fontElementSize)
	if _, OK := utf.tableDescriptions["OS/2"]; OK {
		utf.SeekTable("OS/2")
		version := utf.readUint16()
		utf.skip(2)
		weightType = utf.readUint16()
		utf.skip(2)
		fsType := utf.readUint16()
		if fsType == 0x0002 || (fsType&0x0300) != 0 {
			fmt.Printf("ERROR - copyright restrictions.\n")
			return 0
		}
		utf.skip(20)
		_ = utf.readInt16()

		utf.skip(36)
		sTypoAscender := utf.readInt16()
		sTypoDescender := utf.readInt16()
		if utf.Ascent == 0 {
			utf.Ascent = int(float64(sTypoAscender) * scale)
		}
		if utf.Descent == 0 {
			utf.Descent = int(float64(sTypoDescender) * scale)
		}
		if version > 1 {
			utf.skip(16)
			sCapHeight := utf.readInt16()
			utf.CapHeight = int(float64(sCapHeight) * scale)
		} else {
			utf.CapHeight = utf.Ascent
		}
	} else {
		weightType = 500
		if utf.Ascent == 0 {
			utf.Ascent = int(float64(utf.Bbox.Ymax) * scale)
		}
		if utf.Descent == 0 {
			utf.Descent = int(float64(utf.Bbox.Ymin) * scale)
		}
		utf.CapHeight = utf.Ascent
	}
	utf.StemV = 50 + int(math.Pow(float64(weightType)/65.0, 2))
	return weightType
}

func (utf *utf8FontFile) parsePOSTTable(weight int) {
	utf.SeekTable("post")
	utf.skip(4)
	utf.ItalicAngle = int(utf.readInt16()) + utf.readUint16()/65536.0
	scale := 1000.0 / float64(utf.fontElementSize)
	utf.UnderlinePosition = float64(utf.readInt16()) * scale
	utf.UnderlineThickness = float64(utf.readInt16()) * scale
	fixed := utf.readUint32()

	utf.Flags = 4

	if utf.ItalicAngle != 0 {
		utf.Flags |= 64
	}
	if weight >= 600 {
		utf.Flags |= 262144
	}
	if fixed != 0 {
		utf.Flags |= 1
	}
}

func (utf *utf8FontFile) parseCMAPTable(_ int) int {
	var format int
	cmapPosition := utf.SeekTable("cmap")
	utf.skip(2)
	cmapTableCount := utf.readUint16()
	cidCMAPPosition := 0
	for range cmapTableCount {
		system := utf.readUint16()
		coded := utf.readUint16()
		position := utf.readUint32()
		oldReaderPosition := utf.fileReader.readerPosition
		if (system == 3 && coded == 1) || system == 0 { // Microsoft, Unicode
			format = utf.getUint16(cmapPosition + position)
			if format == 4 {
				if cidCMAPPosition == 0 {
					cidCMAPPosition = cmapPosition + position
				}
				break
			}
		}
		utf.seek(int(oldReaderPosition))
	}
	if cidCMAPPosition == 0 {
		fmt.Printf("Font does not have cmap for Unicode\n")
		return cidCMAPPosition
	}
	return cidCMAPPosition
}

func (utf *utf8FontFile) parseTables() {
	f := utf.parseNAMETable()
	utf.parseHEADTable()
	n := utf.parseHHEATable()
	w := utf.parseOS2Table()
	utf.parsePOSTTable(w)
	runeCMAPPosition := utf.parseCMAPTable(f)

	utf.SeekTable("maxp")
	utf.skip(4)
	numSymbols := utf.readUint16()

	symbolCharDictionary := make(map[int][]int)
	charSymbolDictionary := make(map[int]int)
	utf.generateSCCSDictionaries(runeCMAPPosition, symbolCharDictionary, charSymbolDictionary)

	scale := 1000.0 / float64(utf.fontElementSize)
	utf.parseHMTXTable(n, numSymbols, symbolCharDictionary, scale)
}

func (utf *utf8FontFile) generateCMAP() map[int][]int {
	cmapPosition := utf.SeekTable("cmap")
	utf.skip(2)
	cmapTableCount := utf.readUint16()
	runeCmapPosition := 0
	for range cmapTableCount {
		system := utf.readUint16()
		coder := utf.readUint16()
		position := utf.readUint32()
		oldPosition := utf.fileReader.readerPosition
		if (system == 3 && coder == 1) || system == 0 {
			format := utf.getUint16(cmapPosition + position)
			if format == 4 {
				runeCmapPosition = cmapPosition + position
				break
			}
		}
		utf.seek(int(oldPosition))
	}

	if runeCmapPosition == 0 {
		fmt.Printf("Font does not have cmap for Unicode\n")
		return nil
	}

	symbolCharDictionary := make(map[int][]int)
	charSymbolDictionary := make(map[int]int)
	utf.generateSCCSDictionaries(runeCmapPosition, symbolCharDictionary, charSymbolDictionary)

	utf.charSymbolDictionary = charSymbolDictionary

	return symbolCharDictionary
}

func (utf *utf8FontFile) parseSymbols(usedRunes map[int]int) (map[int]int, map[int]int, map[int]int, []int) {
	symbolCollection := map[int]int{0: 0}
	charSymbolPairCollection := make(map[int]int)
	for _, char := range usedRunes {
		if _, OK := utf.charSymbolDictionary[char]; OK {
			symbolCollection[utf.charSymbolDictionary[char]] = char
			charSymbolPairCollection[char] = utf.charSymbolDictionary[char]
		}
		utf.LastRune = max(utf.LastRune, char)
	}

	if utf.tableDescriptions["glyf"] == nil {
		utf.fileReader.err = errors.New("missing required font table: glyf")
		return symbolCollection, charSymbolPairCollection, nil, nil
	}
	begin := utf.tableDescriptions["glyf"].position
	glyfData := utf.getTableData("glyf")
	if utf.fileReader.err != nil {
		return symbolCollection, charSymbolPairCollection, nil, nil
	}

	symbolArray := make(map[int]int)
	symbolCollectionKeys := keySortInt(symbolCollection)

	symbolCounter := 0
	maxRune := 0
	for _, oldSymbolIndex := range symbolCollectionKeys {
		maxRune = max(maxRune, symbolCollection[oldSymbolIndex])
		symbolArray[oldSymbolIndex] = symbolCounter
		symbolCounter++
	}
	charSymbolPairCollectionKeys := keySortInt(charSymbolPairCollection)
	runeSymbolPairCollection := make(map[int]int)
	for _, runa := range charSymbolPairCollectionKeys {
		runeSymbolPairCollection[runa] = symbolArray[charSymbolPairCollection[runa]]
	}
	utf.CodeSymbolDictionary = runeSymbolPairCollection

	for _, oldSymbolIndex := range symbolCollectionKeys {
		_, symbolArray, symbolCollection, symbolCollectionKeys = utf.getSymbols(oldSymbolIndex, glyfData, &begin, symbolArray, symbolCollection, symbolCollectionKeys)
	}

	return runeSymbolPairCollection, symbolArray, symbolCollection, symbolCollectionKeys
}

func (utf *utf8FontFile) generateCMAPTable(cidSymbolPairCollection map[int]int, numSymbols int) []byte {
	cidSymbolPairCollectionKeys := keySortInt(cidSymbolPairCollection)
	cidID := 0
	cidArray := make(map[int][]int)
	prevCid := -2
	prevSymbol := -1
	for _, cid := range cidSymbolPairCollectionKeys {
		if cid == (prevCid+1) && cidSymbolPairCollection[cid] == (prevSymbol+1) {
			if n, OK := cidArray[cidID]; !OK || n == nil {
				cidArray[cidID] = make([]int, 0)
			}
			cidArray[cidID] = append(cidArray[cidID], cidSymbolPairCollection[cid])
		} else {
			cidID = cid
			cidArray[cidID] = make([]int, 0)
			cidArray[cidID] = append(cidArray[cidID], cidSymbolPairCollection[cid])
		}
		prevCid = cid
		prevSymbol = cidSymbolPairCollection[cid]
	}
	cidArrayKeys := keySortArrayRangeMap(cidArray)
	segCount := len(cidArray) + 1

	searchRange := 1
	entrySelector := 0
	for searchRange*2 <= segCount {
		searchRange *= 2
		entrySelector++
	}
	searchRange *= 2
	rangeShift := segCount*2 - searchRange
	length := 16 + (8 * segCount) + (numSymbols + 1)
	cmap := make([]byte, 0, (13+segCount*4+numSymbols+1)*2)
	for _, value := range [...]int{0, 1, 3, 1, 0, 12, 4, length, 0, segCount * 2, searchRange, entrySelector, rangeShift} {
		cmap = appendUint16BE(cmap, value)
	}

	for _, start := range cidArrayKeys {
		values := cidArray[start]
		if len(values) == 0 {
			continue
		}
		endCode := start + len(values) - 1
		cmap = appendUint16BE(cmap, endCode)
	}
	cmap = appendUint16BE(cmap, 0xFFFF)
	cmap = appendUint16BE(cmap, 0)

	for _, start := range cidArrayKeys {
		cmap = appendUint16BE(cmap, start)
	}
	cmap = appendUint16BE(cmap, 0xFFFF)
	for _, cidKey := range cidArrayKeys {
		values := cidArray[cidKey]
		if len(values) == 0 {
			continue
		}
		idDelta := -(cidKey - values[0])
		cmap = appendUint16BE(cmap, idDelta)
	}
	cmap = appendUint16BE(cmap, 1)
	for range cidArray {
		cmap = appendUint16BE(cmap, 0)
	}
	cmap = appendUint16BE(cmap, 0)
	for _, start := range cidArrayKeys {
		values := cidArray[start]
		if len(values) == 0 {
			continue
		}
		for _, value := range values {
			cmap = appendUint16BE(cmap, value)
		}
	}
	cmap = appendUint16BE(cmap, 0)
	return cmap
}

// buildUTF8StaticTables parses the immutable, font-only tables from raw
// TrueType bytes. The returned value is shared read-only across all documents
// that embed the same font.
func buildUTF8StaticTables(data []byte) (*utf8StaticTables, error) {
	reader := fileReader{array: data}
	utf := newUTF8Font(&reader)
	return utf.parseStaticTables()
}

// parseStaticTables parses the font-only tables (table directory, cmap
// dictionaries and loca glyph offsets) that do not depend on which runes a
// document uses. The result is immutable and may be shared read-only across
// documents via utf8FontFile.static, letting GenerateCutFont skip this work on
// every document. generateCMAP's returned symbol->char map is intentionally
// discarded: its only former consumer was a redundant hmtx parse that allocated
// a 512KB CharWidths array per call and was never read (the emitted width data
// comes from fontDefinition.Cw, computed once at font load time).
func (utf *utf8FontFile) parseStaticTables() (*utf8StaticTables, error) {
	utf.fileReader.readerPosition = 0
	utf.symbolPosition = make([]int, 0)
	utf.charSymbolDictionary = make(map[int]int)
	utf.tableDescriptions = make(map[string]*tableDescription)
	utf.skip(4)
	utf.generateTableDescriptions()

	utf.SeekTable("head")
	utf.skip(50)
	locaFormat := utf.readUint16()

	utf.SeekTable("hhea")
	utf.skip(34)
	oldMetrics := utf.readUint16()

	utf.SeekTable("maxp")
	utf.skip(4)
	numSymbols := utf.readUint16()

	if utf.generateCMAP() == nil {
		return nil, errors.New("font does not have a Unicode cmap table")
	}

	utf.parseLOCATable(locaFormat, numSymbols)
	if utf.fileReader.err != nil {
		return nil, utf.fileReader.err
	}

	return &utf8StaticTables{
		tableDescriptions:    utf.tableDescriptions,
		charSymbolDictionary: utf.charSymbolDictionary,
		symbolPosition:       utf.symbolPosition,
		oldMetrics:           oldMetrics,
		numSymbols:           numSymbols,
		locaFormat:           locaFormat,
	}, nil
}

// GenerateCutFont builds a font subset containing only the runes in usedRunes.
func (utf *utf8FontFile) GenerateCutFont(usedRunes map[int]int) []byte {
	cacheKey := utf.subsetCacheKey(usedRunes)
	if cached, ok := lookupUTF8SubsetCache(cacheKey); ok {
		utf.CodeSymbolDictionary = cloneIntMap(cached.codeSymbolDictionary)
		utf.LastRune = cached.lastRune
		utf.fileReader.err = nil
		return append([]byte(nil), cached.data...)
	}
	utf.fileReader.readerPosition = 0
	utf.outTablesData = make(map[string][]byte)
	utf.Ascent = 0
	utf.Descent = 0
	utf.LastRune = 0

	// Reuse shared, immutable font tables when available; otherwise parse them
	// once for this (uncached) font. parseSymbols and the assembly below only
	// read these structures, so sharing them across concurrent documents is safe.
	oldMetrics := 0
	if utf.static != nil {
		utf.tableDescriptions = utf.static.tableDescriptions
		utf.charSymbolDictionary = utf.static.charSymbolDictionary
		utf.symbolPosition = utf.static.symbolPosition
		oldMetrics = utf.static.oldMetrics
	} else {
		st, err := utf.parseStaticTables()
		if err != nil {
			return nil
		}
		oldMetrics = st.oldMetrics
	}

	cidSymbolPairCollection, symbolArray, symbolCollection, symbolCollectionKeys := utf.parseSymbols(usedRunes)

	metricsCount := len(symbolCollection)
	numSymbols := metricsCount

	utf.setOutTable("name", utf.getTableData("name"))
	utf.setOutTable("cvt ", utf.getTableData("cvt "))
	utf.setOutTable("fpgm", utf.getTableData("fpgm"))
	utf.setOutTable("prep", utf.getTableData("prep"))
	utf.setOutTable("gasp", utf.getTableData("gasp"))

	postTable := utf.getTableData("post")
	if utf.fileReader.err != nil {
		return nil
	}
	if len(postTable) < 16 {
		utf.fileReader.err = errors.New("invalid post font table")
		return nil
	}
	nextPostTable := make([]byte, 32)
	copy(nextPostTable, []byte{0x00, 0x03, 0x00, 0x00})
	copy(nextPostTable[4:16], postTable[4:16])
	postTable = nextPostTable
	utf.setOutTable("post", postTable)

	delete(cidSymbolPairCollection, 0)

	utf.setOutTable("cmap", utf.generateCMAPTable(cidSymbolPairCollection, numSymbols))

	symbolData := utf.getTableData("glyf")
	if utf.fileReader.err != nil {
		return nil
	}

	glyphDataCapacity := 0
	for _, originalSymbolIdx := range symbolCollectionKeys {
		_, symbolLen, ok := utf.glyphBounds(originalSymbolIdx, symbolData)
		if !ok {
			return nil
		}
		glyphDataCapacity += symbolLen + 3
	}

	offsets := make([]int, 0, len(symbolCollectionKeys)+1)
	glyfData := make([]byte, 0, glyphDataCapacity)
	pos := 0
	hmtxData := make([]byte, 0, len(symbolCollectionKeys)*4)
	hmtxTable := utf.getTableData("hmtx")
	if utf.fileReader.err != nil {
		return nil
	}

	for _, originalSymbolIdx := range symbolCollectionKeys {
		hmtxData = utf.appendMetricsFromTable(hmtxData, hmtxTable, oldMetrics, originalSymbolIdx)
		if utf.fileReader.err != nil {
			return nil
		}

		offsets = append(offsets, pos)
		symbolPos, symbolLen, ok := utf.glyphBounds(originalSymbolIdx, symbolData)
		if !ok {
			return nil
		}
		data := symbolData[symbolPos : symbolPos+symbolLen]
		var up int
		if symbolLen > 0 && symbolLen < 2 {
			utf.fileReader.err = errors.New("invalid glyph data length")
			return nil
		}
		if symbolLen >= 2 {
			up = unpackUint16(data[0:2])
		}

		if symbolLen > 2 && (up&(1<<15)) != 0 {
			data = append([]byte(nil), data...)
			posInSymbol := 10
			flags := symbolContinue
			nComponentElements := 0
			for (flags & symbolContinue) != 0 {
				if posInSymbol+4 > len(data) {
					utf.fileReader.err = errors.New("invalid composite glyph component")
					return nil
				}
				nComponentElements++
				up = unpackUint16(data[posInSymbol : posInSymbol+2])
				flags = up
				up = unpackUint16(data[posInSymbol+2 : posInSymbol+4])
				symbolIdx := up
				newSymbolIdx, ok := symbolArray[symbolIdx]
				if !ok {
					utf.fileReader.err = errors.New("invalid composite glyph index")
					return nil
				}
				data = utf.insertUint16(data, posInSymbol+2, newSymbolIdx)
				posInSymbol += 4
				if (flags & symbolWords) != 0 {
					posInSymbol += 4
				} else {
					posInSymbol += 2
				}
				switch {
				case (flags & symbolScale) != 0:
					posInSymbol += 2
				case (flags & symbolAllScale) != 0:
					posInSymbol += 4
				case (flags & symbol2x2) != 0:
					posInSymbol += 8
				}
			}
		}

		glyfData = append(glyfData, data...)
		pos += symbolLen
		if pos%4 != 0 {
			padding := 4 - (pos % 4)
			switch padding {
			case 1:
				glyfData = append(glyfData, 0)
			case 2:
				glyfData = append(glyfData, 0, 0)
			case 3:
				glyfData = append(glyfData, 0, 0, 0)
			}
			pos += padding
		}
	}

	offsets = append(offsets, pos)
	utf.setOutTable("glyf", glyfData)

	utf.setOutTable("hmtx", hmtxData)

	LocaFormat := 0
	if ((pos + 1) >> 1) > 0xFFFF {
		LocaFormat = 1
		locaData := make([]byte, 0, len(offsets)*4)
		for _, offset := range offsets {
			locaData = appendUint32BE(locaData, offset)
		}
		utf.setOutTable("loca", locaData)
	} else {
		LocaFormat = 0
		locaData := make([]byte, 0, len(offsets)*2)
		for _, offset := range offsets {
			locaData = appendUint16BE(locaData, offset/2)
		}
		utf.setOutTable("loca", locaData)
	}

	headData := utf.getTableData("head")
	headData = utf.insertUint16(headData, 50, LocaFormat)
	if utf.fileReader.err != nil {
		return nil
	}
	utf.setOutTable("head", headData)

	hheaData := utf.getTableData("hhea")
	hheaData = utf.insertUint16(hheaData, 34, metricsCount)
	if utf.fileReader.err != nil {
		return nil
	}
	utf.setOutTable("hhea", hheaData)

	maxp := utf.getTableData("maxp")
	maxp = utf.insertUint16(maxp, 4, numSymbols)
	if utf.fileReader.err != nil {
		return nil
	}
	utf.setOutTable("maxp", maxp)

	os2Data := utf.getTableData("OS/2")
	utf.setOutTable("OS/2", os2Data)

	out := utf.assembleTables()
	if utf.fileReader.err == nil && len(out) > 0 {
		storeUTF8SubsetCache(cacheKey, utf8SubsetCacheValue{
			data:                 append([]byte(nil), out...),
			codeSymbolDictionary: cloneIntMap(utf.CodeSymbolDictionary),
			lastRune:             utf.LastRune,
		})
	}
	return out
}

func (utf *utf8FontFile) subsetCacheKey(usedRunes map[int]int) utf8SubsetCacheKey {
	fontPtr := uintptr(unsafe.Pointer(utf))
	if utf.static != nil {
		fontPtr = uintptr(unsafe.Pointer(utf.static))
	} else if utf.fileReader != nil && len(utf.fileReader.array) > 0 {
		fontPtr = uintptr(unsafe.Pointer(&utf.fileReader.array[0]))
	}
	var hash uint64
	var hash2 uint64
	for _, char := range usedRunes {
		mixed := mixUTF8SubsetRune(uint64(char))
		hash += mixed
		hash2 ^= bits.RotateLeft64(mixed, int(mixed&63))
	}
	return utf8SubsetCacheKey{fontPtr: fontPtr, hash: hash, hash2: hash2, count: len(usedRunes)}
}

func mixUTF8SubsetRune(value uint64) uint64 {
	value += 0x9e3779b97f4a7c15
	value = (value ^ (value >> 30)) * 0xbf58476d1ce4e5b9
	value = (value ^ (value >> 27)) * 0x94d049bb133111eb
	return value ^ (value >> 31)
}

func lookupUTF8SubsetCache(key utf8SubsetCacheKey) (utf8SubsetCacheValue, bool) {
	utf8SubsetCache.Lock()
	defer utf8SubsetCache.Unlock()
	value, ok := utf8SubsetCache.entries[key]
	return value, ok
}

func storeUTF8SubsetCache(key utf8SubsetCacheKey, value utf8SubsetCacheValue) {
	utf8SubsetCache.Lock()
	defer utf8SubsetCache.Unlock()
	if _, ok := utf8SubsetCache.entries[key]; !ok {
		utf8SubsetCache.order = append(utf8SubsetCache.order, key)
	}
	utf8SubsetCache.entries[key] = value
	for len(utf8SubsetCache.order) > maxUTF8SubsetCacheEntries {
		oldest := utf8SubsetCache.order[0]
		copy(utf8SubsetCache.order, utf8SubsetCache.order[1:])
		utf8SubsetCache.order = utf8SubsetCache.order[:len(utf8SubsetCache.order)-1]
		delete(utf8SubsetCache.entries, oldest)
	}
}

func cloneIntMap(in map[int]int) map[int]int {
	if in == nil {
		return nil
	}
	out := make(map[int]int, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func (utf *utf8FontFile) getSymbols(originalSymbolIdx int, glyfData []byte, start *int, symbolSet map[int]int, symbolsCollection map[int]int, symbolsCollectionKeys []int) (*int, map[int]int, map[int]int, []int) {
	symbolPos, symbolSize, ok := utf.glyphBounds(originalSymbolIdx, glyfData)
	if !ok {
		return start, symbolSet, symbolsCollection, symbolsCollectionKeys
	}
	if symbolSize == 0 {
		return start, symbolSet, symbolsCollection, symbolsCollectionKeys
	}
	utf.seek(*start + symbolPos)

	lineCount := utf.readInt16()

	if lineCount < 0 {
		utf.skip(8)
		flags := symbolContinue
		for flags&symbolContinue != 0 {
			flags = utf.readUint16()
			symbolIndex := utf.readUint16()
			if _, OK := symbolSet[symbolIndex]; !OK {
				symbolSet[symbolIndex] = len(symbolsCollection)
				symbolsCollection[symbolIndex] = 1
				symbolsCollectionKeys = append(symbolsCollectionKeys, symbolIndex)
				oldPosition, _ := utf.fileReader.seek(0, 1)
				_, _, _, symbolsCollectionKeys = utf.getSymbols(symbolIndex, glyfData, start, symbolSet, symbolsCollection, symbolsCollectionKeys)
				utf.seek(int(oldPosition))
			}
			if flags&symbolWords != 0 {
				utf.skip(4)
			} else {
				utf.skip(2)
			}
			switch {
			case flags&symbolScale != 0:
				utf.skip(2)
			case flags&symbolAllScale != 0:
				utf.skip(4)
			case flags&symbol2x2 != 0:
				utf.skip(8)
			}
		}
	}
	return start, symbolSet, symbolsCollection, symbolsCollectionKeys
}

func (utf *utf8FontFile) glyphBounds(glyphIdx int, glyfData []byte) (pos, length int, ok bool) {
	if glyphIdx < 0 || glyphIdx+1 >= len(utf.symbolPosition) {
		utf.fileReader.err = errors.New("invalid glyph index")
		return 0, 0, false
	}
	pos = utf.symbolPosition[glyphIdx]
	next := utf.symbolPosition[glyphIdx+1]
	if pos < 0 || next < pos || next > len(glyfData) {
		utf.fileReader.err = errors.New("invalid glyph bounds")
		return 0, 0, false
	}
	return pos, next - pos, true
}

func (utf *utf8FontFile) parseHMTXTable(numberOfHMetrics, numSymbols int, symbolToChar map[int][]int, scale float64) {
	var widths int
	start := utf.SeekTable("hmtx")
	arrayWidths := 0
	utf.CharWidths = make([]int, 256*256)
	charCount := 0
	data := utf.getRange(start, numberOfHMetrics*4)
	for symbol := range numberOfHMetrics {
		arrayWidths = unpackUint16(data[symbol*4 : symbol*4+2])
		if _, OK := symbolToChar[symbol]; OK || symbol == 0 {
			if arrayWidths >= (1 << 15) {
				arrayWidths = 0
			}
			if symbol == 0 {
				utf.DefaultWidth = scale * float64(arrayWidths)
				continue
			}
			for _, char := range symbolToChar[symbol] {
				if char != 0 && char != 65535 {
					widths = int(math.Round(scale * float64(arrayWidths)))
					if widths == 0 {
						widths = 65535
					}
					if char < 196608 {
						utf.CharWidths[char] = widths
						charCount++
					}
				}
			}
		}
	}
	diff := numSymbols - numberOfHMetrics
	for pos := range diff {
		symbol := pos + numberOfHMetrics
		if _, OK := symbolToChar[symbol]; OK {
			for _, char := range symbolToChar[symbol] {
				if char != 0 && char != 65535 {
					widths = int(math.Round(scale * float64(arrayWidths)))
					if widths == 0 {
						widths = 65535
					}
					if char < 196608 {
						utf.CharWidths[char] = widths
						charCount++
					}
				}
			}
		}
	}
	utf.CharWidths[0] = charCount
}

func (utf *utf8FontFile) appendMetricsFromTable(dst, hmtx []byte, metricCount, gid int) []byte {
	if gid < metricCount {
		offset := gid * 4
		if offset < 0 || offset+4 > len(hmtx) {
			utf.fileReader.err = errors.New("invalid hmtx metric bounds")
			return dst
		}
		return append(dst, hmtx[offset:offset+4]...)
	}
	advanceOffset := (metricCount - 1) * 4
	leftSideBearingOffset := (metricCount * 2) + (gid * 2)
	if advanceOffset < 0 || advanceOffset+2 > len(hmtx) || leftSideBearingOffset < 0 || leftSideBearingOffset+2 > len(hmtx) {
		utf.fileReader.err = errors.New("invalid hmtx metric bounds")
		return dst
	}
	dst = append(dst, hmtx[advanceOffset:advanceOffset+2]...)
	return append(dst, hmtx[leftSideBearingOffset:leftSideBearingOffset+2]...)
}

func (utf *utf8FontFile) parseLOCATable(format, numSymbols int) {
	start := utf.SeekTable("loca")
	utf.symbolPosition = make([]int, 0, numSymbols+1)
	switch format {
	case 0:
		data := utf.getRange(start, (numSymbols*2)+2)
		for n := 0; n <= numSymbols; n++ {
			offset := n * 2
			utf.symbolPosition = append(utf.symbolPosition, unpackUint16(data[offset:offset+2])*2)
		}
	case 1:
		data := utf.getRange(start, (numSymbols*4)+4)
		for n := 0; n <= numSymbols; n++ {
			offset := n * 4
			utf.symbolPosition = append(utf.symbolPosition, int(binary.BigEndian.Uint32(data[offset:offset+4])))
		}
	default:
		fmt.Printf("Unknown loca table format %d\n", format)
		return
	}
}

func (utf *utf8FontFile) generateSCCSDictionaries(runeCmapPosition int, symbolCharDictionary map[int][]int, charSymbolDictionary map[int]int) {
	maxRune := 0
	utf.seek(runeCmapPosition + 2)
	size := utf.readUint16()
	rim := runeCmapPosition + size
	utf.skip(2)

	segmentSize := utf.readUint16() / 2
	utf.skip(6)
	completers := make([]int, 0)
	for range segmentSize {
		completers = append(completers, utf.readUint16())
	}
	utf.skip(2)
	beginners := make([]int, 0)
	for range segmentSize {
		beginners = append(beginners, utf.readUint16())
	}
	sizes := make([]int, 0)
	for range segmentSize {
		sizes = append(sizes, int(utf.readInt16()))
	}
	readerPositionStart := utf.fileReader.readerPosition
	positions := make([]int, 0)
	for range segmentSize {
		positions = append(positions, utf.readUint16())
	}
	var symbol int
	for n := range segmentSize {
		completePosition := completers[n] + 1
		for char := beginners[n]; char < completePosition; char++ {
			if positions[n] == 0 {
				symbol = (char + sizes[n]) & 0xFFFF
			} else {
				position := (char-beginners[n])*2 + positions[n]
				position = int(readerPositionStart) + 2*n + position
				if position >= rim {
					symbol = 0
				} else {
					symbol = utf.getUint16(position)
					if symbol != 0 {
						symbol = (symbol + sizes[n]) & 0xFFFF
					}
				}
			}
			charSymbolDictionary[char] = symbol
			if char < 196608 {
				maxRune = max(char, maxRune)
			}
			symbolCharDictionary[symbol] = append(symbolCharDictionary[symbol], char)
		}
	}
}

func (utf *utf8FontFile) assembleTables() []byte {
	tablesCount := len(utf.outTablesData)
	findSize := 1
	writer := 0
	for findSize*2 <= tablesCount {
		findSize *= 2
		writer++
	}
	findSize *= 16
	rOffset := tablesCount*16 - findSize

	tables := utf.outTablesData
	tablesNames := keySortStrings(tables)

	offset := 12 + tablesCount*16
	totalSize := offset
	for _, name := range tablesNames {
		totalSize += (len(tables[name]) + 3) &^ 3
	}
	answer := make([]byte, 0, totalSize)
	answer = appendUint32BE(answer, 0x00010000)
	answer = appendUint16BE(answer, tablesCount)
	answer = appendUint16BE(answer, findSize)
	answer = appendUint16BE(answer, writer)
	answer = appendUint16BE(answer, rOffset)
	begin := 0

	for _, name := range tablesNames {
		if name == "head" {
			begin = offset
		}
		answer = append(answer, name...)
		checksum := utf.generateChecksum(tables[name])
		answer = appendUint16BE(answer, checksum[0])
		answer = appendUint16BE(answer, checksum[1])
		answer = appendUint32BE(answer, offset)
		answer = appendUint32BE(answer, len(tables[name]))
		paddedLength := (len(tables[name]) + 3) &^ 3
		offset += paddedLength
	}

	var padding [3]byte
	for _, key := range tablesNames {
		data := tables[key]
		answer = append(answer, data...)
		if pad := ((len(data) + 3) &^ 3) - len(data); pad > 0 {
			answer = append(answer, padding[:pad]...)
		}
	}

	checksum := utf.generateChecksum(answer)
	checksum = utf.calcInt32([2]int{0xB1B0, 0xAFBA}, checksum)
	var checksumAdjustment [4]byte
	binary.BigEndian.PutUint16(checksumAdjustment[0:2], uint16(checksum[0]))
	binary.BigEndian.PutUint16(checksumAdjustment[2:4], uint16(checksum[1]))
	answer = utf.patchBytes(answer, (begin + 8), checksumAdjustment[:])
	return answer
}

func unpackUint16(data []byte) int {
	return int(binary.BigEndian.Uint16(data))
}

func appendUint16BE(dst []byte, n int) []byte {
	return append(dst, byte(n>>8), byte(n))
}

func appendUint32BE(dst []byte, n int) []byte {
	return append(dst, byte(n>>24), byte(n>>16), byte(n>>8), byte(n))
}

func keySortStrings(s map[string][]byte) []string {
	keys := make([]string, len(s))
	i := 0
	for key := range s {
		keys[i] = key
		i++
	}
	sort.Strings(keys)
	return keys
}

func keySortInt(s map[int]int) []int {
	keys := make([]int, len(s))
	i := 0
	for key := range s {
		keys[i] = key
		i++
	}
	sort.Ints(keys)
	return keys
}

func keySortArrayRangeMap(s map[int][]int) []int {
	keys := make([]int, len(s))
	i := 0
	for key := range s {
		keys[i] = key
		i++
	}
	sort.Ints(keys)
	return keys
}

// UTF8CutFont is a utility function that generates a TrueType font composed
// only of the runes included in cutset. The rune glyphs are copied from inBuf.
// This function is demonstrated in ExampleUTF8CutFont().
func UTF8CutFont(inBuf []byte, cutset string) (outBuf []byte) {
	f := newUTF8Font(&fileReader{readerPosition: 0, array: inBuf})
	runes := map[int]int{}
	for i, r := range cutset {
		runes[i] = int(r)
	}
	outBuf = f.GenerateCutFont(runes)
	return
}
