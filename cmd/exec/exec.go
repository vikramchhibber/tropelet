package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/troplet/internal/shared"
	"github.com/troplet/pkg/exec"
)

func main() {
	path, err := GetFilesystemMount("./")
	if err != nil {
		return
	}
	fmt.Printf("Got path: %s\n", path)
	major, minor, err := GetDeviceNumbers(path)
	if err != nil {
		return
	}
	fmt.Printf("Got major=%d, minor=%d\n", major, minor)
	var wg sync.WaitGroup
	stdoutChan := make(chan []byte, 0)
	stderrChan := make(chan []byte, 0)
	readFunc := func() {
		numChannels := 2
		for numChannels > 0 {
			select {
			case data, ok := <-stdoutChan:
				if !ok {
					stdoutChan = nil
					numChannels--
					continue
				}
				fmt.Print(string(data))
			case data, ok := <-stderrChan:
				if !ok {
					stderrChan = nil
					numChannels--
					continue
				}
				fmt.Print("\033[31m" + string(data) + "\033[0m")
			}
		}
		wg.Done()
	}
	wg.Add(1)
	go readFunc()
	var cmd *exec.Command
	if len(os.Args) == 2 {
		cmd, err = exec.NewCommand(os.Args[1], []string{},
			exec.WithStdoutChan(stdoutChan), exec.WithStderrChan(stderrChan),
			exec.WithNewRootBase("./"), exec.WithCPULimit(100, 1000),
			exec.WithUsePIDNS(), exec.WithUseNetNS(), exec.WithMemoryLimit(4*1024),
			exec.WithIOLimits(major, minor, 4*1024, 1024))
	} else if len(os.Args) > 2 {
		cmd, err = exec.NewCommand(os.Args[1], os.Args[2:],
			exec.WithStdoutChan(stdoutChan), exec.WithStderrChan(stderrChan),
			exec.WithNewRootBase("./"), exec.WithCPULimit(100, 1000),
			exec.WithUsePIDNS(), exec.WithUseNetNS(), exec.WithMemoryLimit(4*1024),
			exec.WithIOLimits(major, minor, 4*1024, 1024))
	}
	if err != nil {
		fmt.Printf("error in cmd creation: %v", err.Error())
		return
	}
	fmt.Printf(cmd.String() + "\n")
	shared.RegisterShutdownSigCallback(func() {
		wg.Add(1)
		cmd.SendTermSignal()
		wg.Done()
	})
	/*
		go func() {
			time.Sleep(5 * time.Second)
			cmd.Terminate()
			cmd.SendTermSignal()
		}()
	*/
	cmd.Execute(context.Background())
	exitErr, _ := cmd.GetExitError()
	exitCode, _ := cmd.GetExitCode()
	fmt.Printf("%s %d\n", exitErr, exitCode)
	wg.Wait()
	cmd.Finish()
	fmt.Printf(cmd.String() + "\n")
}

func GetFilesystemMount(path string) (string, error) {
	// Get absolute path to handle relative paths
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %v", err)
	}

	// Read /proc/mounts
	file, err := os.Open("/proc/mounts")
	if err != nil {
		return "", fmt.Errorf("failed to read /proc/mounts: %v", err)
	}
	defer file.Close()

	// Find the most specific mount point
	var bestMatch string
	bestLen := -1

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}

		mountPoint := fields[1]
		// Unescape the mount point
		mountPoint = strings.ReplaceAll(mountPoint, "\\040", " ")
		mountPoint = strings.ReplaceAll(mountPoint, "\\011", "\t")

		// Check if this mount point is a parent of our path
		rel, err := filepath.Rel(mountPoint, absPath)
		if err != nil {
			continue
		}

		// Skip if path goes outside mount point
		if strings.HasPrefix(rel, "..") {
			continue
		}

		// Keep track of the longest (most specific) mount point
		if len(mountPoint) > bestLen {
			bestLen = len(mountPoint)
			bestMatch = mountPoint
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("error reading /proc/mounts: %v", err)
	}

	if bestLen == -1 {
		return "", fmt.Errorf("no filesystem found for %s", path)
	}

	return bestMatch, nil
}

func GetDeviceNumbers(dirPath string) (int32, int32, error) {
	info, err := os.Stat(dirPath)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to stat directory: %v", err)
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, fmt.Errorf("failed to get system stat info")
	}

	// Get device numbers from the filesystem device ID
	major := int32((stat.Dev >> 8) & 0xfff)
	minor := int32(stat.Dev & 0xff)

	return major, minor, nil
}
