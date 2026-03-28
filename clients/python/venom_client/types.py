"""Type definitions for Venom client using TypedDict."""

from datetime import datetime
from typing import Iterator, TypedDict


class LogEntry(TypedDict):
    """Log entry from a process stream."""

    timestamp: datetime
    stream: str
    line: str


class ProcessDefinition(TypedDict):
    """Process definition details."""

    name: str
    args: list[str]
    dir: str | None
    env: list[str] | None


class ProcessStatus(TypedDict):
    """Current status of a process."""

    complete: bool
    exit_code: int | None
    error: str | None


class ProcessInfo(TypedDict):
    """Information about a running process."""

    id: str
    name: str
    args: list[str]
    dir: str | None
    env: list[str] | None
    complete: bool
    exit_code: int | None
    error: str | None


class StartResult(TypedDict):
    """Result of starting a process."""

    success: bool
    id: str | None
    error: str | None


class StopResult(TypedDict):
    """Result of stopping a process."""

    success: bool
    error: str | None


class Metrics(TypedDict):
    """System metrics from the daemon."""

    cpu: float
    mem_percent: float
    mem_total: int
    mem_used: int


class StreamLogsRequest(TypedDict):
    """Request to stream logs from a process."""

    process_id: str


class StreamLogsResponse(TypedDict):
    """Response from log streaming."""

    timestamp: datetime
    stream: str
    line: str
