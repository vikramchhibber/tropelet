package exec

import (
	"context"
)

type Command interface {
	GetID() string
	String() string
	Execute(ctx context.Context) error
	IsTerminated() bool
	GetExitError() error
	GetExitCode() int
	SendTermSignal()
	Finish()
}

type ReadChannel chan []byte

type CommandOption func(*commandImpl)

func NewCommand(name string, args []string, options ...CommandOption) (Command, error) {
	return newCommand(name, args, options...)
}

func WithStdoutChan(stdoutChan ReadChannel) CommandOption {
	return func(c *commandImpl) {
		c.setStdoutChan(stdoutChan)
	}
}

func WithStderrChan(stderrChan ReadChannel) CommandOption {
	return func(c *commandImpl) {
		c.setStderrChan(stderrChan)
	}
}

func WithCPULimit(quotaMillSeconds, periodMillSeconds int64) CommandOption {
	return func(c *commandImpl) {
		c.setCPULimit(quotaMillSeconds, periodMillSeconds)
	}
}

func WithMemoryLimit(memKB int64) CommandOption {
	return func(c *commandImpl) {
		c.setMemoryLimit(memKB)
	}
}

func WithIOLimits(deviceMajorNum, deviceMinorNum int32, rbps, wbps int64) CommandOption {
	return func(c *commandImpl) {
		c.setIOLimits(deviceMajorNum, deviceMinorNum, rbps, wbps)
	}
}

func WithNewRootBase(newRootBase string) CommandOption {
	return func(c *commandImpl) {
		c.setNewRootBase(newRootBase)
	}
}

func WithNewNetNS() CommandOption {
	return func(c *commandImpl) {
		c.setNewNetNS()
	}
}

func WithNewPidNS() CommandOption {
	return func(c *commandImpl) {
		c.setNewPidNS()
	}
}
