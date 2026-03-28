"""
Venom Client - Python client library for Venom process management daemon.

Usage:
    from venom_client import VenomClient

    with VenomClient("localhost:9988") as client:
        result = client.start_process(name="echo", args=["hello", "world"])
        if result["success"]:
            for log in client.subscribe(result["id"]):
                print(f"{log['stream']}: {log['line']}")
"""

from .client import VenomClient
from .types import (
    LogEntry,
    Metrics,
    ProcessInfo,
    StartResult,
    StopResult,
)

__all__ = [
    "VenomClient",
    "LogEntry",
    "Metrics",
    "ProcessInfo",
    "StartResult",
    "StopResult",
]
