package main

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/troplet/internal/server"
	"github.com/troplet/internal/shared"
)

func main() {
	var address, certsDir string
	// Root command starts the server
	var rootCmd = &cobra.Command{
		Run: func(cmd *cobra.Command, args []string) {
			config := server.Config{
				Address:      address,
				CABundlePath: filepath.Join(certsDir, shared.ServerDefaultCAFile),
				CertPath:     filepath.Join(certsDir, shared.ServerDefaultCertFile),
				CertKeyPath:  filepath.Join(certsDir, shared.ServerDefaultCertKeyFile),
			}
			logger := shared.CreateLogger()
			defer logger.Sync()
			jobManager, err := server.NewJobManager(logger)
			if err != nil {
				return
			}
			defer jobManager.Finish()
			logger.Infof("Starting server with config: " + config.String())
			server := server.NewServer(&config, logger, jobManager)
			// Shutdown server on SIGINT or SIGTERM
			shared.RegisterShutdownSigCallback(func() {
				server.Finish()
			})
			if err := server.Start(); err != nil {
				logger.Errorf(err.Error())
			}
		},
	}
	// Persistent CLI flags
	rootCmd.PersistentFlags().StringVarP(&address, "address", "a",
		shared.ServerDefaultListenAddress, "Server listen address in [address:port] format")
	// Certificates directory
	rootCmd.PersistentFlags().StringVarP(&certsDir, "certs-dir", "c",
		shared.ServerDefaultCertsDir, "Path of directory where certificates are located")

	if err := rootCmd.Execute(); err != nil {
		fmt.Errorf("failed executing server: %w", err)
	}
}
