.PHONY: build build-server build-worker build-cli proto test clean

build: build-server build-worker build-cli

build-server:
	go build -o bin/buildworld233.exe ./cmd/server

build-worker:
	go build -o bin/buildworld233-worker.exe ./cmd/worker

build-cli:
	go build -o bin/bwctl.exe ./cmd/cli

proto:
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		proto/worker.proto
	mv proto/worker.pb.go internal/rpc/generated/worker.pb.go
	mv proto/worker_grpc.pb.go internal/rpc/generated/worker_grpc.pb.go

test:
	go test ./...

clean:
	rm -rf bin/
