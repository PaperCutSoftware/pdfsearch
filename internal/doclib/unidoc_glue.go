// Copyright 2019 PaperCut Software International Pty Ltd. All rights reserved.

package doclib

// Based on Unidoc Console Logger.
import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/unidoc/unipdf/v3/common"
	"github.com/unidoc/unipdf/v3/common/license"
	"github.com/unidoc/unipdf/v3/extractor"
	"github.com/unidoc/unipdf/v3/model"
)

var (
	Debug bool
	Trace bool
	// ExposeErrors can be set to true to not recover from errors in library functions.
	ExposeErrors bool
)

const (
	// Make sure to enter a valid license key.
	// Otherwise text is truncated and a watermark added to the text.
	// License keys are available via: https://unidoc.io
	uniDocLicenseKey = `
-----BEGIN UNIDOC LICENSE KEY-----
....
-----END UNIDOC LICENSE KEY-----
`
	companyName = "(Your company)"
	creatorName = "PDF Search"
)

// init sets up UniDoc licensing and logging.
func init() {
	err := license.SetLicenseKey(uniDocLicenseKey, companyName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading UniDoc license: %v\n", err)
	}
	model.SetPdfCreator(creatorName)

	flag.BoolVar(&Debug, "d", false, "Print debugging information.")
	flag.BoolVar(&Trace, "e", false, "Print detailed debugging information.")
	flag.BoolVar(&ExposeErrors, "x", ExposeErrors, "Don't recover from library panics.")

	if Trace {
		Debug = true
	}
	if Trace {
		common.SetLogger(common.NewConsoleLogger(common.LogLevelTrace))
	} else if Debug {
		common.SetLogger(common.NewConsoleLogger(common.LogLevelDebug))
	} else {
		common.SetLogger(common.NewConsoleLogger(common.LogLevelInfo))
	}
}

// PdfOpenFile opens PDF file `inPath` and attempts to handle null encryption schemes.
func PdfOpenFile(inPath string) (*model.PdfReader, error) {
	f, err := os.Open(inPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return PdfOpenReader(f, false)
}

// PdfOpenFile opens PDF file `inPath` lazily and attempts to handle null encryption schemes.
// Caller must close the returned file handle if there are no errors.
func PdfOpenFileLazy(inPath string) (*os.File, *model.PdfReader, error) {
	f, err := os.Open(inPath)
	if err != nil {
		return nil, nil, err
	}
	pdfReader, err := PdfOpenReader(f, true)
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	return f, pdfReader, nil
}

// PdfOpenReader opens the PDF file accessed by `rs` and attempts to handle null encryption schemes.
// If `lazy` is true, a lazy PDF reader is opened.
func PdfOpenReader(rs io.ReadSeeker, lazy bool) (*model.PdfReader, error) {
	var pdfReader *model.PdfReader
	var err error
	if lazy {
		pdfReader, err = model.NewPdfReaderLazy(rs)
	} else {
		pdfReader, err = model.NewPdfReader(rs)
	}
	if err != nil {
		return nil, err
	}
	isEncrypted, err := pdfReader.IsEncrypted()
	if err != nil {
		return nil, err
	}
	if isEncrypted {
		_, err = pdfReader.Decrypt([]byte(""))
		if err != nil {
			return nil, err
		}
	}
	return pdfReader, nil
}

// PageSizePt returns the width and height of `page` in points.
func PageSizePt(page *model.PdfPage) (width, height float64, err error) {
	b, err := page.GetMediaBox()
	if err != nil {
		return 0, 0, err
	}
	return b.Urx - b.Llx, b.Ury - b.Lly, nil
}

// ExtractPageTextMarks returns the extracted text and corresponding TextMarks on page `page`.
func ExtractPageTextMarks(page *model.PdfPage) (string, *extractor.TextMarkArray, error) {
	ex, err := extractor.New(page)
	if err != nil {
		return "", nil, err
	}
	pageText, _, _, err := ex.ExtractPageText()
	if err != nil {
		return "", nil, err
	}
	return pageText.Text(), pageText.Marks(), nil
}

// PDFPageProcessor is used for processing a PDF file one page at a time.
// It is an opaque struct.
type PDFPageProcessor struct {
	inPath    string
	pdfFile   *os.File
	pdfReader *model.PdfReader
}

// CreatePDFPageProcessorFile creates a PDFPageProcessor for reading the PDF file `inPath`.
func CreatePDFPageProcessorFile(inPath string) (*PDFPageProcessor, error) {
	f, err := os.Open(inPath)
	if err != nil {
		common.Log.Error("CreatePDFPageProcessorFile: Could not open inPath=%q. err=%v", inPath, err)
		return nil, err
	}
	processor, err := CreatePDFPageProcessorReader(inPath, f)
	if err != nil {
		f.Close()
		return nil, err
	}
	processor.pdfFile = f
	return processor, err
}

// CreatePDFPageProcessorReader creates a  PDFPageProcessor for reading the PDF file referenced by
// `rs`.
// `inPath` is provided for logging only but it is expected to be the path referenced by `rs`.
func CreatePDFPageProcessorReader(inPath string, rs io.ReadSeeker) (*PDFPageProcessor, error) {
	processor := PDFPageProcessor{inPath: inPath}
	var err error
	processor.pdfReader, err = PdfOpenReader(rs, true)
	if err != nil {
		common.Log.Debug("CreatePDFPageProcessor: PdfOpenReader failed. inPath=%q. err=%v",
			inPath, err)
		return nil, err
	}
	return &processor, nil
}

// Close closes file handles opened by CreatePDFPageProcessorFile.
func (p *PDFPageProcessor) Close() error {
	if p.pdfFile == nil {
		return nil
	}
	err := p.pdfFile.Close()
	p.pdfFile = nil
	return err
}

// NumPages return the number of pages in the PDF file referenced by `p`.
func (p PDFPageProcessor) NumPages() (uint32, error) {
	numPages, err := p.pdfReader.GetNumPages()
	return uint32(numPages), err
}

// Process runs `processPage` on every page in PDF file `p.inPath`.
// It can recover from errors in the libraries it calls if `ExposeErrors` is false.
func (p *PDFPageProcessor) Process(processPage func(pageNum uint32, page *model.PdfPage) error) (
	err error) {
	if !ExposeErrors {
		defer func() {
			if r := recover(); r != nil {
				common.Log.Error("Recovering from a panic!!!: %q r=%#v", p.inPath, r)
				switch t := r.(type) {
				case error:
					err = t
				case string:
					err = errors.New(t)
				}
			}
		}()
	}
	err = processPDFPages(p.inPath, p.pdfReader, processPage)
	return err
}

// ProcessPDFPagesFile runs `processPage` on every page in PDF file `inPath`.
// It is a convenience function.
func ProcessPDFPagesFile(inPath string, processPage func(pageNum uint32, page *model.PdfPage) error) error {
	p, err := CreatePDFPageProcessorFile(inPath)
	if err != nil {
		return err
	}
	defer p.Close()
	return p.Process(processPage)
}

// ProcessPDFPagesReader runs `processPage` on every page in PDF file opened in `rs`.
// It is a convenience function.
func ProcessPDFPagesReader(inPath string, rs io.ReadSeeker,
	processPage func(pageNum uint32, page *model.PdfPage) error) error {

	p, err := CreatePDFPageProcessorReader(inPath, rs)
	if err != nil {
		return err
	}
	return p.Process(processPage)
}

// processPDFPages runs `processPage` on every page in PDF file `inPath`.
func processPDFPages(inPath string, pdfReader *model.PdfReader,
	processPage func(pageNum uint32, page *model.PdfPage) error) error {

	numPages, err := pdfReader.GetNumPages()
	if err != nil {
		return err
	}

	common.Log.Debug("processPDFPages: inPath=%q numPages=%d", inPath, numPages)

	for pageNum := uint32(1); pageNum <= uint32(numPages); pageNum++ {
		page, err := pdfReader.GetPage(int(pageNum))
		if err != nil {
			return err
		}
		if err = processPage(pageNum, page); err != nil {
			return err
		}
	}
	return nil
}
