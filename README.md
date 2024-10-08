# Coldy

E-commerce platform with microservices on GKE.

## Services

- `users` - auth, user management
- `catalog` - products, pricing
- `orders` - order processing
- `payments` - payment handling
- `inventory` - stock management
- `notification` - events and notifications

## Stack

- Go 1.21
- PostgreSQL (Cloud SQL)
- Redis (MemoryStore)
- Pub/Sub for events
- gRPC for service communication
- OpenTelemetry for tracing
- Helm for deployment

## Setup

### Prerequisites

```bash
# Install protoc compiler
brew install protobuf  # macOS
# or download from https://github.com/protocolbuffers/protobuf/releases

# Install Go plugins
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

### Local dev

```bash
# Generate proto files first
make proto

# Start infrastructure
docker-compose up -d

# Run migrations
make migrate-up

# Run services (each in separate terminal)
cd services/users && go run cmd/server/main.go
cd services/catalog && go run cmd/server/main.go
# etc...
```

### Deploy to GCP

```bash
./scripts/setup-gcp.sh
make docker-build-all
make docker-push-all
helm install coldy ./deploy/helm/coldy -n coldy-prod
```

## Key patterns

- Outbox pattern for reliable event publishing
- Idempotency keys in Redis (24h TTL)
- Optimistic locking for inventory
- Circuit breaker on payment provider

## Tests

```bash
make test
k6 run ops/k6/steady.js
```

## Docs

- `docs/ARCHITECTURE.md` - system design
- `docs/DEPLOYMENT.md` - deployment guide
- `docs/RUNBOOK.md` - operations

