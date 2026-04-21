.PHONY: build test fmt vet clean release-snapshot build-macos-app build-windows-ui

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

# Assemble GoSaid.app for the host architecture.  Requires a Mac and Swift.
build-macos-app:
	mkdir -p out
	go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o out/gosaid $(PKG)
	VERSION=$(VERSION) DAEMON_BIN=out/gosaid OUT_DIR=out scripts/build-macos-app.sh

# Assemble the Windows GoSaid/ folder.  Requires .NET 9 SDK.  When run from
# a non-Windows host, the Go cross-build produces gosaid.exe first.
build-windows-ui:
	mkdir -p out
	GOOS=windows GOARCH=amd64 go build -trimpath \
		-ldflags "-s -w -X main.version=$(VERSION)" \
		-o out/gosaid.exe $(PKG)
	VERSION=$(VERSION) DAEMON_EXE=out/gosaid.exe OUT_DIR=out scripts/build-windows-ui.sh

# Native-arch dry run of the release packaging step (no signing, no upload).
# Useful before pushing a tag to confirm the package step still works.
#
# On darwin hosts this now packages GoSaid.app; on Linux/Windows it
# packages the plain binary as before.
release-snapshot:
	@OS=$$(go env GOOS); ARCH=$$(go env GOARCH); \
	stem="$(BIN)-$(VERSION)-$$OS-$$ARCH"; \
	rm -rf dist/$$stem dist/$$stem.tar.gz dist/$$stem.zip dist/$$stem.tar.gz.sha256 dist/$$stem.zip.sha256; \
	mkdir -p dist/$$stem; \
	if [ "$$OS" = "darwin" ]; then \
		$(MAKE) build-macos-app VERSION=$(VERSION); \
		cp -R out/GoSaid.app dist/$$stem/; \
		cp LICENSE README.md dist/$$stem/ 2>/dev/null || true; \
		tar -C dist -czf dist/$$stem.tar.gz $$stem; \
		rm -rf dist/$$stem; \
		shasum -a 256 dist/$$stem.tar.gz > dist/$$stem.tar.gz.sha256; \
		echo "wrote dist/$$stem.tar.gz"; \
	elif [ "$$OS" = "windows" ]; then \
		$(MAKE) build-windows-ui VERSION=$(VERSION); \
		cp -r out/GoSaid dist/$$stem/; \
		cp LICENSE README.md dist/$$stem/ 2>/dev/null || true; \
		( cd dist && zip -r -q $$stem.zip $$stem ); \
		rm -rf dist/$$stem; \
		shasum -a 256 dist/$$stem.zip > dist/$$stem.zip.sha256; \
		echo "wrote dist/$$stem.zip"; \
	else \
		go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o dist/$$stem/$(BIN) $(PKG); \
		cp LICENSE README.md dist/$$stem/ 2>/dev/null || true; \
		cp -r examples dist/$$stem/ 2>/dev/null || true; \
		tar -C dist -czf dist/$$stem.tar.gz $$stem; \
		rm -rf dist/$$stem; \
		shasum -a 256 dist/$$stem.tar.gz > dist/$$stem.tar.gz.sha256; \
		echo "wrote dist/$$stem.tar.gz"; \
	fi
