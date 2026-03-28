#!/usr/bin/env python3
"""
Example usage of VenomClient demonstrating common use cases.
"""

import logging
from venom_client import VenomClient

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)


def example_basic():
    """Basic process start and stop."""
    print("\n=== Basic Example ===")
    with VenomClient("localhost:9988") as client:
        result = client.start_process(name="echo", args=["hello", "world"])
        if result["success"] and result["id"]:
            print(f"Started process: {result['id']}")
            client.stop_process(result["id"])
        else:
            print(f"Failed to start: {result['error']}")


def example_wait_for_exit():
    """Start process and wait for it to exit."""
    print("\n=== Wait for Exit Example ===")
    with VenomClient("localhost:9988") as client:
        result = client.start_process(
            name="echo",
            args=["Process completed successfully!"],
            wait_for_exit=True,
            timeout=5.0,
        )
        if result["success"]:
            print(f"Process exited: {result['id']}")
        else:
            print(f"Failed: {result['error']}")


def example_stream_logs():
    """Subscribe to process logs."""
    print("\n=== Stream Logs Example ===")
    with VenomClient("localhost:9988") as client:
        result = client.start_process(
            name="sh", args=["-c", "echo line1; echo line2; echo line3"]
        )
        if result["success"] and result["id"]:
            print(f"Streaming logs for {result['id']}:")
            for log in client.subscribe(result["id"]):
                print(f"  [{log['stream']}] {log['line']}")


def example_list_processes():
    """List running processes."""
    print("\n=== List Processes Example ===")
    with VenomClient("localhost:9988") as client:
        processes = client.list_processes()
        print(f"Running processes: {len(processes)}")
        for proc in processes:
            print(f"  - {proc['id']}: {proc['name']} {' '.join(proc['args'])}")


def example_metrics():
    """Get system metrics."""
    print("\n=== Metrics Example ===")
    with VenomClient("localhost:9988") as client:
        metrics = client.get_metrics()
        if metrics:
            print(f"CPU: {metrics['cpu']:.1f}%")
            print(
                f"Memory: {metrics['mem_percent']:.1f}% ({metrics['mem_used'] / 1024**3:.1f} GB / {metrics['mem_total'] / 1024**3:.1f} GB)"
            )
        else:
            print("Failed to get metrics")


def main():
    """Run all examples."""
    logger.info("VenomClient Examples")
    logger.info("=====================")

    try:
        example_basic()
        example_wait_for_exit()
        example_stream_logs()
        example_list_processes()
        example_metrics()
    except ConnectionError as e:
        print(f"Connection failed: {e}")
        print("Make sure the Venom daemon is running on localhost:9988")
    except Exception as e:
        print(f"Error: {e}")


if __name__ == "__main__":
    main()
