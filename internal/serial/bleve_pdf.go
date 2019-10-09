// Copyright 2019 PaperCut Software International Pty Ltd. All rights reserved.

package serial

import (
	"errors"
	"fmt"

	flatbuffers "github.com/google/flatbuffers/go"
	"github.com/papercutsoftware/pdfsearch/internal/serial/locations"
	"github.com/papercutsoftware/pdfsearch/internal/serial/pdf_index"
	"github.com/unidoc/unipdf/v3/common"
)

// SerialBlevePdf is for serializing and deserializing doclib.BlevePdf.
// It corresponds to the following flatbuffers schema.
// table PdfIndex  {
// 	num_files:   uint32;
// 	num_pages:   uint32;
// 	index :     [byte];
// 	hipd:       [HashIndexPathDoc];
// }
type SerialBlevePdf struct {
	NumFiles uint32
	NumPages uint32
	HIPDs    []HashIndexPathDoc
}

// WriteSerialBlevePdf converts `spi` into a byte array.
func WriteSerialBlevePdf(spi SerialBlevePdf) []byte {
	b := flatbuffers.NewBuilder(0)
	buf := MakeSerialBlevePdf(b, spi)
	return buf
}

// MakeSerialBlevePdf returns a flatbuffers serialized byte array for `spi`.
func MakeSerialBlevePdf(b *flatbuffers.Builder, spi SerialBlevePdf) []byte {
	b.Reset()

	var locOffsets []flatbuffers.UOffsetT
	for _, hipd := range spi.HIPDs {
		locOfs := addHashIndexPathDoc(b, hipd)
		locOffsets = append(locOffsets, locOfs)
	}
	pdf_index.PdfIndexStartHipdVector(b, len(spi.HIPDs))
	// Prepend TextLocations in reverse order.
	for i := len(locOffsets) - 1; i >= 0; i-- {
		b.PrependUOffsetT(locOffsets[i])
	}
	locationsOfs := b.EndVector(len(spi.HIPDs))

	// Write the SerialBlevePdf object.
	pdf_index.PdfIndexStart(b)
	pdf_index.PdfIndexAddNumFiles(b, spi.NumFiles)
	pdf_index.PdfIndexAddNumPages(b, spi.NumPages)
	pdf_index.PdfIndexAddHipd(b, locationsOfs)
	dplOfs := pdf_index.PdfIndexEnd(b)

	// Finish the write operations by our SerialBlevePdf the root object.
	b.Finish(dplOfs)

	// return the byte slice containing encoded data:
	return b.Bytes[b.Head():]
}

// ReadSerialBlevePdf converts byte array `b` into a SerialBlevePdf.
// Write round trip tests. !@#$
func ReadSerialBlevePdf(buf []byte) (SerialBlevePdf, error) {
	// Initialize a SerialBlevePdf reader from `buf`.
	spi := pdf_index.GetRootAsPdfIndex(buf, 0)

	// Vectors, such as `Hipd`, have a method suffixed with 'Length' that can be used
	// to query the length of the vector. You can index the vector by passing an index value
	// into the accessor.
	var hipds []HashIndexPathDoc
	common.Log.Trace("ReadSerialBlevePdf: spi.HipdLength=%d", spi.HipdLength())
	for i := 0; i < spi.HipdLength(); i++ {
		var loc pdf_index.HashIndexPathDoc
		ok := spi.Hipd(&loc, i)
		if !ok {
			return SerialBlevePdf{}, errors.New("bad HashIndexPathDoc")
		}
		h, err := getHashIndexPathDoc(&loc)
		if err != nil {
			return SerialBlevePdf{}, err
		}
		hipds = append(hipds, h)
	}

	common.Log.Trace("ReadSerialBlevePdf: NumFiles=%d NumPages=%d HIPDs=%d",
		spi.NumFiles(), spi.NumPages(), len(hipds))
	for i := 0; i < len(hipds) && i < 2; i++ {
		common.Log.Trace("ReadSerialBlevePdf: hipds[%d]=%v", i, hipds[i])
	}

	return SerialBlevePdf{
		NumFiles: spi.NumFiles(),
		NumPages: spi.NumPages(),
		// Index    []byte
		HIPDs: hipds,
	}, nil
}

// HashIndexPathDoc is used for serializing a doclib.BlevePdf. They key+values of the maps in
// the BlevePdf are stored in []HashIndexPathDoc.
// table HashIndexPathDoc {
// 	hash: string;
// 	index: uint64;
// 	path: string;
// 	doc: DocPositions;
// }
type HashIndexPathDoc struct {
	Hash  string
	Index uint64
	Path  string
	Doc   DocPositions
}

// addHashIndexPathDoc writes HashIndexPathDoc `hipd` to builder `b`.
func addHashIndexPathDoc(b *flatbuffers.Builder, hipd HashIndexPathDoc) flatbuffers.UOffsetT {
	// hipd.Doc.Check()
	hash := b.CreateString(hipd.Hash)
	path := b.CreateString(hipd.Path)
	doc := addDocPositions(b, hipd.Doc)

	// Write the HashIndexPathDoc object.
	pdf_index.HashIndexPathDocStart(b)
	pdf_index.HashIndexPathDocAddHash(b, hash)
	pdf_index.HashIndexPathDocAddIndex(b, hipd.Index)
	pdf_index.HashIndexPathDocAddPath(b, path)
	pdf_index.HashIndexPathDocAddDoc(b, doc)
	return pdf_index.HashIndexPathDocEnd(b)
}

// getHashIndexPathDoc reads a HashIndexPathDoc. !@#$
func getHashIndexPathDoc(loc *pdf_index.HashIndexPathDoc) (HashIndexPathDoc, error) {
	// Copy the HashIndexPathDoc's fields (since these are numbers).
	var pos pdf_index.DocPositions
	sdoc := loc.Doc(&pos)

	numPageNums := sdoc.PageNumsLength()
	numPageTexts := sdoc.PageTextsLength()
	common.Log.Trace("numPageNums=%d numPageTexts=%d", numPageNums, numPageTexts)

	var pageNums []uint32
	for i := 0; i < sdoc.PageNumsLength(); i++ {
		pageNum := sdoc.PageNums(i)
		pageNums = append(pageNums, pageNum)
	}

	var pageTexts []string
	for i := 0; i < sdoc.PageTextsLength(); i++ {
		text := string(sdoc.PageTexts(i))
		common.Log.Trace("getHashIndexPathDoc: pageTexts[%d]=%d %q", i, len(text), truncate(text, 100))
		pageTexts = append(pageTexts, text)
	}

	var pagePositions [][]OffsetBBox
	for i := 0; i < sdoc.PageDplLength(); i++ {
		var sdpl locations.PagePositions
		ok := sdoc.PageDpl(&sdpl, i)
		if !ok {
			common.Log.Error("getHashIndexPathDoc: No PageDpl(%d)", i)
			return HashIndexPathDoc{}, errors.New("no PageDpl")
		}
		ppos, err := getDocPageLocations(&sdpl)
		if err != nil {
			return HashIndexPathDoc{}, err
		}
		pagePositions = append(pagePositions, ppos)
	}

	doc := DocPositions{
		Path:          string(sdoc.Path()),
		DocIdx:        sdoc.DocIdx(),
		PageNums:      pageNums,
		PageTexts:     pageTexts,
		PagePositions: pagePositions,
	}

	hipd := HashIndexPathDoc{
		Hash:  string(loc.Hash()),
		Path:  string(loc.Path()),
		Index: loc.Index(),
		Doc:   doc,
	}

	return hipd, nil
}

// DocPositions is used to serialize a doclib.DocPositions.
// table DocPositions {
// 	path:  string;
// 	doc_idx:  uint64;
// 	page_dpl: [locations.PagePositions];
// 	page_nums:  [uint32];
// 	page_texts: [string];
// }
type DocPositions struct {
	Path          string         // Path of input PDF file.
	DocIdx        uint64         // Index into blevePdf.fileList.
	PagePositions [][]OffsetBBox // PagePositions[i] = doc.pagePositions[doc.pageNums[i]].offsetBBoxes
	PageNums      []uint32       // 1-offset page numbers of entries.
	PageTexts     []string       // Extracted page text of entries.
}

// String returns a text description of `doc`.
func (doc DocPositions) String() string {
	return fmt.Sprintf("{DocPositions: DocIdx=%d PageNums=%d PageTexts=%d %q}",
		doc.DocIdx, len(doc.PageNums), len(doc.PageTexts), doc.Path)
}

// MakeDocPositions returns a flatbuffers serialized byte array for `doc`.
func MakeDocPositions(b *flatbuffers.Builder, doc DocPositions) []byte {
	common.Log.Info("MakeDocPositions: doc=%s", doc)
	b.Reset()

	dplOfs := addDocPositions(b, doc)

	// Finish the write operations by our SerialBlevePdf the root object.
	b.Finish(dplOfs)

	// return the byte slice containing encoded data:
	return b.Bytes[b.Head():]
}

// addDocPositions writes `doc` to builder `b` and returns the root-table offset.
func addDocPositions(b *flatbuffers.Builder, doc DocPositions) flatbuffers.UOffsetT {
	path := b.CreateString(doc.Path)

	var pageOffsets []flatbuffers.UOffsetT
	for _, pageNum := range doc.PageNums {
		b.StartObject(1)
		b.PrependUint32Slot(0, pageNum, 0)
		locOfs := b.EndObject()
		pageOffsets = append(pageOffsets, locOfs)
	}

	pdf_index.DocPositionsStartPageNumsVector(b, len(doc.PageNums))
	// Prepend PageNums in reverse order.
	for i := len(doc.PageNums) - 1; i >= 0; i-- {
		b.PrependUint32(doc.PageNums[i])
	}
	pageOfs := b.EndVector(len(doc.PageNums))

	var textOffsets []flatbuffers.UOffsetT
	for _, text := range doc.PageTexts {
		textOfs := b.CreateString(text)
		textOffsets = append(textOffsets, textOfs)
	}
	pdf_index.DocPositionsStartPageTextsVector(b, len(doc.PageTexts))
	// Prepend TextLocations in reverse order.
	for i := len(textOffsets) - 1; i >= 0; i-- {
		b.PrependUOffsetT(textOffsets[i])
	}
	textOfs := b.EndVector(len(doc.PageTexts))

	var dplOffsets []flatbuffers.UOffsetT
	for i, ppos := range doc.PagePositions {
		common.Log.Trace("addDocPositions: PagePositions[%d]=%d", i, len(ppos))
		dplOfs := addDocPageLocations(b, ppos)
		dplOffsets = append(dplOffsets, dplOfs)
	}
	pdf_index.DocPositionsStartPageDplVector(b, len(doc.PagePositions))
	// Prepend TextLocations in reverse order.
	for i := len(dplOffsets) - 1; i >= 0; i-- {
		b.PrependUOffsetT(dplOffsets[i])
	}
	dplOfs := b.EndVector(len(doc.PagePositions))

	// Write the SerialBlevePdf object.
	pdf_index.DocPositionsStart(b)

	pdf_index.DocPositionsAddPath(b, path)
	pdf_index.DocPositionsAddDocIdx(b, doc.DocIdx)
	pdf_index.DocPositionsAddPageNums(b, pageOfs)
	pdf_index.DocPositionsAddPageTexts(b, textOfs)
	pdf_index.DocPositionsAddPageDpl(b, dplOfs)
	return pdf_index.DocPositionsEnd(b)
}

func ReadDocPositions(buf []byte) (DocPositions, error) {
	// Initialize a SerialBlevePdf reader from `buf`.
	sdoc := pdf_index.GetRootAsDocPositions(buf, 0)
	return getDocPositions(sdoc)
}

func getDocPositions(sdoc *pdf_index.DocPositions) (DocPositions, error) {
	// Vectors, such as `PageNums`, have a method suffixed with 'Length' that can be used
	// to query the length of the vector. You can index the vector by passing an index value
	// into the accessor.
	var pageNums []uint32
	for i := 0; i < sdoc.PageNumsLength(); i++ {
		pageNum := sdoc.PageNums(i)
		pageNums = append(pageNums, pageNum)
	}

	var pagePositions [][]OffsetBBox
	for i := 0; i < sdoc.PageDplLength(); i++ {
		var sdpl locations.PagePositions
		if !sdoc.PageDpl(&sdpl, i) {
			common.Log.Error("PageDpl(%d) does not exist", i)
			return DocPositions{}, errors.New("no PageDpl entry")
		}
		ppos, err := getDocPageLocations(&sdpl)
		if err != nil {
			return DocPositions{}, err
		}
		pagePositions = append(pagePositions, ppos)
	}

	var pageTexts []string
	for i := 0; i < sdoc.PageTextsLength(); i++ {
		text := string(sdoc.PageTexts(i))
		pageTexts = append(pageTexts, text)
	}

	doc := DocPositions{
		Path:          string(sdoc.Path()),
		DocIdx:        sdoc.DocIdx(),
		PageNums:      pageNums,
		PageTexts:     pageTexts,
		PagePositions: pagePositions,
	}

	return doc, nil
}

func truncate(text string, n int) string {
	if len(text) <= n {
		return text
	}
	return text[:n]
}
