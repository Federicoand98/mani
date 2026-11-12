# Makefile for mani project

# Variables
BINARY_NAME=mani
BUILD_DIR=build
MAIN_FILE=cmd/mani/main.go

# Default target
.DEFAULT_GOAL := build

# Build the web console into web/build, where the Go build embeds it
web:
	@echo "Building the web console..."
	cd web && npm ci && npm run build

# Build the project (with the web console)
build: web
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/mani
	@echo "Build completed: $(BUILD_DIR)/$(BINARY_NAME)"

# Run the project
run:
	@echo "Running $(BINARY_NAME)..."
	go run $(MAIN_FILE)

# Run tests
test:
	@echo "Running tests..."
	go test -v ./...

# Test the web console: unit tests, then end-to-end against the built binary
test-web: build
	cd web && npm run check && npm test && npm run test:e2e

# Clean build artifacts
# clean:
# 	@echo "Cleaning build artifacts..."
# 	rm -rf $(BUILD_DIR)
# 	rm -rf ~/.config/gorf/config.json

# Help target
help:
	@echo "Available targets:"
	@echo "  web       - Build the web console (embedded by build)"
	@echo "  build     - Build the project, web console included"
	@echo "  test-web  - Type-check and test the web console"
	@echo "  run       - Run the project"
	@echo "  test      - Run tests"
	# @echo "  clean     - Clean build artifacts"

.PHONY: web build run test test-web deps run-cmd build-all help
