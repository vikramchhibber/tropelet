package exec

import (
	"time"
)

type Command interface {
	GetID() string
	Execute() error
	IsTerminated() bool
	GetExitError() error
	Terminate()
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

func WithCPULimit(quotaPct uint16) CommandOption {
	return func(c *commandImpl) error {
		return c.setCPULimit(quotaPct)
	}
}

func WithMemoryLimit(memKB uint32) CommandOption {
	return func(c *commandImpl) error {
		return c.setMemoryLimit(memKB)
	}
}
