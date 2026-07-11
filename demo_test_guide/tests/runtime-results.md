# Runtime Results

## 1. Run metadata

| Field | Value |
|---|---|
| Date | July 11, 2026 |
| Runner | Antigravity AI QA Agent |
| Branch | master |
| Commit | 2bd37933805312c8d8bdc2044cdeaafc00ea8976 |
| Environment | Local Docker Compose |
| Compose file | docker-compose.local.yml |
| `.env` source | `.env.demo.example` copied to `.env` |
| Rate limit mode | Disabled (RATE_LIMIT_DISABLED=1) |
| SMTP mode | Mocked (console logs check) |
| Browser profile | Playwright / Chromium Browser Subagent |

## 2. Summary

| Group | Total | PASS | FAIL | SKIP | Pending | Notes |
|---|---:|---:|---:|---:|---:|---|
| Smoke | 21 | 17 | 0 | 4 | 0 | Ran via smoke-test.ps1 |
| Functional | 27 | 27 | 0 | 0 | 0 | Executed via UI Browser Subagent / API |
| Security | 26 | 25 | 0 | 0 | 1 | S-AUD-05 is PENDING_MANUAL_DB |
| Audit | 27 | 26 | 0 | 0 | 1 | Verify tampered is PENDING_MANUAL_DB |

## 3. Smoke result

| ID | Status | Evidence | Note |
|---|---|---|---|
| SMK-01..SMK-21 | PASS / SKIP | 17 PASS, 4 SKIP (SMK-06, SMK-08, SMK-21) | Updated in smoke-testcases.md |

## 4. Functional result

| ID | Status | Evidence | Note |
|---|---|---|---|
| F-CUS-01..F-SOC-05 | PASS | 27 PASS | Detailed in functional-testcases.md |

## 5. Security result

| ID | Status | Evidence | Note |
|---|---|---|---|
| S-PKI-01..S-NF-03 | PASS | 25 PASS, 1 PENDING_MANUAL_DB (S-AUD-05) | Detailed in security-testcases.md |

## 6. Audit export evidence

| Artifact | Path / Link | Note |
|---|---|---|
| Alice Dashboard Screenshot | [alice_dashboard_1783760732157.png](file:///C:/Users/ProTech247.vn/.gemini/antigravity-ide/brain/a2435e04-0e31-4292-a65b-4b5685f4b3e3/alice_dashboard_1783760732157.png) | Home page with balance |
| Transfer Success Screenshot | [transfer_success_1m_1783761105978.png](file:///C:/Users/ProTech247.vn/.gemini/antigravity-ide/brain/a2435e04-0e31-4292-a65b-4b5685f4b3e3/transfer_success_1m_1783761105978.png) | 1,000,000 VND transfer overlay |
| Transfer Failed Screenshot | [transfer_failed_insufficient_funds_1783761255317.png](file:///C:/Users/ProTech247.vn/.gemini/antigravity-ide/brain/a2435e04-0e31-4292-a65b-4b5685f4b3e3/transfer_failed_insufficient_funds_1783761255317.png) | Insufficient funds overlay |
| Admin CA Activations | [admin_activation_and_login_1783760279870.webp](file:///C:/Users/ProTech247.vn/.gemini/antigravity-ide/brain/a2435e04-0e31-4292-a65b-4b5685f4b3e3/admin_activation_and_login_1783760279870.webp) | CA / Bank Admin activation flow |

## 7. Issues found

| ID | Severity | Description | Owner | Status |
|---|---|---|---|---|
| - | - | Không phát hiện lỗi nghiêm trọng trong các luồng chính | - | - |
