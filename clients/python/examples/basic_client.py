#!/usr/bin/env python3
"""
Example usage of VenomClient demonstrating common use cases.
"""

import logging

from venomctl import ProcessError, VenomClient

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)


def example_basic():
    """Basic process start and stop."""
    print("\n=== Basic Example ===")
    with VenomClient("localhost:9988") as client:
        proc = client.start_process(name="echo", args=["hello", "world"])
        print(f"Started process: {proc.id}")
        proc.stop()


def example_stream_logs():
    """Stream process logs."""
    print("\n=== Stream Logs Example ===")
    with VenomClient("localhost:9988") as client:
        proc = client.start_process(name="sh", args=["-c", "echo line1; echo line2; echo line3"])
        print(f"Streaming logs for {proc.id}:")
        for log in proc.logs():
            print(f"  [{log.stream}] {log.line}")
        proc.stop()


def example_list_processes():
    """List running processes."""
    print("\n=== List Processes Example ===")
    with VenomClient("localhost:9988") as client:
        processes = client.list_processes()
        print(f"Running processes: {len(processes)}")
        for proc in processes:
            print(f"  - {proc.id}: {proc.name} {' '.join(proc.args)}")


def example_metrics():
    """Get system metrics."""
    print("\n=== Metrics Example ===")
    with VenomClient("localhost:9988") as client:
        metrics = client.get_metrics()
        if metrics:
            print(f"CPU: {metrics['cpu']:.1f}%")
            mem_used_gb = metrics["mem_used"] / 1024**3
            mem_total_gb = metrics["mem_total"] / 1024**3
            print(f"Memory: {metrics['mem_percent']:.1f}% ({mem_used_gb:.1f} GB / {mem_total_gb:.1f} GB)")
        else:
            print("Failed to get metrics")


def example_with_stdin():
    """Send input to process stdin."""
    print("\n=== With Stdin Example ===")
    with VenomClient("localhost:9988") as client:
        proc = client.start_process(name="sh", args=["-c", "read line; echo got: $line"], with_stdin=True)
        print(f"Started process: {proc.id}")
        proc.send_input("hello")
        proc.close_stdin()
        for log in proc.logs():
            print(f"  [{log.stream}] {log.line}")
        proc.wait()


def main():
    """Run all examples."""
    logger.info("VenomClient Examples")
    logger.info("=====================")

    try:
        example_basic()
        example_stream_logs()
        example_list_processes()
        example_metrics()
        example_with_stdin()
    except ProcessError as e:
        print(f"Process error: {e}")
        print("Make sure the Venom daemon is running on localhost:9988")
    except Exception as e:
        print(f"Error: {e}")


if __name__ == "__main__":
    main()
