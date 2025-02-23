package client

import (
	"github.com/troplet/internal/shared"
)

type Config struct {
	ServerAddress string
	CABundlePath  string
	CertPath      string
	CertKeyPath   string
}

func (c Config) String() string {
	return "\nServer address   :" + c.ServerAddress +
		"\nCA bundle        :" + c.CABundlePath +
		"\nCert             :" + c.CertPath +
		"\nCert key         :" + c.CertKeyPath
}

type Client struct {
	config *Config
	logger shared.Logger
}

func NewClient(config *Config, logger shared.Logger) *Client {
	return &Client{config, logger}
}

func (c *Client) Finish() {
}

func (c *Client) ListJobs() {
	c.logger.Infof("Listing remote jobs")
}

func (c *Client) GetJobStatus() {
	c.logger.Infof("Listing remote jobs")
}

func (c *Client) LaunchJob(args []string) {
	c.logger.Infof("Exec. job: %v", args)
}

func (c *Client) TerminateJob(jobID string) {
	c.logger.Infof("Terminate job: %v", jobID)
}

func (c *Client) AttachJob(jobID string) {
	c.logger.Infof("Attach job: %v", jobID)
}
