[GopherCon-EU-2020](https://www.papercall.io/cfps/2742/submissions/new) Talk Proposal
=============================================================================

ELEVATOR PITCH (<= 300 characters)
--------------

I wrote a full IPP stack for a network print server in a few months while learning Go.
This was challenging because IPP is a complex protocol and commercial network stacks have no room for error so this.
By recapping that experience I will show how Go is designed for efficient development of commercial system software.


TALK PROPOSAL
=============

How I learned Go by Writing an IPP Network Print Server.
-------------------------------------------------------

In 2016, [PaperCut](https://www.papercut.com/), a printing control company, decided it needed to build its own print server. At the same time our CEO had decided all our printing systems code, which until then had been written in C, show be written in Go
going forward.

PaperCut’s strategic goals for the print server were to
* Reduce dependence on the proprietary Windows printing stack
* Build an OS independent print server that worked with iOS and Android
* Support printing from laptops outside Windows network domains.

An [IPP](https://en.wikipedia.org/wiki/Internet_Printing_Protocol) print server plus DNS and mDNS printer discovery achieved all these goals and was compatible with the IPP based CUPS printing system used on Linux and MacOs. *We just had to write this in a few months as we learned Go.*

IPP is a big spec, about 400 page. Its main open source C++ implementation in
[CUPS](https://en.wikipedia.org/wiki/CUPS) looked daunting, even allowing for the fact that CUPS contains a printing framework in addition to IPP. Real world implementations of IPP have to deal with the variabile quality of printer implementations. For comparison, CUPS is many developer years of work.

*Textbook methods of software development weren’t going to be fast enough.*

* We needed to compress development time and to learn Go, so instead of logging IPP packets with WireShark, _I hacked the HTTP proxy example that comes with Go to build a transparent proxy that dumped IPP packets_.
* IPP is a specified protocol. The spec has a lot of detail but is conceptually straightford: printers and their capabilities, print jobs and their attributes. I did a table driven design and filled the tables by parsing the ASCII text version of the spec.

In a week I learned a lot about the Go HTTP stack, IPP byte packets and the how IPP implementation differ from the spec.

At this stage, I had a base design that implemented the spec and Go code that showed me the problems that I was likely to encounter in implementation in Go libraries and variations from the IPP spec. The final IPP server was a straightforward extraction of the server half of the proxy. The client half of the proxy was the foundation of an IPP client which we used for testing the server.

In this talk I will relive this development and reproduce some of the coding that led to the final design. The main takeaways will be learning how to.
* Develop code from a spec using a table driven design
* Build a network server protocol with existing but opaque implementation starting from a proxy

I aslo how to show how the Go designers made this as easy as I could imagine it being through the design philosophies of pragmatic elegance that show up all over the Go ecosystem.



BIO
===
[Peter Williams](https://www.linkedin.com/in/peterwilliams97/) is the lead developer of enabling technologies at [PaperCut](https://www.papercut.com/). He develops libraries and features
for PaperCut’s products.

Peter has been developing in Go for the last few years. Some of the features and components he has
recently written in Go for PaperCut are

* A printing back-end for Google Cloud Print.
* A printing back-end and IPP stack for PaperCut Mobility.
* PDF grayscale conversion.
* PDF watermarking.

