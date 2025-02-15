package tctl

import (
	"github.com/troplet/internal/shared"
)

type Client struct {
	serverAddress       string
	certificatesDirPath string
	logger              shared.Logger
}

func (c *Client) init() {
	c.logger = shared.InitializeLogger()
	c.logger.Infof("Using server-address: %s, certs-dir: %s",
		c.serverAddress, c.certificatesDirPath)
}

func (c *Client) finish() {
	if c.logger != nil {
		c.logger.Sync()
	}
}

func (c *Client) listJobs() {
	c.logger.Infof("Listing remote jobs")
}

func (c *Client) execJob(args []string) {
	c.logger.Infof("Exec. job: %v", args)
}

func (c *Client) terminateJob(jobID string) {
	c.logger.Infof("Terminate job: %v", jobID)
}

func (c *Client) attachJob(jobID string) {
	c.logger.Infof("Attach job: %v", jobID)
}
