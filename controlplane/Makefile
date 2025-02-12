.PHONY: all build test clean vet fmt

# Default target
all: clean vet fmt test build

# Build the controlplane binary
build:
	@echo "Building controlplane..."
	@go build -v -o bin/orra-cp

# Run tests with coverage
test:
	@echo "Running tests..."
	@go test -v -race -cover ./...

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
