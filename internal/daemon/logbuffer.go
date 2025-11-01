package daemon

import (
	"bufio"
	"fmt"
	"os"
	"sync"
	"time"
)

type LogBuffer struct {
	mu            sync.RWMutex
	lines         []string
	subs          map[chan string]struct{}
	maxLines      int      // absolute hard cap in memory
	highWaterMark int      // threshold to start spilling
	tailSize      int      // how many lines to keep in memory after spill
	file          *os.File // spill file handle
	filePath      string
	spilledCount  int
}

func NewLogBuffer(maxLines int) *LogBuffer {
	return &LogBuffer{
		lines:         make([]string, 0, maxLines),
		subs:          make(map[chan string]struct{}),
		maxLines:      maxLines,
		highWaterMark: maxLines / 2, // spill when half full (tunable)
		tailSize:      500,          // keep this many recent lines
	}
}

func (lb *LogBuffer) Add(line string) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	// Check if we should spill to file
	if len(lb.lines) >= lb.highWaterMark {
		if lb.file == nil {
			path := fmt.Sprintf("/tmp/venomd_%d.log", time.Now().UnixNano())
			f, err := os.Create(path)
			if err == nil {
				lb.file = f
				lb.filePath = path
			}
		}
		if lb.file != nil {
			for _, old := range lb.lines[:len(lb.lines)-lb.tailSize] {
				_, _ = lb.file.WriteString(old + "\n")
				lb.spilledCount++
			}
			// keep only recent tail in memory
			lb.lines = append([]string(nil), lb.lines[len(lb.lines)-lb.tailSize:]...)
		}
	}

	lb.lines = append(lb.lines, line)

	for ch := range lb.subs {
		select {
		case ch <- line:
		default:
		}
	}
}

func (lb *LogBuffer) All() []string {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	var result []string
	if lb.file != nil {
		f, err := os.Open(lb.filePath)
		if err == nil {
			sc := bufio.NewScanner(f)
			for sc.Scan() {
				result = append(result, sc.Text())
			}
			f.Close()
		}
	}
	result = append(result, lb.lines...)
	return result
}

func (lb *LogBuffer) Subscribe() chan string {
	ch := make(chan string, 200)

	lb.mu.RLock()
	defer lb.mu.RUnlock()

	// Step 1: Send historical logs (file + in-memory)
	go func(path string, memLines []string) {
		// Send spilled logs from file first (if any)
		if path != "" {
			f, err := os.Open(path)
			if err == nil {
				sc := bufio.NewScanner(f)
				for sc.Scan() {
					line := sc.Text()
					select {
					case ch <- line:
					default:
						// if the subscriber is slow, drop backlog
						// (or you could block here if guaranteed fast)
					}
				}
				f.Close()
			}
		}

		// Then send the in-memory buffer
		for _, line := range memLines {
			select {
			case ch <- line:
			default:
			}
		}
	}(lb.filePath, append([]string(nil), lb.lines...))

	// Step 2: Add to live subscribers
	lb.mu.Lock()
	lb.subs[ch] = struct{}{}
	lb.mu.Unlock()

	return ch
}

// Unsubscribe removes a subscription.
func (lb *LogBuffer) Unsubscribe(ch chan string) {
	lb.mu.Lock()
	delete(lb.subs, ch)
	close(ch)
	lb.mu.Unlock()
}

// DumpToFile writes the entire buffer to a file.
func (lb *LogBuffer) DumpToFile(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	lb.mu.RLock()
	defer lb.mu.RUnlock()

	for _, line := range lb.lines {
		if _, err := f.WriteString(line + "\n"); err != nil {
			return err
		}
	}
	return nil
}
