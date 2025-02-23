// Package exec provides convenient APIs for executing commands
// with optional features, including PID isolation, network isolation,
// a new root, and cgroup-based limits on CPU, memory, or I/O.
package exec

import (
	"context"
)

// Command interface represents contract between this package
// implementation and APIs
type Command interface {
	// Unique identifier of this command.
	// This identifier is used to create unique new root,
	// and cgroups directory
	GetID() string
	// String representation of this command.
	String() string
	// Executes this command. This call blocks till the
	// command has terminated.
	Execute(ctx context.Context)
	// Checks if the command has terminated.
	IsTerminated() bool
	// Gets exit error of terminated or failed command.
	GetExitError() error
	// Gets exit code of terminated or failed command.
	GetExitCode() int
	// Tries terminating the command gracefully by sending
	// SIGTERM signal if running.
	SendTermSignal()
	// Abruptly terminates the command by sending SIGKILL
	// signal and performs necessary cleanups.
	Finish()
}

// Channel type to send stdout or stderror data to application
type ReadChannel chan []byte

// Command options to construct the command
type CommandOption func(*commandImpl)

// Returns new command with given name, args and options.
// The name is mandatory argument.
func NewCommand(name string, args []string, options ...CommandOption) (Command, error) {
	return newCommand(name, args, options...)
}

// Option to register stdout channel
func WithStdoutChan(stdoutChan ReadChannel) CommandOption {
	return func(c *commandImpl) {
		c.stdoutChan = stdoutChan
	}
}

// Option to register stderr channel
func WithStderrChan(stderrChan ReadChannel) CommandOption {
	return func(c *commandImpl) {
		c.stderrChan = stderrChan
	}
}

// Option to set CPU cgroups limit
func WithCPULimit(quotaMillSeconds, periodMillSeconds int64) CommandOption {
	return func(c *commandImpl) {
		c.setCPULimit(quotaMillSeconds, periodMillSeconds)
	}
}

// Option to set memory cgroups limit
func WithMemoryLimit(memKB int64) CommandOption {
	return func(c *commandImpl) {
		c.setMemoryLimit(memKB)
	}
}

// Option to set IO cgroups limit
func WithIOLimits(deviceMajorNum, deviceMinorNum int32, rbps, wbps int64) CommandOption {
	return func(c *commandImpl) {
		c.setIOLimits(deviceMajorNum, deviceMinorNum, rbps, wbps)
	}
}

// Option to set new root-base. Command's new root directory
// with name "id" is created under this base.
func WithNewRootBase(newRootBase string) CommandOption {
	return func(c *commandImpl) {
		c.setNewRootBase(newRootBase)
	}
}

// Options to isolate network
func WithUseNetNS() CommandOption {
	return func(c *commandImpl) {
		c.useNetNS = true
	}
}

// Options to isolate PID
func WithUsePIDNS() CommandOption {
	return func(c *commandImpl) {
		c.usePIDNS = true
	}
}
