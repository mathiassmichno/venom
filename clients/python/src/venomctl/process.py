"""Process class for OOP interface."""

from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime
from typing import Iterator

from .exceptions import ConnectionError, ProcessError, ProcessStartError


@dataclass
class Process:
    """Represents a running process on the Venom daemon.

    Attributes:
        id: Unique process identifier
        name: Process name or command
        args: Command-line arguments
        complete: Whether the process has exited
        exit_code: Exit code if process has exited
    """

    id: str
    name: str
    args: list[str]
    complete: bool = False
    exit_code: int | None = None
    _client: VenomClient | None = None  # type: ignore[assignment]

    def __post_init__(self) -> None:
        object.__setattr__(self, "_client", self._client)

    @property
    def client(self) -> VenomClient:
        """Return the client this process belongs to."""
        if self._client is None:
            raise RuntimeError("Process client not set")
        return self._client

    def logs(self, from_start: bool = True) -> Iterator[LogEntry]:
        """Stream log entries from this process.

        Args:
            from_start: If True, include logs from process start

        Yields:
            LogEntry objects with timestamp, stream, and line
        """
        return self._client._subscribe(self.id, from_start)

    def stop(self) -> bool:
        """Stop this process.

        Returns:
            True if process was stopped successfully
        """
        return self._client._stop_process(self.id)

    def send_input(self, text: str, close_stdin: bool = False) -> int:
        """Send text input to the process stdin.

        Args:
            text: Text to send
            close_stdin: Whether to close stdin after sending

        Returns:
            Number of bytes written

        Raises:
            ProcessError: If process wasn't started with stdin pipe
        """
        return self._client._send_input(self.id, text=text, close_stdin=close_stdin)

    def send_bytes(self, data: bytes, close_stdin: bool = False) -> int:
        """Send binary input to the process stdin.

        Args:
            data: Bytes to send
            close_stdin: Whether to close stdin after sending

        Returns:
            Number of bytes written

        Raises:
            ProcessError: If process wasn't started with stdin pipe
        """
        return self._client._send_input(self.id, data=data, close_stdin=close_stdin)

    def close_stdin(self) -> None:
        """Close the process stdin."""
        self._client._send_input(self.id, text="", close_stdin=True)

    def wait(self, timeout: float | None = None) -> int | None:
        """Wait for the process to exit.

        Args:
            timeout: Optional timeout in seconds

        Returns:
            Exit code if process exited, None if timeout or still running
        """
        if self.complete:
            return self.exit_code

        proc = self._client._get_process(self.id)
        if proc:
            self.exit_code = proc.exit_code
            self.complete = proc.complete
            return self.exit_code
        return None

    def __repr__(self) -> str:
        return f"Process(id={self.id!r}, name={self.name!r}, complete={self.complete})"


class LogEntry:
    """Log entry from a process stream.

    Attributes:
        timestamp: Time the log entry was generated
        stream: Stream type ('stdout' or 'stderr')
        line: Log line content
    """

    timestamp: datetime
    stream: str
    line: str

    def __init__(self, timestamp: datetime, stream: str, line: str) -> None:
        """Initialize a log entry.

        Args:
            timestamp: Time the log entry was generated
            stream: Stream type ('stdout' or 'stderr')
            line: Log line content
        """
        self.timestamp = timestamp
        self.stream = stream
        self.line = line

    def __repr__(self) -> str:
        return f"LogEntry(stream={self.stream!r}, line={self.line!r})"

    def __getitem__(self, key: str) -> str | datetime:
        """Allow dict-style access for backward compatibility."""
        return {"timestamp": self.timestamp, "stream": self.stream, "line": self.line}[key]


class VenomClient:
    """Venom gRPC client with persistent connection.

    Usage:
        with VenomClient("localhost:9988") as client:
            proc = client.start_process(name="echo", args=["hello"])
            for log in proc.logs():
                print(f"[{log.stream}] {log.line}")
            proc.stop()
    """

    def __init__(
        self,
        address: str,
        connect_timeout: float = 10.0,
        call_timeout: float = 30.0,
    ) -> None:
        """Initialize client with server address.

        Args:
            address: gRPC server address (host:port)
            connect_timeout: Timeout for establishing connection in seconds
            call_timeout: Default timeout for RPC calls in seconds
        """
        self._address = address
        self._connect_timeout = connect_timeout
        self._call_timeout = call_timeout
        self._channel = None
        self._stub = None
        self._connected = False

    def __enter__(self) -> VenomClient:
        return self

    def __exit__(self, *args: object) -> None:
        self.close()

    def close(self) -> None:
        """Close the gRPC connection."""
        if self._channel is not None:
            self._channel.close()
            self._channel = None
            self._stub = None
            self._connected = False

    def _ensure_connected(self) -> None:
        """Ensure connection is established. Lazy connection on first RPC call."""
        if self._connected:
            return

        import grpc

        self._channel = grpc.insecure_channel(
            self._address,
            options=[
                ("grpc.lb_policy_name", "pick_first"),
                ("grpc.enable_retries", 1),
            ],
        )

        try:
            grpc.channel_ready_future(self._channel).result(timeout=self._connect_timeout)
            from .venom_pb2_grpc import VenomDaemonStub

            self._stub = VenomDaemonStub(self._channel)
            self._connected = True
        except grpc.FutureTimeoutError as e:
            self._channel.close()
            self._channel = None
            raise ConnectionError(f"Connection timeout: {e}") from e
        except grpc.RpcError as e:
            if self._channel:
                self._channel.close()
                self._channel = None
            raise ConnectionError(f"Connection failed: {e}") from e

    def start_process(
        self,
        name: str,
        args: list[str] | None = None,
        cwd: str | None = None,
        env: list[str] | None = None,
        with_stdin: bool = False,
    ) -> Process:
        """Start a new process on the daemon.

        Args:
            name: Process name or path
            args: Command-line arguments
            cwd: Working directory
            env: Environment variables
            with_stdin: Enable stdin pipe for interactive input

        Returns:
            Process object

        Raises:
            ProcessStartError: If process fails to start
        """
        from .venom_pb2 import ProcessDefinition, StartProcessRequest

        self._ensure_connected()

        definition = ProcessDefinition(
            name=name,
            args=args or [],
            dir=cwd or "",
            env=env or [],
        )

        request = StartProcessRequest(definition=definition, with_stdin_pipe=with_stdin)

        try:
            response = self._stub.StartProcess(request, timeout=self._call_timeout)
            if response.success:
                return Process(
                    id=response.id,
                    name=name,
                    args=list(args) if args else [],
                    complete=False,
                    exit_code=None,
                    _client=self,
                )
            status_error = None
            if response.status.HasField("error"):
                status_error = response.status.error
            raise ProcessStartError(status_error or "Process failed to start")
        except Exception as e:
            if isinstance(e, ProcessStartError):
                raise
            raise ProcessStartError(str(e)) from e

    def _get_process(self, process_id: str) -> Process | None:
        """Get a process by ID from the daemon.

        Args:
            process_id: ID of the process to retrieve

        Returns:
            Process object if found, None otherwise
        """
        procs = self.list_processes()
        for p in procs:
            if p.id == process_id:
                return p
        return None

    def list_processes(self) -> list[Process]:
        """List all running processes.

        Returns:
            List of Process objects
        """
        from google.protobuf.empty_pb2 import Empty

        self._ensure_connected()

        try:
            response = self._stub.ListProcesses(Empty(), timeout=self._call_timeout)
            result: list[Process] = []
            for p in response.processes:
                exit_code: int | None = None
                complete = False
                if p.status.HasField("exit"):
                    exit_code = p.status.exit
                    complete = True
                elif p.status.HasField("error"):
                    complete = True

                result.append(
                    Process(
                        id=p.id,
                        name=p.definition.name,
                        args=list(p.definition.args),
                        complete=complete,
                        exit_code=exit_code,
                        _client=self,
                    )
                )
            return result
        except Exception:
            return []

    def get_metrics(self) -> dict | None:
        """Get system metrics from the daemon.

        Returns:
            Dictionary with cpu, mem_percent, mem_total, mem_used keys,
            or None on error
        """
        from google.protobuf.empty_pb2 import Empty

        self._ensure_connected()

        try:
            response = self._stub.GetMetrics(Empty(), timeout=self._call_timeout)
            return {
                "cpu": response.cpu_percent,
                "mem_percent": response.mem_percent,
                "mem_total": response.mem_total,
                "mem_used": response.mem_used,
            }
        except Exception:
            return None

    def _subscribe(self, process_id: str, from_start: bool = True) -> Iterator[LogEntry]:
        """Stream logs for a process.

        Args:
            process_id: ID of the process to stream logs from
            from_start: If True, include logs from process start

        Yields:
            LogEntry objects
        """
        from datetime import datetime as dt

        from .venom_pb2 import StreamLogsRequest

        self._ensure_connected()

        request = StreamLogsRequest(id=process_id, from_start=from_start)

        try:
            for response in self._stub.StreamLogs(request, timeout=self._call_timeout):
                stream_name = "stdout"
                if response.stream == response.STDERR:
                    stream_name = "stderr"

                yield LogEntry(
                    timestamp=dt.fromtimestamp(response.ts.seconds + response.ts.nanos / 1e9),
                    stream=stream_name,
                    line=response.line,
                )
        except Exception:
            pass

    def _stop_process(self, process_id: str) -> bool:
        """Stop a process by ID.

        Args:
            process_id: ID of the process to stop

        Returns:
            True if stopped successfully, False otherwise
        """
        from .venom_pb2 import StopProcessRequest

        self._ensure_connected()

        request = StopProcessRequest(id=process_id, wait=False)

        try:
            response = self._stub.StopProcess(request, timeout=self._call_timeout)
            return response.success
        except Exception:
            return False

    def _send_input(
        self,
        process_id: str,
        text: str | None = None,
        data: bytes | None = None,
        close_stdin: bool = False,
    ) -> int:
        """Send input to a process stdin.

        Args:
            process_id: ID of the process
            text: Text to send (mutually exclusive with data)
            data: Bytes to send (mutually exclusive with text)
            close_stdin: Whether to close stdin after sending

        Returns:
            Number of bytes written

        Raises:
            ProcessError: If send fails
        """
        from .venom_pb2 import ProcessInput

        self._ensure_connected()

        request = ProcessInput(id=process_id)
        if text is not None:
            request.text = text
        elif data is not None:
            request.data = data
        request.close = close_stdin

        try:
            response = self._stub.SendProcessInput(request, timeout=self._call_timeout)
            return response.written
        except Exception as e:
            raise ProcessError(str(e)) from e
