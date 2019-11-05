// Copyright 2019 PaperCut Software International Pty Ltd. All rights reserved.

/*
 * This source file implements the main doclib function IndexPdfFiles().
 */
package doclib

import (
	"fmt"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/blevesearch/bleve"
	"github.com/unidoc/unipdf/v3/common"
)

// continueOnFailure tells us whether to continue indexing PDFs after errors have occurred.
const continueOnFailure = true

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
//
// !@#$ Parallelize this
func IndexPdfFiles(pathList []string, persistDir string, forceCreate bool, report func(string)) (
	*BlevePdf, bleve.Index, int, int, time.Duration, time.Duration, error) {
	common.Log.Debug("Indexing %d PDFs. forceCreate=%t", len(pathList), forceCreate)
	var dtPdf, dtBleve, dtB time.Duration

	// !@#$
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
		common.Log.Debug("indexPath=%q", indexPath)
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
	pathChan := make(chan string, 100)
	extractedChan := make(chan extractedDoc, 2*numWorkers)
	summaries := make([]string, numWorkers)
	for i := 0; i < numWorkers; i++ {
		go func(i int, summary *string) {
			extractPDFText(i, pathChan, extractedChan, summary)
			wg.Done()
		}(i, &summaries[i])
	}
	go func() {
		// Dispatch all the PDFs
		dispatchPDFs(pathList, pathChan)
		close(pathChan)
		// Wait for all the workers to finish processing the PDFs
		wg.Wait()
		close(extractedChan)
	}()

	numFiles := 0
	docCount00, err := index.DocCount()
	if err != nil {
		return nil, nil, 0, 0, dtPdf, dtBleve, err
	}

	// Add the pages of all the PDFs in the text extraction results channel `extractedChan` to
	// `blevePdf` and `index`.
	for e := range extractedChan {
		fd, docContents, dtPdf, err := e.fd, e.docContents, e.dt, e.err
		if err != nil {
			common.Log.Error("IndexPdfFiles: Couldn't extract pages from %q err=%v", fd.InPath, err)
			continue //!@#$ should be configurable
			// return nil, nil, 0, 0, dtPdf, dtBleve, err
		}

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
		totalPages := int(docCount)
		numFiles++
		totalSec := dtTotal.Seconds()
		rate := 0.0
		if totalSec > 0.0 {
			rate = float64(totalPages) / totalSec
		}
		if report != nil {
			report(fmt.Sprintf("%3d of %d: %3d pages %3.1f sec (total: %3d pages %3.1f sec %3.1f pages/sec) %q",
				numFiles, len(pathList),
				docPages, dt.Seconds(),
				totalPages, totalSec, rate,
				fd.InPath))
		}
	}

	// Write out the worker loads to see how evenly they are spread.
	for i, summary := range summaries {
		common.Log.Info("extractPDFText %d: %s", i, summary)
	}

	docCount, err := index.DocCount()
	if err != nil {
		return nil, nil, 0, 0, dtPdf, dtBleve, err
	}
	totalPages := int(docCount - docCount00)
	return blevePdf, index, numFiles, totalPages, dtPdf, dtBleve, err
}

// extractedDoc is the result of PDF text extraction.
type extractedDoc struct {
	fd          fileDesc
	docContents []pageContents
	dt          time.Duration
	err         error
}

// dispatchPDFs dispatches the PDFs in `pathList` to `pathChan`.
func dispatchPDFs(pathList []string, pathChan chan<- string) {
	for _, inPath := range pathList {
		pathChan <- inPath
	}
}

// extractPDFText takes PDF paths from `pathChan`, extracts text from them and writes the text
// extraction results to `extractedChan`. When extractPDFText is done it returns a summary in
// `summary`.
func extractPDFText(workerNum int, pathChan <-chan string, extractedChan chan<- extractedDoc,
	summary *string) {
	numDocs := 0
	numPages := 0
	var processTime time.Duration
	for inPath := range pathChan {
		t0 := time.Now()
		fd, docContents, err := extractDocPagePositions(inPath)
		dt := time.Since(t0)
		e := extractedDoc{
			fd:          fd,
			docContents: docContents,
			dt:          dt,
			err:         err,
		}
		extractedChan <- e
		numDocs++
		numPages += len(docContents)
		processTime += dt
	}

	processSec := processTime.Seconds()
	docsSec := 0.0
	pagesSec := 0.0
	if processSec > 0.0 {
		docsSec = float64(numDocs) / processSec
		pagesSec = float64(numPages) / processSec
	}

	*summary = fmt.Sprintf("processed %d PDFs %d pages in %.1f sec (%.1f PDFs/sec %.1f pages/sec)",
		numDocs, numPages, processSec, docsSec, pagesSec)
}
