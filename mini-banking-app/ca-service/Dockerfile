## ── Stage 1: Builder ─────────────────────────────────────────
FROM golang:1.25-alpine AS builder
ENV GOPROXY=https://proxy.golang.org,direct
ENV GOSUMDB=sum.golang.org

WORKDIR /src

# Build context is the mini-banking-app runtime root because ca-service depends
# on the sibling pkg module through `replace mini_banking/pkg => ../pkg`.
COPY pkg/go.mod pkg/go.sum ./pkg/
COPY ca-service/go.mod ca-service/go.sum ./ca-service/
WORKDIR /src/ca-service
RUN go mod download

# Copy source và build
WORKDIR /src
COPY pkg ./pkg
COPY ca-service ./ca-service
WORKDIR /src/ca-service
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s" \
    -o /ca-service \
    ./cmd/server

## ── Stage 2: Dev (hot-reload với air) ────────────────────────
FROM golang:1.25-alpine AS dev
ENV GOPROXY=https://proxy.golang.org,direct
ENV GOSUMDB=sum.golang.org

WORKDIR /src/ca-service
RUN go install github.com/air-verse/air@latest

COPY pkg/go.mod pkg/go.sum /src/pkg/
COPY ca-service/go.mod ca-service/go.sum ./
RUN go mod download

COPY pkg /src/pkg
COPY ca-service .
CMD ["air", "-c", ".air.toml"]

## ── Stage 3: Production ──────────────────────────────────────
FROM alpine:3.19 AS prod

# ca-certificates để gRPC TLS hoạt động
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /ca-service /app/ca-service

# Thư mục certs sẽ được mount từ Docker volume
RUN mkdir -p /certs/root-ca /certs/issued /certs/ca-store

# Chạy với non-root user (bảo mật)
RUN addgroup -g 10001 -S cagroup \
    && adduser -u 10001 -S causer -G cagroup
RUN chown -R causer:cagroup /certs
USER causer

EXPOSE 50051

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
    CMD nc -z 127.0.0.1 50051 || exit 1

ENTRYPOINT ["/app/ca-service"]
