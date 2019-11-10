// Copyright 2019 PaperCut Software International Pty Ltd. All rights reserved.

/*
 * This source file implements the main doclib function IndexPdfFiles().
 */
package doclib

import (
	"fmt"
	"math"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/blevesearch/bleve"
	"github.com/unidoc/unipdf/v3/common"
)

// continueOnFailure tells us whether to continue indexing PDFs after errors have occurred.
const continueOnFailure = true

// ReopenDelta is a hack to to hold off the "too many open files" error
const ReopenDelta = 1 * 1000

// IndexPdfFiles returns a BlevePdf and a bleve.Index over the PDFs in `pathList`.
// The index is stored on disk in `persistDir`.
// `report` is a supplied function that is called to report progress.
// Returns: (blevePdf, index, numFiles, totalPages, dtPdf, dtBleve, err) where
//   blevePdf: mapping of a bleve index to PDF pages and text coordinates
//   index: a bleve index
//   numFiles: number of PDFs succesfully indexed
//   totalPages: number of PDF pages succesfully indexed
//   dtPdf: number of seconds spent building blevePdf
//   dtBleve: number of seconds spent building index
//   err: error, if one occurred
func IndexPdfFiles(pathList []string, persistDir string, forceCreate bool, report func(string)) (
	*BlevePdf, bleve.Index, int, int, time.Duration, time.Duration, error) {
	common.Log.Debug("Indexing %d PDFs. forceCreate=%t", len(pathList), forceCreate)
	if forceCreate {
		panic("forceCreate")
	}
	var dtPdf, dtBleve, dtB time.Duration

	blevePdf, err := openBlevePdf(persistDir, forceCreate)
	if err != nil {
		return nil, nil, 0, 0, dtPdf, dtBleve, fmt.Errorf("Could not create positions store %q. "+
			"err=%v", persistDir, err)
	}
	defer blevePdf.flush()
	defer blevePdf.check()

	var index bleve.Index
	if len(persistDir) == 0 {
		index, err = createBleveMemIndex()
		if err != nil {
			return nil, nil, 0, 0, dtPdf, dtBleve, fmt.Errorf("Could not create Bleve memoryindex. "+
				"err=%v", err)
		}
	} else {
		indexPath := filepath.Join(persistDir, "bleve")
		common.Log.Info("indexPath=%q", indexPath)
		common.Log.Info("indexPath=%q", indexPath)
		indexPath, _ = filepath.Abs(indexPath)
		common.Log.Info("indexPath=%q", indexPath)
		// Create a new Bleve index.
		index, err = createBleveDiskIndex(indexPath, forceCreate)
		if err != nil {
			return nil, nil, 0, 0, dtPdf, dtBleve, fmt.Errorf("Could not create Bleve index in %q",
				indexPath)
		}
	}

	t00 := time.Now()

	numWorkers := (runtime.NumCPU() * 3) / 4
	if numWorkers > len(pathList) {
		numWorkers = len(pathList)
	}
	if numWorkers < 1 {
		numWorkers = 1
	}
	common.Log.Info("numCPU=%d numWorkers=%d", runtime.NumCPU(), numWorkers)
	wg := &sync.WaitGroup{}
	wg.Add(numWorkers)
	pathChan := make(chan orderedPath, 100)
	extractedChan := make(chan extractedDoc, 2*numWorkers)
	profiles := make([]extractorProfile, numWorkers)
	for i := 0; i < numWorkers; i++ {
		go func(i int, profile *extractorProfile) {
			extractPDFText(i, pathChan, extractedChan, profile)
			wg.Done()
		}(i, &profiles[i])
	}
	go func() {
		// Dispatch all the PDFs
		dispatchPDFs(pathList, pathChan)
		close(pathChan)
		// Wait for all the workers to finish processing the PDFs
		wg.Wait()
		close(extractedChan)
	}()

	fileNum := 0
	totalFiles := 0
	totalPages := 0
	openForPages := 0
	maxOpenPages := 0
	docCount00, err := index.DocCount()
	if err != nil {
		return nil, nil, 0, 0, dtPdf, dtBleve, err
	}

	// Add the pages of all the PDFs in the text extraction results channel `extractedChan` to
	// `blevePdf` and `index`.
	for e := range extractedChan {
		fileNum++
		fd, docContents, dtPdf, err := e.fd, e.docContents, e.dt, e.err
		if err != nil {
			common.Log.Error("IndexPdfFiles: Couldn't extract pages from %q err=%v", fd.InPath, err)
			panic(err)
			continue //!@#$ should be configurable
			// return nil, nil, 0, 0, dtPdf, dtBleve, err
		}
		common.Log.Info("fileNum=%d\n\tfd=%v\n\tdocContents=%d", fileNum, fd, len(docContents))
		if len(docContents) == 0 {
			continue
		}

		// if openForPages+len(docContents) > ReopenDelta {
		// 	index, err = reopenBleve(index)
		// 	if err != nil {
		// 		panic(err)
		// 		return nil, nil, 0, 0, dtPdf, dtBleve, err
		// 	}
		// 	common.Log.Info("++++ Reopened at %d to avoid %d open pages",
		// 		openForPages, openForPages+len(docContents))
		// 	openForPages = 0
		// }

		blevePdf.check()
		t0 := time.Now()
		docCount0, err := index.DocCount()
		if err != nil {
			return nil, nil, 0, 0, dtPdf, dtBleve, err
		}

		_, dtB, err = blevePdf.indexDocPagesLoc(index, fd, docContents)

		dtBleve += dtB

		dt := time.Since(t0)
		dtTotal := time.Since(t00)
		blevePdf.check()
		if err != nil {
			if continueOnFailure {
				continue
			}
			return nil, nil, 0, 0, dtPdf, dtBleve, fmt.Errorf("could not index file %q", fd.InPath)
		}
		blevePdf.check()
		docCount, err := index.DocCount()
		if err != nil {
			return nil, nil, 0, 0, dtPdf, dtBleve, err
		}
		common.Log.Debug("Indexed %q. Total %d pages indexed.", fd.InPath, docCount)
		docPages := int(docCount - docCount0)
		if BleveIsLive && docPages <= 0 {
			for i, p := range docContents {
				common.Log.Info("page %d %d---------------------------\n%s", i, len(p.text),
					truncate(p.text, 100))
			}
			err := fmt.Errorf("didn't add pages to Bleve: docCount0=%d docCount=%d docPages=%d docContents=%d",
				docCount0, docCount, docPages, len(docContents))
			panic(err)
		}
		totalPages += docPages
		openForPages += docPages
		if openForPages > maxOpenPages {
			maxOpenPages = openForPages
			common.Log.Info("maxOpenPages=%d", maxOpenPages)
		}
		totalFiles++
		totalSec := dtTotal.Seconds()
		rate := 0.0
		if totalSec > 0.0 {
			rate = float64(totalPages) / totalSec
		}
		if report != nil {
			report(fmt.Sprintf("%3d (%3d) of %d: %5.1f MB %3d pages %3.1f sec (total: %3d pages %4.1f sec %5.1f pages/sec) %q",
				fileNum, e.i+1, len(pathList), fd.SizeMB,
				docPages, dt.Seconds(),
				totalPages, totalSec, rate,
				fd.InPath))
		}
	}

	// Write out the worker loads to see how evenly they are spread.
	for i, profile := range sortedProfiles(profiles) {
		common.Log.Info("extractPDFText %d: %s", i, profile)
	}
	dtPdf = extractionDuration(profiles)

	docCount, err := index.DocCount()
	if err != nil {
		return nil, nil, 0, 0, dtPdf, dtBleve, err
	}
	totalPages2 := int(docCount - docCount00)
	if totalPages2 != totalPages {
		common.Log.Error("^@^ totalPages=%d totalPages2=%d docCount=%d docCount00=%d",
			totalPages, totalPages2, docCount, docCount00)
	}
	return blevePdf, index, totalFiles, totalPages, dtPdf, dtBleve, err
}

type orderedPath struct {
	i      int
	inPath string
}

// extractedDoc is the result of PDF text extraction.
type extractedDoc struct {
	i           int
	fd          fileDesc
	docContents []pageContents
	dt          time.Duration
	err         error
}

type extractorProfile struct {
	numDocs   int
	numPages  int
	dtProcess time.Duration
	dtIdle    time.Duration
}

// dispatchPDFs dispatches the PDFs in `pathList` to `pathChan`.
func dispatchPDFs(pathList []string, pathChan chan<- orderedPath) {
	for i, inPath := range pathList {
		pathChan <- orderedPath{i: i, inPath: inPath}
	}
}

// extractPDFText takes PDF paths from `pathChan`, extracts text from them and writes the text
// extraction results to `extractedChan`. When extractPDFText is done it returns a summary in
// `summary`.
func extractPDFText(workerNum int, pathChan <-chan orderedPath, extractedChan chan<- extractedDoc,
	profile *extractorProfile) {
	numDocs := 0
	numPages := 0
	var processTime time.Duration
	var idleTime time.Duration

	tIdle := time.Now()
	for op := range pathChan {
		// dtIdle := time.Since(tIdle)
		t0 := time.Now()
		fd, docContents, err := extractDocPagePositions(op.inPath)
		if fd.InPath == "" {
			panic(fmt.Errorf("No path 1). op=%+v", op))
		}
		t1 := time.Now()
		// dt := time.Since(t0)
		dtIdle := t0.Sub(tIdle)
		dt := t1.Sub(t0)
		e := extractedDoc{
			i:           op.i,
			fd:          fd,
			docContents: docContents,
			dt:          dt,
			err:         err,
		}
		if e.fd.InPath == "" {
			panic(fmt.Errorf("No path 2). op=%+v e=%+v", op, e))
		}
		extractedChan <- e
		numDocs++
		numPages += len(docContents)
		processTime += dt
		idleTime += dtIdle
		tIdle = time.Now()
	}

	*profile = extractorProfile{
		numDocs:   numDocs,
		numPages:  numPages,
		dtProcess: processTime,
		dtIdle:    idleTime,
	}
}

func (p extractorProfile) String() string {
	docsSec := 0.0
	pagesSec := 0.0
	processSec := p.dtProcess.Seconds()
	if processSec > 0.0 {
		docsSec = float64(p.numDocs) / processSec
		pagesSec = float64(p.numPages) / processSec
	}
	return fmt.Sprintf("processed %3d PDFs %4d pages in %5.1f sec [%5.1f sec idle] (%3.1f PDFs/sec %4.1f pages/sec)",
		p.numDocs, p.numPages, processSec, p.dtIdle.Seconds(), docsSec, pagesSec)
}

func sortedProfiles(profiles []extractorProfile) []extractorProfile {
	sort.Slice(profiles, func(i, j int) bool {
		pi, pj := profiles[i], profiles[j]
		si, sj := pi.dtProcess.Seconds(), pj.dtProcess.Seconds()
		if math.Abs(si-sj) >= 0.1 {
			return si > sj
		}
		if pi.numPages != pj.numPages {
			return pi.numPages < pj.numPages
		}
		if pi.numDocs != pj.numDocs {
			return pi.numDocs < pj.numDocs
		}
		return i < j
	})
	return profiles
}

func extractionDuration(profiles []extractorProfile) time.Duration {
	var dtProcess time.Duration
	for _, p := range profiles {
		if p.dtProcess > dtProcess {
			dtProcess = p.dtProcess
		}
	}
	return dtProcess
}
