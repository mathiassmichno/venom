package daemon

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
)

type LogChunk struct {
	Stream string
	Data   []byte
	Time   time.Time
}

type ProcStart struct {
	ID     string
	Cmd    string
	Args   []string
	Cwd    string
	Env    map[string]string
	UsePTY bool
}

type ProcInfo struct {
	ID        string
	Cmd       *exec.Cmd
	Pid       int
	StartedAt time.Time
	StoppedAt time.Time
	ExitErr   error
	logSubs   []chan LogChunk
	logChunks []LogChunk
	mu        sync.Mutex
}

type ProcManager struct {
	mu    sync.Mutex
	procs map[string]*ProcInfo
}

func NewProcManager() *ProcManager {
	return &ProcManager{procs: make(map[string]*ProcInfo)}
}

func (pm *ProcManager) Start(id string, cmd string, args []string, cwd string, env map[string]string) (*ProcInfo, error) {
	if id == "" {
		id = uuid.NewString()
	}

	command := exec.Command(cmd, args...)
	command.Dir = cwd
	for k, v := range env {
		command.Env = append(command.Env, fmt.Sprintf("%s=%s", k, v))
	}

	stdout, _ := command.StdoutPipe()
	stderr, _ := command.StderrPipe()

	if err := command.Start(); err != nil {
		return nil, err
	}

	info := &ProcInfo{ID: id, Cmd: command, Pid: command.Process.Pid, StartedAt: time.Now()}
	pm.mu.Lock()
	pm.procs[id] = info
	pm.mu.Unlock()

	go pm.readStream(id, "stdout", stdout)
	go pm.readStream(id, "stderr", stderr)
	go func() {
		info.ExitErr = command.Wait()
		info.StoppedAt = time.Now()
	}()

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

func (pm *ProcManager) readStream(id, stream string, r io.Reader) {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		pm.broadcast(id, stream, sc.Bytes())
	}
}

func (pm *ProcManager) broadcast(id, stream string, data []byte) {
	pm.mu.Lock()
	p, ok := pm.procs[id]
	pm.mu.Unlock()
	if !ok {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	chunk := LogChunk{Stream: stream, Data: append([]byte{}, data...), Time: time.Now()}
	p.logChunks = append(p.logChunks, chunk)
	for _, c := range p.logSubs {
		select {
		case c <- chunk:
		default:
		}
	}
}

func (pm *ProcManager) SubscribeLogs(id string, fromStart bool, ch chan LogChunk) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	p, ok := pm.procs[id]
	if !ok {
		return fmt.Errorf("process not found")
	}
	p.mu.Lock()
	p.logSubs = append(p.logSubs, ch)
	if fromStart {
		for _, chunk := range p.logChunks {
			ch <- chunk
		}
	}
	p.mu.Unlock()
	return nil
}

func (pm *ProcManager) UnsubscribeLogs(id string, ch chan LogChunk) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	p, ok := pm.procs[id]
	if !ok {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	filtered := p.logSubs[:0]
	for _, sub := range p.logSubs {
		if sub != ch {
			filtered = append(filtered, sub)
		}
	}
	p.logSubs = filtered
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
