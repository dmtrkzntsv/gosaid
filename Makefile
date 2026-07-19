.PHONY: build test fmt vet clean release-snapshot vendor vendor-ggml vendor-whisper vendor-llama

BIN := gosaid
PKG := ./cmd/gosaid
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

build:
	go build -ldflags "-s -w -X main.version=$(VERSION)" -o $(BIN) $(PKG)

test:
	go test ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

clean:
	rm -f $(BIN)
	rm -rf dist out

# Re-vendor the C libraries at the tags pinned in scripts/vendor-versions.env.
# ggml is shared: whisper.cpp and llama.cpp must agree on its version, so
# change tags together and re-run `make vendor`.
vendor: vendor-ggml vendor-whisper vendor-llama

vendor-ggml:
	scripts/vendor-ggml.sh $(LLAMA_TAG)

vendor-whisper:
	scripts/vendor-whisper.sh $(WHISPER_TAG)

vendor-llama:
	scripts/vendor-llama.sh $(LLAMA_TAG)

# Native-arch dry run of the release packaging step (no signing, no upload).
# Useful before pushing a tag to confirm the package step still works.
release-snapshot:
	@OS=$$(go env GOOS); ARCH=$$(go env GOARCH); \
	stem="$(BIN)-$(VERSION)-$$OS-$$ARCH"; \
	rm -rf dist/$$stem dist/$$stem.tar.gz dist/$$stem.tar.gz.sha256; \
	mkdir -p dist/$$stem; \
	go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o dist/$$stem/$(BIN) $(PKG); \
	cp LICENSE README.md dist/$$stem/ 2>/dev/null || true; \
	cp -r examples dist/$$stem/ 2>/dev/null || true; \
	tar -C dist -czf dist/$$stem.tar.gz $$stem; \
	rm -rf dist/$$stem; \
	shasum -a 256 dist/$$stem.tar.gz > dist/$$stem.tar.gz.sha256; \
	echo "wrote dist/$$stem.tar.gz"
