BINARY  := tea-issue-sync
PKG     := ./cmd/tea-issue-sync
MODULE  := github.com/theultimatelazydev/tea-issue-sync
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X $(MODULE)/internal/teasync.Version=$(VERSION)
PLATFORMS := darwin/arm64 darwin/amd64 linux/amd64 linux/arm64

.PHONY: build test vet install dist clean

build:
	go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY) $(PKG)

test:
	go test ./...

vet:
	go vet ./...

install:
	go install -ldflags "$(LDFLAGS)" $(PKG)

# Cross-compile a binary per platform into dist/ (named like the release assets).
dist:
	@mkdir -p dist
	@for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; \
		out=dist/$(BINARY)-$$os-$$arch; \
		echo "building $$out"; \
		GOOS=$$os GOARCH=$$arch go build -ldflags "$(LDFLAGS)" -o $$out $(PKG) || exit 1; \
	done

clean:
	rm -rf dist
