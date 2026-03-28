"""
Simple Venom client test to verify protobuf generation works.
"""

from . import venom_pb2, venom_pb2_grpc


def test_protobuf():
    """Test that protobuf classes are available."""
    # Test creating request objects
    req = venom_pb2.StartProcessRequest()
    req.definition.name = "echo"
    req.definition.args.extend(["hello"])

    print(f"Created request: {req}")

    # Test process status
    status = venom_pb2.ProcessStatus()
    status.exit = 0

    print(f"Created status: {status}")

    print("Protobuf classes working correctly!")


if __name__ == "__main__":
    test_protobuf()
