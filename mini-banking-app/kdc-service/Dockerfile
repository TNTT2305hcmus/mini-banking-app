## ── Stage 1: Builder ─────────────────────────────────────────
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Copy go.mod trước để cache layer dependency
COPY go.mod go.sum ./
RUN go mod download

# Copy source và build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s" \
    -o /kdc-service \
    ./cmd/server

## ── Stage 2: Dev (hot-reload với air) ────────────────────────
FROM golang:1.22-alpine AS dev

WORKDIR /app
RUN go install github.com/air-verse/air@latest

COPY go.mod go.sum ./
RUN go mod download

COPY . .
CMD ["air", "-c", ".air.toml"]

## ── Stage 3: Production ──────────────────────────────────────
FROM alpine:3.19 AS prod

# ca-certificates để gRPC TLS hoạt động
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /kdc-service /app/kdc-service

# Thư mục certs sẽ được mount từ Docker volume
RUN mkdir -p /certs/kdc

# Chạy với non-root user (bảo mật)
RUN addgroup -S kdcgroup && adduser -S kdcuser -G kdcgroup
RUN chown -R kdcuser:kdcgroup /certs
USER kdcuser

EXPOSE 50051

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
    CMD grpc_health_probe -addr=:50051 || exit 1

ENTRYPOINT ["/app/kdc-service"]
