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

const usage = `Usage: go run search.go [OPTIONS] PaperCut NG
  Performs a full text search for "PaperCut NG" over PDF pages in the current index.
`

func main() {
	persistDir := filepath.Join(pdfsearch.DefaultPersistRoot, "my.computer")
	var serialize bool
	var nameOnly bool
	maxSearchResults := 10
	outPath := "search.results.pdf"
	outDir := "search.history"

	flag.StringVar(&outPath, "o", outPath, "Name of PDF file that will show marked up results.")
	flag.StringVar(&persistDir, "s", persistDir, "The on-disk index is stored here.")
	flag.BoolVar(&serialize, "m", serialize, "Serialize in-memory index to byte array.")
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

	// Run the tests.
	if err := runSearchShow(term, persistDir, nameOnly, maxResults, outPath); err != nil {
		fmt.Fprintf(os.Stderr, "runSearchShow failed. err=%v\n", err)
		os.Exit(1)
	}
	// Save a copy of the marked up file for posterity.
	if err := copyMarkedupResults(outDir, outPath, termExt); err != nil {
		fmt.Fprintf(os.Stderr, "copyMarkedupResults failed. err=%v\n", err)
		os.Exit(1)
	}
}

// runSearchShow searches for `term` in current index and shows the results.
// It also creates a marked-up PDF containing the original PDF pages with the matched terms marked
//  and saves it to `outPath`.
//
//  `nameOnly`: Show matching file names only.
//  `maxResults`: Max number of search results to return.
func runSearchShow(term, persistDir string, nameOnly bool, maxResults int, outPath string) error {
	results, dt, err := runSearch(term, persistDir, maxResults)
	if err != nil {
		return err
	}
	return showResults(results, dt, nameOnly, maxResults, outPath)
}

// runSearch searches for `term` in the PDF index stored in directory `persistDir` and returns the
//the search results and the  search duration.
// This is the main function. It shows you how to search a persistent index.
//
//  `maxResults`: Max number of search results to return.
func runSearch(term, persistDir string, maxResults int) (
	results pdfsearch.PdfMatchSet, dt time.Duration, err error) {
	t0 := time.Now()
	pdfIndex := pdfsearch.ReuseIndex(persistDir)
	results, err = pdfIndex.Search(term, maxResults)
	dt = time.Since(t0)
	return results, dt, err
}

// showResults writes a report on `results`, some search results (for a term that we don't show
//  here) on `pdfIndex` that was build from the PDF files in `pathList`.
// It also creates a marked-up PDF containing the original PDF pages with the matched terms marked
//  and saves it to `outPath`.
//
//  `dt` is the duration of the search.
//  `nameOnly`: Show matching file names only.
//  `maxResults`: Max number of search results to return.
func showResults(results pdfsearch.PdfMatchSet, dt time.Duration, nameOnly bool, maxResults int, outPath string) error {
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

	fmt.Fprintf(os.Stderr, "Duration=%.1f sec\n"+
		"Marked up search results in %q\n",
		dt.Seconds(), outPath)
	return nil
}

// copyMarkedupResults saves a copy of the PDF in `outPath` in the search history directory `outDir`.
// The file name is synthesized from `pathPattern`, the PDFs in the index and `termExt` the search
// term with spaces replaced by periods.
func copyMarkedupResults(outDir, outPath, termExt string) error {
	base := "SEARCH"
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
