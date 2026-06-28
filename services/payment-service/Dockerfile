# services/payment-service/Dockerfile
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
    -o /payment-service \
    ./cmd/payment-service

# Build the healthcheck helper tool
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /healthcheck \
    ./cmd/healthcheck/main.go

# ── Stage 2: Runtime (Distroless) ─────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot

LABEL org.opencontainers.image.title="payment-service" \
      org.opencontainers.image.description="Payment Service — payment processing and refunds" \
      org.opencontainers.image.version="1.0.0" \
      org.opencontainers.image.source="https://github.com/rohithsandiri/Summer_Project" \
      org.opencontainers.image.vendor="Enterprise" \
      org.opencontainers.image.licenses="MIT"

WORKDIR /app
COPY --from=builder /payment-service .
COPY --from=builder /healthcheck .

# Expose port
EXPOSE 8082

ENTRYPOINT ["./payment-service"]
