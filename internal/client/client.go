package client

import (
	"context"
	"crypto/tls"

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
	c.logger.Infof(">>%v", resp)
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
	c.logger.Infof("%v", resp)
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
	c.logger.Infof("%v", resp)
}

func (c *Client) TerminateJob(jobID string) {
	client, err := c.createClient()
	if err != nil {
		return
	}
	resp, err := client.TerminateJob(context.Background(),
		&proto.TerminateJobRequest{Id: jobID})
	if err != nil {
		c.logger.Errorf("Failed terminating job: %v", err)
		return
	}
	c.logger.Infof("%v", resp)
}

func (c *Client) AttachJob(jobID string) {
	c.logger.Infof("Attach job: %v", jobID)
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
