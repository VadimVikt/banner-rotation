BIN := "./bin/server"

.PHONY: build run test clean

build:
	go build -o $(BIN) ./cmd/server

run: build
	$(BIN)

test:
	go test -race -count 100 ./...

clean:
	rm -rf bin/