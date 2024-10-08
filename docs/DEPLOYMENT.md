# Deployment Guide

## Prerequisites

- Go 1.21+
- Docker & Docker Compose
- kubectl & Helm 3.12+
- GCP account with billing enabled
- gcloud CLI configured

## Local Development

### 1. Start Infrastructure

```bash
# Start PostgreSQL, Redis, Pub/Sub emulator, Jaeger
docker-compose up -d

# Verify services
docker-compose ps
```

### 2. Run Migrations

```bash
# Users service
migrate -path services/users/migrations \
  -database "postgres://coldy:coldy123@localhost:5432/coldy?sslmode=disable" up

# Catalog service
migrate -path services/catalog/migrations \
  -database "postgres://coldy:coldy123@localhost:5432/coldy?sslmode=disable" up

# Orders service
migrate -path services/orders/migrations \
  -database "postgres://coldy:coldy123@localhost:5432/coldy?sslmode=disable" up

# Payments service
migrate -path services/payments/migrations \
  -database "postgres://coldy:coldy123@localhost:5432/coldy?sslmode=disable" up

# Inventory service
migrate -path services/inventory/migrations \
  -database "postgres://coldy:coldy123@localhost:5432/coldy?sslmode=disable" up
```

### 3. Generate Proto Files

```bash
make proto
```

### 4. Run Services

```bash
# Terminal 1: Users service
cd services/users && go run cmd/server/main.go

# Terminal 2: Catalog service
cd services/catalog && go run cmd/server/main.go

# Terminal 3: Orders service
cd services/orders && go run cmd/server/main.go

# Terminal 4: Payments service
cd services/payments && go run cmd/server/main.go

# Terminal 5: Inventory service
cd services/inventory && go run cmd/server/main.go

# Terminal 6: Notification service
cd services/notification && go run cmd/server/main.go
```

## GCP Deployment

### 1. Setup GCP Infrastructure

```bash
export PROJECT_ID="coldy-prod"
export REGION="us-central1"

# Run setup script
./scripts/setup-gcp.sh
```

This creates:
- GKE Autopilot cluster
- Cloud SQL (PostgreSQL)
- MemoryStore (Redis)
- Pub/Sub topics & subscriptions
- Secret Manager secrets
- Artifact Registry
- IAM service accounts

### 2. Build and Push Images

```bash
# Authenticate Docker
gcloud auth configure-docker gcr.io

# Build all services
make docker-build-all

# Push to Artifact Registry
make docker-push-all
```

### 3. Configure Secrets

```bash
# Create database password secret
kubectl create secret generic coldy-db-secret \
  --from-literal=password='YOUR_DB_PASSWORD' \
  -n coldy-prod

# Create JWT secret
kubectl create secret generic coldy-jwt-secret \
  --from-literal=secret='YOUR_JWT_SECRET' \
  -n coldy-prod
```

### 4. Deploy with Helm

```bash
# Get GKE credentials
gcloud container clusters get-credentials coldy-cluster \
  --region us-central1 \
  --project coldy-prod

# Deploy to production
helm upgrade --install coldy ./deploy/helm/coldy \
  --namespace coldy-prod \
  --create-namespace \
  -f deploy/helm/coldy/values.yaml \
  --set global.projectId=coldy-prod \
  --set global.imageTag=latest \
  --wait \
  --timeout 10m
```

### 5. Run Migrations in Production

```bash
# Port-forward to database
gcloud sql instances describe coldy-db

# Get private IP and connect
kubectl run -it --rm migrate --image=migrate/migrate:latest \
  --restart=Never \
  -- -path /migrations -database "postgres://..." up
```

### 6. Verify Deployment

```bash
# Check pods
kubectl get pods -n coldy-prod

# Check services
kubectl get svc -n coldy-prod

# Check ingress
kubectl get ingress -n coldy-prod

# View logs
kubectl logs -n coldy-prod -l app=coldy-orders -f
```

## CI/CD Pipeline

### GitHub Actions Workflow

The CI/CD pipeline automatically:

1. **Lint & Test**: Run golangci-lint, gosec, Trivy, unit tests
2. **Build**: Build Docker images for all services
3. **Push**: Push to Artifact Registry
4. **Deploy Dev**: Auto-deploy to dev on `develop` branch
5. **Deploy Prod**: Canary deployment to prod on `main` branch
   - 10% traffic → monitor 5min → check SLO
   - 50% traffic → monitor 5min → check SLO
   - 100% traffic (or rollback on SLO violation)

### Required Secrets

Configure in GitHub repository settings:

- `WIF_PROVIDER`: Workload Identity Federation provider
- `WIF_SERVICE_ACCOUNT`: Service account email
- `SLACK_WEBHOOK`: Slack webhook for notifications

### Manual Deploy

```bash
# Trigger deployment
git tag v1.0.0
git push origin v1.0.0

# Or use GitHub Actions UI to trigger workflow
```

## Monitoring

### Access Dashboards

```bash
# Prometheus
kubectl port-forward -n coldy-prod svc/prometheus 9090:9090

# Grafana
kubectl port-forward -n coldy-prod svc/grafana 3000:3000

# Jaeger
kubectl port-forward -n coldy-prod svc/jaeger 16686:16686
```

### View Metrics

- Prometheus: http://localhost:9090
- Grafana: http://localhost:3000 (admin/admin)
- Jaeger: http://localhost:16686

### Check SLO Compliance

```promql
# Order API p95 latency
histogram_quantile(0.95, rate(coldy_orders_request_duration_seconds_bucket[5m]))

# Order API error rate
sum(rate(coldy_orders_errors_total[10m])) / sum(rate(coldy_orders_requests_total[10m]))
```

## Load Testing

### Run k6 Tests

```bash
# Smoke test (basic health check)
k6 run ops/k6/smoke.js

# Steady state load test
BASE_URL=https://api.coldy.example.com \
AUTH_TOKEN=your_token \
k6 run ops/k6/steady.js

# Spike test
k6 run ops/k6/spike.js

# Idempotency test
k6 run ops/k6/idempotency.js
```

## Troubleshooting

### Pods Not Starting

```bash
# Describe pod to see events
kubectl describe pod coldy-orders-xxx -n coldy-prod

# Check logs
kubectl logs coldy-orders-xxx -n coldy-prod

# Common issues:
# - Image pull errors: Check Artifact Registry permissions
# - CrashLoopBackOff: Check application logs for startup errors
# - Secrets not found: Ensure secrets are created in namespace
```

### Database Connection Issues

```bash
# Check Cloud SQL proxy
kubectl logs -n coldy-prod -l app=cloud-sql-proxy

# Verify network connectivity
kubectl run -it --rm debug --image=busybox --restart=Never -- \
  nc -zv 10.0.0.3 5432
```

### High Latency

```bash
# Check HPA status
kubectl get hpa -n coldy-prod

# Scale manually if needed
kubectl scale deployment coldy-orders --replicas=10 -n coldy-prod

# Check database performance
gcloud sql operations list --instance=coldy-db
```

## Rollback

### Helm Rollback

```bash
# List releases
helm history coldy -n coldy-prod

# Rollback to previous
helm rollback coldy -n coldy-prod

# Rollback to specific revision
helm rollback coldy 5 -n coldy-prod
```

### Kubectl Rollback

```bash
# Rollback deployment
kubectl rollout undo deployment/coldy-orders -n coldy-prod

# Check status
kubectl rollout status deployment/coldy-orders -n coldy-prod
```

## Scaling

### Manual Scaling

```bash
# Scale specific service
kubectl scale deployment coldy-orders --replicas=10 -n coldy-prod
```

### HPA (Horizontal Pod Autoscaler)

Already configured in Helm chart:
- Min replicas: 3
- Max replicas: 10-15 (depending on service)
- Target CPU: 70%

### Verify HPA

```bash
kubectl get hpa -n coldy-prod
kubectl describe hpa coldy-orders -n coldy-prod
```

## Maintenance

### Update Dependencies

```bash
go get -u ./...
go mod tidy
go mod vendor
```

### Security Updates

```bash
# Run security scan
gosec ./...
trivy fs .

# Update vulnerable dependencies
go get -u github.com/vulnerable/package
```

### Database Maintenance

```bash
# Create backup
gcloud sql backups create --instance=coldy-db

# List backups
gcloud sql backups list --instance=coldy-db

# Restore backup
gcloud sql backups restore BACKUP_ID \
  --backup-instance=coldy-db \
  --backup-instance=coldy-db-restored
```

## Cost Optimization

### Monitor Costs

```bash
# View GKE costs
gcloud billing accounts list
gcloud billing projects describe coldy-prod
```

