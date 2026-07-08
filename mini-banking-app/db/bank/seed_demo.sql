-- =============================================================================
-- Mini Banking — DEMO SEED DATA (Bank DB)
-- File: db/bank/seed_demo.sql
--
-- Mục đích: Tạo dữ liệu demo sẵn cho Admin Bank (users, accounts, transactions
--           mẫu, audit log mẫu). Script idempotent: chạy lại nhiều lần không lỗi.
--
-- Lưu ý: Transactions mẫu bên dưới được INSERT trực tiếp vào DB (bypass flow
--        thật) chỉ để Admin Bank có data hiển thị khi demo. Chúng không có
--        client_signature thật (dùng placeholder 'SEED_DEMO_PLACEHOLDER').
--        Để demo transfer thật, chạy flow PKI → AS → TGS → transfer qua Gateway.
--
-- UUID cố định dùng cho seed (không random để idempotent):
--   alice  user:  a0000000-0000-0000-0000-000000000001
--   bob    user:  b0000000-0000-0000-0000-000000000001
--   charlie user: c0000000-0000-0000-0000-000000000001
--   alice  acct1: a0000000-0000-0000-0001-000000000001 (10,000,000 VND)
--   alice  acct2: a0000000-0000-0000-0001-000000000002 (5,000,000 VND)
--   bob    acct1: b0000000-0000-0000-0001-000000000001 (20,000,000 VND)
--   charlie acct1:c0000000-0000-0000-0001-000000000001 (15,000,000 VND)
-- =============================================================================

-- Đảm bảo extension uuid-ossp đã có (migration 001 đã tạo, dùng thêm ở đây cho an toàn)
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- =============================================================================
-- USERS
-- =============================================================================

INSERT INTO users (id, email, full_name, status, created_at, updated_at)
VALUES
    ('a0000000-0000-0000-0000-000000000001',
     'alice@demo.minibanking.local',
     'Nguyễn Thị Alice',
     'active',
     NOW() - INTERVAL '30 days',
     NOW() - INTERVAL '30 days'),

    ('b0000000-0000-0000-0000-000000000001',
     'bob@demo.minibanking.local',
     'Trần Văn Bob',
     'active',
     NOW() - INTERVAL '25 days',
     NOW() - INTERVAL '25 days'),

    ('c0000000-0000-0000-0000-000000000001',
     'charlie@demo.minibanking.local',
     'Lê Văn Charlie',
     'active',
     NOW() - INTERVAL '20 days',
     NOW() - INTERVAL '20 days')
ON CONFLICT (id) DO NOTHING;

-- =============================================================================
-- ACCOUNTS
-- =============================================================================

INSERT INTO accounts (
    id, user_id, account_number, balance, daily_transfer_limit,
    currency, status, created_at, updated_at
)
VALUES
    -- Alice — tài khoản chính
    ('a0000000-0000-0000-0001-000000000001',
     'a0000000-0000-0000-0000-000000000001',
     '110001000001',
     1000000000,    -- 10,000,000 VND (lưu dạng cents = VND * 100 theo bank-server.sql comment; ở đây theo convention 1 VND = 1 cent nên 10,000,000 VND = 10_000_000)
     5000000000,   -- 50,000,000 VND daily limit
     'VND', 'active',
     NOW() - INTERVAL '30 days',
     NOW() - INTERVAL '1 day'),

    -- Alice — tài khoản thứ hai (để demo history)
    ('a0000000-0000-0000-0001-000000000002',
     'a0000000-0000-0000-0000-000000000001',
     '110001000002',
     500000000,    -- 5,000,000 VND
     5000000000,
     'VND', 'active',
     NOW() - INTERVAL '29 days',
     NOW() - INTERVAL '2 days'),

    -- Bob
    ('b0000000-0000-0000-0001-000000000001',
     'b0000000-0000-0000-0000-000000000001',
     '110002000001',
     2000000000,   -- 20,000,000 VND
     5000000000,
     'VND', 'active',
     NOW() - INTERVAL '25 days',
     NOW() - INTERVAL '1 day'),

    -- Charlie
    ('c0000000-0000-0000-0001-000000000001',
     'c0000000-0000-0000-0000-000000000001',
     '110003000001',
     1500000000,   -- 15,000,000 VND
     5000000000,
     'VND', 'active',
     NOW() - INTERVAL '20 days',
     NOW() - INTERVAL '3 days')
ON CONFLICT (id) DO NOTHING;

-- =============================================================================
-- TRANSACTIONS MẪU (bypass flow thật — chỉ để Admin Bank có data hiển thị)
-- Lưu ý: client_signature = 'SEED_DEMO_PLACEHOLDER' không phải chữ ký thật.
-- =============================================================================

-- Cập nhật ledger_state genesis nếu chưa có
INSERT INTO ledger_state (id, last_hash)
VALUES ('main', 'genesis')
ON CONFLICT (id) DO NOTHING;

-- Giao dịch 1: Alice acct1 → Bob (5,000,000 VND)
INSERT INTO transactions (
    id, from_account_id, to_account_id,
    from_account_number, to_account_number,
    amount, currency, status,
    description, payload_hash, client_signature,
    cert_serial, scope, nonce, idempotency_key,
    previous_hash, current_hash,
    created_at, completed_at
)
VALUES (
    'd1000000-0000-0000-0000-000000000001',
    'a0000000-0000-0000-0001-000000000001',
    'b0000000-0000-0000-0001-000000000001',
    '110001000001', '110002000001',
    500000000, 'VND', 'completed',
    'Chuyển tiền demo Alice→Bob',
    'seed_payload_hash_demo_001_placeholder_64chars_xxxxxxxxxxxx',
    'SEED_DEMO_PLACEHOLDER',
    'SEED_CERT_SERIAL_ALICE_001',
    'transfer:create',
    'seed-nonce-demo-txn-001-alice-to-bob-placeholder',
    'seed-idem-key-demo-txn-001-alice-to-bob-placeholder',
    'genesis',
    'seed_hash_demo_001_placeholder_64chars_xxxxxxxxxxxxxxxxxxxxxx',
    NOW() - INTERVAL '10 days',
    NOW() - INTERVAL '10 days'
)
ON CONFLICT (id) DO NOTHING;

-- Giao dịch 2: Bob → Charlie (2,000,000 VND)
INSERT INTO transactions (
    id, from_account_id, to_account_id,
    from_account_number, to_account_number,
    amount, currency, status,
    description, payload_hash, client_signature,
    cert_serial, scope, nonce, idempotency_key,
    previous_hash, current_hash,
    created_at, completed_at
)
VALUES (
    'd2000000-0000-0000-0000-000000000001',
    'b0000000-0000-0000-0001-000000000001',
    'c0000000-0000-0000-0001-000000000001',
    '110002000001', '110003000001',
    200000000, 'VND', 'completed',
    'Chuyển tiền demo Bob→Charlie',
    'seed_payload_hash_demo_002_placeholder_64chars_xxxxxxxxxxxx',
    'SEED_DEMO_PLACEHOLDER',
    'SEED_CERT_SERIAL_BOB_001',
    'transfer:create',
    'seed-nonce-demo-txn-002-bob-to-charlie-placeholder',
    'seed-idem-key-demo-txn-002-bob-to-charlie-placeholder',
    'seed_hash_demo_001_placeholder_64chars_xxxxxxxxxxxxxxxxxxxxxx',
    'seed_hash_demo_002_placeholder_64chars_xxxxxxxxxxxxxxxxxxxxxx',
    NOW() - INTERVAL '7 days',
    NOW() - INTERVAL '7 days'
)
ON CONFLICT (id) DO NOTHING;

-- Giao dịch 3: Alice acct1 → Charlie (1,000,000 VND)
INSERT INTO transactions (
    id, from_account_id, to_account_id,
    from_account_number, to_account_number,
    amount, currency, status,
    description, payload_hash, client_signature,
    cert_serial, scope, nonce, idempotency_key,
    previous_hash, current_hash,
    created_at, completed_at
)
VALUES (
    'd3000000-0000-0000-0000-000000000001',
    'a0000000-0000-0000-0001-000000000001',
    'c0000000-0000-0000-0001-000000000001',
    '110001000001', '110003000001',
    100000000, 'VND', 'completed',
    'Chuyển tiền demo Alice→Charlie',
    'seed_payload_hash_demo_003_placeholder_64chars_xxxxxxxxxxxx',
    'SEED_DEMO_PLACEHOLDER',
    'SEED_CERT_SERIAL_ALICE_001',
    'transfer:create',
    'seed-nonce-demo-txn-003-alice-to-charlie-placeholder',
    'seed-idem-key-demo-txn-003-alice-to-charlie-placeholder',
    'seed_hash_demo_002_placeholder_64chars_xxxxxxxxxxxxxxxxxxxxxx',
    'seed_hash_demo_003_placeholder_64chars_xxxxxxxxxxxxxxxxxxxxxx',
    NOW() - INTERVAL '3 days',
    NOW() - INTERVAL '3 days'
)
ON CONFLICT (id) DO NOTHING;

-- Cập nhật ledger_state với hash cuối cùng (idempotent)
UPDATE ledger_state
SET last_hash           = 'seed_hash_demo_003_placeholder_64chars_xxxxxxxxxxxxxxxxxxxxxx',
    last_transaction_id = 'd3000000-0000-0000-0000-000000000001',
    updated_at          = NOW() - INTERVAL '3 days'
WHERE id = 'main'
  AND last_hash = 'genesis';  -- Chỉ cập nhật khi vẫn là genesis (chưa có tx thật)

-- =============================================================================
-- AUDIT LOG MẪU (chỉ để Admin Bank có data; seed không đại diện flow thật)
-- =============================================================================

INSERT INTO bank_audit_log (
    id, action, user_id, account_id, transaction_id,
    cert_serial, request_id, reason, metadata, created_at
)
VALUES
    -- Transfer completed
    ('e1000000-0000-0000-0000-000000000001',
     'transfer_completed',
     'a0000000-0000-0000-0000-000000000001',
     'a0000000-0000-0000-0001-000000000001',
     'd1000000-0000-0000-0000-000000000001',
     'SEED_CERT_SERIAL_ALICE_001',
     'seed-req-id-demo-001',
     NULL,
     '{"scope": "transfer:create", "note": "seed demo data"}',
     NOW() - INTERVAL '10 days'),

    ('e2000000-0000-0000-0000-000000000001',
     'transfer_completed',
     'b0000000-0000-0000-0000-000000000001',
     'b0000000-0000-0000-0001-000000000001',
     'd2000000-0000-0000-0000-000000000001',
     'SEED_CERT_SERIAL_BOB_001',
     'seed-req-id-demo-002',
     NULL,
     '{"scope": "transfer:create", "note": "seed demo data"}',
     NOW() - INTERVAL '7 days'),

    ('e3000000-0000-0000-0000-000000000001',
     'transfer_completed',
     'a0000000-0000-0000-0000-000000000001',
     'a0000000-0000-0000-0001-000000000001',
     'd3000000-0000-0000-0000-000000000001',
     'SEED_CERT_SERIAL_ALICE_001',
     'seed-req-id-demo-003',
     NULL,
     '{"scope": "transfer:create", "note": "seed demo data"}',
     NOW() - INTERVAL '3 days'),

    -- Transfer rejected (insufficient funds — demo negative case)
    ('e4000000-0000-0000-0000-000000000001',
     'insufficient_funds',
     'c0000000-0000-0000-0000-000000000001',
     'c0000000-0000-0000-0001-000000000001',
     NULL,
     'SEED_CERT_SERIAL_CHARLIE_001',
     'seed-req-id-demo-004',
     'insufficient_balance',
     '{"scope": "transfer:create", "requested_amount": 9999999999, "available": 1500000000, "note": "seed demo data"}',
     NOW() - INTERVAL '1 day')
ON CONFLICT (id) DO NOTHING;
