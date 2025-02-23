package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"sort"

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
	// Must not log sensitive credentials
	return "\nAddress   :" + c.Address +
		"\nCA bundle :" + c.CABundlePath +
		"\nCert      :" + c.CertPath +
		"\nCert key  :" + c.CertKeyPath
}

type Server struct {
	config     *Config
	logger     shared.Logger
	jobManager *JobManager
	grpcServer *grpc.Server
	proto.UnimplementedJobServiceServer
}

func NewServer(config *Config, logger shared.Logger, jobManager *JobManager) *Server {
	return &Server{config: config, logger: logger, jobManager: jobManager}
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

func (s *Server) ListJobs(ctx context.Context,
	req *proto.ListJobsRequest) (*proto.ListJobsResponse, error) {
	jobs := s.jobManager.GetAllJobStatuses(ctx, "abc")
	// Sort by latest jobs first
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].StartTs.AsTime().After(jobs[j].StartTs.AsTime())
	})

	return &proto.ListJobsResponse{Jobs: jobs}, nil
}

func (s *Server) GetJobStatus(ctx context.Context,
	req *proto.GetJobStatusRequest) (*proto.GetJobStatusResponse, error) {
	job := s.jobManager.GetJobStatus(ctx, "abc", req.Id)

	return &proto.GetJobStatusResponse{Job: job}, nil
}

func (s *Server) LaunchJob(ctx context.Context,
	req *proto.LaunchJobRequest) (*proto.LaunchJobResponse, error) {
	id := s.jobManager.Launch(ctx, "abc", req.Command, req.Args)

	return &proto.LaunchJobResponse{Id: id}, nil
}

func (s *Server) AttachJob(req *proto.AttachJobRequest, stream proto.JobService_AttachJobServer) error {
	return nil
}

func (s *Server) TerminateJob(ctx context.Context,
	req *proto.TerminateJobRequest) (*proto.TerminateJobResponse, error) {
	s.jobManager.Terminate(ctx, "abc", req.Id)

	return &proto.TerminateJobResponse{}, nil
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
