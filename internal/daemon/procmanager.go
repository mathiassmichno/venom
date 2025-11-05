package daemon

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"sync"
	"time"

	"github.com/go-cmd/cmd"
	"github.com/google/uuid"
)

type ProcInfo struct {
	ID  string
	Cmd *cmd.Cmd

	sync.RWMutex
	logSubs map[chan LogEntry]struct{}
}

type ProcManager struct {
	sync.RWMutex
	Procs map[string]*ProcInfo
}

func NewProcManager() *ProcManager {
	return &ProcManager{Procs: make(map[string]*ProcInfo)}
}

type ProcStartOptions struct {
	Cwd            string
	Dir            string
	Env            []string
	WaitFor        *regexp.Regexp
	WaitForTimeout time.Duration
}

type LogEntry struct {
	ts     time.Time
	stream Stream
	line   string
}

type Stream int

const (
	StreamNONE Stream = iota
	StreamSTDOUT
	StreamSTDERR
)

func (p *ProcInfo) PublishLine(stream Stream, line string) {
	slog.Info("PublishLine", "stream", stream, "line", line)
	ts := time.Now()
	for ch := range p.logSubs {
		select {
		case ch <- LogEntry{ts: ts, line: line, stream: stream}:
		default:
		}
	}
}

func (p *ProcInfo) Subscribe() chan LogEntry {
	ch := make(chan LogEntry, 200)
	slog.Info("subscribing", "ch", ch, "id", p.ID, "name", p.Cmd.Name)

	p.Lock()
	slog.Info("locked proc")
	defer p.Unlock()

	go func(bufferedLines []string) {
		for _, line := range bufferedLines {
			select {
			case ch <- LogEntry{
				ts:     time.Now(),
				stream: StreamNONE,
				line:   line,
			}:
				slog.Info("sent buffered line", "id", p.ID, "line", line)
			default:
				slog.Info("DEFAULTED!", "id", p.ID, "line", line)
			}
		}
	}(append([]string(nil), p.Cmd.Status().Stdout...))

	p.logSubs[ch] = struct{}{}

	return ch
}

// Unsubscribe removes a subscription.
func (p *ProcInfo) Unsubscribe(ch chan LogEntry) {
	slog.Info("unsubscribing", "ch", ch, "id", p.ID, "name", p.Cmd.Name)
	p.Lock()
	delete(p.logSubs, ch)
	if _, ok := <-ch; ok {
		close(ch)
	}
	p.Unlock()
}

// DumpToFile writes the entire buffer to a file.
func (p *ProcInfo) DumpToFile(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}

	p.RLock()
	defer p.RUnlock()

	// TODO: Stderr???
	for _, line := range p.Cmd.Status().Stdout {
		if _, err := f.WriteString(line + "\n"); err != nil {
			return err
		}
	}
	return f.Close()
}

func (pm *ProcManager) Start(name string, args []string, opts ProcStartOptions) (*ProcInfo, error) {
	id := uuid.NewString()
	slog.Info("starting process", "id", id, "name", name, "args", args, "opts", opts)

	command := cmd.NewCmdOptions(cmd.Options{
		Buffered:       true,
		Streaming:      true,
		CombinedOutput: true,
	}, name, args...)

	command.Dir = opts.Cwd
	command.Env = append(command.Env, opts.Env...)

	command.Start()

	info := &ProcInfo{
		ID:      id,
		Cmd:     command,
		logSubs: make(map[chan LogEntry]struct{}),
	}

	pm.Lock()
	if pm.Procs == nil {
		pm.Procs = make(map[string]*ProcInfo)
	}
	pm.Procs[id] = info
	pm.Unlock()

	matchCh := make(chan struct{})
	matched := false

	go func() {
		for command.Stdout != nil || command.Stderr != nil {
			var line string
			var open bool
			var stream Stream
			select {
			case line, open = <-command.Stdout:
				stream = StreamSTDOUT
				if !open {
					command.Stdout = nil
				}
			case line, open = <-command.Stderr:
				stream = StreamSTDERR
				if !open {
					command.Stderr = nil
				}
			}
			if opts.WaitFor != nil && !matched && opts.WaitFor.MatchString(line) {
				matched = true
				close(matchCh)
			}
			if open {
				info.PublishLine(stream, line)
			}
		}
		info.Lock()
		defer info.Unlock()
		for ch := range info.logSubs {
			close(ch)
		}
	}()

	if opts.WaitFor != nil {
		select {
		case <-matchCh:
		case <-time.After(opts.WaitForTimeout):
			return info, fmt.Errorf("timeout waiting for %q", opts.WaitFor.String())
		}
	}

	return info, nil
}

func (pm *ProcManager) Stop(id string, wait bool) (*cmd.Status, error) {
	pm.Lock()
	p, ok := pm.Procs[id]
	pm.Unlock()
	if !ok {
		return nil, fmt.Errorf("process not found")
	}
	err := p.Cmd.Stop()
	if wait {
		<-p.Cmd.Done()
	}
	status := p.Cmd.Status()
	return &status, err
}

func (pm *ProcManager) Shutdown() error {
	pm.Lock()
	defer pm.Unlock()
	var errs []error

	for _, p := range pm.Procs {
		if err := p.Cmd.Stop(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
