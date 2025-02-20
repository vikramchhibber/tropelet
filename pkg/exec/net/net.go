package net

import (
	"fmt"
	"os"
	"os/exec"
)

type NetworkManager struct {
}

func NewNetworkManager() *NetworkManager {
	return &NetworkManager{}
}

func (m *NetworkManager) AttachLocalIntf(pid int) error {
	nsenter := exec.Command("nsenter",
		fmt.Sprintf("--net=/proc/%d/ns/net", pid),
		"--", "ip", "link", "set", "lo", "up")
	nsenter.Stdout = os.Stdout
	nsenter.Stderr = os.Stderr
	if err := nsenter.Run(); err != nil {
		return err
	}

	return nil
}

func (m *NetworkManager) Finish() {
	// Nothing to do
}
