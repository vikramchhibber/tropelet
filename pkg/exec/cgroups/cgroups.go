package cgroups

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// ControlGroup is a contract between ControlGroupsManager
// and different ControlGroup implementation
type ControlGroup interface {
	Set() error
	GetName() string
}

type ControlGroupsManager struct {
	cgroups          []ControlGroup
	cgroupFile       *os.File
	cgroupPath       string
	supportedCGroups []string
}

func NewControlGroupsManager(name string) (*ControlGroupsManager, error) {
	// Get cgroups path from /proc/mounts
	cgroupV2Path, err := findCGroupV2Mount()
	if err != nil {
		return nil, fmt.Errorf("failed to get cgroups path: %w", err)
	}
	// Get all the enabled controllers
	supportedCGroups, err := readSubtreeControls(cgroupV2Path)
	if err != nil {
		return nil, fmt.Errorf("failed to get supported cgroups: %w", err)
	}

	return &ControlGroupsManager{
		cgroupPath:       filepath.Join(cgroupV2Path, name),
		supportedCGroups: supportedCGroups, cgroups: []ControlGroup{}}, nil
}

func (m *ControlGroupsManager) NewCPUControlGroup(quotaMillSeconds,
	periodMillSeconds int64) *CPUControlGroup {
	cpu := NewCPUControlGroup(m.cgroupPath, quotaMillSeconds, periodMillSeconds)
	m.cgroups = append(m.cgroups, cpu)

	return cpu
}

func (m *ControlGroupsManager) NewMemoryControlGroup(memoryKB int64) *MemoryControlGroup {
	mem := NewMemoryControlGroup(m.cgroupPath, memoryKB)
	m.cgroups = append(m.cgroups, mem)

	return mem
}

func (m *ControlGroupsManager) NewIOControlGroup(deviceMajorNum, deviceMinorNum int32,
	rbps, wbps int64) *IOControlGroup {
	io := NewIOControlGroup(m.cgroupPath, deviceMajorNum, deviceMinorNum, rbps, wbps)
	m.cgroups = append(m.cgroups, io)

	return io
}

func (m *ControlGroupsManager) Set() error {
	if err := os.Mkdir(m.cgroupPath, 0755); err != nil {
		return fmt.Errorf("failed to create cgroup path %s: %w",
			m.cgroupPath, err)
	}
	for _, cgroup := range m.cgroups {
		// My environment somehow does not have "io" controller
		// enabled in subtree_control
		// sudo cat /sys/fs/cgroup/cgroup.controllers
		// cpuset cpu io memory hugetlb pids rdma misc
		// sudo cat /sys/fs/cgroup/cgroup.subtree_control
		// cpu memory pids
		// Instead of failing the entire set, logging
		// this and continuing
		if !slices.Contains(m.supportedCGroups, cgroup.GetName()) {
			// Print this in red
			fmt.Print("\033[31m" + cgroup.GetName() +
				" control group is NOT enabled, continuing...\n" + "\033[0m")
			continue
		}
		if err := cgroup.Set(); err != nil {
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
		return 0, fmt.Errorf("failed to open %s: %w", m.cgroupPath, err)
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

func findCGroupV2Mount() (string, error) {
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
		return fmt.Errorf("failed writing to %s: %w", filePath, err)
	}

	return nil
}
