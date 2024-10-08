# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /bin/orders ./services/orders/cmd/server

# Final stage
FROM gcr.io/distroless/static-debian11:nonroot

COPY --from=builder /bin/orders /bin/orders

EXPOSE 50053 9092

ENTRYPOINT ["/bin/orders"]

