.PHONY: build test clean run lint

# Binary name
BINARY_NAME=zanadir

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOCLEAN=$(GOCMD) clean
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Build the application
build:
	$(GOBUILD) -o $(BINARY_NAME) ./cmd/zanadir

# Run tests
test:
	$(GOTEST) -v ./...

# Clean build files
clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)

# Run the application
run: build
	./$(BINARY_NAME)

# Install dependencies
deps:
	$(GOMOD) download

# Lint the code
lint:
	golangci-lint run

# Install golangci-lint if not present
install-lint:
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin v1.55.2

# Help command
help:
	@echo "Available commands:"
	@echo "  make build        - Build the application"
	@echo "  make test         - Run tests"
	@echo "  make clean        - Clean build files"
	@echo "  make run          - Build and run the application"
	@echo "  make deps         - Download dependencies"
	@echo "  make lint         - Run linter"
	@echo "  make install-lint - Install golangci-lint" 