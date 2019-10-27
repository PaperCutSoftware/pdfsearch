// Copyright 2019 PaperCut Software International Pty Ltd. All rights reserved.

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/papercutsoftware/pdfsearch"
	"github.com/papercutsoftware/pdfsearch/examples/cmd_utils"
)

const usage = `Usage: go run index.go [OPTIONS]  pcng-manual*.pdf
  Adds PDF files that match "pcng-manual*.pdf" to the index
`

func main() {
	persistDir := filepath.Join(pdfsearch.DefaultPersistRoot, "my.computer")
	flag.StringVar(&persistDir, "s", persistDir, "The on-disk index is stored here.")
	cmd_utils.MakeUsage(usage)
	flag.Parse()

	if len(flag.Args()) < 1 {
		flag.Usage()
		os.Exit(1)
	}

	// We always want to see all errors in our testing.
	pdfsearch.ExposeErrors()

	// Read the files to index into `pathList`.
	pathList, err := cmd_utils.PatternsToPaths(flag.Args(), true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "PatternsToPaths failed. args=%#q err=%v\n", flag.Args(), err)
		os.Exit(1)
	}
	pathList = cmd_utils.CleanCorpus(pathList) //!@#$ pc-only
	if len(pathList) < 1 {
		fmt.Fprintf(os.Stderr, "No files matching %q.\n", flag.Args())
		os.Exit(1)
	}

	// Run the tests.
	if err := runIndexShow(pathList, persistDir); err != nil {
		fmt.Fprintf(os.Stderr, "runIndexShow failed. err=%v\n", err)
		os.Exit(1)
	}
}

// runIndexShow creates a pdfsearch.PdfIndex for the PDF files in `pathList`, searches for
// `term` in this index, and shows the results.
//  `persistDir`: The directory the pdfsearch.PdfIndex is saved in if `persist` is true.
func runIndexShow(pathList []string, persistDir string) error {

	pdfIndex, dt, err := runIndex(pathList, persistDir)
	if err != nil {
		return err
	}
	return showIndex(pathList, pdfIndex, dt)
}

// runIndex creates a pdfsearch.PdfIndex for the PDF files in `pathList` and returns the
// pdfsearch.PdfIndex, the search results and the indexing duration.
// Rhe pdfsearch.PdfIndex is saved in directory `persistDir`.
// This is the main function. It shows you how to create or open an index.
func runIndex(pathList []string, persistDir string) (pdfIndex pdfsearch.PdfIndex, dt time.Duration,
	err error) {
	fmt.Fprintf(os.Stderr, "Indexing %d files. Index stored in %q.\n", len(pathList), persistDir)

	t0 := time.Now()
	pdfIndex, err = pdfsearch.IndexPdfFiles(pathList, true, persistDir, report)
	if err != nil {
		return pdfIndex, dt, err
	}
	dt = time.Since(t0)
	return pdfIndex, dt, nil
}

// showIndex writes a report on  on `pdfIndex` that was build from the PDF files in `pathList`.
// `dt` is the duration of the indexing.
func showIndex(pathList []string, pdfIndex pdfsearch.PdfIndex, dt time.Duration) error {
	numFiles := pdfIndex.NumFiles()
	numPages := pdfIndex.NumPages()
	pagesSec := 0.0
	if dt.Seconds() >= 0.01 {
		pagesSec = float64(numPages) / dt.Seconds()
	}
	fmt.Fprintf(os.Stderr, "%d pages in %d files (%.1f pages/min)\n",
		numPages, numFiles, pagesSec*60.0)
	fmt.Fprintf(os.Stderr, "%s\n", pdfIndex)
	return nil
}

// `report` is called  by IndexPdfMem to report progress.
func report(msg string) {
	fmt.Fprintf(os.Stderr, ">> %s\n", msg)
}
