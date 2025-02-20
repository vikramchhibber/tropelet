package exec

import (
	"time"
)

type Command interface {
	GetID() string
	Execute() error
	IsTerminated() bool
	GetExitError() error
	GetExitCode() int
	SendTermSignal() error
	Finish()
}

type ReadChannel chan []byte

type CommandOption func(*commandImpl) error

func NewCommand(name string, args []string, options ...CommandOption) (Command, error) {
	return newCommand(name, args, options...)
}

func WithTimeout(timeout time.Duration) CommandOption {
	return func(c *commandImpl) error {
		return c.withTimeout(timeout)
	}
}

func WithStdoutChan(stdoutChan ReadChannel) CommandOption {
	return func(c *commandImpl) error {
		return c.withStdoutChan(stdoutChan)
	}
}

func WithStderrChan(stderrChan ReadChannel) CommandOption {
	return func(c *commandImpl) error {
		return c.withStderrChan(stderrChan)
	}
}

func WithCPULimit(quotaMillSeconds, periodMillSeconds int64) CommandOption {
	return func(c *commandImpl) error {
		return c.setCPULimit(quotaMillSeconds, periodMillSeconds)
	}
}

func WithMemoryLimit(memKB int64) CommandOption {
	return func(c *commandImpl) error {
		return c.setMemoryLimit(memKB)
	}
}

func WithIOLimit(deviceMajorNum, deviceMinorNum int32, rbps, wbps int64) CommandOption {
	return func(c *commandImpl) error {
		return c.setIOLimit(deviceMajorNum, deviceMinorNum, rbps, wbps)
	}
}

func WithNewRoot(newRoot string) CommandOption {
	return func(c *commandImpl) error {
		return c.withNewRoot(newRoot)
	}
}

func WithNewNS() CommandOption {
	return func(c *commandImpl) error {
		return c.withNewNS()
	}
}
