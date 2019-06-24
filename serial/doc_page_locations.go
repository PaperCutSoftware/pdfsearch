// Copyright 2019 PaperCut Software International Pty Ltd. All rights reserved.

package serial

import (
	"errors"

	flatbuffers "github.com/google/flatbuffers/go"
	"github.com/papercutsoftware/pdfsearch/base"
	"github.com/papercutsoftware/pdfsearch/serial/locations"
	"github.com/unidoc/unipdf/v3/common"
)

// MakeDocPageLocations returns a flatbuffers serialized byte array for `dpl`.
func MakeDocPageLocations(b *flatbuffers.Builder, dpl base.DocPageLocations) []byte {
	b.Reset()

	dplOfs := addDocPageLocations(b, dpl)

	// Finish the write operations by our DocPageLocations the root object.
	b.Finish(dplOfs)

	// return the byte slice containing encoded data:
	return b.Bytes[b.Head():]
}

// addDocPageLocations writes `dpl` to builder `b` and returns the root-table offset.
func addDocPageLocations(b *flatbuffers.Builder, dpl base.DocPageLocations) flatbuffers.UOffsetT {
	var locOffsets []flatbuffers.UOffsetT
	for _, loc := range dpl.Locations() {
		locOfs := addTextLocation(b, loc)
		locOffsets = append(locOffsets, locOfs)
	}
	locations.DocPageLocationsStartLocationsVector(b, dpl.Len())
	// Prepend TextLocations in reverse order.
	for i := len(locOffsets) - 1; i >= 0; i-- {
		b.PrependUOffsetT(locOffsets[i])
	}
	locationsOfs := b.EndVector(dpl.Len())

	// Write the DocPageLocations object.
	locations.DocPageLocationsStart(b)
	locations.DocPageLocationsAddLocations(b, locationsOfs)
	dplOfs := locations.DocPageLocationsEnd(b)

	return dplOfs
}

func ReadDocPageLocations(buf []byte) (base.DocPageLocations, error) {
	// Initialize a DocPageLocations reader from `buf`.
	dpl := locations.GetRootAsDocPageLocations(buf, 0)
	return getDocPageLocations(dpl)
}

func getDocPageLocations(sdpl *locations.DocPageLocations) (base.DocPageLocations, error) {

	// Vectors, such as `Locations`, have a method suffixed with 'Length' that can be used
	// to query the length of the vector. You can index the vector by passing an index value
	// into the accessor.
	var dpl base.DocPageLocations
	for i := 0; i < sdpl.LocationsLength(); i++ {
		var sloc locations.TextLocation
		ok := sdpl.Locations(&sloc, i)
		if !ok {
			return base.DocPageLocations{}, errors.New("bad TextLocation")
		}
		dpl.AppendTextLocation(getTextLocation(&sloc))
	}
	common.Log.Debug("ReadDocPageLocations: dpl=%s", dpl)
	return dpl, nil
}

// MakeTextLocation returns a flatbuffers serialized byte array for `loc`.
func MakeTextLocation(b *flatbuffers.Builder, loc base.TextLocation) []byte {
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
func addTextLocation(b *flatbuffers.Builder, loc base.TextLocation) flatbuffers.UOffsetT {
	// Write the TextLocation object.
	locations.TextLocationStart(b)
	locations.TextLocationAddOffset(b, loc.Start)
	locations.TextLocationAddLlx(b, loc.Llx)
	locations.TextLocationAddLly(b, loc.Lly)
	locations.TextLocationAddUrx(b, loc.Urx)
	locations.TextLocationAddUry(b, loc.Ury)
	return locations.TextLocationEnd(b)
}

func ReadTextLocation(buf []byte) base.TextLocation {
	// Initialize a TextLocation reader from `buf`.
	loc := locations.GetRootAsTextLocation(buf, 0)
	return getTextLocation(loc)
}

func getTextLocation(loc *locations.TextLocation) base.TextLocation {
	// Copy the TextLocation's fields (since these are numbers).
	return base.TextLocation{
		loc.Offset(),
		0,
		loc.Llx(),
		loc.Lly(),
		loc.Urx(),
		loc.Ury(),
	}
}
