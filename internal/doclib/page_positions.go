// Copyright 2019 PaperCut Software International Pty Ltd. All rights reserved.

package doclib

import (
	"fmt"
	"math"
	"sort"

	"github.com/papercutsoftware/pdfsearch/internal/serial"
	"github.com/unidoc/unipdf/v3/common"
	"github.com/unidoc/unipdf/v3/extractor"
	"github.com/unidoc/unipdf/v3/model"
)

// PagePositions is used to link per-document data in a bleve index to the PDF file the data was
// extracted from.
// There is one PagePositions per PDF page.
// PagePositions stores the locations of text fragments on a page. PdfPageMatch includes a
// binary copy of PagePositions, so our goal is to make PagePositions compact.
type PagePositions struct {
	offsetBBoxes []serial.OffsetBBox
}

// String returns a string describing PagePositions `ppos`.
func (ppos PagePositions) String() string {
	return fmt.Sprintf("{PagePositions: %d}", len(ppos.offsetBBoxes))
}

// Empty return true if `ppos` has no entries.
func (ppos PagePositions) Empty() bool {
	return len(ppos.offsetBBoxes) == 0
}

// PagePositionsFromTextMarks converts extractor.TextMarkArray `textMarks` to a more compact
// PagePositions. We do this because PagePositions is stored in our index which we want to be small.
func PagePositionsFromTextMarks(textMarks *extractor.TextMarkArray) PagePositions {
	var ppos PagePositions
	for _, uloc := range textMarks.Elements() {
		loc := fromExtractorLocation(uloc)
		ppos.offsetBBoxes = append(ppos.offsetBBoxes, loc)
	}
	return ppos
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

// BBox returns a rectangle that bounds the text with offsets `start` and `end`.
// ofs: `start` <= ofs < `end` on the PDF page indexed by `ppos`.
// Caller must check that ppos.offsetBBoxes is not empty.
func (ppos PagePositions) BBox(start, end uint32) (model.PdfRectangle, bool) {
	i0, ok0 := ppos.positionIndex(start)
	i1, ok1 := ppos.positionIndex(end)
	if !(ok0 && ok1) {
		common.Log.Error("start=%d i0=%d ok0=%t end=%d i1=%d ok1=%t",
			start, i0, ok0, end, i1, ok1)
		return model.PdfRectangle{}, false
	}
	if i1 <= i0 {
		return model.PdfRectangle{}, false
	}
	bbox := ppos.offsetBBoxes[i0].BBox()
	for i := i0 + 1; i < i1; i++ {
		b := ppos.offsetBBoxes[i].BBox()
		// Ignore the filler entries. These are spaces etc.
		if b.Urx == 0 && b.Ury == 0 {
			continue
		}
		bbox = rectUnion(bbox, b)
	}
	if bbox.Height() > 200 || bbox.Width() > 200 {
		// TODO Remove panic
		for i := i0 + 1; i < i1; i++ {
			common.Log.Debug("i=%d bbox=%v", i, ppos.offsetBBoxes[i].BBox())
		}
		if bbox.Height() > 200 {
			panic(fmt.Errorf("bbox=%+v start=%d end=%d", bbox, start, end))
		}
	}
	return bbox, true
}

// positionIndex returns the index of the element of ppos.offsetBBoxes that spans `offset`
// (i.e  idx: ppos.offsetBBoxes[idx] <= offset < ppos.offsetBBoxes[idx+1])
// Caller must check that ppos.offsetBBoxes is not empty.
func (ppos PagePositions) positionIndex(offset uint32) (int, bool) {
	locations := ppos.offsetBBoxes
	n := len(locations)
	if n == 0 {
		common.Log.Debug("positionIndex: No locations")
		return 0, false
	}
	if !(locations[0].Offset <= offset && offset < locations[n-1].Offset) {
		common.Log.Debug("positionIndex: Out of range. offset=%d len=%d\n\tfirst=%v\n\t last=%v",
			offset, len(locations), locations[0], locations[n-1])
		return 0, false
	}
	i := sort.Search(len(locations), func(i int) bool { return locations[i].Offset >= offset })
	ok := 0 <= i && i < len(locations)
	if !ok {
		common.Log.Debug("positionIndex: Out of range. offset=%d i=%d len=%d\n\tfirst=%v\n\t last=%v",
			offset, i, len(locations), locations[0], locations[len(locations)-1])
	}
	return i, ok
}

// rectUnion returns the smallest axis-aligned rectangle that contains `b1` and `b2`.
func rectUnion(b1, b2 model.PdfRectangle) model.PdfRectangle {
	return model.PdfRectangle{
		Llx: math.Min(b1.Llx, b2.Llx),
		Lly: math.Min(b1.Lly, b2.Lly),
		Urx: math.Max(b1.Urx, b2.Urx),
		Ury: math.Max(b1.Ury, b2.Ury),
	}
}
