package tctl

import (
	"github.com/spf13/cobra"
	"github.com/troplet/internal/shared"
)

func (c *Client) Execute() error {
	// Root command list remote jobs
	var rootCmd = &cobra.Command{
		Run: func(cmd *cobra.Command, args []string) {
			// Late initilization as at this point we
			// are ready with user passed config args
			c.init()
			defer c.finish()
			c.listJobs()
		},
	}
	var listCmd = &cobra.Command{
		Use:   "list",
		Short: "List remote jobs",
		Long:  "List remote jobs details",
		Args:  cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			c.init()
			defer c.finish()
			c.listJobs()
		},
	}
	var execCmd = &cobra.Command{
		Use:   "exec",
		Short: "Executes command on server",
		Long:  "The first argument is name of command and the rest are the command arguments",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			c.init()
			defer c.finish()
			c.execJob(args)
		},
	}
	var terminateCmd = &cobra.Command{
		Use:   "terminate",
		Short: "Terminates remote running job",
		Long:  "The command takes the job identifier",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			c.init()
			defer c.finish()
			c.terminateJob(args[0])
		},
	}
	var attachCmd = &cobra.Command{
		Use:   "attach",
		Short: "Attaches to remote running job",
		Long:  "The command takes the job identifier",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			c.init()
			defer c.finish()
			c.attachJob(args[0])
		},
	}

	rootCmd.AddCommand(listCmd, execCmd, terminateCmd, attachCmd)
	// Persistent CLI flags applicable for all the commands
	// Server address
	rootCmd.PersistentFlags().StringVarP(&c.serverAddress, "server-address", "a",
		shared.ServerDefaultAddress, "Server address in [address:port] format")
	// Certificates directory path
	rootCmd.PersistentFlags().StringVarP(&c.certificatesDirPath, "certs-path", "s",
		"./certs/client", "Path of client certificates directory")

	return rootCmd.Execute()
}
