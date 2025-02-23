package client

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/troplet/internal/shared"
	"github.com/troplet/pkg/proto"
)

type Config struct {
	ServerAddress string
	CABundlePath  string
	CertPath      string
	CertKeyPath   string
}

func (c Config) String() string {
	// Must not log sensitive credentials
	return "\nServer address   :" + c.ServerAddress +
		"\nCA bundle        :" + c.CABundlePath +
		"\nCert             :" + c.CertPath +
		"\nCert key         :" + c.CertKeyPath
}

type Client struct {
	config     *Config
	logger     shared.Logger
	clientConn *grpc.ClientConn
}

func NewClient(config *Config, logger shared.Logger) *Client {
	return &Client{config, logger, nil}
}

func (c *Client) Finish() {
	if c.clientConn != nil {
		c.clientConn.Close()
	}
}

func (c *Client) ListJobs() {
	client, err := c.createClient()
	if err != nil {
		return
	}
	resp, err := client.ListJobs(context.Background(), &proto.ListJobsRequest{})
	if err != nil {
		c.logger.Errorf("Failed getting jobs list: %v", err)
		return
	}
	c.dumpJobEntries(resp.Jobs)
}

func (c *Client) GetJobStatus(jobID string) {
	client, err := c.createClient()
	if err != nil {
		return
	}
	resp, err := client.GetJobStatus(context.Background(),
		&proto.GetJobStatusRequest{Id: jobID})
	if err != nil {
		c.logger.Errorf("Failed getting jobs status: %v", err)
		return
	}
	c.dumpJobEntries([]*proto.JobEntry{resp.Job})
}

func (c *Client) LaunchJob(cmd string, args []string) {
	client, err := c.createClient()
	if err != nil {
		return
	}
	resp, err := client.LaunchJob(context.Background(),
		&proto.LaunchJobRequest{Command: cmd, Args: args})
	if err != nil {
		c.logger.Errorf("Failed launching job: %v", err)
		return
	}
	fmt.Printf("Job ID: %s\n", resp.Id)
}

func (c *Client) TerminateJob(jobID string) {
	client, err := c.createClient()
	if err != nil {
		return
	}
	_, err = client.TerminateJob(context.Background(),
		&proto.TerminateJobRequest{Id: jobID})
	if err != nil {
		c.logger.Errorf("Failed terminating job: %v", err)
	}
}

func (c *Client) AttachJob(jobID string) {
	client, err := c.createClient()
	if err != nil {
		return
	}
	stream, err := client.AttachJob(context.Background(),
		&proto.AttachJobRequest{Id: jobID})
	if err != nil {
		c.logger.Errorf("Failed attaching job: %v", err)
		return
	}
	for {
		response, err := stream.Recv()
		if err != nil {
			if err != io.EOF {
				c.logger.Errorf("Server returned error: %v", err)
			}
			return
		}
		if !response.StreamEntry.IsStdError {
			fmt.Print(string(response.StreamEntry.Entry))
		} else {
			// Print std errors in red
			fmt.Print("\033[31m" + string(response.StreamEntry.Entry) + "\033[0m")
		}
	}
}

func (c *Client) dumpJobEntries(entries []*proto.JobEntry) {
	for _, entry := range entries {
		fmt.Printf("\n")
		fmt.Printf("Job id     : %s\n", entry.Id)
		fmt.Printf("Command    : %s\n", entry.Command)
		fmt.Printf("Args       : %s\n", entry.Args)
		fmt.Printf("Start time : %s\n", entry.StartTs.AsTime().String())
		if entry.EndTs.AsTime().After(entry.StartTs.AsTime()) {
			fmt.Printf("End time   : %s\n", entry.EndTs.AsTime().String())
			fmt.Printf("Exit error : %s\n", entry.ExitError)
			fmt.Printf("Exit code  : %d\n", entry.ExitCode)
		}
	}
}

func (c *Client) createClient() (proto.JobServiceClient, error) {
	tlsCredentials, err := c.createTLSTransportCredentials()
	if err != nil {
		return nil, err
	}
	conn, err := grpc.Dial(c.config.ServerAddress,
		grpc.WithTransportCredentials(tlsCredentials),
	)
	if err != nil {
		c.logger.Errorf("failed connecting to %s: %w", c.config.ServerAddress, err)
		return nil, err
	}
	c.clientConn = conn

	return proto.NewJobServiceClient(conn), nil
}

func (c *Client) createTLSTransportCredentials() (credentials.TransportCredentials, error) {
	certPool, certificate, err := shared.LoadCertificates(c.config.CABundlePath,
		c.config.CertPath, c.config.CertKeyPath)
	if err != nil {
		c.logger.Errorf("Failed creating TLS credentials: %v", err)
		return nil, err
	}
	tlsConfig := &tls.Config{
		MinVersion:   tls.VersionTLS13,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		Certificates: []tls.Certificate{*certificate},
		RootCAs:      certPool,
	}

	return credentials.NewTLS(tlsConfig), nil
}
