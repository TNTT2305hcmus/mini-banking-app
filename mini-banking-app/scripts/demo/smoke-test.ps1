<#
.SYNOPSIS
    Mini Banking Demo Smoke Test — PowerShell

.DESCRIPTION
    Kiểm tra nhanh toàn bộ critical path của demo end-to-end.
    Chạy sau khi: docker compose -f docker-compose.local.yml up --build -d

.PARAMETER GW
    Base URL API Gateway (default: http://localhost:3000)

.PARAMETER SkipSmtp
    Bỏ qua bước OTP qua email thật (khi SMTP chưa cấu hình)

.PARAMETER ComposeFile
    File docker-compose đang dùng (default: docker-compose.local.yml)

.EXAMPLE
    .\scripts\demo\smoke-test.ps1
    .\scripts\demo\smoke-test.ps1 -SkipSmtp -GW "http://localhost:3000"
#>

param(
    [string]$GW = "http://localhost:3000",
    [string]$AdminCaToken = $env:ADMIN_CA_TOKEN,
    [string]$AdminSecToken = $env:ADMIN_SEC_DEMO_TOKEN,
    [string]$AdminSecEmail = $env:ADMIN_SEC_DEMO_EMAIL,
    [string]$AdminSecPassword = $env:ADMIN_SEC_DEMO_PASSWORD,
    [string]$DemoEmail = "alice@demo.minibanking.local",
    [switch]$SkipSmtp,
    [string]$ComposeFile = "docker-compose.local.yml",
    [string]$DemoOtp = ""
)

# ─── Config & helpers ───────────────────────────────────────────────────────
$PassCount = 0
$FailCount = 0
$SkipCount = 0

function Write-Header($msg) {
    Write-Host "`n════════════════════════════════════════" -ForegroundColor Blue
    Write-Host "  $msg" -ForegroundColor Blue
    Write-Host "════════════════════════════════════════" -ForegroundColor Blue
}
function Write-Step($msg)  { Write-Host "`n▶ $msg" -ForegroundColor Yellow }
function Write-Pass($msg)  { Write-Host "  ✓ PASS: $msg" -ForegroundColor Green;  $script:PassCount++ }
function Write-Fail($msg)  { Write-Host "  ✗ FAIL: $msg" -ForegroundColor Red;    $script:FailCount++ }
function Write-Skip($msg)  { Write-Host "  ⊘ SKIP: $msg" -ForegroundColor Yellow; $script:SkipCount++ }

function New-RequestId { [System.Guid]::NewGuid().ToString() }

function Invoke-GW {
    param([string]$Method, [string]$Path, [hashtable]$Headers = @{}, [string]$Body = "")
    try {
        $uri = "$GW$Path"
        $headers = @{ "X-Request-ID" = (New-RequestId) } + $Headers
        if ($Body) {
            $headers["Content-Type"] = "application/json"
            $resp = Invoke-WebRequest -Method $Method -Uri $uri -Headers $headers -Body $Body -UseBasicParsing -TimeoutSec 10 -ErrorAction Stop
        } else {
            $resp = Invoke-WebRequest -Method $Method -Uri $uri -Headers $headers -UseBasicParsing -TimeoutSec 10 -ErrorAction Stop
        }
        return @{ Status = [int]$resp.StatusCode; Body = $resp.Content }
    } catch {
        $statusCode = 0
        if ($_.Exception.Response) { $statusCode = [int]$_.Exception.Response.StatusCode }
        $errBody = ""
        try { $errBody = $_.ErrorDetails.Message } catch {}
        return @{ Status = $statusCode; Body = $errBody }
    }
}

function Test-Port {
    param([int]$Port, [string]$Name)
    Write-Step "Port $Port ($Name)"
    try {
        $tcp = New-Object System.Net.Sockets.TcpClient
        $result = $tcp.BeginConnect("127.0.0.1", $Port, $null, $null)
        $success = $result.AsyncWaitHandle.WaitOne(3000, $false)
        $tcp.Close()
        if ($success) { Write-Pass "Port $Port ($Name) đang lắng nghe" }
        else          { Write-Fail "Port $Port ($Name) không phản hồi — service có thể chưa khởi động" }
    } catch { Write-Fail "Port $Port ($Name) lỗi kết nối: $_" }
}

# ─── Bước 1: Docker daemon ─────────────────────────────────────────────────
Write-Header "Bước 1: Kiểm tra Docker daemon"
Write-Step "docker info"
try {
    $dockerInfo = docker info 2>&1
    if ($LASTEXITCODE -eq 0) { Write-Pass "Docker daemon đang chạy" }
    else { Write-Fail "Docker daemon không chạy — hãy khởi động Docker Desktop"; exit 1 }
} catch { Write-Fail "Lệnh 'docker' không tìm thấy — cài Docker Desktop trước"; exit 1 }

# ─── Bước 2: Port listening ────────────────────────────────────────────────
Write-Header "Bước 2: Kiểm tra port đang lắng nghe"
Test-Port 3000  "api-gateway"
Test-Port 50051 "ca-service gRPC"
Test-Port 50052 "kdc-service gRPC"
Test-Port 50053 "banking-service gRPC"
Test-Port 6379  "Redis (gateway-redis)"
Test-Port 5432  "Bank Postgres"

# ─── Bước 3: Redis PING ────────────────────────────────────────────────────
Write-Header "Bước 3: Redis PING"
Write-Step "redis-cli PING qua Docker"
try {
    $redisPong = docker compose -f $ComposeFile exec -T gateway-redis redis-cli ping 2>&1
    if ($redisPong -match "PONG") { Write-Pass "Redis trả PONG" }
    else { Write-Fail "Redis không trả PONG (got: $redisPong)" }
} catch { Write-Fail "Không thể exec vào gateway-redis: $_" }

Write-Step "Clear Redis (FLUSHDB db0) trước khi chạy test"
try {
    $flushResult = docker compose -f $ComposeFile exec -T gateway-redis redis-cli flushdb 2>&1
    if ($flushResult -match "OK") { Write-Pass "Redis FLUSHDB OK" }
    else { Write-Fail "Redis FLUSHDB thất bại (got: $flushResult)" }
} catch { Write-Fail "Không thể FLUSHDB Redis: $_" }

# ─── Bước 4: API Gateway health check ─────────────────────────────────────
Write-Header "Bước 4: API Gateway health check"
Write-Step "HTTP connect tới $GW"
try {
    $gwResp = Invoke-WebRequest -Uri "$GW/" -UseBasicParsing -TimeoutSec 5 -ErrorAction SilentlyContinue
    Write-Pass "API Gateway phản hồi HTTP (status: $($gwResp.StatusCode))"
} catch {
    if ($_.Exception.Response) {
        Write-Pass "API Gateway phản hồi HTTP (status: $([int]$_.Exception.Response.StatusCode))"
    } else {
        Write-Fail "API Gateway không phản hồi tại $GW"
    }
}

# ─── Bước 5: OTP request ──────────────────────────────────────────────────
Write-Header "Bước 5: OTP request"

if ($SkipSmtp) {
    Write-Skip "SkipSmtp=true — bỏ qua bước OTP (SMTP chưa cấu hình hoặc đang bypass)"
    Write-Host "  → Để test flow thật: bỏ -SkipSmtp và đặt SMTP_USER/SMTP_PASS trong .env" -ForegroundColor Gray
} else {
    $smtpUser = $env:SMTP_USER
    $smtpPass = $env:SMTP_PASS
    if (-not $smtpUser -or -not $smtpPass) {
        Write-Skip "SMTP_USER/SMTP_PASS chưa set — bỏ qua OTP test"
        Write-Host "  → Set `$env:SMTP_USER và `$env:SMTP_PASS rồi chạy lại" -ForegroundColor Gray
    } else {
        Write-Step "POST $GW/v1/otp/request"
        $otpBody = "{`"email`":`"$DemoEmail`"}"
        $otpResp = Invoke-GW -Method POST -Path "/v1/otp/request" -Body $otpBody
        if ($otpResp.Status -eq 200) {
            Write-Pass "OTP request OK (200) — email đã gửi tới $DemoEmail"
        } else {
            Write-Fail "OTP request thất bại (HTTP $($otpResp.Status)): $($otpResp.Body)"
        }
    }
}

# ─── Bước 6: PKI register ─────────────────────────────────────────────────
Write-Header "Bước 6: PKI register flow"
Write-Host "  ℹ️  Flow PKI register yêu cầu OTP verify → CSR → POST /v1/pki/register" -ForegroundColor Gray
Write-Host "     Xem scripts/demo/README.md → Mục 'Bypass OTP' để biết cách demo" -ForegroundColor Gray

if ($DemoOtp -and -not $SkipSmtp) {
    Write-Step "OTP verify với DemoOtp=$DemoOtp"
    $verifyBody = "{`"email`":`"$DemoEmail`",`"otp`":`"$DemoOtp`"}"
    $verifyResp = Invoke-GW -Method POST -Path "/v1/otp/verify" -Body $verifyBody
    if ($verifyResp.Status -eq 200) {
        $verifyData = $verifyResp.Body | ConvertFrom-Json
        $regToken = $verifyData.data.registration_token
        if ($regToken) {
            Write-Pass "OTP verify OK — registration_token nhận được"
            $script:RegistrationToken = $regToken
        } else {
            Write-Fail "OTP verify OK nhưng không lấy được registration_token"
        }
    } else {
        Write-Fail "OTP verify thất bại (HTTP $($verifyResp.Status)): $($verifyResp.Body)"
    }
} else {
    Write-Skip "Bỏ qua tự động PKI register. Set -DemoOtp <otp> để tự động verify."
}

# ─── Bước 7: Admin CA auth ────────────────────────────────────────────────
Write-Header "Bước 7: Admin CA auth → AS/TGS note"
Write-Host "  ℹ️  AS/TGS/Bank flows yêu cầu cert từ PKI register + private key" -ForegroundColor Gray
Write-Host "     Xem scripts/demo/README.md → Mục 'Chạy flow đầy đủ'" -ForegroundColor Gray

Write-Step "Admin CA cert-backed session token"
if ($AdminCaToken) {
    Write-Pass "ADMIN_CA_TOKEN đã được cung cấp"
} else {
    Write-Skip "Bỏ qua Admin CA API auto-test. Đăng nhập /admin-ca bằng cert/PIN rồi truyền -AdminCaToken nếu cần test API."
}

# ─── Bước 8: Admin CA endpoints ───────────────────────────────────────────
Write-Header "Bước 8: Admin CA — list certificates, detail"

if ($AdminCaToken) {
    Write-Step "GET $GW/v1/admin-ca/certificates (list)"
    $listResp = Invoke-GW -Method GET -Path "/v1/admin-ca/certificates?limit=5" `
        -Headers @{ "Authorization" = "Bearer $AdminCaToken" }
    if ($listResp.Status -eq 200) {
        Write-Pass "Admin CA list certificates OK (200)"
        try {
            $listData = $listResp.Body | ConvertFrom-Json
            $firstSerial = $listData.data.items[0].serial
        } catch { $firstSerial = "" }
    } else {
        Write-Fail "Admin CA list certificates thất bại (HTTP $($listResp.Status)): $($listResp.Body)"
        $firstSerial = ""
    }

    if ($firstSerial) {
        Write-Step "GET $GW/v1/admin-ca/certificates/$firstSerial (detail)"
        $detailResp = Invoke-GW -Method GET -Path "/v1/admin-ca/certificates/$firstSerial" `
            -Headers @{ "Authorization" = "Bearer $AdminCaToken" }
        if ($detailResp.Status -eq 200) {
            Write-Pass "Admin CA detail certificate OK (200) serial=$firstSerial"
        } else {
            Write-Fail "Admin CA detail certificate thất bại (HTTP $($detailResp.Status))"
        }
    } else {
        Write-Skip "Không có certificate nào để test detail (CA DB có thể trống)"
    }

    Write-Step "Negative: token Admin CA sai role / sai token"
    $negResp = Invoke-GW -Method GET -Path "/v1/admin-ca/certificates?limit=1" `
        -Headers @{ "Authorization" = "Bearer INVALID_TOKEN_12345" }
    if ($negResp.Status -eq 401 -or $negResp.Status -eq 403) {
        Write-Pass "Token sai → HTTP $($negResp.Status) (đúng spec)"
    } else {
        Write-Fail "Token sai → HTTP $($negResp.Status) (unexpected — nên là 401/403)"
    }
} else {
    Write-Skip "Bỏ qua Admin CA tests (không có token)"
}

# ─── Bước 9: Admin Bank ────────────────────────────────────────────────────
Write-Header "Bước 9: Admin Bank — negative test + hướng dẫn session"
Write-Host "  ℹ️  Admin Bank dùng session cookie (activate → session workflow)" -ForegroundColor Gray
Write-Host "  → Xem scripts/demo/README.md mục 5 'Admin Bank session' để chạy flow đầy đủ" -ForegroundColor Gray
Write-Host ""

# Negative test tự động: gọi endpoint không có session cookie → phải trả 401/403
Write-Step "Negative: POST /v1/admin/bank/audit/query không có session cookie"
$bankUnauthResp = Invoke-GW -Method POST -Path "/v1/admin/bank/audit/query" `
    -Body '{"limit":5}'
if ($bankUnauthResp.Status -eq 401 -or $bankUnauthResp.Status -eq 403) {
    Write-Pass "Admin Bank audit/query không có session → HTTP $($bankUnauthResp.Status) (đúng — yêu cầu auth)"
} else {
    Write-Fail "Admin Bank audit/query không có session → HTTP $($bankUnauthResp.Status) (unexpected — nên là 401/403)"
}

Write-Skip "Admin Bank session flow (activate → session → query) — kiểm tra thủ công theo README §5"

# ─── Bước 10: SOC / KDC audit ─────────────────────────────────────────────
Write-Header "Bước 10: SOC — KDC audit, verify, summary, export"

if (-not $AdminSecToken -and $AdminSecEmail -and $AdminSecPassword) {
    Write-Step "POST /v1/admin-sec/auth để lấy security-admin token"
    $secBody = "{`"email`":`"$AdminSecEmail`",`"password`":`"$AdminSecPassword`"}"
    $secResp = Invoke-GW -Method POST -Path "/v1/admin-sec/auth" -Body $secBody
    if ($secResp.Status -eq 200) {
        try {
            $secData = $secResp.Body | ConvertFrom-Json
            $AdminSecToken = $secData.data.token
        } catch { $AdminSecToken = "" }
        if ($AdminSecToken) {
            Write-Pass "SOC login OK — token security-admin nhận được"
        } else {
            Write-Fail "SOC login OK nhưng không lấy được token"
        }
    } else {
        Write-Fail "SOC login thất bại (HTTP $($secResp.Status)): $($secResp.Body)"
    }
}

if ($AdminSecToken) {
    $secHeaders = @{ "Authorization" = "Bearer $AdminSecToken" }

    Write-Step "GET /v1/admin-kdc/audit?limit=5"
    $kdcAuditResp = Invoke-GW -Method GET -Path "/v1/admin-kdc/audit?limit=5" -Headers $secHeaders
    if ($kdcAuditResp.Status -eq 200) {
        Write-Pass "SOC KDC audit list OK (200)"
    } else {
        Write-Fail "SOC KDC audit list thất bại (HTTP $($kdcAuditResp.Status)): $($kdcAuditResp.Body)"
    }

    Write-Step "GET /v1/admin/audit/verify"
    $verifyResp = Invoke-GW -Method GET -Path "/v1/admin/audit/verify" -Headers $secHeaders
    if ($verifyResp.Status -eq 200) {
        Write-Pass "SOC audit verify OK (200)"
    } else {
        Write-Fail "SOC audit verify thất bại (HTTP $($verifyResp.Status)): $($verifyResp.Body)"
    }

    Write-Step "GET /v1/admin/audit/summary?window=24h"
    $summaryResp = Invoke-GW -Method GET -Path "/v1/admin/audit/summary?window=24h" -Headers $secHeaders
    if ($summaryResp.Status -eq 200) {
        Write-Pass "SOC audit summary OK (200)"
    } else {
        Write-Fail "SOC audit summary thất bại (HTTP $($summaryResp.Status)): $($summaryResp.Body)"
    }

    Write-Step "GET /v1/admin/audit/export?source=all&format=json"
    $exportResp = Invoke-GW -Method GET -Path "/v1/admin/audit/export?source=all&format=json" -Headers $secHeaders
    if ($exportResp.Status -eq 200) {
        Write-Pass "SOC audit export JSON OK (200)"
    } else {
        Write-Fail "SOC audit export thất bại (HTTP $($exportResp.Status)): $($exportResp.Body)"
    }

    $traceId = New-RequestId
    Write-Step "GET /v1/admin/audit/timeline?request_id=$traceId"
    $timelineResp = Invoke-GW -Method GET -Path "/v1/admin/audit/timeline?request_id=$traceId" -Headers $secHeaders
    if ($timelineResp.Status -eq 200) {
        Write-Pass "SOC timeline endpoint OK (200). Trace rỗng vẫn hợp lệ nếu chưa có flow dùng request_id này."
    } else {
        Write-Fail "SOC timeline thất bại (HTTP $($timelineResp.Status)): $($timelineResp.Body)"
    }

    Write-Step "Negative: SOC endpoint không có token"
    $socNegResp = Invoke-GW -Method GET -Path "/v1/admin-kdc/audit?limit=1"
    if ($socNegResp.Status -eq 401 -or $socNegResp.Status -eq 403) {
        Write-Pass "SOC không token → HTTP $($socNegResp.Status) (đúng — yêu cầu security-admin)"
    } else {
        Write-Fail "SOC không token → HTTP $($socNegResp.Status) (unexpected — nên là 401/403)"
    }
} else {
    Write-Skip "Bỏ qua SOC auto-test. Set ADMIN_SEC_DEMO_TOKEN hoặc ADMIN_SEC_DEMO_EMAIL/ADMIN_SEC_DEMO_PASSWORD."
}

Write-Skip "Duplicate register `409 EMAIL_ALREADY_REGISTERED` cần OTP/CSR hoặc browser flow; kiểm tra ở functional testcase/rehearsal."

# ─── Summary ───────────────────────────────────────────────────────────────
Write-Header "Kết quả Smoke Test"
Write-Host ""
Write-Host "  PASS: $PassCount" -ForegroundColor Green
Write-Host "  FAIL: $FailCount" -ForegroundColor Red
Write-Host "  SKIP: $SkipCount" -ForegroundColor Yellow
Write-Host ""

if ($FailCount -gt 0) {
    Write-Host "  ⚠️  Có $FailCount lỗi cần kiểm tra trước khi demo!" -ForegroundColor Red
    exit 1
} else {
    Write-Host "  ✅ Smoke test hoàn thành — sẵn sàng demo!" -ForegroundColor Green
    exit 0
}
