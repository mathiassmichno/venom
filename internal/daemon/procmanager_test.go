package daemon

import (
	"testing"
)

func TestProcessStart(t *testing.T) {
	info, err := NewProcManager().Start("cat", []string{}, ProcStartOptions{WaitForExit: true})
	if info.Cmd.Status().Exit != 0 || err != nil {
		t.Errorf("Failed to start and wait for cat: %v", err)
	}
}
