package mountfs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

var fsInfo = []struct {
	source       string
	targetPrefix string
	fstype       string
	flags        uintptr
	permissions  os.FileMode
}{
	{"/usr/bin", "usr/bin", "", syscall.MS_BIND | syscall.MS_RDONLY, 500},
	{"/usr/lib", "usr/lib", "", syscall.MS_BIND | syscall.MS_RDONLY, 500},
	{"/usr/sbin", "usr/sbin", "", syscall.MS_BIND | syscall.MS_RDONLY, 500},
	{"/lib", "lib", "", syscall.MS_BIND | syscall.MS_RDONLY, 500},
	{"/bin", "bin", "", syscall.MS_BIND | syscall.MS_RDONLY, 500},
	{"/lib64", "lib64", "", syscall.MS_BIND | syscall.MS_RDONLY, 500},
	{"proc", "proc", "proc", 0, 600},
	{"", "/sys/fs/cgroup", "cgroup2", 0, 500},
}

type MountFSManager struct {
	mountRoot       string
	mountedPrefixes []string
}

func NewMountFSManager(mountRoot string) *MountFSManager {
	return &MountFSManager{mountRoot, []string{}}
}

func (m *MountFSManager) GetMountRoot() string {
	return m.mountRoot
}

func (m *MountFSManager) Mount() error {
	if m.mountRoot == "" {
		return nil
	}

	absPath, err := filepath.Abs(m.mountRoot)
	if err != nil {
		return err
	}
	// Lets make sure that the prefix is user's home directory
	// for safety reasons
	if !strings.HasPrefix(absPath, "/home") {
		return fmt.Errorf("mount directory provided must be anywhere under user's home")
	}
	m.mountRoot = absPath

	for _, d := range fsInfo {
		target := filepath.Join(m.mountRoot, d.targetPrefix)
		if err := os.MkdirAll(target, d.permissions); err != nil {
			return fmt.Errorf("failed to create %s: %v", target, err)
		}
		if err := syscall.Mount(d.source, target, d.fstype, d.flags, ""); err != nil {
			return fmt.Errorf("failed to mount %s: %v", target, err)
		}
		m.mountedPrefixes = append(m.mountedPrefixes, d.targetPrefix)
	}

	return nil
}

func (m *MountFSManager) Finish() {
	if m.mountRoot == "" {
		return
	}

	// Unmount only the directories that were mounted
	for i := len(m.mountedPrefixes) - 1; i >= 0; i-- {
		target := filepath.Join(m.mountRoot, m.mountedPrefixes[i])
		if err := syscall.Unmount(target, 0); err != nil {
			fmt.Printf("failed unmounting %s\n", target)
			// TODO: Log error and continue
		}
		if err := os.Remove(target); err != nil {
			fmt.Printf("failed deleting %s\n", target)
			// TODO: Log error and continue
		}
	}

	return
}
