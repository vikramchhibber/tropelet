# Introduction
These notes cover brief design and scope of a job execution service that can run arbitrary Linux jobs initiated by authenticated remote clients.
This prototype consists of three components:
1.	A library providing APIs to launch, terminate, and check the status of Linux job. It will isolate each job in its own network namespace with separate PID and filesystem, and will use cgroups to limit CPU, memory, and I/O usage.
2.	The gRPC service leveraging this library to offer server-side calls for launching, terminating, and querying running job status. The server will authenticate the connecting clients using certificates.
3.	The command-line interface utility connecting the gRPC service to interact with the user and making client side gRPC calls.

# Security
## Authentication
### mTLS
1. The server and its clients will use mutual TLS authentication (mTLS) mechanism to verify each other. 
2. The solution will include script to generate self-signed root certificate (CA), and scripts for signing server and client certificates.
3. This generated root CA will be included in both the client and server certificate authority bundles, enabling them to verify each other’s certificate chain.
4. The TLS client authentication policy on server side will mandate requesting client certificate and verification during handshake. The client will also verify server certificate.
5. All certificates will be **ECDSA** algorithms based, as these are computationally faster than RSA, providing the same level of security with smaller key sizes. The signing algorithm for the certificate will be **ecdsa-with-SHA384**.

>An ideal implementation introduces intermediate CA level for signing client and server certificates that minimizes the risk of compromising root CA if intermediate CA is compromised. We will not introduce intermediate CA level for this solution.

### Choice of cipher suits and TLS version
1. Since both the client and server are under our control and the server does not need to interoperate with multiple types of clients, we will use only TLS 1.3 version. This version of TLS includes strongest cipher suites and key exchange algorithms supporting perfect forward secrecy (**ECDHE**). We will support EC curves **P384** and **P521**.
2. Since the underlying TLS go library does not support application to configure the bulk encryption algorithms, we will rely on the algorithm chosen by the library. This should ideally be **AES-GCM** (128 or 256 bits), or **CHACHA20_POLY1305**.

>Newer version of Go >= 1.24 also support post-quantum key exchange (**ML-KEM**) mechanisms. The solution will not support this.

## Client identification
1. A client can start a job by connecting to the server. However, the network connection may break for various reasons. The solution will provide ability to query the status of running jobs and reattach to continue receiving job output. Therefore, the server must be able to identify connecting clients and associate some internal state with them.
2. Since we are using mTLS for authentication, the server will identify clients based solely on the verified client certificate. It will not rely on any other identifier from application-level messages.
3. The server will derive a composite key using the **SHA-1** hash (Weaker hash, meant for key and no cryptographic significance) of the received client certificate’s **Issuer** and **Serial Number**. This key will be used by server to associate client connections with internal state.
4. Assuming every CA signs certificates with unique serial numbers, this approach ensures that each client can be uniquely identified, even if certificates are issued by the same authority.
5. Including the **Issuer** in the identifier ensures that if the issuing authority changes in the future, the server can still uniquely identify certificates/clients, even in the event of a serial number conflict.


# Server
1. The server service will implement a gRPC service and will support multiple concurrent client connections. These connections may originate from clients with the same identity, such as when multiple CLI clients are started with same client certificate. Thus, the server will have ability to fork the output of running job to multiple gRPC client connections.
2. The server internally will have a map associating **SHA-1** client identity with multiple incoming gRPC connection streams and multiple running job handlers.
3. If a job is running, the server will continue to maintain state associated with client identity even after all incoming client connections have terminated under that identity, since the solution will support CLI clients reattaching to running jobs.
4. The server will support graceful shutdown terminating running jobs and client connections if **SIGINT**, **SIGTERM** signals are received.
5. The server will disconnect client connections once the associated job terminates.
6. Since the expectation for the server solution is to create c-groups, network namespaces and mounts, the server needs to run as superuser/privileged process. Many of the operations like c-group, change root, cannot be performed just by using **capabilities**.


# Exec library
1.	This library will be integrated as part of server and will not include any dependency of server or client solution.
2.	The library will encapsulate the complexities of Linux c-groups, network namespaces, and filesystem management, providing a set of intuitive, high-level APIs for applications.
3.	The library will be stateful, as it needs to manage job lifecycle and will perform all the necessary cleanup once the job terminates.
4.	The library will use c-groups v2, assuming that the target Linux kernel is recent enough to support it.
5.	The library will provide file system isolation my mounting necessary host OS directories and changing root of the job.
6.	The library will isolate network traffic by running each job in its own network namespace, creating a single host bridge that connects multiple namespaces. It will support only one subnet for the bridge and virtual Ethernet interfaces. This is stretch goal functionality.
7. The library streams stdout and stderr using Go channels provided by the application. This approach gives the application the flexibility to buffer the stream or support multiple readers, and it also conveniently notifies the application when EOF is reached or an error occurs.8. The proposed public interface exposed by this library:
```
type Command interface {
        GetID() string
        Execute() error
        IsTerminated() bool
        GetExitError() error
        Terminate()
        Finish()
}

type ReadChannel chan []byte
type CommandOption func(*commandImpl) error

func WithTimeout(timeout time.Duration) CommandOption
func WithStdoutChan(stdoutChan ReadChannel) CommandOption
func WithStderrChan(stderrChan ReadChannel) CommandOption
func WithMemoryLimit(memKB uint32) CommandOption
...

func NewCommand(name string, args []string, options ...CommandOption) (Command, error)
```

# CLI Client
1. Connect to the remote server and get details on running jobs for this client
```
tctl
# OR
tctl list
# Proposed output
ba7371d5-a848-4b5d-b90a-7479342051a4 running    find / -name "foobar*go"
c19cd05a-42dd-40a0-a0e6-b56c0ffd98ed terminated sleep 10
```
2. Execute remote job.
```
tctl exec journalctl -f
```
3. Attach to a remote running job to get the output.
```
tctl attach ba7371d5-a848-4b5d-b90a-7479342051a4
```
4. Request termination of remote running job.
```
tctl terminate ba7371d5-a848-4b5d-b90a-7479342051a4
```
5. Get remote job status
```
tctl status ba7371d5-a848-4b5d-b90a-7479342051a4
ba7371d5-a848-4b5d-b90a-7479342051a4 terminated foo-bar [not found]
```


# gRPC
The service proto definition: https://github.com/vikramchhibber/tropelet/tree/design_doc/proto
