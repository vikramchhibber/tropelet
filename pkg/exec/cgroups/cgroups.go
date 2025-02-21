package cgroups

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type ControlGroupsManager struct {
	cpu            *CPUControlGroup
	mem            *MemoryControlGroup
	io             *IOManager
	cgroupFile     *os.File
	cgroupPath     string
	cgroupControls []string
}

func NewControlGroupsManager(name string) (*ControlGroupsManager, error) {
	cgroupV2Path, err := findCgroupV2Mount()
	if err != nil {
		return nil, err
	}
	cgroupControls, err := readSubtreeControls(cgroupV2Path)
	if err != nil {
		return nil, err
	}
	return &ControlGroupsManager{cgroupPath: filepath.Join(cgroupV2Path, name),
		cgroupControls: cgroupControls}, nil
}

func (m *ControlGroupsManager) NewCPUControlGroup(quotaMillSeconds,
	periodMillSeconds int64) *CPUControlGroup {
	m.cpu = NewCPUControlGroup(m.cgroupPath, quotaMillSeconds, periodMillSeconds)

	return m.cpu
}

func (m *ControlGroupsManager) NewMemoryControlGroup(memoryKB int64) *MemoryControlGroup {
	m.mem = NewMemoryControlGroup(m.cgroupPath, memoryKB)

	return m.mem
}

func (m *ControlGroupsManager) NewIOManager(deviceMajorNum, deviceMinorNum int32,
	rbps, wbps int64) *IOManager {
	m.io = NewIOManager(m.cgroupPath, deviceMajorNum, deviceMinorNum, rbps, wbps)

	return m.io
}

func (m *ControlGroupsManager) Set() error {
	if err := os.Mkdir(m.cgroupPath, 0755); err != nil {
		return fmt.Errorf("failed to create cgroup path %s: %v",
			m.cgroupPath, err)
	}

	if m.cpu != nil {
		if !slices.Contains(m.cgroupControls, "cpu") {
			return nil
		}
		if err := m.cpu.Set(); err != nil {
			return err
		}
	}
	if m.mem != nil {
		if !slices.Contains(m.cgroupControls, "memory") {
			return nil
		}
		if err := m.mem.Set(); err != nil {
			return err
		}
	}
	if m.io != nil {
		if !slices.Contains(m.cgroupControls, "io") {
			fmt.Printf("IO is disabled\n")
			return nil
		}
		if err := m.io.Set(); err != nil {
			return err
		}
	}

	return nil
}

func (m *ControlGroupsManager) GetControlGroupsFD() (int, error) {
	if m.cgroupFile != nil {
		return int(m.cgroupFile.Fd()), nil
	}
	cgroupFile, err := os.Open(m.cgroupPath)
	if err != nil {
		return 0, err
	}
	m.cgroupFile = cgroupFile

	return int(m.cgroupFile.Fd()), nil
}

func (m *ControlGroupsManager) Finish() {
	if m.cgroupPath != "" {
		os.RemoveAll(m.cgroupPath)
	}
	if m.cgroupFile != nil {
		m.cgroupFile.Close()
	}
}

func findCgroupV2Mount() (string, error) {
	f, err := os.Open("/proc/mounts")
	if err != nil {
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), " ")
		if len(fields) >= 3 && fields[2] == "cgroup2" {
			return fields[1], nil
		}
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	return "", os.ErrNotExist
}

func readSubtreeControls(cgroupPath string) ([]string, error) {
	filePath := cgroupPath + "/cgroup.subtree_control"
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	ret := []string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		ret = append(ret, strings.Split(line, " ")...)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return ret, nil
}

func writeToFile(filePath, value string) error {
	if err := os.WriteFile(filePath, []byte(value), 0644); err != nil {
		return fmt.Errorf("failed to write to %s, %s: %v", filePath, value, err)
	}

	return nil
}
