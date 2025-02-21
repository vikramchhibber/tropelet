package mountfs

import (
	"errors"
	"fmt"
	"io/fs"
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
	mountRoot               string
	mountRootAlreadyCreated bool
	mountedPrefixes         []string
}

func NewMountFSManager(mountRoot string) *MountFSManager {
	return &MountFSManager{mountRoot, false, []string{}}
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
	// Lets make sure that the prefix is some home directory
	// for safety reasons
	if !strings.HasPrefix(absPath, "/home") {
		return fmt.Errorf("mount directory provided must be anywhere under user's home")
	}

	// We expect this path should not exist and we will create it. We will
	// remember this so that we can delete this as part of cleanup
	mountRootAlreadyCreated, err := m.exists(absPath)
	if err != nil {
		return fmt.Errorf("failed to check mount-root %s: [%w]", absPath, err)
	}

	m.mountRoot = absPath
	m.mountRootAlreadyCreated = mountRootAlreadyCreated
	for _, d := range fsInfo {
		target := filepath.Join(m.mountRoot, d.targetPrefix)
		if err := os.MkdirAll(target, d.permissions); err != nil {
			return fmt.Errorf("failed to create %s: [%w]", target, err)
		}
		if err := syscall.Mount(d.source, target, d.fstype, d.flags, ""); err != nil {
			return fmt.Errorf("failed to mount %s: [%w]", target, err)
		}
		// Remember what got mounted
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
	}
	if !m.mountRootAlreadyCreated {
		if err := os.RemoveAll(m.mountRoot); err != nil {
			fmt.Printf("failed deleting %s\n", m.mountRoot)
			// TODO: Log error and continue
		}
	}
}

func (m *MountFSManager) exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}

	return false, err
}
