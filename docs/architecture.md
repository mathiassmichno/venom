# Architecture

## Overview

Venom follows a client-server architecture:
- **venomd** (server): Process management daemon exposing gRPC API
- **venomctl** (Go client): CLI for interacting with the daemon
- **venom_client** (Python client): Python library for the daemon

```
┌─────────────┐     gRPC      ┌─────────────┐
│  venomctl   │ ─────────────>│   venomd    │
│ (Go client) │               │  (daemon)   │
└─────────────┘               └──────┬──────┘
                                     │
┌─────────────┐                     │
│ venom_client│ ────────────────────┘
│(Python lib) │
└─────────────┘
```

## Components

### Daemon (`cmd/venomd/main.go`)

The main entry point that:
1. Sets up signal handling (SIGINT, SIGTERM)
2. Starts pprof HTTP server on `localhost:6060`
3. Creates the ProcessManager
4. Starts the gRPC server with reflection enabled
5. Handles graceful shutdown

### Process Manager (`daemon/procmanager.go`)

Core process management:
- **ProcManager**: Manages a map of running processes (`map[string]*ProcInfo`)
- **ProcInfo**: Per-process state including:
  - Command reference (`*cmd.Cmd`)
  - Stdin pipe (`*io.PipeWriter`)
  - Log subscriptions (`map[chan LogEntry]struct{}`)
  - Log file writer (`*bufio.Writer`)

**Key operations:**
- `Start()`: Spawns a new process, sets up stdout/stderr forwarding, creates log file
- `Stop()`: Signals process termination, optionally waits for exit
- `Subscribe()` / `Unsubscribe()`: Manage log subscribers
- `Shutdown()`: Terminates all processes on daemon shutdown

### Server (`daemon/server.go`)

gRPC service implementation that bridges RPC requests to ProcManager:
- `StartProcess`: Delegates to `pm.Start()`
- `StopProcess`: Delegates to `pm.Stop()`
- `SendProcessInput`: Writes to process stdin pipe
- `StreamLogs`: Subscribes to log channel and streams to client
- `ListProcesses`: Returns snapshot of all processes
- `GetMetrics`: Collects CPU/memory metrics

### Monitor (`daemon/monitor.go`)

System metrics collection using `gopsutil`:
- CPU percentage
- Memory usage (total, used, percentage)

---

## Concurrency Patterns

### Channel Ownership (logSubs)

The `logSubs` map uses Go's channel ownership pattern:

- **Single sender**: Only the goroutine spawned in `Start()` (the "stdout/stderr forwarding goroutine") closes channels
- **Multiple receivers**: `Subscribe()` creates new channels; `Unsubscribe()` only removes from the map
- **Channel lifecycle**: When a process ends, the goroutine closes all remaining channels and sets `logSubs = nil`

This prevents double-close panics. If `logSubs` is `nil`, `Subscribe()` returns a closed channel.

### Mutex Protection

- **`ProcInfo.RWMutex`**: Protects `logSubs` map and process state
- **`logFileMu sync.Mutex`**: Separately protects `logFile` (bufio.Writer) to allow concurrent writes during process execution while ensuring flush happens safely
- **`ProcManager.RWMutex`**: Protects the `Procs` map

### Map Iteration Safety

When iterating over `Procs` map that may be modified concurrently:
```go
// Take a snapshot under lock, then process outside the lock
pm.RLock()
snapshot := make(map[string]*ProcInfo, len(pm.Procs))
for k, v := range pm.Procs {
    snapshot[k] = v
}
pm.RUnlock()
for _, info := range snapshot {
    // Safe to process
}
```

---

## Process Lifecycle

1. **Start**: Client sends `StartProcessRequest`
   - ProcManager creates process with `cmd.NewCmd()`
   - Spawns goroutine to forward stdout/stderr to log file and subscribers
   - Optionally waits for exit or regex match

2. **Run**: Process executes, logs streamed to:
   - Log file (`{process_id}.log`)
   - Subscribers via channels

3. **Stop**: Client sends `StopProcessRequest`
   - Sends SIGTERM (or custom signal) to process
   - Optionally waits for graceful exit
   - Cleans up stdin pipe, log file

4. **Cleanup**: When process ends:
   - Forwarding goroutine closes all log channels
   - Log file flushed and closed
   - Process removed from ProcManager map

---

## Log Streaming

```
Process stdout/stderr
        │
        ▼
┌───────────────────┐
│ Forwarding        │  Goroutine
│ Goroutine         │
└────────┬──────────┘
         │
    ┌────┴────┐
    ▼         ▼
┌───────┐ ┌───────┐
│  Log  │ │  Ch1  │ ──> Subscriber 1
│ File  │ └───────┘
└───────┘ ┌───────┐
          │  Ch2  │ ──> Subscriber 2
          └───────┘
```

- Each process has one log file
- Each subscriber gets their own channel
- Forwarding goroutine distributes to all channels and file

---

## gRPC API

The daemon exposes these RPCs:
- `StartProcess` - Start a new process
- `StopProcess` - Stop a running process
- `SendProcessInput` - Send input to stdin
- `ListProcesses` - List all processes
- `StreamLogs` - Stream log output (server-side streaming)
- `GetMetrics` - Get system metrics

### Debugging

- gRPC reflection enabled - use `grpcurl` to list services
- pprof available at `localhost:6060`
- Process logs written to `{process_id}.log` files in daemon working directory

---

## Dependencies

- `github.com/go-cmd/cmd` - Process command execution
- `github.com/urfave/cli/v3` - CLI framework
- `google.golang.org/grpc` - gRPC server/client
- `github.com/shirou/gopsutil/v3` - System metrics
- `github.com/google/uuid` - UUID generation