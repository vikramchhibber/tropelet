package exec

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

type testJobReadData struct {
	testName         string
	command          string
	args             []string
	timeout          time.Duration
	wg               sync.WaitGroup
	stdoutChan       ReadChannel
	stderrChan       ReadChannel
	stdoutStrBuilder strings.Builder
	stderrStrBuilder strings.Builder

	expectError      bool
	expectedErrorStr string
	expectStdout     bool
	expectStdoutStr  string
	expectStderr     bool
	expectStderrStr  string
}

func (d *testJobReadData) testStartRead() {
	d.stdoutChan = make(ReadChannel)
	d.stderrChan = make(ReadChannel)
	d.wg.Add(1)
	go func() {
		numChannels := 2
		for numChannels > 0 {
			select {
			case data, ok := <-d.stdoutChan:
				if !ok {
					d.stdoutChan = nil
					numChannels--
					continue
				}
				d.stdoutStrBuilder.Write(data)
			case data, ok := <-d.stderrChan:
				if !ok {
					d.stderrChan = nil
					numChannels--
					continue
				}
				d.stderrStrBuilder.Write(data)
			}
		}
		d.wg.Done()
	}()
}

func (d *testJobReadData) testWait() {
	d.wg.Wait()
}

func TestBasic(t *testing.T) {
	createCommand := func(d *testJobReadData) (*Command, error) {
		return NewCommand(d.command, d.args,
			WithStdoutChan(d.stdoutChan),
			WithStderrChan(d.stderrChan))
	}

	testData := []*testJobReadData{
		{
			testName:    "Basic ls cmd",
			command:     "ls",
			args:        []string{"-lrt"},
			expectError: false,
		},
		{
			testName:        "Find bash",
			command:         "which",
			args:            []string{"bash"},
			expectError:     false,
			expectStdoutStr: "/usr/bin/bash\n",
		},
		{
			testName:    "Unknown command",
			command:     "FooBar123",
			args:        []string{""},
			expectError: true,
		},
		{
			testName:         "Timeout command",
			command:          "yes",
			args:             []string{""},
			timeout:          2 * time.Second,
			expectError:      true,
			expectedErrorStr: "signal: killed",
		},
		{
			testName:    "Wildcard ls with error",
			command:     "ls",
			args:        []string{"*"},
			expectError: true,
		},
		{
			testName: "Wildcard ls fixed",
			command:  "/usr/bin/bash",
			args:     []string{"-c", "ls *"},
		},
	}
	for _, d := range testData {
		t.Logf("Executing test: %s", d.testName)
		d.testStartRead()
		cmd, err := createCommand(d)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		ctx := context.Background()
		var cancel context.CancelFunc
		if d.timeout != 0 {
			ctx, cancel = context.WithTimeout(ctx, d.timeout)
		}
		// This will wait for the command to finish
		cmd.Execute(ctx)
		d.testWait()
		if d.expectStdoutStr != "" {
			if diff := cmp.Diff(d.expectStdoutStr, d.stdoutStrBuilder.String()); diff != "" {
				t.Errorf("Unexpected result: %s", diff)
			}
		}
		if d.expectStderrStr != "" {
			if diff := cmp.Diff(d.expectStderrStr, d.stderrStrBuilder.String()); diff != "" {
				t.Errorf("Unexpected result: %s", diff)
			}
		}
		if d.expectedErrorStr != "" {
			exitErr, err := cmd.GetExitError()
			if diff := cmp.Diff(nil, err); diff != "" {
				t.Errorf("Unexpected result: %s", diff)
			}
			if diff := cmp.Diff(true, exitErr != nil); diff != "" {
				t.Errorf("Unexpected result: %s", diff)
			}
			if diff := cmp.Diff(d.expectedErrorStr, exitErr.Error()); diff != "" {
				t.Errorf("Unexpected result: %s", diff)
			}
		}
		cmd.Finish()
		if cancel != nil {
			cancel()
		}
	}
}

func TestNewPIDNetNS(t *testing.T) {
	createCommand := func(d *testJobReadData) (*Command, error) {
		return NewCommand(d.command, d.args,
			WithStdoutChan(d.stdoutChan),
			WithStderrChan(d.stderrChan),
			WithUseNetNS(),
			WithUsePIDNS())
	}
	testData := []*testJobReadData{
		{
			testName:        "PID should be one",
			command:         "/usr/bin/bash",
			args:            []string{"-c", "echo echo $$ > ./1; chmod +x ./1;./1"},
			expectError:     false,
			expectStdoutStr: "1\n",
		},
		{
			testName:        "Add localhost loopback address on lo interface",
			command:         "ip",
			args:            []string{"addr", "add", "127.0.0.1/8", "dev", "lo"},
			expectError:     false,
			expectStdoutStr: "",
		},
	}
	for _, d := range testData {
		t.Logf("Executing test: %s", d.testName)
		d.testStartRead()
		cmd, err := createCommand(d)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		ctx := context.Background()
		var cancel context.CancelFunc
		if d.timeout != 0 {
			ctx, cancel = context.WithTimeout(ctx, d.timeout)
		}
		// This will wait for the command to finish
		cmd.Execute(ctx)
		d.testWait()
		if d.expectStdoutStr != "" {
			if diff := cmp.Diff(d.expectStdoutStr, d.stdoutStrBuilder.String()); diff != "" {
				t.Errorf("Unexpected result: %s", diff)
			}
		}
		if d.expectStderrStr != "" {
			if diff := cmp.Diff(d.expectStderrStr, d.stderrStrBuilder.String()); diff != "" {
				t.Errorf("Unexpected result: %s", diff)
			}
		}
		if d.expectedErrorStr != "" {
			exitErr, err := cmd.GetExitError()
			if diff := cmp.Diff(nil, err); diff != "" {
				t.Errorf("Unexpected result: %s", diff)
			}
			if diff := cmp.Diff(true, exitErr != nil); diff != "" {
				t.Errorf("Unexpected result: %s", diff)
			}
			if diff := cmp.Diff(d.expectedErrorStr, exitErr.Error()); diff != "" {
				t.Errorf("Unexpected result: %s", diff)
			}
		}
		cmd.Finish()
		if cancel != nil {
			cancel()
		}
	}
}

func TestNewRootCGroups(t *testing.T) {
	createCommand := func(d *testJobReadData) (*Command, error) {
		return NewCommand(d.command, d.args,
			WithStdoutChan(d.stdoutChan),
			WithStderrChan(d.stderrChan),
			WithUseNetNS(),
			WithUsePIDNS(),
			WithNewRootBase("./"),
			WithMemoryLimit(16*1024),
			WithCPULimit(50, 1000))
	}
	testData := []*testJobReadData{
		{
			testName:        "Listing mounted directories",
			command:         "/usr/bin/bash",
			args:            []string{"-c", "ls -t / > ./1; sort ./1"},
			expectError:     false,
			expectStdoutStr: "1\nbin\nlib\nlib64\nproc\nsys\nusr\n",
		},
	}
	for _, d := range testData {
		t.Logf("Executing test: %s", d.testName)
		d.testStartRead()
		cmd, err := createCommand(d)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		ctx := context.Background()
		var cancel context.CancelFunc
		if d.timeout != 0 {
			ctx, cancel = context.WithTimeout(ctx, d.timeout)
		}
		// This will wait for the command to finish
		cmd.Execute(ctx)
		d.testWait()
		if d.expectStdoutStr != "" {
			if diff := cmp.Diff(d.expectStdoutStr, d.stdoutStrBuilder.String()); diff != "" {
				t.Errorf("Unexpected result: %s", diff)
			}
		}
		if d.expectStderrStr != "" {
			if diff := cmp.Diff(d.expectStderrStr, d.stderrStrBuilder.String()); diff != "" {
				t.Errorf("Unexpected result: %s", diff)
			}
		}
		if d.expectedErrorStr != "" {
			exitErr, err := cmd.GetExitError()
			if diff := cmp.Diff(nil, err); diff != "" {
				t.Errorf("Unexpected result: %s", diff)
			}
			if diff := cmp.Diff(true, exitErr != nil); diff != "" {
				t.Errorf("Unexpected result: %s", diff)
			}
			if diff := cmp.Diff(d.expectedErrorStr, exitErr.Error()); diff != "" {
				t.Errorf("Unexpected result: %s", diff)
			}
		}
		cmd.Finish()
		if cancel != nil {
			cancel()
		}
	}
}
