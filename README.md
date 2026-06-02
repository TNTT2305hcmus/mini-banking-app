# Mini-Banking-App

Mini-Banking-App là đồ án mô phỏng hệ thống ngân hàng số tập trung vào xác thực an toàn, quản lý chứng chỉ và giao dịch có chữ ký số. Dự án kết hợp PKI/X.509, Kerberos-like ticket, session key, replay protection, access control và hash chaining để minh họa một luồng giao dịch có tính bảo mật cao.

## Mục tiêu chính

- Xác thực khách hàng nhiều lớp bằng OTP, PKI/X.509 và ticket kiểu Kerberos.
- Đảm bảo private key của khách hàng được sinh và sử dụng ở trình duyệt theo hướng Zero-Knowledge.
- Chống replay attack bằng nonce, timestamp, request id và Redis replay cache.
- Chống chối bỏ giao dịch bằng chữ ký số trên payload giao dịch.
- Kiểm soát quyền bằng scope trong `Ticket_v` và authorization tại Bank Service.
- Quản trị PKI qua Admin Dashboard: tra cứu, xem trạng thái và revoke chứng chỉ X.509.
- Bảo vệ lịch sử giao dịch bằng immutable ledger và hash chaining.

## Kiến trúc tổng quan

Hệ thống dùng Layered Service Architecture:

| Layer | Thành phần |
|---|---|
| Client | Customer Web App, Admin Web App Dashboard |
| Gateway / DMZ | API Gateway |
| Internal Services | CA Service, KDC Service, Bank Service |
| Data Stores | CA PostgreSQL DB, Bank PostgreSQL DB, Redis |
| External | Email/OTP Provider |

Các service nội bộ giao tiếp bằng `gRPC + mTLS/Auth`. Client gọi API Gateway qua HTTPS/REST.

## Công nghệ

| Thành phần | Công nghệ |
|---|---|
| Customer/Admin Web App | React + TypeScript |
| API Gateway | Node.js + TypeScript |
| CA Service | Go |
| KDC Service | Go |
| Bank Service | Go |
| Database | PostgreSQL |
| Cache / Replay Store | Redis |
| Internal Contract | Protocol Buffers + gRPC |
| Client Crypto | WebCrypto API |

## Luồng chính

1. Khách hàng đăng ký bằng OTP, sinh key pair ở browser và gửi CSR.
2. CA Service cấp chứng chỉ X.509 và lưu metadata vào CA PostgreSQL DB.
3. Khách hàng thực hiện AS Exchange để lấy TGT và `K_{c,tgs}`.
4. Khách hàng thực hiện TGS Exchange để lấy `Ticket_v` và `K_{c,v}` theo scope.
5. Khách hàng ký số payload giao dịch, mã hóa request và gửi đến Bank Service.
6. Bank Service kiểm tra ticket, replay, revocation, chữ ký, authorization rồi ghi giao dịch vào ledger.
7. Admin dùng Dashboard để quản lý chứng chỉ X.509 trong PKI/CA.

## Cấu trúc repo

```text
.
+-- blueprint/
|   +-- proposal.md
|   +-- design.md
+-- mini-banking-app/
|   +-- api-gateway/
|   +-- banking-service/
|   +-- ca-service/
|   +-- db/
|   +-- frontend/
|   +-- kdc-service/
|   +-- pkg/
|   +-- proto/
+-- set_up_guide.md
+-- WORKFLOW.md
+-- CODEX.md
```

## Tài liệu

- [Project Proposal](./blueprint/proposal.md): vấn đề, mục tiêu, người dùng, phạm vi và rủi ro.
- [Technical Design](./blueprint/design.md): kiến trúc, C4 diagram, high-level diagram, key model, trust model, access control, system protection và ADR.
- [Setup Guide](./set_up_guide.md): hướng dẫn cài đặt/chạy dự án.
- [Workflow Handoff](./WORKFLOW.md): bối cảnh dự án và cách tiếp tục làm việc trong repo.

## Trạng thái

Dự án đang ở giai đoạn thiết kế blueprint và tái cấu trúc implementation. CA Service và KDC Service đã có phần hiện thực ban đầu, nhưng phạm vi/API/spec sẽ được chuẩn hóa lại theo `blueprint/design.md`.
