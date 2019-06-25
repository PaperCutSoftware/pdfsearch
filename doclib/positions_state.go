// Copyright 2019 PaperCut Software International Pty Ltd. All rights reserved.

/*
   What is the purpose of this source file !@#$
 */
package doclib

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/blevesearch/bleve"
	"github.com/papercutsoftware/pdfsearch/base"
	"github.com/papercutsoftware/pdfsearch/serial"
	"github.com/unidoc/unipdf/v3/common"
	pdf "github.com/unidoc/unipdf/v3/model"
)

var ErrRange = errors.New("out of range")

// FileDesc describes a PDF file.
type FileDesc struct {
	InPath string  // Full path to PDF file.
	Hash   string  // SHA-256 hash of file contents.
	SizeMB float64 // Size of PDF file on disk.
}

// String returns a human readable description of `fd`.
func (fd FileDesc) String() string {
	return fmt.Sprintf("{FileDesc: %#q %.2f MB %q}", fd.Hash, fd.SizeMB, fd.InPath)
}

// IndexPdfFiles creates a bleve+PositionsState index for `pathList`.
// If `persistDir` is not empty, the index is written to this directory.
// If `forceCreate` is true and `persistDir` is not empty, a new directory is always created.
// If `allowAppend` is true and `persistDir` is not empty and a bleve index already exists on disk
// then the bleve index will be appended to.
// `report` is a supplied function that is called to report progress.
// TODO: Remove `allowAppend` argument. Instead always append to a bleve index if it exists and
//      `forceCreate` is not set.
func IndexPdfFiles(pathList []string, persistDir string, forceCreate, allowAppend bool,
	report func(string)) (*PositionsState, bleve.Index, int, time.Duration, time.Duration, error) {

	var rsList []io.ReadSeeker
	for _, inPath := range pathList {
		rs, err := os.Open(inPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Opened %d files\n", len(rsList))
			break
		}
		defer rs.Close()
		rsList = append(rsList, rs)
	}
	return IndexPdfReaders(pathList, rsList, persistDir, forceCreate, allowAppend, report)
}

// IndexPdfReaders returns a PositionsState and a bleve.Index over the PDF contents read by the
// io.ReaderSeeker's in `rsList`.
// The names of the PDFs are in the corresponding position in `pathList`.
// The inde`persistDir
// If `persist` is false, the index is stored in memory.
// If `persist` is true, the index is stored on disk in `persistDir`.
// `report` is a supplied function that is called to report progress.
func IndexPdfReaders(pathList []string, rsList []io.ReadSeeker, persistDir string, forceCreate,
	allowAppend bool, report func(string)) (*PositionsState, bleve.Index,
	int, time.Duration, time.Duration, error) {

	common.Log.Info("Indexing %d PDF files.", len(pathList))
	var dtPdf, dtBleve, dtP, dtB time.Duration

	lState, err := OpenPositionsState(persistDir, forceCreate)
	if err != nil {
		return nil, nil, 0, dtPdf, dtBleve, fmt.Errorf("Could not create positions store %q. err=%v", persistDir, err)
	}
	defer lState.Flush()
	lState.Check()

	var index bleve.Index
	if len(persistDir) == 0 {
		index, err = CreateBleveMemIndex()
		if err != nil {
			return nil, nil, 0, dtPdf, dtBleve, fmt.Errorf("Could not create Bleve memoryindex. err=%v", err)
		}
	} else {
		indexPath := filepath.Join(persistDir, "bleve")
		common.Log.Info("indexPath=%q", indexPath)
		// Create a new Bleve index.
		index, err = CreateBleveIndex(indexPath, forceCreate, allowAppend)
		if err != nil {
			return nil, nil, 0, dtPdf, dtBleve, fmt.Errorf("Could not create Bleve index in %q", indexPath)
		}
	}

	totalPages := 0
	// Add the pages of all the PDFs in `pathList` to `index`.
	for i, inPath := range pathList {
		readerOnly := ""
		if len(rsList) > 0 {
			readerOnly = " (readerOnly)"
		}
		if report != nil {
			report(fmt.Sprintf("%3d of %d: %q%s", i+1, len(pathList), inPath, readerOnly))
		}
		lState.Check()
		var err error
		if len(rsList) > 0 {
			lState.Check()
			rs := rsList[i]
			dtP, dtB, err = lState.indexDocPagesLocReader(index, inPath, rs)
			dtPdf += dtP
			dtBleve += dtB
			fmt.Fprintf(os.Stderr, "***1 %d: (%.3f %.3f) (%.3f %.3f)\n", i,
				dtPdf.Seconds(), dtP.Seconds(), dtBleve.Seconds(), dtB.Seconds())
		} else {
			dtP, dtB, err = lState.indexDocPagesLocFile(index, inPath)
			dtPdf += dtP
			dtBleve += dtB
		}
		lState.Check()
		if err != nil {
			return nil, nil, 0, dtPdf, dtBleve, fmt.Errorf("Could not index file %q", inPath)
		}
		lState.Check()
		docCount, err := index.DocCount()
		if err != nil {
			return nil, nil, 0, dtPdf, dtBleve, err
		}
		common.Log.Debug("Indexed %q. Total %d pages indexed.", inPath, docCount)
		totalPages += int(docCount)
	}

	return lState, index, totalPages, dtPdf, dtBleve, err
}
type IDText struct {
	ID   string
	Text string
}

// indexDocPagesLocFile adds the text of all the pages in PDF file `inPath` to Bleve index `index`.
func (lState *PositionsState) indexDocPagesLocFile(index bleve.Index, inPath string) (
	dtPdf, dtBleve time.Duration, err error) {
	rs, err := os.Open(inPath)
	if err != nil {
		return dtPdf, dtBleve, err
	}
	defer rs.Close()
	return lState.indexDocPagesLocReader(index, inPath, rs)
}

// indexDocPagesLocReader updates `index` and `lState` with the text positions of the text in the
// PDF file accessed by `rs`. `inPath` is the name of the PDF file.
func (lState *PositionsState) indexDocPagesLocReader(index bleve.Index, inPath string,
	rs io.ReadSeeker) (dtPdf, dtBleve time.Duration, err error) {

	t0 := time.Now()
	docPages, err := lState.extractDocPagePositionsReader(inPath, rs)
	if err != nil {
		common.Log.Error("indexDocPagesLocReader: Couldn't extract pages from %q err=%v", inPath, err)
		lState.Check()
		return dtPdf, dtBleve, err
	}
	dtPdf = time.Since(t0)
	common.Log.Debug("indexDocPagesLocReader: inPath=%q docPages=%d", inPath, len(docPages))
	lState.Check()

	t0 = time.Now()
	for i, l := range docPages {
		// Don't weigh down the Bleve index with the text bounding boxes.
		id := fmt.Sprintf("%04X.%d", l.DocIdx, l.PageIdx)
		idText := IDText{ID: id, Text: l.Text}

		err = index.Index(id, idText)
		dt := time.Since(t0)
		if err != nil {
			lState.Check()
			return dtPdf, dtBleve, err
		}
		if i%100 == 0 {
			common.Log.Debug("\tIndexed %2d of %d pages in %5.1f sec (%.2f sec/page)",
				i+1, len(docPages), dt.Seconds(), dt.Seconds()/float64(i+1))
			common.Log.Debug("\tid=%q text=%d", id, len(idText.Text))
		}
		lState.Check()
	}
	dtBleve = time.Since(t0)
	dt := dtPdf + dtBleve
	common.Log.Debug("\tIndexed %d pages in %.1f (Pdf) + %.1f (bleve) = %.1f sec (%.3f sec/page)\n",
		len(docPages), dtPdf.Seconds(), dtBleve.Seconds(), dt.Seconds(), dt.Seconds()/float64(len(docPages)))
	lState.Check()
	return dtPdf, dtBleve, err
}

/*
   PositionsState is for serializing and accessing DocPageLocations.

   Positions are read from disk a page at a time by ReadPositions which returns the
   []DocPageLocations for the PDF page given by `doc` and `page`.

   func (lState *PositionsState) ReadPositions(doc uint64, page uint32) ([]DocPageLocations, error)

   We use this to allow an efficient look up of DocPageLocation of an offset within a page's text.
   1) Look up []DocPageLocations for the PDF page given by `doc` and `page`
   2) Binary search []DocPageLocations to find location for `offset`.

   Persistent storage
   -----------------
   1 data file + 1 index file per document.
   index file is small and contains offsets of pages in data file. It is made up of
     byteSpan (12 byte data structure)
         offset uint32
         size   uint32
         check  uint32

   <root>/
      file_list.json
      positions/
          <hash1>.dat
          <hash1>.idx
          <hash1>.pages
              <page1>.txt
              <page2>.txt
              ...
          <hash2>.dat
          <hash2>.idx
          <hash2>.pages
              <page1>.txt
              <page2>.txt
              ...
          ...
*/

const storeUpdatePeriodSec = 60.0

// PositionsState links a bleve index to the PDF files that the per-document data in the bleve index
// was extracted from
// A PositionsState can be optionally saved to and retreived from disk, in which case isMem()
// returns true.
type PositionsState struct {
	root       string                   // Top level directory of the data saved to disk. "" for in-memory.
	fileList   []FileDesc               // List of FileDesc for PDFs the indexed data was extracted from.
	hashIndex  map[string]uint64        // {file hash: index into fileList}
	hashPath   map[string]string        // {file hash: file path}
	hashDoc    map[string]*DocPositions // {file hash: DocPositions} Links to positions of extracted text to location on PDF page.
	indexHash  map[uint64]string        // Reverse map of hashIndex.
	updateTime time.Time                // Time of last Flush()
}

// Equals returns true if `l` contains the same information as `m`.
func (l *PositionsState) Equals(m *PositionsState) bool {
	for hash, ldoc := range l.hashDoc {
		mdoc, ok := m.hashDoc[hash]
		if !ok {
			common.Log.Error("PositionsState.Equal.hash=%#q", hash)
			return false
		}
		if !ldoc.Equals(mdoc) {
			common.Log.Error("PositionsState.Equal.doc hash=%#q\n%s\n%s", hash, ldoc, mdoc)
			return false
		}
	}
	return true
}

func (l PositionsState) String() string {
	var parts []string
	parts = append(parts,
		fmt.Sprintf("%q fileList=%d hashIndex=%d indexHash=%d hashPath=%d hashDoc=%d %s",
			l.root, len(l.fileList), len(l.hashIndex), len(l.indexHash), len(l.hashPath),
			len(l.hashDoc), l.updateTime))
	for k, lDoc := range l.hashDoc {
		parts = append(parts, fmt.Sprintf("%q: %d", k, lDoc.Len()))
	}
	return fmt.Sprintf("{PositionsState: %s}", strings.Join(parts, "\t"))
}

// Len returns the number of reachable documents (and their corresponding PDF file contents) in `l`.
func (l PositionsState) Len() int {
	return len(l.hashIndex)
}

// isMem returns true if `l` is stored in memory or false if `l` is stored on disk.
func (l PositionsState) isMem() bool {
	return l.root == ""
}

// remove deletes all the PositionsState map entries with key `hash` as well as the corresponding
// reverse map entry in hashIndex.
// NOTE: l.remove(hash) leaves l.fileList[l.hashIndex[hash]] with no references to it, so we waste
//       a small amount of memory that we don't care about.
func (l *PositionsState) remove(hash string) {
	if index, ok := l.hashIndex[hash]; ok {
		delete(l.indexHash, index)
	}
	delete(l.hashIndex, hash)
	delete(l.hashPath, hash)
	delete(l.hashDoc, hash)
}

func (l PositionsState) Check() error {
	if len(l.fileList) == 0 || len(l.hashIndex) == 0 || len(l.indexHash) == 0 ||
		len(l.hashPath) == 0 || len(l.hashDoc) == 0 {
		return fmt.Errorf("bad PositionsState: %s", l)
	}
	for _, lDoc := range l.hashDoc {
		if err := lDoc.Check(); err != nil {
			return err
		}
	}
	var keys []string
	for h := range l.hashDoc {
		keys = append(keys, h)
	}
	sort.Strings(keys)
	var keyt []string
	for h := range l.hashIndex {
		keyt = append(keyt, h)
	}
	sort.Strings(keyt)

	for h, i := range l.hashIndex {
		if hh, ok := l.indexHash[i]; !ok {
			common.Log.Info("%#q\nhashIndex:%d %+v",
				h, len(keyt), keyt)
			panic("PositionsState.Check:1")
		} else if hh != h {
			common.Log.Info("hash=%q indexHash=%#q index=%d\nhashIndex:%d %+v",
				h, hh, i, len(keyt), keyt)
			panic("PositionsState.Check:2")
		}
		if _, ok := l.hashPath[h]; !ok {
			common.Log.Info("%#q\nhashDoc  :%d %+v\nhashIndex:%d %+v",
				h, len(keys), keys, len(keyt), keyt)
			panic("PositionsState.Check:3")
		}
		if _, ok := l.hashDoc[h]; !ok {
			common.Log.Info("%#q\nhashDoc  :%d %+v\nhashIndex:%d %+v",
				h, len(keys), keys, len(keyt), keyt)
			panic("PositionsState.Check:4")
		}
	}
	return nil
}

// PositionsStateFromHIPDs converts `hipds` to a PositionsState.
// !@#$ Round trip test PositionsStateFromHIPDs + ToHIPDs
func PositionsStateFromHIPDs(hipds []serial.HashIndexPathDoc) (PositionsState, error) {
	l := PositionsState{
		hashIndex: map[string]uint64{},
		indexHash: map[uint64]string{},
		hashPath:  map[string]string{},
		hashDoc:   map[string]*DocPositions{},
	}
	for _, h := range hipds {
		hash := h.Hash
		idx := h.Index
		path := h.Path
		sdoc := h.Doc

		common.Log.Debug("PositionsStateFromHIPDs: sdoc.PageNums=%d sdoc.PageDpl=%d",
			len(sdoc.PageNums), len(sdoc.PageDpl))
		// sdoc.PageDpl is a slice with entries corresponding to page numbers in sdoc.PageNums
		pageDpl := map[uint32]base.DocPageLocations{}
		for i, pageNum := range sdoc.PageNums {
			pageDpl[pageNum] = sdoc.PageDpl[i]
		}
		lDoc := DocPositions{
			inPath:  sdoc.Path,   // Path of input PDF file.
			docIdx:  sdoc.DocIdx, // Index into lState.fileList.
			pageDpl: pageDpl,
			docData: &docData{
				pageNums:  sdoc.PageNums,
				pageTexts: sdoc.PageTexts,
			},
		}
		if err := lDoc.Check(); err != nil {
			return PositionsState{}, err
		}
		l.hashPath[hash] = path
		l.hashDoc[hash] = &lDoc
		l.hashIndex[hash] = idx
		l.indexHash[idx] = hash

		desc := FileDesc{
			InPath: path,
			Hash:   hash,
		}
		l.fileList = append(l.fileList, desc)
	}
	if err := l.Check(); err != nil {
		return PositionsState{}, err
	}
	return l, nil
}

// ToHIPDs converts `l` to a serial.HashIndexPathDoc.
// l.Check() is run before saving to avoid empty serializations.
func (l PositionsState) ToHIPDs() ([]serial.HashIndexPathDoc, error) {
	if err := l.Check(); err != nil {
		return nil, err
	}
	var hipds []serial.HashIndexPathDoc
	for hash, idx := range l.hashIndex {
		path, ok := l.hashPath[hash]
		if !ok {
			panic(fmt.Errorf("%#q not in hashPath", hash))
		}
		doc, ok := l.hashDoc[hash]
		if !ok {
			var keys []string
			for h := range l.hashDoc {
				keys = append(keys, h)
			}
			sort.Strings(keys)
			var keyt []string
			for h := range l.hashIndex {
				keyt = append(keyt, h)
			}
			sort.Strings(keyt)
			panic(fmt.Errorf("%#q not in hashDoc.\nhashDoc  :%d %+v\nhashIndex:%d %+v",
				hash, len(keys), keys, len(keyt), keyt))
		}
		// sdoc.PageDpl is a slice with entries corresponding to page numbers in sdoc.PageNums
		common.Log.Trace("doc.pageDpl=%d", len(doc.pageDpl))
		pageDpl := make([]base.DocPageLocations, len(doc.pageDpl))
		for i, pageNum := range doc.pageNums {
			pageDpl[i] = doc.pageDpl[pageNum]
		}
		sdoc := serial.DocPositions{
			Path:      doc.inPath,
			DocIdx:    doc.docIdx,
			PageNums:  doc.pageNums,
			PageTexts: doc.pageTexts,
			PageDpl:   pageDpl,
		}
		common.Log.Debug("ToHIPDs: sdoc=%d %+v", len(sdoc.PageNums), sdoc.PageNums)
		h := serial.HashIndexPathDoc{
			Hash:  hash,
			Index: idx,
			Path:  path,
			Doc:   sdoc,
		}
		hipds = append(hipds, h)
	}
	return hipds, nil
}

func (lState PositionsState) indexToPath(idx uint64) (string, bool) {
	hash, ok := lState.indexHash[idx]
	if !ok {
		return "", false
	}
	inPath, ok := lState.hashPath[hash]
	return inPath, ok
}

func (lState PositionsState) positionsDir() string {
	return filepath.Join(lState.root, "positions")
}

// OpenPositionsState loads indexes from an existing locations directory `root` or creates one if it
// doesn't exist.
// When opening for writing, do this to ensure final index is written to disk:
//    lState, err := doclib.OpenPositionsState(persistDir, forceCreate)
//    defer lState.Flush()
func OpenPositionsState(root string, forceCreate bool) (*PositionsState, error) {
	lState := PositionsState{
		root:      root,
		hashIndex: map[string]uint64{},
		indexHash: map[uint64]string{},
		hashPath:  map[string]string{},
	}
	if lState.isMem() {
		lState.hashDoc = map[string]*DocPositions{}
	} else {
		if forceCreate {
			if err := lState.removePositionsState(); err != nil {
				return nil, err
			}
		}
		filename := lState.fileListPath()
		fileList, err := loadFileList(filename)
		if err != nil {
			return nil, err
		}
		lState.fileList = fileList
		for i, hip := range fileList {
			lState.hashIndex[hip.Hash] = uint64(i)
			lState.indexHash[uint64(i)] = hip.Hash
			lState.hashPath[hip.Hash] = hip.InPath
		}
	}

	lState.updateTime = time.Now()
	common.Log.Debug("OpenPositionsState: lState=%s", lState)
	return &lState, nil
}

// extractDocPagePositionsReader extracts the text of the PDF file referenced by `rs`.
// It returns the text as a DocPageText per page.
// The []DocPageText refer to DocPositions which are stored in lState.hashDoc which is updated in
// this function.
func (lState *PositionsState) extractDocPagePositionsReader(inPath string, rs io.ReadSeeker) (
	[]DocPageText, error) {

	fd, err := CreateFileDesc(inPath, rs)
	lState.Check()
	if err != nil {
		return nil, err
	}

	lDoc, err := lState.createDocPositions(fd)
	if err != nil {
		return nil, err
	}
	// We need to do be able to back out of partially added entries in lState.
	// The DocPositions is added near the end of lState.doExtract():
	//		See lState.hashDoc[fd.Hash] = lDoc
	// while other maps are updated earlier in lState.addFile()
	// Therefore if there is an error and early exit from State.doExtract(), the lState maps will be
	// inconsistent.
	// I am ashamed of this hack.
	// FIXME: Add a function that updates all the lState maps atomically. !@#$
	docPages, err := lState.doExtract(fd, rs, lDoc)
	if err != nil {
		lState.remove(fd.Hash)
		lState.Check()
		return nil, err
	}
	lState.Check()
	return docPages, err
}

func (lState *PositionsState) doExtract(fd FileDesc, rs io.ReadSeeker, lDoc *DocPositions) (
	[]DocPageText, error) {

	pdfPageProcessor, err := CreatePDFPageProcessorReader(fd.InPath, rs)
	if err != nil {
		return nil, err
	}
	defer pdfPageProcessor.Close()

	numPages, err := pdfPageProcessor.NumPages()
	if err != nil {
		return nil, err
	}

	common.Log.Debug("doExtract: %s numPages=%d", fd, numPages)

	var docPages []DocPageText

	err = pdfPageProcessor.Process(func(pageNum uint32, page *pdf.PdfPage) error {
		common.Log.Trace("doExtract: page %d of %d", pageNum, numPages)
		text, locations, err := ExtractPageTextLocation(page)
		if err != nil {
			common.Log.Error("ExtractDocPagePositions: ExtractPageTextLocation failed. "+
				"%s pageNum=%d err=%v", fd, pageNum, err)
			return nil // !@#$ Skip errors for now
		}
		if text == "" {
			common.Log.Debug("doExtract: No text. %s page %d of %d", fd, pageNum, numPages)
			return nil
		}

		dpl := base.DplFromExtractorLocations(locations)
		pageIdx, err := lDoc.AddDocPage(pageNum, dpl, text)
		if err != nil {
			return err
		}

		docPages = append(docPages, DocPageText{
			DocIdx:  lDoc.docIdx,
			PageIdx: pageIdx,
			PageNum: pageNum,
			Text:    text,
		})
		if len(docPages)%100 == 99 {
			common.Log.Debug("  pageNum=%d of %d docPages=%d %q", pageNum, numPages, len(docPages),
				filepath.Base(fd.InPath))
		}
		dp := docPages[len(docPages)-1]
		common.Log.Trace("doExtract: DocIdx=%d PageIdx=%d dpl=%d", dp.DocIdx, dp.PageIdx, dpl.Len())

		return nil
	})
	if err != nil {
		return docPages, err
	}
	err = lDoc.Close()
	if err != nil {
		return nil, err
	}

	if lState.isMem() {
		common.Log.Debug("ExtractDocPagePositions: pageNums=%v", lDoc.docData.pageNums)
		lState.hashDoc[fd.Hash] = lDoc
	}
	lState.Check()
	return docPages, err
}

// addFile adds PDF file `fd` to `lState`.fileList.
// returns: docIdx, inPath, exists
//     docIdx: Index of PDF file in `lState`.fileList.
//     inPath: Path to file. This the first path this file was added to the index with.
//     exists: true if `fd` was already in lState`.fileList.
func (lState *PositionsState) addFile(fd FileDesc) (uint64, string, bool) {
	hash := fd.Hash
	docIdx, ok := lState.hashIndex[hash]
	lState.Check()
	if ok {
		return docIdx, lState.hashPath[hash], true
	}
	lState.fileList = append(lState.fileList, fd)
	docIdx = uint64(len(lState.fileList) - 1)
	lState.hashIndex[hash] = docIdx
	lState.indexHash[docIdx] = hash
	lState.hashPath[hash] = fd.InPath
	dt := time.Since(lState.updateTime)
	if dt.Seconds() > storeUpdatePeriodSec {
		lState.Flush()
		lState.updateTime = time.Now()
	}
	common.Log.Trace("addFile=%#q docIdx=%d", hash, docIdx)
	return docIdx, fd.InPath, false
}

func (lState *PositionsState) Flush() error {
	if lState.isMem() {
		return nil
	}
	dt := time.Since(lState.updateTime)
	docIdx := uint64(len(lState.fileList) - 1)
	common.Log.Debug("*** Flush %3d files (%4.1f sec) %s",
		docIdx+1, dt.Seconds(), lState.updateTime)
	return saveFileList(lState.fileListPath(), lState.fileList)
}

// fileListPath is the path where lState.fileList is stored on disk.
func (lState *PositionsState) fileListPath() string {
	return filepath.Join(lState.root, "file_list.json")
}

// removePositionsState removes the PositionsState persistent data in the directory tree under
// `root` from disk.
func (lState *PositionsState) removePositionsState() error {
	if !Exists(lState.root) {
		return nil
	}
	flPath := lState.fileListPath()
	if !Exists(flPath) && !strings.HasPrefix(flPath, "store.") {
		common.Log.Error("%q doesn't appear to a be a PositionsState directory. %q doesn't exist.",
			lState.root, flPath)
		return errors.New("not a PositionsState directory")
	}
	err := RemoveDirectory(lState.root)
	if err != nil {
		common.Log.Error("RemoveDirectory(%q) failed. err=%v", lState.root, err)
	}
	return err
}

// docPath returns the file path to the positions files for PDF with hash `hash`.
func (lState *PositionsState) docPath(hash string) string {
	common.Log.Trace("docPath: %q %s", lState.positionsDir(), hash)
	return filepath.Join(lState.positionsDir(), hash)
}

// createIfNecessary creates `lState`.positionsDir if it doesn't already exist.
// It is called at the start of createDocPositions() which allows us to avoid creating our directory
// structure until we have successfully extracted the text from a PDF pages.
func (lState *PositionsState) createIfNecessary() error {
	if lState.root == "" {
		return fmt.Errorf("lState=%s", *lState)
	}
	d := lState.positionsDir()
	common.Log.Trace("createIfNecessary: 1 positionsDir=%q", d)
	if Exists(d) {
		return nil
	}
	// lState.positionsDir = filepath.Join(lState.root, "positions")
	// common.Log.Info("createIfNecessary: 2 positionsDir=%q", lState.positionsDir)
	err := MkDir(d)
	common.Log.Trace("createIfNecessary: err=%v", err)
	return err
}

func (lState *PositionsState) ReadDocPageText(docIdx uint64, pageIdx uint32) (string, error) {
	lDoc, err := lState.OpenPositionsDoc(docIdx)
	if err != nil {
		return "", err
	}
	defer lDoc.Close()
	common.Log.Debug("ReadDocPageText: lDoc=%s", lDoc)
	return lDoc.ReadPageText(pageIdx)
}

// ReadDocPagePositions is inefficient. A DocPositions (a file) is opened and closed to read a page.
func (lState *PositionsState) ReadDocPagePositions(docIdx uint64, pageIdx uint32) (
	string, uint32, base.DocPageLocations, error) {

	lDoc, err := lState.OpenPositionsDoc(docIdx)
	if err != nil {
		return "", 0, base.DocPageLocations{}, err
	}
	defer lDoc.Close()
	pageNum, dpl, err := lDoc.ReadPagePositions(pageIdx)
	common.Log.Trace("docIdx=%d lDoc=%s pageNum=%d", docIdx, lDoc, pageNum)
	lDoc.Check()
	return lDoc.inPath, pageNum, dpl, err
}

// createDocPositions creates a DocPositions for writing.
// createDocPositions always populates the DocPositions with base fields.
// In a persistent `lState`, necessary directories are created and files are opened.
func (lState *PositionsState) createDocPositions(fd FileDesc) (*DocPositions, error) {
	common.Log.Debug("createDocPositions: lState.positionsDir=%q", lState.positionsDir())

	docIdx, inPath, exists := lState.addFile(fd)
	if exists {
		common.Log.Error("createDocPositions: %q is the same PDF as %q. Ignoring",
			fd.InPath, inPath)
		return nil, errors.New("duplicate PDF")
	}
	lDoc, err := lState.baseFields(docIdx)
	if err != nil {
		return nil, err
	}

	if lState.isMem() {
		return lDoc, nil
	}

	// Persistent case
	if err = lState.createIfNecessary(); err != nil {
		return nil, err
	}

	lDoc.dataFile, err = os.Create(lDoc.dataPath)
	if err != nil {
		return nil, err
	}
	err = MkDir(lDoc.textDir)
	return lDoc, err
}

// OpenPositionsDoc opens a DocPositions for reading.
// In a persistent `lState`, necessary files are opened in lDoc.openDoc().
func (lState *PositionsState) OpenPositionsDoc(docIdx uint64) (*DocPositions, error) {
	if lState.isMem() {
		hash := lState.indexHash[docIdx]
		lDoc := lState.hashDoc[hash]
		common.Log.Debug("OpenPositionsDoc(%d)->%s", docIdx, lDoc)
		lDoc.Check()
		return lDoc, nil
	}

	// Persistent handling.
	lDoc, err := lState.baseFields(docIdx)
	if err != nil {
		return nil, err
	}
	err = lDoc.openDoc()
	return lDoc, err
}

// baseFields populates a DocPositions with the fields that are the same for Open and Create.
func (lState *PositionsState) baseFields(docIdx uint64) (*DocPositions, error) {
	if int(docIdx) >= len(lState.fileList) {
		common.Log.Error("docIdx=%d lState=%s\n=%#v", docIdx, *lState, *lState)
		return nil, ErrRange
	}
	inPath := lState.fileList[docIdx].InPath
	hash := lState.fileList[docIdx].Hash

	lDoc := DocPositions{
		lState:  lState,
		inPath:  inPath,
		docIdx:  docIdx,
		pageDpl: map[uint32]base.DocPageLocations{},
	}

	if lState.isMem() {
		mem := docData{}
		lDoc.docData = &mem
	} else {
		locPath := lState.docPath(hash)
		persist := docPersist{
			dataPath:    locPath + ".dat",
			spansPath:   locPath + ".idx.json",
			textDir:     locPath + ".pages",
			pageDplPath: locPath + ".dpl.json",
		}
		lDoc.docPersist = &persist
	}
	if err := lDoc.validate(); err != nil {
		return nil, err
	}
	common.Log.Debug("baseFields: docIdx=%d lDoc=%+v", docIdx, lDoc)
	if lState.isMem() != lDoc.isMem() {
		return nil, fmt.Errorf("lState.isMem()=%t lDoc.isMem()=%t", lState.isMem(), lDoc.isMem())
	}
	return &lDoc, nil
}

func (lState *PositionsState) GetHashPath(docIdx uint64) (hash, inPath string) {
	hash = lState.indexHash[docIdx]
	inPath = lState.hashPath[hash]
	return hash, inPath
}

func loadFileList(filename string) ([]FileDesc, error) {
	b, err := ioutil.ReadFile(filename)
	if err != nil {
		if !Exists(filename) {
			return nil, nil
		}
		return nil, err
	}
	var fileList []FileDesc
	err = json.Unmarshal(b, &fileList)
	return fileList, err
}

func saveFileList(filename string, fileList []FileDesc) error {
	b, err := json.MarshalIndent(fileList, "", "\t")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(filename, b, 0666)
}
