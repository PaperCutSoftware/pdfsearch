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
const usage = `Usage: go run index_search_example.go [OPTIONS] -f "pcng-manual*.pdf"  PaperCut NG
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
	var useReaderSeeker bool
	maxSearchResults := 10
	outPath := "search.results.pdf"
	groupSize := -1
	var compare, compareDisk bool

	flag.StringVar(&pathPattern, "f", pathPattern, "PDF file(s) to index.")
	flag.StringVar(&outPath, "o", outPath, "Name of PDF file that will show marked up results.")
	flag.StringVar(&persistDir, "s", pdfsearch.DefaultPersistDir, "The on-disk index is stored here.")
	flag.BoolVar(&serialize, "m", serialize, "Serialize in-memory index to byte array.")
	flag.BoolVar(&persist, "p", persist, "Store index on disk (slower but allows more PDF files).")
	flag.BoolVar(&reuse, "r", reuse, "Reused stored index on disk for the last -p run.")
	flag.BoolVar(&nameOnly, "l", nameOnly, "Show matching file names only.")
	flag.IntVar(&maxSearchResults, "n", maxSearchResults, "Max number of search results to return.")
	flag.IntVar(&groupSize, "g", groupSize, "Max number of files per index. (For stress testing).")
	flag.BoolVar(&useReaderSeeker, "j", useReaderSeeker, "Exercise the io.ReaderSeeker API.")
	flag.BoolVar(&compare, "c", compare, "Compare serialized and non-memoryd in-memory results.")
	flag.BoolVar(&compareDisk, "cd", compareDisk, "Compare in-memory and on-disk results.")

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
	if compareDisk {
		compare = true
	}
	if compare && groupSize <= 0 {
		groupSize = 1
	}
	if compare {
		// We exercise more code in the comparison if the search returns more matches, so we
		// augment the seach term with common non-stop words
		term += " people color target man math company manage science find language" +
			"paper cost art business sport count"
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

	// Run the tests in one of the following ways:
	// - runIndexSearchShow: Default. Creates an index over all PDF files and searches it.
	// - runIndexSearchShowGroups: -g <n>. Like runIndexSearchShow but over groups of <n> files.
	// - runAllModesGroups: -c or -cd. Like runIndexSearchShowGroups but runs in serialized and
	//                     unserialized in-memory(and on-disk for -cd) modes and compares results.
	if groupSize > 0 {
		if compare {
			runAllModesGroups(pathList, term, persistDir, nameOnly,
				useReaderSeeker, maxResults, outPath, groupSize, compareDisk)
		} else {
			runIndexSearchShowGroups(pathList, term, persistDir, serialize, persist, nameOnly, useReaderSeeker,
				maxResults, outPath, groupSize)
		}
	} else {
		if err := runIndexSearchShow(pathList, term, persistDir, serialize, persist, reuse, nameOnly,
			useReaderSeeker, maxResults, outPath); err != nil {
			panic(err)
		}
	}
}

// runIndexSearchShow creates a pdfsearch.PdfIndex for the PDF files in `pathList`, searches for `term` in
// this index, and shows the results.
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
//  `useReaderSeeker`: Exercise the io.ReaderSeeker API.
//  `maxResults`: Max number of search results to return.
func runIndexSearchShow(pathList []string, term, persistDir string, serialize, persist, reuse, nameOnly,
	useReaderSeeker bool, maxResults int, outPath string) error {

	pdfIndex, results, dt, dtIndex, err := runIndexSearch(pathList, term, persistDir, serialize,
		persist, reuse, useReaderSeeker, maxResults)
	if err != nil {
		return err
	}
	return showResults(pathList, pdfIndex, results, dt, dtIndex, serialize, nameOnly, maxResults, outPath)
}

// runIndexSearchShowGroups is like runIndexSearchShow() except that it splits `pathList` into
// groups of `groupSize` and creates an index for and searches that index for each group.
// It also creates a marked-up PDF containing the original PDF pages with the matched terms marked
//  and saves it to `outPath`.
//
//  `persistDir`: The directory the pdfsearch.PdfIndex is saved in if `persist` is true.
//  `serialize`: Serialize in-memory pdfsearch.PdfIndex to a []byte.
//  `persist`: Persist pdfsearch.PdfIndex to disk.
//  `reuse`: Don't create a pdfsearch.PdfIndex. Reuse one that was previously persisted to disk.
//  `nameOnly`: Show matching file names only.
//  `useReaderSeeker`: Exercise the io.ReaderSeeker API.
//  `maxResults`: Max number of search results to return.
func runIndexSearchShowGroups(pathList []string, term, persistDir string, serialize, persist, nameOnly,
	useReaderSeeker bool, maxResults int, outPath string, groupSize int) {

	for i0 := 0; i0 <= len(pathList); i0 += groupSize {
		i1 := i0 + groupSize
		if i1 > len(pathList) {
			i1 = len(pathList)
		}
		paths := pathList[i0:i1]
		fmt.Fprintln(os.Stderr, "~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~")
		fmt.Fprintf(os.Stderr, "%d-%d of %d: %q\n", i0, i1, len(pathList), paths)
		err := runIndexSearchShow(paths, term, persistDir, serialize, persist, false, nameOnly, useReaderSeeker,
			maxResults, outPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%d-%d of %d: err=%v\n", i0, i1, len(pathList), err)
		}
	}
}

// runAllModesGroups is like runIndexSearchShowGroups() except that it creates the pdfsearch.PdfIndex as
// a) in-memory unserialized and b) serialized and compares the indexes and search results for a)
// and b). It does this for each group.
// If `testDisk` is true then an on-disk index is also created and the index and search results are
// compared to the in-memory index and results.
//
//  `persistDir`: The directory the pdfsearch.PdfIndex is saved in if `persist` is true.
//  `nameOnly`: Show matching file names only.
//  `useReaderSeeker`: Exercise the io.ReaderSeeker API.
//  `maxResults`: Max number of search results to return.
//  `outPath`: File name to save marked-up PDF to.
//  `groupSize`: pathList is split into groups of this size and the tests are done on each group.
func runAllModesGroups(pathList []string, term, persistDir string, nameOnly,
	useReaderSeeker bool, maxResults int, outPath string, groupSize int, testDisk bool) {

	fmt.Fprintf(os.Stderr, "groupSize=%d\n", groupSize)

	for i0 := 0; i0 <= len(pathList); i0 += groupSize {
		i1 := i0 + groupSize
		if i1 > len(pathList) {
			i1 = len(pathList)
		}
		paths := pathList[i0:i1]
		fmt.Fprintln(os.Stderr, "~~~~~~~~~~~~~~~~~~~~~^^~~~~~~~~~~~~~~~~~~~~~~~~~~~")
		fmt.Fprintf(os.Stderr, "### %d-%d of %d: %q\n", i0, i1, len(pathList), paths)
		err := runAllModes(paths, term, persistDir, nameOnly, useReaderSeeker, maxResults, outPath, testDisk)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%d-%d of %d: err=%v\n", i0, i1, len(pathList), err)
		}
	}
}

// runAllModes is like runIndexSearchShow() except that it creates the pdfsearch.PdfIndex as
// a) in-memory unserialized and b) serialized and compares the indexes and search results for a)
// and b).
// If `testDisk` is true then an on-disk index is also created and the index and search results are
// compared to the in-memory index and results.
//  `persistDir`: The directory the pdfsearch.PdfIndex is saved in if `persist` is true.
//  `nameOnly`: Show matching file names only.
//  `useReaderSeeker`: Exercise the io.ReaderSeeker API.
//  `maxResults`: Max number of search results to return.
//  `outPath`: File name to save marked-up PDF to.
func runAllModes(pathList []string, term, persistDir string, nameOnly,
	useReaderSeeker bool, maxResults int, outPath string, testDisk bool) error {

	var pdfIndexDisk pdfsearch.PdfIndex
	var resultsDisk doclib.PdfMatchSet
	var dtDisk, dtIndexDisk time.Duration

	pdfIndex0, results0, dt0, dtIndex0, err := runIndexSearch(pathList, term, persistDir,
		false, false, false, useReaderSeeker, maxResults)
	if err != nil {
		panic(err)
		return err
	}
	pdfIndexMem, resultsMem, dtMem, dtIndexMem, err := runIndexSearch(pathList, term, persistDir,
		true, false, false, useReaderSeeker, maxResults)
	if err != nil {
		panic(err)
		return err
	}
	if testDisk {
		pdfIndexDisk, resultsDisk, dtDisk, dtIndexDisk, err = runIndexSearch(pathList, term, persistDir,
			false, true, false, useReaderSeeker, maxResults)
		if err != nil {
			panic(err)
			return err
		}
	}

	err = showResults(pathList, pdfIndex0, results0, dt0, dtIndex0, false, nameOnly, maxResults, outPath)
	if err != nil {
		return err
	}
	err = showResults(pathList, pdfIndexMem, resultsMem, dtMem, dtIndexMem, true, nameOnly, maxResults, outPath)
	if err != nil {
		return err
	}
	if testDisk {
		err = showResults(pathList, pdfIndexDisk, resultsDisk, dtDisk, dtIndexDisk, false, nameOnly, maxResults, outPath)
		if err != nil {
			return err
		}
	}

	if !pdfIndex0.Equals(pdfIndexMem) {
		panic("pdfIndex: 0 - mem")
	}
	if testDisk {
		if !pdfIndex0.Equals(pdfIndexDisk) {
			panic("pdfIndex: 0 - disk")
		}
	}

	if !results0.Equals(resultsMem) {
		panic("results: 0 - mem")
	}
	if testDisk {
		if !results0.Equals(resultsDisk) {
			panic("results: 0 - disk")
		}
	}
	return nil
}

// runIndexSearch creates a pdfsearch.PdfIndex for the PDF files in `pathList`, searches for `term` in
// this index and returns the pdfsearch.PdfIndex, the search results and the indexing and search durations.
// This is the main function. It shows you how to create an index annd search it.
//
//  `persistDir`: The directory the pdfsearch.PdfIndex is saved in if `persist` is true.
//  `serialize`: Serialize in-memory pdfsearch.PdfIndex to a []byte.
//  `persist`: Persist pdfsearch.PdfIndex to disk.
//  `reuse`: Don't create a pdfsearch.PdfIndex. Reuse one that was previously persisted to disk.
//  `useReaderSeeker`: Exercise the io.ReaderSeeker API.
//  `maxResults`: Max number of search results to return.
func runIndexSearch(pathList []string, term, persistDir string, serialize, persist, reuse,
	useReaderSeeker bool, maxResults int) (
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
		pdfIndex, err = pdfsearch.IndexPdfFiles(pathList, persist, persistDir, report, useReaderSeeker)
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
//  `useReaderSeeker`: Exercise the io.ReaderSeeker API.
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
