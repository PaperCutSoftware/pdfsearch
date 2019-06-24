// Copyright 2019 PaperCut Software International Pty Ltd. All rights reserved.

/*
 * Full text search of a list of PDF files.
 *   p, err := IndexPdfFiles(pathList, persist) creates a PdfIndex `p` for the PDF files in `pathList`.
 *   m, err := p.Search(term, -1) searches `p` for string `term`.
 *
 * e.g.
 *   pathList := []string{"PDF32000_2008.pdf"}
 *   p, _ := pdf.IndexPdfFiles(pathList, false)
 *   matches, _ := p.Search("Type 1", -1)
 *   fmt.Printf("Matches=%s\n", matches)
 */

package pdf

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/blevesearch/bleve"
	"github.com/papercutsoftware/pdfsearch/doclib"
	"github.com/papercutsoftware/pdfsearch/serial"
	"github.com/unidoc/unipdf/v3/common"
)

const (
	// DefaultMaxResults is the default maximum number of results returned.
	DefaultMaxResults = 10
	// DefaultPersistDir is the default root for on-disk indexes.
	DefaultPersistDir = "store.pdf.index"
)

// IndexPdfFiles returns an index for the PDF files in `pathList`.
// If `persist` is false, the index is stored in memory.
// If `persist` is true, the index is stored on disk in `persistDir`.
// `report` is a supplied function that is called to report progress.
// If `useReaderSeeker` then a slice of io.ReadSeeker is passed to the PDF processing library. This
// should only be used for testing the io.ReadSeeker API.
func IndexPdfFiles(pathList []string, persist bool, persistDir string, report func(string),
	useReaderSeeker bool) (PdfIndex, error) {

	if !useReaderSeeker {
		return IndexPdfReaders(pathList, []io.ReadSeeker{}, persist, persistDir, report)
	}

	var rsList []io.ReadSeeker
	for _, inPath := range pathList {
		rs, err := os.Open(inPath)
		if err != nil {
			return PdfIndex{}, err
		}
		defer rs.Close()
		rsList = append(rsList, rs)
	}
	return IndexPdfReaders(pathList, rsList, persist, persistDir, report)
}

// IndexPdfMem returns a byte array that contains an index for PDF io.ReaderSeeker's in `rsList`.
// The names of the PDFs are in the corresponding position in `pathList`.
// `report` is a supplied function that is called to report progress.
func IndexPdfMem(pathList []string, rsList []io.ReadSeeker, report func(string)) ([]byte, error) {
	pdfIndex, err := IndexPdfReaders(pathList, rsList, false, "", report)
	if err != nil {
		return nil, err
	}
	data, err := pdfIndex.ToBytes()
	if err != nil {
		return nil, err
	}
	common.Log.Info("IndexPdfMem: hash=%s", sliceHash(data))
	return data, nil
}

// IndexPdfReaders returns a PdfIndex over the PDF contents read by the io.ReaderSeeker's in `rsList`.
// The names of the PDFs are in the corresponding position in `pathList`.
// If `persist` is false, the index is stored in memory.
// If `persist` is true, the index is stored on disk in `persistDir`.
// `report` is a supplied function that is called to report progress.
func IndexPdfReaders(pathList []string, rsList []io.ReadSeeker, persist bool, persistDir string,
	report func(string)) (PdfIndex, error) {

	if !persist {
		lState, bleveIdx, numPages, dtPdf, dtBleve, err := doclib.IndexPdfReaders(pathList, rsList,
			"", true, false, report)
		if err != nil {
			return PdfIndex{}, err
		}
		lState.Check()
		return PdfIndex{
			persist:    false,
			lState:     lState,
			bleveIdx:   bleveIdx,
			numFiles:   len(pathList),
			numPages:   numPages,
			readSeeker: len(rsList) > 0,
			dtPdf:      dtPdf,
			dtBleve:    dtBleve,
		}, nil
	}
	_, bleveIdx, numPages, dtPdf, dtBleve, err := doclib.IndexPdfReaders(pathList, rsList,
		persistDir, true, false, report)
	if err != nil {
		return PdfIndex{}, err
	}
	bleveIdx.Close()
	return PdfIndex{
		persist:    true,
		persistDir: persistDir,
		numFiles:   len(pathList),
		numPages:   numPages,
		dtPdf:      dtPdf,
		dtBleve:    dtBleve,
	}, nil
}

// ReuseIndex returns a reused on-disk PdfIndex with directory `persistDir`.
func ReuseIndex(persistDir string) PdfIndex {
	return PdfIndex{
		persist:    true,
		reused:     true,
		persistDir: persistDir,
	}
}

// SearchMem does a full-text search over the PdfIndex in `data` for `term` and returns up to
// `maxResults` matches.
// `data` is the serialized PdfIndex returned from IndexPdfMem.
func SearchMem(data []byte, term string, maxResults int) (doclib.PdfMatchSet, error) {
	common.Log.Info(" SearchMem: hash=%s", sliceHash(data))
	pdfIndex, err := FromBytes(data)
	pdfIndex.lState.Check()
	if err != nil {
		return doclib.PdfMatchSet{}, err
	}
	return pdfIndex.Search(term, maxResults)
}

// Search does a full-text search over PdfIndex `p` for `term` and returns up to `maxResults` matches.
// This is the main search function.
func (p PdfIndex) Search(term string, maxResults int) (doclib.PdfMatchSet, error) {
	if maxResults < 0 {
		maxResults = DefaultMaxResults
	}
	if !p.persist {
		p.lState.Check()
		return p.lState.SearchBleveIndex(p.bleveIdx, term, maxResults)
	}
	return doclib.SearchPersistentPdfIndex(p.persistDir, term, maxResults)
}

// MarkupPdfResults adds rectangles to the text positions of all matches on their PDF pages,
// combines these pages together and writes the resulting PDF to `outPath`,
func MarkupPdfResults(results doclib.PdfMatchSet, outPath string) error {
	extractList := doclib.CreateExtractList(100)
	common.Log.Info("=================!!!=====================")
	common.Log.Info("Matches=%d", len(results.Matches))
	for i, m := range results.Matches {
		inPath := m.InPath
		pageNum := m.PageNum
		dpl := m.DocPageLocations
		common.Log.Info("  %d: dpl=%s m=%s", i, dpl, m)
		if dpl.Len() == 0 {
			return errors.New("no Locations")
		}
		loc := dpl.GetPosition(m.Start, m.End)
		llx, lly, urx, ury := loc.Llx, loc.Lly, loc.Urx, loc.Ury
		extractList.AddRect(inPath, pageNum, llx, lly, urx, ury)
	}
	return extractList.SaveOutputPdf(outPath)
}

// PdfIndex is an opaque struct that describes an index over some PDF files.
// It consists of
// - a bleve index (bleveIdx),
// - a mapping between the PDF files and the index (lState)
// - controls and statistics.
type PdfIndex struct {
	persist    bool
	reused     bool
	readSeeker bool
	persistDir string
	lState     *doclib.PositionsState
	bleveIdx   bleve.Index
	numFiles   int
	numPages   int
	dtPdf      time.Duration
	dtBleve    time.Duration
}

// Equals returns true if `p` contains the same information as `q`.
func (p PdfIndex) Equals(q PdfIndex) bool {
	if p.numFiles != q.numFiles {
		common.Log.Error("PdfIndex.Equals.numFiles: %d %d\np=%s\nq=%s", p.numFiles, q.numFiles, p, q)
		return false
	}
	if p.numPages != q.numPages {
		common.Log.Error("PdfIndex.Equals.numPages: %d %d", p.numPages, q.numPages)
		return false
	}
	if !p.lState.Equals(q.lState) {
		common.Log.Error("PdfIndex.Equals.lState:")
		return false
	}
	return true
}

// String returns a string describing `p`.
func (p PdfIndex) String() string {
	return fmt.Sprintf("PdfIndex{[%s index] numFiles=%d numPages=%d duration=%s lState=%s}",
		p.StorageName(), p.numFiles, p.numPages, p.Duration(), p.lState.String())
}

func (p PdfIndex) Duration() string {
	return fmt.Sprintf("%.3f sec(PDF)+%.3f sec(bleve)=%.3f sec",
		p.dtPdf.Seconds(), p.dtBleve.Seconds(), p.dtPdf.Seconds()+p.dtBleve.Seconds())
}

func (p PdfIndex) NumFiles() int {
	return p.numFiles
}

func (p PdfIndex) NumPages() int {
	return p.numPages
}

// StorageName returns a descriptive name for index storage mode.
func (p PdfIndex) StorageName() string {
	storage := "In-memory"
	if p.reused {
		storage = "Reused"
	} else if p.persist {
		storage = "On-disk"
	}
	if p.readSeeker {
		storage += " (ReadSeeker)"
	}
	return storage
}

// ToBytes serializes `i` to a byte array.
func (i PdfIndex) ToBytes() ([]byte, error) {
	pdfMem, bleveMem, err := i.to2Bufs()
	if err != nil {
		return nil, err
	}
	return mergeBufs(pdfMem, bleveMem)
}

// from2Bufs extracts a PdfIndex from the bytes in `data`.
func FromBytes(data []byte) (PdfIndex, error) {
	pdfMem, bleveMem, err := splitBufs(data)
	if err != nil {
		return PdfIndex{}, err
	}
	return from2Bufs(pdfMem, bleveMem)
}

// to2Bufs serializes `i` to buffers `pdfMem` and `bleveMem`.
// `pdfMem` contains a serialized SerialPdfIndex.
// `bleveMem` contains a serialized bleve.Index.
func (i PdfIndex) to2Bufs() (pdfMem, bleveMem []byte, err error) {

	hipds, err := i.lState.ToHIPDs()
	if err != nil {
		return nil, nil, err
	}
	bleveMem, err = doclib.ExportBleveMem(i.bleveIdx)
	if err != nil {
		return nil, nil, err
	}
	spi := serial.SerialPdfIndex{
		NumFiles: uint32(i.numFiles),
		NumPages: uint32(i.numPages),
		HIPDs:    hipds,
	}
	pdfMem = serial.WriteSerialPdfIndex(spi)

	if len(pdfMem) == 0 || len(bleveMem) == 0 {
		common.Log.Error("Zero entry: pdfMem=%d bleveMem=%d", len(pdfMem), len(bleveMem))
	}
	if err := doclib.CompressInPlace(&pdfMem); err != nil {
		return nil, nil, err
	}
	if err := doclib.CompressInPlace(&bleveMem); err != nil {
		return nil, nil, err
	}
	return pdfMem, bleveMem, nil
}

// from2Bufs extracts a PdfIndex from the bytes in `pdfMem` and `bleveMem`.
// `pdfMem` contains a serialized SerialPdfIndex.
// `bleveMem` contains a serialized bleve.Index.
func from2Bufs(pdfMem, bleveMem []byte) (PdfIndex, error) {
	if err := doclib.DecompressInPlace(&pdfMem); err != nil {
		return PdfIndex{}, err
	}
	if err := doclib.DecompressInPlace(&bleveMem); err != nil {
		return PdfIndex{}, err
	}

	spi, err := serial.ReadSerialPdfIndex(pdfMem)
	if err != nil {
		return PdfIndex{}, err
	}
	lState, err := doclib.PositionsStateFromHIPDs(spi.HIPDs)
	if err != nil {
		return PdfIndex{}, err
	}
	bleveIdx, err := doclib.ImportBleveMem(bleveMem)
	if err != nil {
		return PdfIndex{}, err
	}
	i := PdfIndex{
		lState:   &lState,
		bleveIdx: bleveIdx,
		numFiles: int(spi.NumFiles),
		numPages: int(spi.NumPages),
	}
	common.Log.Trace("FromBytes: numFiles=%d numPages=%d lState=%s",
		i.numFiles, i.numPages, *i.lState)
	lState.Check()
	return i, nil
}

// mergeBufs combines `b1` and `b2` in a single byte array `b`.
// b1 and b2 can be retreived by splitBufs(`b`)
func mergeBufs(b1, b2 []byte) ([]byte, error) {
	n1 := uint32(len(b1))
	n2 := uint32(len(b2))

	p1, err := uint32ToBytes(n1)
	if err != nil {
		return nil, fmt.Errorf("mergeBufs: n1=%d err=%v", n1, err)
	}
	p2, err := uint32ToBytes(n2)
	if err != nil {
		return nil, fmt.Errorf("mergeBufs: n1=%d err=%v", n1, err)
	}
	b := make([]byte, wordSize*2+n1+n2)
	copy(b, p1)
	copy(b[wordSize:], p2)
	copy(b[wordSize*2:], b1)
	copy(b[wordSize*2+n1:], b2)

	common.Log.Debug("mergeBufs: b=%d n1=%d n2=%d\n\thash1=%s\n\thash2=%s",
		len(b), n1, n2, sliceHash(b1), sliceHash(b2))

	return b, nil
}

// splitBufs byte array `b` that was created by mergeBufs(b1, b2) into  b1 and b2.
func splitBufs(b []byte) (b1, b2 []byte, err error) {
	if len(b) < 2*wordSize {
		return nil, nil, fmt.Errorf("splitBufs: b=%d", len(b))
	}
	n1, err := uint32FromBytes(b[:wordSize])
	if err != nil {
		return nil, nil, fmt.Errorf("splitBufs: n1 err=%v", err)
	}
	n2, err := uint32FromBytes(b[wordSize : wordSize*2])
	if err != nil {
		return nil, nil, fmt.Errorf("splitBufs: n2 err=%v", err)
	}
	if wordSize*2+n1+n2 != uint32(len(b)) {
		return nil, nil, fmt.Errorf("splitBufs: n1=%d n2=%d b=%d", n1, n2, len(b))
	}
	b1 = b[wordSize*2 : wordSize*2+n1]
	b2 = b[wordSize*2+n1:]

	common.Log.Debug("splitBufs: b=%d n1=%d n2=%d\n\thash1=%s\n\thash2=%s",
		len(b), n1, n2, sliceHash(b1), sliceHash(b2))
	if n1 == 0 || n2 == 0 {
		return nil, nil, fmt.Errorf("splitBufs: empty n1=%d n2=%d b=%d", n1, n2, len(b))
	}
	return b1, b2, nil
}

// wordSize is the number of bytes of the size fields in mergeBufs() and splitBufs().
const wordSize = 4

// uint32ToBytes converts `n` into a little endian byte array.
func uint32ToBytes(n uint32) ([]byte, error) {
	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.LittleEndian, n)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// uint32ToBytes little endian byte array `b` into an integer.
func uint32FromBytes(b []byte) (uint32, error) {
	var n uint32
	buf := bytes.NewReader(b)
	err := binary.Read(buf, binary.LittleEndian, &n)
	if err != nil {
		return 0, err
	}
	return n, nil
}

// sliceHash returns a SHA-1 hash of `data` as a hexidecimal string.
func sliceHash(data []byte) string {
	h := sha1.New()
	h.Write(data)
	return fmt.Sprintf("%x", h.Sum(nil))
}
