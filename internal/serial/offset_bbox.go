// Copyright 2019 PaperCut Software International Pty Ltd. All rights reserved.

package serial

import (
	"errors"

	flatbuffers "github.com/google/flatbuffers/go"
	"github.com/papercutsoftware/pdfsearch/internal/serial/locations"
	"github.com/unidoc/unipdf/v3/common"
	"github.com/unidoc/unipdf/v3/model"
)

// OffsetBBox provides a mapping between the location of a piece of text on a PDF page and the
// offset of that piece of text in the text extracted from the PDF page.
// The text extracted from PDF pages is sent to bleve for indexing. BBox() is used to map the
// results of bleve searches (offsets in the extracted text) back to PDF contents.
// (Members need to be public because they are accessed by the doclib package.
type OffsetBBox struct {
	Offset             uint32  // Offset of the text fragment in extracted page text.
	Llx, Lly, Urx, Ury float32 // Bounding box of fragment on PDF page.
}

// BBox returns `t` as a UniDoc rectangle. This is convenient for drawing bounding rectangles around
// text in a PDF file.
func (t OffsetBBox) BBox() model.PdfRectangle {
	return model.PdfRectangle{
		Llx: float64(t.Llx),
		Lly: float64(t.Lly),
		Urx: float64(t.Urx),
		Ury: float64(t.Ury),
	}
}

// Equals returns true if `t` has the same text interval and bounding box as `u`.
func (t OffsetBBox) Equals(u OffsetBBox) bool {
	if t.Offset != u.Offset {
		return false
	}
	if t.Llx != u.Llx {
		return false
	}
	if t.Lly != u.Lly {
		return false
	}
	if t.Urx != u.Urx {
		return false
	}
	if t.Ury != u.Ury {
		return false
	}
	return true
}

// MakeDocPageLocations returns a flatbuffers serialized byte array for `ppos`.
func MakeDocPageLocations(b *flatbuffers.Builder, ppos []OffsetBBox) []byte {
	b.Reset()

	dplOfs := addDocPageLocations(b, ppos)

	// Finish the write operations by our PagePositions the root object.
	b.Finish(dplOfs)

	// return the byte slice containing encoded data:
	return b.Bytes[b.Head():]
}

// addDocPageLocations writes `ppos` to builder `b` and returns the root-table offset.
func addDocPageLocations(b *flatbuffers.Builder, ppos []OffsetBBox) flatbuffers.UOffsetT {
	var locOffsets []flatbuffers.UOffsetT
	for _, loc := range ppos {
		locOfs := addTextLocation(b, loc)
		locOffsets = append(locOffsets, locOfs)
	}
	locations.DocPageLocationsStartLocationsVector(b, len(ppos))
	// Prepend TextLocations in reverse order.
	for i := len(locOffsets) - 1; i >= 0; i-- {
		b.PrependUOffsetT(locOffsets[i])
	}
	locationsOfs := b.EndVector(len(ppos))

	// Write the PagePositions object.
	locations.DocPageLocationsStart(b)
	locations.DocPageLocationsAddLocations(b, locationsOfs)
	dplOfs := locations.DocPageLocationsEnd(b)

	return dplOfs
}

func ReadDocPageLocations(buf []byte) ([]OffsetBBox, error) {
	// Initialize a PagePositions reader from `buf`.
	ppos := locations.GetRootAsDocPageLocations(buf, 0)
	return getDocPageLocations(ppos)
}

func getDocPageLocations(sdpl *locations.PagePositions) ([]OffsetBBox, error) {

	// Vectors, such as `Locations`, have a method suffixed with 'Length' that can be used
	// to query the length of the vector. You can index the vector by passing an index value
	// into the accessor.
	var ppos []OffsetBBox
	for i := 0; i < sdpl.LocationsLength(); i++ {
		var sloc locations.TextLocation
		ok := sdpl.Locations(&sloc, i)
		if !ok {
			return []OffsetBBox{}, errors.New("bad TextLocation")
		}
		ppos = append(ppos, getTextLocation(&sloc))
	}
	common.Log.Debug("ReadDocPageLocations: ppos=%d", len(ppos))
	return ppos, nil
}

// MakeTextLocation returns a flatbuffers serialized byte array for `loc`.
func MakeTextLocation(b *flatbuffers.Builder, loc OffsetBBox) []byte {
	// Re-use the already-allocated Builder.
	b.Reset()

	// Write the TextLocation object.
	locOffset := addTextLocation(b, loc)

	// Finish the write operations by our TextLocation the root object.
	b.Finish(locOffset)

	// Return the byte slice containing encoded data.
	return b.Bytes[b.Head():]
}

// addTextLocation writes `loc` to builder `b` and returns the root-table offset.
func addTextLocation(b *flatbuffers.Builder, loc OffsetBBox) flatbuffers.UOffsetT {
	// Write the TextLocation object.
	locations.TextLocationStart(b)
	locations.TextLocationAddOffset(b, loc.Offset)
	locations.TextLocationAddLlx(b, loc.Llx)
	locations.TextLocationAddLly(b, loc.Lly)
	locations.TextLocationAddUrx(b, loc.Urx)
	locations.TextLocationAddUry(b, loc.Ury)
	return locations.TextLocationEnd(b)
}

func ReadTextLocation(buf []byte) OffsetBBox {
	// Initialize a TextLocation reader from `buf`.
	loc := locations.GetRootAsTextLocation(buf, 0)
	return getTextLocation(loc)
}

func getTextLocation(loc *locations.TextLocation) OffsetBBox {
	// Copy the TextLocation's fields (since these are numbers).
	return OffsetBBox{
		loc.Offset(),
		loc.Llx(),
		loc.Lly(),
		loc.Urx(),
		loc.Ury(),
	}
}
