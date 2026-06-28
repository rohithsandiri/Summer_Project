# operator/Dockerfile
#
# Production-ready hardened multi-stage Dockerfile for the Self-Healing Operator.
# Switched to gcr.io/distroless/static-debian12:nonroot for minimal attack surface.

# ── Stage 1: Builder ──────────────────────────────────────────────────────────
FROM golang:1.23-bookworm AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build the main operator binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /rollback-operator \
    ./cmd/operator

# Build the healthcheck helper tool
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /healthcheck \
    ./cmd/healthcheck/main.go

# ── Stage 2: Runtime (Distroless) ─────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot

LABEL org.opencontainers.image.title="self-healing-operator" \
      org.opencontainers.image.description="Kubernetes SRE Control Plane Operator for automated Helm recovery" \
      org.opencontainers.image.version="1.0.0" \
      org.opencontainers.image.source="https://github.com/rohithsandiri/Summer_Project" \
      org.opencontainers.image.vendor="Enterprise" \
      org.opencontainers.image.licenses="MIT"

WORKDIR /app
COPY --from=builder /rollback-operator .
COPY --from=builder /healthcheck .

EXPOSE 8090

ENTRYPOINT ["./rollback-operator"]
