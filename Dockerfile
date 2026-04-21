# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o network-service ./cmd/api


# Final stage
FROM alpine:latest

WORKDIR /app

# required runtime deps for iptables/network tools
RUN apk add --no-cache \
    ca-certificates \
    iptables \
    iproute2 \
    bridge-utils

COPY --from=builder /app/network-service .
COPY --from=builder /app/internal/infrastructure/migrations ./internal/infrastructure/migrations
COPY --from=builder /app/docs ./docs

EXPOSE 8084

CMD ["./network-service"]