package daemon

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
)

type ProcInfo struct {
	ID        string
	Cmd       *exec.Cmd
	Pid       int
	StartedAt time.Time
	StoppedAt time.Time
	ExitErr   error
	Logs      *LogBuffer
	mu        sync.Mutex
}

type ProcManager struct {
	mu    sync.RWMutex
	procs map[string]*ProcInfo
}

func NewProcManager() *ProcManager {
	return &ProcManager{procs: make(map[string]*ProcInfo)}
}

type ProcStartOptions struct {
	ID             string
	Cmd            string
	Args           []string
	Cwd            string
	Env            map[string]string
	WaitFor        *regexp.Regexp // optional
	WaitForTimeout time.Duration  // optional
}

func (pm *ProcManager) Start(opts ProcStartOptions) (*ProcInfo, error) {
	id := opts.ID
	if id == "" {
		id = uuid.NewString()
	}

	cmd := exec.Command(opts.Cmd, opts.Args...)
	cmd.Dir = opts.Cwd
	for k, v := range opts.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	info := &ProcInfo{
		ID:        id,
		Cmd:       cmd,
		Pid:       cmd.Process.Pid,
		StartedAt: time.Now(),
		Logs:      NewLogBuffer(10_000), // 10k lines buffer
	}

	pm.mu.Lock()
	if pm.procs == nil {
		pm.procs = make(map[string]*ProcInfo)
	}
	pm.procs[id] = info
	pm.mu.Unlock()

	// Monitor for process exit
	go func() {
		info.ExitErr = cmd.Wait()
		info.StoppedAt = time.Now()
	}()

	matchCh := make(chan struct{})
	matched := false

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := "[stdout] " + scanner.Text()
			info.Logs.Add(line)
			if opts.WaitFor != nil && !matched && opts.WaitFor.MatchString(line) {
				matched = true
				close(matchCh)
			}
		}
	}()

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := "[stderr] " + scanner.Text()
			info.Logs.Add(line)
			if opts.WaitFor != nil && !matched && opts.WaitFor.MatchString(line) {
				matched = true
				close(matchCh)
			}
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

func (pm *ProcManager) Stop(id string, sig int32, wait bool) (*os.ProcessState, error) {
	pm.mu.Lock()
	p, ok := pm.procs[id]
	pm.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("process not found")
	}
	err := p.Cmd.Process.Signal(syscall.Signal(sig))
	if wait {
		return p.Cmd.Process.Wait()
	}
	return nil, err
}

func (pm *ProcManager) Shutdown() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	for _, p := range pm.procs {
		if runtime.GOOS == "windows" {
			_ = p.Cmd.Process.Signal(os.Kill)
		} else {
			go func() {
				time.Sleep(5 * time.Second)
				err := p.Cmd.Process.Signal(os.Kill)
				if err != nil {
					fmt.Printf("MMM %s\n", err)
				}
			}()
			_ = p.Cmd.Process.Signal(os.Interrupt)
		}
	}
	return nil
}
