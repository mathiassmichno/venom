package daemon

import (
	"io"
	"regexp"
	"sync"
	"testing"
	"time"
)

func TestProcessStart(t *testing.T) {
	info, err := NewProcManager().Start("cat", []string{}, ProcStartOptions{WaitForExit: false, WithStdinPipe: true})
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

func TestProcessStop(t *testing.T) {
	pm := NewProcManager()
	info, err := pm.Start("sleep", []string{"5"}, ProcStartOptions{})
	if err != nil {
		t.Fatalf("Failed to start process: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	_, err = pm.Stop(info.ID, true)
	if err != nil {
		t.Errorf("Failed to stop process: %v", err)
	}
}

func TestProcessStartWithRegex(t *testing.T) {
	regex := regexp.MustCompile("hello")
	timeout := 2 * time.Second
	info, err := NewProcManager().Start("sh", []string{"-c", "echo hello"}, ProcStartOptions{
		WaitForRegex: regex,
		WaitTimeout:  &timeout,
	})
	if err != nil {
		t.Fatalf("Failed to start process with regex: %v", err)
	}
	if info.Cmd.Status().Complete {
		t.Error("Process should still be running")
	}
}

func TestLogSubscription(t *testing.T) {
	pm := NewProcManager()
	info, err := pm.Start("echo", []string{"hello"}, ProcStartOptions{WaitForExit: true})
	if err != nil {
		t.Fatalf("Failed to start process: %v", err)
	}

	ch := info.Subscribe()
	defer info.Unsubscribe(ch)

	var wg sync.WaitGroup
	wg.Go(func() {
		for entry := range ch {
			if entry.line == "hello" {
				return
			}
		}
	})

	wg.Wait()
}

func TestShutdown(t *testing.T) {
	pm := NewProcManager()
	_, err := pm.Start("sleep", []string{"5"}, ProcStartOptions{})
	if err != nil {
		t.Fatalf("Failed to start process: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	err = pm.Shutdown()
	if err != nil {
		t.Errorf("Shutdown failed: %v", err)
	}

	if len(pm.Procs) != 0 {
		t.Errorf("Expected 0 procs after shutdown, got %d", len(pm.Procs))
	}
}
