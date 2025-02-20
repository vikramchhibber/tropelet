package cgroups

import (
	"fmt"
	"path/filepath"
)

type CPUControlGroup struct {
	QuotaMillSeconds  int64
	PeriodMillSeconds int64

	cgroupPath string
}

func NewCPUControlGroup(cgroupPath string, quotaMillSeconds,
	periodMillSeconds int64) *CPUControlGroup {
	return &CPUControlGroup{quotaMillSeconds, periodMillSeconds, cgroupPath}

}

func (c *CPUControlGroup) Set() error {
	if c.QuotaMillSeconds != 0 && c.PeriodMillSeconds != 0 {
		cpuMaxPath := filepath.Join(c.cgroupPath, "cpu.max")
		value := fmt.Sprintf("%d %d", c.QuotaMillSeconds*1000,
			c.PeriodMillSeconds*1000)
		if err := writeToFile(cpuMaxPath, value); err != nil {
			return err
		}
	}

	return nil
}
