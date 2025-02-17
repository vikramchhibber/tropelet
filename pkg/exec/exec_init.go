package exec

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
)

const cgroupV2Path = "/sys/fs/cgroup"

type commandImpl struct {
	id         string
	name       string
	args       []string
	stdoutChan ReadChannel
	stderrChan ReadChannel
	timeout    time.Duration
	// CPU, memory disk utilization limits
	cpuLimitPct uint16
	memLimitKB  uint32

	cgroupPath string
	cmd        *exec.Cmd
	waitGroup  sync.WaitGroup
	err        error
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
		cpuLimit := fmt.Sprintf("%d %d", int64(c.cpuLimitPct)*1000, 1000*1000)
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

func (c *commandImpl) setCPULimit(quotaPct uint16) error {
	if quotaPct == 0 {
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
