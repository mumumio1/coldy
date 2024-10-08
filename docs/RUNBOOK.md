# Operations Runbook

## Health checks

```bash
# Check all pods
kubectl get pods -n coldy-prod

# Check service endpoints
kubectl get svc -n coldy-prod

# Check pod logs
kubectl logs -n coldy-prod -l app=coldy-orders -f

# Check metrics
kubectl port-forward -n coldy-prod svc/prometheus 9090:9090
# Visit http://localhost:9090
```

## Common Incidents

### 1. High Latency Alert

Alert: OrderAPIHighLatency  
Threshold: p95 > 120ms for 5 minutes

Diagnosis:
```bash
# Check pod CPU/memory
kubectl top pods -n coldy-prod

# Check database connections
kubectl exec -n coldy-prod orders-0 -- curl localhost:9092/metrics | grep db_connections

# Check for slow queries
gcloud sql operations list --instance=coldy-db --filter="status=RUNNING"
```

Mitigation:
1. Scale up replicas: `kubectl scale deployment coldy-orders --replicas=10 -n coldy-prod`
2. Check database indexes: Review slow query log
3. Verify cache hit rate: Should be >90%

Long-term fix:
- Add database indexes on hot paths
- Increase cache TTL for stable data
- Optimize N+1 queries

### 2. Payment Circuit Breaker Open

Alert: CircuitBreakerOpen  
Impact: Payments failing

Diagnosis:
```bash
# Check payment provider status
curl https://status.payment-provider.com

# Check circuit breaker metrics
kubectl exec -n coldy-prod payments-0 -- curl localhost:9093/metrics | grep circuit_breaker

# Review recent errors
kubectl logs -n coldy-prod -l app=coldy-payments --tail=100 | grep error
```

Mitigation:
1. If provider is down: Wait for recovery (circuit auto-recovers in 30s)
2. If internal issue: Check payment service logs for errors
3. Manual reset: Restart payment pods (circuit resets)

Communication:
```
Payment Processing Degraded
Status: Experiencing issues with payment processing
Impact: ~X% of payments affected  
ETA: Investigating, updates every 15 min
Workaround: Retry failed payments after 5 minutes
```

### 3. Database Connection Saturation

Alert: DatabaseConnectionSaturation  
Threshold: >80% of max connections (25)

Diagnosis:
```bash
# Check active connections per service
kubectl exec -n coldy-prod orders-0 -- curl localhost:9092/metrics | grep db_connections

# List long-running queries
gcloud sql instances describe coldy-db
```

Immediate action:
1. Identify service with most connections
2. Check for connection leaks (not closing connections)
3. Increase connection pool if needed

Prevention:
- Set connection max lifetime (5 min)
- Use connection pooling properly
- Add connection timeout monitoring

### 4. High Error Rate

Alert: OrderAPIHighErrorRate  
Threshold: >0.8% error rate for 10 minutes

Diagnosis:
```bash
# Check error breakdown
kubectl logs -n coldy-prod -l app=coldy-orders | grep ERROR | tail -50

# Check dependent services
kubectl get pods -n coldy-prod | grep -v Running

# Check recent deployments
kubectl rollout history deployment/coldy-orders -n coldy-prod
```

Rollback:
```bash
# Rollback to previous version
kubectl rollout undo deployment/coldy-orders -n coldy-prod

# Verify rollback
kubectl rollout status deployment/coldy-orders -n coldy-prod
```

### 5. Outbox Processing Lag

Alert: OutboxProcessingLag  
Threshold: >100 unpublished events for 5 minutes

Symptoms: Events not being published to Pub/Sub

Diagnosis:
```bash
# Check outbox worker logs
kubectl logs -n coldy-prod -l app=coldy-orders | grep "outbox publisher"

# Check Pub/Sub quotas
gcloud pubsub topics list --project=coldy-prod

# Count unpublished events
kubectl exec -n coldy-prod orders-db-0 -- psql -c "SELECT COUNT(*) FROM outbox WHERE published = false"
```

Fix:
1. Check Pub/Sub permissions: Verify service account has publisher role
2. Increase worker frequency: Currently 5s, can reduce to 1s
3. Scale up workers: Add more order service replicas

## Deployment

### Standard Deployment

```bash
# Deploy to dev
helm upgrade --install coldy-dev ./deploy/helm/coldy \
  -n coldy-dev --create-namespace \
  -f deploy/helm/coldy/values-dev.yaml

# Deploy to prod (canary)
helm upgrade --install coldy ./deploy/helm/coldy \
  -n coldy-prod --create-namespace \
  -f deploy/helm/coldy/values-prod.yaml \
  --set global.imageTag=<NEW_TAG>
```

### Emergency Rollback

```bash
# Rollback all services
helm rollback coldy -n coldy-prod

# Rollback specific service
kubectl rollout undo deployment/coldy-orders -n coldy-prod
```

## Database Operations

### Run Migration

```bash
# Connect to database
gcloud sql connect coldy-db --user=coldy

# Run migration
migrate -path services/orders/migrations -database "postgres://..." up
```

### Backup and Restore

```bash
# Create backup
gcloud sql backups create --instance=coldy-db

# Restore from backup
gcloud sql backups restore <BACKUP_ID> --backup-instance=coldy-db --backup-instance=coldy-db-new
```

## Monitoring Queries

### Prometheus Queries

```promql
# Request rate per service
sum(rate(coldy_orders_requests_total[5m])) by (service)

# Error rate
sum(rate(coldy_orders_errors_total[5m])) / sum(rate(coldy_orders_requests_total[5m]))

# P95 latency
histogram_quantile(0.95, rate(coldy_orders_request_duration_seconds_bucket[5m]))

# Active database connections
coldy_orders_db_connections_active

# Circuit breaker state (0=closed, 1=half-open, 2=open)
coldy_payments_circuit_breaker_state
```

## Escalation

### Severity Levels

P0 (Critical) - Complete service outage - page on-call immediately

P1 (High) - Partial outage or severe degradation - notify on-call within 15 minutes

P2 (Medium) - Minor degradation, workaround available - create ticket, fix in next sprint

P3 (Low) - No customer impact - create ticket, backlog

### On-Call Rotation

- Primary: Responds within 15 min
- Secondary: Backup if primary unavailable (30 min)
- Manager: Escalation point for P0 incidents

### Communication Channels

- Incidents: `#coldy-incidents` (Slack)
- Status Page: `https://status.coldy.example.com`
- Postmortems: `docs/incidents/`

