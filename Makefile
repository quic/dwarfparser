DEBUG ?= false
GOFLAGS := -mod=mod
ifeq ($(DEBUG), true)
GOFLAGS += -gcflags="all=-N -l"
endif

.PHONY:	all
all: prep cmd lint

prep:
	mkdir -p bin

lint: prep
	[ -f bin/golangci-lint ] || curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b bin v1.55.1
	bin/golangci-lint run ./...

cmd: addr2line callgraph

addr2line: prep
	go build $(GOFLAGS) -o bin/addr2line ./cmd/addr2line

callgraph: prep
	go build $(GOFLAGS) -o bin/callgraph ./cmd/callgraph

bench: build
	bash bench.sh
