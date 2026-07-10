# Admin CA curl examples

Set these variables first:

```bash
BASE_URL=http://localhost:3000
ADMIN_CA_TOKEN=<cert-backed-admin-ca-session-token>
REQUEST_ID=11111111-1111-4111-8111-111111111111
```

1. Create a cert-backed session and copy `data.token`.

The browser flow at `/admin-ca` does this after the CA Admin cert has been
activated. For a raw API test, sign a fresh challenge with the local private key
matching the CA Admin certificate and call:

```bash
curl -s -X POST "$BASE_URL/v1/admin-ca/session" \
  -H "Content-Type: application/json" \
  -H "X-Request-ID: $REQUEST_ID" \
  -d '{"cert_serial":"<ca-admin-cert-serial>","challenge":"<challenge>","signature":"<base64-signature>"}'
```

2. List certificates:

```bash
curl -s "$BASE_URL/v1/admin-ca/certificates?limit=20&offset=0" \
  -H "Authorization: Bearer $ADMIN_CA_TOKEN" \
  -H "X-Request-ID: $REQUEST_ID"
```

3. Filter active certificates:

```bash
curl -s "$BASE_URL/v1/admin-ca/certificates?status=active&limit=20&offset=0" \
  -H "Authorization: Bearer $ADMIN_CA_TOKEN" \
  -H "X-Request-ID: $REQUEST_ID"
```

4. Search by email:

```bash
curl -s "$BASE_URL/v1/admin-ca/certificates?email=alice@example.com&limit=20&offset=0" \
  -H "Authorization: Bearer $ADMIN_CA_TOKEN" \
  -H "X-Request-ID: $REQUEST_ID"
```

5. Get certificate detail:

```bash
curl -s "$BASE_URL/v1/admin-ca/certificates/$CERT_SERIAL" \
  -H "Authorization: Bearer $ADMIN_CA_TOKEN" \
  -H "X-Request-ID: $REQUEST_ID"
```

6. Revoke an active certificate:

```bash
curl -s -X POST "$BASE_URL/v1/admin-ca/certificates/$CERT_SERIAL/revoke" \
  -H "Authorization: Bearer $ADMIN_CA_TOKEN" \
  -H "Content-Type: application/json" \
  -H "X-Request-ID: $REQUEST_ID" \
  -d '{"reason":"operator_request"}'
```

7. Revoke the same certificate again, expected HTTP 409:

```bash
curl -i -X POST "$BASE_URL/v1/admin-ca/certificates/$CERT_SERIAL/revoke" \
  -H "Authorization: Bearer $ADMIN_CA_TOKEN" \
  -H "Content-Type: application/json" \
  -H "X-Request-ID: $REQUEST_ID" \
  -d '{"reason":"operator_request"}'
```
