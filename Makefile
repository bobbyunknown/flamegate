NAME       := flamegate
BINDIR     := bin
ENTRYPOINT := ./cmd/flamegate
STATIC_DIR := internal/infrastructure/http/static/dist

# Dynamic Metadata & Stamping
VERSION   ?= $(shell git describe --tags --exact-match 2>/dev/null || cat VERSION 2>/dev/null || echo "0.0.1")
COMMIT    ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILDTIME ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')

LDFLAGS   := -X "main.Version=$(VERSION)" \
             -X "main.Commit=$(COMMIT)" \
             -X "main.BuildTime=$(BUILDTIME)" \
             -w -s -buildid=

GOBUILD   := CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)'

# Standard Architectures
PLATFORMS := \
	darwin-arm64 \
	darwin-amd64 \
	linux-amd64 \
	linux-arm64 \
	windows-amd64 \
	windows-arm64

.PHONY: help build build-ui all all-ui dev ui-dev ui-build ui-sync \
        test test-race vet lint clean releases releases-ui ext-build \
        $(PLATFORMS)

# Default target
all: build

# ------------------------------------------------------------------------------
# Help / Usage
# ------------------------------------------------------------------------------
help:
	@echo "FlameGate Build System ($(VERSION))"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Build Commands:"
	@echo "  build         Compile native host binary (bin/$(NAME))"
	@echo "  build-ui      Compile frontend with Bun + build native binary with UI"
	@echo "  all           Compile all 6 cross-platform binaries"
	@echo "  all-ui        Compile frontend with Bun + all 6 cross-platform binaries"
	@echo "  releases      Package all 6 binaries into .gz / .zip archives"
	@echo "  releases-ui   Compile frontend + package all 6 binaries into archives"
	@echo ""
	@echo "Single Platform Targets:"
	@echo "  darwin-arm64  Build for macOS Apple Silicon (M1/M2/M3/M4)"
	@echo "  darwin-amd64  Build for macOS Intel"
	@echo "  linux-amd64   Build for Linux x86_64"
	@echo "  linux-arm64   Build for Linux ARM64"
	@echo "  windows-amd64 Build for Windows x86_64"
	@echo "  windows-arm64 Build for Windows ARM64"
	@echo ""
	@echo "Development & Frontend (Bun):"
	@echo "  dev           Run Go backend (air hot-reload or go run)"
	@echo "  ui-dev        Run React frontend Vite dev server (bun run dev)"
	@echo "  ui-build      Build React frontend assets (bun run build)"
	@echo "  ui-sync       Build React frontend + sync to Go embed directory"
	@echo ""
	@echo "Quality & Testing:"
	@echo "  test          Run all Go unit tests"
	@echo "  test-race     Run Go unit tests with race detector"
	@echo "  vet           Run go vet static analyzer"
	@echo "  lint          Run golangci-lint"
	@echo ""
	@echo "Maintenance:"
	@echo "  clean         Remove compiled binaries and release archives"
	@echo "  ext-build     Compile all WASM extensions in flamegate-ext/"

# ------------------------------------------------------------------------------
# Build Targets
# ------------------------------------------------------------------------------

build:
	@mkdir -p $(BINDIR)
	@echo "--> Building $(NAME) (native)..."
	@$(GOBUILD) -o $(BINDIR)/$(NAME) $(ENTRYPOINT)

build-ui: ui-sync build

all-ui: ui-sync $(addprefix build-, $(PLATFORMS))

# Pattern rule for building individual OS-ARCH targets
build-%:
	@mkdir -p $(BINDIR)
	@os=$$(echo $* | cut -d- -f1); \
	arch=$$(echo $* | cut -d- -f2); \
	ext=""; [ "$$os" = "windows" ] && ext=".exe"; \
	echo "--> Building $(NAME)-$$os-$$arch$$ext..."; \
	GOOS=$$os GOARCH=$$arch $(GOBUILD) -o $(BINDIR)/$(NAME)-$$os-$$arch$$ext $(ENTRYPOINT)

darwin-arm64: build-darwin-arm64
darwin-amd64: build-darwin-amd64
linux-amd64: build-linux-amd64
linux-arm64: build-linux-arm64
windows-amd64: build-windows-amd64
windows-arm64: build-windows-arm64

# ------------------------------------------------------------------------------
# Packaging & Releases
# ------------------------------------------------------------------------------

releases: $(addprefix build-, $(PLATFORMS))
	@echo "--> Packaging release archives..."
	@cd $(BINDIR) && for file in $(NAME)-*; do \
		case "$$file" in \
			*.exe) \
				which zip > /dev/null 2>&1 && zip -q "$${file%.exe}-$(VERSION).zip" "$$file" && rm "$$file" || true ;; \
			*.zip|*.gz) ;; \
			*) \
				gzip -f -c "$$file" > "$$file-$(VERSION).gz" && rm "$$file" ;; \
		esac; \
	done
	@echo "--> Release archives ready in $(BINDIR)/"

releases-ui: ui-sync releases

# ------------------------------------------------------------------------------
# Frontend (Bun) & Embed Sync
# ------------------------------------------------------------------------------

ui-dev:
	cd frontend && bun run dev

ui-build:
	cd frontend && bun install && bun run build

ui-sync: ui-build
	@echo "--> Syncing frontend assets to $(STATIC_DIR)..."
	@rm -rf $(STATIC_DIR)
	@mkdir -p $(STATIC_DIR)
	@cp -r frontend/dist/* $(STATIC_DIR)/
	@touch $(STATIC_DIR)/.gitkeep

# ------------------------------------------------------------------------------
# Testing, Quality & Maintenance
# ------------------------------------------------------------------------------

dev:
	@which air > /dev/null 2>&1 && air || go run $(ENTRYPOINT)

test:
	go test ./...

test-race:
	go test -race ./...

vet:
	go vet ./... ./cmd/...

lint:
	golangci-lint run ./...

clean:
	@echo "--> Cleaning $(BINDIR)..."
	@rm -rf $(BINDIR)/*

ext-build:
	@for dir in flamegate-ext/*/; do \
		if [ -f "$$dir/Makefile" ]; then \
			echo "Building extension in $$dir..."; \
			$(MAKE) -C "$$dir" build || exit 1; \
		fi \
	done
