# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /bin/catalog ./services/catalog/cmd/server

# Final stage
FROM gcr.io/distroless/static-debian11:nonroot

COPY --from=builder /bin/catalog /bin/catalog

EXPOSE 50052 9091

ENTRYPOINT ["/bin/catalog"]

