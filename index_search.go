// Copyright 2019 PaperCut Software International Pty Ltd. All rights reserved.

/*
 * Full text search of a list of PDFs.
 *
 * Call like this.
 *   p, err := IndexPdfFiles(pathList) creates a PdfIndex `p` for the PDFs in `pathList`.
 *   m, err := p.Search(term, -1) searches `p` for string `term`.
 *
 * e.g.
 *   pathList := []string{"PDF32000_2008.pdf"}
 *   p, _ := pdf.IndexPdfFiles(pathList, false)
 *   matches, _ := p.Search("Type 1", -1)
 *   fmt.Printf("Matches=%s\n", matches)
 */

package pdfsearch

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/blevesearch/bleve"
	"github.com/papercutsoftware/pdfsearch/internal/doclib"
	"github.com/papercutsoftware/pdfsearch/internal/utils"
	"github.com/unidoc/unipdf/v3/common"
)

// InitLogging makes doclib.InitLogging public.
var InitLogging func() = doclib.InitLogging

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

// IndexPdfFiles returns an index for the PDFs in `pathList`.
// The index is stored on disk in `persistDir`.
// `report` is a supplied function that is called to report progress.
func IndexPdfFiles(pathList []string, persistDir string, report func(string)) (PdfIndex, error) {
	t0 := time.Now()
	_, bleveIdx, numFiles, numPages, dtPdf, dtBleve, err := doclib.IndexPdfFiles(pathList,
		persistDir, false, report)
	if err != nil {
		return PdfIndex{}, err
	}
	if bleveIdx != nil {
		bleveIdx.Close()
	}
	dt := time.Since(t0)
	return PdfIndex{
		persistDir: persistDir,
		numFiles:   numFiles,
		numPages:   numPages,
		dt:         dt,
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

	s, err := doclib.SearchPdfIndex(p.persistDir, term, maxResults)
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

// PdfIndex is an opaque struct that describes an index over some PDFs.
// It consists of
// - a bleve index (bleveIdx)
// - a mapping between the PDFs and the bleve index (blevePdf)
// - controls and statistics.
type PdfIndex struct {
	persistDir string           // Root directory for storing on-disk indexes.
	bleveIdx   bleve.Index      // The bleve index used on text extracted from PDFs.
	blevePdf   *doclib.BlevePdf // Mapping between the PDFs and the bleve index.
	numFiles   int              // Number of PDFs indexes.
	numPages   int              // Total number of PDF pages indexed.
	dt         time.Duration    // Total indexing time.
	dtPdf      time.Duration    // The time it took to extract text from PDFs.
	dtBleve    time.Duration    // The time it tool to build the bleve index.
	reused     bool             // Did on-disk index exist before we ran? Helpful for debugging.
}

// String returns a string describing `p`.
func (p PdfIndex) String() string {
	d := p.Duration()
	var b string
	if p.blevePdf != nil {
		b = fmt.Sprintf(" blevePdf=%s", p.blevePdf.String())
	}
	return fmt.Sprintf("PdfIndex{numFiles=%d numPages=%d Duration=%s%s}",
		p.numFiles, p.numPages, d, b)
}

// Duration returns a string describing how long indexing took and where the time was spent.
func (p PdfIndex) Duration() string {
	return fmt.Sprintf("%.3f sec(%.3f PDF, %.3f bleve)",
		p.dt.Seconds(), p.dtPdf.Seconds(), p.dtBleve.Seconds())
}

func (p PdfIndex) NumFiles() int {
	return p.numFiles
}

// NumPages returns the total number of PDF pages in PdfIndex `p`? !@#$ Is this possible?
func (p PdfIndex) NumPages() int {
	return p.numPages
}

// ExposeErrors turns off recovery from panics in called libraries.
func ExposeErrors() {
	doclib.ExposeErrors = true
	doclib.CheckConsistency = true
}

// CopyMarkedupResults saves a copy of the PDF in `outPath` in the search history directory `outDir`.
// The file name is synthesized from `pathPattern`, the PDFs in the index and `termExt` the search
// term with spaces replaced by periods.
func CopyMarkedupResults(outDir, outPath, pathPattern, termExt string) error {
	var base string
	if pathPattern != "" {
		base = filepath.Base(pathPattern)
		base = strings.Replace(base, "*", "_all_", -1)
	} else {
		base = "search"
	}
	base = utils.ChangePathExt(base, "")
	base = fmt.Sprintf("%s.%s.pdf", base, termExt)

	outPath2 := filepath.Join(outDir, base)
	err := utils.MkDir(outDir)
	if err != nil {
		return err
	}
	err = utils.CopyFile(outPath, outPath2)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Marked up search results in %q\n", outPath2)
	return nil
}
