BIN  := led-panel
PORT ?= 8765

.PHONY: run build test cover vet lint fmt check clean

run:
	go run ./cmd/server $(PORT)

build:
	go build -o $(BIN) ./cmd/server

test:
	go test -race ./...

cover:
	go test -race -covermode=atomic -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1

vet:
	go vet ./...

lint:
	golangci-lint run ./...

fmt:
	golangci-lint fmt ./...

check: vet lint test

clean:
	rm -f $(BIN) coverage.out
