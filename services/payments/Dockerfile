# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /bin/payments ./services/payments/cmd/server

# Final stage
FROM gcr.io/distroless/static-debian11:nonroot

COPY --from=builder /bin/payments /bin/payments

EXPOSE 50054 9093

ENTRYPOINT ["/bin/payments"]

