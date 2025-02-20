package cgroups

import (
	"fmt"
	"path/filepath"
)

type IOManager struct {
	deviceMajorNum int32
	deviceMinorNum int32
	rbps           int64
	wbps           int64
	cgroupPath     string
}

func NewIOManager(cgroupPath string, deviceMajorNum, deviceMinorNum int32, rbps, wbps int64) *IOManager {
	return &IOManager{deviceMajorNum, deviceMinorNum, rbps, wbps, cgroupPath}
}

func (c *IOManager) Set() error {
	if c.rbps != 0 {
		target := filepath.Join(c.cgroupPath, "io.max")
		value := fmt.Sprintf("%d:%d rbps=%d", c.deviceMajorNum,
			c.deviceMinorNum, c.rbps)
		if err := writeToFile(target, value); err != nil {
			return err
		}
	}
	if c.wbps != 0 {
		target := filepath.Join(c.cgroupPath, "io.max")
		value := fmt.Sprintf("%d:%d wbps=%d", c.deviceMajorNum,
			c.deviceMinorNum, c.wbps)
		if err := writeToFile(target, value); err != nil {
			return err
		}
	}

	if c.rbps != 0 || c.wbps != 0 {
		target := filepath.Join(c.cgroupPath, "cgroup.subtree_control")
		if err := writeToFile(target, "+io"); err != nil {
			return err
		}
	}

	return nil
}
