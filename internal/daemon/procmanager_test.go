package daemon

import (
	"io"
	"testing"
)

func TestProcessStart(t *testing.T) {
	info, err := NewProcManager().Start("cat", []string{}, ProcStartOptions{WaitForExit: true, WithStdinPipe: true})
	if info.Stdin != nil {
		written, err := io.WriteString(info.Stdin, "test\n")
		t.Logf("written: %v err: %v", written, err)
		err = info.Stdin.Close()
		if err != nil {
			t.Error(err.Error())
		}
	}

	<-info.Cmd.Done()

	if info.Cmd.Status().Exit != 0 || err != nil {
		t.Errorf("Failed to start and wait for cat: %v", err)
	}
	t.Logf("got output: %v", info.Cmd.Status().Stdout)
}
