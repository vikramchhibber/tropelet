package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"sort"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"

	"github.com/troplet/internal/shared"
	"github.com/troplet/pkg/proto"
)

type Config struct {
	Address      string
	CABundlePath string
	CertPath     string
	CertKeyPath  string
}

const cnCtxKey = "CommonName"

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
	s.grpcServer = grpc.NewServer(grpc.Creds(tlsCredentials),
		grpc.UnaryInterceptor(s.setPeerCertCNInCtx))
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
	jobs := s.jobManager.GetAllJobStatuses(ctx, s.getCNFromCtx(ctx))
	// Sort by latest jobs first
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].StartTs.AsTime().After(jobs[j].StartTs.AsTime())
	})

	return &proto.ListJobsResponse{Jobs: jobs}, nil
}

func (s *Server) GetJobStatus(ctx context.Context,
	req *proto.GetJobStatusRequest) (*proto.GetJobStatusResponse, error) {
	job, err := s.jobManager.GetJobStatus(ctx, s.getCNFromCtx(ctx), req.Id)
	if err != nil {
		return nil, err
	}

	return &proto.GetJobStatusResponse{Job: job}, nil
}

func (s *Server) LaunchJob(ctx context.Context,
	req *proto.LaunchJobRequest) (*proto.LaunchJobResponse, error) {
	id := s.jobManager.Launch(ctx, s.getCNFromCtx(ctx), req.Command, req.Args)

	return &proto.LaunchJobResponse{Id: id}, nil
}

func (s *Server) AttachJob(req *proto.AttachJobRequest, stream proto.JobService_AttachJobServer) error {
	stdoutChan := make(chan *proto.JobStreamEntry)
	stderrChan := make(chan *proto.JobStreamEntry)
	ctx := stream.Context()
	commonName, err := s.getCNFromContext(ctx)
	if err != nil {
		return err
	}
	subscriberID, err := s.jobManager.Attach(ctx, commonName, req.Id, stdoutChan, stderrChan)
	if err != nil {
		return err
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		numChannels := 2
		for numChannels > 0 {
			select {
			case entry, ok := <-stdoutChan:
				if !ok {
					stdoutChan = nil
					numChannels--
					break
				}
				if err := stream.Send(&proto.AttachJobResponse{StreamEntry: entry}); err != nil {
					// Send detach event to close channels
					s.jobManager.Detach(ctx, commonName, req.Id, subscriberID)
				}
			case entry, ok := <-stderrChan:
				if !ok {
					stderrChan = nil
					numChannels--
					break
				}
				if err := stream.Send(&proto.AttachJobResponse{StreamEntry: entry}); err != nil {
					// Send detach event to close channels
					s.jobManager.Detach(ctx, commonName, req.Id, subscriberID)
				}
			}
		}
		// If we are here, the channels have closed
	}()
	go func() {
		<-ctx.Done()
		// We are here as the client terminated the connection.
		// We should detach to close the client channels
		s.jobManager.Detach(ctx, commonName, req.Id, subscriberID)
	}()

	// We will wait for the client channels to close
	wg.Wait()

	return nil
}

func (s *Server) TerminateJob(ctx context.Context,
	req *proto.TerminateJobRequest) (*proto.TerminateJobResponse, error) {
	err := s.jobManager.Terminate(ctx, s.getCNFromCtx(ctx), req.Id)
	if err != nil {
		return nil, err
	}

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

func (s *Server) setPeerCertCNInCtx(ctx context.Context, req interface{},
	info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	commonName, err := s.getCNFromContext(ctx)
	if err != nil {
		return nil, err
	}

	return handler(context.WithValue(ctx, cnCtxKey, commonName), req)
}

func (s *Server) getCNFromContext(ctx context.Context) (string, error) {
	peer, ok := peer.FromContext(ctx)
	if !ok {
		return "", fmt.Errorf("failed getting remote peer")
	}
	tlsInfo, ok := peer.AuthInfo.(credentials.TLSInfo)
	if !ok || tlsInfo.State.VerifiedChains == nil {
		return "", fmt.Errorf("failed getting TLS info from remote peer")
	}
	commonName := tlsInfo.State.VerifiedChains[0][0].Subject.CommonName
	if commonName == "" {
		return "", fmt.Errorf("invalid CN received")
	}

	return commonName, nil
}

func (s *Server) getCNFromCtx(ctx context.Context) string {
	if commonName, ok := ctx.Value(cnCtxKey).(string); ok {
		return commonName
	}

	// This should never happen
	return ""
}
