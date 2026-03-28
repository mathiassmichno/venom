"""Venom gRPC client with persistent connection."""

from __future__ import annotations

import logging
from datetime import datetime
from typing import Iterator

import grpc
from google.protobuf.empty_pb2 import Empty

from .venom_pb2_grpc import VenomDaemonStub
from .venom_pb2 import (
    ProcessDefinition,
    StartProcessRequest,
    StreamLogsRequest,
    StopProcessRequest,
)
from .types import (
    LogEntry,
    Metrics,
    ProcessInfo,
    StartResult,
    StopResult,
)

logger = logging.getLogger(__name__)


class VenomClient:
    """Venom gRPC client with persistent connection.

    Usage:
        with VenomClient("localhost:9988") as client:
            result = client.start_process(name="echo", args=["hello"])
            if result["success"]:
                for log in client.subscribe(result["id"]):
                    print(log)
    """

    def __init__(
        self,
        address: str,
        connect_timeout: float = 10.0,
        call_timeout: float = 30.0,
    ):
        """Initialize client with server address.

        Args:
            address: gRPC server address (host:port)
            connect_timeout: Timeout for establishing connection in seconds
            call_timeout: Default timeout for RPC calls in seconds
        """
        self._address = address
        self._connect_timeout = connect_timeout
        self._call_timeout = call_timeout
        self._channel: grpc.Channel | None = None
        self._stub: VenomDaemonStub | None = None
        self._connected = False

    def __enter__(self) -> VenomClient:
        """Context manager entry."""
        return self

    def __exit__(self, *args) -> None:
        """Close connection on exit."""
        self.close()

    def close(self) -> None:
        """Close the gRPC connection."""
        if self._channel is not None:
            self._channel.close()
            self._channel = None
            self._stub = None
            self._connected = False
            logger.debug("Connection closed")

    def _ensure_connected(self) -> None:
        """Ensure connection is established. Lazy connection on first RPC call."""
        if self._connected:
            return

        logger.debug(f"Connecting to {self._address}")
        self._channel = grpc.insecure_channel(
            self._address,
            options=[
                ("grpc.lb_policy_name", "pick_first"),
                ("grpc.enable_retries", 1),
            ],
        )

        try:
            grpc.channel_ready_future(self._channel).result(
                timeout=self._connect_timeout
            )
            self._stub = VenomDaemonStub(self._channel)
            self._connected = True
            logger.debug("Connected successfully")
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
        wait_for_exit: bool = False,
        wait_for_regex: str | None = None,
        timeout: float = 60.0,
    ) -> StartResult:
        """Start a new process on the daemon.

        Args:
            name: Process name or path
            args: Command-line arguments
            cwd: Working directory
            env: Environment variables
            wait_for_exit: Block until process exits
            wait_for_regex: Block until regex matches output
            timeout: Timeout for wait operations

        Returns:
            StartResult with success status and process id or error
        """
        self._ensure_connected()

        definition = ProcessDefinition(
            name=name,
            args=args or [],
            dir=cwd or "",
            env=env or [],
        )

        request = StartProcessRequest(
            definition=definition,
            exit=wait_for_exit,
            regex=wait_for_regex or "",
        )

        try:
            response = self._stub.StartProcess(
                request,
                timeout=timeout
                if (wait_for_exit or wait_for_regex)
                else self._call_timeout,
            )
            if response.success:
                return StartResult(
                    success=True,
                    id=response.id,
                    error=None,
                )
            else:
                status_error = None
                if response.status.HasField("error"):
                    status_error = response.status.error
                return StartResult(
                    success=False,
                    id=None,
                    error=status_error or "Process failed to start",
                )
        except grpc.RpcError as e:
            code = e.code()
            if code == grpc.StatusCode.DEADLINE_EXCEEDED:
                return StartResult(
                    success=False,
                    id=None,
                    error="Timeout waiting for process",
                )
            return StartResult(
                success=False,
                id=None,
                error=str(e),
            )

    def stop_process(self, process_id: str) -> StopResult:
        """Stop a running process.

        Args:
            process_id: ID of the process to stop

        Returns:
            StopResult with success status or error
        """
        self._ensure_connected()

        from .venom_pb2 import StopProcessRequest

        request = StopProcessRequest(id=process_id, wait=False)

        try:
            response = self._stub.StopProcess(request, timeout=self._call_timeout)
            if response.success:
                return StopResult(success=True, error=None)
            else:
                status_error = None
                if response.status.HasField("error"):
                    status_error = response.status.error
                return StopResult(success=False, error=status_error)
        except grpc.RpcError as e:
            return StopResult(success=False, error=str(e))

    def list_processes(self) -> list[ProcessInfo]:
        """List all running processes.

        Returns:
            List of ProcessInfo dictionaries
        """
        self._ensure_connected()

        try:
            response = self._stub.ListProcesses(
                Empty(),
                timeout=self._call_timeout,
            )
            result: list[ProcessInfo] = []
            for p in response.processes:
                exit_code: int | None = None
                error: str | None = None
                if p.status.HasField("exit"):
                    exit_code = p.status.exit
                elif p.status.HasField("error"):
                    error = p.status.error

                result.append(
                    ProcessInfo(
                        id=p.id,
                        name=p.definition.name,
                        args=list(p.definition.args),
                        dir=p.definition.dir or None,
                        env=list(p.definition.env) if p.definition.env else None,
                        complete=p.status.exit is not None
                        or p.status.error is not None,
                        exit_code=exit_code,
                        error=error,
                    )
                )
            return result
        except grpc.RpcError as e:
            logger.error(f"Failed to list processes: {e}")
            return []

    def get_metrics(self) -> Metrics | None:
        """Get system metrics from the daemon.

        Returns:
            Metrics dictionary or None on error
        """
        self._ensure_connected()

        try:
            response = self._stub.GetMetrics(
                Empty(),
                timeout=self._call_timeout,
            )
            return Metrics(
                cpu=response.cpu_percent,
                mem_percent=response.mem_percent,
                mem_total=response.mem_total,
                mem_used=response.mem_used,
            )
        except grpc.RpcError as e:
            logger.error(f"Failed to get metrics: {e}")
            return None

    def subscribe(self, process_id: str) -> Iterator[LogEntry]:
        """Stream log entries for a process.

        Args:
            process_id: ID of the process to subscribe to

        Yields:
            LogEntry dictionaries with timestamp, stream, and line

        Note:
            Blocks until the process ends or the stream is interrupted.
        """
        self._ensure_connected()

        request = StreamLogsRequest(id=process_id, from_start=True)

        try:
            for response in self._stub.StreamLogs(request, timeout=self._call_timeout):
                stream_name = "stdout"
                if response.stream == response.STDERR:
                    stream_name = "stderr"

                yield LogEntry(
                    timestamp=datetime.fromtimestamp(
                        response.ts.seconds + response.ts.nanos / 1e9
                    ),
                    stream=stream_name,
                    line=response.line,
                )
        except grpc.RpcError as e:
            if e.code() != grpc.StatusCode.CANCELLED:
                logger.debug(f"Subscription ended: {e}")
        except Exception as e:
            logger.error(f"Subscription error: {e}")
