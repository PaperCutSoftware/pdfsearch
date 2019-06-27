// Copyright 2019 PaperCut Software International Pty Ltd. All rights reserved.

/*
 *  This source implements the main function IndexPdfReaders().
 * IndexPdfFiles() is a convenience function that opens files and calls IndexPdfReaders().
 */
package doclib

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/blevesearch/bleve"
	"github.com/unidoc/unipdf/v3/common"
)

var ErrRange = errors.New("out of range")

// IndexPdfFilesUsingReaders creates a bleve+BlevePdf index for `pathList`.
// If `persistDir` is not empty, the index is written to this directory.
// If `forceCreate` is true and `persistDir` is not empty, a new directory is always created.
// then the bleve index will be appended to.
// `report` is a supplied function that is called to report progress.
// NOTE: This is for testing only. It doesn't make sense to access IndexPdfFilesOrReaders() with a
//      list of opened files as this can exhaust available file handles.
func IndexPdfFilesUsingReaders(pathList []string, persistDir string, forceCreate bool,
	report func(string)) (*BlevePdf, bleve.Index, int, time.Duration, time.Duration, error) {

	var rsList []io.ReadSeeker
	for _, inPath := range pathList {
		rs, err := os.Open(inPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Opened %d files\n", len(rsList))
			break
		}
		defer rs.Close()
		rsList = append(rsList, rs)
	}
	return IndexPdfFilesOrReaders(pathList, rsList, persistDir, forceCreate, report)
}

// IndexPdfFilesOrReaders returns a BlevePdf and a bleve.Index over
//   the PDF contents referenced by the io.ReaderSeeker's in `rsList` if `rsList` is not empty, or
//   the PDF filenames in `pathList` if `rsList` is not empty.
// If `persist` is false, the index is stored in memory.
// If `persist` is true, the index is stored on disk in `persistDir`.
// `report` is a supplied function that is called to report progress.
// NOTE: If you have access to your PDF files then use `pathList` and set `rsList` to nil as a long
//     list of file handles may exhaust system resources.
func IndexPdfFilesOrReaders(pathList []string, rsList []io.ReadSeeker, persistDir string,
	forceCreate bool, report func(string)) (*BlevePdf, bleve.Index,
	int, time.Duration, time.Duration, error) {

	useReaders := len(rsList) > 0

	common.Log.Info("Indexing %d PDF files. useReaders=%t", len(pathList), useReaders)
	var dtPdf, dtBleve, dtP, dtB time.Duration

	blevePdf, err := openBlevePdf(persistDir, forceCreate)
	if err != nil {
		return nil, nil, 0, dtPdf, dtBleve, fmt.Errorf("Could not create positions store %q. err=%v", persistDir, err)
	}
	defer blevePdf.Flush()
	blevePdf.Check()

	var index bleve.Index
	if len(persistDir) == 0 {
		index, err = createBleveMemIndex()
		if err != nil {
			return nil, nil, 0, dtPdf, dtBleve, fmt.Errorf("Could not create Bleve memoryindex. err=%v", err)
		}
	} else {
		indexPath := filepath.Join(persistDir, "bleve")
		common.Log.Info("indexPath=%q", indexPath)
		// Create a new Bleve index.
		index, err = createBleveDiskIndex(indexPath, forceCreate)
		if err != nil {
			return nil, nil, 0, dtPdf, dtBleve, fmt.Errorf("Could not create Bleve index in %q", indexPath)
		}
	}

	totalPages := 0
	// Add the pages of all the PDFs in `pathList` to `index`.
	for i, inPath := range pathList {
		readerOnly := ""
		if useReaders {
			readerOnly = " (readerOnly)"
		}
		if report != nil {
			report(fmt.Sprintf("%3d of %d: %q%s", i+1, len(pathList), inPath, readerOnly))
		}
		blevePdf.Check()
		var err error
		if useReaders {
			blevePdf.Check()
			rs := rsList[i]
			dtP, dtB, err = blevePdf.indexDocPagesLocReader(index, inPath, rs)
			dtPdf += dtP
			dtBleve += dtB
			fmt.Fprintf(os.Stderr, "***1 %d: (%.3f %.3f) (%.3f %.3f)\n", i,
				dtPdf.Seconds(), dtP.Seconds(), dtBleve.Seconds(), dtB.Seconds())
		} else {
			dtP, dtB, err = blevePdf.indexDocPagesLocFile(index, inPath)
			dtPdf += dtP
			dtBleve += dtB
		}
		blevePdf.Check()
		if err != nil {
			return nil, nil, 0, dtPdf, dtBleve, fmt.Errorf("Could not index file %q", inPath)
		}
		blevePdf.Check()
		docCount, err := index.DocCount()
		if err != nil {
			return nil, nil, 0, dtPdf, dtBleve, err
		}
		common.Log.Debug("Indexed %q. Total %d pages indexed.", inPath, docCount)
		totalPages += int(docCount)
	}

	return blevePdf, index, totalPages, dtPdf, dtBleve, err
}
