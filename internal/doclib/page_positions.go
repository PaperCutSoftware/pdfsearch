// Copyright 2019 PaperCut Software International Pty Ltd. All rights reserved.

package doclib

import (
	"fmt"
	"math"
	"sort"

	"github.com/papercutsoftware/pdfsearch/internal/serial"
	"github.com/unidoc/unipdf/v3/common"
	"github.com/unidoc/unipdf/v3/extractor"
	pdf "github.com/unidoc/unipdf/v3/model"
)

// PagePositions is used to the link per-document data in a bleve index to the PDF file that the
// per-document data was extracted from.
// There is one PagePositions per page.
// PagePositions stores the locations of text fragments on a page. The search index includes a
// binary copy of PagePositions, so our goal is to make PagePositions compact.
type PagePositions struct {
	locations []serial.OffsetBBox
}

// Equals returns true if `dpl` contains the same information as `epl`.
func (dpl PagePositions) Equals(epl PagePositions) bool {
	if len(dpl.locations) != len(epl.locations) {
		return false
	}
	for i, dloc := range dpl.locations {
		eloc := epl.locations[i]
		if !dloc.Equals(eloc) {
			return false
		}
	}
	return true
}

// String returns a string describing PagePositions `dpl`.
func (dpl PagePositions) String() string {
	return fmt.Sprintf("{PagePositions: %d}", len(dpl.locations))
}

// Empty return true if `dpl` has no entries.
func (dpl PagePositions) Empty() bool {
	return len(dpl.locations) == 0
}

// PagePositionsFromLocations converts []extractor.TextMark `locations` to a more compact
// PagePositions.
// We do this because PagePositions is stored in our index which we want to be small.
func PagePositionsFromLocations(locations []extractor.TextMark) PagePositions {
	var dpl PagePositions
	for _, uloc := range locations {
		loc := fromExtractorLocation(uloc)
		dpl.locations = append(dpl.locations, loc)
	}
	return dpl
}

// fromExtractorLocation converts extractor.TextMark `uloc` to a more compact serial.OffsetBBox.
func fromExtractorLocation(uloc extractor.TextMark) serial.OffsetBBox {
	b := uloc.BBox
	return serial.OffsetBBox{
		Offset: uint32(uloc.Offset),
		Llx:    float32(b.Llx),
		Lly:    float32(b.Lly),
		Urx:    float32(b.Urx),
		Ury:    float32(b.Ury),
	}
}

// BBox returns a rectangle that bounds the text with offsets
// ofs: `start` <= ofs <= `end` on the PDF page indexed by `dpl`.
// Caller must check that dpl.locations is not empty.
func (dpl PagePositions) BBox(start, end uint32) pdf.PdfRectangle {
	i0, ok0 := dpl.positionIndex(end)
	i1, ok1 := dpl.positionIndex(start)
	if !(ok0 && ok1) {
		return pdf.PdfRectangle{}
	}
	p0, p1 := dpl.locations[i0], dpl.locations[i1]
	return pdf.PdfRectangle{
		Llx: math.Min(float64(p0.Llx), float64(p1.Llx)),
		Lly: math.Min(float64(p0.Lly), float64(p1.Lly)),
		Urx: math.Max(float64(p0.Urx), float64(p1.Urx)),
		Ury: math.Max(float64(p0.Ury), float64(p1.Ury)),
	}
}

// positionIndex returns the index of the element of dpl.locations that spans `offset`
// (i.e  idx: dpl.locations[idx] <= offset < dpl.locations[idx+1])
// Caller must check that dpl.locations is not empty.
func (dpl PagePositions) positionIndex(offset uint32) (int, bool) {
	positions := dpl.locations
	if len(positions) == 0 {
		common.Log.Error("positionIndex: No positions")
		panic("no positions")
	}
	i := sort.Search(len(positions), func(i int) bool { return positions[i].Offset >= offset })
	ok := 0 <= i && i < len(positions)
	if !ok {
		common.Log.Error("positionIndex: offset=%d i=%d len=%d %v==%v", offset, i, len(positions),
			positions[0], positions[len(positions)-1])
	}
	return i, ok
}
