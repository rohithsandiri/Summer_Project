# services/order-service/Dockerfile
#
# Production-ready hardened multi-stage Dockerfile.
# Switched to gcr.io/distroless/static-debian12:nonroot for minimal attack surface.

# ── Stage 1: Builder ──────────────────────────────────────────────────────────
FROM golang:1.23-bookworm AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download --verify

COPY . .

# Build the main service binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /order-service \
    ./cmd/order-service

# Build the healthcheck helper tool
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /healthcheck \
    ./cmd/healthcheck/main.go

# ── Stage 2: Runtime (Distroless) ─────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot

LABEL org.opencontainers.image.title="order-service" \
      org.opencontainers.image.description="Order Service — orchestrates inventory reservation and payment" \
      org.opencontainers.image.version="1.0.0" \
      org.opencontainers.image.source="https://github.com/rohithsandiri/Summer_Project" \
      org.opencontainers.image.vendor="Enterprise" \
      org.opencontainers.image.licenses="MIT"

WORKDIR /app
COPY --from=builder /order-service .
COPY --from=builder /healthcheck .

# Expose port
EXPOSE 8080

ENTRYPOINT ["./order-service"]
