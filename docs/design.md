The server and its clients will use mutual TLS authentication (mTLS) mechanism to verify each other. 
The solution will include script to generate self-signed root certificate (CA), and scripts for signing server and client certificates.
This generated root CA will be included in both the client and server certificate authority bundles, enabling them to verify each other’s certificate chain.
The TLS client authentication policy on server side will mandate requesting client certificate and verification during handshake. The client will also verify server certificate.

An ideal implementation introduces intermediate CA level for signing client and server certificates that minimizes the risk of compromising root CA if intermediate CA is compromised. We will not introduce intermediate CA level for this solution.

All certificates will be ECDSA algorithms based, as they are computationally faster than RSA, providing the same level of security with smaller key sizes. The signing algorithm for the certificate will be “ecdsa-with-SHA384”.


Choice of cipher suits and TLS version

Since both the client and server are under our control and the server does not need to interoperate with multiple types of clients, we will use only TLS 1.3 version. This version of TLS includes strongest cipher suites and key exchange algorithms supporting perfect forward secrecy (ECDHE). We will support EC curves P384 and P521.

Since the underlying TLS go library does not support application to configure the bulk encryption algorithms, we will rely on the algorithm chosen by the library. This should ideally be AES-GCM 128 or 256.


Client identification
Since we are using mTLS for authentication, the server will identify clients based solely on the verified client certificate. It will not rely on any other identifier from application-level messages.
The server will derive a composite key using the SHA-256 hash of the received client certificate’s Issuer and Serial Number. Assuming every CA signs certificates with unique serial numbers, this approach ensures that each client can be uniquely identified, even if certificates are issued by the same authority.
Including the Issuer in the identifier ensures that if the issuing authority changes in the future, the server can still uniquely identify certificates/clients, even in the event of a serial number conflict.

