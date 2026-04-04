"""Custom exceptions for Venom client."""


class ProcessError(Exception):
    """Base exception for process-related errors."""

    pass


class ProcessNotFoundError(ProcessError):
    """Raised when process doesn't exist."""

    pass


class ProcessStartError(ProcessError):
    """Raised when process fails to start."""

    pass


class ProcessStopError(ProcessError):
    """Raised when process fails to stop."""

    pass


class ConnectionError(ProcessError):
    """Raised when connection to daemon fails."""

    pass
