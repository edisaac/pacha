# Timetable API Makefile

.PHONY: build run run-local test clean deps docker-build lambda-build

# Build the API binary
build:
	@echo "Building Timetable API..."
	@go build -o bin/api cmd/api/main.go
	@echo "Build complete: bin/api"

# Build for AWS Lambda
lambda-build:
	@echo "Building for AWS Lambda..."
	@GOOS=linux GOARCH=amd64 go build -tags lambda.norpc -o bootstrap cmd/lambda/main.go
	@zip lambda-deployment.zip bootstrap
	@echo "Lambda build complete: lambda-deployment.zip"

# Run the API server (production mode)
run:
	@echo "Starting Timetable API server..."
	@go run cmd/api/main.go

# Run the API server (local development mode)
run-local:
	@echo "Starting Timetable API server (local mode)..."
	@if [ -f .env.local ]; then \
		export $$(cat .env.local | xargs) && go run cmd/api/main.go; \
	else \
		echo "❌ .env.local not found. Run ../start-local.sh first"; \
	fi

# Run with hot reload (requires air: go install github.com/cosmtrek/air@latest)
dev:
	@echo "Starting development server with hot reload..."
	@if command -v air > /dev/null; then \
		air; \
	else \
		echo "Installing air for hot reload..."; \
		go install github.com/cosmtrek/air@latest; \
		air; \
	fi

# Run tests
test:
	@echo "Running tests..."
	@go test ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	@go test -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf bin/ bootstrap lambda-deployment.zip coverage.out coverage.html
	@echo "Clean complete"

# Install dependencies
deps:
	@echo "Installing dependencies..."
	@go mod tidy
	@go mod download
	@echo "Dependencies installed"

# Docker build
docker-build:
	@echo "Building Docker image..."
	@docker build -t horarios-api:latest .
	@echo "Docker build complete"

# Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...
	@echo "Code formatted"

# Lint code (requires golangci-lint)
lint:
	@echo "Linting code..."
	@if command -v golangci-lint > /dev/null; then \
		golangci-lint run; \
	else \
		echo "Installing golangci-lint..."; \
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; \
		golangci-lint run; \
	fi

# Generate API documentation (requires swag)
docs:
	@echo "Generating API documentation..."
	@if command -v swag > /dev/null; then \
		swag init -g cmd/api/main.go; \
	else \
		echo "Installing swag..."; \
		go install github.com/swaggo/swag/cmd/swag@latest; \
		swag init -g cmd/api/main.go; \
	fi

# Help
help:
	@echo "Available commands:"
	@echo "  build         - Build the API binary"
	@echo "  lambda-build  - Build for AWS Lambda deployment"
	@echo "  run           - Run the API server (production)"
	@echo "  run-local     - Run the API server (local development)"
	@echo "  dev           - Run with hot reload (requires air)"
	@echo "  test          - Run tests"
	@echo "  test-coverage - Run tests with coverage report"
	@echo "  clean         - Clean build artifacts"
	@echo "  deps          - Install dependencies"
	@echo "  docker-build  - Build Docker image"
	@echo "  fmt           - Format code"
	@echo "  lint          - Lint code"
	@echo "  docs          - Generate API documentation"
	@echo "  help          - Show this help message"
