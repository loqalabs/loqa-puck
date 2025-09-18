module github.com/loqalabs/loqa-relay/test-go

go 1.25.1

replace (
	golang.org/x/net => golang.org/x/net v0.19.0
	golang.org/x/sys => golang.org/x/sys v0.15.0
)

require (
	github.com/gordonklaus/portaudio v0.0.0-20250206071425-98a94950218b
	google.golang.org/grpc v1.75.0
)

require (
	github.com/hajimehoshi/go-mp3 v0.3.4 // indirect
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/nats-io/nats.go v1.45.0 // indirect
	github.com/nats-io/nkeys v0.4.11 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	golang.org/x/crypto v0.39.0 // indirect
)

// Development mode: Uncomment the line below to use local proto changes for testing
// replace github.com/loqalabs/loqa-proto/go => ../../loqa-proto/go

require (
	github.com/loqalabs/loqa-proto/go v0.0.20
	golang.org/x/net v0.43.0 // indirect
	golang.org/x/sys v0.35.0 // indirect
	golang.org/x/text v0.28.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250707201910-8d1bb00bc6a7 // indirect
	google.golang.org/protobuf v1.36.8 // indirect
)
