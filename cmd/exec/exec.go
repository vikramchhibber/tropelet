package main

import (
	"fmt"
	"os"
	"sync"

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
	cmd, err := exec.NewCommand("/usr/bin/bash", append([]string{"-c"}, os.Args[1:]...),
		exec.WithStdoutChan(stdoutChan), exec.WithStderrChan(stderrChan),
		exec.WithCPULimit(100), exec.WithMemoryLimit(1024*1024))
	if err != nil {
		panic(err.Error())
	}
	shared.RegisterShutdownSigCallback(func() {
		cmd.Terminate()
	})
	if err := cmd.Execute(); err != nil {
		fmt.Printf("error: %v\n", err.Error())
	}
	cmd.Finish()
	wg.Wait()
}
