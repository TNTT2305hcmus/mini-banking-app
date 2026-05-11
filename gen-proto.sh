#!/usr/bin/env bash
# =============================================================
# scripts/gen-proto.sh
# Generate gRPC stubs from .proto files for all of services
#
# USE:
#   chmod +x scripts/gen-proto.sh
#   ./scripts/gen-proto.sh          # generate all
#   ./scripts/gen-proto.sh --go     # just Go stubs
#   ./scripts/gen-proto.sh --ts     # just TypeScript stubs
#
# REQUIRE (see the INSTALL section below if anything is missing):
#   - protoc
#   - protoc-gen-go + protoc-gen-go-grpc
#   - ts-proto (for TypeScript Gateway)
# =============================================================

set -euo pipefail

GREEN='\033[0;32m'; YELLOW='\033[1;33m'; RED='\033[0;31m'; CYAN='\033[0;36m'; NC='\033[0m'
ok()   { echo -e "  ${GREEN}✅ $*${NC}"; }
warn() { echo -e "  ${YELLOW}⚠️  $*${NC}"; }
err()  { echo -e "  ${RED}❌ $*${NC}"; }
info() { echo -e "  ${CYAN}→  $*${NC}"; }

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
PROTO_DIR="$ROOT/proto"

# ============================== Parse args ===================================
GEN_GO=true
GEN_TS=true
if [[ $# -gt 0 ]]; then
  GEN_GO=false; GEN_TS=false
  for arg in "$@"; do
    case $arg in
      --go) GEN_GO=true ;;
      --ts) GEN_TS=true ;;
      --help|-h)
        echo "Usage: $0 [--go] [--ts]"
        echo "  --go   Chỉ generate Go stubs"
        echo "  --ts   Chỉ generate TypeScript stubs"
        exit 0 ;;
      *) err "Unknown option: $arg"; exit 1 ;;
    esac
  done
fi

# ========================== Check dependencies ===============================
echo ""
echo " Checking dependencies..."

MISSING=0

check_cmd() {
  local cmd=$1 hint=$2
  if command -v "$cmd" &>/dev/null; then
    ok "$cmd"
    return 0
  else
    err "$cmd not found  →  $hint"
    return 1
  fi
}

check_cmd protoc \
  "apt install protobuf-compiler  |  brew install protobuf" || MISSING=$((MISSING+1))

if $GEN_GO; then
  check_cmd protoc-gen-go \
    "go install google.golang.org/protobuf/cmd/protoc-gen-go@latest" || MISSING=$((MISSING+1))
  check_cmd protoc-gen-go-grpc \
    "go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest" || MISSING=$((MISSING+1))
fi

if $GEN_TS; then
  TS_PROTO_BIN="$ROOT/gateway/node_modules/.bin/protoc-gen-ts_proto"
  if [[ -f "$TS_PROTO_BIN" ]]; then
    ok "ts-proto (gateway/node_modules)"
  elif command -v protoc-gen-ts_proto &>/dev/null; then
    ok "ts-proto (global)"
    TS_PROTO_BIN=$(command -v protoc-gen-ts_proto)
  else
    warn "ts-proto not found — skip TypeScript. Fix: cd gateway && npm install ts-proto"
    GEN_TS=false
  fi
fi

if [[ $MISSING -gt 0 ]]; then
  echo ""
  err "Missing $MISSING compulsory dependency. Install before running again."
  echo ""
  echo "  macOS:  brew install protobuf"
  echo "  Linux:  apt install -y protobuf-compiler"
  echo "  Go:     go install google.golang.org/protobuf/cmd/protoc-gen-go@latest"
  echo "          go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest"
  echo ""
  exit 1
fi

echo ""
echo " Generating..."
echo ""

# ============================== Go Stubs ===================================
if $GEN_GO; then
  for service in ca kdc bank; do
    case $service in
      ca)   svc_dir="$ROOT/ca-service";   proto_file="ca/ca.proto" ;;
      kdc)  svc_dir="$ROOT/kdc-service";  proto_file="kdc/kdc.proto" ;;
      bank) svc_dir="$ROOT/bank-service"; proto_file="bank/bank.proto" ;;
    esac

    OUT="$svc_dir/internal/grpc/pb"
    mkdir -p "$OUT"
    info "Go → ${service}-service/internal/grpc/pb/"

    protoc \
      --proto_path="$PROTO_DIR" \
      --go_out="$OUT" --go_opt=paths=source_relative \
      --go-grpc_out="$OUT" --go-grpc_opt=paths=source_relative \
      "$proto_file"

    ok "${service}-service"
  done
fi

# ====================== TypeScript Stubs (Gateway) ==========================
if $GEN_TS; then
  TS_OUT="$ROOT/gateway/src/proto"
  mkdir -p "$TS_OUT"
  info "TypeScript → gateway/src/proto/"

  protoc \
    --proto_path="$PROTO_DIR" \
    --plugin="$TS_PROTO_BIN" \
    --ts_proto_out="$TS_OUT" \
    --ts_proto_opt=outputServices=grpc-js \
    --ts_proto_opt=env=node \
    --ts_proto_opt=useOptionals=messages \
    --ts_proto_opt=esModuleInterop=true \
    ca/ca.proto kdc/kdc.proto bank/bank.proto

  ok "gateway TypeScript stubs"
fi

# ========================== Summary ==============================
echo ""
echo "────────────────────────────────────────────────────"
echo "✅ Done! Files generated:"
echo ""
if $GEN_GO; then
  find "$ROOT" -path "*/internal/grpc/pb/*.go" 2>/dev/null | sed "s|$ROOT/||" | sort | \
    while read -r f; do echo "    📄 $f"; done
fi
if $GEN_TS; then
  find "$ROOT/gateway/src/proto" -name "*.ts" 2>/dev/null | sed "s|$ROOT/||" | sort | \
    while read -r f; do echo "    📄 $f"; done
fi
echo ""
echo "  Tip: Using 'make proto' instead of calling script directly"
echo "────────────────────────────────────────────────────"
echo ""
