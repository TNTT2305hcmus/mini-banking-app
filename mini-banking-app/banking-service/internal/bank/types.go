package bank

// ServiceTicketPlaintext is the decrypted Ticket_V contract the KDC TGS issues
// and the bank consumes. The JSON tags MUST stay byte-for-byte identical to the
// KDC's producer type (kdc-service/internal/kdc/types.go: ServiceTicketPlaintext);
// any divergence silently breaks ticket parsing between the two services.
//
// KCV is the client-to-service session key (K_c_v); encoding/json marshals it as
// a base64 string, matching how the KDC emits it.
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

// Authenticator is the AP-exchange authenticator the client encrypts with the
// client-to-service session key (K_c_v) and the bank decrypts in authorize().
// It is the canonical contract the (future) frontend MUST emit; the JSON tags
// follow the Kerberos naming used across the system (ts_3 = authenticator
// timestamp). Keep these in sync with whatever the frontend produces.
type Authenticator struct {
	ClientID  string `json:"client_id"`
	Nonce     string `json:"nonce"`
	RequestID string `json:"request_id"`
	Timestamp int64  `json:"ts_3"`
}
