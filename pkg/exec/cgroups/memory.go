package cgroups

import (
	"path/filepath"
	"strconv"
)

type MemoryControlGroup struct {
	MemoryKB int64

	cgroupPath string
}

func NewMemoryControlGroup(cgroupPath string, memoryKB int64) *MemoryControlGroup {
	return &MemoryControlGroup{memoryKB, cgroupPath}
}

func (c *MemoryControlGroup) Set() error {
	if c.MemoryKB != 0 {
		memoryMaxPath := filepath.Join(c.cgroupPath, "memory.max")
		if err := writeToFile(memoryMaxPath,
			strconv.FormatInt(c.MemoryKB*1024, 10)); err != nil {
			return err
		}
	}

	return nil
}
