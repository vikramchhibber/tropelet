package exec

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/google/uuid"

	"github.com/troplet/pkg/exec/cgroups"
	"github.com/troplet/pkg/exec/mountfs"
)

// These are internal library states and may not directly
// correspond to the command's lifecycle states.
const (
	cmdStateInit       cmdStateType = "init"
	cmdStateRunning    cmdStateType = "running"
	cmdStateTerminated cmdStateType = "terminated"
	cmdStateFinished   cmdStateType = "finished"
)

func newCommand(name string, args []string, options ...CommandOption) (execCmd *Command, err error) {
	// Every command is assigned a unique id
	id := uuid.NewString()

	// Initialize defaults and mandatory params
	execCmd = &Command{id: id, name: name, args: args,
		cmdState: cmdStateInit}

	// Cleanup of incomplete initialization
	defer func() {
		if err != nil && execCmd != nil {
			execCmd.Finish()
		}
	}()

	execCmd.cgroupsMgr, err = cgroups.NewControlGroupsManager(id)
	if err != nil {
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

func (c *Command) finish() error {
	changeState := func() bool {
		c.lock.Lock()
		defer c.lock.Unlock()
		if c.cmdState == cmdStateTerminated ||
			c.cmdState == cmdStateInit {
			c.cmdState = cmdStateFinished
			return true
		}
		return false
	}
	if !changeState() {
		return fmt.Errorf("invalid command state")
	}

	if c.cgroupsMgr != nil {
		c.cgroupsMgr.Finish()
	}
	if c.mountFSMgr != nil {
		c.mountFSMgr.Finish()
	}

	return nil
}

func (c *Command) setCPULimit(quotaMillSeconds, periodMillSeconds int64) {
	c.cgroupsMgr.NewCPUControlGroup(quotaMillSeconds, periodMillSeconds)
}

func (c *Command) setMemoryLimit(memKB int64) {
	c.cgroupsMgr.NewMemoryControlGroup(memKB)
}

func (c *Command) setNewRootBase(newRootBase string) {
	// The new root for each process will be created under the passed root base
	// concatenated with a unique command ID, ensuring that multiple
	// commands do not share the same root.
	c.mountFSMgr = mountfs.NewMountFSManager(filepath.Join(newRootBase, c.id))
}

func (c *Command) setIOLimits(deviceMajorNum, deviceMinorNum int32, rbps, wbps int64) {
	c.cgroupsMgr.NewIOControlGroup(deviceMajorNum, deviceMinorNum, rbps, wbps)
}

func (c *Command) execute(ctx context.Context) error {
	c.cmd = exec.CommandContext(ctx, c.name, c.args...)
	var wg sync.WaitGroup
	defer wg.Wait()
	if c.stdoutChan != nil {
		stdoutPipe, err := c.cmd.StdoutPipe()
		if err != nil {
			return fmt.Errorf("failed creating stdout pipe: %w", err)
		}
		wg.Add(1)
		go func() {
			c.readPipe(c.stdoutChan, stdoutPipe)
			wg.Done()
		}()
	}
	if c.stderrChan != nil {
		stderrPipe, err := c.cmd.StderrPipe()
		if err != nil {
			return fmt.Errorf("failed creating stderr pipe: %w", err)
		}
		wg.Add(1)
		go func() {
			c.readPipe(c.stderrChan, stderrPipe)
			wg.Done()
		}()
	}
	// TODO: Config candidate
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
		c.cmd.SysProcAttr.CgroupFD, err = c.cgroupsMgr.GetControlGroupsFD()
		if err != nil {
			return err
		}
		c.cmd.SysProcAttr.Cloneflags |= syscall.CLONE_INTO_CGROUP
		c.cmd.SysProcAttr.UseCgroupFD = true
	}

	// Execute command
	if err := c.cmd.Start(); err != nil {
		return fmt.Errorf("failed starting command: %w", err)
	}

	// Wait for the process to terminate
	err := c.cmd.Wait()

	// We will wait for all the goroutines to finish
	return err
}

func (c *Command) readPipe(dst ReadChannel, src io.Reader) {
	for {
		// TODO: Config candidate
		// TODO: This has GC overhead
		buf := make([]byte, 128)
		n, err := io.ReadFull(src, buf)
		if n != 0 {
			dst <- buf[:n]
		}
		if err != nil || c.closeReaders.Load() {
			close(dst)
			break
		}
	}
}

func (c *Command) kill() error {
	c.lock.Lock()
	defer c.lock.Unlock()
	if c.cmdState != cmdStateRunning ||
		c.cmd == nil || c.cmd.Process == nil {
		return fmt.Errorf("invalid command state to send signal")
	}
	c.closeReaders.Store(true)

	if err := c.cmd.Process.Kill(); err != nil {
		return fmt.Errorf("failed to kill process: %w", err)
	}

	return nil
}

func (c *Command) sendSignalToGroup(sig syscall.Signal) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	if c.cmdState != cmdStateRunning ||
		c.cmd == nil || c.cmd.Process == nil {
		return fmt.Errorf("invalid command state to send signal")
	}

	pgid, err := syscall.Getpgid(c.cmd.Process.Pid)
	if err != nil {
		return fmt.Errorf("failed getting group PID: %w", err)
	}
	if err := syscall.Kill(-pgid, sig); err != nil {
		return fmt.Errorf("failed to send signal to process group: %w", err)
	}

	return nil
}
