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
	pdf "github.com/unidoc/unipdf/v3/model"
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
	pdf.SetPdfCreator(creatorName)

	flag.BoolVar(&Debug, "d", false, "Print debugging information.")
	flag.BoolVar(&Trace, "e", false, "Print detailed debugging information.")
	if Trace {
		Debug = true
	}
	flag.BoolVar(&ExposeErrors, "x", ExposeErrors, "Don't recover from library panics.")
}

func _SetLogging() {
	if Trace {
		common.SetLogger(common.NewConsoleLogger(common.LogLevelTrace))
	} else if Debug {
		common.SetLogger(common.NewConsoleLogger(common.LogLevelDebug))
	} else {
		common.SetLogger(common.NewConsoleLogger(common.LogLevelInfo))
	}
	common.Log.Info("Debug=%t Trace=%t", Debug, Trace)
}

// PdfOpenFile opens PDF file `inPath` and attempts to handle null encryption schemes.
func PdfOpenFile(inPath string, lazy bool) (*pdf.PdfReader, error) {

	f, err := os.Open(inPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return PdfOpenReader(f, lazy)
}

func PdfOpenReader(f io.ReadSeeker, lazy bool) (*pdf.PdfReader, error) {

	var pdfReader *pdf.PdfReader
	var err error
	if lazy {
		pdfReader, err = pdf.NewPdfReaderLazy(f)
	} else {
		pdfReader, err = pdf.NewPdfReader(f)
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

// PdfOpenDescribe returns numPages, width, height for PDF file `inPath`.
// Width and height are in mm.
func PdfOpenDescribe(inPath string) (numPages int, width, height float64, err error) {
	pdfReader, err := PdfOpenFile(inPath, true)
	if err != nil {
		return 0, 0.0, 0.0, err
	}
	return Describe(pdfReader)
}

// Describe returns numPages, width, height for the PDF in `pdfReader`.
// Width and height are in mm.
func Describe(pdfReader *pdf.PdfReader) (numPages int, width, height float64, err error) {
	pageSizes, err := pageSizeListMm(pdfReader)
	if err != nil {
		return
	}
	numPages = len(pageSizes)
	width, height = DocPageSize(pageSizes)
	return
}

// DocPageSize returns the width and height of a document whose page sizes are `pageSizes`.
// This is a single source of truth for our definition of document page size.
// Currently the document width is defined as the longest page width in the document.
func DocPageSize(pageSizes [][2]float64) (w, h float64) {
	for _, wh := range pageSizes {
		if wh[0] > w {
			w = wh[0]
		}
		if wh[1] > h {
			h = wh[1]
		}
	}
	return
}

// pageSizeListMm returns a slice of the pages sizes for the pages `pdfReader`.
// width and height are in mm.
func pageSizeListMm(pdfReader *pdf.PdfReader) (pageSizes [][2]float64, err error) {
	numPages, err := pdfReader.GetNumPages()
	if err != nil {
		return
	}

	for i := 0; i < numPages; i++ {
		pageNum := i + 1
		page := pdfReader.PageList[i]
		common.Log.Debug("==========================================================")
		common.Log.Debug("page %d", pageNum)
		var w, h float64
		w, h, err = PageSizeMm(page)
		if err != nil {
			return
		}
		size := [2]float64{w, h}
		pageSizes = append(pageSizes, size)
	}

	return
}

// PageSizeMm returns the width and height of `page` in mm.
func PageSizeMm(page *pdf.PdfPage) (width, height float64, err error) {
	width, height, err = PageSizePt(page)
	return PointToMM(width), PointToMM(height), err
}

// PageSizePt returns the width and height of `page` in points.
func PageSizePt(page *pdf.PdfPage) (width, height float64, err error) {
	b, err := page.GetMediaBox()
	if err != nil {
		return 0, 0, err
	}
	return b.Urx - b.Llx, b.Ury - b.Lly, nil
}

// ExtractPageText returns the text on page `page`.
func ExtractPageText(page *pdf.PdfPage) (string, error) {
	pageText, err := ExtractPageTextObject(page)
	if err != nil {
		return "", err
	}
	return pageText.ToText(), nil
}

// ExtractPageTextLocation returns the locations of text on page `page`.
func ExtractPageTextLocation(page *pdf.PdfPage) (string, []extractor.TextLocation, error) {
	pageText, err := ExtractPageTextObject(page)
	if err != nil {
		return "", nil, err
	}
	text, locations := pageText.ToTextLocation()
	return text, locations, nil
}

// ExtractPageTextObject returns the PageText on page `page`.
// PageText is an opaque UniDoc struct that describes the text marks on a PDF page.
// extractDocPages uses UniDoc to extract the text from all pages in PDF file `inPath` as a slice
// of PdfPage.
func ExtractPageTextObject(page *pdf.PdfPage) (*extractor.PageText, error) {
	ex, err := extractor.New(page)
	if err != nil {
		return nil, err
	}
	pageText, _, _, err := ex.ExtractPageText()
	return pageText, err
}

// PDFPageProcessor is used for processing a PDF file one page at a time.
type PDFPageProcessor struct {
	inPath    string
	pdfFile   *os.File
	pdfReader *pdf.PdfReader
}

// CreatePDFPageProcessorFile creates a  PDFPageProcessor for reading the PDF file `inPath`.
func CreatePDFPageProcessorFile(inPath string) (*PDFPageProcessor, error) {
	pdfFile, err := os.Open(inPath)
	if err != nil {
		common.Log.Error("CreatePDFPageProcessorFile: Could not open inPath=%q. err=%v", inPath, err)
		return nil, err
	}
	p, err := CreatePDFPageProcessorReader(inPath, pdfFile)
	if err != nil {
		pdfFile.Close()
		return nil, err
	}
	p.pdfFile = pdfFile
	return p, err
}

// CreatePDFPageProcessorFile creates a  PDFPageProcessor for reading the PDF file referenced by
// `rs`.
// `inPath` is provided for logging only but it is expected to be the path referenced by `rs`.
func CreatePDFPageProcessorReader(inPath string, rs io.ReadSeeker) (*PDFPageProcessor, error) {
	p := PDFPageProcessor{inPath: inPath}
	var err error

	p.pdfReader, err = PdfOpenReader(rs, true)
	if err != nil {
		common.Log.Error("CreatePDFPageProcessor: PdfOpenReader failed. inPath=%q. err=%v", inPath, err)
		return &p, err
	}
	return &p, nil
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
func (p *PDFPageProcessor) Process(processPage func(pageNum uint32, page *pdf.PdfPage) error) error {
	var err error
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
func ProcessPDFPagesFile(inPath string, processPage func(pageNum uint32, page *pdf.PdfPage) error) error {
	p, err := CreatePDFPageProcessorFile(inPath)
	if err != nil {
		return err
	}
	defer p.Close()
	return p.Process(processPage)
}

// ProcessPDFPagesFile runs `processPage` on every page in PDF file opened in `rs`.
// It is a convenience function.
func ProcessPDFPagesReader(inPath string, rs io.ReadSeeker,
	processPage func(pageNum uint32, page *pdf.PdfPage) error) error {

	p, err := CreatePDFPageProcessorReader(inPath, rs)
	if err != nil {
		return err
	}
	return p.Process(processPage)
}

// processPDFPages runs `processPage` on every page in PDF file `inPath`.
func processPDFPages(inPath string, pdfReader *pdf.PdfReader,
	processPage func(pageNum uint32, page *pdf.PdfPage) error) error {

	numPages, err := pdfReader.GetNumPages()
	if err != nil {
		return err
	}

	common.Log.Debug("processPDFPages: inPath=%q numPages=%d", inPath, numPages)

	for pageNum := uint32(1); pageNum < uint32(numPages); pageNum++ {
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
