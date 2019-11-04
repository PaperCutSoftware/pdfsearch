// Copyright 2019 PaperCut Software International Pty Ltd. All rights reserved.

/*
 * Functions for searching a PdfIndex
 *  - BlevePdf.SearchBleveIndex()
 *  - SearchPdfIndex()
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
	"github.com/blevesearch/bleve/analysis"
	"github.com/blevesearch/bleve/analysis/lang/en"
	"github.com/blevesearch/bleve/registry"
	"github.com/blevesearch/bleve/search"
	"github.com/unidoc/unipdf/v3/common"
)

// PdfMatchSet is the result of a search over a PdfIndex.
type PdfMatchSet struct {
	TotalMatches   int            // Total number of matches.
	SearchDuration time.Duration  // The time it took to perform the search.
	Matches        []PdfPageMatch // The per-page matches which may come from different PDFs.
}

// PdfPageMatch describes the search results for a PDF page returned from a search over a PDF index.
// It is the analog of a bleve search.DocumentMatch.
type PdfPageMatch struct {
	InPath        string   // Path of the PDF file that was matched. (A name stored in the index.)
	PageNum       uint32   // 1-offset page number of the PDF page containing the matched text.
	LineNums      []int    // 1-offset line number of the matched text within the extracted page text.
	Lines         []string // The contents of the line containing the matched text.
	PagePositions          // This is used to find the bounding box of the match text on the PDF page.
	bleveMatch             // Internal information on the match returned from the bleve query.
}

// bleveMatch is the match information returned by a bleve query.
type bleveMatch struct {
	docIdx   uint64  // Document index.
	pageIdx  uint32  // Page index.
	Score    float64 // bleve score.
	Fragment string  // bleve's marked up string. Useful for debugging. TODO. Remove from production code?
	Spans    []Span
}

// Span gives the offsets in extracted text that span a phrase.
type Span struct {
	Start uint32  // Offset of the start of the bleve match in the page.
	End   uint32  // Offset of the end of the bleve match in the page.
	Score float64 // Score for this match
}

// Best return a copy of `s` trimmed to the results with the highest score.
func (s PdfMatchSet) Best() PdfMatchSet {
	best := PdfMatchSet{
		SearchDuration: s.SearchDuration,
	}
	bestScore := 0.0
	for _, m := range s.Matches {
		for _, s := range m.Spans {
			if s.Score >= bestScore {
				bestScore = s.Score
			}
		}
	}
	numMatches := 0
	numBest := 0
	for _, m := range s.Matches {
		var lineNums []int
		var lines []string
		var spans []Span
		for i, a := range m.Spans {
			numMatches++
			if a.Score >= bestScore {
				lineNums = append(lineNums, m.LineNums[i])
				lines = append(lines, m.Lines[i])
				spans = append(spans, a)
			}
		}
		if len(spans) > 0 {
			o := m
			o.LineNums = lineNums
			o.Lines = lines
			o.Spans = spans
			best.Matches = append(best.Matches, o)
			best.TotalMatches += len(spans)
			numBest++
		}
	}
	common.Log.Debug("PdfMatchSet.Best: bestScore=%g numMatches=%d numBest=%d",
		bestScore, numMatches, numBest)
	return best
}

// ErrNoMatch indicates there was no match for a bleve hit. It is not a real error.
var ErrNoMatch = errors.New("no match for hit")

// ErrNoMatch indicates there was no match for a bleve hit. It is not a real error.
var ErrNoPositions = errors.New("no match for hit")

// SearchPdfIndex performs a bleve search on the persistent index in `persistDir/bleve`
// for `term` and returns up to `maxResults` matches. It maps the results to PDF file names, page
// numbers, line numbers and page locations using the BlevePdf that was saved in directory
// `persistDir` by IndexPdfFiles().
func SearchPdfIndex(persistDir, term string, maxResults int) (PdfMatchSet, error) {
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
func (blevePdf *BlevePdf) SearchBleveIndex(index bleve.Index, term0 string, maxResults int) (
	PdfMatchSet, error) {
	p := PdfMatchSet{}
	common.Log.Debug("SearchBleveIndex: term0=%q maxResults=%d", term0, maxResults)

	if blevePdf.Len() == 0 {
		common.Log.Info("SearchBleveIndex: Empty positions store %s", blevePdf)
		return p, nil
	}

	// TODO precompute analyzer?
	// TODO: Are tokens needed? Is there a better way of computing spans/.
	cache := registry.NewCache()
	analyzer, err := cache.AnalyzerNamed(en.AnalyzerName)
	if err != nil {
		return p, nil
	}
	tokens := analyzer.Analyze([]byte(term0))
	common.Log.Debug("term0=%q", term0)
	common.Log.Debug("tokens=%d", len(tokens))
	for i, t := range tokens {
		common.Log.Debug("%4d: %v", i, t)
	}
	term := term0

	// query0 := bleve.NewMatchQuery(term)
	// query0.SetOperator(query.MatchQueryOperatorAnd)
	// query0.SetBoost(10.0)
	// // query0.Fuzziness = 1
	// query0.Analyzer = "en"
	query1 := bleve.NewMatchQuery(term)
	// query1.SetOperator(query.MatchQueryOperatorOr)
	// query1.Analyzer = "en"
	// query1.Fuzziness = 1
	// queryX := bleve.NewDisjunctionQuery(query0, query1)
	queryX := query1
	search := bleve.NewSearchRequest(queryX)
	search.Highlight = bleve.NewHighlight()
	search.Fields = []string{"Text"}
	search.Highlight.Fields = search.Fields
	search.Size = maxResults
	// search.Explain = true

	searchResults, err := index.Search(search)
	if err != nil {
		return p, err
	}

	common.Log.Debug("=================!!!=====================")
	common.Log.Debug("search.Size=%d", search.Size)
	common.Log.Debug("searchResults=%T", searchResults)

	if len(searchResults.Hits) == 0 {
		common.Log.Debug("No matches")
		common.Log.Debug("searchResults=%+v", searchResults)
		return p, nil
	}

	for _, hit := range searchResults.Hits {
		// common.Log.Debug("#######################################")
		// common.Log.Debug("hit %d=%v", i, hit)
		// common.Log.Debug("Index=%q ID=%q Score=%.3f Fragments=%q Sort=%s",
		// 	hit.Index,
		// 	hit.ID,
		// 	hit.Score,
		// 	hit.Fragments["Text"],
		// 	hit.Sort)
		// // common.Log.Debug("Explanation: %s", hit.Expl)

		locations := hit.Locations["Text"]
		common.Log.Debug("locations=%v", locations)

		var terms []string
		for term := range locations {
			terms = append(terms, term)
		}
		sort.Strings(terms)
		for j, term := range terms {
			loc := locations[term]
			common.Log.Debug("   loc %d %q=%v", j, term, loc)
			for k, pos := range loc {
				common.Log.Debug("     pos %d =%v", k, pos)
			}
		}

	}

	common.Log.Debug("%d Hits", len(searchResults.Hits))
	for i, hit := range searchResults.Hits {
		common.Log.Debug("%3d: %4.2f %3d %q", i, hit.Score, hit.Size(), hit.String())
	}
	return blevePdf.srToMatchSet(tokens, searchResults)
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
func (blevePdf *BlevePdf) srToMatchSet(tokens analysis.TokenStream, sr *bleve.SearchResult) (PdfMatchSet, error) {
	var matches []PdfPageMatch
	if sr.Total > 0 && sr.Request.Size > 0 {
		for _, hit := range sr.Hits {
			m, err := blevePdf.hitToPdfMatch(tokens, hit)
			if err != nil {
				if err == ErrNoMatch {
					continue
				}
				return PdfMatchSet{}, err
			}
			matches = append(matches, m)
		}
	}

	common.Log.Debug("srToMatchSet: hits=%d matches=%d", len(sr.Hits), len(matches))

	results := PdfMatchSet{
		TotalMatches:   int(sr.Total),
		SearchDuration: sr.Took,
		Matches:        matches,
	}
	return results, nil
}

const showlen = 5

func (p PdfPageMatch) String() string {
	spans := p.Spans
	if len(spans) > showlen {
		spans = spans[:showlen]
	}
	return fmt.Sprintf("PDFMATCH{%q:%-3d Score=%.3f Spans=%d%v}",
		p.InPath, p.PageNum, p.Score,
		len(p.Spans), spans)
	// len(p.PagePositions.offsetBBoxes)
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
	fmt.Fprintf(&sb, "%d matches on %d pages, SearchDuration %s\n",
		s.TotalMatches, len(s.Matches), s.SearchDuration)
	for i, m := range s.Matches {
		nl := "\n"
		if i == len(s.Matches)-1 {
			nl = ""
		}
		fmt.Fprintf(&sb, "%4d: %s%s", i+1, m, nl)
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

// hitToPdfMatch returns the PdfPageMatch corresponding the bleve DocumentMatch `hit`.
// The returned PdfPageMatch also contains information that is not in `hit` that is looked up in `blevePdf`.
// We purposely try to keep `hit` small to improve bleve indexing speed and to reduce the
// bleve index size.
func (blevePdf *BlevePdf) hitToPdfMatch(tokens analysis.TokenStream, hit *search.DocumentMatch) (PdfPageMatch, error) {
	m, err := hitToBleveMatch(tokens, hit)
	if err != nil {
		return PdfPageMatch{}, err
	}
	inPath, pageNum, ppos, err := blevePdf.docPagePositions(m.docIdx, m.pageIdx)
	if err != nil {
		return PdfPageMatch{}, err
	}
	text, err := blevePdf.docPageText(m.docIdx, m.pageIdx)
	if err != nil {
		return PdfPageMatch{}, err
	}
	var lineNums []int
	var lines []string
	for _, span := range m.Spans {
		lineNum, line, ok := lineNumber(text, span.Start)
		if !ok {
			return PdfPageMatch{}, fmt.Errorf("No line number. m=%s span=%v", m, span)
		}
		lineNums = append(lineNums, lineNum)
		lines = append(lines, line)
	}

	return PdfPageMatch{
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

func bestPhrases(tokens analysis.TokenStream, termLocMap search.TermLocationMap) []Phrase {
	var terms []string
	for _, tok := range tokens {
		terms = append(terms, string(tok.Term))
	}
	common.Log.Debug("$^$ bestPhrases: terms=%d %q", len(terms), terms)

	termPositions := map[string]map[int]struct{}{}
	startMap := map[int]struct{}{}
	posLoc := map[int]search.Location{}

	var matchedTerms []string
	for i, term := range terms {
		// common.Log.Debug("$^$ %4d: term=%q", i, term)
		locs, ok := termLocMap[term]
		if !ok {
			common.Log.Debug("term=%9q no match", term)
			continue
		}
		common.Log.Debug("$^$ %4d: %9q %d locs", i, term, len(locs))
		matchedTerms = append(matchedTerms, term)
		termPositions[term] = map[int]struct{}{}
		for j, loc := range locs {
			common.Log.Debug("$^$ %8d: %+v", j, loc)
			pos := int(loc.Pos)
			posLoc[pos] = *loc

			termPositions[term][pos] = struct{}{}
			startPos := pos - i
			if startPos < 0 {
				panic("not possible")
			}
			startMap[startPos] = struct{}{}
		}
	}

	if len(matchedTerms) == len(terms) {
		common.Log.Debug("all terms matched! %q", terms)
	} else {
		common.Log.Debug("all terms NOT matched! %d %v", len(matchedTerms), matchedTerms)
	}

	var starts []int
	for v := range startMap {
		starts = append(starts, v)
	}
	sort.Ints(starts)
	common.Log.Debug("starts=%d %v", len(starts), starts)

	var positions []int
	for v := range posLoc {
		positions = append(positions, v)
	}
	sort.Ints(positions)
	common.Log.Debug("positions=%d %v", len(positions), positions)

	var phrases []Phrase
	for _, pos0 := range starts {
		common.Log.Debug("pos0=%d ---------------", pos0)
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
			common.Log.Debug(" k=%d pos=%d ok=%5t term=%q phrase=%v", k, pos, ok, term, phrase)
		}

		if len(phrase.terms) > 0 {
			phrase.start = int(phrase.locations[0].Start)
			phrase.end = int(phrase.locations[len(phrase.terms)-1].End)
			phrases = append(phrases, phrase)
		}
	}
	common.Log.Debug("-------------+++------------- %d phrases", len(phrases))
	for i, phrase := range phrases {
		common.Log.Debug("%4d: %v", i, phrase)
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
	common.Log.Debug("-------------&&&------------- %d phrases", len(phrases))
	for i, phrase := range phrases {
		common.Log.Debug("%4d: %v", i, phrase)
	}
	return phrases
}

// hitToBleveMatch returns a bleveMatch filled with the information in `hit` that comes from bleve.
func hitToBleveMatch(tokens analysis.TokenStream, hit *search.DocumentMatch) (bleveMatch, error) {
	docIdx, pageIdx, err := decodeID(hit.ID)
	if err != nil {
		return bleveMatch{}, err
	}

	locations := hit.Locations["Text"]
	common.Log.Debug("locations=%v", locations)
	var hitTerms []string
	for term := range locations {
		hitTerms = append(hitTerms, term)
	}

	var frags strings.Builder
	var phrases []Phrase
	common.Log.Debug("----------xxx------------ %d Fragments", len(hit.Fragments))
	for k, fragments := range hit.Fragments {
		for _, fragment := range fragments {
			frags.WriteString(fragment)
		}
		termLocMap := hit.Locations[k]
		common.Log.Debug("%q: %d %q", k, len(termLocMap), frags.String())
		phrases = bestPhrases(tokens, termLocMap)
	}

	var spans []Span
	for _, p := range phrases {
		spn := Span{Start: uint32(p.start), End: uint32(p.end), Score: float64(p.score)}
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

// decodeID decodes the ID string passed to bleve in indexDocPagesLoc().
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

// lineNumber returns the 1-offset line number and the text of the line of the contains the 0-offset
//  `offset` in `text`.
// !@#$ precalculate this and stop storing text!
func lineNumber(text string, offset uint32) (int, string, bool) {
	endings := lineEndings(text)
	n := len(endings)
	i := sort.Search(len(endings), func(i int) bool { return endings[i] > offset })
	ok := 0 <= i && i < n
	if !ok {
		common.Log.Error("lineNumber: offset=%d text=%d i=%d endings=%d %+v\n%s",
			offset, len(text), i, n, endings, text)
		return 0, "", false
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
