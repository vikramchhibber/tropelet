package mountfs

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/troplet/pkg/exec/cgroups"
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
	{"", cgroups.CGroupV2Path, "cgroup2", 0, 500},
}

type MountFSManager struct {
	mountRoot string
}

func NewMountFSManager(mountRoot string) *MountFSManager {
	return &MountFSManager{mountRoot}
}

func (m *MountFSManager) GetMountRoot() string {
	return m.mountRoot
}

func (m *MountFSManager) Mount() error {
	if m.mountRoot == "" {
		return nil
	}
	for _, d := range fsInfo {
		target := filepath.Join(m.mountRoot, d.targetPrefix)
		if err := os.MkdirAll(target, d.permissions); err != nil {
			return fmt.Errorf("failed to create %s: %v", target, err)
		}
		if err := syscall.Mount(d.source, target, d.fstype, d.flags, ""); err != nil {
			return fmt.Errorf("failed to mount %s: %v", target, err)
		}
	}

	return nil
}

func (m *MountFSManager) Finish() {
	if m.mountRoot == "" {
		return
	}

	for i := len(fsInfo) - 1; i >= 0; i-- {
		target := filepath.Join(m.mountRoot, fsInfo[i].targetPrefix)
		if err := syscall.Unmount(target, 0); err != nil {
			fmt.Printf("failed unmounting %s\n", target)
			// TODO: Log error and continue
		}
		if err := os.Remove(target); err != nil {
			fmt.Printf("failed deleting %s\n", target)
			// TODO: Log error and continue
		}
	}

	// Special handling for these directories as they are nested
	for _, d := range []string{"sys", "usr"} {
		target := filepath.Join(m.mountRoot, d)
		if err := os.RemoveAll(target); err != nil {
			fmt.Printf("failed deleting %s\n", target)
			// TODO: Log error and continue
		}
	}

	return
}
