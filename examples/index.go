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
)

const usage = `Usage: go run index.go [OPTIONS] pcng-manual*.pdf
  Adds PDFs that match "pcng-manual*.pdf" to the index.
`

func main() {
	persistDir := filepath.Join(pdfsearch.DefaultPersistRoot, "my.computer")
	forceCreate := false
	useScorch := false
	doCPUProfile := false
	flag.StringVar(&persistDir, "s", persistDir, "The on-disk index is stored here.")
	flag.BoolVar(&forceCreate, "c", forceCreate, "Delete existing index and create a new one.")
	flag.BoolVar(&useScorch, "S", useScorch, "Use Bleve's Scorch storage. Faster but buggy.")
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
	// if len(pathList) > 8000 {
	// 	pathList = pathList[7700:]
	// }
	if len(pathList) < 1 {
		fmt.Fprintf(os.Stderr, "No files matching %q.\n", flag.Args())
		os.Exit(1)
	}
	pathList = cmd_utils.PartShuffle(pathList)
	// if len(pathList) > 1800 {
	// 	pathList = pathList[1800:]
	// }

	if len(pathList) > 4000 {
		pathList = pathList[4000:]
		pathList = append(pathList, pathList[:4000]...)
	}

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
	if err := runIndexShow(pathList, persistDir, forceCreate, useScorch); err != nil {
		fmt.Fprintf(os.Stderr, "runIndexShow failed. err=%v\n", err)
		os.Exit(1)
	}
}

// runIndexShow creates a pdfsearch.PdfIndex for the PDFs in `pathList`, searches for `term` in this
// index, and shows the results.
//  `persistDir`: The directory the pdfsearch.PdfIndex is saved.
func runIndexShow(pathList []string, persistDir string, forceCreate, useScorch bool) error {
	pdfIndex, dt, err := runIndex(pathList, persistDir, forceCreate, useScorch)
	if err != nil {
		return err
	}
	return showIndex(pathList, pdfIndex, dt)
}

// runIndex creates a pdfsearch.PdfIndex for the PDFs in `pathList` and returns the
// pdfsearch.PdfIndex, the search results and the indexing duration.
// The pdfsearch.PdfIndex is saved in directory `persistDir`.
// This is the main function. It shows you how to create or open an index.
func runIndex(pathList []string, persistDir string, forceCreate, useScorch bool) (
	pdfIndex pdfsearch.PdfIndex, dt time.Duration, err error) {
	fmt.Fprintf(os.Stderr, "Indexing %d files. Index stored in %q.\n", len(pathList), persistDir)

	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "Recovered r=%v\n", r)
			fmt.Fprintln(os.Stderr, "sleeping..")
			time.Sleep(time.Hour)
			panic(r)
		}
	}()

	t0 := time.Now()
	pdfIndex, err = pdfsearch.IndexPdfFiles(pathList, persistDir, forceCreate, useScorch, report)
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

// `report` is called  by IndexPdfMem to report progress.
func report(msg string) {
	fmt.Fprintf(os.Stderr, ">> %s\n", msg)
}
