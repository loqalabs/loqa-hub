module github.com/loqalabs/loqa-hub/tests/e2e

go 1.25.1

require (
	github.com/loqalabs/loqa-hub v0.0.0
	github.com/nats-io/nats.go v1.45.0
)

require (
	github.com/google/uuid v1.6.0 // indirect
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/nats-io/nkeys v0.4.11 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	go.uber.org/multierr v1.10.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
	golang.org/x/crypto v0.41.0 // indirect
	golang.org/x/sys v0.35.0 // indirect
)

replace github.com/loqalabs/loqa-hub => ../..
