// Copyright 2019 PaperCut Software International Pty Ltd. All rights reserved.

package doclib

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/unidoc/unipdf/v3/common"
	"github.com/unidoc/unipdf/v3/creator"
	"github.com/unidoc/unipdf/v3/model"
)

// ExtractList is a list of PDF file:page inputs that are to be marked up then combined in a
// specificed order.
// If i is the (0-offset) ith page, then content is the contents to be added to this page.
// src := sources[i]
// content := contents[src.inPath][src.pageNum]
type ExtractList struct {
	maxPages   int                               // Maximum number of pages in output PDF.
	maxPerPage int                               // Maximum number of objects to add to each page.
	sources    []pdfPage                         // Source pages in order they will be combined.
	sourceSet  map[string]bool                   // Used to ensure PDF pages are only added once.
	contents   map[string]map[uint32]pageContent // contents[inPath][pageNum] is the contents to be added to inPath:pageNum.
}

var errMissing = errors.New("Missing value")

// pdfPage is a PDF page.
type pdfPage struct {
	inPath  string // Path of PDF that page comes from.
	pageNum uint32 // Page number (1-offset) of page in source document.
}

// pageContent is instructions for adding items to a PDF page.
// Currently this is a list of rectangles.
type pageContent struct {
	rects []model.PdfRectangle // the rectangles to be drawn on the PDF page
	page  *model.PdfPage       // the UniDoc PDF page. Created as needed.
}

// String returns a string describing `l`.
func (l ExtractList) String() string {
	parts := []string{fmt.Sprintf("maxPages: %d", l.maxPages)}
	for i, src := range l.sources {
		parts = append(parts, fmt.Sprintf("%6d: %20q:%d", i, filepath.Base(src.inPath), src.pageNum))
	}
	return strings.Join(parts, "\n")
}

// CreateExtractList returns an empty *ExtractList with `maxPages` maximum number of pages and
// `maxPerPage` maximum rectangles per page.
func CreateExtractList(maxPages, maxPerPage int) *ExtractList {
	return &ExtractList{
		maxPages:   maxPages,
		maxPerPage: maxPerPage,
		contents:   map[string]map[uint32]pageContent{},
		sourceSet:  map[string]bool{},
	}
}

// AddRect adds to `l`, instructions to draw rectangle `r` on (1-offset) page number `pageNum` of
// PDF file `inPath`
func (l *ExtractList) AddRect(inPath string, pageNum uint32, r model.PdfRectangle) {
	common.Log.Debug("AddRect: %q %3d %v", filepath.Base(inPath), pageNum, r)
	if pageNum == 0 {
		common.Log.Error("inPath=%q pageNum=%d", inPath, pageNum)
		panic("pageNum = 0")
	}
	pathPage := fmt.Sprintf("%s.%d", inPath, pageNum)
	if !l.sourceSet[pathPage] {
		if len(l.sourceSet) >= l.maxPages {
			common.Log.Info("AddRect: %q:%d len=%d MAX PAGES EXCEEDED", inPath, pageNum)
			return
		}
		l.sourceSet[pathPage] = true
		l.sources = append(l.sources, pdfPage{inPath, pageNum})
		common.Log.Debug("AddRect: %q:%d len=%d", filepath.Base(inPath), pageNum, len(l.sourceSet))
	}

	docContent, ok := l.contents[inPath]
	if !ok {
		docContent = map[uint32]pageContent{}
		l.contents[inPath] = docContent
	}
	pageContent := docContent[pageNum]
	if len(pageContent.rects) >= l.maxPerPage {
		common.Log.Debug("AddRect: Maximum number of rectangle per page reached. %d", l.maxPerPage)
		return
	}
	pageContent.rects = append(pageContent.rects, r)
	docContent[pageNum] = pageContent
}

const (
	// BorderWidth is the width of rectangle sides in points
	BorderWidth = 3.0
	// ShadowWidth is the with of the shadow on the inside and outside of the rectangles
	ShadowWidth = 0.2
)

// SaveOutputPdf is called  to markup a PDF file with the locations of text.
// `l` contains the input PDF names and the pages and coordinates to mark.
// The resulting PDF is written to `outPath`.
func (l *ExtractList) SaveOutputPdf(outPath string) error {
	common.Log.Debug("l=%s", *l)
	for inPath, docContents := range l.contents {
		f, pdfReader, err := PdfOpenFileLazy(inPath)
		if err != nil {
			common.Log.Error("SaveOutputPdf: Could not open inPath=%q. err=%v", inPath, err)
			return err
		}
		defer f.Close()

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
			mediaBox, err := page.GetMediaBox()
			if err == nil && page.MediaBox == nil {
				common.Log.Info("$$$ MediaBox: %v -> %v", page.MediaBox, mediaBox)
				page.MediaBox = mediaBox
			}
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
		mediaBox, err := pageContent.page.GetMediaBox()
		if err != nil {
			common.Log.Error("%d: GetMediaBox returned err=%v", i, err)
			continue
			return err
		}
		if err := c.AddPage(pageContent.page); err != nil {
			common.Log.Error("%d: %+v ", i, src)
			return err
		}
		outPages++

		h := mediaBox.Ury
		shift := 2.0 // !@#$ Hack to line up highlight box
		for _, r := range pageContent.rects {
			common.Log.Debug("SaveOutputPdf: %q:%d %s", filepath.Base(src.inPath), src.pageNum, rectString(r))
			rect := c.NewRectangle(r.Llx, h-r.Lly+shift, r.Urx-r.Llx, -(r.Ury - r.Lly + shift))
			rect.SetBorderColor(creator.ColorRGBFromHex("#ffffff")) // White border shadow.
			rect.SetBorderWidth(BorderWidth + 2*ShadowWidth)
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

// rectString returns a string describing `r`.
func rectString(r model.PdfRectangle) string {
	return fmt.Sprintf("{llx: %4.1f lly: %4.1f urx: %4.1f ury: %4.1f} %.1f x %.1f",
		r.Llx, r.Lly, r.Urx, r.Ury, r.Urx-r.Llx, r.Ury-r.Lly)
}

// contentsKeys returns the keys of `contents` sorted alphabetically.
func contentsKeys(contents map[string]map[uint32]pageContent) []string {
	var keys []string
	for k := range contents {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
