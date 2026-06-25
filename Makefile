# Variables
BINARY_NAME=ak-scout
BUILD_DIR=bin
GO ?= go

GO_CACHE_DIR=$(CURDIR)/.cache/go-build
GO_MOD_CACHE_DIR=$(CURDIR)/.cache/go-mod
GO_ENV=GOCACHE=$(GO_CACHE_DIR) GOMODCACHE=$(GO_MOD_CACHE_DIR)

.PHONY: all build clean test run check-outcomes report fmt vet help snapshot

all: build

# Build binary
build:
	@echo "🔨 Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR) $(GO_CACHE_DIR) $(GO_MOD_CACHE_DIR)
	$(GO_ENV) $(GO) build -buildvcs=false -o $(BUILD_DIR)/$(BINARY_NAME) .

# Clean build artifacts
clean:
	@echo "🧹 Cleaning..."
	rm -rf $(BUILD_DIR) $(GO_CACHE_DIR)

# Run tests
test:
	@echo "🧪 Running tests..."
	$(GO_ENV) $(GO) test -v ./...

# Format code
fmt:
	@echo "✨ Formatting code..."
	$(GO) fmt ./...

# Vet code
vet:
	@echo "🔍 Vetting code..."
	$(GO_ENV) $(GO) vet ./...

# Run snapshot (default BNBUSD_PERP)
snapshot: build
	@echo "📸 Running snapshot..."
	./$(BUILD_DIR)/$(BINARY_NAME) snapshot --symbol BNBUSD_PERP --context BTCUSD_PERP

# Run outcome check
check-outcomes: build
	@echo "✅ Checking outcomes..."
	./$(BUILD_DIR)/$(BINARY_NAME) check-outcomes

# Generate report
report: build
	@echo "📊 Generating report..."
	./$(BUILD_DIR)/$(BINARY_NAME) report

# Help command
help:
	@echo "Available targets:"
	@echo "  build           - Build the ak-scout binary"
	@echo "  clean           - Remove build artifacts and local cache"
	@echo "  test            - Run Go tests"
	@echo "  fmt             - Run go fmt"
	@echo "  vet             - Run go vet"
	@echo "  snapshot        - Take a market snapshot"
	@echo "  check-outcomes  - Check outcome status for recorded snapshots"
	@echo "  report          - Show performance report"
