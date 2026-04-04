# Developing

## Prerequisites

- Go 1.21+
- Python 3.10+
- `protoc` (Protocol Buffer compiler)
- `uv` (optional, for Python development)

## Development Setup

### Install Dependencies

```bash
make install-deps
```

This installs:
- Go dependencies via `go mod download`
- `protoc-gen-go` and `protoc-gen-go-grpc` for protobuf generation
- Python dependencies: grpcio, grpcio-tools, build, pytest

### Generate Protobuf Code

```bash
make proto
```

This generates:
- Go code in `proto/generated/go/`
- Python code in `clients/python/venom_client/`

**Note**: Re-run `make proto` whenever you modify `proto/venom.proto`.

---

## Building

### Build All Binaries

```bash
make build
```

Produces:
- `bin/venomd` - Daemon server
- `bin/venomctl` - Go CLI client

### Build Individual Binaries

```bash
go build -o bin/venomd ./cmd/venomd
go build -o bin/venomctl ./clients/go/venomctl
```

### Build Python Client

```bash
make python-client
```

Creates a distributable wheel in `clients/python/dist/`.

---

## Running

### Start the Daemon

```bash
./bin/venomd --port 9988
```

Options:
- `--host <host>` - Host to listen on (default: `localhost`)
- `--port <port>` - Port to listen on (default: `9988`)

### Run the CLI

```bash
./bin/venomctl --addr localhost:9988 <command>
```

### Use Python Client

```bash
cd clients/python
uv run python examples/basic_client.py
```

---

## Testing

### Run All Tests

```bash
make test
```

### Run Go Tests

```bash
go test ./...
```

Run specific tests:
```bash
go test -run TestProcessStart ./daemon/...
go test -v ./daemon/...
```

With race detector (recommended for catching concurrency bugs):
```bash
go test -race ./...
```

### Run Python Tests

```bash
cd clients/python && uv run pytest tests/ -v
```

---

## Code Style

### Go

- **Formatting**: Use `gofmt` before committing
  ```bash
  go fmt ./...
  ```

- **Imports**: Standard Go import order (stdlib, then third-party)
  ```go
  import (
      "bufio"
      "fmt"
      "io"
      "log/slog"
      "os"
      "sync"

      "github.com/go-cmd/cmd"
      "github.com/google/uuid"
  )
  ```

- **Error Handling**: Return errors as last return value
  ```go
  if err != nil {
      return nil, fmt.Errorf("failed to start: %w", err)
  }
  ```

- **Logging**: Use `log/slog` for structured logging
  ```go
  slog.Info("starting process", "id", id, "name", name)
  slog.Warn("failed to close log file", "id", id, "err", err.Error())
  ```

- **Concurrency**:
  - Use `sync.RWMutex` for read-heavy concurrent access
  - Always use `defer` for unlocking mutexes when possible
  - Use `context.Context` for cancellation and timeouts
  - Close channels from the sender side

### Python

- Follow PEP 8 guidelines
- Use type hints where beneficial

---

## Adding New Features

### Add a New gRPC Method

1. **Define RPC in proto file** (`proto/venom.proto`):
   ```proto
   rpc NewMethod(NewMethodRequest) returns (NewMethodResponse);
   ```

2. **Run `make proto`** to regenerate code

3. **Implement server method** in `daemon/server.go`:
   ```go
   func (s *Server) NewMethod(ctx context.Context, req *pb.NewMethodRequest) (*pb.NewMethodResponse, error) {
       // implementation
   }
   ```

4. **Add CLI subcommand** in `clients/go/venomctl/main.go` (if CLI access is needed)

### Add a New Daemon Subsystem

1. Create package in `daemon/`
2. Export constructor function (e.g., `NewXxx()`)
3. Integrate in `cmd/venomd/main.go`
4. Add tests in `daemon/*_test.go`

---

## Debugging

### gRPC Reflection

The daemon has gRPC reflection enabled. Use `grpcurl` to explore:

```bash
# List services
grpcurl -plaintext localhost:9988 list

# List methods for VenomDaemon
grpcurl -plaintext localhost:9988 list venom.VenomDaemon

# Inspect proto
grpcurl -plaintext localhost:9988 describe venom.VenomDaemon
```

### pprof

CPU and memory profiling available at `localhost:6060`:

```bash
go tool pprof http://localhost:6060/debug/pprof/heap
```

### Log Files

Each process writes logs to `{process_id}.log` in the daemon's working directory.

---

## Common Tasks

| Task | Command |
|------|---------|
| Build | `make build` |
| Test | `make test` |
| Proto gen | `make proto` |
| Clean | `make clean` |
| Install binaries | `make install` |
| Full dev setup | `make dev-setup` |