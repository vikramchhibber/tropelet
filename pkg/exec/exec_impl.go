package exec

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
)

func (c *commandImpl) GetID() string {
	return c.id
}

func (c *commandImpl) Execute() error {
	if err := c.setState([]jobStateType{jobStateInit},
		jobStateRunning); err != nil {
		return err
	}
	c.err = c.execute()
	if err := c.setState([]jobStateType{jobStateRunning},
		jobStateTerminated); err != nil {
		return err
	}
	fmt.Printf("done\n")

	return c.err
}

func (c *commandImpl) IsTerminated() bool {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.jobState == jobStateTerminated
}

func (c *commandImpl) GetExitError() error {
	return c.err
}

func (c *commandImpl) GetExitCode() int {
	if c.cmd != nil && c.cmd.ProcessState != nil {
		return c.cmd.ProcessState.ExitCode()
	}

	return 0
}

func (c *commandImpl) SendTermSignal() error {
	c.lock.Lock()
	defer c.lock.Unlock()
	if c.cmd != nil && c.cmd.Process != nil {
		if process, err := os.FindProcess(c.cmd.Process.Pid); err == nil {
			return process.Signal(syscall.SIGTERM)
		}
	}

	return errors.New("process not in running state")
}

func (c *commandImpl) Finish() {
	// Finish can be called in any of these states
	// except "finished"
	if err := c.setState([]jobStateType{jobStateInit,
		jobStateRunning, jobStateTerminated},
		jobStateFinished); err != nil {
		// Already in finished state
		return
	}

	// Get the process group ID.
	if c.cmd != nil && c.cmd.Process != nil {
		if pgid, err := syscall.Getpgid(c.cmd.Process.Pid); err == nil {
			// Send SIGKILL to the entire process group.
			fmt.Printf("sending kill to all processes in group\n")
			if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil {
				fmt.Printf("failed to send SIGKILL to process group: %v", err)
			}
		}
	}

	// We will wait for all the goroutines
	// to finish before closing the channels
	c.waitGroup.Wait()
	/*
		if c.stdoutChan != nil {
			close(c.stdoutChan)
		}
		if c.stderrChan != nil {
			close(c.stderrChan)
		}
	*/
	if c.cgroupsMgr != nil {
		c.cgroupsMgr.Finish()
	}
	if c.mountFSMgr != nil {
		c.mountFSMgr.Finish()
	}
}

func (c *commandImpl) execute() error {
	timeoutCtx, cancel := context.WithTimeout(
		context.Background(), c.timeout)
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
	c.cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
		Cloneflags: syscall.CLONE_NEWPID | syscall.CLONE_NEWNET |
			syscall.CLONE_NEWNS | syscall.CLONE_INTO_CGROUP,
		Unshareflags: syscall.CLONE_NEWNS | syscall.CLONE_NEWNET,
		UseCgroupFD:  true,
	}
	if c.mountFSMgr != nil {
		c.cmd.SysProcAttr.Chroot = c.mountFSMgr.GetMountRoot()
		c.cmd.Dir = "/"
	}

	// Pass control-group FD to the process
	if c.cgroupsMgr != nil {
		var err error
		if c.cmd.SysProcAttr.CgroupFD, err =
			c.cgroupsMgr.GetControlGroupsFD(); err != nil {
			return err
		}
	}

	// Execute command
	if err := c.cmd.Start(); err != nil {
		return err
	}

	// Wait for the process to terminate
	return c.cmd.Wait()
}

func (c *commandImpl) readPipe(dst ReadChannel, src io.Reader) {
	for {
		// TODO: Config candidate
		// TODO: This has GC overhead
		buf := make([]byte, 16)
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
