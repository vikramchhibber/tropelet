package server

import (
	"context"
	"fmt"
	"sync"
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
	cmd    *exec.Command
	// Wait group for stdout/stderr read and cmd execute go routines cleanup
	wg sync.WaitGroup
	// Control chan to communicate attach or detach of streams
	controlChan chan *ControlChanEntry
	// Lock to protect subscriber counter and terminated flag
	lock              sync.RWMutex
	subscriberCounter uint64
	isTerminated      bool
}

func NewJobInfo(logger shared.Logger, cmd string, args []string) *JobInfo {
	// TODO: configuration candidate
	jobInfo := &JobInfo{logger: logger, controlChan: make(chan *ControlChanEntry, 16)}
	jobInfo.info.Command = cmd
	jobInfo.info.Args = args

	return jobInfo
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
	go func() {
		defer j.wg.Done()
		j.readStreams(stdoutChan, stderrChan)
	}()

	// Launch command execution asynchronously.
	j.wg.Add(1)
	go func() {
		defer j.wg.Done()
		// This will block till the command completes.
		// Pass non timeout context.
		j.logger.Infof("Job: %s is running", j.info.Id)
		cmd.Execute(context.Background())
		// Command got terminated here
		var exitError string
		exitErr, err := cmd.GetExitError()
		if err != nil {
			j.logger.Errorf("failed getting exit error: %v", err)
		} else if exitErr != nil {
			exitError = exitErr.Error()
		}
		exitCode, err := cmd.GetExitCode()
		if err != nil {
			j.logger.Errorf("failed getting exit code: %v", err)
		}
		j.updateJobEntryOnExit(cmd.GetID(), exitError, exitCode)
		// Finish must be called if command object is created
		if err := cmd.Finish(); err != nil {
			j.logger.Errorf("finish failed: %v", err)
		}
	}()

	return cmd.GetID()
}

func (j *JobInfo) Terminate() error {
	terminateCmd := func() error {
		j.lock.Lock()
		defer j.lock.Unlock()
		if j.isTerminated {
			return fmt.Errorf("job already terminated")
		}
		// cmd can be null if it failed the initialization
		if j.cmd != nil {
			if err := j.cmd.Kill(); err != nil {
				j.logger.Errorf("failed killing job: %v", err)
			}
		}
		j.isTerminated = true

		return nil
	}
	if err := terminateCmd(); err != nil {
		return err
	}
	j.wg.Wait()
	close(j.controlChan)

	return nil
}

func (j *JobInfo) Attach(stdoutChan, stderrChan chan *proto.JobStreamEntry) (uint64, error) {
	j.lock.Lock()
	defer j.lock.Unlock()
	if j.isTerminated {
		return 0, fmt.Errorf("job already terminated")
	}
	j.subscriberCounter++

	// Send control message to add these streams
	if len(j.controlChan) < cap(j.controlChan) {
		j.controlChan <- &ControlChanEntry{id: j.subscriberCounter,
			stdoutChan: stdoutChan,
			stderrChan: stderrChan,
			isAdd:      true}
	} else {
		return 0, fmt.Errorf("reached maximum control channel capacity")
	}

	return j.subscriberCounter, nil
}

func (j *JobInfo) Detach(subscriberID uint64) {
	j.lock.RLock()
	defer j.lock.RUnlock()
	if j.isTerminated {
		return
	}
	// Send control message to remove these streams
	if len(j.controlChan) < cap(j.controlChan) {
		j.controlChan <- &ControlChanEntry{id: subscriberID, isAdd: false}
	} else {
		// TODO: The caller might need to retry
		j.logger.Errorf("reached maximum control channel capacity")
	}
}

func (j *JobInfo) GetJobStatus() proto.JobEntry {
	j.lock.RLock()
	defer j.lock.RUnlock()

	return j.info
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
}

func (j *JobInfo) updateJobEntryOnExit(jobID string, exitError string,
	exitCode int) string {
	j.lock.Lock()
	defer j.lock.Unlock()
	j.info.Id = jobID
	j.info.EndTs = timestamppb.New(time.Now())
	j.info.ExitError = exitError
	j.info.ExitCode = int32(exitCode)
	j.isTerminated = true
	j.logger.Infof("Job: %s has terminated", j.info.Id)

	return j.info.Id
}
