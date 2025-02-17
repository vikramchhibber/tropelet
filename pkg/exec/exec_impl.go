package exec

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
	// 	return c.cmd.ProcessState.Exited()
	return c.jobState == jobStateTerminated
}

func (c *commandImpl) GetExitError() error {
	return c.err
}

func (c *commandImpl) Terminate() {
	if c.cmd != nil && c.cmd.Cancel != nil {
		c.cmd.Cancel()
	}
}

func (c *commandImpl) Finish() {
	if err := c.setState([]jobStateType{jobStateInit,
		jobStateRunning, jobStateTerminated},
		jobStateFinished); err != nil {
		return
	}
	c.Terminate()
	// We will wait for all the goroutines
	// to finish before closing the channels
	c.waitGroup.Wait()
	// If there is an error, pass on to the
	// stderr channel
	if c.stderrChan != nil && c.err != nil {
		c.stderrChan <- []byte(c.err.Error())
	}
	if c.cgroupPath != "" {
		os.RemoveAll(c.cgroupPath)
	}
	if c.stdoutChan != nil {
		close(c.stdoutChan)
	}
	if c.stderrChan != nil {
		close(c.stderrChan)
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

	// Command execution
	if err := c.cmd.Start(); err != nil {
		return err
	}

	cgroupProcs := filepath.Join(c.cgroupPath, "cgroup.procs")
	if err := os.WriteFile(cgroupProcs, []byte(strconv.Itoa(
		c.cmd.Process.Pid)), 0644); err != nil {
		return fmt.Errorf("failed to add process to cgroup: %v", err)
	}

	// Process the outputs
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
			break
		}
	}
	c.waitGroup.Done()
}
