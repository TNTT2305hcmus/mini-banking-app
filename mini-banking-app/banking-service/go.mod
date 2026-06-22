module mini-banking/banking-service

go 1.25.4

require (
	github.com/DATA-DOG/go-sqlmock v1.5.2
	github.com/jackc/pgx/v5 v5.5.0
	github.com/joho/godotenv v1.5.1
	github.com/redis/go-redis/v9 v9.19.0
	google.golang.org/grpc v1.81.1
	mini-banking/pkg v0.0.0-00010101000000-000000000000
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20221227161230-091c0ba34f0a // indirect
	github.com/jackc/puddle/v2 v2.2.1 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/crypto v0.48.0 // indirect
	golang.org/x/net v0.51.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/text v0.34.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260226221140-a57be14db171 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

replace mini-banking/pkg => ../pkg
