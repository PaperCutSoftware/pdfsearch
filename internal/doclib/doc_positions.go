// Copyright 2019 PaperCut Software International Pty Ltd. All rights reserved.

package doclib

import (
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"

	flatbuffers "github.com/google/flatbuffers/go"
	"github.com/papercutsoftware/pdfsearch/internal/serial"
	"github.com/unidoc/unipdf/v3/common"
)

// DocPositions is used to the link per-document data in a bleve index to the PDF file that the
// data was extracted from.
// There is one DocPositions per PDF file.
type DocPositions struct {
	blevePdf      *BlevePdf                // State of whole store.
	inPath        string                   // Path of input PDF file.
	docIdx        uint64                   // Index into blevePdf.fileList.
	pagePositions map[uint32]PagePositions // {pageNum: locations of text on page}
	*docPersist                            // Optional extra fields for on-disk indexes.
	*docData                               // Optional extra fields for in-memory indexes.
}

// docPersist tracks the info for indexing a PDF file on disk.
type docPersist struct {
	dataFile          *os.File   // Positions are stored in this file.
	spans             []byteSpan // Indexes into `dataFile`. These is a byteSpan per page.
	dataPath          string     // Path of `dataFile`.
	spansPath         string     // Path where `spans` is saved.
	textDir           string     // Used for debugging
	pagePositionsPath string     // !@## What is this?
}

// docData is the data for indexing a PDF file in memory.
// TODO: This is now only informational. Remove.
type docData struct {
	pageNums  []uint32 // (1-offset) PDF page numbers.
	pageTexts []string // extracted text for pages.
}

// byteSpan is the location of the bytes of a PagePositions in a data file.
// The span is over [Offset, Offset+Size).
// There is one byteSpan (corresponding to a PagePositions) per page.
type byteSpan struct {
	Offset  uint32 // Offset in the data file for the PagePositions for a page.
	Size    uint32 // Size of the PagePositions in the data file.
	Check   uint32 // CRC checksum for the PagePositions data.
	PageNum uint32 // PDF page number.
}

// Equals returns true if `d` contains the same information as `e`.
func (docPos *DocPositions) Equals(e *DocPositions) bool {
	if len(docPos.pageNums) != len(e.pageNums) {
		common.Log.Error("DocPositions.Equal.pageNums: %d %d", len(docPos.pageNums), len(e.pageNums))
		return false
	}
	if len(docPos.pageTexts) != len(e.pageTexts) {
		common.Log.Error("DocPositions.Equal.pageTexts: %d %d", len(docPos.pageTexts), len(e.pageTexts))
		return false
	}
	if len(docPos.pagePositions) != len(e.pagePositions) {
		common.Log.Error("DocPositions.Equal.pagePositions: %d %d", len(docPos.pagePositions), len(e.pagePositions))
		return false
	}
	for i, dp := range docPos.pageNums {
		ep := e.pageNums[i]
		if dp != ep {
			common.Log.Error("DocPositions.Equal.pageNums[%d]: %d %d", i, dp, ep)
			return false
		}
	}
	for i, dt := range docPos.pageTexts {
		et := e.pageTexts[i]
		if dt != et {
			common.Log.Error("DocPositions.Equal.pageTexts[%d]: %d %d", i, dt, et)
			return false
		}
	}
	for i, dp := range docPos.pagePositions {
		ep, ok := e.pagePositions[i]
		if !ok || !dp.Equals(ep) {
			common.Log.Error("DocPositions.Equal.pagePositions[%d]: %t %d %d", i, ok, dp, ep)
			return false
		}
	}
	return true
}

// String returns a human readable string describing `d`.
func (docPos DocPositions) String() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "DocPositions{%q docIdx=%d mem=%t",
		filepath.Base(docPos.inPath), docPos.docIdx, docPos.isMem())
	if docPos.docPersist != nil {
		sb.WriteString(docPos.docPersist.String())
	}
	if docPos.docData != nil {
		sb.WriteString(docPos.docData.String())
	}
	sb.WriteString("}")
	return sb.String()
}

// Len returns the number of pages in `d`.
func (docPos DocPositions) Len() int {
	return len(docPos.pageNums)
}

// isMem returns true if `d` is an in-memory database.
// Caller must check that (docPos.docPersist != nil) != (docPos.docData != nil)
func (docPos DocPositions) isMem() bool {
	docPos.check()
	return docPos.docData != nil
}

// check panics is `d` is an inconsistent state, which should never happen.
func (docPos DocPositions) check() {
	persist := docPos.docPersist != nil
	mem := docPos.docData != nil
	if persist == mem {
		panic(fmt.Errorf("docPos=%s should not happen\n%#v", docPos, docPos))
	}
	if mem {
		return
	}

	keys := docPos.pageKeys()
	for _, pageNum := range keys {
		if pageNum == 0 {
			common.Log.Error("docPos.check.:\n\tlDoc=%#v\n\tpagePositions=%#v", docPos,
				docPos.pagePositions)
			common.Log.Error("docPos.check.: keys=%d %+v", len(keys), keys)
			panic(errors.New("docPos.check.: bad pageNum"))
		}
	}
}

// String returns a human readable string describing docPersist `d`.
func (d docPersist) String() string {
	var parts []string
	for i, span := range d.spans {
		parts = append(parts, fmt.Sprintf("\t%2d: %v", i+1, span))
	}
	return fmt.Sprintf("docPersist{%s}", strings.Join(parts, "\n"))
}

// String returns a human readable string describing docData `d`.
func (d docData) String() string {
	np := len(d.pageNums)
	nt := len(d.pageTexts)
	bad := ""
	if np != nt {
		bad = " [BAD]"
	}
	return fmt.Sprintf("docData{pageNums=%d pageTexts=%d%s}", np, nt, bad)
}

// openDoc opens `docPos` for reading. If `docPos` is persistent, the necessary files are opened.
func (docPos *DocPositions) openDoc() error {
	if docPos.isMem() {
		return nil
	}

	// Persistent case.
	f, err := os.Open(docPos.dataPath)
	if err != nil {
		return err
	}
	docPos.dataFile = f

	b, err := ioutil.ReadFile(docPos.spansPath)
	if err != nil {
		return err
	}
	var spans []byteSpan
	if err := json.Unmarshal(b, &spans); err != nil {
		return err
	}
	docPos.spans = spans

	return nil
}

// Save saves `docPos` to disk if it peristent.
func (docPos *DocPositions) Save() error {
	if docPos.isMem() {
		return nil
	}

	// Persistent case.
	b, err := json.MarshalIndent(docPos.spans, "", "\t")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(docPos.spansPath, b, 0666)
}

// Close closes `docPos`'s open files if it peristent.
func (docPos *DocPositions) Close() error {
	if docPos.isMem() {
		return nil
	}

	// Persistent case.
	if err := docPos.saveJsonDebug(); err != nil {
		return err
	}
	if err := docPos.Save(); err != nil {
		return err
	}
	return docPos.dataFile.Close()
}

// saveJsonDebug serializes `docPos` to file `docPos.pagePositionsPath` as JSON.
// TODO: Remove from production code?
func (docPos *DocPositions) saveJsonDebug() error {
	common.Log.Debug("saveJsonDebug: pagePositions=%d pagePositionsPath=%q", len(docPos.pagePositions),
		docPos.pagePositionsPath)
	var pageNums []uint32
	for p := range docPos.pagePositions {
		pageNums = append(pageNums, uint32(p))
	}
	sort.Slice(pageNums, func(i, j int) bool { return pageNums[i] < pageNums[j] })
	common.Log.Debug("saveJsonDebug: pageNums=%+v", pageNums)
	var data []byte
	for _, pageNum := range pageNums {
		ppos, ok := docPos.pagePositions[pageNum]
		if !ok {
			common.Log.Error("saveJsonDebug: pageNum=%d not in pagePositions", pageNum)
			return errors.New("pageNum no in pagePositions")
		}
		b, err := json.MarshalIndent(ppos, "", "\t")
		if err != nil {
			return err
		}
		common.Log.Debug("saveJsonDebug: page %d: %d bytes", pageNum, len(b))
		data = append(data, b...)
	}
	return ioutil.WriteFile(docPos.pagePositionsPath, data, 0666)
}

// AddDocPage adds a page with (1-offset) page number `pageNum` and contents `ppos` to `docPos`.
// It returns the page index, that can be used to access this page from ReadPagePositions()
// TODO: Can we remove `text` param for production code? By the time this function is called we have
// already indexed the test.
func (docPos *DocPositions) AddDocPage(pageNum uint32, ppos PagePositions, text string) (
	uint32, error) {
	if pageNum == 0 {
		return 0, errors.New("pageNum=0")
	}
	docPos.pagePositions[pageNum] = ppos

	if docPos.isMem() {
		docPos.docData.pageTexts = append(docPos.docData.pageTexts, text)
		docPos.docData.pageNums = append(docPos.docData.pageNums, pageNum)
		return uint32(len(docPos.docData.pageNums)) - 1, nil
	}
	return docPos.addDocPagePersist(pageNum, ppos, text)
}

func (docPos *DocPositions) addDocPagePersist(pageNum uint32, ppos PagePositions, text string) (uint32,
	error) {
	b := flatbuffers.NewBuilder(0)
	buf := serial.MakeDocPageLocations(b, ppos.offsetBBoxes)
	check := crc32.ChecksumIEEE(buf) // uint32
	offset, err := docPos.dataFile.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, err
	}

	span := byteSpan{
		Offset:  uint32(offset),
		Size:    uint32(len(buf)),
		Check:   check,
		PageNum: uint32(pageNum),
	}

	if _, err := docPos.dataFile.Write(buf); err != nil {
		return 0, err
	}

	docPos.spans = append(docPos.spans, span)
	pageIdx := uint32(len(docPos.spans) - 1)

	filename := docPos.textPath(pageIdx)
	err = ioutil.WriteFile(filename, []byte(text), 0644)
	if err != nil {
		return 0, err
	}
	return pageIdx, err
}

// pageText returns the text extracted for page with in `docPos` with page index `pageIdx`.
// TODO: Can we remove this? It seems to be called after the extracted text is indexed.
func (docPos *DocPositions) pageText(pageIdx uint32) (string, error) {
	if docPos.isMem() {
		return docPos.pageTexts[pageIdx], nil
	}
	return docPos.readPersistedPageText(pageIdx)
}

// readPersistedPageText returns the text extracted for page with in `docPos` with page index
// `pageIdx` for a persisted index.
// TODO: Can we remove this? See pageText().
func (docPos *DocPositions) readPersistedPageText(pageIdx uint32) (string, error) {
	filename := docPos.textPath(pageIdx)
	b, err := ioutil.ReadFile(filename)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// pageNumPositions returns the page number (1-offset) and PagePositions of the text on the `pageIdx`
// (0-offset) in `docPos`.
func (docPos *DocPositions) pageNumPositions(pageIdx uint32) (uint32, PagePositions, error) {
	if docPos.isMem() {
		if pageIdx >= uint32(len(docPos.pageNums)) {
			return 0, PagePositions{}, fmt.Errorf("bad pageIdx=%d docPos=%s", pageIdx, docPos)
		}
		common.Log.Debug("ReadPagePositions: pageIdx=%d pageNums=%d %+v", pageIdx, len(docPos.pageNums),
			docPos.pageNums)
		pageNum := docPos.pageNums[pageIdx]
		if pageNum == 0 {
			return 0, PagePositions{}, fmt.Errorf("No pageNum. docPos=%s", docPos)
		}
		ppos, ok := docPos.pagePositions[pageNum]
		if !ok {
			common.Log.Error("ReadPagePositions: pageIdx=%d pageNum=%d docPos=%s",
				pageIdx, pageNum, docPos)
			common.Log.Error("ReadPagePositions: pageNums=%d %+v", len(docPos.pageNums), docPos.pageNums)
			keys := docPos.pageKeys()
			common.Log.Error("ReadPagePositions: keys=%d %+v", len(docPos.pagePositions), keys)
			return 0, PagePositions{}, errors.New("pageNum not in pagePositions")
		}
		if len(ppos.offsetBBoxes) == 0 {
			common.Log.Error("ReadPagePositions: pageIdx=%d pageNum=%d docPos=%s",
				pageIdx, pageNum, docPos)
			return 0, PagePositions{}, errors.New("no locations")
		}
		return pageNum, ppos, nil
	}
	return docPos.readPersistedPagePositions(pageIdx)
}

// pageKeys returns the `docPos.pagePositions` keys.
func (docPos *DocPositions) pageKeys() []int {
	var keys []int
	for k := range docPos.pagePositions {
		keys = append(keys, int(k))
	}
	sort.Ints(keys)
	return keys
}

func (docPos *DocPositions) readPersistedPagePositions(pageIdx uint32) (
	uint32, PagePositions, error) {
	e := docPos.spans[pageIdx]
	if e.PageNum == 0 {
		return 0, PagePositions{}, fmt.Errorf("Bad span pageIdx=%d e=%+v", pageIdx, e)
	}

	offset, err := docPos.dataFile.Seek(int64(e.Offset), io.SeekStart)
	if err != nil || uint32(offset) != e.Offset {
		common.Log.Error("ReadPagePositions: Seek failed e=%+v offset=%d err=%v",
			e, offset, err)
		return 0, PagePositions{}, err
	}
	buf := make([]byte, e.Size)
	if _, err := docPos.dataFile.Read(buf); err != nil {
		return 0, PagePositions{}, err
	}
	size := len(buf)
	check := crc32.ChecksumIEEE(buf)
	if check != e.Check {
		common.Log.Error("readPersistedPagePositions: e=%+v size=%d check=%d", e, size, check)
		return 0, PagePositions{}, errors.New("bad checksum")
	}
	locations, err := serial.ReadDocPageLocations(buf)
	return e.PageNum, PagePositions{locations}, err
}

// textPath returns the path to the file holding the extracted text of the page with index `pageIdx`.
func (docPos *DocPositions) textPath(pageIdx uint32) string {
	return filepath.Join(docPos.textDir, fmt.Sprintf("%03d.txt", pageIdx))
}

// DocPageText contains doc:page indexes, the PDF page number and the text extracted from a PDF page.
type DocPageText struct {
	DocIdx  uint64 // Doc index (0-offset) into BlevePdf.fileList .
	PageIdx uint32 // Page index (0-offset) into DocPositions.index .
	PageNum uint32 // Page number in PDF file (1-offset)
	Text    string // Extracted page text.
}
