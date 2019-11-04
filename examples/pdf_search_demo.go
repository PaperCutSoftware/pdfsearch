// Copyright 2019 PaperCut Software International Pty Ltd. All rights reserved.

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/papercutsoftware/pdfsearch"
	"github.com/papercutsoftware/pdfsearch/examples/cmd_utils"
	"github.com/papercutsoftware/pdfsearch/internal/utils"
)

const usage = `Usage: go run pdf_search_demo.go [OPTIONS] -f "pcng-manual*.pdf"  PaperCut NG
  Performs a full text search for "PaperCut NG" in PDFs that match "pcng-manual*.pdf".
`

func main() {
	var pathPattern string
	persistDir := filepath.Join(pdfsearch.DefaultPersistRoot, "pdf_search_demo")
	var reuse bool
	var nameOnly bool
	maxSearchResults := 10
	outPath := "search.results.pdf"
	outDir := "search.history"

	flag.StringVar(&pathPattern, "f", pathPattern, "PDF(s) to index.")
	flag.StringVar(&outPath, "o", outPath, "Name of PDF that will show marked up results.")
	flag.StringVar(&persistDir, "s", persistDir, "The on-disk index is stored here.")
	flag.BoolVar(&reuse, "r", reuse, "Reused stored index on disk for the last -p run.")
	flag.BoolVar(&nameOnly, "l", nameOnly, "Show matching file names only.")
	flag.IntVar(&maxSearchResults, "n", maxSearchResults, "Max number of search results to return.")

	cmd_utils.MakeUsage(usage)
	flag.Parse()

	if len(flag.Args()) < 1 {
		flag.Usage()
		os.Exit(1)
	}

	// We always want to see all errors in our testing.
	pdfsearch.ExposeErrors()

	// The term to search for.
	term := strings.Join(flag.Args(), " ")
	// File extension based on term.
	termExt := strings.Join(flag.Args(), ".")

	maxResults := maxSearchResults
	if nameOnly {
		maxResults = 1e9
	}

	// Read the files to index into `pathList`.
	var err error
	var pathList []string
	if !reuse {
		pathList, err = cmd_utils.PatternsToPaths([]string{pathPattern}, true)
		if err != nil {
			fmt.Fprintf(os.Stderr, "PatternsToPaths failed. args=%#q err=%v\n", flag.Args(), err)
			os.Exit(1)
		}
		if len(pathList) < 1 {
			fmt.Fprintf(os.Stderr, "No files matching %q.\n", pathPattern)
			os.Exit(1)
		}
	}

	// Run the tests.
	if err := runIndexSearchShow(pathList, term, persistDir, reuse, nameOnly, maxResults, outPath); err != nil {
		fmt.Fprintf(os.Stderr, "runIndexSearchShow failed. err=%v\n", err)
		os.Exit(1)
	}
	// Save a copy of the marked up file for posterity.
	if err := copyMarkedupResults(outDir, outPath, pathPattern, termExt); err != nil {
		fmt.Fprintf(os.Stderr, "copyMarkedupResults failed. err=%v\n", err)
		os.Exit(1)
	}
}

// runIndexSearchShow creates a pdfsearch.PdfIndex for the PDFs in `pathList`, searches for
// `term` in this index, and shows the results.
// It also creates a marked-up PDF containing the original PDF pages with the matched terms marked
//  and saves it to `outPath`.
//
//  `persistDir`: The directory the pdfsearch.PdfIndex is saved in.
//  `reuse`: Don't create a pdfsearch.PdfIndex. Reuse one that was previously persisted to disk.
//  `nameOnly`: Show matching file names only.
//  `maxResults`: Max number of search results to return.
func runIndexSearchShow(pathList []string, term, persistDir string, reuse, nameOnly bool,
	maxResults int, outPath string) error {
	pdfIndex, results, dt, dtIndex, err := runIndexSearch(pathList, term, persistDir, reuse, maxResults)
	if err != nil {
		return err
	}
	return showResults(pathList, pdfIndex, results, dt, dtIndex, nameOnly, maxResults, outPath)
}

// runIndexSearch creates a pdfsearch.PdfIndex for the PDFs in `pathList`, searches for `term`
//  in this index and returns the pdfsearch.PdfIndex, the search results and the indexing and search
//  durations.
// This is the main function. It shows you how to create an index annd search it.
//
//  `persistDir`: The directory the pdfsearch.PdfIndex is saved.
//  `reuse`: Don't create a pdfsearch.PdfIndex. Reuse one that was previously persisted to disk.
//  `maxResults`: Max number of search results to return.
func runIndexSearch(pathList []string, term, persistDir string, reuse bool, maxResults int) (
	pdfIndex pdfsearch.PdfIndex, results pdfsearch.PdfMatchSet, dt, dtIndex time.Duration, err error) {
	t0 := time.Now()

	if reuse {
		pdfIndex = pdfsearch.ReuseIndex(persistDir)
	} else {
		pdfIndex, err = pdfsearch.IndexPdfFiles(pathList, persistDir, report)
		if err != nil {
			return pdfIndex, results, dt, dtIndex, err
		}
	}

	dtIndex = time.Since(t0)

	results, err = pdfIndex.Search(term, maxResults)
	if err != nil {
		return pdfIndex, results, dt, dtIndex, err
	}

	dt = time.Since(t0)

	return pdfIndex, results, dt, dtIndex, nil
}

// showResults writes a report on `results`, some search results (for a term that we don't show
//  here) on `pdfIndex` that was build from the PDFs in `pathList`.
// It also creates a marked-up PDF containing the original PDF pages with the matched terms marked
//  and saves it to `outPath`.
//
//  `dt` and `dtSearch` are the durations of the indexing + search, and for indexing only.
//  `reuse`: Don't create a pdfsearch.PdfIndex. Reuse one that was previously persisted to disk.
//  `nameOnly`: Show matching file names only.
//  `maxResults`: Max number of search results to return.
func showResults(pathList []string, pdfIndex pdfsearch.PdfIndex, results pdfsearch.PdfMatchSet,
	dt, dtIndex time.Duration, nameOnly bool, maxResults int, outPath string) error {
	if nameOnly {
		files := results.Files()
		if len(files) > maxResults {
			files = files[:maxResults]
		}
		for i, fn := range files {
			fmt.Printf("%4d: %q\n", i, fn)
		}
	} else {
		fmt.Printf("%+v\n", results)
	}

	if err := pdfsearch.MarkupPdfResults(results, outPath); err != nil {
		return err
	}

	numPages := pdfIndex.NumPages()
	pagesSec := 0.0
	if dt.Seconds() >= 0.01 {
		pagesSec = float64(numPages) / dt.Seconds()
	}
	showList := pathList
	if len(showList) > 10 {
		showList = showList[:10]
		for i, fn := range showList {
			showList[i] = filepath.Base(fn)
		}
	}

	dtSearch := dt - dtIndex

	fmt.Fprintf(os.Stderr, "Duration=%.1f sec (%.3f index + %.3f search) (%.1f pages/min) "+
		"%d pages in %d files %+v\n"+
		"Index duration=%s\n"+
		"Marked up search results in %q\n",
		dt.Seconds(), dtIndex.Seconds(), dtSearch.Seconds(), pagesSec*60.0,
		numPages, len(pathList), showList,
		pdfIndex.Duration(),
		outPath)
	return nil
}

// copyMarkedupResults saves a copy of the PDF in `outPath` in the search history directory `outDir`.
// The file name is synthesized from `pathPattern`, the PDFs in the index and `termExt` the search
// term with spaces replaced by periods.
func copyMarkedupResults(outDir, outPath, pathPattern, termExt string) error {
	base := filepath.Base(pathPattern)
	base = strings.Replace(base, "*", "_all_", -1)
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

// `report` is called  by IndexPdfMem to report progress.
func report(msg string) {
	fmt.Fprintf(os.Stderr, ">> %s\n", msg)
}
