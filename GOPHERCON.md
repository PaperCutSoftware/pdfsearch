[GopherCon-AU-2019](https://www.papercall.io/gophercon-au-2019) Talk Proposal
=============================================================================

ELEVATOR PITCH (<= 300 characters)
--------------
Many modern software product developers work close to the top of a powerful open source
software stack and focus on their customer problems.

This talk is about how I worked further down the Go software stack to write a PDF Full Text
Search library and solve customer problems in unexpected ways.

TALK PROPOSAL
=============
PDF Full Text Search in Pure Go. Why and How I Wrote it.
-------------------------------------------------------
This talk is about how I wrote a
[PDF Full Text Search library](https://github.com/PaperCutSoftware/pdfsearch). This sounds like it
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
 [UniDoc](https://unidoc.io/) for the PDF text extraction and
 [bleve](http://blevesearch.com/) for the indexing and full text search.
* These two libraries were written in Go style so they were simple and I could understand how they
 worked which allowed me to figure out how to combine them to solve my problem.
* It was possible to do PDF full text search with these two libraries using one simple additional
  concept, _a mapping between PDF text bounding boxes and the offsets of substrings in the text extracted from PDF pages_.
* It took only a small [pull request](https://github.com/unidoc/unipdf/pull/75) to UniDoc to get a
 function that provided these mappings. UniDoc's idiomatic Go style made this simple.
* It was easy to create bleve indexes over the text extracted by UniDoc then do full text search in
 bleve to get back the page numbers and offsets of the matches. Then I used the offset-bounding-box mappings above and more UniDoc code to mark up the original PDFs with rectangles around the matches.

This sounds straightforward and it was. But it didn't have to be. Not all software stacks have code
much functionality that is as easy to understand and use as that in the Go ecosystem.

Doing PDF full text search with a pure Go library provided several benefits for the software
products my employer, [PaperCut](https://www.papercut.com/), makes.
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
* It runs fast. This is a Go app that does nothing but index and search PDFs. It is a tiny fraction
  of the code in Adobe Reader. Therefore it can run fast.
* It can be fixed fast. There are heuristics in text extraction. These are much easier to tweek in
  idiomatic Go than in mature Java code.
* It is possible to extend to domain-specific searches with some extra Go coding. E.g. Extract
 tables from the PDFs and create indexes over tables for scientific and financial work.

PaperCut decided to open source this code to allow our software product teams to work at the top of
the Go software stack and use a simple high-value open source library for functionality. (This means
that I will spend some time cleaning up the code over the next few weeks in the hope that software
product developers can use it the way I used Go libraries it is based on.)

BIO
===
[Peter Williams](https://www.linkedin.com/in/peterwilliams97/) is the lead developer of
enabling technologies at [PaperCut](https://www.papercut.com/). He develops libraries and features
for PaperCut’s products.

Peter has been developing in Go for the last few years. Some of the features and components he has
recently written in Go for PaperCut are
* A printing back-end for Google Cloud Print.
* A printing back-end and IPP stack for PaperCut Mobility.
* PDF grayscale conversion.
* PDF watermarking.

