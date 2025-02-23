package server

import (
	"context"
	"sync"

	"github.com/troplet/internal/shared"
	"github.com/troplet/pkg/proto"
)

type ClientInfo struct {
	// Map of job id (UUID) to job info
	jobInfoMap map[string]*JobInfo
}

type JobManager struct {
	logger shared.Logger
	// Map of client-id to client info
	clientInfoMap map[string]*ClientInfo

	// To protect the map
	lock sync.RWMutex
}

func NewJobManager(logger shared.Logger) *JobManager {
	return &JobManager{logger: logger, clientInfoMap: make(map[string]*ClientInfo)}
}

func (m *JobManager) Launch(ctx context.Context, clientID string,
	name string, args []string) string {
	jobInfo := NewJobInfo(name, args)
	jobID := jobInfo.Launch()

	m.lock.Lock()
	defer m.lock.Unlock()
	// Any failed or successful job execution needs to be recorded
	clientInfo, found := m.clientInfoMap[clientID]
	if !found {
		clientInfo = &ClientInfo{jobInfoMap: make(map[string]*JobInfo)}
		m.clientInfoMap[clientID] = clientInfo
	}
	clientInfo.jobInfoMap[jobID] = jobInfo

	return jobID
}

func (m *JobManager) Terminate(ctx context.Context, clientID string, jobID string) {
	m.lock.RLock()
	defer m.lock.RUnlock()
	jobInfo := m.getJobInfo(clientID, jobID)
	if jobInfo == nil {
		return
	}
	jobInfo.Finish()
}

func (m *JobManager) GetJobStatus(ctx context.Context, clientID string,
	jobID string) *proto.JobEntry {
	m.lock.RLock()
	defer m.lock.RUnlock()
	jobInfo := m.getJobInfo(clientID, jobID)
	if jobInfo == nil {
		return nil
	}

	return &jobInfo.info
}

func (m *JobManager) GetAllJobStatuses(ctx context.Context,
	clientID string) []*proto.JobEntry {
	m.lock.RLock()
	defer m.lock.RUnlock()
	clientInfo, found := m.clientInfoMap[clientID]
	if !found {
		m.logger.Errorf("Client info for id %s not found", clientID)
		return nil
	}
	jobs := []*proto.JobEntry{}
	for _, jobInfo := range clientInfo.jobInfoMap {
		jobs = append(jobs, &jobInfo.info)
	}

	return jobs
}

func (m *JobManager) Finish() {
	m.lock.Lock()
	defer m.lock.Unlock()
	// Cleanup all the jobs
	for _, clientInfo := range m.clientInfoMap {
		for _, jobInfo := range clientInfo.jobInfoMap {
			jobInfo.Finish()
		}
	}
}

func (m *JobManager) getJobInfo(clientID string, jobID string) *JobInfo {
	clientInfo, found := m.clientInfoMap[clientID]
	if !found {
		m.logger.Errorf("Client info for id %s not found", clientID)
		return nil
	}
	jobInfo, found := clientInfo.jobInfoMap[jobID]
	if !found {
		m.logger.Errorf("Job info for id %s not found for client %s",
			jobID, clientID)
		return nil
	}

	return jobInfo
}
