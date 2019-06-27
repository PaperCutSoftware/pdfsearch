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
// per-document data was extracted from.
// There is one DocPositions per document.
type DocPositions struct {
	blevePdf    *BlevePdf                // State of whole store.
	inPath      string                   // Path of input PDF file.
	docIdx      uint64                   // Index into blevePdf.fileList.
	pageDpl     map[uint32]PagePositions // {pageNum: locations of text on page}
	*docPersist                          // Optional extra fields for in-memory indexes.
	*docData                             // Optional extra fields for on-disk indexes.
}

// docPersist tracks the info for indexing a PDF file on disk.
type docPersist struct {
	dataFile    *os.File   // Positions are stored in this file.
	spans       []byteSpan // Indexes into `dataFile`. These is a byteSpan per page.
	dataPath    string     // Path of `dataFile`.
	spansPath   string     // Path where `spans` is saved.
	textDir     string     // !@#$ Debugging
	pageDplPath string     // !@## What is this?
}

// docData is the data for indexing a PDF file in memory.
// How is this used? !@#$
type docData struct {
	pageNums  []uint32
	pageTexts []string
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
func (lDoc *DocPositions) Equals(e *DocPositions) bool {
	if len(lDoc.pageNums) != len(e.pageNums) {
		common.Log.Error("DocPositions.Equal.pageNums: %d %d", len(lDoc.pageNums), len(e.pageNums))
		return false
	}
	if len(lDoc.pageTexts) != len(e.pageTexts) {
		common.Log.Error("DocPositions.Equal.pageTexts: %d %d", len(lDoc.pageTexts), len(e.pageTexts))
		return false
	}
	if len(lDoc.pageDpl) != len(e.pageDpl) {
		common.Log.Error("DocPositions.Equal.pageDpl: %d %d", len(lDoc.pageDpl), len(e.pageDpl))
		return false
	}
	for i, dp := range lDoc.pageNums {
		ep := e.pageNums[i]
		if dp != ep {
			common.Log.Error("DocPositions.Equal.pageNums[%d]: %d %d", i, dp, ep)
			return false
		}
	}
	for i, dt := range lDoc.pageTexts {
		et := e.pageTexts[i]
		if dt != et {
			common.Log.Error("DocPositions.Equal.pageTexts[%d]: %d %d", i, dt, et)
			return false
		}
	}
	for i, dp := range lDoc.pageDpl {
		ep, ok := e.pageDpl[i]
		if !ok || !dp.Equals(ep) {
			common.Log.Error("DocPositions.Equal.pageDpl[%d]: %t %d %d", i, ok, dp, ep)
			return false
		}
	}
	return true
}

// String returns a human readable string describing `d`.
func (lDoc DocPositions) String() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "DocPositions{%q docIdx=%d mem=%t",
		filepath.Base(lDoc.inPath), lDoc.docIdx, lDoc.isMem())
	if lDoc.docPersist != nil {
		sb.WriteString(lDoc.docPersist.String())
	}
	if lDoc.docData != nil {
		sb.WriteString(lDoc.docData.String())
	}
	sb.WriteString("}")
	return sb.String()
}

// Len returns the number of pages in `d`.
func (lDoc DocPositions) Len() int {
	return len(lDoc.pageNums)
}

// isMem returns true if `d` is an in-memory database.
// Caller must check that (lDoc.docPersist != nil) != (lDoc.docData != nil)
func (lDoc DocPositions) isMem() bool {
	lDoc.check()
	return lDoc.docData != nil
}

// check panics is `d` is an inconsistent state, which should never happen.
func (lDoc DocPositions) check() {
	persist := lDoc.docPersist != nil
	mem := lDoc.docData != nil
	if persist == mem {
		panic(fmt.Errorf("lDoc=%s should not happen\n%#v", lDoc, lDoc))
	}
	if mem {
		return
	}

	keys := lDoc.pageKeys()
	for _, pageNum := range keys {
		if pageNum == 0 {
			common.Log.Error("lDoc.check.:\n\tlDoc=%#v\n\tpageDpl=%#v", lDoc, lDoc.pageDpl)
			common.Log.Error("lDoc.check.: keys=%d %+v", len(keys), keys)
			panic(errors.New("lDoc.check.: bad pageNum"))
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

// openDoc opens `lDoc` for reading. If `lDoc` is persistent, the necessary files are opened.
func (lDoc *DocPositions) openDoc() error {
	if lDoc.isMem() {
		return nil
	}

	// Persistent case.
	f, err := os.Open(lDoc.dataPath)
	if err != nil {
		return err
	}
	lDoc.dataFile = f

	b, err := ioutil.ReadFile(lDoc.spansPath)
	if err != nil {
		return err
	}
	var spans []byteSpan
	if err := json.Unmarshal(b, &spans); err != nil {
		return err
	}
	lDoc.spans = spans

	return nil
}

// Save saves `lDoc` to disk if it peristent.
func (lDoc *DocPositions) Save() error {
	if lDoc.isMem() {
		return nil
	}

	// Persistent case.
	b, err := json.MarshalIndent(lDoc.spans, "", "\t")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(lDoc.spansPath, b, 0666)
}

// Close closees `lDoc`'s open files  if it peristent.
func (lDoc *DocPositions) Close() error {
	if lDoc.isMem() {
		return nil
	}

	// Persistent case.
	if err := lDoc.saveJsonDebug(); err != nil {
		return err
	}
	if err := lDoc.Save(); err != nil {
		return err
	}
	return lDoc.dataFile.Close()
}

// saveJsonDebug serializes `lDoc` to file `lDoc.pageDplPath` as JSON.
func (lDoc *DocPositions) saveJsonDebug() error {
	common.Log.Debug("saveJsonDebug: pageDpl=%d pageDplPath=%q", len(lDoc.pageDpl), lDoc.pageDplPath)
	var pageNums []uint32
	for p := range lDoc.pageDpl {
		pageNums = append(pageNums, uint32(p))
	}
	sort.Slice(pageNums, func(i, j int) bool { return pageNums[i] < pageNums[j] })
	common.Log.Debug("saveJsonDebug: pageNums=%+v", pageNums)
	var data []byte
	for _, pageNum := range pageNums {
		dpl, ok := lDoc.pageDpl[pageNum]
		if !ok {
			common.Log.Error("saveJsonDebug: pageNum=%d not in pageDpl", pageNum)
			return errors.New("pageNum no in pageDpl")
		}
		b, err := json.MarshalIndent(dpl, "", "\t")
		if err != nil {
			return err
		}
		common.Log.Debug("saveJsonDebug: page %d: %d bytes", pageNum, len(b))
		data = append(data, b...)
	}
	return ioutil.WriteFile(lDoc.pageDplPath, data, 0666)
}

// AddDocPage adds a page (with page number `pageNum` and contents `dpl`) to `lDoc`.
// It returns the page index, that can be used to access this page from ReadPagePositions()
// !@#$ Remove `text` param.
func (lDoc *DocPositions) AddDocPage(pageNum uint32, dpl PagePositions, text string) (
	uint32, error) {

	if pageNum == 0 {
		return 0, errors.New("pageNum=0")
	}
	lDoc.pageDpl[pageNum] = dpl

	if lDoc.isMem() {
		lDoc.docData.pageTexts = append(lDoc.docData.pageTexts, text)
		lDoc.docData.pageNums = append(lDoc.docData.pageNums, pageNum)
		return uint32(len(lDoc.docData.pageNums)) - 1, nil
	}
	return lDoc.addDocPagePersist(pageNum, dpl, text)
}

func (lDoc *DocPositions) addDocPagePersist(pageNum uint32, dpl PagePositions,
	text string) (uint32, error) {

	b := flatbuffers.NewBuilder(0)
	buf := serial.MakeDocPageLocations(b, dpl.locations)
	check := crc32.ChecksumIEEE(buf) // uint32
	offset, err := lDoc.dataFile.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, err
	}

	span := byteSpan{
		Offset:  uint32(offset),
		Size:    uint32(len(buf)),
		Check:   check,
		PageNum: uint32(pageNum),
	}

	if _, err := lDoc.dataFile.Write(buf); err != nil {
		return 0, err
	}

	lDoc.spans = append(lDoc.spans, span)
	pageIdx := uint32(len(lDoc.spans) - 1)

	filename := lDoc.textPath(pageIdx)
	err = ioutil.WriteFile(filename, []byte(text), 0644)
	if err != nil {
		return 0, err
	}
	return pageIdx, err
}

// !@#$ Needed?
func (lDoc *DocPositions) pageText(pageIdx uint32) (string, error) {
	if lDoc.isMem() {
		return lDoc.pageTexts[pageIdx], nil
	}
	return lDoc.readPersistedPageText(pageIdx)
}

func (lDoc *DocPositions) readPersistedPageText(pageIdx uint32) (string, error) {
	filename := lDoc.textPath(pageIdx)
	b, err := ioutil.ReadFile(filename)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// pagePositions returns the PagePositions of the text on the `pageIdx` (0-offset)
// returned text in document `lDoc`.
func (lDoc *DocPositions) pagePositions(pageIdx uint32) (uint32, PagePositions, error) {
	if lDoc.isMem() {
		if pageIdx >= uint32(len(lDoc.pageNums)) {
			return 0, PagePositions{}, fmt.Errorf("bad pageIdx=%d lDoc=%s", pageIdx, lDoc)
		}
		common.Log.Debug("ReadPagePositions: pageIdx=%d pageNums=%d %+v", pageIdx, len(lDoc.pageNums),
			lDoc.pageNums)
		pageNum := lDoc.pageNums[pageIdx]
		if pageNum == 0 {
			return 0, PagePositions{}, fmt.Errorf("No pageNum. lDoc=%s", lDoc)
		}
		dpl, ok := lDoc.pageDpl[pageNum]
		if !ok {
			common.Log.Error("ReadPagePositions: pageIdx=%d pageNum=%d lDoc=%s",
				pageIdx, pageNum, lDoc)
			common.Log.Error("ReadPagePositions: pageNums=%d %+v", len(lDoc.pageNums), lDoc.pageNums)
			keys := lDoc.pageKeys()
			common.Log.Error("ReadPagePositions: keys=%d %+v", len(lDoc.pageDpl), keys)
			return 0, PagePositions{}, errors.New("pageNum not in pageDpl")
		}
		if len(dpl.locations) == 0 {
			common.Log.Error("ReadPagePositions: pageIdx=%d pageNum=%d lDoc=%s",
				pageIdx, pageNum, lDoc)
			return 0, PagePositions{}, errors.New("no locations")
		}
		return pageNum, dpl, nil
	}
	return lDoc.readPersistedPagePositions(pageIdx)
}

// pageKeys returns the `lDoc.pageDpl` keys.
func (lDoc *DocPositions) pageKeys() []int {
	var keys []int
	for k := range lDoc.pageDpl {
		keys = append(keys, int(k))
	}
	sort.Ints(keys)
	return keys
}

func (lDoc *DocPositions) readPersistedPagePositions(pageIdx uint32) (
	uint32, PagePositions, error) {

	e := lDoc.spans[pageIdx]
	if e.PageNum == 0 {
		return 0, PagePositions{}, fmt.Errorf("Bad span pageIdx=%d e=%+v", pageIdx, e)
	}

	offset, err := lDoc.dataFile.Seek(int64(e.Offset), io.SeekStart)
	if err != nil || uint32(offset) != e.Offset {
		common.Log.Error("ReadPagePositions: Seek failed e=%+v offset=%d err=%v",
			e, offset, err)
		return 0, PagePositions{}, err
	}
	buf := make([]byte, e.Size)
	if _, err := lDoc.dataFile.Read(buf); err != nil {
		return 0, PagePositions{}, err
	}
	size := len(buf)
	check := crc32.ChecksumIEEE(buf)
	if check != e.Check {
		common.Log.Error("ReadPagePositions: e=%+v size=%d check=%d", e, size, check)
		return 0, PagePositions{}, errors.New("bad checksum")
	}
	locations, err := serial.ReadDocPageLocations(buf)
	return e.PageNum, PagePositions{locations}, err
}

// textPath returns the path to the file holding the extracted text of the page with index `pageIdx`.
func (lDoc *DocPositions) textPath(pageIdx uint32) string {
	return filepath.Join(lDoc.textDir, fmt.Sprintf("%03d.txt", pageIdx))
}

// DocPageText contains doc:page indexes, the PDF page number and the text extracted from from a PDF
// page.
type DocPageText struct {
	DocIdx  uint64 // Doc index (0-offset) into BlevePdf.fileList .
	PageIdx uint32 // Page index (0-offset) into DocPositions.index .
	PageNum uint32 // Page number in PDF file (1-offset)
	Text    string // Extracted page text.
}
