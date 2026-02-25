VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

.PHONY: build test clean

build:
	go build $(LDFLAGS) -o bin/xcbackup ./cmd/xcbackup

test:
	go test -v -race ./...

clean:
	rm -rf bin/
