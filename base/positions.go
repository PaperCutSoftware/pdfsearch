// Copyright 2019 PaperCut Software International Pty Ltd. All rights reserved.

package base

import (
	"fmt"
	"sort"

	"github.com/unidoc/unipdf/v3/common"
	"github.com/unidoc/unipdf/v3/extractor"
)

// DocPageLocations stores the locations of text fragments on a page.
type DocPageLocations struct {
	locations []TextLocation
}

// TextLocation describes the location of a text fragment on a PDF page.
// The text of PDF pages is extracted and sent to bleve for indexing. TextLocation is used to map
// the results of bleve searches back to PDF contents.
// bleve searches returns offsets of the matches on the extracted text.
type TextLocation struct {
	Start, End         uint32  // Offsets of start and end of the fragment in extracted page text.
	Llx, Lly, Urx, Ury float32 // Bounding box of fragment on PDF page.
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

func (dpl DocPageLocations) Locations() []TextLocation {
	return dpl.locations
}

func (dpl *DocPageLocations) AppendTextLocation(loc TextLocation) {
	dpl.locations = append(dpl.locations, loc)
}

// Equals returns true if `t` has the same text interval and bounding box as `u`.
func (t TextLocation) Equals(u TextLocation) bool {
	if t.Start != u.Start {
		return false
	}
	if t.End != u.End {
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

func (t TextLocation) String() string {
	return fmt.Sprintf("{TextLocation: %d:%d (%5.1f, %5.1f) (%5.1f, %5.1f)}",
		t.Start, t.End,
		t.Llx, t.Lly, t.Urx, t.Ury)
}

// DplFromExtractorLocations converts []extractor.TextLocation `locations` to a more compact
// base.DocPageLocations.
// Why? !@#$
func DplFromExtractorLocations(locations []extractor.TextLocation) DocPageLocations {
	var dpl DocPageLocations
	for _, uloc := range locations {
		loc := fromExtractorLocation(uloc)
		dpl.locations = append(dpl.locations, loc)
	}
	return dpl
}

// fromExtractorLocation converts extractor.TextLocation `loc` to a more compact base.TextLocation.
func fromExtractorLocation(uloc extractor.TextLocation) TextLocation {
	b := uloc.BBox
	return TextLocation{
		Start: uint32(uloc.Offset),
		Llx:   float32(b.Llx),
		Lly:   float32(b.Lly),
		Urx:   float32(b.Urx),
		Ury:   float32(b.Ury),
	}
}

// GetPosition returns a base.TextLocation that bounds the text in `dpl.locations` with Offset
// `start` <= Offset <= `end`.
// Caller must check that dpl.locations is not empty.
func (dpl DocPageLocations) GetPosition(start, end uint32) TextLocation {
	i0, ok0 := dpl.getPositionIndex(end)
	i1, ok1 := dpl.getPositionIndex(start)
	if !(ok0 && ok1) {
		return TextLocation{}
	}
	p0, p1 := dpl.locations[i0], dpl.locations[i1]
	return TextLocation{
		Start: start,
		End:   end,
		Llx:   min(p0.Llx, p1.Llx),
		Lly:   min(p0.Lly, p1.Lly),
		Urx:   max(p0.Urx, p1.Urx),
		Ury:   max(p0.Ury, p1.Ury),
	}
}

// getPositionIndex returns a base.TextLocation that bounds the text in `dpl.locations` with Offset
// `offset`.
// Caller must check that positions is not empty.
func (dpl DocPageLocations) getPositionIndex(offset uint32) (int, bool) {
	positions := dpl.locations
	if len(positions) == 0 {
		common.Log.Error("getPositionIndex: No positions")
		panic("no positions")
	}
	i := sort.Search(len(positions), func(i int) bool { return positions[i].Start >= offset })
	ok := 0 <= i && i < len(positions)
	if !ok {
		common.Log.Error("getPositionIndex: offset=%d i=%d len=%d %v==%v", offset, i, len(positions),
			positions[0], positions[len(positions)-1])
	}
	return i, ok
}

func min(x, y float32) float32 {
	if x < y {
		return x
	}
	return y
}

func max(x, y float32) float32 {
	if x > y {
		return x
	}
	return y
}
