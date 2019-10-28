# Pure Go Full Text Search of PDF Files

This library implements full text search for PDF files.
* The public APIs are in [index_search.go](index_search.go).

The are some command lines programs that demonstrate the library's functionality.
* [examples/pdf_search_demo.go](examples/pdf_search_demo.go) demonstrates the main APIs.
* [examples/pdf_search_verify.go](examples/pdf_search_verify.go) verifies the consistency of the
  in-memory and on-disk APIs.
* [examples/index.go](examples/index.go) builds an index over a set of PDFs.
* [examples/search.go](examples/search.go) searches the index build by [examples/index.go](examples/index.go).

Binary versions (executables) of these four programs are available in [releases](../releases/tag/v0.0.0).
There are 64-bit binaries for Windows, Mac and Linux.

## Installation

    git clone https://github.com/PaperCutSoftware/pdfsearch
    cd pdfsearch/examples
    go build pdf_search_demo.go
    go build pdf_search_verify.go
    go build index.go
    go build search.go

### [examples/pdf_search_demo.go](examples/pdf_search_demo.go)

__Usage__: `./pdf_search_demo  -f <PDF path> <search term>`

__Example__: `./pdf_search_demo  -f PDF32000_2008.pdf cubic Bézier curve`

The example will search `PDF32000_2008.pdf` for _cubic Bézier curve_.

`pdf_search_demo.go` shows how to use the APIs in [index_search.go](index_search.go) to
* create indexes over PDF files,
* search those indexes using full-text search, and
* mark up PDF files with the locations of the search matches on pages.

It has 3 types of index
* On-disk. These can be as large as your disk but are slower.
* In-memory with the index stored in a Go struct. Faster but limited to (virtual) memory size.
* In-memory with the index serialized to a []byte. Useful for non-Go callers such as web apps.

### [examples/index.go](examples/index.go)

__Usage__: `./index <file pattern>`

__Example__: `./index ~/climate/**/*.pdf`

The example creates an on-disk index over the PDFs in `~/climate/` and its subdirectories.

### [examples/search.go](examples/search.go)

__Usage__: `./search <search term>`

__Example__: `./search integrated assessment model`

The example searches the on-disk index created by [examples/index.go](examples/index.go)
for _integrated assessment model_.

## Libraries

[index_search.go](index_search.go) uses [UniDoc](https://unidoc.io/) for PDF parsing and [bleve](http://github.com/blevesearch/bleve) for search.
