package exec

import (
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
	d.wg.Add(2)
	readChanFunc := func(strBuilder *strings.Builder, ch ReadChannel) {
		for {
			data, ok := <-ch
			if !ok {
				break
			}
			strBuilder.Write(data)
		}
		d.wg.Done()
	}
	go readChanFunc(&d.stdoutStrBuilder, d.stdoutChan)
	go readChanFunc(&d.stderrStrBuilder, d.stderrChan)
}

func (d *testJobReadData) testWait() {
	d.wg.Wait()
}

func (d *testJobReadData) testNewCommand() (Command, error) {
	if d.timeout == 0 {
		return NewCommand(d.command, d.args,
			WithStdoutChan(d.stdoutChan),
			WithStderrChan(d.stderrChan))
	}
	return NewCommand(d.command, d.args,
		WithStdoutChan(d.stdoutChan),
		WithStderrChan(d.stderrChan),
		WithTimeout(d.timeout))
}

func TestBasic(t *testing.T) {
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
		cmd, err := d.testNewCommand()
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		err = cmd.Execute()
		if diff := cmp.Diff(d.expectError, err != nil); diff != "" {
			t.Errorf("Unexpected result: %s", diff)
		}
		cmd.Finish()
		d.testWait()
		if diff := cmp.Diff(d.expectError, cmd.GetExitError() != nil); diff != "" {
			t.Errorf("Unexpected result: %s", diff)
		}
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
			if diff := cmp.Diff(true, cmd.GetExitError() != nil); diff != "" {
				t.Fatalf("Unexpected result: %s", diff)
			}
			if diff := cmp.Diff(d.expectedErrorStr, cmd.GetExitError().Error()); diff != "" {
				t.Errorf("Unexpected result: %s", diff)
			}
		}
	}
}
