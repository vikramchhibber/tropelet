package server

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/troplet/pkg/exec"
	"github.com/troplet/pkg/proto"
)

const (
	memKB             = 16 * 1024       // 16MB
	rbps              = 4 * 1024 * 1024 // 4MB
	wbps              = 1024 * 1024     // 1MB
	quotaMillSeconds  = 100
	periodMillSeconds = 1000
)

type JobInfo struct {
	info proto.JobEntry
	cmd  exec.Command
	wg   sync.WaitGroup
	// Map of gRPC stream id to stream entry channel
	chanMap map[uint64]chan *proto.JobStreamEntry
}

func NewJobInfo(cmd string, args []string) *JobInfo {
	jobInfo := &JobInfo{}
	jobInfo.info.Command = cmd
	jobInfo.info.Args = args

	return jobInfo
}

func (j *JobInfo) Finish() {
	if j.cmd != nil {
		// The exec library ignores accidental
		// second time Finish call
		j.cmd.Finish()
	}
	j.wg.Wait()
}

func (j *JobInfo) Launch() string {
	// TODO: vikram
	major, minor := int32(0), int32(0)
	cmdOptions := []exec.CommandOption{}
	stdoutChan, stderrChan := make(exec.ReadChannel, 0), make(exec.ReadChannel, 0)
	cmdOptions = append(cmdOptions, exec.WithStdoutChan(stdoutChan))
	cmdOptions = append(cmdOptions, exec.WithStderrChan(stderrChan))
	cmdOptions = append(cmdOptions, exec.WithNewRootBase("./"))
	cmdOptions = append(cmdOptions, exec.WithCPULimit(quotaMillSeconds, periodMillSeconds))
	cmdOptions = append(cmdOptions, exec.WithNewPidNS())
	cmdOptions = append(cmdOptions, exec.WithNewNetNS())
	cmdOptions = append(cmdOptions, exec.WithMemoryLimit(memKB))
	cmdOptions = append(cmdOptions, exec.WithIOLimits(major, minor, rbps, wbps))
	j.info.StartTs = timestamppb.New(time.Now())
	cmd, err := exec.NewCommand(j.info.Command, j.info.Args, cmdOptions...)
	if err != nil {
		// Since the initiation of this job failed, we will generate
		// a unique id to keep details about this launch attempt
		return j.updateJobEntryOnExit(uuid.New().String(), err.Error(), 1)
	}
	j.cmd = cmd
	j.info.Id = cmd.GetID()

	// Prepare reading stdout and stderr streams
	j.wg.Add(1)
	go j.readStreams(stdoutChan, stderrChan)

	// Launch command execution asynchronously.
	j.wg.Add(1)
	go func() {
		// This will block till the command completes.
		// Pass non timeout context.
		cmd.Execute(context.Background())
		var exitError string
		err := cmd.GetExitError()
		if err != nil {
			exitError = err.Error()
		}
		j.updateJobEntryOnExit(cmd.GetID(), exitError, cmd.GetExitCode())
		// Finish must be called if command object is created
		cmd.Finish()
		j.wg.Done()
	}()

	return j.info.Id
}

func (j *JobInfo) readStreams(stdoutChan, stderrChan exec.ReadChannel) {
	numChannels := 2
	for numChannels > 0 {
		select {
		case data, ok := <-stdoutChan:
			if !ok {
				stdoutChan = nil
				numChannels--
				continue
			}
			fmt.Printf(string(data))
		case data, ok := <-stderrChan:
			if !ok {
				stderrChan = nil
				numChannels--
				continue
			}
			fmt.Printf(string(data))
		}
	}
	j.wg.Done()
}

func (j *JobInfo) updateJobEntryOnExit(jobID string, exitError string,
	exitCode int) string {
	j.info.Id = jobID
	j.info.EndTs = timestamppb.New(time.Now())
	j.info.ExitError = exitError
	j.info.ExitCode = int32(exitCode)

	return j.info.Id
}
