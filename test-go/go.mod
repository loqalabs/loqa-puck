module github.com/loqalabs/loqa-puck/test-go

go 1.22.2

replace github.com/loqalabs/loqa-proto/go => ../../loqa-proto/go

require (
	github.com/gordonklaus/portaudio v0.0.0-20250206071425-98a94950218b
	github.com/loqalabs/loqa-proto/go v0.0.0-00010101000000-000000000000
	google.golang.org/grpc v1.65.0
)

require (
	golang.org/x/net v0.26.0 // indirect
	golang.org/x/sys v0.21.0 // indirect
	golang.org/x/text v0.16.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240604185151-ef581f913117 // indirect
	google.golang.org/protobuf v1.34.2 // indirect
)
