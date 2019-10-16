Pure Go Full Text Search of PDF Files
=====================================

This library implements full text search for PDF files.
* The public APIs are in [index_search.go](index_search.go).
* [examples/pdf_search_demo.go](examples/pdf_search_demo.go) is a command line program
  that demonstrates these APIs.
* [examples/pdf_search_verify.go](examples/pdf_search_verify.go) is a command line program
  that verifies the consistency of these  APIs.

Installation
---------------------
    git clone https://github.com/PaperCutSoftware/pdfsearch
    cd pdfsearch/examples
    go build -ldflags "-s -w" pdf_search_demo.go
    upx index_search_example

    (Gives a 5,931,024 byte binary on Peter's macbook)

Usage
-----
    ./pdf_search_demo -f PDF32000_2008.pdf Adobe


[pdf_search_demo.go](examples/pdf_search_demo.go) program shows how to use the APIs in
[index_search.go](index_search.go) to
* create indexes over PDF files,
* search those indexes using full-text search, and
* mark up PDF files with the locations of the search matches on pages.

It has 3 types of index
* On-disk. These can be as large as your disk but are slower.
* In-memory with the index stored in a Go struct. Faster but limited to (virtual) memory size.
* In-memory with the index serialized to a []byte. Useful for non-Go callers such as web apps.


Libraries
--------

This simple programs  uses [UniDoc](https://unidoc.io/) for PDF parsing and [bleve](http://github.com/blevesearch/bleve) for search.  It can be used explore the UniDoc and Bleve libraries.



### Page Indexing
https://www.hathitrust.org/blogs/large-scale-search/tale-two-solrs-0

https://www.hathitrust.org/full-text-search-features-and-analysis

TIMINGS
-------

	Some timings from Peter's old MacBook:

	./pdf_search_demo -p -f ~/testdata/adobe/PDF32000_2008.pdf  Type 1
	[On-disk index] Duration=72.4 sec

	./pdf_search_demo -f ~/testdata/adobe/PDF32000_2008.pdf  Type 1
	[In-memory index] Duration=22.7 sec

	Timings from Peter's Mac Book Pro.
	./pdf_search_demo -f ~/testdata/other/pcng/docs/target/output/pcng-manual.pdf  PaperCut NG
	[In-memory index] Duration=87.3 sec (87.220 index + 0.055 search) (454.4 pages/min)
	[In-memory index] Duration=91.9 sec (91.886 index + 0.060 search) (431.3 pages/min)
	[In-memory index] Duration=83.1 sec (83.027 index + 0.068 search) (477.3 pages/min)
	[On-disk index] Duration=126.2 sec (126.039 index + 0.152 search) (314.3 pages/min)
	[Reused index] Duration=0.2 sec (0.000 index + 0.159 search) (0.0 pages/min) 0 pages in 0 files []
	661 pages in 1 files [/Users/pcadmin/testdata/other/pcng/docs/target/output/pcng-manual.pdf]


TODO
----
Cleanup validate() Check()
Simple command line program
Remove panic()'s
Remove !@#$'s
Remove uint32 etc
Remove abstraction layers, especially unidoc_glue.go
Get free text search working

References
----------
[Full-Text Search Tips & Tricks (Denis Rosa)](https://www.youtube.com/watch?v=B9qRJhA1ONs)
https://blevesearch.com/docs/Query-String-Query/
https://twitter.com/hashtag/fulltextsearch
