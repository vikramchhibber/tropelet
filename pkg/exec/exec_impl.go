package exec

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
)

const cgroupV2Path = "/sys/fs/cgroup"

type execImpl struct {
}

type commandImpl struct {
	id         string
	name       string
	args       []string
	stdoutChan chan []byte
	stderrChan chan []byte
	timeout    time.Duration
	// CPU, memory disk utilization limits
	cpuLimitPct uint8
	memLimitKB  uint32

	cgroupPath string
	cmd        *exec.Cmd
	waitGroup  sync.WaitGroup
}

func (c *commandImpl) GetID() string {
	return c.id
}

func (c *commandImpl) Execute() error {
	return c.execute(nil)
}

func (c *commandImpl) ExecuteAsync(onTerminateCB OnTerminateCB) {
	c.waitGroup.Add(1)
	go func() {
		c.execute(onTerminateCB)
		c.waitGroup.Done()
	}()
}

func (c *commandImpl) IsTerminated() bool {
	return false
}

func (c *commandImpl) GetExitError() error {
	return nil
}

func (c *commandImpl) Terminate() {
	if c.cmd != nil && c.cmd.Cancel != nil {
		c.cmd.Cancel()
	}
}

func (c *commandImpl) Finish() {
	c.Terminate()
	c.waitGroup.Wait()
	if c.cgroupPath != "" {
		os.RemoveAll(c.cgroupPath)
	}
}

func ptr[T any](v T) *T {
	return &v
}

func newCommand(name string, args []string, options ...CommandOption) (Command, error) {
	// Initialize defaults and mandatory params
	execCmd := &commandImpl{id: uuid.New().String(),
		name: name, args: args, timeout: 10 * time.Minute}
	for _, option := range options {
		if err := option(execCmd); err != nil {
			// Cleanup of incomplete initialization
			execCmd.Finish()
			return nil, err
		}
	}
	if err := execCmd.initControlGroup(); err != nil {
		// Cleanup of incomplete initialization
		execCmd.Finish()
		return nil, err
	}

	return execCmd, nil
}

func (c *commandImpl) initControlGroup() error {
	if c.cpuLimitPct == 0 && c.memLimitKB == 0 {
		return nil
	}
	// Create control group directory structure in the current
	// directory with job id as identifier
	// Create new cgroup directory
	c.cgroupPath = filepath.Join(cgroupV2Path, "job-"+c.id)
	if err := os.Mkdir(c.cgroupPath, 0755); err != nil {
		return fmt.Errorf("failed to create cgroup path: %v", err)
	}

	// Set CPU limits
	if c.cpuLimitPct != 0 {
		cpuMaxPath := filepath.Join(c.cgroupPath, "cpu.max")
		cpuLimit := fmt.Sprintf("%d %d", 1000*1000, int64(c.cpuLimitPct)*1000)
		if err := os.WriteFile(cpuMaxPath, []byte(cpuLimit), 0644); err != nil {
			return fmt.Errorf("failed to set CPU limit: %v", err)
		}
	}

	// Set memory limit
	if c.memLimitKB != 0 {
		memoryMaxPath := filepath.Join(c.cgroupPath, "memory.max")
		if err := os.WriteFile(memoryMaxPath,
			[]byte(strconv.FormatInt(int64(c.memLimitKB*1024), 10)), 0644); err != nil {
			return fmt.Errorf("failed to set memory limit: %v", err)
		}
	}

	return nil
}

func (c *commandImpl) setCPULimit(quotaPct uint8) error {
	if quotaPct == 0 || quotaPct > 100 {
		return errors.New("invalid CPU quota")
	}
	c.cpuLimitPct = quotaPct

	return nil
}

func (c *commandImpl) setMemoryLimit(memKB uint32) error {
	if memKB == 0 {
		return errors.New("invalid memory quota")
	}
	c.memLimitKB = memKB

	return nil
}

func (c *commandImpl) execute(onTerminateCB OnTerminateCB) error {
	timeoutCtx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	c.cmd = exec.CommandContext(timeoutCtx, c.name, c.args...)
	if c.stdoutChan != nil {
		stdoutPipe, err := c.cmd.StdoutPipe()
		if err != nil {
			return err
		}
		c.waitGroup.Add(1)
		go c.readPipe(c.stdoutChan, stdoutPipe)
	}
	if c.stderrChan != nil {
		stderrPipe, err := c.cmd.StderrPipe()
		if err != nil {
			return err
		}
		c.waitGroup.Add(1)
		go c.readPipe(c.stderrChan, stderrPipe)
	}

	// Command execution
	if err := c.cmd.Start(); err != nil {
		return err
	}

	cgroupProcs := filepath.Join(c.cgroupPath, "cgroup.procs")
	if err := os.WriteFile(cgroupProcs, []byte(strconv.Itoa(c.cmd.Process.Pid)), 0644); err != nil {
		return fmt.Errorf("failed to add process to cgroup: %v", err)
	}

	// Process the outputs
	err := c.cmd.Wait()
	if err != nil {
		if timeoutCtx.Err() == context.DeadlineExceeded {
			return timeoutCtx.Err()
		}
	}
	if onTerminateCB != nil {
		onTerminateCB(err)
	}

	return nil
}

func (c *commandImpl) readPipe(dst chan []byte, src io.Reader) {
	for {
		// TODO: We are allocating this buffer for every
		// read. This has GC overhead. This can be avoided
		// by using circular queue with pre-allocated buffers.
		// Can be achieved using two channels where the
		// consumer sends back the read buffer to producer
		// after using it.
		// TODO: Config candidate
		buf := make([]byte, 512)
		n, err := io.ReadFull(src, buf)
		if n != 0 {
			// This can happen if EOF is reached
			// before reading len(buf) bytes
			if n != len(buf) {
				dst <- buf[:n]
			} else {
				dst <- buf
			}
		}
		if err != nil {
			close(dst)
			break
		}
	}
	c.waitGroup.Done()
}
