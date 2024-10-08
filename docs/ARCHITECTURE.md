# Architecture

## Overview

6 microservices handling orders, payments, inventory with event-driven communication.

## Core patterns

- Event-driven via Pub/Sub
- Idempotency with Redis
- Optimistic locking for concurrency
- Circuit breakers for external deps
- Outbox pattern for reliable events

## Services

Users - JWT auth, bcrypt passwords, PostgreSQL  
Catalog - Products with Redis cache (5min TTL), full-text search  
Orders - Order management, outbox pattern, idempotent POST  
Payments - Payment processing, circuit breaker, mock provider  
Inventory - Stock reservation, optimistic locking (version column)  
Notification - Pub/Sub consumer, sends emails/webhooks

## Order flow

1. POST /orders creates order + writes to outbox (transaction)
2. Worker reads outbox, publishes to Pub/Sub
3. Inventory reserves stock
4. Payment processes
5. On success: commit stock, update order
6. On failure: release stock, cancel order

## Reliability

Outbox pattern - write event in same transaction, worker publishes later  
Idempotency - Redis keys (sha256 hash), 24h TTL  
Optimistic locking - version column in inventory table  
Circuit breaker - 5 failures opens circuit for 30s

## Monitoring

- Prometheus for metrics (RED + USE patterns)
- OpenTelemetry for distributed tracing
- Structured logs with zap
- Alerts on SLO violations (p95 latency, error rate)

## Deployment

GKE Autopilot with Helm charts. Canary deploys: 10% → 50% → 100% with automatic rollback on SLO breach.

Migrations are two-phase to avoid downtime.

