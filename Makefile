.PHONY: build build-loadtest test bench lint pprof clean

BINARY     := bin/gochat
LOADTEST   := bin/loadtest
GO         := go
GOFLAGS    := -trimpath
LDFLAGS    := -s -w

build:
	$(GO) build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BINARY) ./cmd/gochat

build-loadtest:
	$(GO) build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(LOADTEST) ./cmd/loadtest

test:
	$(GO) test -race -count=1 ./...

bench:
	$(GO) test -bench=. -benchmem -count=3 ./internal/engine/... ./pkg/...

lint:
	golangci-lint run ./...

pprof-heap:
	$(GO) tool pprof http://localhost:6060/debug/pprof/heap

pprof-cpu:
	$(GO) tool pprof http://localhost:6060/debug/pprof/profile?seconds=30

clean:
	rm -rf bin/
