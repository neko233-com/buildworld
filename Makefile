.PHONY: build build-server build-worker build-cli test clean

build: build-server build-worker build-cli

build-server:
	go build -o bin/buildworld233.exe ./cmd/server

build-worker:
	go build -o bin/buildworld233-worker.exe ./cmd/worker

build-cli:
	go build -o bin/bwctl.exe ./cmd/cli

test:
	go test ./...

clean:
	rm -rf bin/
