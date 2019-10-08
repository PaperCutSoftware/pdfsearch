// Copyright 2019 PaperCut Software International Pty Ltd. All rights reserved.

package doclib

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/blevesearch/bleve"
	"github.com/unidoc/unipdf/v3/common"
)

// TestBleveMemIndex checks that in-memory (unpersisted) bleve indexes are working correctly.
func TestBleveMemIndex(t *testing.T) {
	testMem(t, "be the person who", 5, 100)
	testMem(t, "but I prefer Python when", 5, 2000)
	testMem(t, "be the person who", 50, 100)
	testMem(t, "with good intentions for", 50, 1000)
	testMem(t, "but I prefer Python when", 100, 2000)
	testMem(t, "in the realm of", 500, 200)
	testMem(t, "a cornucopia of", 5000, 100)
	testMem(t, "if you are wrong then", 100, 100000)
}

// testMem creates in-memory (unpersisted) bleve indexs, populates with `numDocs`, each of size
// `docLen` and some containing the substring `term`, then checks that a query on the index for
// `term` returns the correct documents.
func testMem(t *testing.T, term string, numDocs, docLen int) {
	common.Log.Debug("testMem: numDocs=%d docLen=%d -> size=%d term=%q",
		numDocs, docLen, numDocs*docLen, term)
	common.Log.Debug("allWords=%d %q", len(allWords), allWords)

	index, matchedIDs := makeMemIndex(t, term, numDocs, docLen)
	// index2 := roundTrip(t, index)

	sr := doQuery(t, index, term, numDocs)
	// sr2 := doQuery(t, index2, term, 10)
	// if len(sr.Hits) != len(sr2.Hits) {
	// 	t.Fatalf("len(sr.Hits)=%d != len(sr2.Hits)=%d", len(sr.Hits), len(sr2.Hits))
	// }

	srIDs := searchResultIDs(sr)
	common.Log.Debug("matchedIDs=%d", len(matchedIDs))
	for i, id := range srIDs {
		common.Log.Debug("%4d: %#q", i, id)
	}
	common.Log.Debug("hits=%d", len(srIDs))
	for i, id := range srIDs {
		common.Log.Debug("%4d: %#q", i, id)
	}
	if len(srIDs) != len(matchedIDs) {
		t.Fatalf("len(srIDs)=%d != len(matchedIDs)=%d", len(srIDs), len(matchedIDs))
	}
	for i, mid := range matchedIDs {
		sid := srIDs[i]
		if mid != sid {
			t.Fatalf("%4d: mid=%#q != sid=%#q", i, mid, sid)
		}
	}
}

// makeMemIndex creates an in-memory (unpersisted) bleve index and populates it with `numDocs`
// documents, some of which contain the substring `term`.
func makeMemIndex(t *testing.T, term string, numDocs, docLen int) (bleve.Index, []string) {
	index, err := createBleveMemIndex()
	if err != nil {
		t.Fatalf("createBleveMemIndex failed. err=%v", err)
	}

	var matchedIDs []string

	for i := 1; i <= numDocs; i++ {
		doMatch := i%3 != 2
		payload := " "
		if doMatch {
			payload = " " + term + " "
		}
		id := fmt.Sprintf("%04d", i)

		text := fmt.Sprintf("Phrase %d: %s%s%s", i, phrase(i, 5), payload, phrase(i+numDocs, 10))
		for j := 1; len(text) < docLen; j++ {
			text += " ||| " + phrase(i+j, docLen-len(text))
		}
		idText := IDText{ID: id, Text: text}

		err = index.Index(id, idText)
		if err != nil {
			t.Fatalf("Index failed. id=%#q err=%v", id, err)
		}
		if doMatch {
			matchedIDs = append(matchedIDs, id)
		}
	}

	sort.Strings(matchedIDs)

	return index, matchedIDs
}

func roundTrip(t *testing.T, index bleve.Index) bleve.Index {
	data, err := ExportBleveMem(index)
	if err != nil {
		t.Fatalf("ExportBleveMem failed.err=%v", err)
	}
	index2, err := ImportBleveMem(data)
	if err != nil {
		t.Fatalf("ImportBleveMem failed.err=%v", err)
	}
	common.Log.Info("!!!! data=%d", len(data))
	return index2
}

// doQuery searches for documents in `index` matching `term` and returns up to `maxResults` results.
func doQuery(t *testing.T, index bleve.Index, term string, maxResults int) *bleve.SearchResult {
	query := bleve.NewMatchQuery(term)
	search := bleve.NewSearchRequest(query)
	search.Highlight = bleve.NewHighlight()
	search.Fields = []string{"Text"}
	search.Highlight.Fields = search.Fields
	search.Size = maxResults

	sr, err := index.Search(search)
	if err != nil {
		t.Fatalf("Search failed. term=%q err=%v", term, err)
	}
	return sr
}

// searchResultIDs return the IDs of hits in `sr`.
func searchResultIDs(sr *bleve.SearchResult) []string {
	var ids []string
	for _, hit := range sr.Hits {
		ids = append(ids, hit.ID)
	}
	sort.Strings(ids)
	return ids
}

// phrase returns a string containing words from `allText`.
func phrase(i0, n int) string {
	i0 = (i0 + 17) * 47
	var words []string
	for i := i0; i < i0+n; i++ {
		w := allWords[i%len(allWords)]
		words = append(words, w)
	}
	return strings.Join(words, " ")
}

var allWords = makeWords(allText)

// makeWords return the words in `text`.
func makeWords(text string) []string {
	lines := strings.Split(text, "\n")
	var words []string
	for _, ln := range lines {
		for _, w := range strings.Split(ln, " ") {
			if w == "" || w == "*" {
				continue
			}
			words = append(words, w)
		}
	}
	return words
}

const allText = `
Many modern software product developers work close to the top of a powerful open source
software stack and focus on their customer problems.

This talk is about how I worked further down the Go software stack to write a PDF Full Text
Search library and solve customer problems in unexpected ways.

This talk is about how I wrote a PDF Full Text Search library. This sounds like it
could take a long time to write and is not necessarily the kind of project that you would
expect a small Australian software product company to undertake.

Modern software product companies often solve customer problems using a powerful open source
software stack, such as the Go ecosystem. It takes extra work to create
libraries further down the software stack, but there is extra value in doing so: if a necessary
library doesn’t exist then you can build it yourself. This is critical for companies who survive on
the technical depth of their software.

The Go programming culture and library ecosystem allowed me to work effectively further down the
software stack to build a PDF Full Text Search library. The main factors that made it possible were:
* Most of the work in my solution was done by the high-quality Go libraries my library calls,
 UniDoc for the PDF text extraction and
 bleve for the indexing and full text search.
* These two libraries were written in Go style so they were simple and I could understand how they
 worked which allowed me to figure out how to combine them to solve my problem.
* It was possible to do PDF full text search with these two libraries using one simple additional
  concept, a mapping between PDF text bounding boxes and the offsets of substrings in the text extracted from PDF pages_.
* It took only a small pull request to UniDoc to get a
 function that provided these mappings. UniDoc's idiomatic Go style made this simple.
* It was easy to create bleve indexes over the text extracted by UniDoc then do full text search in
 bleve to get back the page numbers and offsets of the matches. Then I used the offset-bounding-box mappings above and more UniDoc code to mark up the original PDFs with rectangles around the matches.

This sounds straightforward and it was. But it didn't have to be. Not all software stacks have code
much functionality that is as easy to understand and use as that in the Go ecosystem.

Doing PDF full text search with a pure Go library provided several benefits for the software
products my employer, PaperCut, makes.
* Product developers could just call my library from my Go code rather than setting up a web service
 running Elasticsearch. The developer time saved here quickly paid back the 2-3 developer weeks I
 spent writing the Go library.
* The code was used in three apps that were all easy with light-weight executables but would have
been harder with big Java apps running on a JVM.
  1) Search over a user’s files stored locally on disk. Nothing leaves the user's computer.
  2) Check for terms in a PDF as it arrives. (Short-lived in-memory index.)
  3) Search over a shared index stored on a bucket. The app writer needed to run the indexing and
   search code on a Google node and to store the index as a flat memory buffer.

Using a simple pure Go library for PDF full text search has several additional advantages:
* It runs fast. This is a Go app that does nothing but index and search PDFs. It is a tiny fraction of the code in Adobe Reader. Therefore it can run fast.
* It can be fixed fast. There are heuristics in text extraction. These are much easier to tweek in idiomatic Go than in mature Java code.
* It is possible to extend to domain-specific searches with some extra Go coding. E.g. Extract
tables from the PDFs and create indexes over tables for scientific and financial work.

PaperCut decided to open source this code to allow our software product teams to work at the top of
the Go software stack and use a simple high-value open source library for functionality. (This means
that I will spend some time cleaning up the code over the next few weeks in the hope that software
product developers can use it the way I used Go libraries it is based on.)
`
