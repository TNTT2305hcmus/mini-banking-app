package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"
)

type User struct {
	ID       string
	Email    string
	FullName string
}

type Account struct {
	ID            string
	UserID        string
	Number        string
	Balance       int64
	DailyLimit    int64
	Currency      string
	Status        string
}

type Transaction struct {
	ID         string
	FromUserID string
	FromAccID  string
	ToAccID    string
	FromNumber string
	ToNumber   string
	Amount     int64
	Currency   string
	Status     string
	Desc       string
	CertSerial string
	Nonce      string
	IdemKey    string
	CreatedAt  time.Time
}

type AuditEvent struct {
	ID        string
	Action    string
	UserID    string
	AccountID string
	TxID      string
	CertSN    string
	RequestID string
	Reason    string
	Metadata  string
	CreatedAt time.Time
}

func sha256Hash(data string) string {
	h := sha256.New()
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

func main() {
	r := rand.New(rand.NewSource(42)) // Fixed seed for determinism

	// 1. Generate 20 Users using valid RFC 4122 UUID v4 formatting (with -4000-8000-)
	users := []User{
		{"d0000000-0000-4000-8000-000000000001", "nguyen.an@demo.local", "Nguyễn Văn An"},
		{"d0000000-0000-4000-8000-000000000002", "tran.binh@demo.local", "Trần Thị Bình"},
		{"d0000000-0000-4000-8000-000000000003", "le.cuong@demo.local", "Lê Hoàng Cường"},
		{"d0000000-0000-4000-8000-000000000004", "pham.duc@demo.local", "Phạm Minh Đức"},
		{"d0000000-0000-4000-8000-000000000005", "hoang.hai@demo.local", "Hoàng Thanh Hải"},
		{"d0000000-0000-4000-8000-000000000006", "vu.huong@demo.local", "Vũ Thị Hương"},
		{"d0000000-0000-4000-8000-000000000007", "ngo.khang@demo.local", "Ngô Minh Khang"},
		{"d0000000-0000-4000-8000-000000000008", "do.lien@demo.local", "Đỗ Hồng Liên"},
		{"d0000000-0000-4000-8000-000000000009", "bui.nam@demo.local", "Bùi Quang Nam"},
		{"d0000000-0000-4000-8000-000000000010", "phan.nhung@demo.local", "Phan Tuyết Nhung"},
		{"d0000000-0000-4000-8000-000000000011", "duong.oai@demo.local", "Dương Quốc Oai"},
		{"d0000000-0000-4000-8000-000000000012", "dang.phuong@demo.local", "Đặng Thu Phương"},
		{"d0000000-0000-4000-8000-000000000013", "dinh.quyet@demo.local", "Đinh Tiến Quyết"},
		{"d0000000-0000-4000-8000-000000000014", "lam.son@demo.local", "Lâm Gia Sơn"},
		{"d0000000-0000-4000-8000-000000000015", "nguyen.trang@demo.local", "Nguyễn Mai Trang"},
		{"d0000000-0000-4000-8000-000000000016", "tran.uy@demo.local", "Trần Quốc Uy"},
		{"d0000000-0000-4000-8000-000000000017", "le.van@demo.local", "Lê Cẩm Vân"},
		{"d0000000-0000-4000-8000-000000000018", "pham.xuan@demo.local", "Phạm Minh Xuân"},
		{"d0000000-0000-4000-8000-000000000019", "hoang.yen@demo.local", "Hoàng Thế Yên"},
		{"d0000000-0000-4000-8000-000000000020", "trinh.son@demo.local", "Trịnh Công Sơn"},
	}

	// 2. Generate 20 Accounts using prefix "d1000000-0000-4000-8000-"
	accounts := make([]Account, len(users))
	for i, u := range users {
		balance := int64(5_000_000 + r.Intn(145)*1_000_000)
		accounts[i] = Account{
			ID:         fmt.Sprintf("d1000000-0000-4000-8000-%012d", i+1),
			UserID:     u.ID,
			Number:     fmt.Sprintf("11000000%04d", i+1),
			Balance:    balance,
			DailyLimit: 50_000_000,
			Currency:   "VND",
			Status:     "active",
		}
	}

	// 3. Generate 50 Transactions using prefix "d2000000-0000-4000-8000-"
	descriptions := []string{
		"Chuyển tiền ăn trưa", "Thanh toán tiền nhà tháng này", "Trả tiền nước mía",
		"Mua cà phê sáng", "Tạp hóa cô Ba", "Chuyển khoản trả nợ mua đồ hộ",
		"Đóng quỹ lớp tháng", "Thanh toán hóa đơn điện nước", "Mua bánh kem sinh nhật",
		"Mua sách giáo khoa", "Nạp tiền điện thoại", "Trả tiền vé xem phim",
		"Đặt đồ ăn trưa ShopeeFood", "Mua quần áo mới", "Chuyển tiền mừng cưới",
	}

	txs := make([]Transaction, 50)
	now := time.Now().UTC()

	for i := 0; i < 50; i++ {
		fromIdx := r.Intn(len(accounts))
		toIdx := r.Intn(len(accounts))
		for toIdx == fromIdx {
			toIdx = r.Intn(len(accounts))
		}

		fromAcc := accounts[fromIdx]
		toAcc := accounts[toIdx]

		status := "completed"
		amount := int64(50_000 + r.Intn(200)*10_000) // 50k to 2M VND
		desc := descriptions[r.Intn(len(descriptions))]

		if i%5 == 0 {
			status = "failed"
			if r.Intn(2) == 0 {
				amount = fromAcc.Balance + 10_000_000 // Overdraft
				desc = desc + " (Lỗi: Vượt quá số dư)"
			} else {
				amount = 60_000_000 // Over daily limit
				desc = desc + " (Lỗi: Vượt hạn mức ngày)"
			}
		}

		if status == "completed" {
			accounts[fromIdx].Balance -= amount
			accounts[toIdx].Balance += amount
		}

		txs[i] = Transaction{
			ID:         fmt.Sprintf("d2000000-0000-4000-8000-%012d", i+1),
			FromUserID: fromAcc.UserID,
			FromAccID:  fromAcc.ID,
			ToAccID:    toAcc.ID,
			FromNumber: fromAcc.Number,
			ToNumber:   toAcc.Number,
			Amount:     amount,
			Currency:   "VND",
			Status:     status,
			Desc:       desc,
			CertSerial: fmt.Sprintf("SEED_CERT_SERIAL_USER_%03d", fromIdx+1),
			Nonce:      fmt.Sprintf("nonce-demo-txn-%03d-placeholder", i+1),
			IdemKey:    fmt.Sprintf("idem-key-demo-txn-%03d-placeholder", i+1),
			CreatedAt:  now.Add(-time.Duration(50-i) * time.Hour),
		}
	}

	// 4. Generate Audit Logs using prefix "d3000000-0000-4000-8000-"
	audits := make([]AuditEvent, 0)
	auditIndex := 1

	for i, t := range txs {
		if t.Status == "completed" {
			audits = append(audits, AuditEvent{
				ID:        fmt.Sprintf("d3000000-0000-4000-8000-%012d", auditIndex),
				Action:    "transfer_completed",
				UserID:    t.FromUserID,
				AccountID: t.FromAccID,
				TxID:      t.ID,
				CertSN:    t.CertSerial,
				RequestID: fmt.Sprintf("req-id-completed-%03d", i+1),
				Reason:    "ok",
				Metadata:  fmt.Sprintf("{\"scope\": \"transfer:create\", \"amount\": %d}", t.Amount),
				CreatedAt: t.CreatedAt.Add(2 * time.Second),
			})
			auditIndex++
		} else {
			action := "insufficient_funds"
			reason := "insufficient_balance"
			metadata := fmt.Sprintf("{\"scope\": \"transfer:create\", \"amount\": %d, \"error\": \"insufficient_balance\"}", t.Amount)
			if t.Amount >= 50_000_000 {
				action = "transfer_rejected"
				reason = "daily_limit_exceeded"
				metadata = fmt.Sprintf("{\"scope\": \"transfer:create\", \"amount\": %d, \"error\": \"daily_limit_exceeded\"}", t.Amount)
			}
			audits = append(audits, AuditEvent{
				ID:        fmt.Sprintf("d3000000-0000-4000-8000-%012d", auditIndex),
				Action:    action,
				UserID:    t.FromUserID,
				AccountID: t.FromAccID,
				TxID:      t.ID,
				CertSN:    t.CertSerial,
				RequestID: fmt.Sprintf("req-id-failed-%03d", i+1),
				Reason:    reason,
				Metadata:  metadata,
				CreatedAt: t.CreatedAt.Add(1 * time.Second),
			})
			auditIndex++
		}
	}

	// Add 5 more security audit logs for diversity
	extraAudits := []struct {
		Action   string
		Reason   string
		Metadata string
	}{
		{"replay_detected", "replay_detected", "{\"nonce\": \"replay-nonce-001\", \"client_id\": \"d0000000-0000-4000-8000-000000000003\"}"},
		{"invalid_signature", "invalid_signature", "{\"client_id\": \"d0000000-0000-4000-8000-000000000005\", \"details\": \"signature verification failed\"}"},
		{"certificate_rejected", "certificate_rejected", "{\"cert_serial\": \"EXPIRED_SERIAL_999\", \"reason\": \"expired\"}"},
		{"forbidden_ownership", "forbidden_ownership", "{\"account_id\": \"d1000000-0000-4000-8000-000000000008\", \"client_id\": \"d0000000-0000-4000-8000-000000000001\"}"},
		{"replay_detected", "replay_detected", "{\"nonce\": \"replay-nonce-002\", \"client_id\": \"d0000000-0000-4000-8000-000000000010\"}"},
	}

	for i, ex := range extraAudits {
		audits = append(audits, AuditEvent{
			ID:        fmt.Sprintf("d3000000-0000-4000-8000-%012d", auditIndex),
			Action:    ex.Action,
			UserID:    "",
			AccountID: "",
			TxID:      "",
			CertSN:    "SEED_CERT_SERIAL_SYSTEM",
			RequestID: fmt.Sprintf("req-id-extra-%03d", i+1),
			Reason:    ex.Reason,
			Metadata:  ex.Metadata,
			CreatedAt: now.Add(-time.Duration(10+i) * time.Minute),
		})
		auditIndex++
	}

	// 5. Calculate HASH CHAINS
	// 5a. Calculate Transaction Chain
	prevTxHash := "genesis"
	txSQL := make([]string, len(txs))
	for i, t := range txs {
		fields := []string{
			t.FromNumber,
			t.ToNumber,
			fmt.Sprintf("%d", t.Amount),
			t.Currency,
			t.Status,
			t.Desc,
			t.CertSerial,
			"transfer:create",
			t.Nonce,
			t.IdemKey,
		}
		dataToHash := prevTxHash
		for _, f := range fields {
			dataToHash += f
		}
		currentHash := sha256Hash(dataToHash)

		txSQL[i] = fmt.Sprintf("    ('%s', '%s', '%s', '%s', '%s', %d, '%s', '%s', '%s', '%s', 'SEED_DEMO_PLACEHOLDER', '%s', 'transfer:create', '%s', '%s', '%s', '%s', '%s', '%s')",
			t.ID, t.FromAccID, t.ToAccID, t.FromNumber, t.ToNumber, t.Amount, t.Currency, t.Status,
			t.Desc, sha256Hash(t.Desc), t.CertSerial, t.Nonce, t.IdemKey, prevTxHash, currentHash,
			t.CreatedAt.Format("2006-01-02 15:04:05.000000+00"), t.CreatedAt.Format("2006-01-02 15:04:05.000000+00"),
		)
		prevTxHash = currentHash
	}

	// 5b. Calculate Audit Chain
	prevAuditHash := "genesis"
	auditSQL := make([]string, len(audits))
	for i, a := range audits {
		fields := []string{
			a.Action,
			a.UserID,
			a.AccountID,
			a.TxID,
			a.CertSN,
			a.RequestID,
			a.Reason,
		}
		dataToHash := prevAuditHash
		for _, f := range fields {
			dataToHash += f
		}
		currentHash := sha256Hash(dataToHash)

		userIDVal := fmt.Sprintf("'%s'", a.UserID)
		if a.UserID == "" {
			userIDVal = "NULL"
		}
		accIDVal := fmt.Sprintf("'%s'", a.AccountID)
		if a.AccountID == "" {
			accIDVal = "NULL"
		}
		txIDVal := fmt.Sprintf("'%s'", a.TxID)
		if a.TxID == "" {
			txIDVal = "NULL"
		}

		auditSQL[i] = fmt.Sprintf("    ('%s', '%s', %s::uuid, %s::uuid, %s::uuid, '%s', '%s', '%s', '%s', '%s', %d, '%s', '%s')",
			a.ID, a.Action, userIDVal, accIDVal, txIDVal, a.CertSN, a.RequestID, a.Reason, a.Metadata,
			a.CreatedAt.Format("2006-01-02 15:04:05.000000+00"), i+1, prevAuditHash, currentHash,
		)
		prevAuditHash = currentHash
	}

	// 6. Generate final SQL string
	var sb strings.Builder
	sb.WriteString("-- =============================================================================\n")
	sb.WriteString("-- Mini Banking — DEMO SEED DATA (Bank DB) - 20 Users, 50 Transactions & Audit Logs\n")
	sb.WriteString("-- File: db/bank/seed_demo.sql\n")
	sb.WriteString("-- Generated automatically by scratch script to ensure mathematically correct hash-chains.\n")
	sb.WriteString("-- =============================================================================\n\n")

	sb.WriteString("CREATE EXTENSION IF NOT EXISTS \"uuid-ossp\";\n\n")

	sb.WriteString("-- CLEAN UP OLD DEMO SEED DATA TO ENSURE CLEAN STATE (Ordered to respect foreign key constraints)\n")
	sb.WriteString("DELETE FROM bank_audit_log WHERE id::text LIKE 'e0000000-%' OR id::text LIKE 'd3000000-%';\n")
	sb.WriteString("DELETE FROM transactions WHERE id::text LIKE 'd1000000-%' OR id::text LIKE 'd2000000-%' OR id::text LIKE 'd3000000-%' OR id::text LIKE 't0000000-%';\n")
	sb.WriteString("DELETE FROM accounts WHERE id::text LIKE 'a0000000-%' OR id::text LIKE 'b0000000-%' OR id::text LIKE 'c0000000-%' OR id::text LIKE 'd1000000-%';\n")
	sb.WriteString("DELETE FROM users WHERE id::text LIKE 'a0000000-%' OR id::text LIKE 'b0000000-%' OR id::text LIKE 'c0000000-%' OR id::text LIKE 'd0000000-%';\n\n")

	sb.WriteString("-- =============================================================================\n")
	sb.WriteString("-- USERS (20 Users)\n")
	sb.WriteString("-- =============================================================================\n")
	sb.WriteString("INSERT INTO users (id, email, full_name, status, created_at, updated_at)\nVALUES\n")
	for i, u := range users {
		comma := ","
		if i == len(users)-1 {
			comma = ";"
		}
		sb.WriteString(fmt.Sprintf("    ('%s', '%s', '%s', 'active', NOW() - INTERVAL '30 days', NOW() - INTERVAL '30 days')%s\n", u.ID, u.Email, u.FullName, comma))
	}
	sb.WriteString("\n")

	sb.WriteString("-- =============================================================================\n")
	sb.WriteString("-- ACCOUNTS (20 Accounts - 1 per User)\n")
	sb.WriteString("-- =============================================================================\n")
	sb.WriteString("INSERT INTO accounts (id, user_id, account_number, balance, daily_transfer_limit, currency, status, created_at, updated_at)\nVALUES\n")
	for i, a := range accounts {
		comma := ","
		if i == len(accounts)-1 {
			comma = ";"
		}
		sb.WriteString(fmt.Sprintf("    ('%s', '%s', '%s', %d, %d, '%s', '%s', NOW() - INTERVAL '30 days', NOW() - INTERVAL '1 day')%s\n", a.ID, a.UserID, a.Number, a.Balance, a.DailyLimit, a.Currency, a.Status, comma))
	}
	sb.WriteString("\n")

	sb.WriteString("-- =============================================================================\n")
	sb.WriteString("-- TRANSACTIONS (50 Transactions - featuring completed and failed with hash-chains)\n")
	sb.WriteString("-- =============================================================================\n")
	sb.WriteString("INSERT INTO ledger_state (id, last_hash)\nVALUES ('main', 'genesis')\nON CONFLICT (id) DO NOTHING;\n\n")
	sb.WriteString("INSERT INTO transactions (id, from_account_id, to_account_id, from_account_number, to_account_number, amount, currency, status, description, payload_hash, client_signature, cert_serial, scope, nonce, idempotency_key, previous_hash, current_hash, created_at, completed_at)\nVALUES\n")
	sb.WriteString(strings.Join(txSQL, ",\n") + ";\n\n")

	sb.WriteString("UPDATE ledger_state\n")
	sb.WriteString(fmt.Sprintf("SET last_hash = '%s',\n    last_transaction_id = '%s',\n    updated_at = NOW()\n", prevTxHash, txs[len(txs)-1].ID))
	sb.WriteString("WHERE id = 'main';\n\n")

	sb.WriteString("-- =============================================================================\n")
	sb.WriteString("-- AUDIT LOGS (55 events with hash-chain verification)\n")
	sb.WriteString("-- =============================================================================\n")
	sb.WriteString("INSERT INTO bank_audit_log (id, action, user_id, account_id, transaction_id, cert_serial, request_id, reason, metadata, created_at, seq, prev_hash, hash)\nVALUES\n")
	sb.WriteString(strings.Join(auditSQL, ",\n") + ";\n")

	// Write to file
	err := os.WriteFile("c:/Users/quoct/Repositories/MaHoaUngDung/mini-banking-app/mini-banking-app/db/bank/seed_demo.sql", []byte(sb.String()), 0644)
	if err != nil {
		fmt.Printf("Error writing file: %v\n", err)
		return
	}
	fmt.Println("seed_demo.sql generated successfully with valid UUID v4 format!")
}
