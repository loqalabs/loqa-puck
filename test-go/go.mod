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
	github.com/loqalabs/loqa-proto/go v0.0.19
	golang.org/x/net v0.43.0 // indirect
	golang.org/x/sys v0.35.0 // indirect
	golang.org/x/text v0.28.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250707201910-8d1bb00bc6a7 // indirect
	google.golang.org/protobuf v1.36.8 // indirect
)
