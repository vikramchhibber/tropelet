package exec

import (
	"errors"
	"os/exec"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/troplet/pkg/exec/cgroups"
	"github.com/troplet/pkg/exec/mountfs"
	"github.com/troplet/pkg/exec/net"
)

type jobStateType string

const (
	jobStateUnknown    jobStateType = "unknown"
	jobStateInit                    = "init"
	jobStateRunning                 = "running"
	jobStateTerminated              = "terminated"
	jobStateFinished                = "finished"
)

type commandImpl struct {
	id         string
	name       string
	args       []string
	stdoutChan ReadChannel
	stderrChan ReadChannel
	timeout    time.Duration
	cgroupsMgr *cgroups.ControlGroupManager
	mountFSMgr *mountfs.MountFSManager
	netMgr     *net.NetworkManager
	cmd        *exec.Cmd
	waitGroup  sync.WaitGroup
	err        error
	lock       sync.Mutex
	jobState   jobStateType
}

func newCommand(name string, args []string, options ...CommandOption) (Command, error) {
	var err error

	// Every command is assigned a unique id
	id := uuid.New().String()

	// Initialize defaults and mandatory params
	execCmd := &commandImpl{id: id, name: name, args: args,
		timeout: 10 * time.Minute, jobState: jobStateInit}

	// Cleanup of incomplete initialization
	defer func() {
		if err != nil && execCmd != nil {
			execCmd.Finish()
		}
	}()

	// Read passed options
	for _, option := range options {
		if err = option(execCmd); err != nil {
			return nil, err
		}
	}

	// Set cgroup values
	if execCmd.cgroupsMgr != nil {
		if err = execCmd.cgroupsMgr.Set(); err != nil {
			return nil, err
		}
	}

	// Prepare filesystem under new root
	if execCmd.mountFSMgr != nil {
		if err = execCmd.mountFSMgr.Mount(); err != nil {
			return nil, err
		}
	}

	return execCmd, nil
}

func (c *commandImpl) setCPULimit(quotaMillSeconds, periodMillSeconds int64) error {
	c.getCGroupsMgr().NewCPUControlGroup(quotaMillSeconds, periodMillSeconds)

	return nil
}

func (c *commandImpl) setMemoryLimit(memKB int64) error {
	c.getCGroupsMgr().NewMemoryControlGroup(memKB)

	return nil
}

func (c *commandImpl) withTimeout(timeout time.Duration) error {
	c.timeout = timeout
	return nil
}

func (c *commandImpl) withStdoutChan(stdoutChan ReadChannel) error {
	c.stdoutChan = stdoutChan
	return nil
}

func (c *commandImpl) withStderrChan(stderrChan ReadChannel) error {
	c.stderrChan = stderrChan
	return nil
}

func (c *commandImpl) withNewRoot(newRoot string) error {
	c.mountFSMgr = mountfs.NewMountFSManager(newRoot)

	return nil
}

func (c *commandImpl) withNewNS() error {
	c.netMgr = net.NewNetworkManager()

	return nil
}

func (c *commandImpl) setIOLimit(deviceMajorNum, deviceMinorNum int32, rbps, wbps int64) error {
	c.getCGroupsMgr().NewIOManager(deviceMajorNum, deviceMinorNum, rbps, wbps)

	return nil
}

func (c *commandImpl) setState(expectedStates []jobStateType, newState jobStateType) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	if !slices.Contains(expectedStates, c.jobState) {
		return errors.New("invalid current state " + string(c.jobState))
	}
	c.jobState = newState

	return nil
}

func (c *commandImpl) getCGroupsMgr() *cgroups.ControlGroupManager {
	if c.cgroupsMgr == nil {
		c.cgroupsMgr = cgroups.NewControlGroupManager(c.id)
	}

	return c.cgroupsMgr
}
