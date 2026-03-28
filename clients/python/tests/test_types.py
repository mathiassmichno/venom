"""Tests for VenomClient types."""

from datetime import datetime

from venom_client.types import (
    LogEntry,
    Metrics,
    ProcessInfo,
    StartResult,
    StopResult,
)


class TestTypes:
    """Test TypedDict type definitions."""

    def test_log_entry(self):
        """Test LogEntry structure."""
        entry: LogEntry = {
            "timestamp": datetime.now(),
            "stream": "stdout",
            "line": "hello world",
        }
        assert entry["stream"] == "stdout"
        assert entry["line"] == "hello world"

    def test_start_result_success(self):
        """Test successful StartResult."""
        result: StartResult = {
            "success": True,
            "id": "test-id-123",
            "error": None,
        }
        assert result["success"] is True
        assert result["id"] == "test-id-123"
        assert result["error"] is None

    def test_start_result_failure(self):
        """Test failed StartResult."""
        result: StartResult = {
            "success": False,
            "id": None,
            "error": "Process not found",
        }
        assert result["success"] is False
        assert result["id"] is None
        assert result["error"] == "Process not found"

    def test_stop_result_success(self):
        """Test successful StopResult."""
        result: StopResult = {
            "success": True,
            "error": None,
        }
        assert result["success"] is True
        assert result["error"] is None

    def test_stop_result_failure(self):
        """Test failed StopResult."""
        result: StopResult = {
            "success": False,
            "error": "Process already stopped",
        }
        assert result["success"] is False
        assert result["error"] == "Process already stopped"

    def test_metrics(self):
        """Test Metrics structure."""
        metrics: Metrics = {
            "cpu": 45.5,
            "mem_percent": 72.3,
            "mem_total": 16000000000,
            "mem_used": 11568000000,
        }
        assert metrics["cpu"] == 45.5
        assert metrics["mem_percent"] == 72.3
        assert metrics["mem_total"] == 16_000_000_000

    def test_process_info(self):
        """Test ProcessInfo structure."""
        info: ProcessInfo = {
            "id": "proc-123",
            "name": "echo",
            "args": ["hello", "world"],
            "dir": "/home/user",
            "env": ["PATH=/usr/bin"],
            "complete": False,
            "exit_code": None,
            "error": None,
        }
        assert info["name"] == "echo"
        assert info["args"] == ["hello", "world"]
        assert info["complete"] is False
