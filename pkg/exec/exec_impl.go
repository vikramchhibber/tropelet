package exec

import (
	"context"
	"io"
	"os/exec"
	"strings"
	"syscall"
)

func (c *commandImpl) GetID() string {
	return c.id
}

func (c *commandImpl) String() string {
	return c.id + ", " + c.name +
		" " + strings.Join(c.args, " ") + ", " +
		string(c.cmdState)
}

func (c *commandImpl) Execute(ctx context.Context) {
	if err := c.setState([]cmdStateType{cmdStateInit},
		cmdStateRunning); err != nil {
		return
	}
	c.err = c.execute(ctx)
	if err := c.setState([]cmdStateType{cmdStateRunning},
		cmdStateTerminated); err != nil {
		return
	}
}

func (c *commandImpl) IsTerminated() bool {
	c.lock.Lock()
	defer c.lock.Unlock()

	return c.cmdState == cmdStateTerminated ||
		c.cmdState == cmdStateFinished
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

func (c *commandImpl) SendTermSignal() {
	c.lock.Lock()
	defer c.lock.Unlock()

	// Send term signal to all the process in the group
	c.sendSignalToGroup(syscall.SIGTERM)
}

func (c *commandImpl) Finish() {
	// Finish can be called in any of these states
	// except "finished"
	if err := c.setState([]cmdStateType{cmdStateInit,
		cmdStateRunning, cmdStateTerminated},
		cmdStateFinished); err != nil {
		// Already in finished state
		return
	}

	// Kill all the processes in the group
	c.sendSignalToGroup(syscall.SIGKILL)

	// We will wait for all the goroutines to finish
	c.waitGroup.Wait()
	if c.cgroupsMgr != nil {
		c.cgroupsMgr.Finish()
	}
	if c.mountFSMgr != nil {
		c.mountFSMgr.Finish()
	}
}

func (c *commandImpl) execute(ctx context.Context) error {
	c.cmd = exec.CommandContext(ctx, c.name, c.args...)
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
	c.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if c.newPidNS {
		c.cmd.SysProcAttr.Cloneflags |= syscall.CLONE_NEWPID
	}
	if c.newNetNS {
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
		if c.cmd.SysProcAttr.CgroupFD, err =
			c.cgroupsMgr.GetControlGroupsFD(); err != nil {
			return err
		}
		c.cmd.SysProcAttr.Cloneflags |= syscall.CLONE_INTO_CGROUP
		c.cmd.SysProcAttr.UseCgroupFD = true
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

func (c *commandImpl) sendSignalToGroup(sig syscall.Signal) {
	if c.cmd != nil && c.cmd.Process != nil {
		if pgid, err := syscall.Getpgid(c.cmd.Process.Pid); err == nil {
			syscall.Kill(-pgid, sig)
		}
	}
}
