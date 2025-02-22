package server

import (
	"crypto/tls"
	"fmt"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/troplet/internal/shared"
	"github.com/troplet/pkg/proto"
)

type Config struct {
	Address      string
	CABundlePath string
	CertPath     string
	CertKeyPath  string
}

func (c Config) String() string {
	return "\nAddress   :" + c.Address +
		"\nCA bundle :" + c.CABundlePath +
		"\nCert      :" + c.CertPath +
		"\nCert key  :" + c.CertKeyPath
}

type Server struct {
	config     *Config
	logger     shared.Logger
	grpcServer *grpc.Server
	proto.UnimplementedJobServiceServer
}

func NewServer(config *Config, logger shared.Logger) *Server {
	return &Server{config: config, logger: logger}
}

func (s *Server) Start() error {
	tlsCredentials, err := s.createTLSTransportCredentials()
	if err != nil {
		return err
	}
	s.grpcServer = grpc.NewServer(grpc.Creds(tlsCredentials))
	proto.RegisterJobServiceServer(s.grpcServer, s)
	listen, err := net.Listen("tcp", s.config.Address)
	if err != nil {
		return fmt.Errorf("server failed to listen on %s: %w",
			s.config.Address, err)
	}
	if err := s.grpcServer.Serve(listen); err != nil {
		return fmt.Errorf("server failed to serve: %w", err)
	}

	return nil
}

func (s *Server) Finish() {
	if s.grpcServer != nil {
		s.grpcServer.GracefulStop()
		s.logger.Infof("Server stopped!")
	}
}

func (s *Server) createTLSTransportCredentials() (credentials.TransportCredentials, error) {
	certPool, certificate, err := shared.LoadCertificates(s.config.CABundlePath,
		s.config.CertPath, s.config.CertKeyPath)
	if err != nil {
		return nil, err
	}
	tlsConfig := &tls.Config{
		MinVersion:               tls.VersionTLS13,
		PreferServerCipherSuites: true,
		CurvePreferences:         []tls.CurveID{tls.CurveP384, tls.CurveP521},
		ClientAuth:               tls.RequireAndVerifyClientCert,
		Certificates:             []tls.Certificate{*certificate},
		ClientCAs:                certPool,
	}

	return credentials.NewTLS(tlsConfig), nil
}
