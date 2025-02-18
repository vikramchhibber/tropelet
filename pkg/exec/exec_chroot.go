package exec

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

var mountData = []struct {
	source       string
	targetPrefix string
	fstype       string
	flags        uintptr
	permissions  os.FileMode
}{
	{"/usr/bin", "usr/bin", "", syscall.MS_BIND | syscall.MS_RDONLY, 500},
	{"/usr/lib", "usr/lib", "", syscall.MS_BIND | syscall.MS_RDONLY, 500},
	{"/lib", "lib", "", syscall.MS_BIND | syscall.MS_RDONLY, 500},
	{"/lib64", "lib64", "", syscall.MS_BIND | syscall.MS_RDONLY, 500},
	{"proc", "proc", "proc", 0, 600},
	{"", cgroupV2Path, "cgroup2", 0, 500},
}

func (c *commandImpl) mountNewRoot() error {
	if c.newRoot == "" {
		return nil
	}
	for _, d := range mountData {
		target := filepath.Join(c.newRoot, d.targetPrefix)
		// Read and execute permissions to the user only
		if err := os.MkdirAll(target, d.permissions); err != nil {
			return fmt.Errorf("failed to create %s: %v", target, err)
		}
		if err := syscall.Mount(d.source, target, d.fstype, d.flags, ""); err != nil {
			return fmt.Errorf("failed to mount %s: %v", target, err)
		}
	}

	return nil
}

func (c *commandImpl) umountNewRoot() error {
	if c.newRoot == "" {
		return nil
	}
	for i := len(mountData) - 1; i >= 0; i-- {
		target := filepath.Join(c.newRoot, mountData[i].targetPrefix)
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
		target := filepath.Join(c.newRoot, d)
		if err := os.RemoveAll(target); err != nil {
			fmt.Printf("failed deleting %s\n", target)
			// TODO: Log error and continue
		}
	}

	return nil
}
