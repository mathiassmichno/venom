# Venom

A Go-based process management daemon with Python and Go client libraries.

## Overview

Venom is a daemon (`venomd`) that manages long-running processes and exposes control via gRPC. It provides:

- Process lifecycle management (start, stop, restart)
- Real-time log streaming (stdout/stderr)
- Interactive stdin forwarding
- System metrics collection
- Python and Go client libraries

## Quick Start

### Prerequisites

- Go 1.21+
- Python 3.10+ (for Python client)
- `protoc` (for protobuf generation)

### Build

```bash
make build
# or
go build -o bin/venomd ./cmd/venomd
go build -o bin/venomctl ./clients/go/venomctl
```

### Run the Daemon

```bash
./bin/venomd --port 9988
```

The daemon listens on `localhost:9988` by default.

### Use the CLI

```bash
# Start a process
./bin/venomctl start echo "hello world"

# List running processes
./bin/venomctl list

# Stream logs
./bin/venomctl logs <process-id>

# Stop a process
./bin/venomctl stop <process-id>

# Send input to process (with stdin pipe)
./bin/venomctl start --with-stdin sh -c "read line; echo got: $line"
./bin/venomctl input <process-id> "hello"
```

### Use the Python Client

```python
from venomctl import VenomClient

with VenomClient("localhost:9988") as client:
    proc = client.start_process(name="echo", args=["hello"])
    for log in proc.logs():
        print(f"[{log.stream}] {log.line}")
    proc.stop()
```

## Components

| Component | Description |
|-----------|-------------|
| `venomd` | The daemon server (gRPC service) |
| `venomctl` | Go CLI client and Python client library |
| `daemon/` | Core daemon logic (procmanager, server, monitor) |
| `proto/` | Protocol buffer definitions |

## Environment Variables

- `VENOMD_ADDR` - Daemon address (default: `localhost:9988`)

## Development

See [docs/developing.md](docs/developing.md) for detailed development instructions.

