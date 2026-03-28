package daemon

import (
	"io"
	"regexp"
	"sync"
	"testing"
	"time"
)

func TestProcessStart(t *testing.T) {
	info, err := NewProcManager().Start(
		"cat",
		[]string{},
		ProcStartOptions{
			WaitForExit:   false,
			WithStdinPipe: true,
			LogInfoPrefix: true,
		},
	)
	if info.Stdin != nil {
		var written int
		var err error
		written, err = io.WriteString(info.Stdin, "test\n")
		t.Logf("written: %v err: %v", written, err)
		written, err = io.WriteString(info.Stdin, "test2\n")
		t.Logf("written: %v err: %v", written, err)
		written, err = io.WriteString(info.Stdin, "test3\n")
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

func TestShutdownNoDeadlock(t *testing.T) {
	pm := NewProcManager()
	for i := 0; i < 5; i++ {
		_, err := pm.Start("sleep", []string{"10"}, ProcStartOptions{})
		if err != nil {
			t.Fatalf("Failed to start process: %v", err)
		}
	}

	done := make(chan struct{})
	go func() {
		pm.Shutdown()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("Shutdown deadlocked")
	}

	if len(pm.Procs) != 0 {
		t.Errorf("Expected 0 procs after shutdown, got %d", len(pm.Procs))
	}
}

func TestConcurrentSubscribeUnsubscribe(t *testing.T) {
	pm := NewProcManager()
	info, err := pm.Start("cat", []string{}, ProcStartOptions{})
	if err != nil {
		t.Fatalf("Failed to start process: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		ch := info.Subscribe()
		wg.Add(2)

		go func() {
			defer wg.Done()
			info.Unsubscribe(ch)
		}()

		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				info.PublishLine(StreamSTDOUT, "test")
			}
		}()
	}
	wg.Wait()
}

func TestMultipleSubscribers(t *testing.T) {
	pm := NewProcManager()
	info, err := pm.Start("cat", []string{}, ProcStartOptions{WithStdinPipe: true})
	if err != nil {
		t.Fatalf("Failed to start process: %v", err)
	}

	sub1 := info.Subscribe()
	sub2 := info.Subscribe()
	defer info.Unsubscribe(sub1)
	defer info.Unsubscribe(sub2)

	go func() {
		io.WriteString(info.Stdin, "hello\n")
		info.Stdin.Close()
	}()

	timeout := time.After(2 * time.Second)
	for {
		select {
		case <-timeout:
			t.Fatal("timeout waiting for messages")
		case entry, ok := <-sub1:
			if !ok {
				t.Fatal("sub1 closed unexpectedly")
			}
			if entry.line == "hello" {
				goto found
			}
		case entry, ok := <-sub2:
			if !ok {
				t.Fatal("sub2 closed unexpectedly")
			}
			if entry.line == "hello" {
				goto found
			}
		}
	}
found:
}

func TestUnsubscribeDoesNotCloseChannel(t *testing.T) {
	pm := NewProcManager()
	info, err := pm.Start("cat", []string{}, ProcStartOptions{WithStdinPipe: true})
	if err != nil {
		t.Fatalf("Failed to start process: %v", err)
	}

	ch := info.Subscribe()
	info.Unsubscribe(ch)

	select {
	case _, ok := <-ch:
		if ok {
			t.Log("channel still open after unsubscribe (expected behavior)")
		} else {
			t.Error("channel closed after unsubscribe - this should not happen anymore")
		}
	case <-time.After(100 * time.Millisecond):
		t.Log("channel buffered and not closed (acceptable)")
	}
}
