Pure Go Full Text Search of PDF Files
=====================================

Main Example Program
--------------------

`examples/index_search_example.go`

This program shows how to use the APIs in `index_search.go` to
* create indexes over PDF files,
* search those indexes using full-text search, and
* mark up PDF files with the locations of the search matches on pages.

It has 3 types of index
* On-disk. These can be as large as your disk but are slower.
* In-memory with the index stored in a Go struct. Faster but limited to (virtual) memory size.
* In-memory with the index serialized to a []byte. Useful for non-Go callers such as web apps.

Other Example Programs
---------------------
The repo also has a series of example programs for doing [full text search](https://en.wikipedia.org/wiki/Full-text_search) on PDF files in pure Go. It uses [UniDoc](https://unidoc.io/) for PDF parsing and [bleve](http://github.com/blevesearch/bleve) for search.



Libraries
=========

The simple programs are to explore the UniDoc and Bleve libraries.

Installation (UniDoc)
---------------------
	cd $GOPATH/src/github.com
	mkdir -p github.com/unidoc
	cd github.com/unidoc
	git clone https://github.com/peterwilliams97/unidoc
	cd unidoc
	git checkout v3.imagemark

Installation (Bleve)
--------------------
	go get github.com/blevesearch/bleve/...
	go get github.com/blevesearch/snowballstem
	go get github.com/kljensen/snowball
	go get github.com/willf/bitset
	go get github.com/couchbase/moss
	go get github.com/syndtr/goleveldb/leveldb
	go get github.com/rcrowley/go-metrics

	cd $GOPATH/src/github.com/blevesearch/
	git clone https://github.com/peterwilliams97/bleve
	cd bleve
	git checkout explore.01

	cd $GOPATH/src/github.com/blevesearch/
	git clone https://github.com/peterwilliams97/blevex
	cd blevex

Installation (flatbuffers)
--------------------------
	brew update
	brew install flatbuffers --HEAD
	go get github.com/google/flatbuffers/go


Build flatbuffers
-----------------
	cd $GOPATH/src/github.com/peterwilliams97/pdf-search/serial
	flatc -g doc_page_locations.fbs
	pushd $GOPATH/src/github.com/peterwilliams97/pdf-search/serial/cmd/locations
	go run main.go
	go test -test.bench .

Example Programs
================

Basic search
------------
	simple_index.go        Index some PDFs
	simple_search.go       Full text search the PDFs indexed by `simple_index.go`

e.g.

	go run simple_index.go ~/testdata/adobe/PDF32000_2008.pdf
	go run simple_search.go Adobe PDF

gives

<strong>searchResults=755 matches, showing 1 through 10, took 5.396772ms</strong>

<strong>1. 1faa31928e.284 (0.414407) </strong>

<code>
<strong><em>PDF</em></strong> 32000-1:2008
Table 119 –  Character collections for predefined CMaps, by <strong><em>PDF</em></strong> version  (continued)
CMAP <strong><em>PDF</em></strong> 1.2 <strong><em>PDF</em></strong> 1.3 <strong><em>PDF</em></strong> 1.4 <strong><em>PDF</em></strong> 1.5
GBK2K-H/V — — <strong><em>Adobe</em></strong>-GB1-4 <strong><em>Adobe</em></strong>-GB1-4
UniGB-UCS2-H/V — <strong><em>Adobe</em></strong>-…
</code>

<strong>2. 1faa31928e.6 (0.322457) </strong>

<code>
<strong><em>PDF</em></strong> 32000-1:2008
Foreword
On January 29, 2007, <strong><em>Adobe</em></strong> Systems Incorporated announced it’s intention to release the full Portable
Document Format (<strong><em>PDF</em></strong>) 1.7 specification to the American National Standa…
...
</code>

or

	go run simple_index.go ~/testdata/adobe/*.pdf
	go run simple_search.go Type1

gives

<strong>searchResults=220 matches, showing 1 through 10, took 12.175059ms</strong>

<strong>1. 1faa31928e.710 (0.721218)</strong>

<code>
…bj

7  0  obj
<<  /Type  /Font
    /Subtype  /<strong><em>Type1</em></strong>
    /Name  /F1
    /BaseFont  /Helvetica
    /<strong><em>Encoding</em></strong> /<strong><em>MacRomanEncoding</em></strong>
>>
endobj
</code>

<strong>2. 1faa31928e.271 (0.427241)</strong>

<code>
…ut this <strong><em>encoding</em></strong>does play a role as a default <strong><em>encoding</em></strong>(as shown in Table 114). The regular encodings
used for Lat
</code>

Concurrent indexing
-------------------
	concurrent_index_doc.go        Index PDFs concurrently. Granularity is PDF file.
	concurrent_index_page.go       Index PDFs concurrently. Granularity is PDF page.


References
==========
* [Full text search](https://en.wikipedia.org/wiki/Full-text_search)
* [Information retrieval](https://en.wikipedia.org/wiki/Information_retrieval)

flatbuffers
-----------
	https://rwinslow.com/posts/use-flatbuffers-in-golang/
	https://github.com/google/flatbuffers/blob/master/tests/go_test.go
	https://google.github.io/flatbuffers/flatbuffers_guide_use_go.html


TODO
====
* Move to unidoc:v3
* Make go gettable
* Add tests
* Markup PDF files
*  - Locations -> Positions
*  - Efficient position encoding
			Line Y and height
			Start of word
*  - ReadDocPageLocations Gather all (doc,page) entries, open the necessary docs and return the pages.
* parts => strings.Builder https://golang.org/pkg/strings/#Builder
* Offset => Start, End

Background
==========
https://www.youtube.com/watch?v=RsOIiW_Ec4c 45:40


### Page Indexing
https://www.hathitrust.org/blogs/large-scale-search/tale-two-solrs-0

https://www.hathitrust.org/full-text-search-features-and-analysis

