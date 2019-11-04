// Copyright 2019 PaperCut Software International Pty Ltd. All rights reserved.

/*
 * This source file implements the main doclib function IndexPdfFiles().
 */
package doclib

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/blevesearch/bleve"
	"github.com/unidoc/unipdf/v3/common"
)

// continueOnFailure tells us whether to continue indexing PDF files after errors have occurred.
const continueOnFailure = true

// IndexPdfFiles returns a BlevePdf and a bleve.Index over the PDFs in `pathList`.
// The index is stored on disk in `persistDir`.
// `report` is a supplied function that is called to report progress.
// Returns: (blevePdf, index, numFiles, totalPages, dtPdf, dtBleve, err) where
//   blevePdf: mapping of a bleve index to PDF pages and text coordinates
//   index: a bleve index
//   numFiles: number of PDF files succesfully indexed
//   totalPages: number of PDF pages succesfully indexed
//   dtPdf: number of seconds spent building blevePdf
//   dtBleve: number of seconds spent building index
//   err: error, if one occurred
//
// !@#$ Parallelize this
func IndexPdfFiles(pathList []string, persistDir string, forceCreate bool, report func(string)) (
	*BlevePdf, bleve.Index, int, int, time.Duration, time.Duration, error) {
	common.Log.Debug("Indexing %d PDF files. forceCreate=%t", len(pathList), forceCreate)
	var dtPdf, dtBleve, dtP, dtB time.Duration

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

	numFiles := 0
	docCount00, err := index.DocCount()
	if err != nil {
		return nil, nil, 0, 0, dtPdf, dtBleve, err
	}
	t00 := time.Now()
	// Add the pages of all the PDFs in `pathList` to `index`.
	for i, inPath := range pathList {
		blevePdf.check()
		var err error
		t0 := time.Now()
		docCount0, err := index.DocCount()
		if err != nil {
			return nil, nil, 0, 0, dtPdf, dtBleve, err
		}
		dtP, dtB, err = blevePdf.indexDocPagesLoc(index, inPath)
		dtPdf += dtP
		dtBleve += dtB

		dt := time.Since(t0)
		dtTotal := time.Since(t00)
		blevePdf.check()
		if err != nil {
			if continueOnFailure {
				continue
			}
			return nil, nil, 0, 0, dtPdf, dtBleve, fmt.Errorf("Could not index file %q", inPath)
		}
		blevePdf.check()
		docCount, err := index.DocCount()
		if err != nil {
			return nil, nil, 0, 0, dtPdf, dtBleve, err
		}
		common.Log.Debug("Indexed %q. Total %d pages indexed.", inPath, docCount)
		docPages := int(docCount - docCount0)
		totalPages := int(docCount)
		numFiles++
		if report != nil {
			report(fmt.Sprintf("%3d (%3d) of %d: %3d pages %3.1f sec (total: %3d pages %3.1fsec) %q",
				i+1, numFiles, len(pathList),
				docPages, dt.Seconds(),
				totalPages, dtTotal.Seconds(),
				inPath))
		}
	}

	docCount, err := index.DocCount()
	if err != nil {
		return nil, nil, 0, 0, dtPdf, dtBleve, err
	}
	totalPages := int(docCount - docCount00)
	return blevePdf, index, numFiles, totalPages, dtPdf, dtBleve, err
}
