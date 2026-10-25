BIN  := led-panel
PORT ?= 8765

.PHONY: run build test vet clean

run:
	go run ./cmd/server $(PORT)

build:
	go build -o $(BIN) ./cmd/server

test:
	go test ./...

vet:
	go vet ./...

clean:
	rm -f $(BIN)
