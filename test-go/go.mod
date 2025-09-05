module github.com/loqalabs/loqa-puck/test-go

go 1.23.0

toolchain go1.24.3

replace github.com/loqalabs/loqa-proto/go => ../../loqa-proto/go

require (
	github.com/gordonklaus/portaudio v0.0.0-20250206071425-98a94950218b
	github.com/loqalabs/loqa-proto/go v0.0.0-00010101000000-000000000000
	google.golang.org/grpc v1.75.0
)

require (
	golang.org/x/net v0.43.0 // indirect
	golang.org/x/sys v0.35.0 // indirect
	golang.org/x/text v0.28.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250707201910-8d1bb00bc6a7 // indirect
	google.golang.org/protobuf v1.36.7 // indirect
)
