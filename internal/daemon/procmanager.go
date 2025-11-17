package daemon

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/go-cmd/cmd"
	"github.com/google/uuid"
)

type ProcInfo struct {
	ID    string
	Cmd   *cmd.Cmd
	Stdin *io.PipeWriter
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
	Cwd           string
	Dir           string
	Env           []string
	WithStdinPipe bool
	WaitForExit   bool
	WaitForRegex  *regexp.Regexp
	WaitTimeout   *time.Duration
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
	defer p.Unlock()

	go func(bufferedLines []string) {
		for _, line := range bufferedLines {
			select {
			case ch <- LogEntry{
				ts:     time.Now(),
				stream: StreamNONE,
				line:   line,
			}:
			default:
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
	close(ch)
	p.Unlock()
}

// DumpToFile writes the entire buffer to a file.
func (p *ProcInfo) DumpToFile(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}

	p.Lock()
	defer p.Unlock()

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
	defer slog.Info("started process", "id", id)

	var stdinReader io.Reader
	var stdinWriter *io.PipeWriter
	if opts.WithStdinPipe {
		stdinReader, stdinWriter = io.Pipe()
	} else {
		stdinReader = strings.NewReader("")
	}
	command := cmd.NewCmdOptions(cmd.Options{
		Buffered:       true,
		Streaming:      true,
		CombinedOutput: true,
	}, name, args...)

	command.Dir = opts.Cwd
	command.Env = append(command.Env, opts.Env...)

	statusCh := command.StartWithStdin(stdinReader)

	info := &ProcInfo{
		ID:      id,
		Cmd:     command,
		logSubs: make(map[chan LogEntry]struct{}),
		Stdin:   stdinWriter,
	}

	pm.Lock()
	if pm.Procs == nil {
		pm.Procs = make(map[string]*ProcInfo)
	}
	pm.Procs[id] = info
	pm.Unlock()

	matchCh := make(chan struct{})
	matched := false

	// Go routine for forwarding stdout/err to log subs and potentially matching regex
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
			if opts.WaitForRegex != nil && !matched && opts.WaitForRegex.MatchString(line) {
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

	if opts.WaitForExit || opts.WaitForRegex != nil {
		timeout := time.Duration(60 * time.Second)
		if opts.WaitTimeout != nil {
			timeout = *opts.WaitTimeout
		}
		if opts.WaitForExit && info.Stdin != nil {
			slog.Warn("waiting for process exit but has stdin pipe. Closing pipe", "id", info.ID)
			if err := info.Stdin.Close(); err != nil {
				return info, err
			}
		}
		select {
		case status := <-statusCh:
			slog.Info("process exited", "id", id, "status", status)
			if !opts.WaitForExit {
				return info, fmt.Errorf("process exited before matching '%v'", opts.WaitForRegex.String())
			}
		case <-matchCh:
		case <-time.After(timeout):
			return info, fmt.Errorf("timeout while waiting")
		}
	}

	return info, nil
}

func (pm *ProcManager) Stop(id string, wait bool) (*ProcInfo, error) {
	slog.Info("stopping process", "id", id)
	defer slog.Info("stopped process", "id", id)
	pm.Lock()
	p, ok := pm.Procs[id]
	pm.Unlock()
	if !ok {
		return nil, fmt.Errorf("process not found")
	}
	err := p.Cmd.Stop()
	if wait {
		select {
		case <-p.Cmd.Done():
		case <-time.After(10 * time.Second):
			slog.Warn("timed out while stopping", "id", id, "status", p.Cmd.Status())
		}
	}
	return p, err
}

func (pm *ProcManager) Shutdown() error {
	slog.Info("shutting down process manager", "processes", pm.Procs)
	pm.Lock()
	defer pm.Unlock()
	var errs []error

	for id, p := range pm.Procs {
		slog.Info("stopping process", "id", p.ID)
		if err := p.Cmd.Stop(); err != nil {
			errs = append(errs, err)
		}
		delete(pm.Procs, id)
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
