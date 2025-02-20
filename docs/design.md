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
3. This generated root CA will be included in both the client and server certificate authority bundles, enabling them to verify each other's certificate chain.
4. The TLS client authentication policy on server side will mandate requesting client certificate and verification during handshake. The client will also verify server certificate.
5. All certificates will be **ECDSA** algorithms based, as these are computationally faster than RSA, providing the same level of security with smaller key sizes. The signing algorithm for the certificate will be **ecdsa-with-SHA384**.

>An ideal implementation introduces intermediate CA level for signing client and server certificates that minimizes the risk of compromising root CA if intermediate CA is compromised. We will not introduce intermediate CA level for this solution.

### Choice of cipher suits and TLS version
1. Since both the client and server are under our control and the server does not need to interoperate with multiple types of clients, we will use only TLS 1.3 version. This version of TLS includes strongest cipher suites and key exchange algorithms supporting perfect forward secrecy (**ECDHE**). We will support EC curves **P384** and **P521**.
2. Since the underlying TLS go library does not support application to configure the bulk encryption algorithms, we will rely on the algorithm chosen by the library. This should ideally be **AES-GCM** (128 or 256 bits), or **CHACHA20_POLY1305**.

>Newer version of Go >= 1.24 also support post-quantum key exchange (**ML-KEM**) mechanisms. The solution will not support this.

## Client identification
1. A client can submit a job on the server, query its status and attach to get the standard output and error. The server must be able to identify connecting clients and associate some internal state with them. This internal state under a client-id, includes information on all the jobs initiated by the client in running or terminated state. It will also include gRPC stream transient state if a gRPC client is connected to the server.
2. Since we are using mTLS for authentication, the server will identify clients based solely on the verified client certificate. It will not rely on any other identifier from application-level messages.
3. The server will use received client certificate subject's **CN** to identify the client and associate client connections with internal state.


# Server
1. The server service will implement a gRPC service and will support multiple concurrent client connections. These connections may originate from clients with the same identity, such as when multiple CLI clients are started with same client certificate. Thus, the server will have ability to fork the output of running job to multiple gRPC client connections.
2. The server internally will have a map associating client identity with multiple incoming gRPC connection streams and multiple running job handlers.
3. The server will maintain the state associated with the client identity even after all connections from that client have terminated, since CLI clients will be able to query and attach to running jobs to retrieve output anytime.
4. The server will support graceful shutdown terminating running jobs and client connections if **SIGINT**, **SIGTERM** signals are received. This is stretch goal.
5. The server will disconnect client connections once the associated job terminates.
6. Since the expectation for the server solution is to create c-groups, network namespaces and mounts, the server needs to run as superuser/privileged process. Many of the operations like c-group, change root, cannot be performed just by using **capabilities**.
7. Server will use following cgroup values:
```
cpu.max: 500000 1000000 (grant period)
memory.max: 268435456 (256MB)
io.max = 1048576 wbps and 4 * 4194304 rbps
```
The server will use new root directory if provided or current running directory mount to get major and minor number of the block device for setting **io.max** limits.
The job-id will be part of cgroup path to uniquely identity it. "cpu.max", "memory.max" and "io.max" will be created under it.
```
// Example
/sys/fs/cgroup/4bf02371-5cc5-47f8-a7bf-c891e38bea3e
```

## Authorization
1. The server will ensure that clients with different identities cannot stream output from jobs initiated by others.
2. The server will enforce a policy limiting concurrent jobs per client. The default limit will be two, configurable per client.


# Exec library
1.	This library will be integrated as part of server and will not include any dependency of server or client solution.
2.	The library will encapsulate the complexities of Linux c-groups, network namespaces, and filesystem management, providing a set of intuitive, high-level APIs for applications.
3.	The library will be stateful, as it needs to manage job lifecycle and will perform all the necessary cleanup once the job terminates.
4.	The library will use c-groups v2, assuming that the target Linux kernel is recent enough to support it.
5.	The library will provide file system isolation my mounting necessary host OS directories and changing root of the job. It will mount following mount directories from host OS:
```
/usr/lib
/usr/bin
/usr/sbin
/lib
/bin
/lib64
proc
cgroup2.
```

6.	The library will isolate network traffic by running each job in its own network namespace, creating a single host bridge that connects multiple namespaces. It will support only one subnet for the bridge and virtual Ethernet interfaces. This is stretch goal functionality.
7.	The library streams stdout and stderr using Go channels provided by the application. This approach gives the application the flexibility to buffer the stream or support multiple readers, and it also conveniently notifies the application when EOF is reached or an error occurs. The proposed public interface exposed by this library:
```
type Command interface {
		// Gets UUIDv4 as unique ID for this command
        GetID() string
		// Starts the command and waits for its exit
        Execute() error
		// Returns true if the command has completed
        IsTerminated() bool
		// Returns exit code of the completed command
        GetExitCode() int
		// Returns error if the command has completed with error
        GetExitError() error
		// Sends term signal to the running command. Has no effect if the command is not running.
        SendTermSignal()
		Sends kill signal to the running command and performs umount, cgroups cleanup
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
8.	The library implements following three operations mainly:

New command init: This includes validate passed arguments, create c-groups hierarchy, and prepare new root by mounting needed directories.

Execute command: This includes creating command context, initializing **SysProcAttr.CgroupFD** with cgroups FS, creating stdout/stderr go routines, start the command, and wait for the process to exit.

Finish: This includes umount, cgroups hierarchy cleanup, wait on go routines exit and closing stdout/stderr channels.


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
1. The service proto definition: https://github.com/vikramchhibber/tropelet/tree/design_doc/proto
2. The server will indicate to the client whether a stream message comes from standard error or standard output. The client can use this information to handle error messages differently, for example by rendering them in red."
