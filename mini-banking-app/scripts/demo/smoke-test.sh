#!/usr/bin/env bash
# smoke-test.sh — Mini Banking Demo Smoke Test (Bash/Linux/macOS/Git Bash)
#
# Mục đích: Kiểm tra nhanh toàn bộ critical path của demo end-to-end.
# Chạy sau khi `docker compose -f docker-compose.local.yml up --build -d`
# hoặc `docker compose -f docker-compose.demo.yml up --build -d`.
#
# Sử dụng:
#   chmod +x scripts/demo/smoke-test.sh
#   ./scripts/demo/smoke-test.sh
#
#   # Hoặc với env tùy chỉnh:
#   GW=http://localhost:3000 SKIP_SMTP_CHECK=1 ./scripts/demo/smoke-test.sh
#
# Biến môi trường:
#   GW                  — Base URL của API Gateway (default: http://localhost:3000)
#   ADMIN_CA_TOKEN      — Optional cert-backed Admin CA session token
#   ADMIN_SEC_DEMO_TOKEN — Optional SOC/security-admin token
#   ADMIN_SEC_DEMO_EMAIL / ADMIN_SEC_DEMO_PASSWORD — Optional SOC login fallback
#   DEMO_EMAIL          — Email user demo để test OTP/PKI (default: customer.demo@demo.minibanking.local)
#   SKIP_SMTP_CHECK     — Set '1' để bỏ qua bước OTP qua email thật
#   COMPOSE_FILE        — File compose đang dùng (default: docker-compose.local.yml)

set -euo pipefail

# ─── Config ────────────────────────────────────────────────────────────────
GW="${GW:-http://localhost:3000}"
ADMIN_CA_TOKEN="${ADMIN_CA_TOKEN:-}"
ADMIN_SEC_DEMO_TOKEN="${ADMIN_SEC_DEMO_TOKEN:-}"
ADMIN_SEC_DEMO_EMAIL="${ADMIN_SEC_DEMO_EMAIL:-}"
ADMIN_SEC_DEMO_PASSWORD="${ADMIN_SEC_DEMO_PASSWORD:-}"
DEMO_EMAIL="${DEMO_EMAIL:-customer.demo@demo.minibanking.local}"
SKIP_SMTP_CHECK="${SKIP_SMTP_CHECK:-0}"
COMPOSE_FILE="${COMPOSE_FILE:-docker-compose.local.yml}"

# Màu sắc
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

PASS_COUNT=0
FAIL_COUNT=0
SKIP_COUNT=0

# ─── Helper functions ───────────────────────────────────────────────────────
log_header() { echo -e "\n${BLUE}════════════════════════════════════════${NC}"; echo -e "${BLUE}  $1${NC}"; echo -e "${BLUE}════════════════════════════════════════${NC}"; }
log_step()   { echo -e "\n${YELLOW}▶ $1${NC}"; }
pass()       { echo -e "  ${GREEN}✓ PASS${NC}: $1"; ((PASS_COUNT+=1)); }
fail()       { echo -e "  ${RED}✗ FAIL${NC}: $1"; ((FAIL_COUNT+=1)); }
skip()       { echo -e "  ${YELLOW}⊘ SKIP${NC}: $1"; ((SKIP_COUNT+=1)); }

RID() { python3 -c "import uuid; print(uuid.uuid4())" 2>/dev/null || cat /proc/sys/kernel/random/uuid 2>/dev/null || echo "$(date +%s)-smoke-test"; }

check_cmd() { command -v "$1" &>/dev/null; }

# ─── Bước 1: Docker daemon ─────────────────────────────────────────────────
log_header "Bước 1: Kiểm tra Docker daemon"
log_step "docker info"
if docker info &>/dev/null; then
    pass "Docker daemon đang chạy"
else
    fail "Docker daemon không chạy — hãy khởi động Docker Desktop hoặc dockerd"
    exit 1
fi

# ─── Bước 2: Port listening ────────────────────────────────────────────────
log_header "Bước 2: Kiểm tra port đang lắng nghe"

check_port() {
    local port=$1 name=$2
    log_step "Port $port ($name)"
    if nc -z 127.0.0.1 "$port" 2>/dev/null; then
        pass "Port $port ($name) đang lắng nghe"
    else
        fail "Port $port ($name) không phản hồi — service có thể chưa khởi động"
    fi
}

check_port 3000  "api-gateway"
check_port 50051 "ca-service gRPC"
check_port 50052 "kdc-service gRPC"
check_port 50053 "banking-service gRPC"
check_port 6379  "Redis (gateway-redis)"
check_port 5432  "Bank Postgres"

# ─── Bước 3: Redis PING ────────────────────────────────────────────────────
log_header "Bước 3: Redis PING"
log_step "redis-cli PING qua Docker"
REDIS_PONG=$(docker compose -f "$COMPOSE_FILE" exec -T gateway-redis redis-cli ping 2>/dev/null || echo "FAIL")
if [[ "$REDIS_PONG" == *"PONG"* ]]; then
    pass "Redis trả PONG"
else
    fail "Redis không trả PONG (got: $REDIS_PONG)"
fi

# FLUSHDB Redis để tránh replay/idempotency cache cũ
log_step "Clear Redis (FLUSHDB db0) trước khi chạy test"
FLUSH_RESULT=$(docker compose -f "$COMPOSE_FILE" exec -T gateway-redis redis-cli flushdb 2>/dev/null || echo "FAIL")
if [[ "$FLUSH_RESULT" == *"OK"* ]]; then
    pass "Redis FLUSHDB OK"
else
    fail "Redis FLUSHDB thất bại (got: $FLUSH_RESULT)"
fi

# ─── Bước 4: API Gateway health check ─────────────────────────────────────
log_header "Bước 4: API Gateway health check"
log_step "TCP connect tới $GW"
GW_STATUS=$(curl -s -o /dev/null -w "%{http_code}" --connect-timeout 5 "$GW/" 2>/dev/null || echo "000")
if [[ "$GW_STATUS" != "000" ]]; then
    pass "API Gateway phản hồi HTTP (status: $GW_STATUS)"
else
    fail "API Gateway không phản hồi tại $GW"
fi

# ─── Bước 5: OTP request ──────────────────────────────────────────────────
log_header "Bước 5: OTP request"

if [[ "$SKIP_SMTP_CHECK" == "1" ]]; then
    skip "SKIP_SMTP_CHECK=1 — bỏ qua bước OTP (SMTP chưa cấu hình hoặc đang bypass)"
    echo "  → Để test flow thật: set SMTP_USER và SMTP_PASS trong .env, bỏ SKIP_SMTP_CHECK"
else
    # Kiểm tra SMTP có cấu hình không
    SMTP_USER_VAL="${SMTP_USER:-}"
    SMTP_PASS_VAL="${SMTP_PASS:-}"
    if [[ -z "$SMTP_USER_VAL" || -z "$SMTP_PASS_VAL" ]]; then
        skip "SMTP_USER/SMTP_PASS chưa set trong môi trường — bỏ qua OTP test"
        echo "  → Export SMTP_USER và SMTP_PASS rồi chạy lại để test OTP"
    else
        log_step "POST $GW/v1/otp/request"
        OTP_RESP=$(curl -s -w "\n%{http_code}" -X POST \
            -H "Content-Type: application/json" \
            -H "X-Request-ID: $(RID)" \
            -d "{\"email\":\"$DEMO_EMAIL\"}" \
            "$GW/v1/otp/request" 2>/dev/null)
        OTP_STATUS=$(echo "$OTP_RESP" | tail -1)
        OTP_BODY=$(echo "$OTP_RESP" | head -1)
        if [[ "$OTP_STATUS" == "200" ]]; then
            pass "OTP request OK (200) — email đã gửi tới $DEMO_EMAIL"
            echo "  → Kiểm tra hộp thư của $DEMO_EMAIL để lấy OTP"
        else
            fail "OTP request thất bại (HTTP $OTP_STATUS): $OTP_BODY"
        fi
    fi
fi

# ─── Bước 6: PKI register ─────────────────────────────────────────────────
log_header "Bước 6: PKI register flow"
echo "  ℹ️  Flow PKI register yêu cầu:"
echo "     1. OTP verify → nhận registration_token"
echo "     2. Tạo RSA key pair và CSR"
echo "     3. POST /v1/pki/register với CSR + registration_token"
echo ""
echo "  Bypass OTP cho demo: nếu có biến DEMO_OTP (dev mode), script sẽ verify tự động."
echo "  Xem docs/testcases.md TC-U-03 và scripts/demo/README.md → Mục 'Bypass OTP'."
echo ""

DEMO_OTP="${DEMO_OTP:-}"
if [[ -n "$DEMO_OTP" && "$SKIP_SMTP_CHECK" != "1" ]]; then
    log_step "OTP verify với DEMO_OTP=$DEMO_OTP"
    VERIFY_RESP=$(curl -s -w "\n%{http_code}" -X POST \
        -H "Content-Type: application/json" \
        -H "X-Request-ID: $(RID)" \
        -d "{\"email\":\"$DEMO_EMAIL\",\"otp\":\"$DEMO_OTP\"}" \
        "$GW/v1/otp/verify" 2>/dev/null)
    VERIFY_STATUS=$(echo "$VERIFY_RESP" | tail -1)
    VERIFY_BODY=$(echo "$VERIFY_RESP" | head -1)
    if [[ "$VERIFY_STATUS" == "200" ]]; then
        REGISTRATION_TOKEN=$(echo "$VERIFY_BODY" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['registration_token'])" 2>/dev/null || echo "")
        if [[ -n "$REGISTRATION_TOKEN" ]]; then
            pass "OTP verify OK — registration_token nhận được"
        else
            fail "OTP verify OK nhưng không lấy được registration_token"
        fi
    else
        fail "OTP verify thất bại (HTTP $VERIFY_STATUS): $VERIFY_BODY"
    fi
else
    skip "Bỏ qua tự động PKI register (cần OTP thủ công). Set DEMO_OTP=<otp> để tự động."
fi

# ─── Bước 7: AS → TGS → Bank flows ───────────────────────────────────────
log_header "Bước 7: AS/TGS/Bank flows (dùng customer demo đã đăng ký)"
echo "  ℹ️  Các flow AS/TGS/Bank yêu cầu:"
echo "     - customer.demo đã có cert từ PKI register (hoặc chạy flow thật trước)"
echo "     - Private key của customer.demo để ký AS_REQ"
echo "  → Xem scripts/demo/README.md → Mục 'Chạy flow đầy đủ' để biết cách test"
echo "  → Hoặc dùng Frontend UI tại http://localhost:5173"
echo ""

log_step "Admin CA cert-backed session token"
if [[ -n "$ADMIN_CA_TOKEN" ]]; then
    pass "ADMIN_CA_TOKEN đã được cung cấp"
else
    skip "Bỏ qua Admin CA API auto-test. Đăng nhập /admin-ca bằng cert/PIN rồi export ADMIN_CA_TOKEN nếu cần test API."
fi

# ─── Bước 8: Admin CA endpoints ───────────────────────────────────────────
log_header "Bước 8: Admin CA — list certificates, detail"

if [[ -n "${ADMIN_CA_TOKEN:-}" ]]; then
    log_step "GET $GW/v1/admin-ca/certificates (list)"
    LIST_RESP=$(curl -s -w "\n%{http_code}" \
        -H "Authorization: Bearer $ADMIN_CA_TOKEN" \
        -H "X-Request-ID: $(RID)" \
        "$GW/v1/admin-ca/certificates?limit=5" 2>/dev/null)
    LIST_STATUS=$(echo "$LIST_RESP" | tail -1)
    LIST_BODY=$(echo "$LIST_RESP" | head -1)
    if [[ "$LIST_STATUS" == "200" ]]; then
        pass "Admin CA list certificates OK (200)"
        # Thử lấy serial đầu tiên để test detail
        FIRST_SERIAL=$(echo "$LIST_BODY" | python3 -c "import sys,json; items=json.load(sys.stdin).get('data',{}).get('items',[]); print(items[0]['serial'] if items else '')" 2>/dev/null || echo "")
    else
        fail "Admin CA list certificates thất bại (HTTP $LIST_STATUS): $LIST_BODY"
        FIRST_SERIAL=""
    fi

    if [[ -n "$FIRST_SERIAL" ]]; then
        log_step "GET $GW/v1/admin-ca/certificates/$FIRST_SERIAL (detail)"
        DETAIL_RESP=$(curl -s -w "\n%{http_code}" \
            -H "Authorization: Bearer $ADMIN_CA_TOKEN" \
            -H "X-Request-ID: $(RID)" \
            "$GW/v1/admin-ca/certificates/$FIRST_SERIAL" 2>/dev/null)
        DETAIL_STATUS=$(echo "$DETAIL_RESP" | tail -1)
        if [[ "$DETAIL_STATUS" == "200" ]]; then
            pass "Admin CA detail certificate OK (200) serial=$FIRST_SERIAL"
        else
            fail "Admin CA detail certificate thất bại (HTTP $DETAIL_STATUS)"
        fi
    else
        skip "Không có certificate nào để test detail (CA DB có thể trống)"
    fi

    log_step "Negative: thiếu X-Request-ID (theo spec phải 400 hoặc auto-generate)"
    NEG_RESP=$(curl -s -w "\n%{http_code}" \
        -H "Authorization: Bearer $ADMIN_CA_TOKEN" \
        "$GW/v1/admin-ca/certificates?limit=1" 2>/dev/null)
    NEG_STATUS=$(echo "$NEG_RESP" | tail -1)
    if [[ "$NEG_STATUS" == "400" ]]; then
        pass "Thiếu X-Request-ID → 400 (đúng spec)"
    elif [[ "$NEG_STATUS" == "200" ]]; then
        pass "Thiếu X-Request-ID → 200 (Gateway auto-generate X-Request-ID — cũng chấp nhận được)"
    else
        fail "Thiếu X-Request-ID → HTTP $NEG_STATUS (unexpected)"
    fi
else
    skip "Bỏ qua Admin CA tests (không có token)"
fi

# ─── Bước 9: Admin Bank endpoints ─────────────────────────────────────────
log_header "Bước 9: Admin Bank — negative test + hướng dẫn session"

echo "  ℹ️  Admin Bank dùng session cookie — cần activate + session workflow."
echo "  → Xem scripts/demo/README.md mục 5 'Admin Bank session' để chạy flow đầy đủ."
echo ""

# Negative test tự động: gọi endpoint không có session cookie → phải trả 401/403
log_step "Negative: POST /v1/admin/bank/audit/query không có session cookie"
BANK_UNAUTH_RESP=$(curl -s -w "\n%{http_code}" -X POST \
    -H "Content-Type: application/json" \
    -H "X-Request-ID: $(RID)" \
    -d '{"limit":5}' \
    "$GW/v1/admin/bank/audit/query" 2>/dev/null)
BANK_UNAUTH_STATUS=$(echo "$BANK_UNAUTH_RESP" | tail -1)
if [[ "$BANK_UNAUTH_STATUS" == "401" || "$BANK_UNAUTH_STATUS" == "403" ]]; then
    pass "Admin Bank audit/query không có session → HTTP $BANK_UNAUTH_STATUS (đúng — yêu cầu auth)"
else
    fail "Admin Bank audit/query không có session → HTTP $BANK_UNAUTH_STATUS (unexpected — nên là 401/403)"
fi

skip "Admin Bank session flow (activate → session → query) — kiểm tra thủ công theo README §5"

# ─── Bước 10: SOC / KDC audit ─────────────────────────────────────────────
log_header "Bước 10: SOC — KDC audit, verify, summary, export"

ADMIN_SEC_TOKEN="$ADMIN_SEC_DEMO_TOKEN"
if [[ -z "$ADMIN_SEC_TOKEN" && -n "$ADMIN_SEC_DEMO_EMAIL" && -n "$ADMIN_SEC_DEMO_PASSWORD" ]]; then
    log_step "POST /v1/admin-sec/auth để lấy security-admin token"
    SEC_RESP=$(curl -s -w "\n%{http_code}" -X POST \
        -H "Content-Type: application/json" \
        -H "X-Request-ID: $(RID)" \
        -d "{\"email\":\"$ADMIN_SEC_DEMO_EMAIL\",\"password\":\"$ADMIN_SEC_DEMO_PASSWORD\"}" \
        "$GW/v1/admin-sec/auth" 2>/dev/null)
    SEC_STATUS=$(echo "$SEC_RESP" | tail -1)
    SEC_BODY=$(echo "$SEC_RESP" | head -1)
    if [[ "$SEC_STATUS" == "200" ]]; then
        ADMIN_SEC_TOKEN=$(echo "$SEC_BODY" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['token'])" 2>/dev/null || echo "")
        if [[ -n "$ADMIN_SEC_TOKEN" ]]; then
            pass "SOC login OK — token security-admin nhận được"
        else
            fail "SOC login OK nhưng không lấy được token"
        fi
    else
        fail "SOC login thất bại (HTTP $SEC_STATUS): $SEC_BODY"
    fi
fi

if [[ -n "$ADMIN_SEC_TOKEN" ]]; then
    log_step "GET /v1/admin-kdc/audit?limit=5"
    KDC_AUDIT_RESP=$(curl -s -w "\n%{http_code}" \
        -H "Authorization: Bearer $ADMIN_SEC_TOKEN" \
        -H "X-Request-ID: $(RID)" \
        "$GW/v1/admin-kdc/audit?limit=5" 2>/dev/null)
    KDC_AUDIT_STATUS=$(echo "$KDC_AUDIT_RESP" | tail -1)
    KDC_AUDIT_BODY=$(echo "$KDC_AUDIT_RESP" | head -1)
    if [[ "$KDC_AUDIT_STATUS" == "200" ]]; then
        pass "SOC KDC audit list OK (200)"
    else
        fail "SOC KDC audit list thất bại (HTTP $KDC_AUDIT_STATUS): $KDC_AUDIT_BODY"
    fi

    log_step "GET /v1/admin/audit/verify"
    VERIFY_SOC_RESP=$(curl -s -w "\n%{http_code}" \
        -H "Authorization: Bearer $ADMIN_SEC_TOKEN" \
        -H "X-Request-ID: $(RID)" \
        "$GW/v1/admin/audit/verify" 2>/dev/null)
    VERIFY_SOC_STATUS=$(echo "$VERIFY_SOC_RESP" | tail -1)
    VERIFY_SOC_BODY=$(echo "$VERIFY_SOC_RESP" | head -1)
    if [[ "$VERIFY_SOC_STATUS" == "200" ]]; then
        pass "SOC audit verify OK (200)"
    else
        fail "SOC audit verify thất bại (HTTP $VERIFY_SOC_STATUS): $VERIFY_SOC_BODY"
    fi

    log_step "GET /v1/admin/audit/summary?window=24h"
    SUMMARY_RESP=$(curl -s -w "\n%{http_code}" \
        -H "Authorization: Bearer $ADMIN_SEC_TOKEN" \
        -H "X-Request-ID: $(RID)" \
        "$GW/v1/admin/audit/summary?window=24h" 2>/dev/null)
    SUMMARY_STATUS=$(echo "$SUMMARY_RESP" | tail -1)
    SUMMARY_BODY=$(echo "$SUMMARY_RESP" | head -1)
    if [[ "$SUMMARY_STATUS" == "200" ]]; then
        pass "SOC audit summary OK (200)"
    else
        fail "SOC audit summary thất bại (HTTP $SUMMARY_STATUS): $SUMMARY_BODY"
    fi

    log_step "GET /v1/admin/audit/export?source=all&format=json"
    EXPORT_RESP=$(curl -s -w "\n%{http_code}" \
        -H "Authorization: Bearer $ADMIN_SEC_TOKEN" \
        -H "X-Request-ID: $(RID)" \
        "$GW/v1/admin/audit/export?source=all&format=json" 2>/dev/null)
    EXPORT_STATUS=$(echo "$EXPORT_RESP" | tail -1)
    EXPORT_BODY=$(echo "$EXPORT_RESP" | head -1)
    if [[ "$EXPORT_STATUS" == "200" ]]; then
        pass "SOC audit export JSON OK (200)"
    else
        fail "SOC audit export thất bại (HTTP $EXPORT_STATUS): $EXPORT_BODY"
    fi

    TRACE_ID="$(RID)"
    log_step "GET /v1/admin/audit/timeline?request_id=$TRACE_ID"
    TIMELINE_RESP=$(curl -s -w "\n%{http_code}" \
        -H "Authorization: Bearer $ADMIN_SEC_TOKEN" \
        -H "X-Request-ID: $(RID)" \
        "$GW/v1/admin/audit/timeline?request_id=$TRACE_ID" 2>/dev/null)
    TIMELINE_STATUS=$(echo "$TIMELINE_RESP" | tail -1)
    TIMELINE_BODY=$(echo "$TIMELINE_RESP" | head -1)
    if [[ "$TIMELINE_STATUS" == "200" ]]; then
        pass "SOC timeline endpoint OK (200). Trace rỗng vẫn hợp lệ nếu chưa có flow dùng request_id này."
    else
        fail "SOC timeline thất bại (HTTP $TIMELINE_STATUS): $TIMELINE_BODY"
    fi

    log_step "Negative: SOC endpoint không có token"
    SOC_NEG_RESP=$(curl -s -w "\n%{http_code}" \
        -H "X-Request-ID: $(RID)" \
        "$GW/v1/admin-kdc/audit?limit=1" 2>/dev/null)
    SOC_NEG_STATUS=$(echo "$SOC_NEG_RESP" | tail -1)
    if [[ "$SOC_NEG_STATUS" == "401" || "$SOC_NEG_STATUS" == "403" ]]; then
        pass "SOC không token → HTTP $SOC_NEG_STATUS (đúng — yêu cầu security-admin)"
    else
        fail "SOC không token → HTTP $SOC_NEG_STATUS (unexpected — nên là 401/403)"
    fi
else
    skip "Bỏ qua SOC auto-test. Set ADMIN_SEC_DEMO_TOKEN hoặc ADMIN_SEC_DEMO_EMAIL/ADMIN_SEC_DEMO_PASSWORD."
fi

skip "Duplicate register 409 EMAIL_ALREADY_REGISTERED cần OTP/CSR hoặc browser flow; kiểm tra ở functional testcase/rehearsal."


# ─── Summary ───────────────────────────────────────────────────────────────
log_header "Kết quả Smoke Test"
echo ""
echo -e "  ${GREEN}PASS: $PASS_COUNT${NC}"
echo -e "  ${RED}FAIL: $FAIL_COUNT${NC}"
echo -e "  ${YELLOW}SKIP: $SKIP_COUNT${NC}"
echo ""

if [[ $FAIL_COUNT -gt 0 ]]; then
    echo -e "  ${RED}⚠️  Có $FAIL_COUNT lỗi cần kiểm tra trước khi demo!${NC}"
    exit 1
else
    echo -e "  ${GREEN}✅ Smoke test hoàn thành — sẵn sàng demo!${NC}"
    exit 0
fi
