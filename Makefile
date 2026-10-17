.PHONY: build test clean run lint corpus corpus-update

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

# Scan the real repositories in handler/testdata/corpus and compare with the
# snapshots; corpus-update rewrites them after an intended change.
CORPUS_DIR ?= $(CURDIR)/.corpus

corpus:
	scripts/fetch-corpus.sh handler/testdata/corpus/repos.txt $(CORPUS_DIR)
	CORPUS_DIR=$(CORPUS_DIR) $(GOTEST) ./handler -run TestCorpus -count=1

corpus-update:
	scripts/fetch-corpus.sh handler/testdata/corpus/repos.txt $(CORPUS_DIR)
	CORPUS_DIR=$(CORPUS_DIR) $(GOTEST) ./handler -run TestCorpus -count=1 -update-corpus

# Clean build files
clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)

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
