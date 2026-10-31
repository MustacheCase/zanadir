.PHONY: build test clean run lint web web-test

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
	$(GOBUILD) -o $(BINARY_NAME) -v .

# Run tests
test:
	$(GOTEST) -v ./...

# The browser scan page, in dist/scan: the page, the in-memory file system,
# Go's wasm_exec.js (it must match the Go that built the module) and the
# module, gzipped.
WEB_VERSION ?= dev
web:
	rm -rf dist/scan && mkdir -p dist/scan
	cp web/scan/index.html web/scan/memfs.js dist/scan/
	cp "$$($(GOCMD) env GOROOT)/lib/wasm/wasm_exec.js" dist/scan/
	GOOS=js GOARCH=wasm $(GOBUILD) -trimpath -ldflags "-s -w -X github.com/MustacheCase/zanadir/app.version=$(WEB_VERSION)" -o dist/zanadir.wasm ./wasm
	gzip -9 -n -c dist/zanadir.wasm > dist/scan/zanadir.wasm.gz

# The browser build's tests, on the page's in-memory file system. Needs node.
web-test:
	mkdir -p dist
	cp "$$($(GOCMD) env GOROOT)/lib/wasm/wasm_exec.js" dist/
	GOOS=js GOARCH=wasm $(GOTEST) -c -o dist/wasm.test ./wasm
	node wasm/run.js dist/wasm.test -test.v

# Clean build files
clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -rf dist

# Run the application
run:
	$(GOBUILD) -o $(BINARY_NAME) -v .
	./$(BINARY_NAME)

# Install dependencies
deps:
	$(GOMOD) download

# Tidy dependencies
tidy:
	$(GOMOD) tidy

# Lint the code
lint:
	golangci-lint run

# Install golangci-lint if not present
install-lint:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2
