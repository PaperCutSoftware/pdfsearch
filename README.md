# Pure Go Full Text Search of PDF Files

This library implements full text search for PDFs.
* The public APIs are in [index_search.go](index_search.go).

The are some command lines programs that demonstrate the library's functionality.
* [examples/pdf_search_demo.go](examples/pdf_search_demo.go) demonstrates the main APIs.
* [examples/index.go](examples/index.go) builds an index over a set of PDFs.
* [examples/search.go](examples/search.go) searches the index build by [examples/index.go](examples/index.go).

Binary versions (executables) of these three programs are available in
[releases](https://github.com/PaperCutSoftware/pdfsearch/releases/tag/v0.0.1).
There are 64-bit binaries for Windows, Mac and Linux. The binaries do not require a UniDoc license.

## Installation

    git clone https://github.com/PaperCutSoftware/pdfsearch

Replace `uniDocLicenseKey` and `companyName` in [unidoc_glue.go](internal/doclib/unidoc_glue.go)
with valid [UniDoc](https://unidoc.io/) license fields.

    cd pdfsearch/examples
    go build pdf_search_demo.go
    go build index.go
    go build search.go

### [examples/pdf_search_demo.go](examples/pdf_search_demo.go)

__Usage__: `./pdf_search_demo  -f <PDF path> <search term>`

__Example__: `./pdf_search_demo  -f PDF32000_2008.pdf cubic Bézier curve`

The example will search `PDF32000_2008.pdf` for _cubic Bézier curve_.

`pdf_search_demo.go` shows how to use the APIs in [index_search.go](index_search.go) to
* create indexes over PDFs,
* search those indexes using full-text search, and
* mark up PDFs with the locations of the search matches on pages.

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


## Talks about this library
[GopherCon AU 2019](https://docs.google.com/presentation/d/14FDuKAPgWM2z4V1xag0HFEzL3IJfaS4a7Wt0ChxDG6s/edit?usp=sharing)
