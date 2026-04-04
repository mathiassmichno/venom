"""
Venom Client - Python client library for Venom process management daemon.

Usage:
    from venomctl import VenomClient

    with VenomClient("localhost:9988") as client:
        proc = client.start_process(name="echo", args=["hello"])
        for log in proc.logs():
            print(f"[{log.stream}] {log.line}")
        proc.stop()
"""

from .exceptions import (
    ConnectionError,
    ProcessError,
    ProcessNotFoundError,
    ProcessStartError,
    ProcessStopError,
)
from .process import LogEntry, Process, VenomClient

__all__ = [
    "VenomClient",
    "Process",
    "LogEntry",
    "ProcessError",
    "ProcessStartError",
    "ProcessStopError",
    "ProcessNotFoundError",
    "ConnectionError",
]
