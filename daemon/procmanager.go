package daemon

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-cmd/cmd"
	"github.com/google/uuid"
)

// ProcInfo holds process state and is protected by its embedded RWMutex.
//
// IMPORTANT: Concurrency invariants:
//   - The "forwarding goroutine" (spawned in Start()) OWNS the logSubs channels.
//     Only this goroutine may CLOSE channels. Unsubscribe() only removes from the map.
//   - When the process ends, the goroutine closes all channels and sets logSubs = nil.
//   - logFile uses a separate mutex (logFileMu) to allow concurrent writes during
//     process execution while ensuring flush happens safely outside the main lock.
type ProcInfo struct {
	ID    string
	Cmd   *cmd.Cmd
	Stdin *io.PipeWriter
	sync.RWMutex
	ioDone    chan struct{}
	logSubs   map[chan LogEntry]struct{}
	logFile   *bufio.Writer
	logFileMu sync.Mutex
	logPrefix bool
}

// ProcManager manages a collection of running processes.
// It is safe to call Start, Stop, and Shutdown concurrently.
type ProcManager struct {
	sync.RWMutex
	Procs map[string]*ProcInfo
}

// NewProcManager creates a new process manager with an empty process map.
func NewProcManager() *ProcManager {
	return &ProcManager{Procs: make(map[string]*ProcInfo)}
}

// ProcStartOptions configures how a process is started.
type ProcStartOptions struct {
	Cwd           string
	Dir           string
	Env           []string
	WithStdinPipe bool
	WaitForExit   bool
	WaitForRegex  *regexp.Regexp
	WaitTimeout   *time.Duration

	LogInfoPrefix bool
	LogEchoStdin  bool
}

// LogEntry represents a single log line from a process.
type LogEntry struct {
	ts     time.Time
	stream Stream
	line   string
}

// Stream represents the source of a log line (stdout or stderr).
type Stream int

const (
	StreamNONE Stream = iota
	StreamSTDOUT
	StreamSTDERR
)

func (s Stream) String() string {
	switch s {
	case StreamSTDERR:
		return "stderr"
	case StreamSTDOUT:
		return "stdout"
	default:
		return ""
	}
}

// PublishLine sends log entries to all subscribers AND writes to the log file.
//
// Gotcha: Uses non-blocking send (select with default) to prevent slow subscribers
// from blocking the process. If a subscriber's channel buffer is full (200 items),
// the log is dropped for that subscriber but still written to file and other
// subscribers receive it. This keeps the process running even if a client disconnects.
func (p *ProcInfo) PublishLine(stream Stream, line string) {
	ts := time.Now()
	p.Lock()
	for ch := range p.logSubs {
		select {
		case ch <- LogEntry{ts: ts, line: line, stream: stream}:
		default:
		}
	}
	p.Unlock()

	if p.logFile == nil {
		return
	}
	var prefix string
	if p.logPrefix {
		prefix = fmt.Sprintf("[%6s] %s: ", stream.String(), ts.Format(time.DateTime))
	}
	p.logFileMu.Lock()
	if p.logFile != nil {
		if _, err := p.logFile.WriteString(prefix + line + "\n"); err != nil {
			slog.Warn("failed to write logline to logfile", "id", p.ID, "err", err.Error())
		}
	}
	p.logFileMu.Unlock()
}

// Subscribe creates a new channel and starts a goroutine to send buffered lines.
//
// Channel ownership: The returned channel is owned by the caller, but the
// forwarding goroutine will CLOSE it when the process exits. Do NOT close it
// in Subscribe/Unsubscribe - only the goroutine closes.
//
// If logSubs is nil (process ended), returns a pre-closed channel.
func (p *ProcInfo) Subscribe() chan LogEntry {
	ch := make(chan LogEntry, 200)
	slog.Debug("subscribing to process logs", "ch", ch, "id", p.ID, "name", p.Cmd.Name)
	p.Lock()
	defer p.Unlock()

	if p.logSubs == nil {
		close(ch)
		return ch
	}

	go func(bufferedLines []string) {
		defer func() {
			if r := recover(); r != nil {
				// Channel was closed, ignore
			}
		}()
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

// Unsubscribe removes a subscriber channel from the process.
// It is safe to call concurrently with PublishLine and other Unsubscribe calls.
func (p *ProcInfo) Unsubscribe(ch chan LogEntry) {
	slog.Debug("unsubscribing from process logs", "ch", ch, "id", p.ID, "name", p.Cmd.Name)
	p.Lock()
	defer p.Unlock()

	if p.logSubs == nil {
		return
	}
	delete(p.logSubs, ch)
}

// Start runs a new process and returns its info.
// The process stdout/stderr are captured and forwarded to subscribers and a log file.
func (pm *ProcManager) Start(name string, args []string, opts ProcStartOptions) (*ProcInfo, error) {
	id := uuid.NewString()

	pm.Lock()
	if pm.Procs == nil {
		pm.Procs = make(map[string]*ProcInfo)
	}
	activeBefore := len(pm.Procs)
	pm.Unlock()

	slog.Info("starting process",
		"id", id,
		"name", name,
		"args", args,
		"opts", opts,
		"active_procs_before", activeBefore,
		"active_procs_after", activeBefore+1,
	)
	defer slog.Info("process started",
		"id", id,
		"name", name,
		"active_procs", activeBefore+1,
	)

	logFile, err := os.Create(id + ".log")
	if err != nil {
		slog.Warn("unable to open logfile, logging disabled", "id", id, "err", err.Error())
	}
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

	var logWriter *bufio.Writer
	if logFile != nil {
		logWriter = bufio.NewWriterSize(logFile, 64*1024)
	}

	info := &ProcInfo{
		ID:        id,
		Cmd:       command,
		logSubs:   make(map[chan LogEntry]struct{}),
		Stdin:     stdinWriter,
		logFile:   logWriter,
		logPrefix: opts.LogInfoPrefix,
		ioDone:    make(chan struct{}),
	}

	pm.Lock()
	if pm.Procs == nil {
		pm.Procs = make(map[string]*ProcInfo)
	}
	pm.Procs[id] = info
	activeAfter := len(pm.Procs)
	pm.Unlock()

	slog.Info("starting process",
		"id", id,
		"name", name,
		"args", args,
		"opts", opts,
		"active_procs_before", activeBefore,
		"active_procs_after", activeAfter,
	)
	defer slog.Info("process started",
		"id", id,
		"name", name,
		"active_procs", activeAfter,
	)

	matchCh := make(chan struct{})
	var matched atomic.Bool

	// Forwarding goroutine: handles stdout/stderr, manages logSubs channels, and cleanup.
	// Lifecycle: runs until process exits, then closes all logSubs channels and ioDone.
	// IMPORTANT: This goroutine ONLY accesses per-process locks (p.Lock), never pm.Procs.
	// This is critical for Shutdown() - if it accessed pm.Procs, we'd deadlock.
	go func() {
		defer func() {
			info.logFileMu.Lock()
			if info.logFile != nil {
				if flushErr := info.logFile.Flush(); flushErr != nil {
					slog.Warn("failed to flush log buffer", "id", id, "err", flushErr.Error())
				}
			}
			info.logFileMu.Unlock()

			if err := logFile.Close(); err != nil {
				slog.Warn("failed to close log file", "id", id, "err", err.Error())
			}
			close(info.ioDone)
		}()

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
			if opts.WaitForRegex != nil && !matched.Load() && opts.WaitForRegex.MatchString(line) {
				matched.Store(true)
				close(matchCh)
			}
			if open {
				info.PublishLine(stream, line)
			}
		}

		info.Lock()
		for ch := range info.logSubs {
			close(ch)
		}
		info.logSubs = nil
		info.Unlock()

		status := info.Cmd.Status()
		slog.Info("process completed",
			"id", id,
			"name", info.Cmd.Name,
			"exit_code", status.Exit,
			"complete", status.Complete,
		)
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

// Stop terminates a running process by ID.
// If wait is true, it waits for the process to fully exit before returning.
func (pm *ProcManager) Stop(id string, wait bool) (*ProcInfo, error) {
	pm.Lock()
	activeBefore := len(pm.Procs)
	pm.Unlock()

	slog.Info("stopping process", "id", id, "active_procs_before", activeBefore)
	defer slog.Info("stopped process", "id", id, "active_procs_after", activeBefore-1)

	pm.Lock()
	p, ok := pm.Procs[id]
	pm.Unlock()
	if !ok {
		return nil, fmt.Errorf("process not found")
	}
	err := p.Cmd.Stop()
	if wait {
		select {
		case <-p.ioDone:
		case <-time.After(10 * time.Second):
			slog.Warn("timed out while stopping", "id", id, "status", p.Cmd.Status())
		}
	}
	return p, err
}

// Shutdown stops all processes.
//
// DEADLOCK WARNING: We hold pm.Lock while waiting on p.ioDone for each process.
// This works because the forwarding goroutine closes ioDone OUTSIDE any lock.
// If the goroutine tried to acquire pm.Procs lock while we hold it here, DEADLOCK.
// The goroutine only accesses p.logSubs (per-process lock), never pm.Procs.
func (pm *ProcManager) Shutdown() error {
	pm.Lock()
	activeCount := len(pm.Procs)
	pm.Unlock()

	slog.Info("shutting down process manager", "active_procs", activeCount)
	pm.Lock()
	defer pm.Unlock()
	var errs []error

	for id, p := range pm.Procs {
		slog.Info("stopping process", "id", p.ID, "remaining", len(pm.Procs))
		if err := p.Cmd.Stop(); err != nil {
			errs = append(errs, err)
		}
		<-p.ioDone
		delete(pm.Procs, id)
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
