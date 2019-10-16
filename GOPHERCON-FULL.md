PDF Full Text Search in Pure Go. Why and How I Wrote it.
========================================================
Is Go a great language?

I don't know and don't care. It works for me.

Go is well suited to the work I do at PaperCut.

* Printer monitoring software
* Tightly coupled to OS
* Run on Windows, Mac OS and many Linux distros.
* Complex
* Soft real time

Go allows me to

* Develop complex software
* Build small executables

Go design choices are suitable for my work

* Simplicity
* Garbage collection
* Compiled
* Strongly typed (apart from runtime ...)
* Low ceremony
* Libraries for most things I need
  - Networking
  - Crypto
* Libraries are understandable so I can build executables that are unlikely to surprise PaperCut's
customers.

My current PaperCut projects are [PaperCut Mobility](https://www.papercut.com/tour/mobility-print/) and
[Pocket](https://www.papercut.com/products/papercut-pocket/). These are
[IPP](https://en.wikipedia.org/wiki/Internet_Printing_Protocol) based print servers.

* IPP is an HTTP(S) printing protocol
* PaperCut's IPP is written in pure Go on top of the Go [HTTP](https://golang.org/pkg/net/http/)
server code.
* Works on Windows, Mac and Linux.
* Therefore can be installed on local servers, edge nodes and in cloud.
* Convenient binary. No JVM required.
* Works well in practice. Millions? of installations.

How Do we Add Value to our IPP Server?

The IPP protocol has a _print in black and white_ setting that for technical reasons is hard to
implement on some operating systems.

* I needed to implemement PDF color to grayscale conversions.

PDF color to grayscale conversion is not in any standard library I know in any language.
There are PDF libraries for some languages. xpdf for C++, PdfBox for Java.
I couldn't find any native Go PDF library as mature as xpdf or PdfBox.
But it would not have helped if I could. xpdf and PdfBox do a lot of useful things but PDF modification
is not one of them.
I found UniPdf which is a small library that was open to new features. They were about to do PDF
conversions and they helped me add PDF grayscale conversion to their library.

This is what I need from open source libraries for the kind of work I do.

Non-mainstream. I need libraries that are under active development and will take major contributions
The alternative would be to do all the work myself. This would be costly to me in the long term
because I would have to maintain a
perpetual branch of the library and keep merging new features back to my branch.

There is good eco-system of such libraries in Go.

* Solving a wide range of problems.
* Code is simple and understandable.
* Libraries have shallow dependencies

That's the world I live in when I program.

A Harder Problem
----------------
PaperCut needs full text search over






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
software stack, such as the Go ecosystem. It takes extra work to create libraries further down the
software stack, but there is extra value in doing so: if a necessary
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

