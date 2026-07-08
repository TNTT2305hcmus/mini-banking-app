## ── Stage 1: Builder ─────────────────────────────────────────
FROM golang:1.25-alpine AS builder

WORKDIR /src

# Build context là mini-banking-app/ vì banking-service phụ thuộc
# vào module pkg anh em qua `replace mini-banking/pkg => ../pkg`.
COPY pkg/go.mod pkg/go.sum ./pkg/
COPY banking-service/go.mod banking-service/go.sum ./banking-service/
WORKDIR /src/banking-service
RUN go mod download

# Copy source và build
WORKDIR /src
COPY pkg ./pkg
COPY banking-service ./banking-service
WORKDIR /src/banking-service
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s" \
    -o /banking-service \
    ./cmd/server

## ── Stage 2: Dev (hot-reload với air) ────────────────────────
FROM golang:1.25-alpine AS dev

WORKDIR /src/banking-service
RUN go install github.com/air-verse/air@latest

COPY pkg/go.mod pkg/go.sum /src/pkg/
COPY banking-service/go.mod banking-service/go.sum ./
RUN go mod download

COPY pkg /src/pkg
COPY banking-service .
CMD ["air", "-c", ".air.toml"]

## ── Stage 3: Production ──────────────────────────────────────
FROM alpine:3.19 AS prod

# ca-certificates để gRPC TLS hoạt động
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /banking-service /app/banking-service

# Thư mục certs sẽ được mount từ Docker volume
RUN mkdir -p /certs/grpc

# Chạy với non-root user (bảo mật)
RUN addgroup -g 10003 -S bankgroup \
    && adduser -u 10003 -S bankuser -G bankgroup
RUN chown -R bankuser:bankgroup /certs
USER bankuser

# BANK_GRPC_PORT=50053 theo banking-service/.env.example
EXPOSE 50053

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
    CMD nc -z 127.0.0.1 50053 || exit 1

ENTRYPOINT ["/app/banking-service"]
