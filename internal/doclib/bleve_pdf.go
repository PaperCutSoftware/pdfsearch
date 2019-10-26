// Copyright 2019 PaperCut Software International Pty Ltd. All rights reserved.

package doclib

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/blevesearch/bleve"
	"github.com/papercutsoftware/pdfsearch/internal/serial"
	"github.com/papercutsoftware/pdfsearch/internal/utils"
	"github.com/unidoc/unipdf/v3/common"
	"github.com/unidoc/unipdf/v3/model"
)

// IDText is what bleve sees for each page of a PDF file.
type IDText struct {
	// ID identifies the document + page index.
	ID string
	// Text is the text that bleve indexes.
	Text string
}

// indexDocPagesLocFile adds the text of all the pages in PDF file `inPath` to Bleve index `index`.
func (blevePdf *BlevePdf) indexDocPagesLocFile(index bleve.Index, inPath string) (
	dtPdf, dtBleve time.Duration, err error) {
	rs, err := os.Open(inPath)
	if err != nil {
		return dtPdf, dtBleve, err
	}
	defer rs.Close()
	return blevePdf.indexDocPagesLocReader(index, inPath, rs)
}

// indexDocPagesLocReader updates `index` and `blevePdf` with the positions of the text in the
// PDF file accessed by `rs`.
// `inPath` is the name of the PDF file. It is provided to help with debugging but is not used.
// !@#$ I don't understand this ^^^
func (blevePdf *BlevePdf) indexDocPagesLocReader(index bleve.Index, inPath string,
	rs io.ReadSeeker) (dtPdf, dtBleve time.Duration, err error) {
	defer blevePdf.check()

	t0 := time.Now()
	// Update blevePdf, the PDF <-> bleve mapping.
	docPages, err := blevePdf.extractDocPagePositionsReader(inPath, rs)
	if err != nil {
		common.Log.Error("indexDocPagesLocReader: Couldn't extract pages from %q err=%v", inPath, err)
		return dtPdf, dtBleve, err
	}
	dtPdf = time.Since(t0)
	common.Log.Debug("indexDocPagesLocReader: inPath=%q docPages=%d", inPath, len(docPages))

	t0 = time.Now()
	// Update index, the bleve index.
	for i, dp := range docPages {
		// Don't weigh down the bleve index with the text bounding boxes, just give it the bare
		// mininum it needs: an id that encodest the document number and page number; and text.
		id := fmt.Sprintf("%04X.%d", dp.DocIdx, dp.PageIdx)
		idText := IDText{ID: id, Text: dp.Text}

		err = index.Index(id, idText)
		if err != nil {
			return dtPdf, dtBleve, err
		}
		dt := time.Since(t0)
		if i%100 == 0 {
			common.Log.Debug("\tIndexed %2d of %d pages in %5.1f sec (%.2f sec/page)",
				i+1, len(docPages), dt.Seconds(), dt.Seconds()/float64(i+1))
			common.Log.Debug("\tid=%q text=%d", id, len(idText.Text))
		}
	}
	dtBleve = time.Since(t0)
	dt := dtPdf + dtBleve
	common.Log.Debug("\tIndexed %d pages in %.1f (Pdf) + %.1f (bleve) = %.1f sec (%.3f sec/page)\n",
		len(docPages), dtPdf.Seconds(), dtBleve.Seconds(), dt.Seconds(), dt.Seconds()/float64(len(docPages)))
	return dtPdf, dtBleve, err
}

/*
   BlevePdf is for serializing and accessing PagePositions.

   Positions are read from disk a page at a time by ReadPositions() which returns the
   []PagePositions for the PDF page given by `doc` and `page`.

   func (blevePdf *BlevePdf) ReadPositions(doc uint64, page uint32) ([]PagePositions, error)

   We use this to allow an efficient look up of DocPageLocation of an offset within a page's text.
   1) Look up []PagePositions for the PDF page given by `doc` and `page`
   2) Binary search []PagePositions to find location for `offset`.

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

// BlevePdf links a bleve index over texts to the PDF files that the texts were extracted from,
// using the hashDoc {file hash: DocPositions} map. For each PDF file, the DocPositions maps
// extracted text to the location on of text on the PDF page it was extracted from.
// A BlevePdf can be optionally saved to and retreived from disk, in which case isMem() returns false.
// BlevePdf is intentionally opaque.
type BlevePdf struct {
	root       string                   // Top level directory of the data saved to disk. "" for in-memory.
	fdList     []fileDesc               // List of fileDescs of PDFs the indexed data was extracted from.
	hashIndex  map[string]uint64        // {file hash: index into fdList}
	hashPath   map[string]string        // {file hash: file path}
	hashDoc    map[string]*DocPositions // {file hash: DocPositions}.
	indexHash  map[uint64]string        // Reverse map of hashIndex.
	updateTime time.Time                // Time of last flush()
}

// Equals returns true if `blevePdf` contains the same information as `other`.
func (blevePdf *BlevePdf) Equals(other *BlevePdf) bool {
	for hash, ldoc := range blevePdf.hashDoc {
		odoc, ok := other.hashDoc[hash]
		if !ok {
			common.Log.Error("BlevePdf.Equal.hash=%#q", hash)
			return false
		}
		if !ldoc.Equals(odoc) {
			common.Log.Error("BlevePdf.Equal.doc hash=%#q\n%s\n%s", hash, ldoc, odoc)
			return false
		}
	}
	return true
}

// String returns a string describing `blevePdf`.
func (blevePdf BlevePdf) String() string {
	var parts []string
	parts = append(parts,
		fmt.Sprintf("%q fdList=%d hashIndex=%d indexHash=%d hashPath=%d hashDoc=%d %s",
			blevePdf.root, len(blevePdf.fdList), len(blevePdf.hashIndex), len(blevePdf.indexHash), len(blevePdf.hashPath),
			len(blevePdf.hashDoc), blevePdf.updateTime))
	for k, docPos := range blevePdf.hashDoc {
		parts = append(parts, fmt.Sprintf("%q: %d", k, docPos.Len()))
	}
	return fmt.Sprintf("{BlevePdf: %s}", strings.Join(parts, "\t"))
}

// Len returns the number of documents in `blevePdf`.
func (blevePdf BlevePdf) Len() int {
	return len(blevePdf.hashIndex)
}

// isMem returns true if `blevePdf` is stored in memory or false if it is stored on disk.
func (blevePdf BlevePdf) isMem() bool {
	return blevePdf.root == ""
}

// remove deletes all the BlevePdf map entries with key `hash` as well as the corresponding
// reverse map entry in hashIndex.
// NOTE: blevePdf.remove(hash) leaves blevePdf.fdList[blevePdf.hashIndex[hash]] with no references
//       to it, so we waste a small amount of memory that we don't care about.
func (blevePdf *BlevePdf) remove(hash string) {
	if index, ok := blevePdf.hashIndex[hash]; ok {
		delete(blevePdf.indexHash, index)
	}
	delete(blevePdf.hashIndex, hash)
	delete(blevePdf.hashPath, hash)
	delete(blevePdf.hashDoc, hash)
}

// CheckConsistency should be set true to regularly check the BlevePdf consistency.
var CheckConsistency = false

// check() performs a consistency check on a BlevePdf.
func (blevePdf BlevePdf) check() {
	if !CheckConsistency {
		return
	}
	if len(blevePdf.fdList) == 0 || len(blevePdf.hashIndex) == 0 || len(blevePdf.indexHash) == 0 ||
		len(blevePdf.hashPath) == 0 || len(blevePdf.hashDoc) == 0 {
		return
	}
	for _, docPos := range blevePdf.hashDoc {
		docPos.check()
	}
	var keys []string
	for h := range blevePdf.hashDoc {
		keys = append(keys, h)
	}
	sort.Strings(keys)
	var keyt []string
	for h := range blevePdf.hashIndex {
		keyt = append(keyt, h)
	}
	sort.Strings(keyt)

	for h, i := range blevePdf.hashIndex {
		if hh, ok := blevePdf.indexHash[i]; !ok {
			common.Log.Info("%#q\nhashIndex:%d %+v",
				h, len(keyt), keyt)
			panic("BlevePdf.Check:1")
		} else if hh != h {
			common.Log.Info("hash=%q indexHash=%#q index=%d\nhashIndex:%d %+v",
				h, hh, i, len(keyt), keyt)
			panic("BlevePdf.Check:2")
		}
		if _, ok := blevePdf.hashPath[h]; !ok {
			common.Log.Info("%#q missing from hashPath\nhashDoc  :%d %+v\nhashIndex:%d %+v",
				h, len(keys), keys, len(keyt), keyt)
			panic("BlevePdf.Check:3")
		}
		if _, ok := blevePdf.hashDoc[h]; !ok {
			common.Log.Info("%#q missing from hashDoc\nhashDoc  :%d %+v\nhashIndex:%d %+v",
				h, len(keys), keys, len(keyt), keyt)
			panic("BlevePdf.Check:4")
		}
	}
}

// BlevePdfFromHIPDs creates a BlevePdf from its seralized form `hipds`.
// It is used to deserialize a BlevePdf.
// !@#$ Round trip test BlevePdfFromHIPDs + ToHIPDs
func BlevePdfFromHIPDs(hipds []serial.HashIndexPathDoc) (BlevePdf, error) {
	blevePdf := BlevePdf{
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

		common.Log.Debug("BlevePdfFromHIPDs: sdoc.PageNums=%d sdoc.PagePositions=%d",
			len(sdoc.PageNums), len(sdoc.PagePositions))
		// sdoc.PagePositions is a slice with entries corresponding to page numbers in sdoc.PageNums
		pagePositions := map[uint32]PagePositions{}
		for i, pageNum := range sdoc.PageNums {
			pagePositions[pageNum] = PagePositions{sdoc.PagePositions[i]}
		}
		docPos := DocPositions{
			inPath:        sdoc.Path,   // Path of input PDF file.
			docIdx:        sdoc.DocIdx, // Index into blevePdf.fdList.
			pagePositions: pagePositions,
			docData: &docData{
				pageNums:  sdoc.PageNums,
				pageTexts: sdoc.PageTexts,
			},
		}

		blevePdf.hashPath[hash] = path
		blevePdf.hashDoc[hash] = &docPos
		blevePdf.hashIndex[hash] = idx
		blevePdf.indexHash[idx] = hash

		desc := fileDesc{
			InPath: path,
			Hash:   hash,
		}
		blevePdf.fdList = append(blevePdf.fdList, desc)
	}
	blevePdf.check()
	return blevePdf, nil
}

// ToHIPDs converts `blevePdf` to a serial.HashIndexPathDoc.
// blevePdf.Check() is run before saving to avoid empty serializations.
func (blevePdf BlevePdf) ToHIPDs() ([]serial.HashIndexPathDoc, error) {
	var hipds []serial.HashIndexPathDoc
	for hash, idx := range blevePdf.hashIndex {
		path, ok := blevePdf.hashPath[hash]
		if !ok {
			return nil, fmt.Errorf("hash=%#q not in hashPath", hash)
		}
		doc, ok := blevePdf.hashDoc[hash]
		if !ok {
			var keys []string
			for h := range blevePdf.hashDoc {
				keys = append(keys, h)
			}
			sort.Strings(keys)
			var keyt []string
			for h := range blevePdf.hashIndex {
				keyt = append(keyt, h)
			}
			sort.Strings(keyt)
			return nil, fmt.Errorf("hash=%#q not in hashDoc.\nhashDoc  :%d %+v\nhashIndex:%d %+v",
				hash, len(keys), keys, len(keyt), keyt)
		}
		// sdoc.PagePositions is a slice with entries corresponding to page numbers in sdoc.PageNums
		common.Log.Trace("doc.pagePositions=%d", len(doc.pagePositions))
		pagePositions := make([][]serial.OffsetBBox, len(doc.pagePositions))
		for i, pageNum := range doc.pageNums {
			pagePositions[i] = doc.pagePositions[pageNum].offsetBBoxes
		}
		sdoc := serial.DocPositions{
			Path:          doc.inPath,
			DocIdx:        doc.docIdx,
			PageNums:      doc.pageNums,
			PageTexts:     doc.pageTexts,
			PagePositions: pagePositions,
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

// pdfXrefDir returns the full path of PDF content <-> bleve index mappings on disk.
func (blevePdf BlevePdf) pdfXrefDir() string {
	return filepath.Join(blevePdf.root, "pdf.xref")
}

// OpenBlevePdf loads indexes from an existing locations directory `root` or creates one if it
// doesn't exist.
// When opening for writing, do the following to ensure final index is written to disk:
//    blevePdf, err := doclib.OpenBlevePdf(persistDir, forceCreate)
//    defer blevePdf.flush()
func openBlevePdf(root string, forceCreate bool) (*BlevePdf, error) {
	blevePdf := BlevePdf{
		root:      root,
		hashIndex: map[string]uint64{},
		indexHash: map[uint64]string{},
		hashPath:  map[string]string{},
	}
	if blevePdf.isMem() {
		blevePdf.hashDoc = map[string]*DocPositions{}
	} else {
		if forceCreate {
			if err := blevePdf.removeBlevePdf(); err != nil {
				return nil, err
			}
		}
		filename := blevePdf.fileListPath()
		fdList, err := loadFileDescList(filename)
		if err != nil {
			return nil, err
		}
		blevePdf.fdList = fdList
		for i, fd := range fdList {
			blevePdf.hashIndex[fd.Hash] = uint64(i)
			blevePdf.indexHash[uint64(i)] = fd.Hash
			blevePdf.hashPath[fd.Hash] = fd.InPath
		}
	}

	blevePdf.updateTime = time.Now()
	common.Log.Debug("OpenBlevePdf: blevePdf=%s", blevePdf)
	return &blevePdf, nil
}

// extractDocPagePositionsReader extracts the text of the PDF file referenced by `rs`.
// It returns the text as a DocPageText per page.
// The []DocPageText refer to DocPositions which are stored in blevePdf.hashDoc which is updated in
// this function.
func (blevePdf *BlevePdf) extractDocPagePositionsReader(inPath string, rs io.ReadSeeker) (
	[]DocPageText, error) {
	fd, err := createFileDesc(inPath, rs)
	if err != nil {
		return nil, err
	}
	defer blevePdf.check()

	docPos, err := blevePdf.createDocPositions(fd)
	if err != nil {
		return nil, err
	}
	// We need to do be able to back out of partially added entries in blevePdf.
	// The DocPositions is added near the end of blevePdf.doExtract():
	//		See blevePdf.hashDoc[fd.Hash] = docPos
	// while other maps are updated earlier in blevePdf.addFile()
	// Therefore if there is an error and early exit from State.doExtract(), the blevePdf maps will
	// be inconsistent.
	// I am ashamed of this hack. TODO: Fix it!
	// FIXME: Add a function that updates all the blevePdf maps atomically. !@#$
	docPages, err := blevePdf.doExtract(fd, rs, docPos)
	if err != nil {
		blevePdf.remove(fd.Hash)
		return nil, err
	}
	return docPages, err
}

// document this !@#$
func (blevePdf *BlevePdf) doExtract(fd fileDesc, rs io.ReadSeeker, docPos *DocPositions) (
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

	err = pdfPageProcessor.Process(func(pageNum uint32, page *model.PdfPage) error {
		common.Log.Trace("doExtract: page %d of %d", pageNum, numPages)
		text, textMarks, err := ExtractPageTextMarks(page)
		if err != nil {
			common.Log.Error("ExtractDocPagePositions: ExtractPageTextMarks failed. "+
				"%s pageNum=%d err=%v", fd, pageNum, err)
			return nil // !@#$ Skip errors for now
		}
		if text == "" {
			common.Log.Debug("doExtract: No text. %s page %d of %d", fd, pageNum, numPages)
			return nil
		}

		ppos := PagePositionsFromTextMarks(textMarks)
		pageIdx, err := docPos.AddDocPage(pageNum, ppos, text)
		if err != nil {
			return err
		}

		docPages = append(docPages, DocPageText{
			DocIdx:  docPos.docIdx,
			PageIdx: pageIdx,
			PageNum: pageNum,
			Text:    text,
		})
		if len(docPages)%100 == 99 {
			common.Log.Debug("  pageNum=%d of %d docPages=%d %q", pageNum, numPages, len(docPages),
				filepath.Base(fd.InPath))
		}
		dp := docPages[len(docPages)-1]
		common.Log.Trace("doExtract: DocIdx=%d PageIdx=%d", dp.DocIdx, dp.PageIdx)

		return nil
	})
	if err != nil {
		return docPages, err
	}
	err = docPos.Close()
	if err != nil {
		return nil, err
	}

	if blevePdf.isMem() {
		common.Log.Debug("ExtractDocPagePositions: pageNums=%v", docPos.docData.pageNums)
		blevePdf.hashDoc[fd.Hash] = docPos
	}
	blevePdf.check()
	return docPages, err
}

// addFile adds PDF fileDesc `fd` to `blevePdf`.fdList.
// returns: docIdx, inPath, exists
//     docIdx: Index of PDF file in `blevePdf`.fdList.
//     inPath: Path to file. This the first path this file was added to the index with.
//     exists: true if `fd` was already in blevePdf`.fdList.
func (blevePdf *BlevePdf) addFile(fd fileDesc) (uint64, string, bool) {
	hash := fd.Hash
	docIdx, exists := blevePdf.hashIndex[hash]
	if exists {
		return docIdx, blevePdf.hashPath[hash], true
	}
	blevePdf.fdList = append(blevePdf.fdList, fd)
	docIdx = uint64(len(blevePdf.fdList) - 1)
	blevePdf.hashIndex[hash] = docIdx
	blevePdf.indexHash[docIdx] = hash
	blevePdf.hashPath[hash] = fd.InPath
	dt := time.Since(blevePdf.updateTime)
	if dt.Seconds() > storeUpdatePeriodSec {
		blevePdf.flush()
		blevePdf.updateTime = time.Now()
	}
	common.Log.Trace("addFile=%#q docIdx=%d dt=%.1f secs", hash, docIdx, dt.Seconds())

	// blevePdf.check() !@#$ Reinstate
	return docIdx, fd.InPath, false
}

// flush saves `blevePdf` to disk
func (blevePdf *BlevePdf) flush() error {
	if blevePdf.isMem() {
		return nil
	}
	dt := time.Since(blevePdf.updateTime)
	docIdx := uint64(len(blevePdf.fdList) - 1)
	common.Log.Debug("*** flush %3d files (%4.1f sec) %s",
		docIdx+1, dt.Seconds(), blevePdf.updateTime)
	return saveFileDescList(blevePdf.fileListPath(), blevePdf.fdList)
}

// fileListPath is the path where blevePdf.fdList is stored on disk.
func (blevePdf *BlevePdf) fileListPath() string {
	return filepath.Join(blevePdf.root, "file_list.json")
}

// removeBlevePdf removes the BlevePdf persistent data in the directory tree under
// `root` from disk. // !@#$ removeFromDisk() ?
func (blevePdf *BlevePdf) removeBlevePdf() error {
	if !utils.Exists(blevePdf.root) {
		return nil
	}
	flPath := blevePdf.fileListPath()
	if !utils.Exists(flPath) && !strings.HasPrefix(flPath, "store.") {
		common.Log.Error("%q doesn't appear to a be a BlevePdf directory. %q doesn't exist.",
			blevePdf.root, flPath)
		return errors.New("not a BlevePdf directory")
	}
	err := utils.RemoveDirectory(blevePdf.root)
	if err != nil {
		common.Log.Error("RemoveDirectory(%q) failed. err=%v", blevePdf.root, err)
	}
	return err
}

// docPath returns the file path to the PDF<-bleve cross-reference files for PDF with hash `hash`.
func (blevePdf *BlevePdf) docPath(hash string) string {
	common.Log.Trace("docPath: %q %s", blevePdf.pdfXrefDir(), hash)
	return filepath.Join(blevePdf.pdfXrefDir(), hash)
}

// createIfNecessary creates `blevePdf`.pdfXrefDir if it doesn't already exist.
// It is called at the start of createDocPositions() which allows us to avoid creating our directory
// structure until we have successfully extracted the text from PDF pages.
func (blevePdf *BlevePdf) createIfNecessary() error {
	if blevePdf.root == "" {
		return fmt.Errorf("blevePdf=%s", *blevePdf)
	}
	d := blevePdf.pdfXrefDir()
	common.Log.Trace("createIfNecessary: 1 pdfXrefDir=%q", d)
	if utils.Exists(d) {
		return nil
	}
	return utils.MkDir(d)
}

// docPageText returns the text extracted from the PDF page with document and page indices
// `docIdx` and `pageIdx`.
func (blevePdf *BlevePdf) docPageText(docIdx uint64, pageIdx uint32) (string, error) {
	docPos, err := blevePdf.openDocPosition(docIdx)
	if err != nil {
		return "", err
	}
	defer docPos.Close()
	common.Log.Trace("docPageText: docPos=%s", docPos)
	return docPos.pageText(pageIdx)
}

// docPagePositions returns (inPath, pageNum, ppos, err) for the PDF page with document and page
// indices `docIdx` and `pageIdx` where
//   inPath: name of PDf file
//   pageNum: (1-offset) page number of PDF page
//   ppos: PagePositions for the page text (maps text offsets to PDF page locations)
// TODO: docPagePositions is inefficient. A DocPositions (a file) is opened and closed to read a page. !@#$
func (blevePdf *BlevePdf) docPagePositions(docIdx uint64, pageIdx uint32) (
	string, uint32, PagePositions, error) {

	docPos, err := blevePdf.openDocPosition(docIdx)
	if err != nil {
		return "", 0, PagePositions{}, err
	}
	defer docPos.Close()
	pageNum, ppos, err := docPos.pageNumPositions(pageIdx)
	common.Log.Trace("docIdx=%d docPos=%s pageNum=%d", docIdx, docPos, pageNum)
	docPos.check()
	return docPos.inPath, pageNum, ppos, err
}

// createDocPositions adds fileDesc `fd` to `blevePdf` and returns a DocPositions that is reading ]
// for writing.
// createDocPositions always populates the returned DocPositions with base fields.
// In a persistent `blevePdf`, necessary directories are created and files are opened.
func (blevePdf *BlevePdf) createDocPositions(fd fileDesc) (*DocPositions, error) {
	common.Log.Debug("createDocPositions: blevePdf.pdfXrefDir=%q", blevePdf.pdfXrefDir())
	docIdx, inPath, exists := blevePdf.addFile(fd)
	if exists {
		common.Log.Error("createDocPositions: %q is the same PDF as %q. Ignoring",
			fd.InPath, inPath)
		return nil, errors.New("duplicate PDF")
	}
	docPos, err := blevePdf.baseFields(docIdx)
	if err != nil {
		return nil, err
	}

	if blevePdf.isMem() {
		return docPos, nil
	}

	// Persistent case.
	if err = blevePdf.createIfNecessary(); err != nil {
		return nil, err
	}
	docPos.dataFile, err = os.Create(docPos.dataPath)
	if err != nil {
		return nil, err
	}
	err = utils.MkDir(docPos.textDir)
	return docPos, err
}

// openDocPosition opens a DocPositions for reading.
// In a persistent `blevePdf`, necessary files are opened in docPos.openDoc().
func (blevePdf *BlevePdf) openDocPosition(docIdx uint64) (*DocPositions, error) {
	if blevePdf.isMem() {
		hash := blevePdf.indexHash[docIdx]
		docPos := blevePdf.hashDoc[hash]
		common.Log.Debug("openDocPosition(%d)->%s", docIdx, docPos)
		docPos.check()
		return docPos, nil
	}

	// Persistent handling.
	docPos, err := blevePdf.baseFields(docIdx)
	if err != nil {
		return nil, err
	}
	err = docPos.openDoc()
	return docPos, err
}

// baseFields returns the DocPositions for document index `docIdx` populated with the fields that
// are the same for Open() and Create().
func (blevePdf *BlevePdf) baseFields(docIdx uint64) (*DocPositions, error) {
	if int(docIdx) >= len(blevePdf.fdList) {
		common.Log.Error("docIdx=%d blevePdf=%s\n=%#v", docIdx, *blevePdf, *blevePdf)
		return nil, errors.New("out of range")
	}
	inPath := blevePdf.fdList[docIdx].InPath
	hash := blevePdf.fdList[docIdx].Hash

	docPos := DocPositions{
		blevePdf:      blevePdf,
		inPath:        inPath,
		docIdx:        docIdx,
		pagePositions: map[uint32]PagePositions{},
	}

	if blevePdf.isMem() {
		mem := docData{}
		docPos.docData = &mem
	} else {
		locPath := blevePdf.docPath(hash)
		persist := docPersist{
			dataPath:          locPath + ".dat",
			spansPath:         locPath + ".idx.json",
			textDir:           locPath + ".pages",
			pagePositionsPath: locPath + ".ppos.json",
		}
		docPos.docPersist = &persist
	}

	common.Log.Debug("baseFields: docIdx=%d docPos=%+v", docIdx, docPos)
	if blevePdf.isMem() != docPos.isMem() {
		return nil, fmt.Errorf("blevePdf.isMem()=%t docPos.isMem()=%t", blevePdf.isMem(), docPos.isMem())
	}
	return &docPos, nil
}
