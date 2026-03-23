BINARY     := bin/gluon-server
CMD        := ./cmd
PB_DIR     := ./pb
PROTO_DIR  := .
PROTOS     := $(wildcard $(PROTO_DIR)/*.proto)
PB_SRCS    := $(patsubst $(PROTO_DIR)/%.proto,$(PB_DIR)/%.pb.go,$(PROTOS))

PROTOC_GEN_GO      := $(shell go env GOPATH)/bin/protoc-gen-go
PROTOC_GEN_GO_GRPC := $(shell go env GOPATH)/bin/protoc-gen-go-grpc

.PHONY: all build run proto tidy test clean

all: build

## proto: regenerate pb/*.go from *.proto
proto: $(PROTOC_GEN_GO) $(PROTOC_GEN_GO_GRPC)
	protoc \
		--go_out=$(PB_DIR) --go_opt=paths=source_relative \
		--go-grpc_out=$(PB_DIR) --go-grpc_opt=paths=source_relative \
		--proto_path=$(PROTO_DIR) \
		$(PROTOS)

## build: compile the server binary
build: proto
	go build -o $(BINARY) $(CMD)

## run: build and start the server (override ADDR to change listen address)
ADDR ?= :50051
run: build
	./$(BINARY) -addr $(ADDR)

## tidy: tidy go.mod / go.sum
tidy:
	go mod tidy

## test: run all tests
test:
	go test ./...

## clean: remove build artifacts
clean:
	rm -f $(BINARY)

# Install protoc plugins if missing
$(PROTOC_GEN_GO):
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest

$(PROTOC_GEN_GO_GRPC):
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
