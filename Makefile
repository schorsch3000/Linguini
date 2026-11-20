PREFIX  ?= $(HOME)/.local
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
PLATFORMS := linux/amd64 linux/arm64

export CGO_ENABLED := 0

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o linguini ./cmd/linguini

install: build
	install -Dm755 linguini $(PREFIX)/bin/linguini

test:
	go test ./...

lint:
	@unformatted="$$(gofmt -l .)"; if [ -n "$$unformatted" ]; then \
		echo "gofmt needed:"; echo "$$unformatted"; exit 1; fi
	go vet ./...

# Release archives in dist/: linguini-<version>-<os>-<arch>.tar.gz + checksums.
dist:
	rm -rf dist && mkdir -p dist
	for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; name=linguini-$(VERSION)-$$os-$$arch; \
		mkdir -p dist/$$name && \
		GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags "$(LDFLAGS)" \
			-o dist/$$name/linguini ./cmd/linguini && \
		cp README.md LICENSE config.example.toml dist/$$name/ && \
		tar -C dist -czf dist/$$name.tar.gz $$name && rm -r dist/$$name || exit 1; \
	done
	cd dist && sha256sum *.tar.gz > SHA256SUMS

clean:
	rm -rf linguini dist

.PHONY: build install test lint dist clean
