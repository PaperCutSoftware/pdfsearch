ELEVATOR PITCH (<= 300 characters)
==============
Many modern software product developers work close to the top of a powerful open source
software stack and focus on their customer problems.

This talk is about how I worked further down the Go software stack to write a PDF Full Text
Search Engine and provided business value in unexpected ways.

TALK PROPOSAL
=============
PDF Full Text Search in Pure Go. What? Why? How?
------------------------------------------------
A common and effective way for modern software product companies to operate is to solve customer
problems using a powerful open source software stack.

Go has become an effective software stack for software product development.

Most product developers try to work as close to the top of the stack as possible and focus on their
customer problems. Go has been good for this.

It takes extra work to create libraries further down the software stack, but there is extra value in
doing so: if a necessary library doesn’t exist then you can build it yourself. This is critical for
companies who survive on the technical depth of their software.

This talk is about how I wrote a PDF Full Text Search Engine, something that seems quite complex and
not a project that you would expect a software product company to undertake. Existing PDF Full Text
Search Engines, such as the one inside Elasticsearch are complex and appear to have several developer
years of work.

In this talk I will explain

* How I wrote a [PDF Full Text Search Engine](https://github.com/PaperCutSoftware/pdfsearch
) in a few developer weeks
* How the maturity of the Go software stack allowed this (and give dates on when the libraries I used
   gained functionality necessary for this). The libraries were
  1) [UniDoc](https://unidoc.io/) for the PDF text extraction and
  2) [bleve](http://github.com/blevesearch/bleve) for the indexing and full text search.
* The business value of a small pure Go Full Text Search Engine with limited functionality over a
  fully-featured Java implementation (see below).
* The development possibilities an idiomatic Go implementation opens up (see below).

Business Value
---------------
* Small memory footprint and runs anywhere means that we don’t need to set up and configure a compute
  instance with a webservice around Elasticsearch. Just call the Go function. Quickly paid back the
  2-3 developer weeks spent writing the Go library.
* Was used in 3 apps.
  1) Search over a user’s files stored locally on disk. Private
  2) Check for terms in a PDF as it arrives. (Short-lived in-memory index.)
  3) Search over a shared index stored on a bucket. The app writer needed the run the indexing code and
   search code one Google node and to store the index as a flat memory buffer.
 * In the talk we will show how we solved these problems by calling Go code.


Development possibilities of idiomatic Go implementations
----------------------------------------------------------------------
* Runs fast. This is a Go app that does nothing but index and search PDFs. It is a tiny fraction of the code in Adobe Reader. Therefore it can run fast.
* Can be fixed fast. There are heuristics in text extraction. These are much easier to tweek in idiomatic Go than in mature Java code.
* Write domain specific searches. E.g. Extract tables from the PDFs and create indexes over tables for scientific and financial work.

NOTES FOR REVIEWERS
===================
The code that I will describe in the talk, [PDF Full Text Search Engine](https://github.com/PaperCutSoftware/pdfsearch
), is referenced in the proposal above. This is currently waiting on [UniDoc](https://unidoc.io/)
to merge a [pull request](https://github.com/unidoc/unipdf/pull/75). We expect this to be completed
before the end of June.

This code is used in [PaperCut](https://www.papercut.com/) products. The PaperCut commercial code
has a few modifications such as licence keys and other private data. Otherwise it is the same as
the open source code.

The PaperCut code runs on Windows, Mac and Unix on customer premises and on Google Cloud for
PaperCut's web based products.

The open source code, as released this week, is messy and reflects the circumstances under which it
was created
1) I hacked together local on-disk search as a proposal for a new feature.
2) A product group requested in-memory search, so I hacked this on top the hacks in 1) to get this working.
3) A product group with a cloud product needed serialization of indexes to/from `[]byte` and
   `io.ReadSeeker` readers for PDFs right away and I hacked this together on top of the growing mess.
Everything worked because
1) Most of the code comes from the 2 high-quality libraries used,
2) The 2 libraries are linked by a simple concept, a mapping between PDF text bounding boxes and
   offsets in the text extacted from the libraries
3) A lot of tests.

PaperCut decided to open source the code so that our software product teams can work at the top of
the software stack and use a simple high-value open source library for functionality. (This means
that I will spend some time cleaning up the code over the next few weeks so product developers can
use it the way I used Go libraries it is based on )

DISCLOSURE: I am a UniDoc contributor and I wrote much of UniDoc's text extraction code. PaperCut
uses UniDoc text extraction code to provide competitive advantage, in ways I cannot share here.
These uses are specific to the market PaperCut sells to and are not widely useful like PDF full
text search.


BIO
===
My name is [Peter Williams](https://www.linkedin.com/in/peterwilliams97/), lead developer of
enabling technologies at [PaperCut](https://www.papercut.com/). I develop libraries and code for
PaperCut’s products.

I have been developing in Go for the last few years. Some of the features and componets I have
recently written in Go for PaperCut are
* A printing back-end for Google Cloud Print
* A printing back-end and IPP stack for PaperCut Mobility
* PDF grayscale conversion
* PDF watermarking

