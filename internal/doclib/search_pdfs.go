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
	"github.com/blevesearch/bleve/search/query"
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
	InPath        string   // Path of the PDF file that was matched. (A name stored in the index.)
	PageNum       uint32   // 1-offset page number of the PDF page containing the matched text.
	LineNums      []int    // 1-offset line number of the matched text within the extracted page text.
	Lines         []string // The contents of the line containing the matched text.
	PagePositions          // This is used to find the bounding box of the match text on the PDF page.
	bleveMatch             // Internal information !@#$
}

// bleveMatch is the match information returned by a bleve query.
type bleveMatch struct {
	docIdx   uint64  // Document index.
	pageIdx  uint32  // Page index.
	Score    float64 // bleve score.
	Fragment string  // bleve's marked up string
	// Start    uint32  // Offset of the start of the bleve match in the page.
	// End      uint32  // Offset of the end of the bleve match in the page.
	Spans []Span
}

type Span struct {
	Start uint32 // Offset of the start of the bleve match in the page.
	End   uint32 // Offset of the end of the bleve match in the page.
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
	// if p.LineNum != q.LineNum {
	// 	return false
	// }
	// if p.Line != q.Line {
	// 	return false
	// }

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
	common.Log.Info("SearchBleveIndex: term=%q maxResults=%d", term, maxResults)

	if blevePdf.Len() == 0 {
		common.Log.Info("SearchBleveIndex: Empty positions store %s", blevePdf)
		return p, nil
	}

	query0 := bleve.NewMatchQuery(term)
	query0.SetOperator(query.MatchQueryOperatorAnd)
	query0.SetBoost(10.0)
	// query0.Fuzziness = 1
	// query0.Analyzer = "en"
	query1 := bleve.NewMatchQuery(term)
	query1.SetOperator(query.MatchQueryOperatorOr)
	// query1.Analyzer = "en"
	// query1.Fuzziness = 1
	queryX := bleve.NewDisjunctionQuery(query0, query1)
	// queryX := query1
	search := bleve.NewSearchRequest(queryX)
	search.Highlight = bleve.NewHighlight()
	search.Fields = []string{"Text"}
	search.Highlight.Fields = search.Fields
	search.Size = maxResults

	searchResults, err := index.Search(search)
	if err != nil {
		return p, err
	}

	common.Log.Info("=================!!!=====================")
	common.Log.Info("search.Size=%d", search.Size)
	common.Log.Info("searchResults=%T", searchResults)

	if len(searchResults.Hits) == 0 {
		common.Log.Info("No matches")
		common.Log.Info("searchResults=%+v", searchResults)
		return p, nil
	}

	common.Log.Info("%d Hits", len(searchResults.Hits))
	for i, hit := range searchResults.Hits {
		common.Log.Info("%3d: %4.2f %3d %q", i, hit.Score, hit.Size(), hit.String())
	}

	return blevePdf.srToMatchSet(term, searchResults)
}

// truncate truncates `text` to its first `n` characters.
func truncate(text string, n int) string {
	if len(text) <= n {
		return text
	}
	return text[:n]
}

// srToMatchSet maps bleve search results `sr` to PDF page names, page numbers, line
// numbers and page locations using the tables in `blevePdf`.
func (blevePdf *BlevePdf) srToMatchSet(fullTerm string, sr *bleve.SearchResult) (PdfMatchSet, error) {
	var matches []PdfMatch
	if sr.Total > 0 && sr.Request.Size > 0 {
		for _, hit := range sr.Hits {
			m, err := blevePdf.hitToPdfMatch(fullTerm, hit)
			if err != nil {
				if err == ErrNoMatch {
					continue
				}
				return PdfMatchSet{}, err
			}
			matches = append(matches, m)
		}
	}

	common.Log.Info("srToMatchSet: hits=%d matches=%d", len(sr.Hits), len(matches))

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

// Files returns the PDF file names names in PdfMatchSet `s`. These are all the PDF that contained
// at least one match of the search term.
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
	return fmt.Sprintf("{PdfMatch: path=%q pageNum=%d lines=%v (score=%.3f)\nmatch=%q\n"+
		"^^^^^^^^ Marked up Text ^^^^^^^^\n"+
		"%s\n}",
		p.InPath, p.PageNum, p.LineNums, p.Score, p.Lines, p.Fragment)
}

// hitToPdfMatch returns the PdfMatch corresponding the bleve DocumentMatch `hit`.
// The returned PdfMatch also contains information that is not in `hit` that is looked up in `blevePdf`.
// We purposely try to keep `hit` small to improve bleve indexing speed and to reduce the
// bleve index size.
func (blevePdf *BlevePdf) hitToPdfMatch(fullTerm string, hit *search.DocumentMatch) (PdfMatch, error) {
	m, err := hitToBleveMatch(fullTerm, hit)
	if err != nil {
		return PdfMatch{}, err
	}
	inPath, pageNum, ppos, err := blevePdf.docPagePositions(m.docIdx, m.pageIdx)
	if err != nil {
		return PdfMatch{}, err
	}
	text, err := blevePdf.docPageText(m.docIdx, m.pageIdx)
	if err != nil {
		return PdfMatch{}, err
	}
	var lineNums []int
	var lines []string
	for _, span := range m.Spans {
		lineNum, line, ok := lineNumber(text, span.Start)
		if !ok {
			return PdfMatch{}, fmt.Errorf("No line number. m=%s span=%v", m, span)
		}
		lineNums = append(lineNums, lineNum)
		lines = append(lines, line)
	}

	return PdfMatch{
		InPath:        inPath,
		PageNum:       pageNum,
		LineNums:      lineNums,
		Lines:         lines,
		PagePositions: ppos,
		bleveMatch:    m,
	}, nil
}

// String() returns a string describing `m`.
func (m bleveMatch) String() string {
	return fmt.Sprintf("docIdx=%d pageIdx=%d (score=%.3f)\n%s",
		m.docIdx, m.pageIdx, m.Score, m.Fragment)
}

type Phrase struct {
	score     int
	terms     []string
	locations []search.Location
	start     int
	end       int
}

func bestPhrases(fullTerm string, termLocMap search.TermLocationMap) []Phrase {
	var terms []string
	for term := range termLocMap {
		terms = append(terms, term)
	}
	sort.Slice(terms, func(i, j int) bool {
		ti, tj := terms[i], terms[j]
		pi := strings.Index(fullTerm, ti)
		if pi < 0 {
			panic(ti)
		}
		pj := strings.Index(fullTerm, tj)
		if pj < 0 {
			panic(tj)
		}
		smaller := ti > tj
		common.Log.Info("i=%d ti=%q pi=%d -- j=%d tj=%q pj=%d -- %t",
			i, ti, pi, j, tj, pj, smaller)
		return smaller
	})
	common.Log.Info("bestPhrases: terms=%d %q", len(terms), terms)
	termPositions := map[string]map[int]struct{}{}
	startMap := map[int]struct{}{}
	posLoc := map[int]search.Location{}

	for i, term := range terms {
		locs := termLocMap[term]
		termPositions[term] = map[int]struct{}{}
		for _, loc := range locs {
			pos := int(loc.Pos)
			posLoc[pos] = *loc

			termPositions[term][pos] = struct{}{}
			startPos := pos - i
			if startPos < 0 {
				panic("not possible")
			}
			startMap[startPos] = struct{}{}
			common.Log.Info("term=%9q pos=%3d startPos=%3d posLoc=%d startMap=%d",
				term, pos, startPos, len(posLoc), len(startMap))
		}
	}

	var starts []int
	for v := range startMap {
		starts = append(starts, v)
	}
	sort.Ints(starts)
	common.Log.Info("starts=%d %v", len(starts), starts)

	var positions []int
	for v := range posLoc {
		positions = append(positions, v)
	}
	sort.Ints(positions)
	common.Log.Info("positions=%d %v", len(positions), positions)

	var phrases []Phrase
	for _, pos0 := range starts {
		common.Log.Info("pos0=%d ---------------", pos0)
		var phrase Phrase
		for k, term := range terms {
			pos := pos0 + k
			loc := posLoc[pos]
			_, ok := termPositions[term][pos]

			if ok {
				phrase.terms = append(phrase.terms, term)
				phrase.locations = append(phrase.locations, loc)
				phrase.score += 1
			}
			common.Log.Info(" k=%d pos=%d ok=%5t term=%q phrase=%v", k, pos, ok, term, phrase)
		}
		if len(phrase.terms) > 0 {
			phrase.start = int(phrase.locations[0].Start)
			phrase.end = int(phrase.locations[len(phrase.terms)-1].End)
			phrases = append(phrases, phrase)
		}
	}
	common.Log.Info("-------------+++------------- %d phrases", len(phrases))
	for i, phrase := range phrases {
		common.Log.Info("%4d: %v", i, phrase)
	}

	bestScore := 0
	for _, phrase := range phrases {
		if phrase.score > bestScore {
			bestScore = phrase.score
		}
	}
	var best []Phrase
	for _, phrase := range phrases {
		if phrase.score >= bestScore {
			best = append(best, phrase)
		}
	}
	phrases = best
	common.Log.Info("-------------&&&------------- %d phrases", len(phrases))
	for i, phrase := range phrases {
		common.Log.Info("%4d: %v", i, phrase)
	}
	return phrases
}

// hitToBleveMatch returns a bleveMatch filled with the information in `hit` that comes from bleve.
func hitToBleveMatch(fullTerm string, hit *search.DocumentMatch) (bleveMatch, error) {
	docIdx, pageIdx, err := decodeID(hit.ID)
	if err != nil {
		return bleveMatch{}, err
	}

	// start, end := -1, -1
	var frags strings.Builder
	var phrases []Phrase
	common.Log.Info("----------xxx------------ %d Fragments", len(hit.Fragments))
	// !@#$ How many fragments are there?
	if len(hit.Fragments) > 1 {
		panic("what?")
	}
	for k, fragments := range hit.Fragments {
		for _, fragment := range fragments {
			frags.WriteString(fragment)
		}
		loc := hit.Locations[k]
		common.Log.Info("%q: %d %q", k, len(loc), frags.String())
		// outer:
		// for i, v := range loc {
		// 	common.Log.Info("%q: v=%T=%+v", i, v, v)
		// 	for j, l := range v {
		// 		common.Log.Info("%8d: l=%+v", j, l)
		// 		if l.Start >= 0 {
		// 			start = int(l.Start)
		// 			end = int(l.End)
		// 			// break outer
		// 		}
		// 	}
		// }
		phrases = bestPhrases(fullTerm, hit.Locations[k])
		// panic("done")
	}
	// if start < 0 {
	// 	// !@#$ Do we need to return an error?
	// 	common.Log.Error("Fragments=%d", len(hit.Fragments))
	// 	for k := range hit.Fragments {
	// 		loc := hit.Locations[k]
	// 		common.Log.Error("%q: %v", k, frags)
	// 		for kk, v := range loc {
	// 			for i, l := range v {
	// 				common.Log.Error("\t%q: %d: %#v", kk, i, l)
	// 			}
	// 		}
	// 	}
	// 	err := ErrNoMatch
	// 	common.Log.Error("hit=%s err=%v", hit, err)
	// 	return bleveMatch{}, err
	// }

	var spans []Span
	for _, p := range phrases {
		spn := Span{Start: uint32(p.start), End: uint32(p.end)}
		spans = append(spans, spn)
	}
	return bleveMatch{
		docIdx:   docIdx,
		pageIdx:  pageIdx,
		Score:    hit.Score,
		Fragment: frags.String(),
		Spans:    spans,
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
