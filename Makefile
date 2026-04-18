.PHONY: build test fmt vet clean

BIN := gosaid
PKG := ./cmd/gosaid

build:
	go build -o $(BIN) $(PKG)

test:
	go test ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

clean:
	rm -f $(BIN)
