go mod tidy
go run main.go
# 1 - DONE
- TicketV (tại bank) trong file handler.go hàm giải mã và ServiceTicketPlaintext trong file tgs_service.go của KDC phải khớp với nhau:
    - Bên bank: 
    clientID := firstString(m, "client_id", "id_c", "ID_c")
	certSN := firstString(m, "cert_sn", "cert_serial", "serial_number")
	scope := firstString(m, "scope")
	publicKeyPEM := firstString(m, "pub_c_pem", "pub_c", "public_key_pem")
	kcv, err := firstBytes(m, "k_c_v", "K_c_v", "kcv", "KCV") 
    - Bên KDC:
    type ServiceTicketPlaintext struct {
	ClientID  string `json:"client_id"`
	ServiceID string `json:"service_id"`
	SName     string `json:"sname"`
	KCV       []byte `json:"k_c_v"`
	PublicKey string `json:"pub_c"`
	PubCPEM   string `json:"pub_c_pem"`
	CertSN    string `json:"cert_sn"`
	Scope     string `json:"scope"`
	NonceReq  string `json:"nonce_req"`
	IssuedAt  int64  `json:"issued_at"`
	ExpiresAt int64  `json:"expires_at"`
    }
    - Cần bổ sung file type.go ở thư mục bank để thống nhất 2 bên

# 2 - DONE
- Cần thống nhất Authenticator giữa Frontend và Bank giải mã Authenticator trong hàm Authorize
- Trong bank:
    // FIX CHỖ NÀY
	clientID := firstString(authMap, "client_id", "id_c", "ID_c")
	if clientID != ticket.clientID {
		h.bank.Audit(ctx, bank.AuditEvent{Action: "transfer_rejected", UserID: ticket.clientID, CertSerial: ticket.certSN, Reason: "authenticator_client_mismatch"})
		return out, status.Error(codes.Unauthenticated, "INVALID_AUTHENTICATOR")
	}
	out.nonce = firstString(authMap, "nonce", "nonce_req")
	out.requestID = firstString(authMap, "request_id", "requestId")
	ts, ok := firstUnix(authMap, "ts_3", "timestamp", "timestamp_unix", "ts")
	if !ok || out.nonce == "" || out.requestID == "" {
		h.bank.Audit(ctx, bank.AuditEvent{Action: "transfer_rejected", UserID: ticket.clientID, CertSerial: ticket.certSN, RequestID: out.requestID, Reason: "authenticator_missing_fields"})
		return out, status.Error(codes.InvalidArgument, "BAD_REQUEST")
	}
- Bổ sung thêm struct trong types.ts của bank gồm clientID, nonce, requestID, timestamp

# 3 - DONE
- Hiện tại trong file handler đang lẫn lộn khá nhiều struct, hãy chuyển bớt sang file tyes.go

# 4 - DONE
- Chuyển logic của auth sang một file auth.ts