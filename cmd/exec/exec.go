package main

import (
	"fmt"
	"os"
	"sync"
	_ "time"

	"github.com/troplet/internal/shared"
	"github.com/troplet/pkg/exec"
)

func main() {
	var wg sync.WaitGroup
	stdoutChan := make(chan []byte, 0)
	stderrChan := make(chan []byte, 0)
	readFunc := func(dataChan <-chan []byte, isStdErr bool) {
		for {
			data, ok := <-dataChan
			if ok {
				if !isStdErr {
					fmt.Print(string(data))
				} else {
					fmt.Print("\033[31m" + string(data) + "\033[0m")
				}
			} else {
				break
			}
		}
		wg.Done()
	}
	wg.Add(2)
	go readFunc(stdoutChan, false)
	go readFunc(stderrChan, true)
	var cmd exec.Command
	var err error
	if len(os.Args) == 2 {
		cmd, err = exec.NewCommand(os.Args[1], []string{},
			exec.WithStdoutChan(stdoutChan), exec.WithStderrChan(stderrChan),
			exec.WithNewRoot("./newroot9"), exec.WithCPULimit(1))
	} else if len(os.Args) > 2 {
		cmd, err = exec.NewCommand(os.Args[1], os.Args[2:],
			exec.WithStdoutChan(stdoutChan), exec.WithStderrChan(stderrChan),
			exec.WithNewRoot("./newroot9"), exec.WithCPULimit(1))
	}
	if err != nil {
		fmt.Printf("error in cmd creation: %v", err.Error())
	}
	fmt.Printf("Command id: %s\n", cmd.GetID())
	shared.RegisterShutdownSigCallback(func() {
		// cmd.Terminate()
		fmt.Printf("Calling finish\n")
		wg.Add(1)
		cmd.Finish()
		fmt.Printf("Done with finish\n")
		wg.Done()
	})
	/*
		go func() {
			time.Sleep(5 * time.Second)
			cmd.Terminate()
		}()*/
	err = cmd.Execute()
	cmd.Finish()
	if err != nil {
		fmt.Printf("%s %d\n", err.Error(), cmd.GetExitCode())
	} else {
		fmt.Printf("%d\n", cmd.GetExitCode())
	}
	wg.Wait()
}
