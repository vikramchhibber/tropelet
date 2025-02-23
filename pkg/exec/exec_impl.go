package exec

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"slices"
	"sync"
	"syscall"

	"github.com/google/uuid"

	"github.com/troplet/pkg/exec/cgroups"
	"github.com/troplet/pkg/exec/mountfs"
)

// These are internal library states and may not directly
// correspond to the command's lifecycle states.
type cmdStateType string

const (
	cmdStateInit       cmdStateType = "init"
	cmdStateRunning    cmdStateType = "running"
	cmdStateTerminated cmdStateType = "terminated"
	cmdStateFinished   cmdStateType = "finished"
)

type commandImpl struct {
	id         string
	name       string
	args       []string
	stdoutChan ReadChannel
	stderrChan ReadChannel
	cgroupsMgr *cgroups.ControlGroupsManager
	mountFSMgr *mountfs.MountFSManager
	useNetNS   bool
	usePIDNS   bool
	cmd        *exec.Cmd
	err        error

	// This lock is meant to protect cmdState
	// comparison and transition
	lock     sync.Mutex
	cmdState cmdStateType
}

func (c *commandImpl) GetID() string {
	return c.id
}

func (c *commandImpl) String() string {
	return fmt.Sprintf("%s, %s %v %s",
		c.id, c.name, c.args, c.cmdState)
}

func (c *commandImpl) Execute(ctx context.Context) {
	if err := c.setState([]cmdStateType{cmdStateInit},
		cmdStateRunning); err != nil {
		return
	}
	c.err = c.execute(ctx)
	if err := c.setState([]cmdStateType{cmdStateRunning},
		cmdStateTerminated); err != nil {
		return
	}
}

func (c *commandImpl) IsTerminated() bool {
	c.lock.Lock()
	defer c.lock.Unlock()

	return c.cmdState == cmdStateTerminated ||
		c.cmdState == cmdStateFinished
}

func (c *commandImpl) GetExitError() error {
	return c.err
}

func (c *commandImpl) GetExitCode() int {
	if c.cmd != nil && c.cmd.ProcessState != nil {
		return c.cmd.ProcessState.ExitCode()
	}

	return 1
}

func (c *commandImpl) SendTermSignal() {
	c.lock.Lock()
	defer c.lock.Unlock()

	// Send term signal to all the process in the group
	c.sendSignalToGroup(syscall.SIGTERM)
}

func (c *commandImpl) Finish() {
	// Finish can be called in any of these states
	// except "finished"
	if err := c.setState([]cmdStateType{cmdStateInit,
		cmdStateRunning, cmdStateTerminated},
		cmdStateFinished); err != nil {
		// Already in finished state
		return
	}

	// Kill all the processes in the group
	c.sendSignalToGroup(syscall.SIGKILL)

	if c.cgroupsMgr != nil {
		c.cgroupsMgr.Finish()
	}
	if c.mountFSMgr != nil {
		c.mountFSMgr.Finish()
	}
}

func newCommand(name string, args []string, options ...CommandOption) (Command, error) {
	var err error

	// Every command is assigned a unique id
	id := uuid.NewString()

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
	if err = execCmd.cgroupsMgr.Set(); err != nil {
		return nil, err
	}

	// Prepare filesystem under new root.  Assigned by options.
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

func (c *commandImpl) setNewRootBase(newRootBase string) {
	// The new root for each process will be created under the passed root base
	// concatenated with a unique command ID, ensuring that multiple
	// commands do not share the same root.
	c.mountFSMgr = mountfs.NewMountFSManager(filepath.Join(newRootBase, c.id))
}

func (c *commandImpl) setIOLimits(deviceMajorNum, deviceMinorNum int32, rbps, wbps int64) {
	c.cgroupsMgr.NewIOControlGroup(deviceMajorNum, deviceMinorNum, rbps, wbps)
}

func (c *commandImpl) setState(expectedStates []cmdStateType, newState cmdStateType) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	if !slices.Contains(expectedStates, c.cmdState) {
		return fmt.Errorf("invalid current state %s", c.cmdState)
	}
	c.cmdState = newState

	return nil
}

func (c *commandImpl) execute(ctx context.Context) error {
	var waitGroup sync.WaitGroup
	c.cmd = exec.CommandContext(ctx, c.name, c.args...)
	if c.stdoutChan != nil {
		stdoutPipe, err := c.cmd.StdoutPipe()
		if err != nil {
			return err
		}
		waitGroup.Add(1)
		go func() {
			c.readPipe(c.stdoutChan, stdoutPipe)
			waitGroup.Done()
		}()
	}
	if c.stderrChan != nil {
		stderrPipe, err := c.cmd.StderrPipe()
		if err != nil {
			return err
		}
		waitGroup.Add(1)
		go func() {
			c.readPipe(c.stderrChan, stderrPipe)
			waitGroup.Done()
		}()
	}
	c.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if c.usePIDNS {
		c.cmd.SysProcAttr.Cloneflags |= syscall.CLONE_NEWPID
	}
	if c.useNetNS {
		c.cmd.SysProcAttr.Cloneflags |= syscall.CLONE_NEWNET
		c.cmd.SysProcAttr.Unshareflags |= syscall.CLONE_NEWNET
	}
	if c.mountFSMgr != nil {
		c.cmd.SysProcAttr.Chroot = c.mountFSMgr.GetMountRoot()
		c.cmd.Dir = "/"

		c.cmd.SysProcAttr.Cloneflags |= syscall.CLONE_NEWNS
		c.cmd.SysProcAttr.Unshareflags |= syscall.CLONE_NEWNS
	}

	// Pass control-groups directory FD to the process
	if c.cgroupsMgr != nil {
		var err error
		if c.cmd.SysProcAttr.CgroupFD, err =
			c.cgroupsMgr.GetControlGroupsFD(); err != nil {
			return err
		}
		c.cmd.SysProcAttr.Cloneflags |= syscall.CLONE_INTO_CGROUP
		c.cmd.SysProcAttr.UseCgroupFD = true
	}

	// Execute command
	if err := c.cmd.Start(); err != nil {
		return err
	}

	// Wait for the process to terminate
	err := c.cmd.Wait()

	// We will wait for all the goroutines to finish
	waitGroup.Wait()

	return err
}

func (c *commandImpl) readPipe(dst ReadChannel, src io.Reader) {
	for {
		// TODO: Config candidate
		// TODO: This has GC overhead
		buf := make([]byte, 128)
		n, err := io.ReadFull(src, buf)
		if n != 0 {
			dst <- buf[:n]
		}
		if err != nil {
			close(dst)
			break
		}
	}
}

func (c *commandImpl) sendSignalToGroup(sig syscall.Signal) {
	if c.cmd != nil && c.cmd.Process != nil {
		if pgid, err := syscall.Getpgid(c.cmd.Process.Pid); err == nil {
			syscall.Kill(-pgid, sig)
		}
	}
}
