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

// DocPositions is used to the link per-document data in a bleve index to the PDF the data was
// extracted from.
// There is one DocPositions per PDF.
type DocPositions struct {
	inPath        string                   // Path of input PDF.
	docIdx        uint64                   // Index into blevePdf.fileList.
	pagePositions map[uint32]PagePositions // {pageNum: locations of text on page}
	*docPersist                            // Optional extra fields for on-disk indexes.
}

// docPersist tracks the info for indexing a PDF on disk.
type docPersist struct {
	dataFile       *os.File        // Positions are stored in this file.
	pagePartitions []pagePartition // Indexes into `dataFile`. There is a pagePartition per page.
	dataPath       string          // Path of `dataFile`.
	partitionsPath string          // Path where `pagePartitions` is saved.
	textDir        string          // Extracted text. Used for debugging
}

// pagePartition is the location of the bytes of a PagePositions in a data file.
// The partition is over [Offset, Offset+Size).
// There is one pagePartition (corresponding to a PagePositions) per page.
type pagePartition struct {
	Offset  uint32 // Offset in the data file for the PagePositions for a page.
	Size    uint32 // Size of the PagePositions in the data file.
	Check   uint32 // CRC checksum for the PagePositions data.
	PageNum uint32 // PDF page number.
}

// String returns a human readable string describing `docPos`.
func (docPos DocPositions) String() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "DocPositions{%q docIdx=%d", filepath.Base(docPos.inPath), docPos.docIdx)
	sb.WriteString(docPos.docPersist.String())
	sb.WriteString("}")
	return sb.String()
}

// Len returns the number of pages in `docPos`.
func (docPos DocPositions) Len() int {
	return len(docPos.pageKeys()) // !@#$
}

// check panics is `docPos` is an inconsistent state, which should never happen.
func (docPos DocPositions) check() {
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
	for i, partition := range d.pagePartitions {
		parts = append(parts, fmt.Sprintf("\t%2d: %v", i+1, partition))
	}
	return fmt.Sprintf("docPersist{%s}", strings.Join(parts, "\n"))
}

// openDoc opens `docPos`. The necessary files are opened.
func (docPos *DocPositions) openDoc() error {
	f, err := os.Open(docPos.dataPath)
	if err != nil {
		return err
	}
	docPos.dataFile = f

	b, err := ioutil.ReadFile(docPos.partitionsPath)
	if err != nil {
		return err
	}
	var pagePartitions []pagePartition
	if err := json.Unmarshal(b, &pagePartitions); err != nil {
		return err
	}
	docPos.pagePartitions = pagePartitions
	return nil
}

// Save saves `docPos` to disk if it peristent.
func (docPos *DocPositions) Save() error {
	b, err := json.MarshalIndent(docPos.pagePartitions, "", "\t")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(docPos.partitionsPath, b, 0666)
}

// Close closes `docPos`'s open files.
func (docPos *DocPositions) Close() error {
	if err := docPos.Save(); err != nil {
		return err
	}
	return docPos.dataFile.Close()
}

// AddDocPage adds a page with (1-offset) page number `pageNum` and contents `ppos` to `docPos`.
// It returns the page index that can be used to access this page from ReadPagePositions().
// TODO: Can we remove `text` param for production code? By the time this function is called we have
// already indexed the text.
func (docPos *DocPositions) AddDocPage(pageNum uint32, ppos PagePositions, text string) (
	uint32, error) {
	if pageNum == 0 {
		return 0, errors.New("pageNum=0")
	}
	docPos.pagePositions[pageNum] = ppos
	return docPos.addDocPagePersist(pageNum, ppos, text)
}

// !@#$ Do we need to be writing to disk here?
func (docPos *DocPositions) addDocPagePersist(pageNum uint32, ppos PagePositions, text string) (
	uint32, error) {
	b := flatbuffers.NewBuilder(0)
	buf := serial.MakeDocPageLocations(b, ppos.offsetBBoxes)
	check := crc32.ChecksumIEEE(buf) // uint32
	offset, err := docPos.dataFile.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, err
	}

	partition := pagePartition{
		Offset:  uint32(offset),
		Size:    uint32(len(buf)),
		Check:   check,
		PageNum: uint32(pageNum),
	}

	if _, err := docPos.dataFile.Write(buf); err != nil {
		return 0, err
	}

	docPos.pagePartitions = append(docPos.pagePartitions, partition)
	pageIdx := uint32(len(docPos.pagePartitions) - 1)

	// !@#$ Remove. Maybe record line numbers
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
	return docPos.readPersistedPageText(pageIdx)
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

// pageNumPositions returns the page number (1-offset) and PagePositions of the text on the `pageIdx`
// (0-offset) in `docPos`.
func (docPos *DocPositions) pageNumPositions(pageIdx uint32) (uint32, PagePositions, error) {
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
	e := docPos.pagePartitions[pageIdx]
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

// DocPageText contains doc:page indexes, the PDF page number and the text extracted from the PDF page.
type DocPageText struct {
	DocIdx  uint64 // Doc index (0-offset) into BlevePdf.fileList .
	PageIdx uint32 // Page index (0-offset) into DocPositions.index .
	PageNum uint32 // Page number in PDF (1-offset)
	Text    string // Extracted page text.
}
