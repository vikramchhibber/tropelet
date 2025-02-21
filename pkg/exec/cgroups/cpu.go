package cgroups

import (
	"fmt"
	"path/filepath"
)

type CPUControlGroup struct {
	quotaMillSeconds  int64
	periodMillSeconds int64
	cgroupPath        string
}

func NewCPUControlGroup(cgroupPath string, quotaMillSeconds,
	periodMillSeconds int64) *CPUControlGroup {
	return &CPUControlGroup{quotaMillSeconds, periodMillSeconds, cgroupPath}

}

func (c *CPUControlGroup) GetName() string {
	return "cpu"
}

func (c *CPUControlGroup) Set() error {
	if c.quotaMillSeconds != 0 && c.periodMillSeconds != 0 {
		target := filepath.Join(c.cgroupPath, "cpu.max")
		value := fmt.Sprintf("%d %d", c.quotaMillSeconds*1000,
			c.periodMillSeconds*1000)
		if err := writeToFile(target, value); err != nil {
			return err
		}
	}

	return nil
}
