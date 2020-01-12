[GopherCon-EU-2020](https://www.papercall.io/cfps/2742/submissions/new) Talk Proposal
=============================================================================

ELEVATOR PITCH (<= 300 characters)
--------------

I wrote a full IPP stack for a network print server in a few months while learning Go. IPP is a complex protocol and network stacks have no room for error so this was challenging.
By recapping that experience I will show how Go is suited to efficient development of commercial system software.


TALK PROPOSAL
=============

How I learned Go by Writing an IPP Network Print Server.
-------------------------------------------------------

In 2016, [PaperCut](https://www.papercut.com/), a printing control company, decided it needed to build its own print server. Around that time, our CEO had decided all our printing systems code, which until then had been written in C, was to be written in Go
going forward.

PaperCut’s strategic goals for the print server were to:
* Reduce dependence on the proprietary Windows printing stack.
* Build an OS-independent print server that worked with iOS and Android.
* Support printing from laptops outside Windows network domains.

An [IPP](https://en.wikipedia.org/wiki/Internet_Printing_Protocol) print server along with [DNS](https://www.cloudflare.com/learning/dns/what-is-dns/) + [mDNS](https://en.wikipedia.org/wiki/Multicast_DNS) printer discovery achieved all these goals and was compatible with the IPP based CUPS printing system used on Linux and MacOs. **We just had to write this in a few months as we learned Go.**

IPP is a big spec, about 400 pages. Its main open source C++ implementation in
[CUPS](https://en.wikipedia.org/wiki/CUPS) looks daunting, even allowing for the fact that CUPS contains a printing framework in addition to IPP.

**Textbook methods of software development weren’t going to be fast enough.**

* We needed to compress development time and to learn Go, so instead of logging IPP packets with WireShark, _I hacked the HTTP proxy example that comes with Go to build a transparent proxy that dumped IPP packets._
* IPP is a specified protocol. The spec has a lot of detail but is conceptually straightfoward: printers and their capabilities, print jobs and their attributes. So _I did a table driven design and filled the tables by parsing the ASCII text version of the spec._

In a week, I learned a lot about the Go HTTP stack, IPP byte packets and the how IPP implementation differed from the spec.

At this stage, I had a base design that implemented the spec and Go code that  foreshadowed likely implementation challenges, both in Go library limitations and real-world deviations from the IPP spec. About 6 weeks later, the final IPP server was a straightforward extraction of the server half of the proxy. The client half of the proxy was the foundation of an IPP client that we used for testing the server.

**In this talk I will relive this development and reproduce some of the coding that led to the initial design. The main takeaways will be learning how to:**
* Develop code from a spec using a table driven design.
* Build a network server protocol with an existing but opaque implementation by starting from a proxy.

I will also show how the Go designers made this as easy as I could imagine it could be through the pragmatic elegance that permeates the Go ecosystem.



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

