#!/usr/bin/env bash
# Generate gRPC stubs from proto/*.proto.
#
# Usage:
#   ./gen-proto.sh          # Go + TypeScript
#   ./gen-proto.sh --go     # Go only
#   ./gen-proto.sh --ts     # TypeScript only

set -euo pipefail

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
CYAN='\033[0;36m'
NC='\033[0m'

ok() { echo -e "  ${GREEN}[ok] $*${NC}"; }
warn() { echo -e "  ${YELLOW}[warn] $*${NC}"; }
err() { echo -e "  ${RED}[err] $*${NC}"; }
info() { echo -e "  ${CYAN}-> $*${NC}"; }

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
PROTO_DIR="$ROOT/proto"
GO_OUT_ROOT="$ROOT/pkg/pb"
TS_OUT="$ROOT/api-gateway/src/proto"
SERVICES=("ca" "kdc" "bank")

GEN_GO=true
GEN_TS=true

if [[ $# -gt 0 ]]; then
  GEN_GO=false
  GEN_TS=false
  for arg in "$@"; do
    case "$arg" in
      --go) GEN_GO=true ;;
      --ts) GEN_TS=true ;;
      --help|-h)
        echo "Usage: $0 [--go] [--ts]"
        exit 0
        ;;
      *)
        err "Unknown option: $arg"
        exit 1
        ;;
    esac
  done
fi

echo ""
echo "Checking dependencies..."

missing=0

check_cmd() {
  local cmd=$1
  local hint=$2
  if command -v "$cmd" >/dev/null 2>&1; then
    ok "$cmd"
  else
    err "$cmd not found - $hint"
    missing=$((missing + 1))
  fi
}

check_cmd protoc "install protobuf compiler"

if $GEN_GO; then
  check_cmd protoc-gen-go "go install google.golang.org/protobuf/cmd/protoc-gen-go@latest"
  check_cmd protoc-gen-go-grpc "go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest"
fi

if $GEN_TS; then
  TS_PROTO_BIN="$ROOT/api-gateway/node_modules/.bin/protoc-gen-ts_proto"
  if [[ -f "${TS_PROTO_BIN}.cmd" ]]; then
    TS_PROTO_BIN="${TS_PROTO_BIN}.cmd"
  fi

  if [[ -f "$TS_PROTO_BIN" ]]; then
    ok "ts-proto (api-gateway/node_modules)"
  elif command -v protoc-gen-ts_proto >/dev/null 2>&1; then
    TS_PROTO_BIN=$(command -v protoc-gen-ts_proto)
    ok "ts-proto (global)"
  else
    warn "ts-proto not found; skipping TypeScript generation"
    GEN_TS=false
  fi
fi

if [[ $missing -gt 0 ]]; then
  echo ""
  err "Missing $missing required dependency/dependencies."
  echo ""
  echo "Install hints:"
  echo "  protobuf compiler: https://grpc.io/docs/protoc-installation/"
  echo "  protoc-gen-go: go install google.golang.org/protobuf/cmd/protoc-gen-go@latest"
  echo "  protoc-gen-go-grpc: go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest"
  echo "  ts-proto: cd api-gateway && npm install"
  exit 1
fi

for svc in "${SERVICES[@]}"; do
  if [[ ! -f "$PROTO_DIR/${svc}.proto" ]]; then
    err "Missing proto/${svc}.proto"
    exit 1
  fi
done

echo ""
echo "Generating..."

if $GEN_GO; then
  for svc in "${SERVICES[@]}"; do
    out="$GO_OUT_ROOT/$svc"
    mkdir -p "$out"
    info "Go -> pkg/pb/$svc"
    protoc \
      --proto_path="$PROTO_DIR" \
      --go_out="$out" --go_opt=paths=source_relative \
      --go-grpc_out="$out" --go-grpc_opt=paths=source_relative \
      "$PROTO_DIR/${svc}.proto"
    ok "$svc Go stubs generated"
  done
fi

if $GEN_TS; then
  mkdir -p "$TS_OUT"
  info "TypeScript -> api-gateway/src/proto"
  protoc \
    --proto_path="$PROTO_DIR" \
    --plugin=protoc-gen-ts_proto="$TS_PROTO_BIN" \
    --ts_proto_out="$TS_OUT" \
    --ts_proto_opt=outputServices=grpc-js \
    --ts_proto_opt=env=node \
    --ts_proto_opt=useOptionals=messages \
    --ts_proto_opt=esModuleInterop=true \
    "$PROTO_DIR/ca.proto" "$PROTO_DIR/kdc.proto" "$PROTO_DIR/bank.proto"
  ok "TypeScript stubs generated"
fi

echo ""
echo "Generated files:"
if $GEN_GO; then
  find "$GO_OUT_ROOT" -name "*.go" 2>/dev/null | sed "s|$ROOT/||" | sort |
    while read -r f; do echo "  $f"; done
fi
if $GEN_TS; then
  find "$TS_OUT" -name "*.ts" 2>/dev/null | sed "s|$ROOT/||" | sort |
    while read -r f; do echo "  $f"; done
fi

echo ""
echo "Done."
