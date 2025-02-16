package exec

import (
	"time"
)

type OnTerminateCB func(error)

type Command interface {
	GetID() string
	Execute() error
	ExecuteAsync(onTerminateCB OnTerminateCB)
	IsTerminated() bool
	GetExitError() error
	Terminate()
	Finish()
}

type CommandOption func(*commandImpl) error

func NewCommand(name string, args []string, options ...CommandOption) (Command, error) {
	return newCommand(name, args, options...)
}

func WithTimeout(timeout time.Duration) CommandOption {
	return func(c *commandImpl) error {
		c.timeout = timeout
		return nil
	}
}

func WithStdoutChan(stdoutChan chan []byte) CommandOption {
	return func(c *commandImpl) error {
		c.stdoutChan = stdoutChan
		return nil
	}
}

func WithStderrChan(stderrChan chan []byte) CommandOption {
	return func(c *commandImpl) error {
		c.stderrChan = stderrChan
		return nil
	}
}

func WithCPULimit(quotaPct uint8) CommandOption {
	return func(c *commandImpl) error {
		return c.setCPULimit(quotaPct)
	}
}

func WithMemoryLimit(memKB uint32) CommandOption {
	return func(c *commandImpl) error {
		return c.setMemoryLimit(memKB)
	}
}
