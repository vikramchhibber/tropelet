package net

import (
	"fmt"
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
	if err := nsenter.Run(); err != nil {
		return err
	}
	nsenter = exec.Command("nsenter",
		fmt.Sprintf("--net=/proc/%d/ns/net", pid),
		"--", "ip", "addr", "add", "127.0.0.1/8", "dev", "lo")

	return nsenter.Run()
}

func (m *NetworkManager) Finish() {
	// Nothing to do
}
