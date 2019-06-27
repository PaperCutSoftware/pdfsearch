// Copyright 2019 PaperCut Software International Pty Ltd. All rights reserved.

/*
 * Functions for searching a PdfIndex
 *  - BlevePdf.SearchBleveIndex()
 *  - SearchPersistentPdfIndex()
 */

package doclib

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/blevesearch/bleve"
	"github.com/blevesearch/bleve/search"
	"github.com/unidoc/unipdf/v3/common"
)

// PdfMatchSet is the result of a search over a PdfIndex.
type PdfMatchSet struct {
	TotalMatches   int           // Number of matches.
	SearchDuration time.Duration // The time it took to perform the search.
	Matches        []PdfMatch    // The matches.
}

// PdfMatch describes a single search match in a PDF document.
// It is the analog of a bleve search.DocumentMatch.
type PdfMatch struct {
	InPath        string // Path of the PDF file that was matched. (A name stored in the index.)
	PageNum       uint32 // 1-offset page number of the PDF page containing the matched text.
	LineNum       int    // 1-offset line number of the matched text within the extracted page text.
	Line          string // The contents of the line containing the matched text.
	PagePositions        // This is used to find the bounding box of the match text on the PDF page.
	bleveMatch           // Internal information !@#$
}

// bleveMatch is the match information returned by a bleve query.
type bleveMatch struct {
	docIdx   uint64  // Document index.
	pageIdx  uint32  // Page index.
	Score    float64 // bleve score.
	Fragment string
	Start    uint32 // Offset of the start of the bleve match in the page.
	End      uint32 // Offset of the end of the bleve match in the page.
}

// ErrNoMatch indicates there was no match for a bleve hit. It is not a real error.
var ErrNoMatch = errors.New("no match for hit")

// ErrNoMatch indicates there was no match for a bleve hit. It is not a real error.
var ErrNoPositions = errors.New("no match for hit")

// Equals returns true if `p` contains the same results as `q`.
func (p PdfMatchSet) Equals(q PdfMatchSet) bool {

	if len(p.Matches) != len(q.Matches) {
		common.Log.Error("PdfMatchSet.Equals.Matches: %d %d", len(p.Matches), len(q.Matches))
		return false
	}
	for i, m := range p.Matches {
		n := q.Matches[i]
		if !m.equals(n) {
			common.Log.Error("PdfMatchSet.Equals.Matches[%d]:\np=%s\nq=%s", i, m, n)
			return false
		}
	}
	return true
}

// equals returns true if `p` contains the same result as `q`.
func (p PdfMatch) equals(q PdfMatch) bool {
	if p.InPath != q.InPath {
		common.Log.Error("PdfMatch.Equals.InPath:\n%q\n%q", p.InPath, q.InPath)
		return false
	}
	if p.PageNum != q.PageNum {
		return false
	}
	if p.LineNum != q.LineNum {
		return false
	}
	if p.Line != q.Line {
		return false
	}

	return true
}

// SearchPositionIndex performs a bleve search on the persistent index in `persistDir`/bleve for
// `term` and returns up to `maxResults` matches. It maps the results to PDF page names, page
// numbers, line numbers and page locations using the BlevePdf that was saved in directory
// `persistDir`  by IndexPdfReaders().
func SearchPersistentPdfIndex(persistDir, term string, maxResults int) (PdfMatchSet, error) {
	p := PdfMatchSet{}

	indexPath := filepath.Join(persistDir, "bleve")

	common.Log.Debug("term=%q", term)
	common.Log.Debug("maxResults=%d", maxResults)
	common.Log.Debug("indexPath=%q", indexPath)

	// Open existing index.
	index, err := bleve.Open(indexPath)
	if err != nil {
		return p, fmt.Errorf("Could not open Bleve index %q", indexPath)
	}
	common.Log.Debug("index=%s", index)

	blevePdf, err := openBlevePdf(persistDir, false)
	if err != nil {
		return p, fmt.Errorf("Could not open positions store %q. err=%v", persistDir, err)
	}
	common.Log.Debug("blevePdf=%s", *blevePdf)

	results, err := blevePdf.SearchBleveIndex(index, term, maxResults)
	if err != nil {
		return p, fmt.Errorf("Could not find term=%q %q. err=%v", term, persistDir, err)
	}

	common.Log.Debug("=================@@@=====================")
	common.Log.Debug("term=%q", term)
	common.Log.Debug("indexPath=%q", indexPath)
	return results, nil
}

// SearchBleveIndex performs a bleve search on `index `for `term` and returns up to
// `maxResults` matches. It maps the results to PDF page names, page numbers, line
// numbers and page locations using `blevePdf`.
func (blevePdf *BlevePdf) SearchBleveIndex(index bleve.Index, term string, maxResults int) (
	PdfMatchSet, error) {

	p := PdfMatchSet{}

	common.Log.Debug("SearchBleveIndex: term=%q maxResults=%d", term, maxResults)

	if blevePdf.Len() == 0 {
		common.Log.Info("SearchBleveIndex: Empty positions store %s", blevePdf)
		return p, nil
	}

	query := bleve.NewMatchQuery(term)
	search := bleve.NewSearchRequest(query)
	search.Highlight = bleve.NewHighlight()
	search.Fields = []string{"Text"}
	search.Highlight.Fields = search.Fields
	search.Size = maxResults

	searchResults, err := index.Search(search)
	if err != nil {
		return p, err
	}

	common.Log.Debug("=================!!!=====================")
	common.Log.Debug("searchResults=%T", searchResults)

	if len(searchResults.Hits) == 0 {
		common.Log.Info("No matches")
		return p, nil
	}

	return blevePdf.srToMatchSet(searchResults)
}

// srToMatchSet maps bleve search results `sr` to PDF page names, page numbers, line
// numbers and page locations using the tables in `blevePdf`.
func (blevePdf *BlevePdf) srToMatchSet(sr *bleve.SearchResult) (PdfMatchSet, error) {
	var matches []PdfMatch
	if sr.Total > 0 && sr.Request.Size > 0 {
		for _, hit := range sr.Hits {
			m, err := blevePdf.hitToPdfMatch(hit)
			if err != nil {
				if err == ErrNoMatch {
					continue
				}
				return PdfMatchSet{}, err
			}
			matches = append(matches, m)
		}
	}

	common.Log.Info("srToMatchSet: matches=%d", len(matches))

	results := PdfMatchSet{
		TotalMatches:   int(sr.Total),
		SearchDuration: sr.Took,
		Matches:        matches,
	}
	return results, nil
}

// String returns a human readable description of `s`.
func (s PdfMatchSet) String() string {
	if s.TotalMatches <= 0 {
		return "No matches"
	}
	if len(s.Matches) == 0 {
		return fmt.Sprintf("%d matches, SearchDuration %s\n", s.TotalMatches, s.SearchDuration)
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "%d matches, showing %d, SearchDuration %s\n",
		s.TotalMatches, len(s.Matches), s.SearchDuration)
	for i, m := range s.Matches {
		fmt.Fprintln(&sb, "--------------------------------------------------")
		fmt.Fprintf(&sb, "%d: %s\n", i+1, m)
	}
	return sb.String()
}

// Files returns the unique file names in `s`.
func (s PdfMatchSet) Files() []string {
	fileSet := map[string]struct{}{}
	var files []string
	for _, m := range s.Matches {
		if _, ok := fileSet[m.InPath]; ok {
			continue
		}
		files = append(files, m.InPath)
		fileSet[m.InPath] = struct{}{}
	}
	return files
}

// String returns a human readable description of PdfMatch `p`.
func (p PdfMatch) String() string {
	return fmt.Sprintf("{PdfMatch: path=%q pageNum=%d line=%d (score=%.3f)\nmatch=%q\n"+
		"^^^^^^^^ Marked up Text ^^^^^^^^\n"+
		"%s\n}",
		p.InPath, p.PageNum, p.LineNum, p.Score, p.Line, p.Fragment)
}

// hitToPdfMatch returns the PdfMatch corresponding the bleve DocumentMatch `hit`.
// The returned PdfMatch contains information that is not in `hit` that is looked up in `blevePdf`.
// We purposely try to keep `hit` small to improve bleve indexing performance and to reduce the
// bleve index size.
func (blevePdf *BlevePdf) hitToPdfMatch(hit *search.DocumentMatch) (PdfMatch, error) {
	m, err := hitToBleveMatch(hit)
	if err != nil {
		return PdfMatch{}, err
	}
	inPath, pageNum, dpl, err := blevePdf.docPagePositions(m.docIdx, m.pageIdx)
	if err != nil {
		return PdfMatch{}, err
	}
	text, err := blevePdf.docPageText(m.docIdx, m.pageIdx)
	if err != nil {
		return PdfMatch{}, err
	}
	lineNum, line, ok := lineNumber(text, m.Start)
	if !ok {
		return PdfMatch{}, fmt.Errorf("No line number. m=%s", m)
	}

	return PdfMatch{
		InPath:        inPath,
		PageNum:       pageNum,
		LineNum:       lineNum,
		Line:          line,
		PagePositions: dpl,
		bleveMatch:    m,
	}, nil
}

func (m bleveMatch) String() string {
	return fmt.Sprintf("docIdx=%d pageIdx=%d (score=%.3f)\n%s",
		m.docIdx, m.pageIdx, m.Score, m.Fragment)
}

// hitToBleveMatch returns a bleveMatch filled with the information in `hit` which comes from bleve.
func hitToBleveMatch(hit *search.DocumentMatch) (bleveMatch, error) {

	docIdx, pageIdx, err := decodeID(hit.ID)
	if err != nil {
		return bleveMatch{}, err
	}

	start, end := -1, -1
	var frags strings.Builder
	common.Log.Debug("------------------------")
	// !@#$ How many fragments are there?
	for k, fragments := range hit.Fragments {
		for _, fragment := range fragments {
			frags.WriteString(fragment)
		}
		loc := hit.Locations[k]
		common.Log.Info("%q: %d %v", k, len(loc), frags)
		for _, v := range loc {
			for _, l := range v {
				if start < 0 {
					start = int(l.Start)
					end = int(l.End)
				}
			}
		}
	}
	if start < 0 {
		// !@#$ Do we need to return an error?
		common.Log.Error("Fragments=%d", len(hit.Fragments))
		for k := range hit.Fragments {
			loc := hit.Locations[k]
			common.Log.Error("%q: %v", k, frags)
			for kk, v := range loc {
				for i, l := range v {
					common.Log.Error("\t%q: %d: %#v", kk, i, l)
				}
			}
		}
		err := ErrNoMatch
		common.Log.Error("hit=%s err=%v", hit, err)
		return bleveMatch{}, err
	}
	return bleveMatch{
		docIdx:   docIdx,
		pageIdx:  pageIdx,
		Score:    hit.Score,
		Fragment: frags.String(),
		Start:    uint32(start),
		End:      uint32(end),
	}, nil
}

// decodeID decodes the ID string passed to bleve in indexDocPagesLocReader().
// id := fmt.Sprintf("%04X.%d", l.DocIdx, l.PageIdx)
func decodeID(id string) (uint64, uint32, error) {
	parts := strings.Split(id, ".")
	if len(parts) != 2 {
		return 0, 0, errors.New("bad format")
	}
	docIdx, err := strconv.ParseUint(parts[0], 16, 64)
	if err != nil {
		return 0, 0, err
	}
	pageIdx, err := strconv.ParseUint(parts[1], 10, 32)
	if err != nil {
		return 0, 0, err
	}
	return uint64(docIdx), uint32(pageIdx), nil
}

// lineNumber returns the 1-offset line number and the text of the line of the contains
// the 0-offset `offset` in `text`.
func lineNumber(text string, offset uint32) (int, string, bool) {
	endings := lineEndings(text)
	n := len(endings)
	i := sort.Search(len(endings), func(i int) bool { return endings[i] > offset })
	ok := 0 <= i && i < n
	if !ok {
		common.Log.Error("lineNumber: offset=%d text=%d i=%d endings=%d %+v\n%s",
			offset, len(text), i, n, endings, text)
		panic("fff")
	}
	common.Log.Debug("offset=%d i=%d endings=%+v", offset, i, endings)
	ofs0 := endings[i-1]
	ofs1 := endings[i+0]
	line := text[ofs0:ofs1]
	runes := []rune(line)
	if len(runes) >= 1 && runes[0] == '\n' {
		line = string(runes[1:])
	}
	return i + 1, line, ok
}

// lineEndings returns the offsets of all the line endings in `text`.
func lineEndings(text string) []uint32 {
	if len(text) == 0 || (len(text) > 0 && text[len(text)-1] != '\n') {
		text += "\n"
	}
	endings := []uint32{0}
	for ofs := 0; ofs < len(text); {
		o := strings.Index(text[ofs:], "\n")
		if o < 0 {
			break
		}
		endings = append(endings, uint32(ofs+o))
		ofs = ofs + o + 1
	}

	return endings
}
