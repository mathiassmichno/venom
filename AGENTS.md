# Agent Guidelines for Venom

Venom is a Go-based process management daemon with a Python client library. This file provides guidelines for agents working on this codebase.

## Project Structure

```
venom/
├── cmd/venomd/          # Daemon entry point
├── clients/
│   ├── go/venomctl/    # Go CLI client
│   └── python/         # Python client library
├── daemon/             # Core daemon logic (procmanager, server, monitor)
├── proto/              # Protobuf definitions and generated code
├── scripts/            # Helper scripts
└── Makefile            # Build automation
```

## Build Commands

### Go Build
```bash
# Build all binaries
make build
# or
go build -o bin/venomd ./cmd/venomd
go build -o bin/venomctl ./clients/go/venomctl

# Single package build
go build ./cmd/venomd
go build ./clients/go/venomctl
```

### Go Test
```bash
# Run all tests
make test
# or
go test ./...

# Run single test file
go test ./daemon/procmanager_test.go ./daemon/procmanager.go ./daemon/server.go

# Run single test function
go test -run TestProcessStart ./daemon/...
go test -run "TestProcess" ./daemon/...        # Run all tests matching pattern
go test -v -run TestProcessStart ./daemon/...  # Verbose output

# Run tests with coverage
go test -cover ./...

# Run tests with race detector (IMPORTANT for catching concurrency bugs)
go test -race ./...
```

### Protobuf Generation
```bash
make proto
```

### Python Client
```bash
# Build Python package
make python-client
cd clients/python && python -m build

# Run Python tests
cd clients/python && python -m pytest tests/ -v
```

## Code Style Guidelines

### Go

1. **Formatting**: Use `gofmt` (standard Go formatting). Run before committing:
   ```bash
   go fmt ./...
   ```

2. **Imports**: Standard Go import organization (stdlib first, then third-party):
   ```go
   import (
       "bufio"
       "fmt"
       "io"
       "log/slog"
       "os"
       "sync"
       "time"

       "github.com/go-cmd/cmd"
       "github.com/google/uuid"
   )
   ```

3. **Error Handling**:
   - Return errors as last return value
   - Use `fmt.Errorf` with `%w` for wrapped errors
   - Log errors with appropriate level (`slog.Warn`, `slog.Error`)
   - Use `errors.Join` for multiple errors

4. **Naming Conventions**:
   - PascalCase for exported names (types, functions)
   - camelCase for unexported names
   - Package names should be short, lowercase
   - Acronyms should follow Go conventions (URL, API, gRPC)

 5. **Concurrency**:
    - Use `sync.RWMutex` for read-heavy concurrent access
    - Always use `defer` for unlocking mutexes when possible
    - Use `context.Context` for cancellation and timeouts
    - Close channels from the sender side
    - Use `sync/atomic` types (`atomic.Bool`, `atomic.Int64`, etc.) for simple shared state
    - When iterating over a map that may be modified concurrently, take a snapshot under lock first:
      ```go
      s.pm.RLock()
      snapshot := make(map[string]*ProcInfo, len(s.pm.Procs))
      for k, v := range s.pm.Procs {
          snapshot[k] = v
      }
      s.pm.RUnlock()
      // Iterate over snapshot
      ```

6. **Logging**: Use `log/slog` for structured logging:
   ```go
   slog.Info("starting process", "id", id, "name", name)
   slog.Warn("failed to close log file", "id", id, "err", err.Error())
   ```

7. **gRPC Services**: Use `urfave/cli/v3` for CLI applications

### Python

1. **Formatting**: Follow PEP 8 guidelines

2. **Type Hints**: Use type hints where beneficial (optional, project doesn't enforce)

3. **Package Structure**: Use `venom_client` as the package name

4. **Protobuf**: Generated code lives in `venom_client/venom_pb2.py` and `venom_client/venom_pb2_grpc.py`

## Testing Guidelines

### Go Tests
- Test files are named `*_test.go`
- Use standard `testing` package
- Test functions: `func TestName(t *testing.T)`
- Use `t.Fatalf` for fatal errors during setup
- Use `t.Errorf` for assertion failures
- Use `t.Logf` for debug output

Example test structure:
```go
func TestProcessStart(t *testing.T) {
    pm := NewProcManager()
    info, err := pm.Start("cat", []string{}, ProcStartOptions{})
    if err != nil {
        t.Fatalf("Failed to start process: %v", err)
    }
    // assertions...
}
```

### Running Tests
```bash
# All tests
go test ./...

# Specific package
go test ./daemon/...

# Single test
go test -v -run TestProcessStart ./daemon/...

# With coverage
go test -cover ./...
```

## Protobuf Development

When modifying `.proto` files:
1. Edit `proto/venom.proto`
2. Run `make proto` to regenerate code
3. Commit both the `.proto` file and generated code

## Key Dependencies

- `github.com/go-cmd/cmd` - Process command execution
- `github.com/urfave/cli/v3` - CLI framework
- `google.golang.org/grpc` - gRPC server/client
- `github.com/shirou/gopsutil/v3` - System metrics
- `github.com/google/uuid` - UUID generation

## Common Tasks

### Add a new gRPC method
1. Define RPC in `proto/venom.proto`
2. Run `make proto`
3. Implement server method in `daemon/server.go`
4. Add CLI subcommand in `clients/go/venomctl/main.go` if needed

### Add a new daemon subsystem
1. Create package in `daemon/`
2. Export constructor function (e.g., `NewXxx()`)
3. Integrate in `cmd/venomd/main.go`
4. Add tests in `daemon/*_test.go`

## Notes
- The daemon uses gRPC reflection for debugging (enabled in `venomd`)
- pprof is available at `localhost:6060`
- Process logs are written to `{process_id}.log` files
- The Python client is generated from protobuf definitions

## Architecture Patterns

### Channel Ownership (ProcInfo.logSubs)

The `logSubs` map and its channels follow Go's channel ownership pattern:
- **Single sender**: Only the goroutine spawned in `Start()` (the "stdout/stderr forwarding goroutine") closes channels
- **Multiple receivers**: `Subscribe()` creates new channels; `Unsubscribe()` only removes from the map
- **Channel lifecycle**: When a process ends, the goroutine closes all remaining channels and sets `logSubs = nil`

This prevents double-close panics. If `logSubs` is `nil`, `Subscribe()` returns a closed channel.

### Mutex Protection

- `ProcInfo.RWMutex` protects `logSubs` map and process state
- `logFileMu sync.Mutex` separately protects `logFile` (bufio.Writer) to allow concurrent writes during process execution while ensuring flush happens safely

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
