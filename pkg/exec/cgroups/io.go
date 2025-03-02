package cgroups

import (
	"fmt"
	"path/filepath"
)

type IOControlGroup struct {
	deviceMajorNum int32
	deviceMinorNum int32
	rbps           int64
	wbps           int64
	cgroupPath     string
}

func NewIOControlGroup(cgroupPath string, deviceMajorNum, deviceMinorNum int32,
	rbps, wbps int64) *IOControlGroup {
	return &IOControlGroup{deviceMajorNum, deviceMinorNum, rbps, wbps, cgroupPath}
}

func (c *IOControlGroup) GetName() string {
	return "io"
}

func (c *IOControlGroup) Set() error {
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

	return nil
}
