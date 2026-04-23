VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

.PHONY: build test test-contract clean

build:
	go build $(LDFLAGS) -o bin/xcbackup ./cmd/xcbackup

test:
	go test -v -race ./...

test-contract:
	go test -v -race -count=1 -tags=contract ./...

clean:
	rm -rf bin/
