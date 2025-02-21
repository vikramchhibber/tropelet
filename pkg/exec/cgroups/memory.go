package cgroups

import (
	"path/filepath"
	"strconv"
)

type MemoryControlGroup struct {
	memKB      int64
	cgroupPath string
}

func NewMemoryControlGroup(cgroupPath string, memKB int64) *MemoryControlGroup {
	return &MemoryControlGroup{memKB, cgroupPath}
}

func (c *MemoryControlGroup) Set() error {
	if c.memKB != 0 {
		target := filepath.Join(c.cgroupPath, "memory.max")
		if err := writeToFile(target,
			strconv.FormatInt(c.memKB*1024, 10)); err != nil {
			return err
		}
		/*
			target = filepath.Join(c.cgroupPath, "cgroup.subtree_control")
			if err := writeToFile(target, "+memory"); err != nil {
				return err
			}
		*/
	}

	return nil
}
