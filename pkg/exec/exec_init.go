package exec

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"sync"

	"github.com/google/uuid"

	"github.com/troplet/pkg/exec/cgroups"
	"github.com/troplet/pkg/exec/mountfs"
)

// These are internal library states and may not directly
// correspond to the command's lifecycle states.
type cmdStateType string

const (
	cmdStateInit       cmdStateType = "init"
	cmdStateRunning                 = "running"
	cmdStateTerminated              = "terminated"
	cmdStateFinished                = "finished"
)

type commandImpl struct {
	id         string
	name       string
	args       []string
	stdoutChan ReadChannel
	stderrChan ReadChannel
	cgroupsMgr *cgroups.ControlGroupsManager
	mountFSMgr *mountfs.MountFSManager
	newNetNS   bool
	newPidNS   bool
	cmd        *exec.Cmd
	waitGroup  sync.WaitGroup
	err        error

	// This lock is meant to protect cmdState
	// comparison and transition
	lock     sync.Mutex
	cmdState cmdStateType
}

func newCommand(name string, args []string, options ...CommandOption) (Command, error) {
	var err error

	// Every command is assigned a unique id
	id := uuid.New().String()

	// Initialize defaults and mandatory params
	execCmd := &commandImpl{id: id, name: name, args: args,
		cmdState: cmdStateInit}

	// Cleanup of incomplete initialization
	defer func() {
		if err != nil && execCmd != nil {
			execCmd.Finish()
		}
	}()

	if execCmd.cgroupsMgr, err = cgroups.NewControlGroupsManager(id); err != nil {
		return nil, err
	}

	// Read passed options
	for _, option := range options {
		option(execCmd)
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

func (c *commandImpl) setCPULimit(quotaMillSeconds, periodMillSeconds int64) {
	c.cgroupsMgr.NewCPUControlGroup(quotaMillSeconds, periodMillSeconds)
}

func (c *commandImpl) setMemoryLimit(memKB int64) {
	c.cgroupsMgr.NewMemoryControlGroup(memKB)
}

func (c *commandImpl) setStdoutChan(stdoutChan ReadChannel) {
	c.stdoutChan = stdoutChan
}

func (c *commandImpl) setStderrChan(stderrChan ReadChannel) {
	c.stderrChan = stderrChan
}

func (c *commandImpl) setNewRootBase(newRootBase string) {
	// The new root for each process will be created under the root base
	// concatenated with a unique command ID, ensuring that multiple
	// commands do not share the same root.
	c.mountFSMgr = mountfs.NewMountFSManager(filepath.Join(newRootBase, c.id))
}

func (c *commandImpl) setNewNetNS() {
	c.newNetNS = true
}

func (c *commandImpl) setNewPidNS() {
	c.newPidNS = true
}

func (c *commandImpl) setIOLimits(deviceMajorNum, deviceMinorNum int32, rbps, wbps int64) {
	c.cgroupsMgr.NewIOControlGroup(deviceMajorNum, deviceMinorNum, rbps, wbps)
}

func (c *commandImpl) setState(expectedStates []cmdStateType, newState cmdStateType) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	if !slices.Contains(expectedStates, c.cmdState) {
		return fmt.Errorf("invalid current state " + string(c.cmdState))
	}
	c.cmdState = newState

	return nil
}
