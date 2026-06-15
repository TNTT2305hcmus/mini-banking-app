# Định nghĩa các khóa mật mã

---

## `K_tgs` — KDC Service Key (long-term)

**Luồng liên quan:** Phase 2 - AS Exchange  
**Vị trí trong design.md:** Key Model (dòng 89), Flow 2 (bước 8), Flow 3 (bước 5)

**Tác dụng:** Khóa bí mật dài hạn của KDC, dùng để **mã hóa TGT** khi phát hành và **giải mã TGT** khi client gửi TGS_REQ. Chỉ KDC giữ khóa này.

**Luồng tóm tắt:**
1. KDC sinh TGT sau khi xác minh AS_REQ → mã hóa TGT bằng `K_tgs` → trả cho client.
2. Client gửi TGS_REQ kèm TGT → KDC giải mã TGT bằng `K_tgs` → xác minh tính hợp lệ → cấp `Ticket_v`.

---

## `K_v` — Bank Service Key (long-term)

**Luồng liên quan:** Phase 3 - TGS Exchange, Phase 4 - AP Exchange & Transaction  
**Vị trí trong design.md:** Key Model (dòng 90), Flow 3 (bước 5, 15)

**Tác dụng:** Khóa bí mật dài hạn của Bank Service, dùng để **mã hóa `Ticket_v`** khi KDC phát hành và **giải mã `Ticket_v`** khi client gửi giao dịch. Chỉ Bank Service giữ khóa này.

**Luồng tóm tắt:**
1. KDC sinh `Ticket_v` trong TGS Exchange → mã hóa bằng `K_v` → trả cho client.
2. Client gửi `Ticket_v` đến Bank Service → Bank Service giải mã bằng `K_v` → lấy ra `K_{c,v}` và thông tin scope/identity để xác thực giao dịch.

---

## `K_{c,tgs}` — Session Key giữa Client và KDC/TGS

**Luồng liên quan:** Phase 2 - AS Exchange, Phase 3 - TGS Exchange  
**Vị trí trong design.md:** Key Model (dòng 91), Session/Subsession Key Policy (dòng 147), Flow 2 (bước 9–15)

**Tác dụng:** Khóa phiên ngắn hạn do KDC sinh trong AS Exchange, chia sẻ giữa client và KDC. Dùng để **mã hóa Authenticator trong TGS_REQ** và **giải mã TGS_REP** — chứng minh client đang giữ TGT hợp lệ mà không cần gửi lại credential gốc. TTL theo TGT (~15–30 phút).

**Luồng tóm tắt:**
1. KDC sinh `K_{c,tgs}` trong AS Exchange → nhúng vào AS_REP (mã hóa bằng public key client) → client lưu trong session memory.
2. Khi gửi TGS_REQ, client dùng `K_{c,tgs}` mã hóa Authenticator (nonce, timestamp, scope) → KDC verify Authenticator → cấp `Ticket_v`.
3. KDC mã hóa TGS_REP bằng `K_{c,tgs}` → client giải mã để lấy `Ticket_v` và `K_{c,v}`.
4. Xóa khỏi session khi TGT hết hạn hoặc logout.

---

## `K_{c,v}` — Service Session Key giữa Client và Bank Service

**Luồng liên quan:** Phase 3 - TGS Exchange, Phase 4 - AP Exchange & Transaction  
**Vị trí trong design.md:** Key Model (dòng 92), Session/Subsession Key Policy (dòng 148), Flow 3 (bước 3, 10, 14)

**Tác dụng:** Khóa phiên ngắn hạn do KDC sinh trong TGS Exchange, chia sẻ giữa client và Bank Service. Dùng để **mã hóa payload giao dịch** phía client và **xác thực AP_REP** từ Bank Service. TTL theo `Ticket_v` (~5–10 phút).

**Luồng tóm tắt:**
1. KDC sinh `K_{c,v}` trong TGS Exchange → nhúng vào `Ticket_v` (mã hóa bằng `K_v`) và trả cho client trong TGS_REP (mã hóa bằng `K_{c,tgs}`).
2. Client dùng `K_{c,v}` + AES-GCM (IV ngẫu nhiên) để mã hóa payload giao dịch → gửi kèm `Ticket_v`.
3. Bank Service giải mã `Ticket_v` bằng `K_v` → lấy `K_{c,v}` → giải mã payload → verify chữ ký.
4. Bank Service trả AP_REP mã hóa bằng `K_{c,v}` → chứng minh Bank Service thật sự giữ `K_v`.
5. Không lưu persistent; xóa khỏi session khi `Ticket_v` hết hạn.

---

## `Ticket_v` — Service Ticket cho Bank Service

**Luồng liên quan:** Phase 3 - TGS Exchange, Phase 4 - AP Exchange & Transaction  
**Vị trí trong design.md:** Key Model (dòng 92), Ticket Reuse Policy (dòng 136–141), Authorization matrix (dòng 187–189), Flow 2 (bước 12–15), Flow 3 (bước 4–5)

**Tác dụng:** Ticket được KDC cấp sau TGS Exchange, chứa `K_{c,v}`, `ID_c`, `service_id`, `scope`, `issued_at`, `expires_at`. Được mã hóa bằng `K_v` nên **chỉ Bank Service đọc được nội dung**. Đóng vai trò chứng thực danh tính client với Bank Service mà không cần client tương tác lại với KDC.

**Luồng tóm tắt:**
1. Client gửi TGS_REQ kèm TGT + Authenticator + scope → KDC verify → sinh `Ticket_v` mã hóa bằng `K_v`.
2. KDC trả TGS_REP (mã hóa bằng `K_{c,tgs}`) chứa `Ticket_v` + `K_{c,v}` → client lưu trong session memory.
3. Client gửi `Ticket_v` + Authenticator + CipherPayload đến Bank Service trong AP_REQ.
4. Bank Service giải mã `Ticket_v` → lấy `K_{c,v}`, `scope`, `ID_c` → verify replay, revocation, chữ ký, ownership, limits.
5. `Ticket_v` reusable trong TTL cho cùng scope/service; mỗi request phải có nonce/timestamp/idempotency key riêng để chống replay.
