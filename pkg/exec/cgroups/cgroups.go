package cgroups

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

const (
	CGroupV2Path          = "/sys/fs/cgroup"
	cgroupFilePermissions = 0644
)

type ControlGroupManager struct {
	cpu        *CPUControlGroup
	mem        *MemoryControlGroup
	cgroupFD   int
	cgroupPath string
}

func NewControlGroupManager(cgroupName string) *ControlGroupManager {
	return &ControlGroupManager{cgroupPath: filepath.Join(CGroupV2Path, cgroupName)}
}

func (m *ControlGroupManager) NewCPUControlGroup(quotaMillSeconds,
	periodMillSeconds int64) *CPUControlGroup {
	m.cpu = NewCPUControlGroup(m.cgroupPath, quotaMillSeconds, periodMillSeconds)

	return m.cpu
}

func (m *ControlGroupManager) NewMemoryControlGroup(memoryKB int64) *MemoryControlGroup {
	m.mem = NewMemoryControlGroup(m.cgroupPath, memoryKB)

	return m.mem
}

func (m *ControlGroupManager) Set() error {
	if err := os.Mkdir(m.cgroupPath, 0755); err != nil {
		return fmt.Errorf("failed to create cgroup path %s: %v",
			m.cgroupPath, err)
	}

	if m.cpu != nil {
		if err := m.cpu.Set(); err != nil {
			return err
		}
	}
	if m.mem != nil {
		if err := m.mem.Set(); err != nil {
			return err
		}
	}

	return nil
}

func (m *ControlGroupManager) Finish() {
	if m.cgroupPath != "" {
		os.RemoveAll(m.cgroupPath)
	}
	if m.cgroupFD != 0 {
		syscall.Close(m.cgroupFD)
	}
}

func (m *ControlGroupManager) AttachPID(pid int) error {
	cgroupProcs := filepath.Join(m.cgroupPath, "cgroup.procs")
	if err := os.WriteFile(cgroupProcs, []byte(strconv.Itoa(pid)),
		cgroupFilePermissions); err != nil {
		return fmt.Errorf("failed to add process %d to cgroup: %v", pid, err)
	}

	return nil
}

func (m *ControlGroupManager) GetControlGroupFD() (int, error) {
	if m.cgroupFD != 0 {
		return m.cgroupFD, nil
	}
	cgroupFD, err := syscall.Open(m.cgroupPath, syscall.O_RDONLY, 0)
	if err != nil {
		return 0, err
	}
	m.cgroupFD = cgroupFD

	return m.cgroupFD, nil
}

func writeToFile(filePath, value string) error {
	if err := os.WriteFile(filePath, []byte(value),
		cgroupFilePermissions); err != nil {
		return fmt.Errorf("failed to write to %s: %v", filePath, value)
	}

	return nil
}
