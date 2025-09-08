module github.com/loqalabs/loqa-hub/tests/integration

go 1.25.1

require (
	github.com/loqalabs/loqa-proto/go v0.0.20
	google.golang.org/grpc v1.75.0
)

require (
	golang.org/x/net v0.43.0 // indirect
	golang.org/x/sys v0.35.0 // indirect
	golang.org/x/text v0.28.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250707201910-8d1bb00bc6a7 // indirect
	google.golang.org/protobuf v1.36.8 // indirect
)

// Development mode: Using local proto changes for integration testing
replace github.com/loqalabs/loqa-proto/go => ../../../loqa-proto/go
