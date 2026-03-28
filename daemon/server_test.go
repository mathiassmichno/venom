package daemon

import (
	"sync"
	"testing"
	"time"
)

func TestListProcessesConcurrentStartStop(t *testing.T) {
	pm := NewProcManager()
	server := NewServer(pm)

	var wg sync.WaitGroup
	stop := make(chan struct{})

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					info, err := pm.Start("sleep", []string{"10"}, ProcStartOptions{})
					if err != nil {
						return
					}
					time.Sleep(10 * time.Millisecond)
					pm.Stop(info.ID, false)
				}
			}
		}()
	}

	for i := 0; i < 100; i++ {
		_, _ = server.ListProcesses(nil, nil)
		time.Sleep(1 * time.Millisecond)
	}

	close(stop)
	wg.Wait()
}
