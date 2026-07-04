<!-- Archived from https://roc-streaming.org/toolkit/docs/internals/network_protocols.html (Roc Toolkit docs, Sphinx). Reference material for ondaire design; not part of the product. -->

# Network protocols

Roc uses several network protocols for exchanging data between sender and receiver.

Roc avoids inventing new protocols and relies on existing open standards, to take advantage of their careful design and potential interoperability with other software. Roc implements from scratch most protocols (but not codecs). Sometimes there is just no suitable and reusable open implementation. Often the protocol logic must be tightly integrated into the pipeline and it would be hard to employ a stand-alone implementation.

Although Roc is designed to support arbitrary protocols, and most of the code is generic, so far all supported protocols are RTP-based. RTP by itself is very basic but also very extensible. To employ RTP, one needs to select an RTP profile and, if necessary, some RTP extensions.

As a basis, Roc uses RTP Audio/Video Profile (AVPF) which specifies how to stream audio and video using RTP.

It also employs FEC Framework (FECFRAME), which specifies how to incorporate various FEC schemes into RTP. Using FECFRAME may prevent interoperability with third-party RTP implementations that don’t support it. If you need compatibility with such applications, you can disable FEC.

Roc also employs various RTCP extensions to exchange non-media reports between sender and receiver. Usually this doesn’t break interoperability with implementations that don’t support extensions, however some features may not work with such implementations (e.g. sender-side latency tuning).

Here is the full list of the network protocols implemented by Roc:

**RFC** | **Name** | **Comment**  
---|---|---  
[RFC 3550](https://tools.ietf.org/html/rfc3550) | RTP: A Transport Protocol for Real-Time Applications | Basic RTP and RTCP  
[RFC 3551](https://tools.ietf.org/html/rfc3551) | RTP Profile for Audio and Video Conferences with Minimal Control | RTP AVPF (Audio/Video Profile)  
[RFC 3611](https://tools.ietf.org/html/rfc3611) | RTP Control Protocol Extended Reports |  RTCP XR (supported blocks: DLRR, RRTR)  
[RFC 6776](https://tools.ietf.org/html/rfc6776) | Measurement Identity and Information Reporting Using a Source Description (SDES) Item and an RTCP Extended Report (XR) Block | RTCP XR Measurement Information Block  
[RFC 6843](https://tools.ietf.org/html/rfc6843) | RTP Control Protocol (RTCP) Extended Report (XR) Block for Delay Metric Reporting | RTCP XR Delay Metrics Block  
[RFC 6363](https://tools.ietf.org/html/rfc6363) | Forward Error Correction (FEC) Framework | FECFRAME  
[RFC 6865](https://tools.ietf.org/html/rfc6865) | Simple Reed-Solomon Forward Error Correction (FEC) Scheme for FECFRAME | Reed-Solomon FEC Scheme  
[RFC 6816](https://tools.ietf.org/html/rfc6816) | Simple Low-Density Parity Check (LDPC) Staircase Forward Error Correction (FEC) Scheme for FECFRAME | LDPC-Staircase FEC Scheme  
  
[ ](../index.html)
