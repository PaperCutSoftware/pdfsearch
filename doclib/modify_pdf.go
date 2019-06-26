// Copyright 2019 PaperCut Software International Pty Ltd. All rights reserved.

package doclib

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/unidoc/unipdf/v3/common"
	"github.com/unidoc/unipdf/v3/creator"
	pdf "github.com/unidoc/unipdf/v3/model"
)

// ExtractList is a list of document:page inputs that are to be combined in a specified order.
type ExtractList struct {
	maxPages  int
	sources   []Extract // Source pages in order they will be combined
	sourceSet map[string]bool
	contents  map[string]map[uint32]pageContent // Pages for each document
	// documentIndex map[string]int
}

func (l ExtractList) String() string {
	parts := []string{fmt.Sprintf("maxPages: %d", l.maxPages)}
	for i, src := range l.sources {
		parts = append(parts, fmt.Sprintf("%6d: %20q:%d", i, filepath.Base(src.inPath), src.pageNum))
	}
	return strings.Join(parts, "\n")
}

type Extract struct {
	inPath  string // Path of PDF that page comes from.
	pageNum uint32 // Page number (1-offset) of page in source document
}

type pageContent struct {
	rects []pdf.PdfRectangle // the rectangles to be drawn on the PDF page
	page  *pdf.PdfPage       // the UniDoc PDF page. Created as needed.
}

// type DocContents struct {
// 	pageNums []int          // page number (1-offset) of page in source document
// 	pages    []*pdf.PdfPage // pages
// }

func (l *ExtractList) AddRect(inPath string, pageNum uint32, r pdf.PdfRectangle) {
	common.Log.Debug("AddRect %q %3d %v", filepath.Base(inPath), pageNum,r)
	pathPage := fmt.Sprintf("%s.%d", inPath, pageNum)
	if !l.sourceSet[pathPage] {
		if len(l.sourceSet) >= l.maxPages {
			common.Log.Debug("AddRect: %q:%d len=%d MAX PAGES EXCEEDED", inPath, pageNum)
			return
		}
		l.sourceSet[pathPage] = true
		l.sources = append(l.sources, Extract{inPath, pageNum})
		common.Log.Debug("AddRect: %q:%d len=%d", filepath.Base(inPath), pageNum, len(l.sourceSet))
	}

	docContent, ok := l.contents[inPath]
	if !ok {
		docContent = map[uint32]pageContent{}
		l.contents[inPath] = docContent
	}
	pageContent := docContent[pageNum]
	if len(pageContent.rects) >= 3 {
		return // !@#$
	}
	pageContent.rects = append(pageContent.rects, r)
	if pageNum == 0 {
		common.Log.Error("inPath=%q pageNum=%d", inPath, pageNum)
	}
	docContent[pageNum] = pageContent
}

func CreateExtractList(maxPages int) *ExtractList {
	return &ExtractList{
		maxPages:  maxPages,
		contents:  map[string]map[uint32]pageContent{},
		sourceSet: map[string]bool{},
	}
}

func (l *ExtractList) NumPages() int {
	return len(l.sources)
}

const BorderWidth = 3.0               // !@#$ For testing.
const ShadowWidth = BorderWidth + 0.5 // !@#$ For testing.

// SaveOutputPdf is called by position_search.go to markup a PDF file with the locations of text.
// `l` contains the input PDF names and the pages and coordinates to mark.
// The resulting PDF is written to `outPath`.
func (l *ExtractList) SaveOutputPdf(outPath string) error {
	common.Log.Debug("l=%s", *l)
	for inPath, docContents := range l.contents {
		pdfReader, err := PdfOpenFile(inPath, false)
		if err != nil {
			common.Log.Error("SaveOutputPdf: Could not open inPath=%q. err=%v", inPath, err)
			return err
		}

		for pageNum := range docContents {
			common.Log.Debug("SaveOutputPdf: %q %d", inPath, pageNum)
			page, err := pdfReader.GetPage(int(pageNum))
			if err != nil {
				common.Log.Error("SaveOutputPdf: Could not get page inPath=%q pageNum=%d. err=%v",
					inPath, pageNum, err)
				return err
			}
			pageContent := l.contents[inPath][pageNum]
			pageContent.page = page
			l.contents[inPath][pageNum] = pageContent
		}
	}

	common.Log.Debug("SaveOutputPdf: outPath=%q sources=%d", outPath, len(l.sources))

	// Make a new PDF creator.
	c := creator.New()

	outPages := 0

	for i, src := range l.sources {
		docContent, ok := l.contents[src.inPath]
		if !ok {
			common.Log.Error("SaveOutputPdf: Not in l.contents. %d: %+v", i, src)
			return errMissing
		}
		pageContent, ok := docContent[src.pageNum]
		if !ok {
			common.Log.Error("%d: No pageContent. %+v", i, src)
			continue
			return errMissing
		}
		if pageContent.page == nil {
			common.Log.Error("%d: No page. %+v", i, src)
			continue
			return errMissing
		}
		if pageContent.page.MediaBox == nil {
			common.Log.Error("%d: No MediaBox. %+v", i, src)
			continue
			return errMissing
		}
		if err := c.AddPage(pageContent.page); err != nil {
			common.Log.Error("%d: %+v ", i, src)
			return err
		}
		outPages++

		h := pageContent.page.MediaBox.Ury
		shift := 2.0 // !@#$ Hack to line up highlight box
		for _, r := range pageContent.rects {
			common.Log.Info("SaveOutputPdf: %q:%d %s", filepath.Base(src.inPath), src.pageNum, rectString(r))
			rect := c.NewRectangle(r.Llx, h-r.Lly+shift, r.Urx-r.Llx, -(r.Ury - r.Lly + shift))
			// rect := c.NewRectangle(r.Llx, r.Lly, r.Urx-r.Llx, r.Ury-r.Lly)
			rect.SetBorderColor(creator.ColorRGBFromHex("#ffffff")) // White border shadow.
			rect.SetBorderWidth(ShadowWidth)
			if err := c.Draw(rect); err != nil {
				return err
			}
			rect.SetBorderColor(creator.ColorRGBFromHex("#0000ff")) // Red border.
			rect.SetBorderWidth(BorderWidth)
			if err := c.Draw(rect); err != nil {
				return err
			}
		}
	}
	if outPages == 0 {
		return errors.New("no pages in marked up PDF")
	}

	return c.WriteToFile(outPath)
}

var errMissing = errors.New("Missing value")

func rectString(r pdf.PdfRectangle) string {
	return fmt.Sprintf("{llx: %4.1f lly: %4.1f urx: %4.1f ury: %4.1f} %.1f x %.1f",
		r.Llx, r.Lly, r.Urx, r.Ury, r.Urx-r.Llx, r.Ury-r.Lly)
}

// func DrawPdfRectangle(inPath, outPath string, pageNumber int, llx, lly, urx, ury float64) error {
// 	return ModifyPdfPage(inPath, outPath, pageNumber,
// 		func(c *creator.Creator) error {
// 			rect := c.NewRectangle(llx, lly, urx-llx, ury-lly)
// 			return c.Draw(rect)
// 		})
// }

// func ModifyPdfPage(inPath, outPath string, pageNumber int,
// 	processPage func(c *creator.Creator) error) error {

// 	pdfReader, err := PdfOpenFile(inPath)
// 	if err != nil {
// 		common.Log.Error("ModifyPdfPage: Could not open inPath=%q. err=%v", inPath, err)
// 		return err
// 	}
// 	numPages, err := pdfReader.GetNumPages()
// 	if err != nil {
// 		return err
// 	}

// 	common.Log.Info("ModifyPdfPage: inPath=%q numPages=%d", inPath, numPages)

// 	// Make a new PDF creator.
// 	c := creator.New()

// 	for pageNum := 1; pageNum < numPages; pageNum++ {
// 		page, err := pdfReader.GetPage(pageNum)
// 		if err != nil {
// 			return err
// 		}
// 		err = c.AddPage(page)
// 		if err != nil {
// 			return err
// 		}
// 		if pageNum == pageNumber {
// 			if err = processPage(c); err != nil {
// 				return err
// 			}
// 		}
// 	}

// 	return c.WriteToFile(outPath)
// }
