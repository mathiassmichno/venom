# Venom - Process Management Daemon

.PHONY: build clean test proto help install-deps python-client all

# Default target
all: build

# Build all Go binaries
build:
	@echo "Building venomd..."
	go build -o bin/venomd ./cmd/venomd
	@echo "Building venomctl..."
	go build -o bin/venomctl ./clients/go/venomctl
	@echo "Build complete. Binaries available in bin/"

# Generate protobuf code for all languages
proto:
	@echo "Generating Go protobuf code..."
	cd proto && protoc --go_out=generated --go-grpc_out=generated --go_opt=paths=source_relative --go-grpc_opt=paths=source_relative venom.proto
	@echo "Generating Python protobuf code..."
	cd proto && python -m grpc_tools.protoc --python_out=../clients/python/venom_client --grpc_python_out=../clients/python/venom_client --proto_path=. venom.proto
	@echo "Protobuf generation complete"

# Run tests for all languages
test:
	@echo "Running Go tests..."
	go test ./...
	@echo "Running Python tests..."
	cd clients/python && uv run pytest tests/ -v

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -rf bin/
	rm -rf proto/generated/go
	rm -rf proto/generated/python
	rm -f clients/python/dist/
	rm -f clients/python/build/
	rm -rf clients/python/*.egg-info/
	rm -rf clients/python/.venv/
	@echo "Clean complete"

# Install build dependencies
install-deps:
	@echo "Installing Go dependencies..."
	go mod download
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	@echo "Installing Python dependencies..."
	python -m pip install --upgrade pip
	python -m pip install grpcio grpcio-tools build pytest
	@echo "Dependencies installed"

# Build Python client package
python-client:
	@echo "Building Python client package..."
	cd clients/python && uv build
	@echo "Python client built in clients/python/dist/"

# Install binaries locally
install: build
	@echo "Installing binaries..."
	cp bin/venomd /usr/local/bin/
	cp bin/venomctl /usr/local/bin/
	@echo "Installation complete"

# Development setup
dev-setup: install-deps proto
	@echo "Development environment ready"

# Show help
help:
	@echo "Available targets:"
	@echo "  build         - Build all Go binaries (venomd, venomctl)"
	@echo "  proto         - Generate protobuf code for all languages"
	@echo "  test          - Run tests for all languages"
	@echo "  clean         - Clean build artifacts"
	@echo "  install-deps  - Install build dependencies"
	@echo "  python-client - Build Python client package"
	@echo "  install       - Install binaries locally"
	@echo "  dev-setup     - Complete development setup"
	@echo "  all           - Build all (default)"
	@echo "  help          - Show this help message"
