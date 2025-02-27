.PHONY: all build test clean vet fmt lint cover

# Default target
all: clean vet fmt lint test build

# Build the plan engine binary
build:
	@echo "Building planengine..."
	@go build -v -o bin/orra-peng

# Run tests with coverage
test:
	@echo "Running tests..."
	@go test -race -cover ./...

# Run the latest build
run:
	@./bin/orra-peng -log-level trace

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@go clean
	@rm -f coverage.out

# Run go vet
vet:
	@echo "Running go vet..."
	@go vet ./...

# Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...

# Run tests with coverage and output to a file
cover:
	@echo "Running tests with coverage..."
	@go test -v -race -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out

# Lint target: Check if golangci-lint exists; if not, download it then run linting
lint:
	@echo "Checking for golangci-lint..."
	@if [ ! -f "$(shell go env GOPATH)/bin/golangci-lint" ]; then \
		echo "golangci-lint not found. Installing..."; \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b $(shell go env GOPATH)/bin v1.63.4; \
	else \
		echo "golangci-lint found."; \
	fi
	@echo "Running golangci-lint..."
	@$(shell go env GOPATH)/bin/golangci-lint run
