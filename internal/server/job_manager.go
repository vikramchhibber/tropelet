package server

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/troplet/internal/shared"
	"github.com/troplet/pkg/proto"
)

const (
	// TODO: Configuration candidates
	memKB             = 16 * 1024       // 16MB
	rbps              = 4 * 1024 * 1024 // 4MB
	wbps              = 1024 * 1024     // 1MB
	quotaMillSeconds  = 100
	periodMillSeconds = 1000
	rootBase          = "./"
)

type ClientInfo struct {
	// Map of job id (UUID) to job info
	jobInfoMap map[string]*JobInfo
}

type JobManager struct {
	logger shared.Logger
	// Map of client-id to client info
	clientInfoMap  map[string]*ClientInfo
	deviceMajorNum int32
	deviceMinorNum int32
	// To protect the map
	lock sync.RWMutex
}

func NewJobManager(logger shared.Logger) (*JobManager, error) {
	mount, err := getFilesystemMount(rootBase)
	if err != nil {
		return nil, err
	}
	deviceMajorNum, deviceMinorNum, err := getDeviceNumbers(mount)
	if err != nil {
		return nil, err
	}
	logger.Infof("Using mount %s, device-major-num: %d, device-minor-num: %d",
		mount, deviceMajorNum, deviceMinorNum)

	return &JobManager{logger: logger, clientInfoMap: make(map[string]*ClientInfo),
		deviceMajorNum: deviceMajorNum, deviceMinorNum: deviceMinorNum}, nil
}

func (m *JobManager) Launch(ctx context.Context, clientID string,
	name string, args []string) string {
	jobInfo := NewJobInfo(m.logger, name, args)
	jobID := jobInfo.Launch(rootBase, quotaMillSeconds, periodMillSeconds,
		memKB, rbps, wbps, m.deviceMajorNum, m.deviceMinorNum)

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
	jobInfo := m.getJobInfo(clientID, jobID)
	if jobInfo == nil {
		return
	}
	jobInfo.Terminate()
}

func (m *JobManager) GetJobStatus(ctx context.Context,
	clientID string, jobID string) (*proto.JobEntry, error) {
	jobInfo := m.getJobInfo(clientID, jobID)
	if jobInfo == nil {
		return nil, fmt.Errorf("job id %s not found", jobID)
	}

	return &jobInfo.info, nil
}

func (m *JobManager) Attach(ctx context.Context, clientID string, jobID string,
	stdoutChan, stderrChan chan *proto.JobStreamEntry) (uint64, error) {
	jobInfo := m.getJobInfo(clientID, jobID)
	if jobInfo == nil {
		return 0, fmt.Errorf("job id %s not found", jobID)
	}

	return jobInfo.Attach(stdoutChan, stderrChan)
}

func (m *JobManager) Detach(ctx context.Context, clientID string,
	jobID string, subscriberID uint64) error {
	jobInfo := m.getJobInfo(clientID, jobID)
	if jobInfo == nil {
		return fmt.Errorf("job id %s not found", jobID)
	}
	jobInfo.Detach(subscriberID)

	return nil
}

func (m *JobManager) GetAllJobStatuses(ctx context.Context,
	clientID string) []*proto.JobEntry {
	m.lock.RLock()
	clientInfo, found := m.clientInfoMap[clientID]
	if !found {
		m.logger.Errorf("Client info for id %s not found", clientID)
		m.lock.RUnlock()
		return nil
	}
	m.lock.RUnlock()
	jobs := []*proto.JobEntry{}
	for _, jobInfo := range clientInfo.jobInfoMap {
		jobs = append(jobs, &jobInfo.info)
	}

	return jobs
}

func (m *JobManager) Finish() {
	m.lock.RLock()
	defer m.lock.RUnlock()
	// Cleanup all the jobs
	for _, clientInfo := range m.clientInfoMap {
		for _, jobInfo := range clientInfo.jobInfoMap {
			jobInfo.Finish()
		}
	}
}

func (m *JobManager) getJobInfo(clientID string, jobID string) *JobInfo {
	m.lock.RLock()
	defer m.lock.RUnlock()
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

func getFilesystemMount(path string) (string, error) {
	// Get absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path on %s: %w", path, err)
	}

	// Read /proc/mounts
	file, err := os.Open("/proc/mounts")
	if err != nil {
		return "", fmt.Errorf("failed to read /proc/mounts: %w", err)
	}
	defer file.Close()

	// Find the most specific mount point
	var longestMatch string
	maxLen := -1

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		// Example format:
		// sysfs /sys sysfs rw,nosuid,nodev,noexec,relatime 0 0
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}
		mountPoint := strings.TrimSpace(fields[1])

		// Check if this mount point is a parent of our path
		rel, err := filepath.Rel(mountPoint, absPath)
		if err != nil {
			continue
		}

		// Skip if path goes outside mount point
		if strings.HasPrefix(rel, "..") {
			continue
		}

		// Keep track of the longest (most specific) mount point
		if len(mountPoint) > maxLen {
			maxLen = len(mountPoint)
			longestMatch = mountPoint
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("error reading /proc/mounts: %w", err)
	}

	if maxLen == -1 {
		return "", fmt.Errorf("no filesystem found for %s", path)
	}

	return longestMatch, nil
}

func getDeviceNumbers(dirPath string) (int32, int32, error) {
	info, err := os.Stat(dirPath)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to stat directory %s: %w", dirPath, err)
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, fmt.Errorf("failed to get system stat info for %s", dirPath)
	}

	// Get device numbers from the filesystem device ID
	major := int32((stat.Dev >> 8) & 0xfff)
	minor := int32(stat.Dev & 0xff)

	return major, minor, nil
}
