Pure Go Full Text Search of PDF Files
=====================================
This library implements full text search for PDF files.
* The public APIs are in [index_search.go](index_search.go).
* A command line program that exercises these APIs in
[examples/index_search_example.go]](examples/index_search_example.go).

Installation
---------------------
	git clone https://github.com/PaperCutSoftware/pdfsearch
	cd pdfsearch/examples

Usage
=====
    go run index_search_example.go -f PDF32000_2008.pdf Adobe


This program shows how to use the APIs in `index_search.go` to
* create indexes over PDF files,
* search those indexes using full-text search, and
* mark up PDF files with the locations of the search matches on pages.

It has 3 types of index
* On-disk. These can be as large as your disk but are slower.
* In-memory with the index stored in a Go struct. Faster but limited to (virtual) memory size.
* In-memory with the index serialized to a []byte. Useful for non-Go callers such as web apps.


Libraries
=========

This simple programs  uses [UniDoc](https://unidoc.io/) for PDF parsing and [bleve](http://github.com/blevesearch/bleve) for search.  It can be used explore the UniDoc and Bleve libraries.





Background
==========
https://www.youtube.com/watch?v=RsOIiW_Ec4c 45:40


### Page Indexing
https://www.hathitrust.org/blogs/large-scale-search/tale-two-solrs-0

https://www.hathitrust.org/full-text-search-features-and-analysis

