// Copyright 2019 PaperCut Software International Pty Ltd. All rights reserved.

package serial

import (
	"errors"

	flatbuffers "github.com/google/flatbuffers/go"
	"github.com/papercutsoftware/pdfsearch/internal/serial/locations"
	"github.com/unidoc/unipdf/v3/common"
)

// OffsetBBox describes the location of a text fragment on a PDF page.
// The text of PDF pages is extracted and sent to bleve for indexing. BBox is used to map
// the results of bleve searches back to PDF contents.
// bleve searches returns offsets of the matches on the extracted text.
// Members need to be public because they are accessedn by serial package.
type OffsetBBox struct {
	Offset             uint32  // Offset of the text fragment in extracted page text.
	Llx, Lly, Urx, Ury float32 // Bounding box of fragment on PDF page.
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

// MakeDocPageLocations returns a flatbuffers serialized byte array for `dpl`.
func MakeDocPageLocations(b *flatbuffers.Builder, dpl []OffsetBBox) []byte {
	b.Reset()

	dplOfs := addDocPageLocations(b, dpl)

	// Finish the write operations by our DocPageLocations the root object.
	b.Finish(dplOfs)

	// return the byte slice containing encoded data:
	return b.Bytes[b.Head():]
}

// addDocPageLocations writes `dpl` to builder `b` and returns the root-table offset.
func addDocPageLocations(b *flatbuffers.Builder, dpl []OffsetBBox) flatbuffers.UOffsetT {
	var locOffsets []flatbuffers.UOffsetT
	for _, loc := range dpl {
		locOfs := addTextLocation(b, loc)
		locOffsets = append(locOffsets, locOfs)
	}
	locations.DocPageLocationsStartLocationsVector(b, len(dpl))
	// Prepend TextLocations in reverse order.
	for i := len(locOffsets) - 1; i >= 0; i-- {
		b.PrependUOffsetT(locOffsets[i])
	}
	locationsOfs := b.EndVector(len(dpl))

	// Write the DocPageLocations object.
	locations.DocPageLocationsStart(b)
	locations.DocPageLocationsAddLocations(b, locationsOfs)
	dplOfs := locations.DocPageLocationsEnd(b)

	return dplOfs
}

func ReadDocPageLocations(buf []byte) ([]OffsetBBox, error) {
	// Initialize a DocPageLocations reader from `buf`.
	dpl := locations.GetRootAsDocPageLocations(buf, 0)
	return getDocPageLocations(dpl)
}

func getDocPageLocations(sdpl *locations.DocPageLocations) ([]OffsetBBox, error) {

	// Vectors, such as `Locations`, have a method suffixed with 'Length' that can be used
	// to query the length of the vector. You can index the vector by passing an index value
	// into the accessor.
	var dpl []OffsetBBox
	for i := 0; i < sdpl.LocationsLength(); i++ {
		var sloc locations.TextLocation
		ok := sdpl.Locations(&sloc, i)
		if !ok {
			return []OffsetBBox{}, errors.New("bad TextLocation")
		}
		dpl = append(dpl, getTextLocation(&sloc))
	}
	common.Log.Debug("ReadDocPageLocations: dpl=%d", len(dpl))
	return dpl, nil
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
