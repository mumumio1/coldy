# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /bin/inventory ./services/inventory/cmd/server

# Final stage
FROM gcr.io/distroless/static-debian11:nonroot

COPY --from=builder /bin/inventory /bin/inventory

EXPOSE 50055 9094

ENTRYPOINT ["/bin/inventory"]

