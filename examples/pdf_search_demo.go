// Copyright 2019 PaperCut Software International Pty Ltd. All rights reserved.

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	pdfsearch "github.com/papercutsoftware/pdfsearch"
	"github.com/papercutsoftware/pdfsearch/doclib"
)

// TODO: Implement -m indexing. Needs bleve PR.
const usage = `Usage: go run pdf_search_demo.go [OPTIONS] -f "pcng-manual*.pdf"  PaperCut NG
  Performs a full text search for "PaperCut NG" in PDF files that match "pcng-manual*.pdf".
  There are 3 modes of indexing:
   1) in-memory unserialized: default
   2) in-memory serialized: -m
   3) on-disk: -p
  There are several ways of grouping files in the index from this test program:
   a) all files on the command line are recorded in one index: default
   b) files are split into groups of <n> and each group is indexed and searched: -g <n>
   c) groups of files are indexed and searched twice, in-memory serialized and unserialzed, and
      the results are compared: -c
   d) groups of files are indexed and searched thrice, in-memory serialized and unserialzed and
     on-disk, and the results are compared: -cd
`

func main() {
	var pathPattern string
	var persistDir string
	var serialize bool
	var persist bool
	var reuse bool
	var nameOnly bool
	maxSearchResults := 10
	outPath := "search.results.pdf"

	flag.StringVar(&pathPattern, "f", pathPattern, "PDF file(s) to index.")
	flag.StringVar(&outPath, "o", outPath, "Name of PDF file that will show marked up results.")
	flag.StringVar(&persistDir, "s", pdfsearch.DefaultPersistDir, "The on-disk index is stored here.")
	flag.BoolVar(&serialize, "m", serialize, "Serialize in-memory index to byte array.")
	flag.BoolVar(&persist, "p", persist, "Store index on disk (slower but allows more PDF files).")
	flag.BoolVar(&reuse, "r", reuse, "Reused stored index on disk for the last -p run.")
	flag.BoolVar(&nameOnly, "l", nameOnly, "Show matching file names only.")
	flag.IntVar(&maxSearchResults, "n", maxSearchResults, "Max number of search results to return.")

	doclib.MakeUsage(usage)
	flag.Parse()

	if len(flag.Args()) < 1 {
		flag.Usage()
		os.Exit(1)
	}

	// We always want to see all errors in our testing.
	doclib.ExposeErrors = true

	// The term to search for.
	term := strings.Join(flag.Args(), " ")

	// Resolve conflicts in command line options.
	if reuse && serialize {
		fmt.Fprintf(os.Stderr,
			"Memory-serialized stores cannot be reused. Using unserialized store.")
		serialize = false
		persist = true
	}
	if persist && serialize {
		fmt.Fprintf(os.Stderr,
			"On-disk stores cannot be serialized to memory. Using unserialized store.")
		serialize = false
	}
	maxResults := maxSearchResults
	if nameOnly {
		maxResults = 1e9
	}

	// Read the files to index into `pathList`.
	var err error
	var pathList []string
	if !reuse {
		pathList, err = doclib.PatternsToPaths([]string{pathPattern}, true)
		if err != nil {
			fmt.Fprintf(os.Stderr, "PatternsToPaths failed. args=%#q err=%v\n", flag.Args(), err)
			os.Exit(1)
		}
		// pathList = doclib.CleanCorpus(pathList) !@#$ pc-only
		if len(pathList) < 1 {
			fmt.Fprintf(os.Stderr, "No files matching %q.\n", pathPattern)
			os.Exit(1)
		}
	}

	// Run the tests.
	if err := runIndexSearchShow(pathList, term, persistDir, serialize, persist, reuse, nameOnly,
		maxResults, outPath); err != nil {
		panic(err)
	}
}

// runIndexSearchShow creates a pdfsearch.PdfIndex for the PDF files in `pathList`, searches for
//`term` in this index, and shows the results.
// It also creates a marked-up PDF containing the original PDF pages with the matched terms marked
//  and saves it to `outPath`.
// This is the main test function. The runIndexSearch() function is calls shows you how to create an
// index annd search it.
//
//  `persistDir`: The directory the pdfsearch.PdfIndex is saved in if `persist` is true.
//  `serialize`: Serialize in-memory pdfsearch.PdfIndex to a []byte.
//  `persist`: Persist pdfsearch.PdfIndex to disk.
//  `reuse`: Don't create a pdfsearch.PdfIndex. Reuse one that was previously persisted to disk.
//  `nameOnly`: Show matching file names only.
//  `maxResults`: Max number of search results to return.
func runIndexSearchShow(pathList []string, term, persistDir string, serialize, persist, reuse,
	nameOnly bool, maxResults int, outPath string) error {

	pdfIndex, results, dt, dtIndex, err := runIndexSearch(pathList, term, persistDir, serialize,
		persist, reuse, maxResults)
	if err != nil {
		return err
	}
	return showResults(pathList, pdfIndex, results, dt, dtIndex, serialize, nameOnly, maxResults, outPath)
}

// runIndexSearch creates a pdfsearch.PdfIndex for the PDF files in `pathList`, searches for `term`
// in this index and returns the pdfsearch.PdfIndex, the search results and the indexing and search
// durations.
// This is the main function. It shows you how to create an index annd search it.
//
//  `persistDir`: The directory the pdfsearch.PdfIndex is saved in if `persist` is true.
//  `serialize`: Serialize in-memory pdfsearch.PdfIndex to a []byte.
//  `persist`: Persist pdfsearch.PdfIndex to disk.
//  `reuse`: Don't create a pdfsearch.PdfIndex. Reuse one that was previously persisted to disk.
//  `maxResults`: Max number of search results to return.
func runIndexSearch(pathList []string, term, persistDir string, serialize, persist, reuse bool, maxResults int) (
	pdfIndex pdfsearch.PdfIndex, results doclib.PdfMatchSet, dt, dtIndex time.Duration, err error) {

	fmt.Fprintf(os.Stderr, "@@@@ %t %t %d %q\n", serialize, persist, len(pathList), pathList)

	t0 := time.Now()
	var data []byte
	if reuse {
		pdfIndex = pdfsearch.ReuseIndex(persistDir)
	} else if serialize {
		var rsList []io.ReadSeeker
		for i, inPath := range pathList {
			fmt.Printf("%d of %d : %q\n", i, len(pathList), inPath)
			rs, err := os.Open(inPath)
			if err != nil {
				panic(err)
			}
			defer rs.Close()
			rsList = append(rsList, rs)
		}
		fmt.Println("````````````````````````````````````")
		data, err = pdfsearch.IndexPdfMem(pathList, rsList, report)
		if err != nil {
			return pdfIndex, results, dt, dtIndex, err
		}
	} else {
		pdfIndex, err = pdfsearch.IndexPdfFiles(pathList, persist, persistDir, report, false)
		if err != nil {
			return pdfIndex, results, dt, dtIndex, err
		}
	}

	dtIndex = time.Since(t0)
	if serialize {
		results, err = pdfsearch.SearchMem(data, term, maxResults)
		if err != nil {
			return pdfIndex, results, dt, dtIndex, err
		}
	} else {
		results, err = pdfIndex.Search(term, maxResults)
		if err != nil {
			panic(err) // !@#$
		}
	}
	dt = time.Since(t0)

	// Deserialize the index for analysis. Not used for searching above.
	if data != nil {
		pdfIndex, _ = pdfsearch.FromBytes(data)
	}
	return pdfIndex, results, dt, dtIndex, nil
}

// showResults writes a report on `results`, some search results (for a term that we don't show
// here) on `pdfIndex` that was build from the PDF files in `pathList`.
// It also creates a marked-up PDF containing the original PDF pages with the matched terms marked
//  and saves it to `outPath`.
//
//  `dt` and `dtSearch` are the durations of the indexing + search, and search.
//  `serialize`: Serialize in-memory pdfsearch.PdfIndex to a []byte.
//  `persist`: Persist pdfsearch.PdfIndex to disk.
//  `reuse`: Don't create a pdfsearch.PdfIndex. Reuse one that was previously persisted to disk.
//  `nameOnly`: Show matching file names only.
//  `maxResults`: Max number of search results to return.
func showResults(pathList []string, pdfIndex pdfsearch.PdfIndex, results doclib.PdfMatchSet,
	dt, dtIndex time.Duration, serialize, nameOnly bool, maxResults int, outPath string) error {

	if nameOnly {
		files := results.Files()
		if len(files) > maxResults {
			files = files[:maxResults]
		}
		for i, fn := range files {
			fmt.Printf("%4d: %q\n", i, fn)
		}
	} else {

		fmt.Println("=================+++=====================")
		fmt.Printf("%s\n", results)
		fmt.Println("=================xxx=====================")
	}

	if err := pdfsearch.MarkupPdfResults(results, outPath); err != nil {
		return err
	}
	fmt.Println("=================+++=====================")

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

	storage := "SerialMem"
	if !serialize {
		storage = pdfIndex.StorageName()
	}

	dtSearch := dt - dtIndex

	fmt.Fprintf(os.Stderr, "[%s index] Duration=%.1f sec (%.3f index + %.3f search) (%.1f pages/min) "+
		"%d pages in %d files %+v\n"+
		"Index duration=%s\n"+
		"Marked up search results in %q\n",
		storage, dt.Seconds(), dtIndex.Seconds(), dtSearch.Seconds(), pagesSec*60.0,
		numPages, len(pathList), showList,
		pdfIndex.Duration(),
		outPath)
	return nil
}

// `report` is called  by IndexPdfMem to report progress.
func report(msg string) {
	fmt.Fprintf(os.Stderr, ">> %s\n", msg)
}
