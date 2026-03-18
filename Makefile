BINARY=bin/archway
ENGINE_SRC=engine/target/release/archway-engine
ENGINE_EMBED_DIR=internal/engineclient/bin
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

.PHONY: build engine test lint vet clean release snapshot install-local help

engine:
	cd engine && cargo build --release
	mkdir -p $(ENGINE_EMBED_DIR)
	cp $(ENGINE_SRC) $(ENGINE_EMBED_DIR)/archway-engine-$(GOOS)-$(GOARCH)

build: engine
	go build -o $(BINARY) ./cmd/archway

test:
	go test ./...

lint:
	golangci-lint run

clean:
	rm -rf bin/ internal/engineclient/bin/

release:
	goreleaser release

snapshot:
	goreleaser release --snapshot --clean

vet:
	go vet ./...

install-local:
	go install ./cmd/archway

help:
	@echo "engine         Build Rust engine for current platform ($(GOOS)/$(GOARCH))"
	@echo "build          Build binary to bin/archway (runs engine first)"
	@echo "test           Run all tests"
	@echo "lint           Run golangci-lint"
	@echo "vet            Run go vet"
	@echo "clean          Remove build artifacts"
	@echo "release        Create a release with GoReleaser"
	@echo "snapshot       Create a snapshot release (no publish)"
	@echo "install-local  Install to GOPATH/bin"
