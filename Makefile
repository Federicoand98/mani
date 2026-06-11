# Makefile for mani project

# Variables
BINARY_NAME=mani
BUILD_DIR=build
MAIN_FILE=cmd/mani/main.go

# Default target
.DEFAULT_GOAL := build

# Build the project
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_FILE)
	@echo "Build completed: $(BUILD_DIR)/$(BINARY_NAME)"

# Run the project
run:
	@echo "Running $(BINARY_NAME)..."
	go run $(MAIN_FILE)

# Run tests
test:
	@echo "Running tests..."
	go test -v ./...

# Clean build artifacts
# clean:
# 	@echo "Cleaning build artifacts..."
# 	rm -rf $(BUILD_DIR)
# 	rm -rf ~/.config/gorf/config.json

# Help target
help:
	@echo "Available targets:"
	@echo "  build     - Build the project"
	@echo "  run       - Run the project"
	@echo "  test      - Run tests"
	# @echo "  clean     - Clean build artifacts"

.PHONY: build run test deps run-cmd build-all help
