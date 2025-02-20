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
		} else {
			fmt.Printf("group pid not found\n")
		}
	}

	// We will wait for all the goroutines
	// to finish before closing the channels
	c.waitGroup.Wait()
	if c.stdoutChan != nil {
		close(c.stdoutChan)
	}
	if c.stderrChan != nil {
		close(c.stderrChan)
	}
	if c.netMgr != nil {
		c.netMgr.Finish()
	}
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
		Pgid:    0,
		Cloneflags: syscall.CLONE_NEWPID | // New PID namespace
			syscall.CLONE_NEWNET | // New network namespace
			syscall.CLONE_NEWNS, // New mount namespace (needed for /proc)
		//		Cloneflags: syscall.CLONE_NEWNET,
		Unshareflags: syscall.CLONE_NEWNS, // Make sure mount changes are private
	}
	if c.mountFSMgr != nil {
		c.cmd.SysProcAttr.Chroot = c.mountFSMgr.GetMountRoot()
		c.cmd.Dir = "/"
	}

	// Execute command
	if err := c.cmd.Start(); err != nil {
		return err
	}

	if c.netMgr != nil {
		if err := c.netMgr.AttachLocalIntf(c.cmd.Process.Pid); err != nil {
			return err
		}
	}

	// Attach the launched process PID
	if c.cgroupsMgr != nil {
		if err := c.cgroupsMgr.AttachPID(c.cmd.Process.Pid); err != nil {
			return err
		}
	}

	// Wait for the process to terminate
	return c.cmd.Wait()
}

func (c *commandImpl) readPipe(dst ReadChannel, src io.Reader) {
	for {
		// TODO: We are allocating this buffer for every
		// read. This has GC overhead. This can be avoided
		// by using circular queue with pre-allocated buffers.
		// Can be achieved using two channels where the
		// consumer sends back the read buffer to producer
		// after using it.
		// TODO: Config candidate
		buf := make([]byte, 64)
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
			break
		}
	}
	c.waitGroup.Done()
}
