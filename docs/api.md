# API Reference

## gRPC Service

The daemon exposes a `VenomDaemon` gRPC service on `localhost:9988`.

### Service Definition

```proto
service VenomDaemon {
  rpc StartProcess(StartProcessRequest) returns (StartProcessResponse);
  rpc StopProcess(StopProcessRequest) returns (StopProcessResponse);
  rpc SendProcessInput(ProcessInput) returns (ProcessInputWritten);
  rpc ListProcesses(google.protobuf.Empty) returns (ListProcessesResponse);
  rpc StreamLogs(StreamLogsRequest) returns (stream LogEntry);
  rpc GetMetrics(google.protobuf.Empty) returns (MetricsResponse);
}
```

---

## RPC Methods

### StartProcess

Starts a new process.

**Request:**

```proto
message StartProcessRequest {
  ProcessDefinition definition = 1;
  bool with_stdin_pipe = 2;
  oneof wait_for {
    string regex = 3;
    bool exit = 4;
  }
  optional google.protobuf.Duration wait_timeout = 5;
}

message ProcessDefinition {
  string name = 1;
  repeated string args = 2;
  string dir = 3;
  repeated string env = 4;
}
```

**Response:**

```proto
message StartProcessResponse {
  string id = 1;
  bool success = 2;
  ProcessStatus status = 3;
}

message ProcessStatus {
  oneof state {
    uint32 exit = 1;
    string error = 2;
  }
  google.protobuf.Duration runtime = 6;
}
```

---

### StopProcess

Stops a running process.

**Request:**

```proto
message StopProcessRequest {
  string id = 1;
  bool wait = 2;
}
```

**Response:**

```proto
message StopProcessResponse {
  bool success = 1;
  ProcessStatus status = 2;
}
```

---

### SendProcessInput

Sends input to a process's stdin. Requires the process to be started with `with_stdin_pipe: true`.

**Request:**

```proto
message ProcessInput {
  string id = 1;
  oneof input {
    string text = 2;
    bytes data = 3;
  }
  bool close = 4;
}
```

**Response:**

```proto
message ProcessInputWritten {
  bool success = 1;
  uint32 written = 2;
  oneof result {
    string error = 3;
    ProcessStatus status = 4;
  }
}
```

---

### ListProcesses

Lists all managed processes.

**Response:**

```proto
message ListProcessesResponse {
  repeated ProcessInfo processes = 1;
}

message ProcessInfo {
  string id = 1;
  ProcessDefinition definition = 2;
  ProcessStatus status = 3;
}
```

---

### StreamLogs

Streams log output from a process. Returns a server-side stream.

**Request:**

```proto
message StreamLogsRequest {
  string id = 1;
  bool from_start = 2;
}
```

**Response (stream):**

```proto
message LogEntry {
  google.protobuf.Timestamp ts = 1;
  string line = 2;
  Stream stream = 3;
  enum Stream {
    STREAM_UNSPECIFIED = 0;
    STDOUT = 1;
    STDERR = 2;
  }
}
```

---

### GetMetrics

Returns system resource usage metrics.

**Response:**

```proto
message MetricsResponse {
  google.protobuf.Timestamp ts = 1;
  double cpu_percent = 2;
  double mem_percent = 3;
  uint64 mem_total = 4;
  uint64 mem_used = 5;
}
```

---

## Go CLI (venomctl)

### Global Flags

| Flag | Env Variable | Default | Description |
|------|--------------|---------|-------------|
| `--addr` | `VENOMD_ADDR` | `localhost:9988` | Daemon address |
| `--timeout` | - | 60s | RPC timeout |

### Commands

#### start

```bash
venomctl start [options] <name> [args...]
```

**Flags:**

- `--cwd <dir>` - Working directory
- `--env "KEY=value"` - Environment variable (repeatable)
- `--with-stdin` - Enable stdin pipe for interactive input
- `--wait-for-exit` - Wait for process to exit
- `--wait-for-regex <regex>` - Wait for output to match regex
- `--wait-timeout <duration>` - Timeout for wait operations

**Example:**

```bash
./bin/venomctl start --with-stdin sh -c "read line; echo got: $line"
./bin/venomctl start --wait-for-exit sleep 5
```

#### list

```bash
venomctl list
```

Lists all running processes.

#### logs

```bash
venomctl logs <id>
```

Streams logs for a process (stdout/stderr).

#### stop

```bash
venomctl stop <id>
```

Stops a running process.

#### input

```bash
venomctl input [options] <id> <input>
```

**Flags:**

- `--close-stdin`, `-c` - Close stdin after sending

#### version

```bash
venomctl version
```

Shows client version.

---

## Python Client

### VenomClient

```python
from venomctl import VenomClient
```

#### Constructor

```python
VenomClient(
    address: str = "localhost:9988",
    connect_timeout: float = 10.0,
    call_timeout: float = 30.0,
)
```

#### Methods

**start_process()**

```python
client.start_process(
    name: str,
    args: list[str] | None = None,
    cwd: str | None = None,
    env: list[str] | None = None,
    with_stdin: bool = False,
) -> Process
```

Returns a `Process` object. Raises `ProcessStartError` on failure.

**list_processes()**

```python
client.list_processes() -> list[Process]
```

Returns a list of `Process` objects.

**get_metrics()**

```python
client.get_metrics() -> dict | None
```

Returns a metrics dictionary:

```python
{
    "cpu": float,
    "mem_percent": float,
    "mem_total": int,
    "mem_used": int,
}
```

---

### Process

Represents a running process on the daemon.

#### Attributes

| Attribute | Type | Description |
|-----------|------|-------------|
| `id` | `str` | Unique process identifier |
| `name` | `str` | Process name or command |
| `args` | `list[str]` | Command-line arguments |
| `complete` | `bool` | Whether the process has exited |
| `exit_code` | `int \| None` | Exit code if process has exited |

#### Methods

**logs()**

```python
process.logs(from_start: bool = True) -> Iterator[LogEntry]
```

Stream log entries from the process.

Yields `LogEntry` objects:

```python
log.stream  # "stdout" or "stderr"
log.line    # log line content
log.timestamp  # datetime object
```

**stop()**

```python
process.stop() -> bool
```

Stop the process. Returns `True` on success.

**send_input()**

```python
process.send_input(text: str, close_stdin: bool = False) -> int
```

Send text input to the process stdin. Returns bytes written.

**send_bytes()**

```python
process.send_bytes(data: bytes, close_stdin: bool = False) -> int
```

Send binary input to the process stdin.

**close_stdin()**

```python
process.close_stdin() -> None
```

Close the process stdin.

**wait()**

```python
process.wait(timeout: float | None = None) -> int | None
```

Wait for the process to exit. Returns exit code or `None`.

---

### Exceptions

```python
from venomctl import (
    ProcessError,
    ProcessStartError,
    ProcessStopError,
    ProcessNotFoundError,
    ConnectionError,
)
```

---

#### Usage Example

```python
from venomctl import VenomClient

with VenomClient("localhost:9988") as client:
    # Start a process - returns Process object
    proc = client.start_process(name="echo", args=["hello world"])
    print(f"Started: {proc.id}")
    
    # Stream logs
    for log in proc.logs():
        print(f"[{log.stream}] {log.line}")
    
    # Stop it
    proc.stop()

# With stdin pipe
with VenomClient("localhost:9988") as client:
    proc = client.start_process(
        name="sh",
        args=["-c", "read line; echo got: $line"],
        with_stdin=True
    )
    proc.send_input("hello")
    proc.close_stdin()
    for log in proc.logs():
        print(f"[{log.stream}] {log.line}")
```

---

## Connection Notes

- The gRPC server uses insecure credentials (no TLS)
- gRPC reflection is enabled for debugging (use `grpcurl` or similar)
- pprof available at `localhost:6060`

