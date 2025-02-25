package main

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/troplet/internal/client"
	"github.com/troplet/internal/shared"
)

func main() {
	var serverAddress, certsDir string
	// Root command list remote jobs by default
	var rootCmd = &cobra.Command{
		Run: func(cmd *cobra.Command, args []string) {
			executeCommand(serverAddress, certsDir, func(c *client.Client) {
				c.ListJobs()
			})
		},
	}
	var listCmd = &cobra.Command{
		Use:   "list",
		Short: "List remote jobs",
		Long:  "List remote jobs",
		Args:  cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			executeCommand(serverAddress, certsDir, func(c *client.Client) {
				c.ListJobs()
			})
		},
	}
	var getStatusCmd = &cobra.Command{
		Use:   "status",
		Short: "Gets remote job status",
		Long:  "Gets remote job status",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			executeCommand(serverAddress, certsDir, func(c *client.Client) {
				c.GetJobStatus(args[0])
			})
		},
	}
	var launchCmd = &cobra.Command{
		Use:   "launch",
		Short: "Starts job on server",
		Long:  "Starts job on server",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			executeCommand(serverAddress, certsDir, func(c *client.Client) {
				c.LaunchJob(args[0], args[1:])
			})
		},
	}
	var terminateCmd = &cobra.Command{
		Use:   "terminate",
		Short: "Terminates remote running job",
		Long:  "Terminates remote running job",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			executeCommand(serverAddress, certsDir, func(c *client.Client) {
				c.TerminateJob(args[0])
			})
		},
	}
	var attachCmd = &cobra.Command{
		Use:   "attach",
		Short: "Attaches to remote running job and gets its standard error and output",
		Long:  "Attaches to remote running job and gets its standard error and output",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			executeCommand(serverAddress, certsDir, func(c *client.Client) {
				c.AttachJob(args[0])
			})
		},
	}

	rootCmd.AddCommand(listCmd, getStatusCmd, launchCmd, terminateCmd, attachCmd)
	// Persistent CLI flags applicable for all the commands
	// Server address
	rootCmd.PersistentFlags().StringVarP(&serverAddress, "server-address", "s",
		shared.ClientDefaultConnectAddress, "Server address in [address:port] format")
	// Certificates directory
	rootCmd.PersistentFlags().StringVarP(&certsDir, "certs-dir", "c",
		shared.ClientDefaultCertsDir, "Path of directory where certificates are located")

	if err := rootCmd.Execute(); err != nil {
		fmt.Errorf("failed executing command: %w", err)
	}
}

func executeCommand(serverAddress, certsDir string, cmdCB func(client *client.Client)) {
	config := client.Config{
		ServerAddress: serverAddress,
		CABundlePath:  filepath.Join(certsDir, shared.ClientDefaultCAFile),
		CertPath:      filepath.Join(certsDir, shared.ClientDefaultCertFile),
		CertKeyPath:   filepath.Join(certsDir, shared.ClientDefaultCertKeyFile),
	}
	logger := shared.CreateLogger()
	defer logger.Sync()
	logger.Infof("Starting client with config: " + config.String())
	c := client.NewClient(&config, logger)
	if cmdCB != nil {
		cmdCB(c)
	}
}
