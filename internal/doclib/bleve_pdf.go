// Copyright 2019 PaperCut Software International Pty Ltd. All rights reserved.

package doclib

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/blevesearch/bleve"
	"github.com/papercutsoftware/pdfsearch/internal/utils"
	"github.com/unidoc/unipdf/v3/common"
	"github.com/unidoc/unipdf/v3/model"
)

// IDText is what bleve sees for each page of a PDF.
type IDText struct {
	// ID identifies the document + page index.
	ID string
	// Text is the text that bleve indexes.
	Text string
}

// indexDocPagesLoc adds the text of all the pages in the PDF `fd.InPath` to `blevePdf` and to bleve
// index `index`.
// writeDocContents updates blevePdf with `fd` which describes a PDF on disk and `docContents`, the
// document contents of the PDF`fd.InPath`.
func (blevePdf *BlevePdf) indexDocPagesLoc(index bleve.Index, fd fileDesc, docContents []pageContents) (
	dtPdf, dtBleve time.Duration, err error) {
	defer blevePdf.check()

	t0 := time.Now()

	// Update blevePdf, the PDF <-> bleve mapping.
	docPages, err := blevePdf.writeDocContents(fd, docContents)
	if err != nil {
		common.Log.Error("indexDocPagesLoc: Couldn't add doc contents to blevePdf %q err=%v",
			fd.InPath, err)
		return dtPdf, dtBleve, err
	}
	if len(docPages) == 0 {
		panic("no pages")
	}
	dtPdf = time.Since(t0)
	common.Log.Debug("indexDocPagesLoc: inPath=%q docPages=%d", fd.InPath, len(docPages))

	t0 = time.Now()
	// Prepare `batch` for the bleve index update.
	batch := index.NewBatch()
	for i, dp := range docPages {
		// Don't weigh down the bleve index with the text bounding boxes, just give it the bare
		// mininum it needs: an id that encodes the document number and page number; and text.
		id := fmt.Sprintf("%04X.%d", dp.DocIdx, dp.PageIdx)
		idText := IDText{ID: id, Text: dp.Text}

		err = batch.Index(id, idText)
		if err != nil {
			return dtPdf, dtBleve, err
		}
		if batch.Size() >= 100 {
			// Update `index`, the bleve index.
			err = index.Batch(batch)
			if err != nil {
				return dtPdf, dtBleve, err
			}
			batch = index.NewBatch()
		}
		dt := time.Since(t0)
		if i%100 == 0 {
			common.Log.Debug("\tIndexed %2d of %d pages in %5.1f sec (%.2f sec/page)",
				i+1, len(docPages), dt.Seconds(), dt.Seconds()/float64(i+1))
			common.Log.Debug("\tid=%q text=%d", id, len(idText.Text))
		}
	}
	if batch.Size() > 0 {
		err = index.Batch(batch)
		if err != nil {
			return dtPdf, dtBleve, err
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
     pagePartition (12 byte data structure)
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

// BlevePdf links a bleve index over texts to the PDFs that the texts were extracted from,
// using the hashDoc {file hash: DocPositions} map. For each PDF, the DocPositions maps
// extracted text to the location of text on the PDF page it was extracted from.
// A BlevePdf can be saved to and retrieved from disk.
// BlevePdf is intentionally opaque.
type BlevePdf struct {
	root   string     // Top level directory of the data saved to disk.
	fdList []fileDesc // List of fileDescs of PDFs the indexed data was extracted from.
	// Should these be disk access functions? !@#$
	hashDoc    map[string]*DocPositions // {file hash: DocPositions}
	indexHash  map[uint64]string        // Reverse map of hashDoc. !@#$ Needed for persistent case?
	updateTime time.Time                // Time of last flush()
}

// String returns a string describing `blevePdf`.
func (blevePdf BlevePdf) String() string {
	var parts []string
	parts = append(parts,
		fmt.Sprintf("%q fdList=%d indexHash=%d hashDoc=%d time=%s",
			blevePdf.root, len(blevePdf.fdList), len(blevePdf.indexHash),
			len(blevePdf.hashDoc), blevePdf.updateTime))
	for k, docPos := range blevePdf.hashDoc {
		parts = append(parts, fmt.Sprintf("%q: %d", k, docPos.Len()))
	}
	return fmt.Sprintf("{BlevePdf: %s}", strings.Join(parts, "\n"))
}

// Len returns the number of documents in `blevePdf`. !@#$ Does it? Test!
func (blevePdf BlevePdf) Len() int {
	return len(blevePdf.hashDoc)
}

// remove deletes all the BlevePdf map entries with key `hash` as well as the corresponding
// reverse map entry in hashIndex.
// NOTE: blevePdf.remove(hash) leaves blevePdf.fdList[blevePdf.hashIndex[hash]] with no references
//       to it, so we waste a small amount of memory that we don't care about.
func (blevePdf *BlevePdf) remove(hash string) {
	if doc, ok := blevePdf.hashDoc[hash]; ok {
		delete(blevePdf.indexHash, doc.docIdx)
	}
	delete(blevePdf.hashDoc, hash)
}

// CheckConsistency should be set true to regularly check the BlevePdf consistency.
var CheckConsistency = false

// check() performs a consistency check on a BlevePdf.
func (blevePdf BlevePdf) check() {
	if !CheckConsistency {
		return
	}
	if len(blevePdf.fdList) == 0 || len(blevePdf.indexHash) == 0 || len(blevePdf.hashDoc) == 0 {
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
	for h := range blevePdf.hashDoc {
		keyt = append(keyt, h)
	}
	sort.Strings(keyt)

	for h, doc := range blevePdf.hashDoc {
		i := doc.docIdx
		if hh, ok := blevePdf.indexHash[i]; !ok {
			common.Log.Info("%#q\nhashIndex:%d %+v", h, len(keyt), keyt)
			panic("BlevePdf.Check:1")
		} else if hh != h {
			common.Log.Info("hash=%q indexHash=%#q index=%d\nhashIndex:%d %+v",
				h, hh, i, len(keyt), keyt)
			panic("BlevePdf.Check:2")
		}
		if _, ok := blevePdf.hashDoc[h]; !ok {
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

// pdfXrefDir returns the full path of PDF content <-> bleve index mappings on disk.
func (blevePdf BlevePdf) pdfXrefDir() string {
	return filepath.Join(blevePdf.root, "pdf.xref")
}

// openBlevePdf loads indexes from an existing locations directory `root` or creates one if it
// doesn't exist.
// When opening for writing, do the following to ensure the final index is written to disk:
//    blevePdf, err := doclib.openBlevePdf(persistDir, forceCreate)
//    defer blevePdf.flush()
// !@#$ Doesn't load hashDoc
func openBlevePdf(root string, forceCreate bool) (*BlevePdf, error) {
	blevePdf := BlevePdf{
		root:      root,
		indexHash: map[uint64]string{},
	}

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
		blevePdf.indexHash[uint64(i)] = fd.Hash
	}

	blevePdf.updateTime = time.Now()
	common.Log.Debug("OpenBlevePdf: blevePdf=%s", blevePdf)
	return &blevePdf, nil
}

// pageContents are the result of text extraction on a PDF page
type pageContents struct {
	pageNum uint32        // (1-offset) PDF page number.
	ppos    PagePositions // Positions of PDF text fragments on page.
	text    string        // Extracted page text.
}

// extractDocPagePositions computes a fileDesc for the PDF `inPath` and extracts the text and text
// positions for all the pages in the PDF. It returns this as a pageContents per page.
func extractDocPagePositions(inPath string) (fileDesc, []pageContents, error) {
	fd, err := createFileDesc(inPath)
	if err != nil {
		panic(err) // !@#$ should never happen
		return fileDesc{}, nil, err
	}

	// Compute the document contents.
	docContents, err := extractDocContents(fd)
	if err != nil {
		return fileDesc{}, nil, err
	}
	return fd, docContents, nil
}

// extractDocContents extracts page text and positions from the PDF described by `fd`.
func extractDocContents(fd fileDesc) ([]pageContents, error) {
	pdfPageProcessor, err := CreatePDFPageProcessorFile(fd.InPath)
	if err != nil {
		return nil, err
	}
	defer pdfPageProcessor.Close()

	numPages, err := pdfPageProcessor.NumPages()
	if err != nil {
		return nil, err
	}
	common.Log.Debug("extractDocContents: %s numPages=%d", fd, numPages)

	var docContents []pageContents
	err = pdfPageProcessor.Process(func(pageNum uint32, page *model.PdfPage) error {
		common.Log.Trace("extractDocContents: page %d of %d", pageNum, numPages)
		text, textMarks, err := ExtractPageTextMarks(page)
		if err != nil {
			common.Log.Debug("ExtractDocPagePositions: ExtractPageTextMarks failed. "+
				"%s pageNum=%d err=%v", fd, pageNum, err)
			return nil // Skip errors for now. TODO: Make error handling configurable.
		}
		if text == "" {
			common.Log.Debug("extractDocContents: No text. %s page %d of %d", fd, pageNum, numPages)
			return nil
		}

		ppos := PagePositionsFromTextMarks(textMarks)
		docContents = append(docContents, pageContents{
			pageNum: pageNum,
			ppos:    ppos,
			text:    text,
		})
		if len(docContents)%100 == 99 {
			common.Log.Debug("  pageNum=%d of %d docContents=%d %q", pageNum, numPages, len(docContents),
				filepath.Base(fd.InPath))
		}
		return nil
	})

	return docContents, err
}

// writeDocContents updates blevePdf with `fd` which describes a PDF on disk and `docContents`, the
// document contents of the PDF `fd.InPath`.
// It returns the docContents as a []DocPageText. !@#$ Why?
// This function is supposed to be atomic. If writing the document contents to disk fails at any
// stage, all references to the document are removed. !@#$ Remove from bleve too.
func (blevePdf *BlevePdf) writeDocContents(fd fileDesc, docContents []pageContents) ([]DocPageText,
	error) {
	defer blevePdf.check()
	docPos, err := blevePdf.createDocPositions(fd)
	if err != nil {
		return nil, err
	}
	docPages, err := blevePdf.addDocContents(docContents, docPos)
	if err != nil {
		//  This should delete the document directory from disk.
		if err2 := blevePdf.deleteDocPositions(docPos); err2 != nil {
			common.Log.Error("writeDocContents. Couldn't delete docPos err=%v", err2)
		}
		blevePdf.remove(fd.Hash)
		return nil, err
	}
	return docPages, err
}

// addDocContents writes `docContents` to the files in `docPos`.
func (blevePdf *BlevePdf) addDocContents(docContents []pageContents, docPos *DocPositions) (
	[]DocPageText, error) {
	var docPages []DocPageText
	for _, contents := range docContents {
		pageIdx, err := docPos.AddDocPage(contents.pageNum, contents.ppos, contents.text)
		if err != nil {
			return nil, err
		}

		docPages = append(docPages, DocPageText{
			DocIdx:  docPos.docIdx,
			PageIdx: pageIdx,
			PageNum: contents.pageNum,
			Text:    contents.text,
		})
		if len(docPages)%100 == 99 {
			common.Log.Debug("  pageNum=%d docPages=%d %q", contents.pageNum, len(docPages))
		}
		dp := docPages[len(docPages)-1]
		common.Log.Trace("addDocContents: DocIdx=%d PageIdx=%d", dp.DocIdx, dp.PageIdx)
	}

	err := docPos.Close()
	if err != nil {
		return nil, err
	}
	blevePdf.check()
	return docPages, err
}

// addFile adds PDF fileDesc `fd` to `blevePdf.fdList`.
// returns: docIdx, inPath, exists
//     docIdx: Index of PDF in `blevePdf.fdList`.
//     inPath: Path to file. This the first path this file was added to the index with.
//     exists: true if `fd` was already in `blevePdf.fdList`.
//  !@#$ hashDoc doesn't get updated
// Make this an atomic add file.
// Create a session for adding a PDF
// Go to work on it.
// When done submit back to index atomically
func (blevePdf *BlevePdf) addFile(fd fileDesc) (uint64, string, bool) {
	hash := fd.Hash

	blevePdf.fdList = append(blevePdf.fdList, fd)
	docIdx := uint64(len(blevePdf.fdList) - 1)
	blevePdf.indexHash[docIdx] = hash
	dt := time.Since(blevePdf.updateTime)
	if dt.Seconds() > storeUpdatePeriodSec {
		blevePdf.flush()
		blevePdf.updateTime = time.Now()
	}
	common.Log.Trace("addFile=%#q docIdx=%d dt=%.1f secs", hash, docIdx, dt.Seconds())

	return docIdx, fd.InPath, false
}

// flush saves `blevePdf` to disk.
func (blevePdf *BlevePdf) flush() error {
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

// removeBlevePdf removes the BlevePdf persistent data in the directory tree under `root` from disk.
// TODO: Improve name. Mayb removeFromDisk() ?
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
// TODO: Deprecate. docPagePositions is inefficient. A DocPositions (a file) is opened and closed
// to read a page.
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

// createDocPositions adds fileDesc `fd` to `blevePdf` and returns a DocPositions for writing.
// createDocPositions always populates the returned DocPositions with base fields.
// Necessary directories are created and files are opened.
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

// !@#$ also delete from bleve
func (blevePdf *BlevePdf) deleteDocPositions(docPos *DocPositions) error {
	common.Log.Info("deleteDocPositions:\n\tblevePdf.pdfXrefDir=%q\n\tdataPath=%q\n\ttextDir=%q",
		blevePdf.pdfXrefDir(), docPos.dataPath, docPos.textDir)
	if utils.Exists(docPos.dataPath) {
		if err := os.Remove(docPos.dataPath); err != nil {
			return err
		}
	}
	if utils.Exists(docPos.partitionsPath) {
		// partitionsPath is written after this function is called, so this isn't necessary right now
		if err := os.Remove(docPos.partitionsPath); err != nil {
			return err
		}
	}
	if utils.Exists(docPos.textDir) {
		if err := os.RemoveAll(docPos.textDir); err != nil {
			return err
		}
	}
	return nil
}

// openDocPosition opens a DocPositions for reading.
// Necessary files are opened in docPos.openDoc().
func (blevePdf *BlevePdf) openDocPosition(docIdx uint64) (*DocPositions, error) {
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
		inPath:        inPath,
		docIdx:        docIdx,
		pagePositions: map[uint32]PagePositions{},
	}

	locPath := blevePdf.docPath(hash)
	// !@#$ No need for this
	persist := docPersist{
		dataPath:       locPath + ".dat",
		partitionsPath: locPath + ".idx.json",
		textDir:        locPath + ".page.contents",
	}
	docPos.docPersist = &persist

	common.Log.Debug("baseFields: docIdx=%d docPos=%+v", docIdx, docPos)
	return &docPos, nil
}
