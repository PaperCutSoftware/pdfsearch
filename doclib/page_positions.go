// Copyright 2019 PaperCut Software International Pty Ltd. All rights reserved.

package doclib

import (
	"fmt"
	"math"
	"sort"

	"github.com/papercutsoftware/pdfsearch/serial"
	"github.com/unidoc/unipdf/v3/common"
	"github.com/unidoc/unipdf/v3/extractor"
	pdf "github.com/unidoc/unipdf/v3/model"
)

// DocPageLocations stores the locations of text fragments on a page.
// The search index includes a binary copy of DocPageLocations, so our goal is to make
// DocPageLocations compact.
type DocPageLocations struct {
	locations []serial.OffsetBBox
}

// Equals returns true if `dpl` contains the same information as `epl`.
func (dpl DocPageLocations) Equals(epl DocPageLocations) bool {
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

func (dpl DocPageLocations) String() string {
	return fmt.Sprintf("{DocPageLocations: %d}", len(dpl.locations))
}

func (dpl DocPageLocations) Len() int {
	return len(dpl.locations)
}

func (dpl DocPageLocations) Locations() []serial.OffsetBBox {
	return dpl.locations
}

func (dpl *DocPageLocations) AppendTextLocation(loc serial.OffsetBBox) {
	dpl.locations = append(dpl.locations, loc)
}



// DplFromExtractorLocations converts []extractor.TextLocation `locations` to a more compact
// DocPageLocations.
// We do this because DocPageLocations is stored in our index.
func DplFromExtractorLocations(locations []extractor.TextLocation) DocPageLocations {
	var dpl DocPageLocations
	for _, uloc := range locations {
		loc := fromExtractorLocation(uloc)
		dpl.locations = append(dpl.locations, loc)
	}
	return dpl
}

// fromExtractorLocation converts extractor.TextLocation `loc` to a more compact serial.OffsetBBox.
func fromExtractorLocation(uloc extractor.TextLocation) serial.OffsetBBox {
	b := uloc.BBox
	return serial.OffsetBBox{
		Offset: uint32(uloc.Offset),
		Llx:    float32(b.Llx),
		Lly:    float32(b.Lly),
		Urx:    float32(b.Urx),
		Ury:    float32(b.Ury),
	}
}

// GetBBox returns a rectangle that bounds the text with offsets
// ofs: `start` <= ofs <= `end` on the PDF page indexed by `dpl`.
// Caller must check that dpl.locations is not empty.
func (dpl DocPageLocations) GetBBox(start, end uint32) pdf.PdfRectangle {
	i0, ok0 := dpl.getPositionIndex(end)
	i1, ok1 := dpl.getPositionIndex(start)
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

// getPositionIndex returns the index of the element of dpl.locations that spans `offset`
// (i.e  idx: dpl.locations[idx] <= offset < dpl.locations[idx+1])
// Caller must check that dpl.locations is not empty.
func (dpl DocPageLocations) getPositionIndex(offset uint32) (int, bool) {
	positions := dpl.locations
	if len(positions) == 0 {
		common.Log.Error("getPositionIndex: No positions")
		panic("no positions")
	}
	i := sort.Search(len(positions), func(i int) bool { return positions[i].Offset >= offset })
	ok := 0 <= i && i < len(positions)
	if !ok {
		common.Log.Error("getPositionIndex: offset=%d i=%d len=%d %v==%v", offset, i, len(positions),
			positions[0], positions[len(positions)-1])
	}
	return i, ok
}
