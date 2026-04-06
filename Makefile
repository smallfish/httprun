BIN_DIR := bin
BIN := $(BIN_DIR)/httprun

.PHONY: all build test fmt run-help clean

all: build

build:
	mkdir -p $(BIN_DIR)
	go build -o $(BIN) ./cmd/httprun

test:
	go test ./...

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './bin/*')

run-help: build
	./$(BIN) --help

clean:
	rm -rf $(BIN_DIR)
