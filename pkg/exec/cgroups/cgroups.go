package cgroups

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	CGroupV2Path = "/sys/fs/cgroup"
)

type ControlGroupsManager struct {
	cpu        *CPUControlGroup
	mem        *MemoryControlGroup
	io         *IOManager
	cgroupFile *os.File
	cgroupPath string
}

func NewControlGroupsManager(name string) *ControlGroupsManager {
	return &ControlGroupsManager{cgroupPath: filepath.Join(CGroupV2Path, name)}
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

func (m *ControlGroupsManager) NewIOManager(deviceMajorNum, deviceMinorNum int32, rbps, wbps int64) *IOManager {
	m.io = NewIOManager(m.cgroupPath, deviceMajorNum, deviceMinorNum, rbps, wbps)

	return m.io
}

func (m *ControlGroupsManager) Set() error {
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
	if m.io != nil {
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
	fmt.Printf("Opening...%s\n", m.cgroupPath)
	cgroupFile, err := os.Open(m.cgroupPath)
	if err != nil {
		return 0, err
	}
	m.cgroupFile = cgroupFile

	return int(m.cgroupFile.Fd()), nil
}

func (m *ControlGroupsManager) Finish() {
	if m.cgroupFile != nil {
		m.cgroupFile.Close()
	}
	if m.cgroupPath != "" {
		os.RemoveAll(m.cgroupPath)
	}
}

func writeToFile(filePath, value string) error {
	if err := os.WriteFile(filePath, []byte(value), 0644); err != nil {
		return fmt.Errorf("failed to write to %s, %s: %v", filePath, value, err)
	}

	return nil
}
