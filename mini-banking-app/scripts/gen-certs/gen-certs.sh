#!/usr/bin/env bash
#
# gen-certs.sh — Generate the gRPC transport TLS material for the mini-banking stack.
#
# Layout produced:
#   1. One internal transport CA           -> out/grpc-ca.{crt,key}
#   2. CA service   gRPC server cert        -> ca-service/certs/grpc/ca-server.{crt,key}
#   3. KDC service  gRPC server cert        -> kdc-service/certs/kdc-server.{crt,key}
#   4. Bank service gRPC server cert        -> banking-service/certs/grpc/bank-server.{crt,key}
#   5. The CA cert is fanned out to every party that must verify a server:
#        - api-gateway/certs/grpc-ca.crt         (gateway CA_CERT_PATH)
#        - kdc-service/certs/grpc-ca.crt          (KDC gRPC server bootstrap)
#        - kdc-service/certs/grpc/ca-server-ca.crt (KDC -> CA client trust anchor)
#        - banking-service/certs/grpc-ca.crt      (Bank -> CA client trust anchor)
#
# These are gRPC *transport* certs. They are unrelated to the application-level
# Root CA that the CA service uses to issue client certificates (certs/root-ca).
#
# Usage:
#   ./gen-certs.sh            # 825-day certs
#   DAYS=90 ./gen-certs.sh    # override validity
#
set -euo pipefail

# On Git-bash / MSYS (Windows) a leading-slash argument like -subj "/C=VN/..." is
# rewritten into a Windows path (e.g. "C:/Program Files/Git/C=VN/..."), which makes
# openssl reject the subject. Exclude only the subject (it always starts with /C=)
# from conversion so real file-path arguments are still translated correctly.
# This variable is inert on Linux/macOS.
export MSYS2_ARG_CONV_EXCL='/C='

# --- locate repo root (this script lives in <root>/scripts/gen-certs) ----------
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
OUT="$SCRIPT_DIR/out"
DAYS="${DAYS:-825}"

mkdir -p "$OUT"

echo "==> Generating internal gRPC transport CA"
if [[ ! -f "$OUT/grpc-ca.key" ]]; then
  openssl genrsa -out "$OUT/grpc-ca.key" 4096
  openssl req -x509 -new -nodes -key "$OUT/grpc-ca.key" -sha256 -days "$((DAYS * 4))" \
    -subj "/C=VN/O=Mini_App_Banking/CN=Mini_App_Banking Internal gRPC CA" \
    -out "$OUT/grpc-ca.crt"
  echo "    created CA"
else
  echo "    reusing existing CA ($OUT/grpc-ca.crt)"
fi

# gen_server <name> <cn> <san-csv> <dest-crt> <dest-key>
gen_server() {
  local name="$1" cn="$2" sans="$3" dest_crt="$4" dest_key="$5"
  local tmp; tmp="$(mktemp -d)"

  echo "==> $name server certificate (CN=$cn)"
  openssl genrsa -out "$tmp/srv.key" 2048
  openssl req -new -key "$tmp/srv.key" -subj "/C=VN/O=Mini_App_Banking/CN=$cn" -out "$tmp/srv.csr"

  printf 'subjectAltName=%s\nextendedKeyUsage=serverAuth\nkeyUsage=digitalSignature,keyEncipherment\n' "$sans" > "$tmp/ext.cnf"

  openssl x509 -req -in "$tmp/srv.csr" \
    -CA "$OUT/grpc-ca.crt" -CAkey "$OUT/grpc-ca.key" -CAcreateserial \
    -days "$DAYS" -sha256 -extfile "$tmp/ext.cnf" -out "$tmp/srv.crt"

  mkdir -p "$(dirname "$dest_crt")" "$(dirname "$dest_key")"
  cp "$tmp/srv.crt" "$dest_crt"
  cp "$tmp/srv.key" "$dest_key"
  echo "    -> $dest_crt"
  rm -rf "$tmp"
}

gen_server "CA"   "ca-service" \
  "DNS:ca-service,DNS:localhost,IP:127.0.0.1" \
  "$ROOT/ca-service/certs/grpc/ca-server.crt" \
  "$ROOT/ca-service/certs/grpc/ca-server.key"

gen_server "KDC"  "kdc-service" \
  "DNS:kdc-service,DNS:localhost,IP:127.0.0.1" \
  "$ROOT/kdc-service/certs/kdc-server.crt" \
  "$ROOT/kdc-service/certs/kdc-server.key"

gen_server "Bank" "banking-service" \
  "DNS:banking-service,DNS:bank-service,DNS:localhost,IP:127.0.0.1" \
  "$ROOT/banking-service/certs/grpc/bank-server.crt" \
  "$ROOT/banking-service/certs/grpc/bank-server.key"

echo "==> Distributing transport CA cert to verifiers"
distribute() { mkdir -p "$(dirname "$1")"; cp "$OUT/grpc-ca.crt" "$1"; echo "    -> $1"; }
distribute "$ROOT/api-gateway/certs/grpc-ca.crt"
distribute "$ROOT/kdc-service/certs/grpc-ca.crt"
distribute "$ROOT/kdc-service/certs/grpc/ca-server-ca.crt"
distribute "$ROOT/banking-service/certs/grpc-ca.crt"

echo
echo "Done. Certs valid for $DAYS days. CA private key kept only in $OUT/grpc-ca.key."
echo "Set the gateway CA_CERT_PATH to api-gateway/certs/grpc-ca.crt (or your deploy path)."
