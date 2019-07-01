// Copyright 2019 PaperCut Software International Pty Ltd. All rights reserved.

/*
 * Insert and image at specified coordinates on a specified pages(s) in a PDf file.
 * If unsure about position, try getting the dimensions of a PDF with
 * unidoc-examples/unipdf/pages/pdf_page_info.go first,
 * or just start with 0,0 and increase to move right, down.
 */

package pdfsearch

import (
	"fmt"
	goimage "image"
	"io"

	"github.com/papercutsoftware/pdfsearch/internal/doclib"
	"github.com/papercutsoftware/pdfsearch/internal/utils"
	"github.com/unidoc/unipdf/v3/creator"
)

// ImageLocation specifies the location of a square image on a page.
type ImageLocation struct {
	PagePosition         // Enumerated position.
	XPosMm       float64 // Custom x coordinate in points (positive from right, negative from left).
	YPosMm       float64 // Custom y coordinate in points (positive from bottom, negative from top).
	WidthMm      float64 // Width of the image in mm.
	HeightMm     float64 // Height of the image in mm.
	MarginXMm    float64 // Horizontal page margin in mm.
	MarginYMm    float64 // Vertical page margin in mm.
}

// PagePosition is an enumerated position on a page.
type PagePosition int

const (
	PageBottomRight = iota
	PageBottomCenter
	PageBottomLeft
	PageCenterRight
	PageCenter
	PageCenterLeft
	PageTopRight
	PageTopCenter
	PageTopLeft
	PageCustomPosition
)

// coords returns the page coordinates in points from top-left corresponding to ImageLocation `loc`.
//  - w: width of page in points.
//  - h: height of page in points.
func (loc ImageLocation) coords(w, h float64) (float64, float64) {
	xPos := utils.MMToPoint(loc.XPosMm)
	yPos := utils.MMToPoint(loc.YPosMm)
	width := utils.MMToPoint(loc.WidthMm)
	height := utils.MMToPoint(loc.HeightMm)
	marginX := utils.MMToPoint(loc.MarginXMm)
	marginY := utils.MMToPoint(loc.MarginYMm)

	// The barcode has dimensions width x width and is postioned at its top left so it needs
	// to be positioned in the region x=0..w-width y=0..h-width to be inside the page.
	w -= width
	h -= height

	switch loc.PagePosition {
	case PageTopLeft:
		return marginX, marginY
	case PageTopCenter:
		return w / 2, marginY
	case PageTopRight:
		return w - marginX, marginY
	case PageCenterLeft:
		return marginX, h / 2
	case PageCenter:
		return w / 2, h / 2
	case PageCenterRight:
		return w - marginX, h / 2
	case PageBottomLeft:
		return marginX, h - marginY
	case PageBottomCenter:
		return w / 2, h - marginY
	case PageBottomRight:
		return w - marginX, h - marginY
	case PageCustomPosition:
		// Positive (or zero): From bottom right.
		x := w - xPos
		y := h - yPos
		// Negative: From top left.
		if xPos < 0 {
			x = -xPos
		}
		if yPos < 0 {
			y = -yPos
		}
		return x, y
	}
	panic(fmt.Errorf("bad PagePosition: loc=%+v", loc))
}

// AddImageToPdf adds an image to a specific page of a PDF.
// NOTE: This function adds the same image at the same position on every page.
//  - rs: io.ReadSeeker for input PDF.
//  - w: io.Writer for output (modified) PDF.
//  - img: Image to be applied to pages.
//  - pageNum: (1-offset) page number to apply image to.
//    Specify 0 for all pages.
//    Specify a negative number to count back from the last page.  For example -1 = last page, -2 = second last page.
// The image's aspect ratio is maintained.
func AddImageToPdf(rs io.ReadSeeker, w io.Writer, image goimage.Image, url string, pageNum int,
	loc ImageLocation) error {

	width := utils.MMToPoint(loc.WidthMm)
	height := utils.MMToPoint(loc.HeightMm)

	// b := img.Bounds()
	// fmt.Printf(" *** img: %d x %d pixels\n", b.Max.X, b.Max.Y)

	// Read the input PDF file.
	pdfReader, err := doclib.PdfOpenReader(rs, true)
	if err != nil {
		return err
	}

	numPages, err := pdfReader.GetNumPages()
	if err != nil {
		return err
	}

	if pageNum < 0 {
		// count back from last page and protect against overflow
		pageNum = 1 + numPages + pageNum
		if pageNum < 1 {
			pageNum = 1
		}
	}

	// Make a new PDF creator.
	c := creator.New()

	img, err := c.NewImageFromGoImage(image)
	if err != nil {
		return err
	}
	img.SetWidth(width)
	img.SetHeight(height)

	// Load the pages and add to creator.  Apply the image to the specified page.
	for pNum := 1; pNum <= numPages; pNum++ {
		page, err := pdfReader.GetPage(pNum)
		if err != nil {
			return err
		}

		w, h, err := doclib.PageSizePt(page)
		if err != nil {
			return err
		}

		// fmt.Printf("page %d: %.1f x %.1f\n", pNum, w, h)

		err = c.AddPage(page)
		if err != nil {
			return err
		}

		// Apply the image to the specified page or all pages if 0.
		if pNum == pageNum || pageNum == 0 {

			x, y := loc.coords(w, h)
			// fmt.Printf("*** pagePosition=%d => %.1f, %.1f\n", pagePosition, x, y)

			if len(url) != 0 {
				p := c.NewStyledParagraph()

				style := c.NewTextStyle()
				style.Color = creator.ColorRGBFrom8bit(255, 255, 255)
				style.FontSize = width
				p.SetTextAlignment(creator.TextAlignmentJustify)
				p.SetMargins(0, 0, 0, 0)

				p.AddExternalLink("M", url).Style = style
				p.SetPos(x, y+height*0.8)
				p.SetWidth(width)
				err = c.Draw(p)
				if err != nil {
					return err
				}
			}

			// fmt.Printf(" *** SetPos(%.1f, %.1f)\n", x, y)
			img.SetPos(x, y)
			err = c.Draw(img)
			if err != nil {
				return err
			}
		}
	}

	return c.Write(w)
}
