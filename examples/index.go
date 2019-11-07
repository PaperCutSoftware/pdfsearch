// Copyright 2019 PaperCut Software International Pty Ltd. All rights reserved.

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime/pprof"
	"time"

	"github.com/papercutsoftware/pdfsearch"
	"github.com/papercutsoftware/pdfsearch/examples/cmd_utils"
	"github.com/papercutsoftware/pdfsearch/internal/utils"
)

const usage = `Usage: go run index.go [OPTIONS] pcng-manual*.pdf
  Adds PDFs that match "pcng-manual*.pdf" to the index.
`

func main() {
	persistDir := filepath.Join(pdfsearch.DefaultPersistRoot, "my.computer")
	doCPUProfile := false
	flag.StringVar(&persistDir, "s", persistDir, "The on-disk index is stored here.")
	flag.BoolVar(&doCPUProfile, "p", doCPUProfile, "Do Go CPU profiling.")
	cmd_utils.MakeUsage(usage)
	cmd_utils.MakeUsage(usage)
	flag.Parse()
	pdfsearch.InitLogging()

	if len(flag.Args()) < 1 {
		flag.Usage()
		os.Exit(1)
	}

	// Read the files to index into `pathList`.
	pathList, err := cmd_utils.PatternsToPaths(flag.Args())
	if err != nil {
		fmt.Fprintf(os.Stderr, "PatternsToPaths failed. args=%#q err=%v\n", flag.Args(), err)
		os.Exit(1)
	}
	pathList = cmd_utils.CleanCorpus(pathList)
	// pathList = pathList[7700:]
	if len(pathList) < 1 {
		fmt.Fprintf(os.Stderr, "No files matching %q.\n", flag.Args())
		os.Exit(1)
	}
	pathList = partShuffle(pathList)

	if doCPUProfile {
		profilePath := "cpu.index.prof"
		fmt.Printf("Profiling to %s\n", profilePath)
		f, err := os.Create(profilePath)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		err = pprof.StartCPUProfile(f)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		defer pprof.StopCPUProfile()
	}

	// Run the tests.
	if err := runIndexShow(pathList, persistDir); err != nil {
		fmt.Fprintf(os.Stderr, "runIndexShow failed. err=%v\n", err)
		os.Exit(1)
	}
}

// runIndexShow creates a pdfsearch.PdfIndex for the PDFs in `pathList`, searches for `term` in this
// index, and shows the results.
//  `persistDir`: The directory the pdfsearch.PdfIndex is saved.
func runIndexShow(pathList []string, persistDir string) error {
	pdfIndex, dt, err := runIndex(pathList, persistDir)
	if err != nil {
		return err
	}
	return showIndex(pathList, pdfIndex, dt)
}

// runIndex creates a pdfsearch.PdfIndex for the PDFs in `pathList` and returns the
// pdfsearch.PdfIndex, the search results and the indexing duration.
// The pdfsearch.PdfIndex is saved in directory `persistDir`.
// This is the main function. It shows you how to create or open an index.
func runIndex(pathList []string, persistDir string) (pdfIndex pdfsearch.PdfIndex, dt time.Duration,
	err error) {
	fmt.Fprintf(os.Stderr, "Indexing %d files. Index stored in %q.\n", len(pathList), persistDir)

	t0 := time.Now()
	pdfIndex, err = pdfsearch.IndexPdfFiles(pathList, persistDir, report)
	if err != nil {
		return pdfIndex, dt, err
	}
	dt = time.Since(t0)
	return pdfIndex, dt, nil
}

// showIndex writes a report on `pdfIndex` that was build from the PDFs in `pathList`.
// `dt` is the duration of the indexing.
func showIndex(pathList []string, pdfIndex pdfsearch.PdfIndex, dt time.Duration) error {
	numFiles := pdfIndex.NumFiles()
	numPages := pdfIndex.NumPages()
	pagesSec := 0.0
	if dt.Seconds() >= 0.01 {
		pagesSec = float64(numPages) / dt.Seconds()
	}
	fmt.Fprintf(os.Stderr, "%d pages from %d PDFs in %.1f secs (%.1f pages/sec)\n",
		numPages, numFiles, dt.Seconds(), pagesSec)
	fmt.Fprintf(os.Stderr, "%s\n", pdfIndex)
	return nil
}

// partShuffle shuffles part of `pathList` while maintaing some order by file size. The partial file
// size ordering is to keep large PDFs away from the end of `pathList` so one worker thread doesn't
// get a big slow file when the other work threads are done.
func partShuffle(pathList []string) []string {
	pathList, _ = cmd_utils.SortFileSize(pathList, -1, -1)

	// NOTE: Shuffle is intended to randomize the list with respect to file size, number of pages
	// etc which should help with load balancing the PDF processing go routines.
	// pathList = cmd_utils.Shuffle(pathList)
	// Keep the small files until last
	if len(pathList) > 100 {
		n := len(pathList) - 100
		p1 := pathList[:n]
		p2 := pathList[n:]
		p1 = cmd_utils.Shuffle(p1)
		pathList = append(p1, p2...)
	}

	var big []string
	var medium []string
	var small []string
	for _, path := range pathList {
		size, _ := utils.FileSizeMB(path)
		if size > 10.0 {
			big = append(big, path)
		} else if size < 1.0 {
			small = append(small, path)
		} else {
			medium = append(medium, path)
		}
	}
	pathList = append(big, medium...)
	pathList = append(pathList, small...)
	if len(pathList) > 100 {
		n := 100
		if n < 4*len(big) {
			n = 4 * len(big)
		}
		if n > len(pathList)/5 {
			n = len(pathList) / 5
		}
		// panic(fmt.Errorf("big=%d(%d) pathList=%d(%d) n=%d",
		// 	len(big), 4*len(big),
		// 	len(pathList), len(pathList)/4,
		// 	n))
		p1 := pathList[:n]
		p2 := pathList[n:]
		p1 = cmd_utils.Shuffle(p1)
		pathList = append(p1, p2...)
	}
	return pathList
}

// `report` is called  by IndexPdfMem to report progress.
func report(msg string) {
	fmt.Fprintf(os.Stderr, ">> %s\n", msg)
}
