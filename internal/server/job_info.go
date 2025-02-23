package server

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/troplet/internal/shared"
	"github.com/troplet/pkg/exec"
	"github.com/troplet/pkg/proto"
)

type ControlChanEntry struct {
	id         uint64
	stdoutChan chan *proto.JobStreamEntry
	stderrChan chan *proto.JobStreamEntry
	isAdd      bool
}

type JobInfo struct {
	logger shared.Logger
	info   proto.JobEntry
	cmd    exec.Command
	wg     sync.WaitGroup
	// Control chan to communicate attach or detach of streams
	controlChan chan *ControlChanEntry
	counter     atomic.Uint64
}

func NewJobInfo(logger shared.Logger, cmd string, args []string) *JobInfo {
	// TODO: configuration candidate
	jobInfo := &JobInfo{logger: logger, controlChan: make(chan *ControlChanEntry, 16)}
	jobInfo.info.Command = cmd
	jobInfo.info.Args = args

	return jobInfo
}

func (j *JobInfo) Terminate() {
	if j.cmd != nil {
		j.cmd.Finish()
	}
}

func (j *JobInfo) Finish() {
	if j.cmd != nil {
		// The exec library ignores accidental
		// second time Finish call
		j.cmd.Finish()
	}
	close(j.controlChan)
	j.wg.Wait()
}

func (j *JobInfo) Launch(rootBase string, quotaMillSeconds, periodMillSeconds int64,
	memKB int64, rbps, wbps int64, deviceMajorNum, deviceMinorNum int32) string {
	cmdOptions := []exec.CommandOption{}
	stdoutChan, stderrChan := make(exec.ReadChannel), make(exec.ReadChannel)
	cmdOptions = append(cmdOptions, exec.WithStdoutChan(stdoutChan))
	cmdOptions = append(cmdOptions, exec.WithStderrChan(stderrChan))
	cmdOptions = append(cmdOptions, exec.WithNewRootBase(rootBase))
	cmdOptions = append(cmdOptions, exec.WithCPULimit(quotaMillSeconds, periodMillSeconds))
	cmdOptions = append(cmdOptions, exec.WithUsePIDNS())
	cmdOptions = append(cmdOptions, exec.WithUseNetNS())
	cmdOptions = append(cmdOptions, exec.WithMemoryLimit(memKB))
	cmdOptions = append(cmdOptions, exec.WithIOLimits(deviceMajorNum, deviceMinorNum, rbps, wbps))
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
		j.logger.Infof("Job: %s is running", j.info.Id)
		cmd.Execute(context.Background())
		// Command got terminated here
		var exitError string
		if err := cmd.GetExitError(); err != nil {
			exitError = err.Error()
		}
		j.updateJobEntryOnExit(cmd.GetID(), exitError, cmd.GetExitCode())
		// Finish must be called if command object is created
		cmd.Finish()
		j.wg.Done()
	}()

	return j.info.Id
}

func (j *JobInfo) Attach(stdoutChan, stderrChan chan *proto.JobStreamEntry) (uint64, error) {
	if j.cmd == nil || j.cmd.IsTerminated() {
		return 0, fmt.Errorf("job already terminated")
	}
	subscriberID := j.counter.Add(1)

	// Send control message to add these streams
	j.controlChan <- &ControlChanEntry{id: subscriberID,
		stdoutChan: stdoutChan,
		stderrChan: stderrChan,
		isAdd:      true}

	return subscriberID, nil
}

func (j *JobInfo) Detach(subscriberID uint64) {
	// Send control message to remove these streams
	j.controlChan <- &ControlChanEntry{id: subscriberID, isAdd: false}
}

func (j *JobInfo) readStreams(stdoutChan, stderrChan exec.ReadChannel) {
	// Local maps to store mapping of subscriber id to subscriber channels.
	// These are modified only by control channel events under one
	// execution context thus there is no issue of concurrent access.
	stdoutChanMap := map[uint64]chan *proto.JobStreamEntry{}
	stderrChanMap := map[uint64]chan *proto.JobStreamEntry{}
	numChannels := 2
	for numChannels > 0 {
		select {
		case data, ok := <-stdoutChan:
			if !ok {
				stdoutChan = nil
				numChannels--
				continue
			}
			entry := &proto.JobStreamEntry{Entry: data, IsStdError: false}
			// Send data to all the clients
			for _, client := range stdoutChanMap {
				// TODO: This will block if one of the client is
				// not reading on the channel
				client <- entry
			}
		case data, ok := <-stderrChan:
			if !ok {
				stderrChan = nil
				numChannels--
				continue
			}
			entry := &proto.JobStreamEntry{Entry: data, IsStdError: true}
			// Send data to all the clients
			for _, client := range stderrChanMap {
				// TODO: This will block if one of the client is
				// not listening on the channel
				client <- entry
			}
		case entry, ok := <-j.controlChan:
			if !ok {
				goto done
			}
			if entry.isAdd {
				stdoutChanMap[entry.id] = entry.stdoutChan
				stderrChanMap[entry.id] = entry.stderrChan
				j.logger.Infof("Job: %s, subscriber %d attached", j.info.Id, entry.id)
			} else {
				if client, found := stdoutChanMap[entry.id]; found {
					close(client)
					delete(stdoutChanMap, entry.id)
					j.logger.Infof("Job: %s, subscriber %d detached", j.info.Id, entry.id)
				}
				if client, found := stderrChanMap[entry.id]; found {
					close(client)
					delete(stderrChanMap, entry.id)
				}
			}
		}
	}
done:
	// The job has terminated.
	// We can close any subscriber channels we have
	for _, client := range stdoutChanMap {
		close(client)
	}
	for _, client := range stderrChanMap {
		close(client)
	}
	j.wg.Done()
}

func (j *JobInfo) updateJobEntryOnExit(jobID string, exitError string,
	exitCode int) string {
	j.info.Id = jobID
	j.info.EndTs = timestamppb.New(time.Now())
	j.info.ExitError = exitError
	j.info.ExitCode = int32(exitCode)
	j.logger.Infof("Job: %s has terminated", j.info.Id)

	return j.info.Id
}
