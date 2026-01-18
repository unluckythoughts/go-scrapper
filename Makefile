.PHONY: help build test test-verbose test-coverage bench clean fmt lint install run-examples deps

# Default target
help:
	@echo "Available targets:"
	@echo "  make build          - Build the project"
	@echo "  make test           - Run tests"
	@echo "  make test-verbose   - Run tests with verbose output"
	@echo "  make test-coverage  - Run tests with coverage report"
	@echo "  make bench          - Run benchmarks"
	@echo "  make clean          - Remove build artifacts"
	@echo "  make fmt            - Format code"
	@echo "  make lint           - Run linter (requires golangci-lint)"
	@echo "  make install        - Install dependencies"
	@echo "  make run-examples   - Build example executables"
	@echo "  make deps           - Download dependencies"
	@echo "  make all            - Run fmt, test, and build"

# Build the project
build:
	@echo "Building..."
	go build -v ./...

# Run tests
test:
	@echo "Running tests..."
	go test -v -timeout 60s

# Run tests with verbose output
test-verbose:
	@echo "Running tests with verbose output..."
	go test -v -timeout 60s -cover

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	go test -v -timeout 60s -coverprofile=coverage.out -covermode=atomic
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run benchmarks
bench:
	@echo "Running benchmarks..."
	go test -bench=. -benchmem -benchtime=10x

# Clean build artifacts
clean:
	@echo "Cleaning..."
	go clean
	rm -f coverage.out coverage.html
	rm -f examples/*.exe examples/example examples/utils_example

# Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...

# Run linter (requires golangci-lint)
lint:
	@echo "Running linter..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not found. Install it from https://golangci-lint.run/"; \
	fi

# Install dependencies
install:
	@echo "Installing dependencies..."
	go get -v ./...
	go mod tidy

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	go mod download

# Build examples
run-examples:
	@echo "Building examples..."
	go build -o examples/example.exe examples/example.go
	go build -o examples/utils_example.exe examples/utils_example.go
	@echo "Examples built successfully!"
	@echo "  Run: ./examples/example.exe"
	@echo "  Run: ./examples/utils_example.exe"

# Run all checks
all: fmt test build
	@echo "All checks passed!"
