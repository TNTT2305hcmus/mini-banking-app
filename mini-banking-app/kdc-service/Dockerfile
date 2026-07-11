## ── Stage 1: Builder ─────────────────────────────────────────
FROM golang:1.25-alpine AS builder
ENV GOPROXY=https://proxy.golang.org,direct
ENV GOSUMDB=sum.golang.org

WORKDIR /src

# Build context phải là mini-banking-app/ vì kdc-service phụ thuộc
# vào module pkg anh em qua `replace mini_banking/pkg => ../pkg`.
COPY pkg/go.mod pkg/go.sum ./pkg/
COPY kdc-service/go.mod kdc-service/go.sum ./kdc-service/
WORKDIR /src/kdc-service
RUN go mod download

# Copy source và build
WORKDIR /src
COPY pkg ./pkg
COPY kdc-service ./kdc-service
WORKDIR /src/kdc-service
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s" \
    -o /kdc-service \
    ./cmd/server

## ── Stage 2: Dev (hot-reload với air) ────────────────────────
FROM golang:1.25-alpine AS dev
ENV GOPROXY=https://proxy.golang.org,direct
ENV GOSUMDB=sum.golang.org

WORKDIR /src/kdc-service
RUN go install github.com/air-verse/air@latest

COPY pkg/go.mod pkg/go.sum /src/pkg/
COPY kdc-service/go.mod kdc-service/go.sum ./
RUN go mod download

COPY pkg /src/pkg
COPY kdc-service .
CMD ["air", "-c", ".air.toml"]

## ── Stage 3: Production ──────────────────────────────────────
FROM alpine:3.19 AS prod

# ca-certificates để gRPC TLS hoạt động
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /kdc-service /app/kdc-service

# Thư mục certs sẽ được mount từ Docker volume
RUN mkdir -p /certs/kdc /certs/grpc

# Chạy với non-root user (bảo mật)
RUN addgroup -g 10002 -S kdcgroup \
    && adduser -u 10002 -S kdcuser -G kdcgroup
RUN chown -R kdcuser:kdcgroup /certs
USER kdcuser

# GRPC_PORT=50052 theo kdc-service/.env.example
EXPOSE 50052

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
    CMD nc -z 127.0.0.1 50052 || exit 1

ENTRYPOINT ["/app/kdc-service"]
