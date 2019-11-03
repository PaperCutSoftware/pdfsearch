// Copyright 2019 PaperCut Software International Pty Ltd. All rights reserved.

/*
 * Full text search of a list of PDF files.
 *
 * Call like this.
 *   p, err := IndexPdfFiles(pathList) creates a PdfIndex `p` for the PDF files in `pathList`.
 *   m, err := p.Search(term, -1) searches `p` for string `term`.
 *
 * e.g.
 *   pathList := []string{"PDF32000_2008.pdf"}
 *   p, _ := pdf.IndexPdfFiles(pathList, false)
 *   matches, _ := p.Search("Type 1", -1)
 *   fmt.Printf("Matches=%s\n", matches)
 *
 * There are 2 ways of reading PDF files
 *   1) By filename.
 *         IndexPdfFiles()
 *   2) By io.ReadSeeker
 *         IndexPdfReaders()
 * The io.ReadSeeker methods are for callers that don't have access to the PDF files on a file
 * system.  TODO: Ask Geoff why he needs this.
 */

package pdfsearch

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/blevesearch/bleve"
	"github.com/papercutsoftware/pdfsearch/internal/doclib"
	"github.com/unidoc/unipdf/v3/common"
)

// PdfMatchSet makes doclib.PdfMatchSet public.
type PdfMatchSet doclib.PdfMatchSet

// String()  makes doclib.PdfMatchSet.String public.
func (s PdfMatchSet) String() string {
	return doclib.PdfMatchSet(s).String()
}

// Files  makes doclib.PdfMatchSet.Files public.
func (s PdfMatchSet) Files() []string {
	return doclib.PdfMatchSet(s).Files()
}

// Equals makes doclib.PdfMatchSet.Equals public.
func (s PdfMatchSet) Equals(t PdfMatchSet) bool {
	return doclib.PdfMatchSet(s).Equals(doclib.PdfMatchSet(t))
}

// Best makes doclib.PdfMatchSet.Best public.
func (s PdfMatchSet) Best() PdfMatchSet {
	return PdfMatchSet(doclib.PdfMatchSet(s).Best())
}

const (
	// DefaultMaxResults is the default maximum number of results returned.
	DefaultMaxResults = 10
	// DefaultPersistRoot is the default root for on-disk indexes.
	DefaultPersistRoot = "pdf.store"
)

// IndexPdfFiles returns an index for the PDF files in `pathList`.
// The index is stored on disk in `persistDir`.
// `report` is a supplied function that is called to report progress.
func IndexPdfFiles(pathList []string, persistDir string, report func(string)) (PdfIndex, error) {
	// A nil rsList makes IndexPdfFilesOrReaders uses paths.
	return IndexPdfReaders(pathList, nil, persistDir, report)
}

// IndexPdfReaders returns a PdfIndex over the PDF contents read by the io.ReaderSeeker's in `rsList`.
// The names of the PDFs are in the corresponding position in `pathList`.
// The index is stored on disk in `persistDir`.
// `report` is a supplied function that is called to report progress.
func IndexPdfReaders(pathList []string, rsList []io.ReadSeeker, persistDir string,
	report func(string)) (PdfIndex, error) {
	_, bleveIdx, numFiles, numPages, dtPdf, dtBleve, err := doclib.IndexPdfFilesOrReaders(pathList,
		rsList, persistDir, true, report)
	if err != nil {
		return PdfIndex{}, err
	}
	if bleveIdx != nil {
		bleveIdx.Close()
	}

	return PdfIndex{
		persistDir: persistDir,
		numFiles:   numFiles,
		numPages:   numPages,
		readSeeker: len(rsList) > 0,
		dtPdf:      dtPdf,
		dtBleve:    dtBleve,
	}, nil
}

// ReuseIndex returns an existing on-disk PdfIndex with directory `persistDir`.
func ReuseIndex(persistDir string) PdfIndex {
	return PdfIndex{
		reused:     true,
		persistDir: persistDir,
	}
}

// Search does a full-text search over PdfIndex `p` for `term` and returns up to `maxResults` matches.
// This is the main search function.
func (p PdfIndex) Search(term string, maxResults int) (PdfMatchSet, error) {
	if maxResults < 0 {
		maxResults = DefaultMaxResults
	}
	common.Log.Debug("maxResults=%d DefaultMaxResults=%d", maxResults, DefaultMaxResults)

	s, err := doclib.SearchPersistentPdfIndex(p.persistDir, term, maxResults)
	if err != nil {
		return PdfMatchSet{}, err
	}

	results := PdfMatchSet(s)
	common.Log.Debug("PdfIndex.Search: results (before)================|||================")
	common.Log.Debug("%s", results.String())
	// This is where were we select the best results to show
	results = results.Best()
	common.Log.Debug("PdfIndex.Search: results (after )================***================")
	common.Log.Debug("%s", results.String())
	common.Log.Debug("PdfIndex.Search: results (after )================---================")
	return results, err
}

// MarkupPdfResults adds rectangles to the text positions of all matches on their PDF pages,
// combines these pages together and writes the resulting PDF to `outPath`.
// The PDF will have at most 100 pages because no-one is likely to read through search results of
// over more than 100 pages. There will at most 100 results per page.
func MarkupPdfResults(results PdfMatchSet, outPath string) error {
	maxPages := 100
	maxPerPage := 100
	extractList := doclib.CreateExtractList(maxPages, maxPerPage)
	common.Log.Debug("=================!!!=====================")
	common.Log.Debug("Matches=%d", len(results.Matches))
	for i, m := range results.Matches {
		inPath := m.InPath
		pageNum := m.PageNum
		ppos := m.PagePositions
		common.Log.Debug("  %d: ppos=%s m=%s", i, ppos, m)
		if ppos.Empty() {
			return errors.New("no Locations")
		}
		for _, span := range m.Spans {
			bbox, ok := ppos.BBox(span.Start, span.End)
			if !ok {
				common.Log.Info("No bbox for m=%s span=%v", m, span)
				continue
			}
			extractList.AddRect(inPath, pageNum, bbox)
		}
	}
	return extractList.SaveOutputPdf(outPath)
}

// PdfIndex is an opaque struct that describes an index over some PDF files.
// It consists of
// - a bleve index (bleveIdx)
// - a mapping between the PDF files and the bleve index (blevePdf)
// - controls and statistics.
type PdfIndex struct {
	persistDir string           // Root directory for storing on-disk indexes.
	bleveIdx   bleve.Index      // The bleve index used on text extracted from PDF files.
	blevePdf   *doclib.BlevePdf // Mapping between the PDF files and the bleve index.
	numFiles   int              // Number of PDF files indexes.
	numPages   int              // Total number of PDF pages indexed.
	dtPdf      time.Duration    // The time it took to extract text from PDF files.
	dtBleve    time.Duration    // The time it tool to build the bleve index.
	reused     bool             // Did on-disk index exist before we ran? Helpful for debugging.
	readSeeker bool             // Were io.ReadSeeker functions used. Helpful for debugging.
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
	if !p.blevePdf.Equals(q.blevePdf) {
		common.Log.Error("PdfIndex.Equals.blevePdf:")
		return false
	}
	return true
}

// String returns a string describing `p`.
func (p PdfIndex) String() string {
	s := p.StorageName()
	d := p.Duration()
	var b string
	if p.blevePdf != nil {
		b = fmt.Sprintf(" levePdf=%s", p.blevePdf.String())
	}
	return fmt.Sprintf("PdfIndex{[%s index] numFiles=%d numPages=%d Duration=%s%s}",
		s, p.numFiles, p.numPages, d, b)
}

// Duration returns a string describing how long indexing took and where the time was spent.
func (p PdfIndex) Duration() string {
	return fmt.Sprintf("%.3f sec(%.3f PDF, %.3f bleve)",
		p.dtPdf.Seconds()+p.dtBleve.Seconds(),
		p.dtPdf.Seconds(), p.dtBleve.Seconds())
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
	} else {
		storage = "On-disk"
	}
	if p.readSeeker {
		storage += " (ReadSeeker)"
	}
	return storage
}

// ExposeErrors turns off recovery from panics in called libraries.
func ExposeErrors() {
	doclib.ExposeErrors = true
	doclib.CheckConsistency = true
}
