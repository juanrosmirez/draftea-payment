# Variables
BINARY_NAME=payment-system
DOCKER_IMAGE=payment-system:latest
COMPOSE_FILE=docker-compose.yml

# Go commands
.PHONY: build
build:
	go build -o bin/$(BINARY_NAME) ./cmd/main.go

.PHONY: run
run:
	go run ./cmd/main.go

.PHONY: test
test:
	go test -v ./...

.PHONY: test-coverage
test-coverage:
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

.PHONY: clean
clean:
	go clean
	rm -f bin/$(BINARY_NAME)
	rm -f coverage.out

# Docker commands
.PHONY: docker-build
docker-build:
	docker build -t $(DOCKER_IMAGE) .

.PHONY: docker-run
docker-run:
	docker run -p 8080:8080 -p 9091:9091 $(DOCKER_IMAGE)

# Docker Compose commands
.PHONY: up
up:
	docker-compose -f $(COMPOSE_FILE) up -d

.PHONY: down
down:
	docker-compose -f $(COMPOSE_FILE) down

.PHONY: logs
logs:
	docker-compose -f $(COMPOSE_FILE) logs -f

.PHONY: restart
restart: down up

# Database commands
.PHONY: db-migrate
db-migrate:
	docker-compose -f $(COMPOSE_FILE) exec postgres psql -U postgres -d payment_system -f /docker-entrypoint-initdb.d/001_create_tables.sql

.PHONY: db-shell
db-shell:
	docker-compose -f $(COMPOSE_FILE) exec postgres psql -U postgres -d payment_system

# Development commands
.PHONY: dev
dev:
	docker-compose -f $(COMPOSE_FILE) up -d postgres redis kafka prometheus grafana
	go run ./cmd/main.go

.PHONY: deps
deps:
	go mod download
	go mod tidy

.PHONY: lint
lint:
	golangci-lint run

.PHONY: format
format:
	go fmt ./...

# Help
.PHONY: help
help:
	@echo "Available commands:"
	@echo "  build         - Build the application"
	@echo "  run           - Run the application locally"
	@echo "  test          - Run tests"
	@echo "  test-coverage - Run tests with coverage"
	@echo "  clean         - Clean build artifacts"
	@echo "  docker-build  - Build Docker image"
	@echo "  docker-run    - Run Docker container"
	@echo "  up            - Start all services with Docker Compose"
	@echo "  down          - Stop all services"
	@echo "  logs          - View logs from all services"
	@echo "  restart       - Restart all services"
	@echo "  db-migrate    - Run database migrations"
	@echo "  db-shell      - Open database shell"
	@echo "  dev           - Start dependencies and run app locally"
	@echo "  deps          - Download and tidy dependencies"
	@echo "  lint          - Run linter"
	@echo "  format        - Format code"
