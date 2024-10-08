.PHONY: help proto build test clean dev-up dev-down migrate-up migrate-down

# Variables
PROJECT_ID ?= coldy-prod
REGION ?= us-central1
CLUSTER_NAME ?= coldy-cluster

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

deps: ## Install dependencies
	@echo "Installing dependencies..."
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	go install github.com/golang-migrate/migrate/v4/cmd/migrate@latest
	go mod download

proto: ## Generate protobuf code
	@echo "Generating protobuf code..."
	@mkdir -p proto/gen
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		proto/common/v1/*.proto \
		proto/users/v1/*.proto \
		proto/catalog/v1/*.proto \
		proto/orders/v1/*.proto \
		proto/payments/v1/*.proto \
		proto/inventory/v1/*.proto

build-users: ## Build users service
	cd services/users && CGO_ENABLED=0 GOOS=linux go build -o ../../bin/users ./cmd/server

build-catalog: ## Build catalog service
	cd services/catalog && CGO_ENABLED=0 GOOS=linux go build -o ../../bin/catalog ./cmd/server

build-orders: ## Build orders service
	cd services/orders && CGO_ENABLED=0 GOOS=linux go build -o ../../bin/orders ./cmd/server

build-payments: ## Build payments service
	cd services/payments && CGO_ENABLED=0 GOOS=linux go build -o ../../bin/payments ./cmd/server

build-inventory: ## Build inventory service
	cd services/inventory && CGO_ENABLED=0 GOOS=linux go build -o ../../bin/inventory ./cmd/server

build-notification: ## Build notification service
	cd services/notification && CGO_ENABLED=0 GOOS=linux go build -o ../../bin/notification ./cmd/server

build-all: build-users build-catalog build-orders build-payments build-inventory build-notification ## Build all services

test: ## Run all tests
	go test -v -race -coverprofile=coverage.out ./...

test-integration: ## Run integration tests
	go test -v -tags=integration ./...

lint: ## Run linters
	golangci-lint run ./...

docker-build-all: ## Build all Docker images
	docker build -t gcr.io/$(PROJECT_ID)/users:latest -f services/users/Dockerfile .
	docker build -t gcr.io/$(PROJECT_ID)/catalog:latest -f services/catalog/Dockerfile .
	docker build -t gcr.io/$(PROJECT_ID)/orders:latest -f services/orders/Dockerfile .
	docker build -t gcr.io/$(PROJECT_ID)/payments:latest -f services/payments/Dockerfile .
	docker build -t gcr.io/$(PROJECT_ID)/inventory:latest -f services/inventory/Dockerfile .
	docker build -t gcr.io/$(PROJECT_ID)/notification:latest -f services/notification/Dockerfile .

docker-push-all: ## Push all Docker images
	docker push gcr.io/$(PROJECT_ID)/users:latest
	docker push gcr.io/$(PROJECT_ID)/catalog:latest
	docker push gcr.io/$(PROJECT_ID)/orders:latest
	docker push gcr.io/$(PROJECT_ID)/payments:latest
	docker push gcr.io/$(PROJECT_ID)/inventory:latest
	docker push gcr.io/$(PROJECT_ID)/notification:latest

dev-up: ## Start local development environment
	docker-compose up -d

dev-down: ## Stop local development environment
	docker-compose down

migrate-create: ## Create new migration (usage: make migrate-create NAME=create_users_table)
	migrate create -ext sql -dir services/users/migrations -seq $(NAME)

migrate-up: ## Run migrations up
	migrate -path services/users/migrations -database "$(DB_URL)" up

migrate-down: ## Run migrations down
	migrate -path services/users/migrations -database "$(DB_URL)" down 1

k6-smoke: ## Run smoke test
	k6 run ops/k6/smoke.js

k6-load: ## Run load test
	k6 run ops/k6/steady.js

k6-spike: ## Run spike test
	k6 run ops/k6/spike.js

helm-install: ## Install with Helm
	helm upgrade --install coldy deploy/helm/coldy \
		--namespace coldy --create-namespace \
		-f deploy/helm/coldy/values-dev.yaml

helm-uninstall: ## Uninstall Helm release
	helm uninstall coldy --namespace coldy

clean: ## Clean build artifacts
	rm -rf bin/
	rm -rf coverage.out
	go clean -cache

