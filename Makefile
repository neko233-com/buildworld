.PHONY: build build-server build-worker build-cli proto test clean

# Keep local builds predictable on shared development machines. Disabling VCS
# stamping avoids a full `git status` scan of large/dirty worktrees; -p limits
# simultaneous compiler processes without changing produced application code.
GO_BUILD_FLAGS ?= -buildvcs=false -p 2
GO_TEST_FLAGS ?= -p 2

build: build-server build-worker build-cli

build-server:
	go build $(GO_BUILD_FLAGS) -o bin/buildworld-server.exe ./cmd/server

build-worker:
	go build $(GO_BUILD_FLAGS) -o bin/buildworld-worker.exe ./cmd/worker

build-cli:
	go build $(GO_BUILD_FLAGS) -o bin/buildworld-cli.exe ./cmd/cli

proto:
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		proto/worker.proto
	mv proto/worker.pb.go internal/rpc/generated/worker.pb.go
	mv proto/worker_grpc.pb.go internal/rpc/generated/worker_grpc.pb.go

test:
	go test $(GO_TEST_FLAGS) ./...

clean:
	rm -rf bin/
