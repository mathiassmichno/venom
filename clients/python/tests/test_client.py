"""Tests for VenomClient using mocks."""

from unittest.mock import MagicMock, patch

import pytest

from venomctl import Process, ProcessStartError, VenomClient
from venomctl.exceptions import ConnectionError


class TestVenomClient:
    """Test VenomClient class."""

    def test_client_initialization(self):
        """Test client initialization."""
        client = VenomClient("localhost:9988")
        assert client._address == "localhost:9988"
        assert client._connect_timeout == 10.0
        assert client._call_timeout == 30.0
        assert client._connected is False
        assert client._channel is None
        assert client._stub is None

    def test_client_initialization_custom_timeouts(self):
        """Test client with custom timeouts."""
        client = VenomClient(
            "localhost:9988",
            connect_timeout=5.0,
            call_timeout=60.0,
        )
        assert client._connect_timeout == 5.0
        assert client._call_timeout == 60.0

    def test_context_manager(self):
        """Test context manager protocol."""
        client = VenomClient("localhost:9988")
        with client as c:
            assert c is client

    def test_close_when_not_connected(self):
        """Test close when not connected."""
        client = VenomClient("localhost:9988")
        client.close()
        assert client._connected is False
        assert client._channel is None

    def test_ensure_connected(self):
        """Test lazy connection establishment."""
        import grpc

        mock_channel_instance = MagicMock()

        with patch.object(grpc, "insecure_channel", return_value=mock_channel_instance):
            with patch.object(grpc, "channel_ready_future") as mock_future:
                mock_future.return_value.result.return_value = None

                client = VenomClient("localhost:9988")
                client._ensure_connected()

                assert client._connected is True
                assert client._stub is not None

    def test_ensure_connected_already_connected(self):
        """Test no reconnection when already connected."""
        import grpc

        mock_channel_instance = MagicMock()

        with patch.object(grpc, "insecure_channel", return_value=mock_channel_instance):
            with patch.object(grpc, "channel_ready_future") as mock_future:
                mock_future.return_value.result.return_value = None

                client = VenomClient("localhost:9988")
                client._ensure_connected()
                client._ensure_connected()

    def test_ensure_connected_timeout(self):
        """Test connection timeout."""
        import grpc

        mock_channel_instance = MagicMock()

        with patch.object(grpc, "insecure_channel", return_value=mock_channel_instance):
            with patch.object(grpc, "channel_ready_future") as mock_future:
                mock_future.return_value.result.side_effect = grpc.FutureTimeoutError()

                client = VenomClient("localhost:9988", connect_timeout=1.0)

                with pytest.raises(ConnectionError, match="Connection timeout"):
                    client._ensure_connected()


class TestVenomClientIntegration:
    """Integration-style tests with mocked gRPC stubs."""

    @patch("venomctl.process.VenomClient._ensure_connected")
    def test_start_process_success(self, mock_connect):
        """Test successful process start."""
        mock_response = MagicMock()
        mock_response.success = True
        mock_response.id = "test-proc-123"

        mock_stub = MagicMock()
        mock_stub.StartProcess.return_value = mock_response

        client = VenomClient("localhost:9988")
        client._stub = mock_stub
        client._connected = True

        result = client.start_process(name="echo", args=["hello"])

        assert isinstance(result, Process)
        assert result.id == "test-proc-123"
        assert result.name == "echo"
        assert result.args == ["hello"]

    @patch("venomctl.process.VenomClient._ensure_connected")
    def test_start_process_failure(self, mock_connect):
        """Test failed process start."""
        mock_response = MagicMock()
        mock_response.success = False
        mock_response.status.HasField.return_value = False

        mock_stub = MagicMock()
        mock_stub.StartProcess.return_value = mock_response

        client = VenomClient("localhost:9988")
        client._stub = mock_stub
        client._connected = True

        with pytest.raises(ProcessStartError):
            client.start_process(name="nonexistent")

    @patch("venomctl.process.VenomClient._ensure_connected")
    def test_stop_process(self, mock_connect):
        """Test stopping a process via Process object."""
        mock_response = MagicMock()
        mock_response.success = True

        mock_stub = MagicMock()
        mock_stub.StopProcess.return_value = mock_response

        client = VenomClient("localhost:9988")
        client._stub = mock_stub
        client._connected = True

        process = Process(id="test-proc-123", name="echo", args=[], _client=client)

        result = process.stop()

        assert result is True
        mock_stub.StopProcess.assert_called_once()

    @patch("venomctl.process.VenomClient._ensure_connected")
    def test_list_processes(self, mock_connect):
        """Test listing processes."""
        mock_process = MagicMock()
        mock_process.id = "proc-1"
        mock_process.definition.name = "echo"
        mock_process.definition.args = ["hello"]
        mock_process.definition.dir = "/home"
        mock_process.definition.env = []
        mock_process.status.HasField.side_effect = lambda x: False
        mock_process.status.exit = None
        mock_process.status.error = None

        mock_response = MagicMock()
        mock_response.processes = [mock_process]

        mock_stub = MagicMock()
        mock_stub.ListProcesses.return_value = mock_response

        client = VenomClient("localhost:9988")
        client._stub = mock_stub
        client._connected = True

        processes = client.list_processes()

        assert len(processes) == 1
        assert processes[0].id == "proc-1"
        assert processes[0].name == "echo"
        assert processes[0].complete is False

    @patch("venomctl.process.VenomClient._ensure_connected")
    def test_get_metrics(self, mock_connect):
        """Test getting metrics."""
        mock_response = MagicMock()
        mock_response.cpu_percent = 45.5
        mock_response.mem_percent = 72.3
        mock_response.mem_total = 16000000000
        mock_response.mem_used = 11568000000

        mock_stub = MagicMock()
        mock_stub.GetMetrics.return_value = mock_response

        client = VenomClient("localhost:9988")
        client._stub = mock_stub
        client._connected = True

        metrics = client.get_metrics()

        assert metrics is not None
        assert metrics["cpu"] == 45.5
        assert metrics["mem_percent"] == 72.3


class TestProcess:
    """Test Process class."""

    @patch("venomctl.process.VenomClient._ensure_connected")
    def test_process_logs(self, mock_connect):
        """Test process logs streaming."""
        mock_response = MagicMock()
        mock_response.ts.seconds = 1000
        mock_response.ts.nanos = 0
        mock_response.stream = 1  # STDOUT
        mock_response.line = "hello"

        mock_stub = MagicMock()
        mock_stub.StreamLogs.return_value = iter([mock_response])

        client = VenomClient("localhost:9988")
        client._stub = mock_stub
        client._connected = True

        process = Process(id="test-1", name="echo", args=[], _client=client)

        logs = list(process.logs())

        assert len(logs) == 1
        assert logs[0].stream == "stdout"
        assert logs[0].line == "hello"

    def test_process_repr(self):
        """Test process repr."""
        process = Process(id="test-1", name="echo", args=["hello"])
        assert repr(process) == "Process(id='test-1', name='echo', complete=False)"

    def test_process_complete_repr(self):
        """Test process repr when complete."""
        process = Process(id="test-1", name="echo", args=[], complete=True, exit_code=0)
        assert "complete=True" in repr(process)
