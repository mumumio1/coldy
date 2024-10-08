#!/bin/bash
set -e

# Configuration
PROJECT_ID="${PROJECT_ID:-coldy-prod}"
REGION="${REGION:-us-central1}"
ZONE="${ZONE:-us-central1-a}"
CLUSTER_NAME="${CLUSTER_NAME:-coldy-cluster}"

echo "=== Coldy GCP Setup ==="
echo "Project ID: $PROJECT_ID"
echo "Region: $REGION"
echo "Cluster: $CLUSTER_NAME"
echo ""

# Enable required APIs
echo "Enabling required Google Cloud APIs..."
gcloud services enable \
  container.googleapis.com \
  sqladmin.googleapis.com \
  redis.googleapis.com \
  pubsub.googleapis.com \
  secretmanager.googleapis.com \
  cloudtrace.googleapis.com \
  monitoring.googleapis.com \
  logging.googleapis.com \
  artifactregistry.googleapis.com \
  --project=$PROJECT_ID

# Create Artifact Registry repository
echo "Creating Artifact Registry repository..."
gcloud artifacts repositories create coldy \
  --repository-format=docker \
  --location=$REGION \
  --description="Coldy container images" \
  --project=$PROJECT_ID || true

# Create GKE Autopilot cluster
echo "Creating GKE Autopilot cluster..."
gcloud container clusters create-auto $CLUSTER_NAME \
  --region=$REGION \
  --project=$PROJECT_ID \
  --release-channel=regular \
  --enable-stackdriver-kubernetes \
  --workload-pool=$PROJECT_ID.svc.id.goog || true

# Get credentials
gcloud container clusters get-credentials $CLUSTER_NAME \
  --region=$REGION \
  --project=$PROJECT_ID

# Create Cloud SQL instance
echo "Creating Cloud SQL instance..."
gcloud sql instances create coldy-db \
  --database-version=POSTGRES_15 \
  --tier=db-custom-2-7680 \
  --region=$REGION \
  --network=default \
  --no-assign-ip \
  --project=$PROJECT_ID || true

# Create databases
gcloud sql databases create coldy \
  --instance=coldy-db \
  --project=$PROJECT_ID || true

# Create MemoryStore (Redis) instance
echo "Creating MemoryStore Redis instance..."
gcloud redis instances create coldy-cache \
  --size=1 \
  --region=$REGION \
  --redis-version=redis_7_0 \
  --network=default \
  --project=$PROJECT_ID || true

# Create Pub/Sub topics
echo "Creating Pub/Sub topics..."
topics=("order.created" "payment.succeeded" "payment.failed" "stock.reserved" "stock.released")
for topic in "${topics[@]}"; do
  gcloud pubsub topics create $topic --project=$PROJECT_ID || true
  gcloud pubsub subscriptions create ${topic}-sub \
    --topic=$topic \
    --ack-deadline=60 \
    --message-retention-duration=7d \
    --project=$PROJECT_ID || true
done

# Create Secret Manager secrets
echo "Creating secrets..."
echo -n "your-secure-password-here" | gcloud secrets create db-password \
  --data-file=- \
  --replication-policy=automatic \
  --project=$PROJECT_ID || true

echo -n "your-jwt-secret-key-here" | gcloud secrets create jwt-secret \
  --data-file=- \
  --replication-policy=automatic \
  --project=$PROJECT_ID || true

# Create service account for Workload Identity
echo "Setting up Workload Identity..."
gcloud iam service-accounts create coldy-sa \
  --display-name="Coldy Service Account" \
  --project=$PROJECT_ID || true

# Grant necessary permissions
gcloud projects add-iam-policy-binding $PROJECT_ID \
  --member="serviceAccount:coldy-sa@$PROJECT_ID.iam.gserviceaccount.com" \
  --role="roles/cloudsql.client"

gcloud projects add-iam-policy-binding $PROJECT_ID \
  --member="serviceAccount:coldy-sa@$PROJECT_ID.iam.gserviceaccount.com" \
  --role="roles/pubsub.publisher"

gcloud projects add-iam-policy-binding $PROJECT_ID \
  --member="serviceAccount:coldy-sa@$PROJECT_ID.iam.gserviceaccount.com" \
  --role="roles/pubsub.subscriber"

gcloud projects add-iam-policy-binding $PROJECT_ID \
  --member="serviceAccount:coldy-sa@$PROJECT_ID.iam.gserviceaccount.com" \
  --role="roles/secretmanager.secretAccessor"

echo ""
echo "=== Setup Complete ==="
echo ""
echo "Next steps:"
echo "1. Build and push Docker images: make docker-build-all && make docker-push-all"
echo "2. Deploy to GKE: make helm-install"
echo "3. Configure DNS for api.coldy.example.com"
echo ""

