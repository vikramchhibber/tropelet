package cgroups

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
)

func TestCGroups(t *testing.T) {
	cgroupsMgr, err := NewControlGroupsManager(uuid.New().String())
	if diff := cmp.Diff(nil, err); diff != "" {
		t.Errorf("Unexpected result: %s", diff)
	}
	cgroupsMgr.NewCPUControlGroup(50, 1000)
	cgroupsMgr.NewMemoryControlGroup(16 * 1024)
	err = cgroupsMgr.Set()
	if diff := cmp.Diff(nil, err); diff != "" {
		t.Errorf("Unexpected result: %s", diff)
	}
	// cpu
	content, err := os.ReadFile(filepath.Join(cgroupsMgr.cgroupPath, "cpu.max"))
	if diff := cmp.Diff(nil, err); diff != "" {
		t.Errorf("Unexpected result: %s", diff)
	}
	if diff := cmp.Diff("50000 1000000\n", string(content)); diff != "" {
		t.Errorf("Unexpected result: %s", diff)
	}
	// memory
	content, err = os.ReadFile(filepath.Join(cgroupsMgr.cgroupPath, "memory.max"))
	if diff := cmp.Diff(nil, err); diff != "" {
		t.Errorf("Unexpected result: %s", diff)
	}
	if diff := cmp.Diff("16777216\n", string(content)); diff != "" {
		t.Errorf("Unexpected result: %s", diff)
	}
	cgroupsMgr.Finish()
}
