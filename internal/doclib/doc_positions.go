// Copyright 2019 PaperCut Software International Pty Ltd. All rights reserved.

package doclib

import (
	"errors"
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/unidoc/unipdf/v3/common"
)

// DocPositions is used to the link per-document data in a bleve index to the PDF the data was
// extracted from.
// There is one DocPositions per PDF.
type DocPositions struct {
	inPath        string                   // Path of input PDF.
	docIdx        uint64                   // Index into blevePdf.fileList.
	pageNums      []uint32                 // {(1-offset) PDF page numbers
	pagePositions map[uint32]PagePositions // {(1-offset) PDF pageNum: locations of text fragments on page}
	pageText      map[uint32]string        // {(1-offset) PDF pageNum: extracted page text}
	docPersist                             // Optional extra fields for on-disk indexes.
}

// docPersist tracks the info for indexing a PDF on disk.
type docPersist struct {
	dataPath       string // `pagePositions` are stored in this file.
	partitionsPath string // `pagePartitions` are stored in this file.
	textDir        string // `pageText` are stored in this directoty. Used for debugging.
}

// pagePartition is the location of the bytes of a PagePositions in a data file.
// The partition is over [Offset, Offset+Size).
// There is one pagePartition (corresponding to a PagePositions) per page.
type pagePartition struct {
	Offset  uint32 // Offset in the data file for the PagePositions for a page.
	Size    uint32 // Size of the PagePositions in the data file.
	Check   uint32 // CRC checksum for the PagePositions data.
	PageNum uint32 // (1-offset) PDF page number.
}

// String returns a human readable string describing `docPos`.
func (docPos DocPositions) String() string {
	return fmt.Sprintf("DocPositions{%q %d %v}",
		docPos.dataPath,
		len(docPos.pageNums), docPos.pageNums)
}

// Len returns the number of pages in `docPos`.
func (docPos DocPositions) Len() int {
	return len(docPos.pageNums) // !@#$
}

// check panics is `docPos` is an inconsistent state, which should never happen.
func (docPos DocPositions) check() {
	for _, pageNum := range docPos.pageNums {
		if pageNum == 0 {
			common.Log.Error("docPos.check.:\n\tlDoc=%#v\n\tpagePositions=%#v", docPos,
				docPos.pagePositions)
			common.Log.Error("docPos.check.: keys=%d %+v", len(docPos.pageNums), docPos.pageNums)
			panic(errors.New("docPos.check.: bad pageNum"))
		}
	}
}

// addDocContents fills the data fields in `docPos` with the almost identical fields from
// `docContents` !@#$ Do we need both structs?
func (docPos *DocPositions) addDocContents(docContents []pageContents) {
	n := len(docContents)
	docPos.pageNums = make([]uint32, n)
	docPos.pagePositions = make(map[uint32]PagePositions, n)
	docPos.pageText = make(map[uint32]string, n)

	for i, contents := range docContents {
		docPos.pageNums[i] = contents.pageNum
		docPos.pagePositions[contents.pageNum] = contents.ppos
		docPos.pageText[contents.pageNum] = contents.text
	}
}

// readPersistedPageText returns the text extracted for page with in `docPos` with page index
// `pageIdx` for a persisted index.
// TODO: Can we remove this? See pageText(). !@#$
func (docPos *DocPositions) readPersistedPageText(pageIdx uint32) (string, error) {
	filename := docPos.textPath(pageIdx)
	b, err := ioutil.ReadFile(filename)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// textPath returns the path to the file holding the extracted text of the page with index `pageIdx`.
func (docPos *DocPositions) textPath(pageIdx uint32) string {
	return filepath.Join(docPos.textDir, fmt.Sprintf("%03d.txt", pageIdx))
}
