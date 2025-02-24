// Package exec provides convenient APIs for executing commands
// with optional features, including PID isolation, network isolation,
// a new root, and cgroup-based limits on CPU, memory, or I/O.
package exec

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
	"syscall"

	"github.com/troplet/pkg/exec/cgroups"
	"github.com/troplet/pkg/exec/mountfs"
)

type cmdStateType string

// Command struct holds the configurable and internal state of the command
type Command struct {
	// State variables initialized during object creation
	name       string
	args       []string
	stdoutChan ReadChannel
	stderrChan ReadChannel
	cgroupsMgr *cgroups.ControlGroupsManager
	mountFSMgr *mountfs.MountFSManager
	useNetNS   bool
	usePIDNS   bool

	// Internal state variables
	id  string
	cmd *exec.Cmd
	// These are exit error and code of command once it
	// has been terminated
	exitError error
	exitCode  int
	// This lock is meant to protect cmdState
	// comparison and transition, setting of exitError and exitCode
	lock     sync.RWMutex
	cmdState cmdStateType
}

// Channel type to send stdout or stderror data to application
type ReadChannel chan []byte

// Command options to construct the command
type CommandOption func(*Command)

// Option to register stdout channel
func WithStdoutChan(stdoutChan ReadChannel) CommandOption {
	return func(c *Command) {
		c.stdoutChan = stdoutChan
	}
}

// Option to register stderr channel
func WithStderrChan(stderrChan ReadChannel) CommandOption {
	return func(c *Command) {
		c.stderrChan = stderrChan
	}
}

// Option to set CPU cgroups limit
func WithCPULimit(quotaMillSeconds, periodMillSeconds int64) CommandOption {
	return func(c *Command) {
		c.setCPULimit(quotaMillSeconds, periodMillSeconds)
	}
}

// Option to set memory cgroups limit
func WithMemoryLimit(memKB int64) CommandOption {
	return func(c *Command) {
		c.setMemoryLimit(memKB)
	}
}

// Option to set IO cgroups limit
func WithIOLimits(deviceMajorNum, deviceMinorNum int32, rbps, wbps int64) CommandOption {
	return func(c *Command) {
		c.setIOLimits(deviceMajorNum, deviceMinorNum, rbps, wbps)
	}
}

// Option to set new root-base. Command's new root directory
// with name "id" is created under this base.
func WithNewRootBase(newRootBase string) CommandOption {
	return func(c *Command) {
		c.setNewRootBase(newRootBase)
	}
}

// Options to isolate network
func WithUseNetNS() CommandOption {
	return func(c *Command) {
		c.useNetNS = true
	}
}

// Options to isolate PID
func WithUsePIDNS() CommandOption {
	return func(c *Command) {
		c.usePIDNS = true
	}
}

// Returns new command with given name, args and options.
// The name is mandatory argument.
func NewCommand(name string, args []string, options ...CommandOption) (*Command, error) {
	return newCommand(name, args, options...)
}

// Unique identifier of this command.
// This identifier is used to create unique new root,
// and cgroups directory
func (c *Command) GetID() string {
	return c.id
}

// String representation of this command.
func (c *Command) String() string {
	return fmt.Sprintf("%s, %s %v", c.id, c.name, c.args)
}

// Executes this command. This call blocks till the
// command has terminated.
func (c *Command) Execute(ctx context.Context) {
	// Move to running state
	changeStateToRunning := func() bool {
		c.lock.Lock()
		defer c.lock.Unlock()
		return c.transitionState(cmdStateRunning)
	}
	if !changeStateToRunning() {
		return
	}

	err := c.execute(ctx)

	// Move to terminated state and collect exit code and error
	changeStateToTerminated := func() {
		c.lock.Lock()
		defer c.lock.Unlock()
		c.exitError = err
		if c.cmd != nil && c.cmd.ProcessState != nil {
			c.exitCode = c.cmd.ProcessState.ExitCode()
		}
		// Not sure if this state transaciton fails.
		// This can happen if the user called Finish before
		// the command has terminated
		c.transitionState(cmdStateTerminated)
	}
	changeStateToTerminated()
}

// Checks if the command has terminated
func (c *Command) IsTerminated() bool {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.cmdState == cmdStateTerminated
}

// Gets exit error of terminated or failed command.
// The call will fail if the command is not in terminated state.
func (c *Command) GetExitError() (error, error) {
	c.lock.RLock()
	defer c.lock.RUnlock()
	if c.cmdState != cmdStateTerminated {
		return nil, fmt.Errorf("invalid command state")
	}

	return c.exitError, nil
}

// Gets exit code of terminated or failed command.
// The call will fail if the command is not in terminated state.
func (c *Command) GetExitCode() (int, error) {
	c.lock.RLock()
	defer c.lock.RUnlock()
	if c.cmdState != cmdStateTerminated {
		return 0, fmt.Errorf("invalid command state")
	}

	return c.exitCode, nil
}

// Tries terminating the command gracefully by sending
// SIGTERM signal if running. Will return error in case
// the command is not running or kill fails.
func (c *Command) SendTermSignal() error {
	return c.sendSignalToGroup(syscall.SIGTERM)
}

// Tries terminating the command forcefully by sending
// SIGKILL signal if running. Will return error in case
// the command is not running or kill fails.
func (c *Command) SendKillSignal() error {
	return c.sendSignalToGroup(syscall.SIGKILL)
}

// Abruptly terminates the command by sending SIGKILL
// signal and performs necessary cleanups.
func (c *Command) Finish() {
	c.finish()
}
